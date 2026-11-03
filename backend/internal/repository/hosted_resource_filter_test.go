//go:build unit

package repository

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestHostedResourceFilterUsesBindings(t *testing.T) {
	for _, tc := range []struct {
		table  string
		column string
	}{
		{"accounts", "account_id"},
		{"groups", "group_id"},
	} {
		for _, ownership := range []string{"standard", "hosted", "all"} {
			t.Run(tc.table+"/"+ownership, func(t *testing.T) {
				ctx, err := service.WithAccountOwnershipFilter(context.Background(), ownership)
				require.NoError(t, err)
				table := entsql.Table(tc.table)
				query := entsql.Dialect(dialect.Postgres).Select(table.C("id")).From(table)
				applyHostedResourceFilter(ctx, query, tc.column)
				statement, _ := query.Query()
				if ownership == "all" {
					require.NotContains(t, statement, "hosted_openai_accounts")
					return
				}
				require.Contains(t, statement, "hosted_openai_accounts")
				require.Contains(t, statement, tc.column)
				require.Contains(t, statement, `"`+tc.table+`"."id"`)
				require.NotContains(t, statement, "status", "archiving must not move a hosted resource into the platform list")
				if ownership == "standard" {
					require.Contains(t, statement, "NOT")
				} else {
					require.NotContains(t, statement, "NOT")
				}
			})
		}
	}
}
