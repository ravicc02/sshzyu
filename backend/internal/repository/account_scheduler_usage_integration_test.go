//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Temporary tables shadow the fixture database only on this transaction's
// dedicated connection. Production tables are never used by this test.
const accountSchedulerUsageFixtureSQL = `
 CREATE TEMP TABLE groups (
  id bigint PRIMARY KEY, name text, platform text, status text,
  model_allowlist jsonb, sort_order integer, deleted_at timestamptz
 ) ON COMMIT DROP;
 CREATE TEMP TABLE usage_logs (
  group_id bigint, account_id bigint, model text, created_at timestamptz
 ) ON COMMIT DROP;
 INSERT INTO groups VALUES
  (1,'one','openai','active','{"models":["allowlisted"]}',1,NULL),
  (2,'two','openai','active','{}',2,NULL),
  (3,'disabled','openai','disabled','{}',3,NULL),
  (4,'deleted','openai','active','{}',4,NOW()),
  (5,'empty','openai','active','{}',5,NULL);
 INSERT INTO usage_logs
 SELECT 1,10,model,NOW() FROM (VALUES ('a',4),('b',4),('c',3),('d',3),('e',2),('f',2),('',20),(NULL,30)) AS counts(model,n)
 CROSS JOIN LATERAL generate_series(1,counts.n);
 INSERT INTO usage_logs SELECT 1,11,'weekly',NOW()-INTERVAL '3 days' FROM generate_series(1,10);
 INSERT INTO usage_logs SELECT 1,11,'expired',NOW()-INTERVAL '8 days' FROM generate_series(1,100);
 INSERT INTO usage_logs SELECT 2,20,'only',NOW() FROM generate_series(1,2);
 INSERT INTO usage_logs SELECT 2,20,NULL,NOW() FROM generate_series(1,3);
 INSERT INTO usage_logs SELECT 2,20,'',NOW() FROM generate_series(1,4);
 INSERT INTO usage_logs SELECT 3,30,'disabled-model',NOW() FROM generate_series(1,8);
 INSERT INTO usage_logs SELECT 4,40,'deleted-model',NOW() FROM generate_series(1,8);
 CREATE INDEX ON usage_logs(group_id,created_at);
`

func TestAccountSchedulerUsageQueryWindowsAndTies(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, accountSchedulerUsageFixtureSQL)
	require.NoError(t, err)
	ids := pq.Array([]int64{1, 2, 5})
	rows, err := tx.QueryContext(ctx, accountSchedulerRecentModelsSQL, ids)
	require.NoError(t, err)
	models := map[int64][]string{}
	for rows.Next() {
		var groupID int64
		var names pq.StringArray
		require.NoError(t, rows.Scan(&groupID, &names))
		models[groupID] = []string(names)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Equal(t, map[int64][]string{1: {"weekly", "a", "b", "c", "d"}, 2: {"only"}}, models)
	rows, err = tx.QueryContext(ctx, accountSchedulerDispatchCountsSQL, ids)
	require.NoError(t, err)
	counts := map[[2]int64]int64{}
	for rows.Next() {
		var groupID, accountID, count int64
		require.NoError(t, rows.Scan(&groupID, &accountID, &count))
		counts[[2]int64{groupID, accountID}] = count
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Equal(t, map[[2]int64]int64{{1, 10}: 68, {2, 20}: 9}, counts)
}
