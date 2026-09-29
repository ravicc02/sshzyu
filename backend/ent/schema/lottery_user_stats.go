package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// LotteryUserStats holds the schema definition for the LotteryUserStats entity.
//
// 用户抽奖状态表(每用户一行)。存放抽奖资格的派生输入，不直接存"可用次数"，
// 可用次数由服务按以下公式实时计算：
//
//	eligible = (first_draw_granted ? 1 : 0)
//	         + threshold_entitlement(spent_since_baseline)
//	         + manual_adjustment
//	used     = count(lottery_draws where user_id = ?)
//	available= max(eligible - used, 0)
//
// threshold_entitlement(spent):
//	spent < $5      -> 0
//	else            -> 1 + floor((spent - $5) / $10)
//
// 金额口径: usage_logs.actual_cost 汇总(billing_type = 0 余额计费)，
// baseline_at 为该用户首次进入抽奖体系的时间戳，只统计其后的新消耗，
// 不追溯历史消耗。
//
// 本行是同一用户并发抽奖的串行化点(draw 事务内 SELECT FOR UPDATE)。
type LotteryUserStats struct {
	ent.Schema
}

func (LotteryUserStats) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "lottery_user_stats"},
	}
}

func (LotteryUserStats) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id").
			Unique().
			Comment("用户 ID"),
		field.Bool("first_draw_granted").
			Default(false).
			Comment("首抽资格是否已发放"),
		// 基线时间戳: 只统计该时刻之后的余额消耗，避免功能启用时
		// 历史消耗一次性解锁大量抽奖次数。
		field.Time("baseline_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("余额消耗基线时间，统计该时刻之后的新增消耗"),
		field.Int("manual_adjustment").
			Default(0).
			Comment("管理员手动补发(可为负)的抽奖次数"),
		// 本地测试偏移:叠加在 usage_logs 汇总之上的消耗偏移量(美分)。
		// 仅本地测试辅助接口写入，生产语义恒为 0，不参与任何真实计费。
		field.Int64("spend_offset_cents").
			Default(0).
			Comment("本地测试用消耗偏移(美分)，叠加在 usage_logs 汇总之上"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (LotteryUserStats) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
	}
}
