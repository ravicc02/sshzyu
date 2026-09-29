package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// LotteryActivity holds the schema definition for the LotteryActivity entity.
//
// 抽奖活动配置。本地 MVP 只维护一个活动，但表结构支持多个活动并存。
// 状态机: draft(草稿) -> active(进行中) -> paused(暂停) -> ended(结束)。
// 暂停/结束的活动不允许抽奖，已产生的历史记录保留。
type LotteryActivity struct {
	ent.Schema
}

func (LotteryActivity) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "lottery_activities"},
	}
}

func (LotteryActivity) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(100).
			NotEmpty().
			Comment("活动名称"),
		field.String("status").
			MaxLen(20).
			Default("draft").
			Comment("状态: draft, active, paused, ended"),
		// 规则版本: 每次修改奖品/权重/阈值规则时递增，写入抽奖流水，
		// 保证历史记录可以按当时的规则解释。
		field.Int("rules_version").
			Default(1).
			Comment("抽奖规则版本号，随规则修改递增"),
		field.Time("starts_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("活动开始时间，null 表示立即开始"),
		field.Time("ends_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("活动结束时间，null 表示不自动结束"),
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

func (LotteryActivity) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("prizes", LotteryPrize.Type),
	}
}

func (LotteryActivity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
	}
}
