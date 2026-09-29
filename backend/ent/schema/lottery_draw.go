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

// LotteryDraw holds the schema definition for the LotteryDraw entity.
//
// 抽奖流水表(只追加，不可修改)。每次成功的抽奖写入一行，是抽奖次数
// 使用情况的权威记录(used = 本表按 user_id 计数)。
//
// 幂等: (user_id, idempotency_key) 唯一约束。客户端对同一抽奖请求重试时
// 携带相同 idempotency_key，服务端命中唯一约束即返回已有结果，不重复扣次数。
//
// 奖品字段为快照(中奖时刻的奖品名/类型/值)，奖品配置后续修改不影响历史记录。
type LotteryDraw struct {
	ent.Schema
}

func (LotteryDraw) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "lottery_draws"},
	}
}

func (LotteryDraw) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id").
			Comment("用户 ID"),
		field.Int64("activity_id").
			Comment("活动 ID"),
		field.Int64("prize_id").
			Optional().
			Nillable().
			Comment("奖品 ID，null 仅在异常兜底时出现"),
		field.String("prize_name").
			MaxLen(100).
			Comment("奖品名称快照"),
		field.String("prize_type").
			MaxLen(20).
			Comment("奖品类型快照: none, balance_bonus, quota"),
		field.Float("prize_value").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0).
			Comment("奖品数值快照"),
		field.Int("rules_version").
			Comment("抽奖时的规则版本"),
		field.String("source").
			MaxLen(20).
			Comment("次数来源: first, threshold, manual"),
		field.Float("balance_spent_at_draw").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Default(0).
			Comment("抽奖时的基线后累计余额消耗快照"),
		// 发放状态: pending(待发放/处理中) -> granted(已发放) / failed(发放失败，可重试)。
		// none 类型奖品在写入时直接置 granted。
		field.String("fulfillment_status").
			MaxLen(20).
			Default("pending").
			Comment("发放状态: pending, granted, failed"),
		field.Time("fulfilled_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("发放完成时间"),
		field.String("fulfillment_error").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Comment("发放失败原因"),
		field.String("idempotency_key").
			MaxLen(64).
			Comment("客户端幂等键"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (LotteryDraw) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("user_id", "created_at"),
		index.Fields("activity_id"),
		index.Fields("prize_id"),
		index.Fields("fulfillment_status"),
		// 幂等唯一约束: 部分唯一索引由 migration 016 的模式启发，
		// 这里用普通唯一索引(流水不可删，无需软删除兼容)。
		index.Fields("user_id", "idempotency_key").Unique(),
	}
}
