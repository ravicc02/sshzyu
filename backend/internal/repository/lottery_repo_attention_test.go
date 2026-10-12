package repository

import (
	"context"
	"database/sql/driver"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestLotteryDrawsNeedsAttentionFilter(t *testing.T) {
	for _, tc := range []struct {
		name      string
		attention bool
		user      bool
	}{
		{"all", false, false}, {"attention", true, false}, {"attention and user", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() })
			filter := service.LotteryDrawListFilter{NeedsAttention: tc.attention}
			pattern := `FROM "lottery_draws"`
			args := []driver.Value{}
			if tc.user {
				uid := int64(42)
				filter.UserID = &uid
				pattern += ` WHERE "lottery_draws"\."user_id" = \$1 AND`
				args = append(args, uid)
			}
			if tc.attention {
				pattern += `.*"fulfillment_status" IN \(\$[0-9]+, \$[0-9]+, \$[0-9]+\)`
				args = append(args, "pending", "pending_review", "failed")
			}
			mock.ExpectQuery(`SELECT COUNT.*` + pattern).WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(7))
			mock.ExpectQuery(`SELECT .*` + pattern + `.*LIMIT 1`).WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"id"}))
			_, page, err := NewLotteryRepository(client).ListDrawsAdmin(context.Background(), filter, pagination.PaginationParams{Page: 1, PageSize: 1})
			require.NoError(t, err)
			require.EqualValues(t, 7, page.Total)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
