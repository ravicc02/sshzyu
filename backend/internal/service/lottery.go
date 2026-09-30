// Package service 抽奖(Lottery)领域类型、仓储接口与业务错误。
//
// 业务规则(本地 MVP):
//   - 正常账号(active、未删除)首次进入抽奖体系时统一获得 1 次免费抽奖;
//   - 基线后累计余额消耗达 $5 解锁第 2 次,此后每新增 $50 解锁 1 次
//     (阈值 $5, $55, $105, ...);
//   - "余额消耗"权威口径 = SUM(usage_logs.actual_cost) WHERE billing_type = 0
//     (钱包余额计费),仅统计 baseline_at 与最近 24 小时两者中较晚时点之后的消耗;
//   - 禁用/封禁/已删除账号不参与抽奖;
//   - 中奖结果完全由服务端生成,前端只做展示。
package service

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// 抽奖金额阈值(整数美分),避免浮点比较误差。
const (
	// LotterySecondDrawThresholdCents 第 2 次抽奖的累计消耗门槛: $5。
	LotterySecondDrawThresholdCents int64 = 500
	// LotteryStepCents 第 3 次起每次解锁的消耗步长: $50。
	// 阶梯序列: $5 → $55 → $105 → $155 → $205(封顶)。
	LotteryStepCents int64 = 5000
)

// LotterySpendWindow 是阶梯余额消耗和阶梯抽奖次数的滚动统计窗口。
// 首抽和管理端手动补发不属于滚动资格，不受此窗口影响。
const LotterySpendWindow = 24 * time.Hour

// 阶梯配置模式。fixed 使用 $5 起、每 $50 递增；custom 使用活动保存的门槛列表。
const (
	LotteryTierModeFixed  = "fixed"
	LotteryTierModeCustom = "custom"
)

// LotteryTierDefinition 描述一个可自定义阶梯。第一档必须是门槛为 0 的基础档，
// 后续档位的门槛必须严格递增；ThresholdCents 单位为美分。
type LotteryTierDefinition struct {
	Name           string `json:"name"`
	ThresholdCents int64  `json:"threshold_cents"`
}

// LotteryTierConfig 描述活动的阶梯解锁规则。Definitions 是新的动态配置；
// Thresholds 仅用于兼容旧版 custom 活动的四个门槛。
type LotteryTierConfig struct {
	Mode        string
	Thresholds  []int64
	Definitions []LotteryTierDefinition
}

func defaultLotteryTierDefinitions() []LotteryTierDefinition {
	return []LotteryTierDefinition{
		{Name: "青铜", ThresholdCents: 0},
		{Name: "白银", ThresholdCents: 500},
		{Name: "黄金", ThresholdCents: 5500},
		{Name: "钻石", ThresholdCents: 10500},
		{Name: "王者", ThresholdCents: 15500},
	}
}

func DefaultLotteryTierConfig() LotteryTierConfig {
	return LotteryTierConfig{Mode: LotteryTierModeFixed, Definitions: defaultLotteryTierDefinitions()}
}

func legacyLotteryTierDefinitions(thresholds []int64) []LotteryTierDefinition {
	names := []string{"白银", "黄金", "钻石", "王者"}
	defs := []LotteryTierDefinition{{Name: "青铜", ThresholdCents: 0}}
	for i, threshold := range thresholds {
		name := "阶梯 " + strconv.Itoa(i+1)
		if i < len(names) {
			name = names[i]
		}
		defs = append(defs, LotteryTierDefinition{Name: name, ThresholdCents: threshold})
	}
	return defs
}

func (c LotteryTierConfig) definitions() []LotteryTierDefinition {
	if len(c.Definitions) > 0 {
		return c.Definitions
	}
	if len(c.Thresholds) > 0 {
		return legacyLotteryTierDefinitions(c.Thresholds)
	}
	return defaultLotteryTierDefinitions()
}

// IsValid 校验动态阶梯：至少一个基础档，名称非空，第一档门槛为 0，
// 其余门槛严格递增。最多 100 档，防止异常请求制造过大配置。
func (c LotteryTierConfig) IsValid() bool {
	if c.Mode == "" || c.Mode == LotteryTierModeFixed {
		return true
	}
	if c.Mode != LotteryTierModeCustom {
		return false
	}
	defs := c.definitions()
	if len(defs) < 1 || len(defs) > 100 || defs[0].ThresholdCents != 0 {
		return false
	}
	var previous int64 = -1
	seenNames := make(map[string]struct{}, len(defs))
	for _, def := range defs {
		name := strings.TrimSpace(def.Name)
		if name == "" || def.ThresholdCents < 0 || def.ThresholdCents <= previous {
			return false
		}
		if _, exists := seenNames[name]; exists {
			return false
		}
		seenNames[name] = struct{}{}
		previous = def.ThresholdCents
	}
	return true
}

// Normalized 规范化旧配置或无效配置到默认固定规则。
func (c LotteryTierConfig) Normalized() LotteryTierConfig {
	if c.Mode == "" || c.Mode == LotteryTierModeFixed {
		return DefaultLotteryTierConfig()
	}
	if !c.IsValid() {
		return DefaultLotteryTierConfig()
	}
	defs := append([]LotteryTierDefinition(nil), c.definitions()...)
	return LotteryTierConfig{Mode: LotteryTierModeCustom, Definitions: defs}
}

// Entitlement 返回滚动窗口内消耗对应的阶梯解锁次数。
func (c LotteryTierConfig) Entitlement(spentCents int64) int64 {
	if spentCents <= 0 {
		return 0
	}
	c = c.Normalized()
	if c.Mode == LotteryTierModeFixed {
		if spentCents < LotterySecondDrawThresholdCents {
			return 0
		}
		return 1 + (spentCents-LotterySecondDrawThresholdCents)/LotteryStepCents
	}
	var count int64
	for i := 1; i < len(c.Definitions); i++ {
		if spentCents < c.Definitions[i].ThresholdCents {
			break
		}
		count++
	}
	return count
}

// NextThresholdCents 返回下一档门槛；自定义模式全部达到后返回 -1。
func (c LotteryTierConfig) NextThresholdCents(spentCents int64) int64 {
	c = c.Normalized()
	if c.Mode == LotteryTierModeFixed {
		return LotterySecondDrawThresholdCents + c.Entitlement(spentCents)*LotteryStepCents
	}
	for i := 1; i < len(c.Definitions); i++ {
		if spentCents < c.Definitions[i].ThresholdCents {
			return c.Definitions[i].ThresholdCents
		}
	}
	return -1
}

func (c LotteryTierConfig) UserTier(spentCents int64) int64 {
	var tier int64
	if c.Normalized().Mode == LotteryTierModeFixed {
		tier = LotteryUserTier(spentCents)
	} else {
		tier = c.Entitlement(spentCents)
	}
	maxTier := c.DisplayTierCount() - 1
	if tier > maxTier {
		tier = maxTier
	}
	return tier
}

func (c LotteryTierConfig) TierName(tier int64) string {
	c = c.Normalized()
	if tier >= 0 && tier < int64(len(c.Definitions)) {
		return c.Definitions[tier].Name
	}
	return LotteryTierName(tier)
}

func (c LotteryTierConfig) TierThresholdCents(tier int64) int64 {
	if tier < 0 {
		return -1
	}
	c = c.Normalized()
	if c.Mode == LotteryTierModeFixed && tier >= int64(len(c.Definitions)) {
		return LotterySecondDrawThresholdCents + (tier-1)*LotteryStepCents
	}
	if tier >= int64(len(c.Definitions)) {
		return -1
	}
	return c.Definitions[tier].ThresholdCents
}

func (c LotteryTierConfig) DisplayTierCount() int64 {
	return int64(len(c.Normalized().Definitions))
}

// TierDefinitions 返回前端/管理端可安全消费的标准化阶梯定义副本。
func (c LotteryTierConfig) TierDefinitions() []LotteryTierDefinition {
	return append([]LotteryTierDefinition(nil), c.Normalized().Definitions...)
}

// 抽奖活动状态。
const (
	LotteryActivityStatusDraft  = "draft"
	LotteryActivityStatusActive = "active"
	LotteryActivityStatusPaused = "paused"
	LotteryActivityStatusEnded  = "ended"
)

// 奖品类型。
const (
	LotteryPrizeTypeNone         = "none"          // 谢谢参与/未中奖
	LotteryPrizeTypeBalanceBonus = "balance_bonus" // 赠送账户余额(本地测试环境)
	LotteryPrizeTypeQuota        = "quota"         // API 使用额度(本地 MVP 记录型发放)
)

// 抽奖次数来源。
const (
	LotteryDrawSourceFirst     = "first"
	LotteryDrawSourceThreshold = "threshold"
	LotteryDrawSourceManual    = "manual"
)

// 首抽保底规则: 用户的第一次抽奖固定中奖该奖品(运营规则,不受阶梯权重影响)。
// 奖品需满足 类型 + 金额 匹配且仍有库存,否则退回正常加权随机。
const (
	LotteryFirstDrawGuaranteedPrizeType  = LotteryPrizeTypeBalanceBonus
	LotteryFirstDrawGuaranteedPrizeValue = 1.0 // $1 余额
	// firstPrizeValueEpsilon 奖品金额为 DECIMAL 存储的浮点值,用容差比较。
	firstPrizeValueEpsilon = 1e-9
)

// 发放状态。
const (
	LotteryFulfillmentPending       = "pending"
	LotteryFulfillmentPendingReview = "pending_review" // balance_bonus 中奖后待管理员审核
	LotteryFulfillmentGranted       = "granted"
	LotteryFulfillmentRejected      = "rejected" // 管理员驳回
	LotteryFulfillmentRevoked       = "revoked"  // 管理员撤回已发放的余额
	LotteryFulfillmentFailed        = "failed"
)

// Lottery redeem types (appear in user's redeem history page).
const (
	LotteryRedeemTypeWin    = "lottery_win"    // 抽奖获得（审核通过）
	LotteryRedeemTypeReject = "lottery_reject" // 抽奖驳回（审核不通过）
)

// 抽奖业务错误。
var (
	// ErrLotteryNoActiveActivity 当前没有进行中的活动。
	ErrLotteryNoActiveActivity = infraerrors.NotFound("LOTTERY_NO_ACTIVE_ACTIVITY", "no active lottery activity")
	// ErrLotteryUserNotEligible 用户不具备抽奖资格(禁用/删除等)。
	ErrLotteryUserNotEligible = infraerrors.Forbidden("LOTTERY_USER_NOT_ELIGIBLE", "user is not eligible for lottery")
	// ErrLotteryNoDrawsLeft 可用抽奖次数已用完。
	ErrLotteryNoDrawsLeft = infraerrors.BadRequest("LOTTERY_NO_DRAWS_LEFT", "no lottery draws left")
	// ErrLotteryNoPrizeAvailable 没有可发放的奖品(配置错误:无 enabled 奖品)。
	ErrLotteryNoPrizeAvailable = infraerrors.InternalServer("LOTTERY_NO_PRIZE_AVAILABLE", "no prize available for draw")
	// ErrLotteryDrawNotFound 抽奖流水不存在。
	ErrLotteryDrawNotFound = infraerrors.NotFound("LOTTERY_DRAW_NOT_FOUND", "lottery draw not found")
	// ErrLotteryDrawIdempotencyConflict 幂等键冲突(同 key 已有其他流水)。
	ErrLotteryDrawIdempotencyConflict = infraerrors.Conflict("LOTTERY_IDEMPOTENCY_CONFLICT", "lottery draw idempotency conflict")
	// ErrLotteryStatsNotFound 用户抽奖状态行不存在。
	ErrLotteryStatsNotFound = infraerrors.NotFound("LOTTERY_STATS_NOT_FOUND", "lottery user stats not found")
	// ErrLotteryStatsExists 并发创建用户抽奖状态行时撞唯一约束(重试即可)。
	ErrLotteryStatsExists = infraerrors.Conflict("LOTTERY_STATS_EXISTS", "lottery user stats already exists")
	// ErrLotteryActivityNotFound 活动不存在。
	ErrLotteryActivityNotFound = infraerrors.NotFound("LOTTERY_ACTIVITY_NOT_FOUND", "lottery activity not found")
	// ErrLotteryPrizeNotFound 奖品不存在。
	ErrLotteryPrizeNotFound = infraerrors.NotFound("LOTTERY_PRIZE_NOT_FOUND", "lottery prize not found")
	// ErrLotteryInvalidIdempotencyKey 幂等键非法(空或超长)。
	ErrLotteryInvalidIdempotencyKey = infraerrors.BadRequest("LOTTERY_INVALID_IDEMPOTENCY_KEY", "invalid idempotency key")
	// ErrLotteryFulfillmentNotRetryable 该流水当前状态不可重试发放。
	ErrLotteryFulfillmentNotRetryable = infraerrors.Conflict("LOTTERY_FULFILLMENT_NOT_RETRYABLE", "fulfillment is not retryable in current status")
)

// LotteryThresholdEntitlement 计算近 24h 累计消耗(美分)对应的阶梯抽奖应得次数。
//
//	spent < $5       -> 0
//	$5 <= spent      -> 1 + floor((spent - $5) / $50)
//
// 例: $4.99 -> 0, $5 -> 1, $54.99 -> 1, $55 -> 2, $105 -> 3。
func LotteryThresholdEntitlement(spentCents int64) int64 {
	if spentCents < LotterySecondDrawThresholdCents {
		return 0
	}
	return 1 + (spentCents-LotterySecondDrawThresholdCents)/LotteryStepCents
}

// LotteryNextThresholdCents 返回下一个未解锁阶梯的累计消耗门槛(美分)。
// e 为当前阶梯应得次数;返回值始终大于当前已达到的门槛。
func LotteryNextThresholdCents(entitlement int64) int64 {
	return LotterySecondDrawThresholdCents + entitlement*LotteryStepCents
}

// 用户阶梯(tier)常量: 按近 24h 累计余额消耗划分,门槛与次数解锁阈值一致。
const (
	LotteryTierBronze  = int64(0) // 青铜: < $5
	LotteryTierSilver  = int64(1) // 白银: >= $5
	LotteryTierGold    = int64(2) // 黄金: >= $55
	LotteryTierDiamond = int64(3) // 钻石: >= $105
	LotteryTierKing    = int64(4) // 王者: >= $155（封顶，再往上不升）
	LotteryMaxTier     = int64(4)
)

// LotteryUserTier 按近 24h 累计余额消耗(美分)计算用户阶梯。
//
//	spent < $5  -> 0(青铜)
//	$5 <= spent -> 1 + floor((spent - $5) / $50), 封顶王者(4)
//
// 例: $4.99 -> 0, $5 -> 1, $54.99 -> 1, $55 -> 2, $105 -> 3, $155 -> 4。
func LotteryUserTier(spentCents int64) int64 {
	if spentCents < LotterySecondDrawThresholdCents {
		return LotteryTierBronze
	}
	tier := LotteryTierSilver + (spentCents-LotterySecondDrawThresholdCents)/LotteryStepCents
	if tier > LotteryMaxTier {
		tier = LotteryMaxTier
	}
	return tier
}

// LotteryTierName 返回阶梯展示名(与前端多语言解耦,后端保底中文名)。
func LotteryTierName(tier int64) string {
	switch tier {
	case LotteryTierSilver:
		return "白银"
	case LotteryTierGold:
		return "黄金"
	case LotteryTierDiamond:
		return "钻石"
	case LotteryTierKing:
		return "王者"
	default:
		return "青铜"
	}
}

// LotteryTierThresholdCents 返回进入指定阶梯的累计消耗门槛(美分)。
// 阶梯 0(青铜)门槛为 0;超过最大阶梯返回 -1 表示不存在。
func LotteryTierThresholdCents(tier int64) int64 {
	if tier <= 0 {
		return 0
	}
	if tier > LotteryMaxTier {
		return -1
	}
	return LotterySecondDrawThresholdCents + (tier-1)*LotteryStepCents
}

// BalanceSpentCentsToFloat 将美分整数转回美元浮点(用于展示/快照列)。
func BalanceSpentCentsToFloat(cents int64) float64 {
	return float64(cents) / 100
}

// BalanceSpentFloatToCents 将美元浮点转为整数美分(四舍五入)。
// usage_logs.actual_cost 为 DECIMAL(20,10),常规金额量级下该转换无歧义。
func BalanceSpentFloatToCents(spent float64) int64 {
	if spent <= 0 || math.IsNaN(spent) || math.IsInf(spent, 0) {
		return 0
	}
	return int64(math.Round(spent * 100))
}

// LotteryActivity 抽奖活动领域对象。
type LotteryActivity struct {
	ID              int64
	Name            string
	Status          string
	RulesVersion    int
	TierMode        string
	TierThresholds  []int64
	TierDefinitions []LotteryTierDefinition
	StartsAt        *time.Time
	EndsAt          *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// TierConfig 返回活动当前有效的阶梯规则。旧活动未设置字段时回退固定模式。
func (a *LotteryActivity) TierConfig() LotteryTierConfig {
	if a == nil {
		return DefaultLotteryTierConfig()
	}
	return LotteryTierConfig{Mode: a.TierMode, Thresholds: a.TierThresholds, Definitions: a.TierDefinitions}.Normalized()
}

// IsOpen 判断活动当前是否可抽奖: 状态为 active 且在时间窗口内。
func (a *LotteryActivity) IsOpen(now time.Time) bool {
	if a == nil || a.Status != LotteryActivityStatusActive {
		return false
	}
	if a.StartsAt != nil && now.Before(*a.StartsAt) {
		return false
	}
	if a.EndsAt != nil && now.After(*a.EndsAt) {
		return false
	}
	return true
}

// LotteryPrize 抽奖奖品领域对象。Stock = -1 表示无限库存。
type LotteryPrize struct {
	ID         int64
	ActivityID int64
	Name       string
	PrizeType  string
	Value      float64
	Weight     int
	// MinTier 可中该奖品的最低用户阶梯;低于该阶梯的奖品不参与随机。
	MinTier int
	// TierWeights 按动态阶梯保存的显式中奖概率,key 为 tier 数字字符串。
	// 新配置应为每个阶梯显式写入；旧数据缺失时仍兼容回退 Weight。
	TierWeights map[string]int
	Stock       int
	StockIssued int
	Enabled     bool
	SortOrder   int
}

// EffectiveWeight 返回奖品在指定阶梯下的有效中奖概率。
// 新配置要求 tier_weights 显式覆盖每个阶梯；旧数据未配置时兼容回退基础 weight。
func (p *LotteryPrize) EffectiveWeight(tier int64) int {
	if w, ok := p.TierWeights[strconv.FormatInt(tier, 10)]; ok {
		return w
	}
	return p.Weight
}

// AvailableAtTier 报告奖品是否对指定阶梯开放(达到 min_tier 且有效权重 > 0)。
func (p *LotteryPrize) AvailableAtTier(tier int64) bool {
	return tier >= int64(p.MinTier) && p.EffectiveWeight(tier) > 0
}

// InStock 报告奖品是否还有库存可发。
func (p *LotteryPrize) InStock() bool {
	return p.Stock < 0 || p.StockIssued < p.Stock
}

// LotteryUserStats 用户抽奖资格状态(每用户一行)。
type LotteryUserStats struct {
	ID               int64
	UserID           int64
	FirstDrawGranted bool
	BaselineAt       time.Time
	ManualAdjustment int
	// SpendOffsetCents 本地测试用消耗偏移(美分)，叠加在 usage_logs 汇总之上。
	SpendOffsetCents int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// LotteryDrawRecord 抽奖流水(只追加)。奖品字段为中奖时刻快照。
type LotteryDrawRecord struct {
	ID                 int64
	UserID             int64
	ActivityID         int64
	PrizeID            *int64
	PrizeName          string
	PrizeType          string
	PrizeValue         float64
	RulesVersion       int
	Source             string
	BalanceSpentAtDraw float64
	FulfillmentStatus  string
	FulfilledAt        *time.Time
	FulfillmentError   *string
	IdempotencyKey     string
	CreatedAt          time.Time
	// User 中奖人摘要,仅管理端查询时填充;用户端查询保持 nil。
	User *DrawUserSummary
}

// HasPrize 报告该次抽奖是否中了实物/权益奖品(非"谢谢参与")。
func (d *LotteryDrawRecord) HasPrize() bool {
	return d.PrizeType != LotteryPrizeTypeNone
}

// LotteryActivityUpdateInput 管理端活动更新入参(局部更新,nil 表示不改)。
type LotteryActivityUpdateInput struct {
	Name            *string
	Status          *string
	TierMode        *string
	TierThresholds  *[]int64
	TierDefinitions *[]LotteryTierDefinition
	StartsAt        *time.Time
	EndsAt          *time.Time
}

// LotteryPrizeUpdateInput 管理端奖品配置更新入参(nil 表示不改)。
// 任何会影响概率/库存的字段变更都会使活动 rules_version 递增。
type LotteryPrizeUpdateInput struct {
	Name      *string
	PrizeType *string
	Value     *float64
	Weight    *int
	// MinTier 可中该奖的最低阶梯(nil 表示不改)。
	MinTier *int
	// TierWeights 按阶梯覆盖权重(nil 表示不改;传空 map 表示清空)。
	TierWeights *map[string]int
	Stock       *int
	Enabled     *bool
	SortOrder   *int
}

// LotteryPrizeWeightUpdate 是活动级概率保存的单个奖品输入。
// Weight 与 TierWeights 均为百分比整数；同一活动每个可用阶梯的总和必须为 100。
type LotteryPrizeWeightUpdate struct {
	ID          int64
	Weight      int
	TierWeights map[string]int
	// Enabled 与 MinTier 同样影响各阶梯的候选奖池，因此必须和概率一起原子保存。
	Enabled bool
	MinTier int
}

// LotteryDrawListFilter 管理端流水查询过滤条件。
type LotteryDrawListFilter struct {
	UserID *int64
}

// LotteryUserStatus 用户抽奖状态聚合视图(用户端 status 接口的领域结果)。
type LotteryUserStatus struct {
	ActivityOpen         bool
	AvailableDraws       int64
	UsedDraws            int64
	FirstDrawGranted     bool
	ThresholdEntitlement int64
	ManualAdjustment     int64
	// BalanceSpentCents 基线后累计余额消耗(美分)。
	BalanceSpentCents int64
	// NextThresholdCents 下一次阶梯解锁门槛(美分)。
	NextThresholdCents int64
	// CurrentTier 用户当前阶梯(0 青铜/1 白银/2 黄金/3 钻石/4 王者)。
	CurrentTier int64
	// TierName 当前阶梯展示名。
	TierName     string
	RulesVersion int
	// PointsExpiresAt 为当前积分有效期；尚无抽奖时为空，成功抽奖刷新24小时。
	PointsExpiresAt *time.Time
	ServerTime      time.Time

	// 以下字段仅供服务层判定本次抽奖消耗哪一种资格，不对外暴露。
	firstRemaining     int64
	thresholdRemaining int64
	manualRemaining    int64
}

// LotteryDrawOutcome 一次抽奖的完整结果。
type LotteryDrawOutcome struct {
	Record *LotteryDrawRecord
	// RemainingDraws 抽奖后的剩余可用次数。
	RemainingDraws int64
}

// LotteryRepository 抽奖仓储接口,由 repository 层基于 Ent 实现。
//
// 参与 service 层事务的方法(GetUserStatsForUpdate/DeductPrizeStock/CreateDraw 等)
// 通过 context 中的事务 client 执行(见 clientFromContext)。
type LotteryRepository interface {
	// ---- 活动 ----
	GetActiveActivity(ctx context.Context) (*LotteryActivity, error)
	GetLatestActivity(ctx context.Context) (*LotteryActivity, error)
	ListActivities(ctx context.Context) ([]LotteryActivity, error)
	GetActivityByID(ctx context.Context, id int64) (*LotteryActivity, error)
	GetActivityByIDForUpdate(ctx context.Context, id int64) (*LotteryActivity, error)
	UpdateActivity(ctx context.Context, id int64, input LotteryActivityUpdateInput) error
	BumpActivityRulesVersion(ctx context.Context, id int64) (int, error)
	EnsureDefaultActivity(ctx context.Context) (*LotteryActivity, error)

	// ---- 奖品 ----
	ListPrizesByActivity(ctx context.Context, activityID int64, enabledOnly bool) ([]LotteryPrize, error)
	GetPrizeByID(ctx context.Context, id int64) (*LotteryPrize, error)
	UpdatePrize(ctx context.Context, id int64, input LotteryPrizeUpdateInput) error
	// DeductPrizeStock 原子扣减库存(stock_issued+1),无库存时返回 false。
	DeductPrizeStock(ctx context.Context, prizeID int64) (bool, error)

	// ---- 用户状态 ----
	// GetUserStatsForUpdate 事务内行锁读取用户状态;不存在返回 ErrLotteryStatsExists。
	GetUserStatsForUpdate(ctx context.Context, userID int64) (*LotteryUserStats, error)
	// GetUserStats 读取用户状态(不加锁);不存在返回 ErrLotteryStatsExists。
	GetUserStats(ctx context.Context, userID int64) (*LotteryUserStats, error)
	CreateUserStats(ctx context.Context, userID int64, firstDrawGranted bool, baselineAt time.Time) (*LotteryUserStats, error)
	UpdateUserStatsManual(ctx context.Context, userID int64, delta int) error
	UpdateUserStatsBaselineAt(ctx context.Context, userID int64, baselineAt time.Time) error
	// UpdateUserStatsSpendOffset 设置本地测试消耗偏移(美分)。
	UpdateUserStatsSpendOffset(ctx context.Context, userID int64, offsetCents int64) error
	DeleteUserStats(ctx context.Context, userID int64) error

	// ---- 余额消耗统计 ----
	// SumBalanceSpentSince 汇总用户自 since 起的余额计费消耗
	// (usage_logs.actual_cost, billing_type = 0)。
	SumBalanceSpentSince(ctx context.Context, userID int64, since time.Time) (float64, error)

	// ---- 抽奖流水 ----
	CountDraws(ctx context.Context, userID int64) (int, error)
	// ListDrawTimes 返回真实抽奖时间(升序)，用于划分24小时积分会话。
	ListDrawTimes(ctx context.Context, userID int64) ([]time.Time, error)
	// CountDrawsBySource 按次数来源统计流水；since 为 nil 时不限制时间。
	// 阶梯来源仅统计最近 24 小时，首抽和手动补发长期有效。
	CountDrawsBySource(ctx context.Context, userID int64, source string, since *time.Time) (int, error)
	GetDrawByIdempotencyKey(ctx context.Context, userID int64, key string) (*LotteryDrawRecord, error)
	// CreateDraw 写入抽奖流水;违反 (user_id, idempotency_key) 唯一约束时
	// 返回 ErrLotteryDrawIdempotencyConflict。
	CreateDraw(ctx context.Context, record *LotteryDrawRecord) error
	GetDrawByID(ctx context.Context, id int64) (*LotteryDrawRecord, error)
	UpdateDrawFulfillment(ctx context.Context, id int64, status string, fulfilledAt *time.Time, failureReason *string) error
	// UpdateDrawFulfillmentIfStatus 仅当当前状态匹配 expectedStatus 时更新；
	// 返回 false 表示记录已被其它管理员处理，调用方不得再调整余额。
	UpdateDrawFulfillmentIfStatus(ctx context.Context, id int64, expectedStatus, status string, fulfilledAt *time.Time, failureReason *string) (bool, error)
	ListDrawsByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]LotteryDrawRecord, *pagination.PaginationResult, error)
	ListRewardsByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]LotteryDrawRecord, *pagination.PaginationResult, error)
	ListDrawsAdmin(ctx context.Context, filter LotteryDrawListFilter, params pagination.PaginationParams) ([]LotteryDrawRecord, *pagination.PaginationResult, error)
	DeleteDrawsByUser(ctx context.Context, userID int64) error
}

// LotteryUserSource 抽奖模块对用户数据的窄依赖,避免测试桩实现整个
// UserRepository。*userRepository 天然满足该接口。
type LotteryUserSource interface {
	GetByID(ctx context.Context, id int64) (*User, error)
	// GetByIDIncludeDeleted 含已软删除用户,管理端展示历史中奖人时使用。
	GetByIDIncludeDeleted(ctx context.Context, id int64) (*User, error)
	// AdjustBalance 原子调整余额,用于 balance_bonus 类型奖品的发放。
	AdjustBalance(ctx context.Context, id int64, delta float64) (BalanceChange, error)
}

// DrawUserSummary 管理端中奖记录附带的用户标识(最小字段集)。
type DrawUserSummary struct {
	ID       int64
	Email    string
	Username string
	// Deleted 用户已被软删除时为 true,前端据此展示"已删除"。
	Deleted bool
}

// drawUserSummaryFromUser 领域用户转中奖人摘要。
func drawUserSummaryFromUser(u *User) *DrawUserSummary {
	if u == nil {
		return nil
	}
	return &DrawUserSummary{
		ID:       u.ID,
		Email:    u.Email,
		Username: u.Username,
		Deleted:  u.DeletedAt != nil,
	}
}

// LotteryBalanceCacheInvalidator 余额缓存失效能力(窄接口,nil 安全)。
// 抽奖发放余额奖励后失效 Redis 余额缓存,与 UserService.UpdateBalance
// 的既有行为保持一致,避免前端读到旧余额。*BillingCacheService 天然满足。
type LotteryBalanceCacheInvalidator interface {
	InvalidateUserBalance(ctx context.Context, userID int64) error
}
