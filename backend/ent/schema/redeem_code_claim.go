package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type RedeemCodeClaim struct{ ent.Schema }

func (RedeemCodeClaim) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "redeem_code_claims"}}
}

func (RedeemCodeClaim) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("redeem_code_id"),
		field.Int64("user_id"),
		field.String("email").NotEmpty(),
		field.String("type").NotEmpty(),
		field.Float("value").SchemaType(map[string]string{"postgres": "decimal(20,8)"}),
		field.Int64("group_id").Optional().Nillable(),
		field.Int("validity_days").Default(30),
		field.Time("claimed_at").Default(time.Now),
	}
}

func (RedeemCodeClaim) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("redeem_code_id", "user_id").Unique(),
		index.Fields("redeem_code_id", "email").Unique(),
		index.Fields("user_id"),
	}
}
