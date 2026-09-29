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

// LotteryPrize holds the schema definition for the LotteryPrize entity.
//
// 抽奖奖品配置。阶梯抽奖模型:
//   1. 按用户基线后累计余额消耗确定阶梯 tier(0 青铜/1 白银/2 黄金/3 钻石/4 王者);
//   2. 候选奖品 = 同活动 enabled 且 min_tier <= 用户 tier 的奖品;
//   3. 每个候选奖品权重 = tier_weights[tier] ?? weight(基础权重);
//   4. 中奖概率 = 该奖品权重 / sum(候选奖品权重)。
// 由服务端计算与执行，前端只做展示。
//
// prize_type 说明:
//   - none:           谢谢参与/未中奖，无限库存，不发奖
//   - balance_bonus:  赠送账户余额(本地测试环境使用，通过 AdjustBalance 原子发放)
//   - quota:          API 使用额度(本地 MVP 为记录型发放，标记 granted 即完成)
//
// stock 语义: -1 表示无限库存(兜底奖品)；>= 0 表示有限库存，发放前原子扣减。
type LotteryPrize struct {
	ent.Schema
}

func (LotteryPrize) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "lottery_prizes"},
	}
}

func (LotteryPrize) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("activity_id").
			Comment("所属活动 ID"),
		field.String("name").
			MaxLen(100).
			NotEmpty().
			Comment("奖品名称"),
		field.String("prize_type").
			MaxLen(20).
			Default("none").
			Comment("奖品类型: none, balance_bonus, quota"),
		field.Float("value").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0).
			Comment("奖品数值: balance_bonus 为金额，quota 为额度，none 为 0"),
		field.Int("weight").
			Default(0).
			Comment("中奖权重(基础)，0 表示不参与随机；tier_weights 未覆盖的 tier 使用该值"),
		field.Int("min_tier").
			Default(0).
			Comment("可中该奖品的最低用户阶梯(0 青铜/1 白银/2 黄金/3 钻石/4 王者)，低于该阶梯的奖品不参与随机"),
		field.JSON("tier_weights", map[string]int{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Comment("按阶梯覆盖权重: key 为 tier 数字字符串，如 {\"0\":70,\"1\":50}；缺失的 tier 回退 weight，显式 0 表示该 tier 不可中"),
		field.Int("stock").
			Default(-1).
			Comment("库存，-1 表示无限"),
		field.Int("stock_issued").
			Default(0).
			Comment("已发放数量"),
		field.Bool("enabled").
			Default(true).
			Comment("是否启用"),
		field.Int("sort_order").
			Default(0).
			Comment("展示排序"),
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

func (LotteryPrize) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("activity", LotteryActivity.Type).
			Ref("prizes").
			Field("activity_id").
			Required().
			Unique(),
	}
}

func (LotteryPrize) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("activity_id"),
		index.Fields("activity_id", "enabled"),
	}
}
