package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// LotteryService 抽奖业务逻辑。
//
// 并发与一致性模型(单机 Postgres):
//   - 同一用户的抽奖请求通过 lottery_user_stats 行级 FOR UPDATE 串行化,
//     次数判定与流水写入在同一事务内完成,重复/并发请求不会重复扣次数;
//   - 奖品库存用条件 UPDATE 原子扣减,不会超发;
//   - 中奖结果只由服务端生成(crypto/rand 加权随机)。
//
// 发放模型:
//   - 抽奖事务只写流水(fulfillment_status = pending);
//   - 事务提交后同步执行发放: none/quota 直接标记 granted,
//     balance_bonus 调 AdjustBalance,失败标记 failed 可由管理端重试,
//     绝不把失败伪装成成功。
type LotteryService struct {
	repo        LotteryRepository
	userRepo    LotteryUserSource
	entClient   *dbent.Client
	nowFunc     func() time.Time
	billingCache LotteryBalanceCacheInvalidator // 可为 nil(本地测试)
}

// NewLotteryService 构造抽奖服务。
func NewLotteryService(repo LotteryRepository, userRepo LotteryUserSource, entClient *dbent.Client, billingCache LotteryBalanceCacheInvalidator) *LotteryService {
	return &LotteryService{
		repo:         repo,
		userRepo:     userRepo,
		entClient:    entClient,
		nowFunc:      time.Now,
		billingCache: billingCache,
	}
}

// SetNowFunc 覆盖时间源(测试用)。
func (s *LotteryService) SetNowFunc(f func() time.Time) {
	if f != nil {
		s.nowFunc = f
	}
}

// ---------------------------------------------------------------
// 用户端
// ---------------------------------------------------------------

// LotteryActivityView 用户端活动视图: 活动 + 奖品列表 + 阶梯分组概率。
type LotteryActivityView struct {
	Activity     LotteryActivity    `json:"-"`
	Prizes       []LotteryPrizeView `json:"prizes"`
	Tiers        []LotteryTierView  `json:"tiers"`
	RulesVersion int                `json:"rules_version"`
	IsOpen       bool               `json:"is_open"`
	ServerTime   time.Time          `json:"server_time"`
}

// LotteryPrizeView 用户端奖品视图。Probability 为基础权重占比
// (未指定阶梯的兜底展示;按阶梯概率见 Tiers)。
type LotteryPrizeView struct {
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	PrizeType   string         `json:"prize_type"`
	Value       float64        `json:"value"`
	Weight      int            `json:"weight"`
	MinTier     int            `json:"min_tier"`
	TierWeights map[string]int `json:"tier_weights,omitempty"`
	Stock       int            `json:"stock"`
	StockIssued int            `json:"stock_issued"`
	Probability float64        `json:"probability"`
}

// LotteryTierView 单个阶梯的奖池视图: 该阶梯下各候选奖品的归一化概率。
type LotteryTierView struct {
	Tier       int64                     `json:"tier"`
	Name       string                    `json:"name"`
	Threshold  float64                   `json:"threshold"` // 进入该阶梯的累计消耗门槛(美元)
	Prizes     []LotteryTierPrizeChance  `json:"prizes"`
}

// LotteryTierPrizeChance 阶梯内单奖品概率。PrizeID 为 0 表示该阶梯无候选。
type LotteryTierPrizeChance struct {
	PrizeID     int64   `json:"prize_id"`
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
	PrizeType   string  `json:"prize_type"`
	Probability float64 `json:"probability"`
}

// GetUserActivityView 返回当前活动与奖品展示视图。
func (s *LotteryService) GetUserActivityView(ctx context.Context) (*LotteryActivityView, error) {
	activity, err := s.repo.GetLatestActivity(ctx)
	if err != nil {
		if errors.Is(err, ErrLotteryActivityNotFound) {
			return nil, ErrLotteryNoActiveActivity
		}
		return nil, err
	}
	prizes, err := s.repo.ListPrizesByActivity(ctx, activity.ID, true)
	if err != nil {
		return nil, err
	}

	now := s.nowFunc()
	view := &LotteryActivityView{
		Activity:     *activity,
		Prizes:       make([]LotteryPrizeView, 0, len(prizes)),
		Tiers:        make([]LotteryTierView, 0, int(LotteryMaxTier)+1),
		RulesVersion: activity.RulesVersion,
		IsOpen:       activity.IsOpen(now),
		ServerTime:   now,
	}
	// 基础权重占比(兜底展示,不指定阶梯时使用)。
	totalWeight := 0
	for i := range prizes {
		p := &prizes[i]
		if p.Weight > 0 && p.InStock() {
			totalWeight += p.Weight
		}
	}
	for i := range prizes {
		p := prizes[i]
		prob := 0.0
		if p.Weight > 0 && totalWeight > 0 && p.InStock() {
			prob = float64(p.Weight) / float64(totalWeight) * 100
		}
		view.Prizes = append(view.Prizes, LotteryPrizeView{
			ID:          p.ID,
			Name:        p.Name,
			PrizeType:   p.PrizeType,
			Value:       p.Value,
			Weight:      p.Weight,
			MinTier:     p.MinTier,
			TierWeights: p.TierWeights,
			Stock:       p.Stock,
			StockIssued: p.StockIssued,
			Probability: prob,
		})
	}
	// 阶梯分组概率: 与 pickPrizeWithStock 同口径
	// (候选 = min_tier 达标 + 有效权重>0 + 有库存,概率按有效权重归一)。
	for tier := int64(0); tier <= LotteryMaxTier; tier++ {
		tv := LotteryTierView{
			Tier:      tier,
			Name:      LotteryTierName(tier),
			Threshold: BalanceSpentCentsToFloat(LotteryTierThresholdCents(tier)),
			Prizes:    make([]LotteryTierPrizeChance, 0, len(prizes)),
		}
		tierTotal := 0
		for i := range prizes {
			p := &prizes[i]
			if p.AvailableAtTier(tier) && p.InStock() {
				tierTotal += p.EffectiveWeight(tier)
			}
		}
		for i := range prizes {
			p := &prizes[i]
			if !p.AvailableAtTier(tier) || !p.InStock() {
				continue
			}
			w := p.EffectiveWeight(tier)
			prob := 0.0
			if w > 0 && tierTotal > 0 {
				prob = float64(w) / float64(tierTotal) * 100
			}
			tv.Prizes = append(tv.Prizes, LotteryTierPrizeChance{
				PrizeID:     p.ID,
				Name:        p.Name,
				Value:       p.Value,
				PrizeType:   p.PrizeType,
				Probability: prob,
			})
		}
		view.Tiers = append(view.Tiers, tv)
	}
	return view, nil
}

// ensureUserStatsForUpdate 事务内获取(或首次创建)用户抽奖状态行。
//
// 首次创建时按用户当前状态决定是否发放首抽资格: 仅 active 且未删除的
// 正常账号获得 first_draw_granted = true。
func (s *LotteryService) ensureUserStatsForUpdate(ctx context.Context, userID int64) (*LotteryUserStats, error) {
	stats, err := s.repo.GetUserStatsForUpdate(ctx, userID)
	if err == nil {
		return stats, nil
	}
	if !errors.Is(err, ErrLotteryStatsNotFound) {
		return nil, err
	}

	// 不存在: 校验用户状态后创建首实行。
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Status != StatusActive || user.DeletedAt != nil {
		return nil, ErrLotteryUserNotEligible
	}
	created, err := s.repo.CreateUserStats(ctx, userID, true, s.nowFunc())
	if err != nil {
		if errors.Is(err, ErrLotteryStatsExists) {
			// 并发首次访问: 另一个请求已插入,回滚当前事务语义无法继续
			// FOR UPDATE 等待,由调用方重试整个事务。
			return nil, ErrLotteryStatsExists
		}
		return nil, err
	}
	return created, nil
}

// ensureUserStats 非事务版本(管理端/状态查询用): 不存在则创建,唯一冲突时回读。
func (s *LotteryService) ensureUserStats(ctx context.Context, userID int64) (*LotteryUserStats, error) {
	stats, err := s.repo.GetUserStats(ctx, userID)
	if err == nil {
		return stats, nil
	}
	if !errors.Is(err, ErrLotteryStatsNotFound) {
		return nil, err
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Status != StatusActive || user.DeletedAt != nil {
		return nil, ErrLotteryUserNotEligible
	}
	created, err := s.repo.CreateUserStats(ctx, userID, true, s.nowFunc())
	if err != nil {
		if errors.Is(err, ErrLotteryStatsExists) {
			return s.repo.GetUserStats(ctx, userID)
		}
		return nil, err
	}
	return created, nil
}

// computeUserDraws 计算用户的资格与剩余次数。
//
//	eligible = first(0/1) + threshold_entitlement + manual
//	used     = 流水计数
//	available= max(eligible - used, 0)
func (s *LotteryService) computeUserDraws(ctx context.Context, userID int64, stats *LotteryUserStats, rulesVersion int) (*LotteryUserStatus, error) {
	spentF, err := s.repo.SumBalanceSpentSince(ctx, userID, stats.BaselineAt)
	if err != nil {
		return nil, err
	}
	spentCents := BalanceSpentFloatToCents(spentF) + stats.SpendOffsetCents
	if spentCents < 0 {
		spentCents = 0
	}

	used, err := s.repo.CountDraws(ctx, userID)
	if err != nil {
		return nil, err
	}

	first := int64(0)
	if stats.FirstDrawGranted {
		first = 1
	}
	entitlement := LotteryThresholdEntitlement(spentCents)
	manual := int64(stats.ManualAdjustment)
	eligible := first + entitlement + manual
	usedI64 := int64(used)
	available := eligible - usedI64
	if available < 0 {
		available = 0
	}

	currentTier := LotteryUserTier(spentCents)
	return &LotteryUserStatus{
		AvailableDraws:       available,
		UsedDraws:            usedI64,
		FirstDrawGranted:     stats.FirstDrawGranted,
		ThresholdEntitlement: entitlement,
		ManualAdjustment:     manual,
		BalanceSpentCents:    spentCents,
		NextThresholdCents:   LotteryNextThresholdCents(entitlement),
		CurrentTier:          currentTier,
		TierName:             LotteryTierName(currentTier),
		RulesVersion:         rulesVersion,
	}, nil
}

// GetUserStatus 查询用户抽奖状态(次数/消耗/下一阈值)。
func (s *LotteryService) GetUserStatus(ctx context.Context, userID int64) (*LotteryUserStatus, error) {
	stats, err := s.ensureUserStats(ctx, userID)
	if err != nil {
		return nil, err
	}
	activity, err := s.repo.GetLatestActivity(ctx)
	if err != nil {
		if errors.Is(err, ErrLotteryActivityNotFound) {
			return nil, ErrLotteryNoActiveActivity
		}
		return nil, err
	}
	status, err := s.computeUserDraws(ctx, userID, stats, activity.RulesVersion)
	if err != nil {
		return nil, err
	}
	status.ActivityOpen = activity.IsOpen(s.nowFunc())
	return status, nil
}

// Draw 执行一次抽奖。
//
// 流程:
//  1. 事务内锁定用户状态行,校验资格/活动/剩余次数;
//  2. 幂等键预检,命中直接返回已有结果(不扣次数);
//  3. 加权随机选中奖品并原子扣库存(库存竞争失败自动降级重选);
//  4. 写入抽奖流水(次数来源 first/threshold/manual),提交事务;
//  5. 事务提交后发放奖品,更新发放状态(失败保留 pending/failed 可重试)。
func (s *LotteryService) Draw(ctx context.Context, userID int64, idempotencyKey string) (*LotteryDrawOutcome, error) {
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return nil, err
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Status != StatusActive || user.DeletedAt != nil {
		return nil, ErrLotteryUserNotEligible
	}

	activity, err := s.repo.GetActiveActivity(ctx)
	if err != nil {
		if errors.Is(err, ErrLotteryActivityNotFound) {
			return nil, ErrLotteryNoActiveActivity
		}
		return nil, err
	}
	if !activity.IsOpen(s.nowFunc()) {
		return nil, ErrLotteryNoActiveActivity
	}

	// 事务: 行锁串行化 + 一致写。
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("lottery begin tx: %w", err)
	}
	doRollback := func() {
		if rbErr := tx.Rollback(); rbErr != nil {
			slog.Warn("lottery tx rollback failed", "user_id", userID, "error", rbErr)
		}
	}

	txCtx := dbent.NewTxContext(ctx, tx)
	stats, err := s.ensureUserStatsForUpdate(txCtx, userID)
	if errors.Is(err, ErrLotteryStatsExists) {
		// 并发首次初始化: 另一事务已插入该用户的 stats 行。回滚后重试一次,
		// 第二次 FOR UPDATE 必然能读到已提交的行。
		doRollback()
		return s.Draw(ctx, userID, idempotencyKey)
	}
	if err != nil {
		doRollback()
		return nil, err
	}

	// 幂等预检: 同 key 重试返回已有结果。
	if existing, gErr := s.repo.GetDrawByIdempotencyKey(txCtx, userID, idempotencyKey); gErr == nil && existing != nil {
		doRollback()
		remaining, cErr := s.remainingAfterNoop(ctx, userID)
		if cErr != nil {
			remaining = 0
		}
		return &LotteryDrawOutcome{Record: existing, RemainingDraws: remaining}, nil
	}

	status, err := s.computeUserDraws(txCtx, userID, stats, activity.RulesVersion)
	if err != nil {
		doRollback()
		return nil, err
	}
	if status.AvailableDraws <= 0 {
		doRollback()
		return nil, ErrLotteryNoDrawsLeft
	}

	prizes, err := s.repo.ListPrizesByActivity(txCtx, activity.ID, true)
	if err != nil {
		doRollback()
		return nil, err
	}
	// 首抽保底: 第一次抽奖固定中 $1 余额(运营规则)。保底奖品缺失/停用/
	// 无库存/并发扣减失败时退回正常加权随机,保证抽奖始终可用。
	var prize *LotteryPrize
	if classifyDrawSource(status, stats.FirstDrawGranted) == LotteryDrawSourceFirst {
		prize = pickGuaranteedFirstPrize(txCtx, s.repo, prizes)
	}
	if prize == nil {
		prize, err = pickPrizeWithStock(txCtx, s.repo, prizes, LotteryUserTier(status.BalanceSpentCents))
		if err != nil {
			doRollback()
			return nil, err
		}
	}

	record := &LotteryDrawRecord{
		UserID:             userID,
		ActivityID:         activity.ID,
		PrizeName:          prize.Name,
		PrizeType:          prize.PrizeType,
		PrizeValue:         prize.Value,
		RulesVersion:       activity.RulesVersion,
		Source:             classifyDrawSource(status, stats.FirstDrawGranted),
		BalanceSpentAtDraw: BalanceSpentCentsToFloat(status.BalanceSpentCents),
		FulfillmentStatus:  LotteryFulfillmentPending,
		IdempotencyKey:     idempotencyKey,
	}
	if prize.PrizeType != LotteryPrizeTypeNone {
		pid := prize.ID
		record.PrizeID = &pid
	}
	if err := s.repo.CreateDraw(txCtx, record); err != nil {
		doRollback()
		if errors.Is(err, ErrLotteryDrawIdempotencyConflict) {
			// 并发同 key: 回读已有流水。
			existing, gErr := s.repo.GetDrawByIdempotencyKey(ctx, userID, idempotencyKey)
			if gErr != nil {
				return nil, gErr
			}
			return &LotteryDrawOutcome{Record: existing, RemainingDraws: status.AvailableDraws - 1}, nil
		}
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("lottery commit: %w", err)
	}

	// 事务已提交: 流水是事实,发放失败不回滚抽奖。
	s.fulfillDraw(ctx, record)

	return &LotteryDrawOutcome{
		Record:         record,
		RemainingDraws: status.AvailableDraws - 1,
	}, nil
}

// remainingAfterNoop 幂等命中时计算当前剩余次数(尽力而为,失败按 0 处理)。
func (s *LotteryService) remainingAfterNoop(ctx context.Context, userID int64) (int64, error) {
	stats, err := s.repo.GetUserStats(ctx, userID)
	if err != nil {
		return 0, err
	}
	status, err := s.computeUserDraws(ctx, userID, stats, 0)
	if err != nil {
		return 0, err
	}
	return status.AvailableDraws, nil
}

// classifyDrawSource 判定本次抽奖消耗的次数来源。
// 名额顺序: 首抽 -> 阶梯 -> 手动补发。
func classifyDrawSource(status *LotteryUserStatus, firstGranted bool) string {
	first := int64(0)
	if firstGranted {
		first = 1
	}
	used := status.UsedDraws
	switch {
	case used < first:
		return LotteryDrawSourceFirst
	case used < first+status.ThresholdEntitlement:
		return LotteryDrawSourceThreshold
	default:
		return LotteryDrawSourceManual
	}
}

// pickPrizeWithStock 按用户阶梯从候选奖品中加权随机选一个并原子扣库存。
// 候选 = 达到 min_tier 且该阶梯有效权重 > 0 且有库存的奖品;
// 有限库存奖品并发扣减失败时将其移出候选重新选择;
// 候选耗尽返回 ErrLotteryNoPrizeAvailable。
func pickPrizeWithStock(ctx context.Context, repo LotteryRepository, prizes []LotteryPrize, tier int64) (*LotteryPrize, error) {
	candidates := make([]LotteryPrize, 0, len(prizes))
	for i := range prizes {
		p := prizes[i]
		if !p.AvailableAtTier(tier) || !p.InStock() {
			continue
		}
		// 将该阶梯的有效权重写入副本,供 weightedPick 使用。
		p.Weight = p.EffectiveWeight(tier)
		candidates = append(candidates, p)
	}
	for len(candidates) > 0 {
		picked := weightedPick(candidates)
		if picked == nil {
			break
		}
		if picked.Stock < 0 {
			// 无限库存无需扣减。
			return picked, nil
		}
		ok, err := repo.DeductPrizeStock(ctx, picked.ID)
		if err != nil {
			return nil, err
		}
		if ok {
			picked.StockIssued++
			return picked, nil
		}
		// 并发竞争失败: 剔除该奖品后重试。
		next := candidates[:0]
		for i := range candidates {
			if candidates[i].ID != picked.ID {
				next = append(next, candidates[i])
			}
		}
		candidates = next
	}
	return nil, ErrLotteryNoPrizeAvailable
}

// pickGuaranteedFirstPrize 首抽保底: 选中首抽固定奖品(balance_bonus $1)并
// 原子扣库存。奖品未配置/已停用/无库存/并发扣减失败时返回 nil,由调用方
// 退回加权随机;无限库存(Stock<0)直接返回不扣减。
func pickGuaranteedFirstPrize(ctx context.Context, repo LotteryRepository, prizes []LotteryPrize) *LotteryPrize {
	for i := range prizes {
		p := prizes[i]
		if p.PrizeType != LotteryFirstDrawGuaranteedPrizeType ||
			math.Abs(p.Value-LotteryFirstDrawGuaranteedPrizeValue) > firstPrizeValueEpsilon ||
			!p.InStock() {
			continue
		}
		if p.Stock < 0 {
			return &p
		}
		ok, err := repo.DeductPrizeStock(ctx, p.ID)
		if err != nil {
			slog.Warn("lottery first-draw guaranteed stock deduct failed, fallback to random",
				"prize_id", p.ID, "error", err)
			return nil
		}
		if !ok {
			// 并发竞争失败: 首抽退回随机,保可用性优先。
			return nil
		}
		p.StockIssued++
		return &p
	}
	return nil
}

// weightedPick 按权重随机选择(crypto/rand)。
func weightedPick(candidates []LotteryPrize) *LotteryPrize {
	total := 0
	for i := range candidates {
		total += candidates[i].Weight
	}
	if total <= 0 {
		return nil
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(total)))
	if err != nil {
		// crypto/rand 失败极罕见(系统熵源故障),此时拒绝抽奖而非降级为伪随机。
		return nil
	}
	threshold := n.Int64()
	var acc int64
	for i := range candidates {
		acc += int64(candidates[i].Weight)
		if threshold < acc {
			return &candidates[i]
		}
	}
	// 理论不可达,防御性返回最后一个。
	return &candidates[len(candidates)-1]
}

// validateIdempotencyKey 校验幂等键: 非空且不超过 64 字符。
func validateIdempotencyKey(key string) error {
	if key == "" || len(key) > 64 {
		return ErrLotteryInvalidIdempotencyKey
	}
	return nil
}

// fulfillDraw 执行奖品发放并更新流水状态。
//
//   - none:      谢谢参与,直接 granted;
//   - quota:     本地 MVP 记录型发放,直接 granted;
//   - balance_bonus: 原子调整余额;失败标记 failed 并记录原因,可重试。
func (s *LotteryService) fulfillDraw(ctx context.Context, record *LotteryDrawRecord) {
	switch record.PrizeType {
	case LotteryPrizeTypeNone, LotteryPrizeTypeQuota:
		s.markFulfillment(ctx, record, LotteryFulfillmentGranted, nil)
	case LotteryPrizeTypeBalanceBonus:
		if _, err := s.userRepo.AdjustBalance(ctx, record.UserID, record.PrizeValue); err != nil {
			slog.Error("lottery balance fulfillment failed",
				"draw_id", record.ID, "user_id", record.UserID, "value", record.PrizeValue, "error", err)
			reason := err.Error()
			s.markFulfillment(ctx, record, LotteryFulfillmentFailed, &reason)
			return
		}
		s.invalidateBalanceCacheAsync(record.UserID)
		s.markFulfillment(ctx, record, LotteryFulfillmentGranted, nil)
	default:
		reason := fmt.Sprintf("unknown prize type %q", record.PrizeType)
		s.markFulfillment(ctx, record, LotteryFulfillmentFailed, &reason)
	}
}

func (s *LotteryService) markFulfillment(ctx context.Context, record *LotteryDrawRecord, status string, failureReason *string) {
	var fulfilledAt *time.Time
	if status == LotteryFulfillmentGranted {
		t := s.nowFunc()
		fulfilledAt = &t
	}
	if err := s.repo.UpdateDrawFulfillment(ctx, record.ID, status, fulfilledAt, failureReason); err != nil {
		// 更新失败时流水停留在 pending,管理端重试接口可再次触发。
		slog.Error("lottery update fulfillment status failed", "draw_id", record.ID, "status", status, "error", err)
		return
	}
	record.FulfillmentStatus = status
	record.FulfilledAt = fulfilledAt
	record.FulfillmentError = failureReason
}

// ListUserRecords 分页查询用户抽奖流水。
func (s *LotteryService) ListUserRecords(ctx context.Context, userID int64, params pagination.PaginationParams) ([]LotteryDrawRecord, *pagination.PaginationResult, error) {
	return s.repo.ListDrawsByUser(ctx, userID, params)
}

// invalidateBalanceCacheAsync 异步失效用户余额缓存(镜像 UserService.UpdateBalance
// 的既有模式)。billingCache 为 nil(本地单测)时静默跳过。
func (s *LotteryService) invalidateBalanceCacheAsync(userID int64) {
	if s.billingCache == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in lottery balance cache invalidation", "user_id", userID, "recover", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.billingCache.InvalidateUserBalance(ctx, userID); err != nil {
			slog.Error("lottery invalidate user balance cache failed", "user_id", userID, "error", err)
		}
	}()
}

// ListUserRewards 分页查询用户中奖记录(排除"谢谢参与")。
func (s *LotteryService) ListUserRewards(ctx context.Context, userID int64, params pagination.PaginationParams) ([]LotteryDrawRecord, *pagination.PaginationResult, error) {
	return s.repo.ListRewardsByUser(ctx, userID, params)
}

// ---------------------------------------------------------------
// 管理端
// ---------------------------------------------------------------

// AdminListActivities 管理端查询全部活动(按 ID 倒序)。
func (s *LotteryService) AdminListActivities(ctx context.Context) ([]LotteryActivity, error) {
	return s.repo.ListActivities(ctx)
}

// AdminListPrizes 管理端查询奖品原始配置(含停用奖品,enabledOnly=false),
// 供管理页展示与编辑;与用户端概率视图(LotteryActivityView)分离。
func (s *LotteryService) AdminListPrizes(ctx context.Context, activityID int64) ([]LotteryPrize, error) {
	return s.repo.ListPrizesByActivity(ctx, activityID, false)
}

// AdminUpdateActivity 管理端更新活动配置(名称/状态/时间)。状态变更不改
// rules_version;奖品/概率变更由 AdminUpdatePrize 触发版本递增。
func (s *LotteryService) AdminUpdateActivity(ctx context.Context, id int64, input LotteryActivityUpdateInput) error {
	if input.Status != nil {
		switch *input.Status {
		case LotteryActivityStatusDraft, LotteryActivityStatusActive,
			LotteryActivityStatusPaused, LotteryActivityStatusEnded:
		default:
			return infraerrors.BadRequest("LOTTERY_INVALID_STATUS", "invalid activity status")
		}
	}
	return s.repo.UpdateActivity(ctx, id, input)
}

// AdminUpdatePrize 管理端更新奖品配置;任何变更都递增活动 rules_version,
// 使历史流水可按当时规则解释。
func (s *LotteryService) AdminUpdatePrize(ctx context.Context, prizeID int64, input LotteryPrizeUpdateInput) error {
	prize, err := s.repo.GetPrizeByID(ctx, prizeID)
	if err != nil {
		return err
	}
	if input.Stock != nil && *input.Stock < -1 {
		return infraerrors.BadRequest("LOTTERY_INVALID_STOCK", "stock must be >= -1")
	}
	if input.Weight != nil && *input.Weight < 0 {
		return infraerrors.BadRequest("LOTTERY_INVALID_WEIGHT", "weight must be >= 0")
	}
	if input.Value != nil && *input.Value < 0 {
		return infraerrors.BadRequest("LOTTERY_INVALID_VALUE", "value must be >= 0")
	}
	if err := s.repo.UpdatePrize(ctx, prizeID, input); err != nil {
		return err
	}
	if _, err := s.repo.BumpActivityRulesVersion(ctx, prize.ActivityID); err != nil {
		return err
	}
	return nil
}

// AdminListDraws 管理端查询抽奖流水(可按用户过滤)。
// 返回的每条记录附带中奖人摘要(含已删除用户,标记 Deleted);
// 单个用户查询失败不阻塞整页,该记录 User 保持 nil。
func (s *LotteryService) AdminListDraws(ctx context.Context, filter LotteryDrawListFilter, params pagination.PaginationParams) ([]LotteryDrawRecord, *pagination.PaginationResult, error) {
	records, result, err := s.repo.ListDrawsAdmin(ctx, filter, params)
	if err != nil {
		return nil, nil, err
	}
	s.attachDrawUserSummaries(ctx, records)
	return records, result, nil
}

// attachDrawUserSummaries 就地填充流水记录的中奖人摘要(页内 user_id 去重)。
func (s *LotteryService) attachDrawUserSummaries(ctx context.Context, records []LotteryDrawRecord) {
	seen := make(map[int64]bool, len(records))
	for i := range records {
		uid := records[i].UserID
		if uid <= 0 || seen[uid] {
			continue
		}
		seen[uid] = true
		u, uerr := s.userRepo.GetByIDIncludeDeleted(ctx, uid)
		if uerr != nil || u == nil {
			continue
		}
		records[i].User = drawUserSummaryFromUser(u)
	}
}

// AdminRetryFulfillment 管理端重试发放。granted 状态不可重试。
func (s *LotteryService) AdminRetryFulfillment(ctx context.Context, drawID int64) (*LotteryDrawRecord, error) {
	record, err := s.repo.GetDrawByID(ctx, drawID)
	if err != nil {
		return nil, err
	}
	if record.FulfillmentStatus == LotteryFulfillmentGranted {
		return nil, ErrLotteryFulfillmentNotRetryable
	}
	s.fulfillDraw(ctx, record)
	return s.repo.GetDrawByID(ctx, drawID)
}

// AdminAdjustDraws 管理端为用户补发(正数)/回收(负数)抽奖次数。
func (s *LotteryService) AdminAdjustDraws(ctx context.Context, userID int64, delta int) error {
	if delta == 0 {
		return nil
	}
	if _, err := s.ensureUserStats(ctx, userID); err != nil {
		return err
	}
	return s.repo.UpdateUserStatsManual(ctx, userID, delta)
}

// AdminSetSpendOffset 本地测试辅助: 设置用户消耗偏移(美分),叠加在
// usage_logs 真实汇总之上,用于验证阶梯解锁。生产环境不得调用。
func (s *LotteryService) AdminSetSpendOffset(ctx context.Context, userID int64, offsetCents int64) error {
	if _, err := s.ensureUserStats(ctx, userID); err != nil {
		return err
	}
	return s.repo.UpdateUserStatsSpendOffset(ctx, userID, offsetCents)
}

// AdminResetUser 本地测试辅助: 清空用户抽奖状态与全部流水(重新从首抽开始)。
// 仅用于本地测试环境。
func (s *LotteryService) AdminResetUser(ctx context.Context, userID int64) error {
	if err := s.repo.DeleteDrawsByUser(ctx, userID); err != nil {
		return err
	}
	return s.repo.DeleteUserStats(ctx, userID)
}
