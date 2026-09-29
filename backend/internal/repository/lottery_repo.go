// Package repository 抽奖(Lottery)仓储的 Ent 实现。
//
// 事务参与: 通过 clientFromContext 自动使用 context 中的事务 client,
// service 层的抽奖事务因此能将"用户状态行锁、库存扣减、流水写入"绑定在
// 同一事务内。原生 SQL 仅用于两类场景:
//   - 条件 UPDATE 原子扣库存(防超发);
//   - usage_logs 聚合统计(SUM 实际扣费)。
package repository

import (
	"context"
	"errors"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/lotteryactivity"
	"github.com/Wei-Shaw/sub2api/ent/lotterydraw"
	"github.com/Wei-Shaw/sub2api/ent/lotteryprize"
	"github.com/Wei-Shaw/sub2api/ent/lotteryuserstats"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	entsql "entgo.io/ent/dialect/sql"
)

type lotteryRepository struct {
	client *dbent.Client
}

// NewLotteryRepository 构造抽奖仓储。
func NewLotteryRepository(client *dbent.Client) service.LotteryRepository {
	return &lotteryRepository{client: client}
}

// ---------------------------------------------------------------
// 实体转换
// ---------------------------------------------------------------

func lotteryActivityToService(m *dbent.LotteryActivity) *service.LotteryActivity {
	return &service.LotteryActivity{
		ID:           m.ID,
		Name:         m.Name,
		Status:       m.Status,
		RulesVersion: m.RulesVersion,
		StartsAt:     m.StartsAt,
		EndsAt:       m.EndsAt,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

func lotteryPrizeToService(m *dbent.LotteryPrize) service.LotteryPrize {
	return service.LotteryPrize{
		ID:          m.ID,
		ActivityID:  m.ActivityID,
		Name:        m.Name,
		PrizeType:   m.PrizeType,
		Value:       m.Value,
		Weight:      m.Weight,
		MinTier:     m.MinTier,
		TierWeights: m.TierWeights,
		Stock:       m.Stock,
		StockIssued: m.StockIssued,
		Enabled:     m.Enabled,
		SortOrder:   m.SortOrder,
	}
}

func lotteryPrizesToService(items []*dbent.LotteryPrize) []service.LotteryPrize {
	out := make([]service.LotteryPrize, 0, len(items))
	for _, m := range items {
		out = append(out, lotteryPrizeToService(m))
	}
	return out
}

func lotteryStatsToService(m *dbent.LotteryUserStats) *service.LotteryUserStats {
	return &service.LotteryUserStats{
		ID:               m.ID,
		UserID:           m.UserID,
		FirstDrawGranted: m.FirstDrawGranted,
		BaselineAt:       m.BaselineAt,
		ManualAdjustment: m.ManualAdjustment,
		SpendOffsetCents: m.SpendOffsetCents,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}
}

func lotteryDrawToService(m *dbent.LotteryDraw) *service.LotteryDrawRecord {
	return &service.LotteryDrawRecord{
		ID:                 m.ID,
		UserID:             m.UserID,
		ActivityID:         m.ActivityID,
		PrizeID:            m.PrizeID,
		PrizeName:          m.PrizeName,
		PrizeType:          m.PrizeType,
		PrizeValue:         m.PrizeValue,
		RulesVersion:       m.RulesVersion,
		Source:             m.Source,
		BalanceSpentAtDraw: m.BalanceSpentAtDraw,
		FulfillmentStatus:  m.FulfillmentStatus,
		FulfilledAt:        m.FulfilledAt,
		FulfillmentError:   m.FulfillmentError,
		IdempotencyKey:     m.IdempotencyKey,
		CreatedAt:          m.CreatedAt,
	}
}

func lotteryDrawsToService(items []*dbent.LotteryDraw) []service.LotteryDrawRecord {
	out := make([]service.LotteryDrawRecord, 0, len(items))
	for _, m := range items {
		out = append(out, *lotteryDrawToService(m))
	}
	return out
}

// ---------------------------------------------------------------
// 活动
// ---------------------------------------------------------------

func (r *lotteryRepository) GetActiveActivity(ctx context.Context) (*service.LotteryActivity, error) {
	client := clientFromContext(ctx, r.client)
	items, err := client.LotteryActivity.Query().
		Where(lotteryactivity.StatusEQ(service.LotteryActivityStatusActive)).
		Order(dbent.Desc(lotteryactivity.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for _, m := range items {
		activity := lotteryActivityToService(m)
		if activity.IsOpen(now) {
			return activity, nil
		}
	}
	return nil, service.ErrLotteryActivityNotFound
}

func (r *lotteryRepository) GetLatestActivity(ctx context.Context) (*service.LotteryActivity, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.LotteryActivity.Query().
		Order(dbent.Desc(lotteryactivity.FieldID)).
		First(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrLotteryActivityNotFound
		}
		return nil, err
	}
	return lotteryActivityToService(m), nil
}

func (r *lotteryRepository) ListActivities(ctx context.Context) ([]service.LotteryActivity, error) {
	client := clientFromContext(ctx, r.client)
	items, err := client.LotteryActivity.Query().
		Order(dbent.Desc(lotteryactivity.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.LotteryActivity, 0, len(items))
	for _, m := range items {
		out = append(out, *lotteryActivityToService(m))
	}
	return out, nil
}

func (r *lotteryRepository) GetActivityByID(ctx context.Context, id int64) (*service.LotteryActivity, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.LotteryActivity.Query().
		Where(lotteryactivity.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrLotteryActivityNotFound
		}
		return nil, err
	}
	return lotteryActivityToService(m), nil
}

func (r *lotteryRepository) UpdateActivity(ctx context.Context, id int64, input service.LotteryActivityUpdateInput) error {
	client := clientFromContext(ctx, r.client)
	builder := client.LotteryActivity.UpdateOneID(id)
	if input.Name != nil {
		builder.SetName(*input.Name)
	}
	if input.Status != nil {
		builder.SetStatus(*input.Status)
	}
	if input.StartsAt != nil {
		builder.SetStartsAt(*input.StartsAt)
	}
	if input.EndsAt != nil {
		builder.SetEndsAt(*input.EndsAt)
	}
	_, err := builder.Save(ctx)
	return translatePersistenceError(err, service.ErrLotteryActivityNotFound, nil)
}

func (r *lotteryRepository) BumpActivityRulesVersion(ctx context.Context, id int64) (int, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.LotteryActivity.UpdateOneID(id).
		AddRulesVersion(1).
		Save(ctx)
	if err != nil {
		return 0, translatePersistenceError(err, service.ErrLotteryActivityNotFound, nil)
	}
	return m.RulesVersion, nil
}

// EnsureDefaultActivity 幂等兜底: 当库中没有任何活动时创建本地默认活动与
// 奖品(与 migration seed 相同的配置),保证本地环境开箱可用。
func (r *lotteryRepository) EnsureDefaultActivity(ctx context.Context) (*service.LotteryActivity, error) {
	if activity, err := r.GetLatestActivity(ctx); err == nil {
		return activity, nil
	} else if !errors.Is(err, service.ErrLotteryActivityNotFound) {
		return nil, err
	}

	client := clientFromContext(ctx, r.client)
	activity, err := client.LotteryActivity.Create().
		SetName("新人幸运抽奖").
		SetStatus(service.LotteryActivityStatusActive).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	type prizeSeed struct {
		name        string
		typ         string
		value       float64
		weight      int
		minTier     int
		tierWeights map[string]int
		stock       int
		sort        int
	}
	// 阶梯: 0 青铜(<$5) / 1 白银(>=$5) / 2 黄金(>=$15) / 3 钻石(>=$25) / 4 王者(>=$35)。
	// 与 migrations/240_lottery_tier.sql 的 seed 配置保持一致。
	seeds := []prizeSeed{
		{"谢谢参与", service.LotteryPrizeTypeNone, 0, 70, 0, map[string]int{"0": 70, "1": 50, "2": 30, "3": 15, "4": 5}, -1, 1},
		{"$0.1 余额", service.LotteryPrizeTypeBalanceBonus, 0.1, 25, 0, map[string]int{"0": 25, "1": 20, "2": 10, "3": 0, "4": 0}, 100, 2},
		{"$1 余额", service.LotteryPrizeTypeBalanceBonus, 1, 5, 0, map[string]int{"0": 5, "1": 20, "2": 30, "3": 25, "4": 0}, 50, 3},
		{"$2 余额", service.LotteryPrizeTypeBalanceBonus, 2, 0, 1, map[string]int{"0": 0, "1": 10, "2": 20, "3": 30, "4": 25}, 20, 4},
		{"$5 余额", service.LotteryPrizeTypeBalanceBonus, 5, 0, 2, map[string]int{"0": 0, "1": 0, "2": 10, "3": 30, "4": 70}, 10, 5},
	}
	for _, s := range seeds {
		if _, err := client.LotteryPrize.Create().
			SetActivityID(activity.ID).
			SetName(s.name).
			SetPrizeType(s.typ).
			SetValue(s.value).
			SetWeight(s.weight).
			SetMinTier(s.minTier).
			SetTierWeights(s.tierWeights).
			SetStock(s.stock).
			SetSortOrder(s.sort).
			Save(ctx); err != nil {
			return nil, err
		}
	}
	return lotteryActivityToService(activity), nil
}

// ---------------------------------------------------------------
// 奖品
// ---------------------------------------------------------------

func (r *lotteryRepository) ListPrizesByActivity(ctx context.Context, activityID int64, enabledOnly bool) ([]service.LotteryPrize, error) {
	client := clientFromContext(ctx, r.client)
	q := client.LotteryPrize.Query().Where(lotteryprize.ActivityIDEQ(activityID))
	if enabledOnly {
		q = q.Where(lotteryprize.EnabledEQ(true))
	}
	items, err := q.
		Order(dbent.Asc(lotteryprize.FieldSortOrder), dbent.Asc(lotteryprize.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return lotteryPrizesToService(items), nil
}

func (r *lotteryRepository) GetPrizeByID(ctx context.Context, id int64) (*service.LotteryPrize, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.LotteryPrize.Query().
		Where(lotteryprize.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrLotteryPrizeNotFound
		}
		return nil, err
	}
	p := lotteryPrizeToService(m)
	return &p, nil
}

func (r *lotteryRepository) UpdatePrize(ctx context.Context, id int64, input service.LotteryPrizeUpdateInput) error {
	client := clientFromContext(ctx, r.client)
	builder := client.LotteryPrize.UpdateOneID(id)
	if input.Name != nil {
		builder.SetName(*input.Name)
	}
	if input.PrizeType != nil {
		builder.SetPrizeType(*input.PrizeType)
	}
	if input.Value != nil {
		builder.SetValue(*input.Value)
	}
	if input.Weight != nil {
		builder.SetWeight(*input.Weight)
	}
	if input.MinTier != nil {
		builder.SetMinTier(*input.MinTier)
	}
	if input.TierWeights != nil {
		builder.SetTierWeights(*input.TierWeights)
	}
	if input.Stock != nil {
		builder.SetStock(*input.Stock)
	}
	if input.Enabled != nil {
		builder.SetEnabled(*input.Enabled)
	}
	if input.SortOrder != nil {
		builder.SetSortOrder(*input.SortOrder)
	}
	_, err := builder.Save(ctx)
	return translatePersistenceError(err, service.ErrLotteryPrizeNotFound, nil)
}

// DeductPrizeStock 原子扣减库存: 仅当 (stock = -1 OR stock_issued < stock)
// 时将 stock_issued 加一。affected = 0 表示库存已尽或奖品不存在。
func (r *lotteryRepository) DeductPrizeStock(ctx context.Context, prizeID int64) (bool, error) {
	client := clientFromContext(ctx, r.client)
	const deductSQL = `
		UPDATE lottery_prizes
		SET stock_issued = stock_issued + 1, updated_at = NOW()
		WHERE id = $1 AND (stock = -1 OR stock_issued < stock)
	`
	result, err := client.ExecContext(ctx, deductSQL, prizeID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// ---------------------------------------------------------------
// 用户状态
// ---------------------------------------------------------------

func (r *lotteryRepository) GetUserStatsForUpdate(ctx context.Context, userID int64) (*service.LotteryUserStats, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.LotteryUserStats.Query().
		Where(lotteryuserstats.UserIDEQ(userID)).
		ForUpdate().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrLotteryStatsNotFound
		}
		return nil, err
	}
	return lotteryStatsToService(m), nil
}

func (r *lotteryRepository) GetUserStats(ctx context.Context, userID int64) (*service.LotteryUserStats, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.LotteryUserStats.Query().
		Where(lotteryuserstats.UserIDEQ(userID)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrLotteryStatsNotFound
		}
		return nil, err
	}
	return lotteryStatsToService(m), nil
}

func (r *lotteryRepository) CreateUserStats(ctx context.Context, userID int64, firstDrawGranted bool, baselineAt time.Time) (*service.LotteryUserStats, error) {
	client := clientFromContext(ctx, r.client)
	created, err := client.LotteryUserStats.Create().
		SetUserID(userID).
		SetFirstDrawGranted(firstDrawGranted).
		SetBaselineAt(baselineAt).
		Save(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, nil, service.ErrLotteryStatsExists)
	}
	return lotteryStatsToService(created), nil
}

func (r *lotteryRepository) UpdateUserStatsManual(ctx context.Context, userID int64, delta int) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.LotteryUserStats.Update().
		Where(lotteryuserstats.UserIDEQ(userID)).
		AddManualAdjustment(delta).
		Save(ctx)
	return translatePersistenceError(err, service.ErrLotteryStatsNotFound, nil)
}

func (r *lotteryRepository) UpdateUserStatsBaselineAt(ctx context.Context, userID int64, baselineAt time.Time) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.LotteryUserStats.Update().
		Where(lotteryuserstats.UserIDEQ(userID)).
		SetBaselineAt(baselineAt).
		Save(ctx)
	return translatePersistenceError(err, service.ErrLotteryStatsNotFound, nil)
}

func (r *lotteryRepository) UpdateUserStatsSpendOffset(ctx context.Context, userID int64, offsetCents int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.LotteryUserStats.Update().
		Where(lotteryuserstats.UserIDEQ(userID)).
		SetSpendOffsetCents(offsetCents).
		Save(ctx)
	return translatePersistenceError(err, service.ErrLotteryStatsNotFound, nil)
}

func (r *lotteryRepository) DeleteUserStats(ctx context.Context, userID int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.LotteryUserStats.Delete().
		Where(lotteryuserstats.UserIDEQ(userID)).
		Exec(ctx)
	return err
}

// ---------------------------------------------------------------
// 余额消耗统计
// ---------------------------------------------------------------

// SumBalanceSpentSince 汇总用户自 since 起的余额计费实际扣费
// (usage_logs.actual_cost, billing_type = 0 钱包余额)。
func (r *lotteryRepository) SumBalanceSpentSince(ctx context.Context, userID int64, since time.Time) (float64, error) {
	client := clientFromContext(ctx, r.client)
	const sumSQL = `
		SELECT COALESCE(SUM(actual_cost), 0)
		FROM usage_logs
		WHERE user_id = $1 AND billing_type = 0 AND created_at > $2
	`
	rows, err := client.QueryContext(ctx, sumSQL, userID, since)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var spent float64
	if rows.Next() {
		if err := rows.Scan(&spent); err != nil {
			return 0, err
		}
	}
	return spent, rows.Err()
}

// ---------------------------------------------------------------
// 抽奖流水
// ---------------------------------------------------------------

func (r *lotteryRepository) CountDraws(ctx context.Context, userID int64) (int, error) {
	client := clientFromContext(ctx, r.client)
	return client.LotteryDraw.Query().
		Where(lotterydraw.UserIDEQ(userID)).
		Count(ctx)
}

func (r *lotteryRepository) GetDrawByIdempotencyKey(ctx context.Context, userID int64, key string) (*service.LotteryDrawRecord, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.LotteryDraw.Query().
		Where(lotterydraw.UserIDEQ(userID), lotterydraw.IdempotencyKeyEQ(key)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrLotteryDrawNotFound
		}
		return nil, err
	}
	return lotteryDrawToService(m), nil
}

func (r *lotteryRepository) CreateDraw(ctx context.Context, record *service.LotteryDrawRecord) error {
	client := clientFromContext(ctx, r.client)
	builder := client.LotteryDraw.Create().
		SetUserID(record.UserID).
		SetActivityID(record.ActivityID).
		SetPrizeName(record.PrizeName).
		SetPrizeType(record.PrizeType).
		SetPrizeValue(record.PrizeValue).
		SetRulesVersion(record.RulesVersion).
		SetSource(record.Source).
		SetBalanceSpentAtDraw(record.BalanceSpentAtDraw).
		SetFulfillmentStatus(record.FulfillmentStatus).
		SetIdempotencyKey(record.IdempotencyKey)
	if record.PrizeID != nil {
		builder.SetPrizeID(*record.PrizeID)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrLotteryDrawIdempotencyConflict)
	}
	record.ID = created.ID
	record.CreatedAt = created.CreatedAt
	return nil
}

func (r *lotteryRepository) GetDrawByID(ctx context.Context, id int64) (*service.LotteryDrawRecord, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.LotteryDraw.Query().
		Where(lotterydraw.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrLotteryDrawNotFound
		}
		return nil, err
	}
	return lotteryDrawToService(m), nil
}

func (r *lotteryRepository) UpdateDrawFulfillment(ctx context.Context, id int64, status string, fulfilledAt *time.Time, failureReason *string) error {
	client := clientFromContext(ctx, r.client)
	builder := client.LotteryDraw.UpdateOneID(id).
		SetFulfillmentStatus(status)
	if fulfilledAt != nil {
		builder.SetFulfilledAt(*fulfilledAt)
	}
	if failureReason != nil {
		builder.SetFulfillmentError(*failureReason)
	}
	_, err := builder.Save(ctx)
	return translatePersistenceError(err, service.ErrLotteryDrawNotFound, nil)
}

func lotteryDrawListOrder(params pagination.PaginationParams) []func(*entsql.Selector) {
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)
	if sortOrder == pagination.SortOrderAsc {
		return []func(*entsql.Selector){dbent.Asc(lotterydraw.FieldID)}
	}
	return []func(*entsql.Selector){dbent.Desc(lotterydraw.FieldID)}
}

func (r *lotteryRepository) ListDrawsByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.LotteryDrawRecord, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q := client.LotteryDraw.Query().Where(lotterydraw.UserIDEQ(userID))

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	drawsQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range lotteryDrawListOrder(params) {
		drawsQuery = drawsQuery.Order(order)
	}
	items, err := drawsQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}
	return lotteryDrawsToService(items), paginationResultFromTotal(int64(total), params), nil
}

func (r *lotteryRepository) ListRewardsByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.LotteryDrawRecord, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q := client.LotteryDraw.Query().
		Where(lotterydraw.UserIDEQ(userID), lotterydraw.PrizeTypeNEQ(service.LotteryPrizeTypeNone))

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	drawsQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range lotteryDrawListOrder(params) {
		drawsQuery = drawsQuery.Order(order)
	}
	items, err := drawsQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}
	return lotteryDrawsToService(items), paginationResultFromTotal(int64(total), params), nil
}

func (r *lotteryRepository) ListDrawsAdmin(ctx context.Context, filter service.LotteryDrawListFilter, params pagination.PaginationParams) ([]service.LotteryDrawRecord, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q := client.LotteryDraw.Query()
	if filter.UserID != nil {
		q = q.Where(lotterydraw.UserIDEQ(*filter.UserID))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	drawsQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range lotteryDrawListOrder(params) {
		drawsQuery = drawsQuery.Order(order)
	}
	items, err := drawsQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}
	return lotteryDrawsToService(items), paginationResultFromTotal(int64(total), params), nil
}

func (r *lotteryRepository) DeleteDrawsByUser(ctx context.Context, userID int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.LotteryDraw.Delete().
		Where(lotterydraw.UserIDEQ(userID)).
		Exec(ctx)
	return err
}
