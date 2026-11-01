package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type accountSchedulerRepository struct {
	db *sql.DB
}

func NewAccountSchedulerRepository(db *sql.DB) service.AccountSchedulerRepository {
	return &accountSchedulerRepository{db: db}
}

func (r *accountSchedulerRepository) ListGroups(ctx context.Context) ([]service.AccountSchedulerGroup, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			g.id,
			g.name,
			g.platform,
			g.status,
			COALESCE(g.model_allowlist->'models', '[]'::jsonb)::text,
			COALESCE((
				SELECT array_agg(model ORDER BY request_count DESC, model ASC)
				FROM (
					SELECT model, COUNT(*) AS request_count
					FROM usage_logs
					WHERE group_id = g.id
						AND created_at >= NOW() - INTERVAL '7 days'
						AND COALESCE(model, '') <> ''
					GROUP BY model
					ORDER BY request_count DESC, model ASC
					LIMIT 5
				) recent_models
			), '{}'::text[])
		FROM groups g
		WHERE g.deleted_at IS NULL
			AND g.status = $1
		ORDER BY g.sort_order ASC, g.id ASC
	`, service.StatusActive)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	groups := make([]service.AccountSchedulerGroup, 0)
	for rows.Next() {
		var group service.AccountSchedulerGroup
		var allowlistRaw string
		var recentModels pq.StringArray
		if err := rows.Scan(&group.ID, &group.Name, &group.Platform, &group.Status, &allowlistRaw, &recentModels); err != nil {
			return nil, err
		}
		group.ModelCandidates = mergeAccountSchedulerModels(decodeAccountSchedulerAllowlist(allowlistRaw), recentModels)
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (r *accountSchedulerRepository) ListGroupSettings(ctx context.Context) (map[int64]service.AccountSchedulerGroupSetting, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			group_id,
			degradation_enabled,
			degradation_factor_enabled,
			interval_seconds,
			test_model,
			test_reasoning_effort,
			updated_at
		FROM account_scheduler_group_settings
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	settings := make(map[int64]service.AccountSchedulerGroupSetting)
	for rows.Next() {
		var setting service.AccountSchedulerGroupSetting
		if err := rows.Scan(
			&setting.GroupID,
			&setting.DegradationEnabled,
			&setting.DegradationFactorEnabled,
			&setting.IntervalSeconds,
			&setting.TestModel,
			&setting.TestReasoningEffort,
			&setting.UpdatedAt,
		); err != nil {
			return nil, err
		}
		settings[setting.GroupID] = setting
	}
	return settings, rows.Err()
}

func (r *accountSchedulerRepository) ListGroupPrimaryModels(
	ctx context.Context,
	groupIDs []int64,
) (map[int64]string, error) {
	if len(groupIDs) == 0 {
		return map[int64]string{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT ON (group_id)
			group_id,
			primary_model
		FROM channel_monitors
		WHERE group_id = ANY($1)
			AND provider = 'openai'
		ORDER BY group_id, updated_at DESC, id DESC
	`, pq.Array(groupIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int64]string, len(groupIDs))
	for rows.Next() {
		var groupID int64
		var model string
		if err := rows.Scan(&groupID, &model); err != nil {
			return nil, err
		}
		if groupID > 0 && strings.TrimSpace(model) != "" {
			out[groupID] = strings.TrimSpace(model)
		}
	}
	return out, rows.Err()
}

func (r *accountSchedulerRepository) UpsertGroupSetting(
	ctx context.Context,
	setting service.AccountSchedulerGroupSetting,
) (*service.AccountSchedulerGroupSetting, error) {
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO account_scheduler_group_settings (
			group_id, degradation_enabled, degradation_factor_enabled,
			interval_seconds, test_model, test_reasoning_effort, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		ON CONFLICT (group_id) DO UPDATE SET
			degradation_enabled = EXCLUDED.degradation_enabled,
			degradation_factor_enabled = EXCLUDED.degradation_factor_enabled,
			interval_seconds = EXCLUDED.interval_seconds,
			test_model = EXCLUDED.test_model,
			test_reasoning_effort = EXCLUDED.test_reasoning_effort,
			updated_at = NOW()
		RETURNING
			group_id,
			degradation_enabled,
			degradation_factor_enabled,
			interval_seconds,
			test_model,
			test_reasoning_effort,
			updated_at
	`,
		setting.GroupID,
		setting.DegradationEnabled,
		setting.DegradationFactorEnabled,
		setting.IntervalSeconds,
		setting.TestModel,
		setting.TestReasoningEffort,
	)
	out := &service.AccountSchedulerGroupSetting{}
	if err := row.Scan(
		&out.GroupID,
		&out.DegradationEnabled,
		&out.DegradationFactorEnabled,
		&out.IntervalSeconds,
		&out.TestModel,
		&out.TestReasoningEffort,
		&out.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *accountSchedulerRepository) ListGroupAccounts(
	ctx context.Context,
	groupIDs []int64,
) ([]service.AccountSchedulerAccount, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		WITH dispatch_counts AS (
			SELECT
				group_id,
				account_id,
				COUNT(*) AS dispatch_count
			FROM usage_logs
			WHERE created_at >= NOW() - INTERVAL '24 hours'
				AND group_id = ANY($1)
			GROUP BY group_id, account_id
		)
		SELECT
			a.id,
			ag.group_id,
			a.name,
			a.platform,
			a.type,
			a.status,
			a.schedulable,
			a.priority,
			ag.priority,
			a.concurrency,
			a.rate_multiplier,
			a.rate_limit_reset_at,
			a.overload_until,
			a.temp_unschedulable_until,
			COALESCE(a.temp_unschedulable_reason, ''),
			a.expires_at,
			a.auto_pause_on_expired,
			COALESCE(dc.dispatch_count, 0)
		FROM account_groups ag
		JOIN accounts a ON a.id = ag.account_id
		LEFT JOIN dispatch_counts dc
			ON dc.group_id = ag.group_id
			AND dc.account_id = a.id
		WHERE ag.group_id = ANY($1)
			AND a.deleted_at IS NULL
		ORDER BY ag.group_id ASC, ag.priority ASC, a.priority ASC, a.id ASC
	`, pq.Array(groupIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	accounts := make([]service.AccountSchedulerAccount, 0)
	for rows.Next() {
		var account service.AccountSchedulerAccount
		if err := rows.Scan(
			&account.ID,
			&account.GroupID,
			&account.Name,
			&account.Platform,
			&account.Type,
			&account.Status,
			&account.Schedulable,
			&account.Priority,
			&account.GroupPriority,
			&account.Concurrency,
			&account.RateMultiplier,
			&account.RateLimitResetAt,
			&account.OverloadUntil,
			&account.TempUnschedulableUntil,
			&account.TempUnschedulableReason,
			&account.ExpiresAt,
			&account.AutoPauseOnExpired,
			&account.DispatchCount,
		); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (r *accountSchedulerRepository) ListDegradationStats(
	ctx context.Context,
	since time.Time,
) ([]service.AccountDegradationStat, error) {
	rows, err := r.db.QueryContext(ctx, `
		WITH ranked AS (
			SELECT
				group_id,
				account_id,
				status,
				latency_ms,
				checked_at,
				ROW_NUMBER() OVER (
					PARTITION BY group_id, account_id
					ORDER BY checked_at DESC, id DESC
				) AS row_number
			FROM account_degradation_probes
			WHERE checked_at >= $1
		)
		SELECT
			group_id,
			account_id,
			COUNT(*) FILTER (WHERE status = 'pass') AS pass_count,
			COUNT(*) FILTER (WHERE status = 'degraded') AS degraded_count,
			COUNT(*) FILTER (WHERE status = 'error') AS error_count,
			BOOL_OR(row_number = 1) AS has_latest,
			(ARRAY_AGG(status ORDER BY checked_at DESC, row_number ASC))[1] AS latest_status,
			(ARRAY_AGG(checked_at ORDER BY checked_at DESC, row_number ASC))[1] AS latest_checked_at,
			COALESCE(AVG(latency_ms), 0) AS average_latency_ms
		FROM ranked
		GROUP BY group_id, account_id
	`, since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	stats := make([]service.AccountDegradationStat, 0)
	for rows.Next() {
		var stat service.AccountDegradationStat
		var hasLatest bool
		if err := rows.Scan(
			&stat.GroupID,
			&stat.AccountID,
			&stat.PassCount,
			&stat.DegradedCount,
			&stat.ErrorCount,
			&hasLatest,
			&stat.LatestStatus,
			&stat.LatestCheckedAt,
			&stat.AverageLatencyMs,
		); err != nil {
			return nil, err
		}
		if !hasLatest {
			stat.LatestStatus = ""
			stat.LatestCheckedAt = nil
		}
		stats = append(stats, stat)
	}
	return stats, rows.Err()
}

func (r *accountSchedulerRepository) ListRecentProbes(
	ctx context.Context,
	groupID int64,
	limit int,
) ([]service.AccountDegradationProbe, error) {
	if limit <= 0 || limit > 200 {
		limit = 80
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			p.id,
			p.account_id,
			COALESCE(a.name, ''),
			p.group_id,
			p.model,
			p.reasoning_effort,
			p.status,
			p.response_text,
			p.error_message,
			p.latency_ms,
			p.checked_at
		FROM account_degradation_probes p
		LEFT JOIN accounts a ON a.id = p.account_id
		WHERE ($1 = 0 OR p.group_id = $1)
		ORDER BY p.checked_at DESC, p.id DESC
		LIMIT $2
	`, groupID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	probes := make([]service.AccountDegradationProbe, 0)
	for rows.Next() {
		var probe service.AccountDegradationProbe
		if err := rows.Scan(
			&probe.ID,
			&probe.AccountID,
			&probe.AccountName,
			&probe.GroupID,
			&probe.Model,
			&probe.ReasoningEffort,
			&probe.Status,
			&probe.ResponseText,
			&probe.ErrorMessage,
			&probe.LatencyMs,
			&probe.CheckedAt,
		); err != nil {
			return nil, err
		}
		probes = append(probes, probe)
	}
	return probes, rows.Err()
}

func (r *accountSchedulerRepository) InsertProbe(
	ctx context.Context,
	probe *service.AccountDegradationProbe,
) error {
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO account_degradation_probes (
			account_id, group_id, model, reasoning_effort, status, response_text, error_message,
			latency_ms, checked_at, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		RETURNING id
	`,
		probe.AccountID,
		probe.GroupID,
		probe.Model,
		probe.ReasoningEffort,
		probe.Status,
		probe.ResponseText,
		probe.ErrorMessage,
		probe.LatencyMs,
		probe.CheckedAt,
	)
	return row.Scan(&probe.ID)
}

func decodeAccountSchedulerAllowlist(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var models []string
	if err := json.Unmarshal([]byte(raw), &models); err != nil {
		return nil
	}
	return models
}

func mergeAccountSchedulerModels(groups ...[]string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, models := range groups {
		for _, model := range models {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			if _, ok := seen[model]; ok {
				continue
			}
			seen[model] = struct{}{}
			result = append(result, model)
		}
	}
	return result
}
