//go:build integration

package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestMigrationsOfficial016KeepCustomLedgerAndSchema(t *testing.T) {
	ctx := context.Background()
	names := []string{"242_lottery.sql", "242_drop_platform_check_constraints.sql", "242_group_i18n.sql", "243_lottery_tier.sql", "243_gemini_cached_contents.sql", "244_lottery_activity_tier_config.sql", "244_gemini_cached_contents_channel_usage.sql", "245_lottery_dynamic_tiers.sql", "245_gemini_cached_contents_mutation_lease.sql", "249_multi_use_redeem_codes.sql"}
	for _, name := range names {
		content, err := dbmigrations.FS.ReadFile(name)
		require.NoError(t, err)
		var checksum string
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE filename=$1", name).Scan(&checksum))
		sum := sha256.Sum256([]byte(strings.TrimSpace(string(content))))
		require.Equal(t, hex.EncodeToString(sum[:]), checksum, name)
	}
	tx := testTx(t)
	requireColumn(t, tx, "groups", "i18n", "jsonb", 0, false)
	requireColumn(t, tx, "gemini_cached_contents", "public_id", "character varying", 64, false)
	requireColumn(t, tx, "gemini_cached_contents", "channel_usage", "jsonb", 0, false)
	requireColumn(t, tx, "gemini_cached_contents", "mutation_token", "character varying", 64, true)
	requireIndex(t, tx, "gemini_cached_contents", "idx_gemini_cached_contents_pending_mutation")
	requireColumn(t, tx, "redeem_codes", "max_uses", "integer", 0, false)
	requireColumn(t, tx, "lottery_activities", "name", "character varying", 100, false)
}

// Upgrade a separate disposable database from the exact 0.2.15-r2 migration set.
func TestMigrationsUpgradeOfficial016From015PreservesCustomData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	name := fmt.Sprintf("validation_016_%d", time.Now().UnixNano())
	_, err := integrationDB.ExecContext(ctx, `CREATE DATABASE "`+name+`"`)
	require.NoError(t, err)
	var db *sql.DB
	t.Cleanup(func() {
		if db != nil {
			_ = db.Close()
		}
		_, err := integrationDB.ExecContext(context.Background(), `DROP DATABASE "`+name+`"`)
		require.NoError(t, err, "cleanup only the isolated test database")
	})
	uri, err := url.Parse(integrationDSN)
	require.NoError(t, err)
	uri.Path = "/" + name
	db, err = openSQLWithRetry(ctx, uri.String(), 10*time.Second)
	require.NoError(t, err)
	// Keep the exact published 0.2.15-r2 baseline, excluding all later additions.
	added := map[string]bool{
		"242_group_i18n.sql": true, "243_gemini_cached_contents.sql": true,
		"244_gemini_cached_contents_channel_usage.sql": true, "245_gemini_cached_contents_mutation_lease.sql": true,
		"250_agent_workspace.sql": true, "251_agent_conversation_management.sql": true,
		"252_agent_file_attachments.sql": true, "253_agent_attachment_mime.sql": true,
		"254_agent_security_storage.sql": true, "255_agent_work_plans.sql": true,
		"256_agent_adaptive_runtime.sql": true, "257_agent_storage_maintenance_indexes.sql": true,
		"258_upstream_balance_monitor.sql": true,
	}
	files, err := fs.Glob(dbmigrations.FS, "*.sql")
	require.NoError(t, err)
	baseline := fstest.MapFS{}
	var baselineNames []string
	for _, file := range files {
		if added[file] {
			continue
		}
		body, err := dbmigrations.FS.ReadFile(file)
		require.NoError(t, err)
		baseline[file] = &fstest.MapFile{Data: body}
		baselineNames = append(baselineNames, file)
	}
	require.Len(t, baseline, 301, "published r2 migrations must all be retained")
	require.NoError(t, applyMigrationsFS(ctx, db, baseline))
	_, err = db.ExecContext(ctx, `INSERT INTO groups(name,description,platform,rate_multiplier) VALUES('legacy-validation-group','custom display text','anthropic',0.25)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO lottery_activities(name,status) VALUES('legacy-lottery-validation','active')`)
	require.NoError(t, err)
	var oldLedger string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT md5(string_agg(filename||':'||checksum,'|' ORDER BY filename)) FROM schema_migrations`).Scan(&oldLedger))
	require.NoError(t, applyMigrationsFS(ctx, db, dbmigrations.FS))
	require.NoError(t, applyMigrationsFS(ctx, db, dbmigrations.FS), "upgrade is idempotent")
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&count))
	require.Equal(t, len(files), count, "every current migration must be applied")
	var sameLedger string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT md5(string_agg(filename||':'||checksum,'|' ORDER BY filename)) FROM schema_migrations WHERE filename = ANY($1)`, pq.Array(baselineNames)).Scan(&sameLedger))
	require.Equal(t, oldLedger, sameLedger, "all prior ledger entries remain unchanged")
	var description string
	var rate float64
	var i18n string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT description,rate_multiplier,i18n::text FROM groups WHERE name='legacy-validation-group'`).Scan(&description, &rate, &i18n))
	require.Equal(t, "custom display text", description)
	require.Equal(t, 0.25, rate)
	require.Equal(t, "{}", i18n)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM lottery_activities WHERE name='legacy-lottery-validation' AND status='active'`).Scan(&count))
	require.Equal(t, 1, count)
	var cacheTable string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT to_regclass('public.gemini_cached_contents')::text`).Scan(&cacheTable))
	require.Equal(t, "gemini_cached_contents", cacheTable)
	for _, table := range []string{"upstream_balance_sites", "upstream_balance_events", "upstream_balance_deliveries", "upstream_balance_channel"} {
		var found string
		require.NoError(t, db.QueryRowContext(ctx, "SELECT to_regclass($1)::text", "public."+table).Scan(&found))
		require.Equal(t, table, found, "new monitor table must be applied by the upgrade")
	}
}
