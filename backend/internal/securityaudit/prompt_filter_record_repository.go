package securityaudit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type FilterRecord struct {
	ID              int64     `json:"id"`
	RequestID       string    `json:"request_id"`
	UserID          int64     `json:"user_id"`
	Username        string    `json:"username"`
	UserEmail       string    `json:"user_email"`
	APIKeyID        int64     `json:"api_key_id"`
	APIKeyName      string    `json:"api_key_name"`
	GroupID         *int64    `json:"group_id,omitempty"`
	GroupName       string    `json:"group_name"`
	Provider        string    `json:"provider"`
	Endpoint        string    `json:"endpoint"`
	Protocol        string    `json:"protocol"`
	Model           string    `json:"model"`
	PromptHash      string    `json:"prompt_hash"`
	RedactedPreview string    `json:"redacted_preview"`
	PromptLength    int       `json:"prompt_length"`
	MessageCount    int       `json:"message_count"`
	Stage           string    `json:"stage"`
	Result          string    `json:"result"`
	RiskLevel       string    `json:"risk_level"`
	Action          string    `json:"action"`
	Categories      []string  `json:"categories"`
	ConfigVersion   int64     `json:"config_version"`
	LatencyMS       int       `json:"latency_ms"`
	CreatedAt       time.Time `json:"created_at"`
}

type FilterRecordPage struct {
	Items    []*FilterRecord `json:"items"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	Pages    int             `json:"pages"`
}

const filterRecordCTE = `
WITH records AS (
	SELECT
		j.id,
		j.request_id,
		j.user_id,
		j.username_snapshot,
		j.user_email_snapshot,
		j.api_key_id,
		j.api_key_name_snapshot,
		j.group_id,
		j.group_name,
		j.provider,
		j.endpoint,
		j.protocol,
		j.model,
		j.prompt_hash,
		j.redacted_preview,
		j.prompt_length,
		j.message_count,
		j.stage,
		COALESCE(
			e.decision,
			CASE
				WHEN j.status = 'failed' THEN 'error'
				WHEN j.status = 'done' THEN 'pass'
				ELSE 'pending'
			END
		) AS result,
		COALESCE(e.risk_level, 'low') AS risk_level,
		COALESCE(e.action, 'Allow') AS action,
		COALESCE(e.categories, '[]'::jsonb) AS categories,
		j.config_version,
		COALESCE(e.latency_ms, 0) AS latency_ms,
		j.created_at
	FROM prompt_audit_jobs j
	LEFT JOIN LATERAL (
		SELECT decision, risk_level, action, categories, latency_ms
		FROM prompt_audit_events
		WHERE job_id = j.id
		ORDER BY id DESC
		LIMIT 1
	) e ON TRUE
)`

func (r *PostgreSQLRepository) ListFilterRecords(ctx context.Context, filter EventFilter, page, pageSize int) (*FilterRecordPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	where, args := buildFilterRecordWhere(filter, 1)
	var total int64
	if err := r.db.QueryRowContext(ctx, filterRecordCTE+` SELECT COUNT(*) FROM records r`+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	queryArgs := append([]any(nil), args...)
	limitIndex := len(queryArgs) + 1
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, filterRecordCTE+`
		SELECT
			r.id, r.request_id, r.user_id, r.username_snapshot, r.user_email_snapshot,
			r.api_key_id, r.api_key_name_snapshot, r.group_id, r.group_name,
			r.provider, r.endpoint, r.protocol, r.model, r.prompt_hash, r.redacted_preview,
			r.prompt_length, r.message_count, r.stage, r.result, r.risk_level, r.action,
			r.categories, r.config_version, r.latency_ms, r.created_at
		FROM records r`+where+fmt.Sprintf(` ORDER BY r.created_at DESC, r.id DESC LIMIT $%d OFFSET $%d`, limitIndex, limitIndex+1), queryArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]*FilterRecord, 0, pageSize)
	for rows.Next() {
		item := &FilterRecord{}
		var userID, apiKeyID, groupID sql.NullInt64
		var categories []byte
		if err := rows.Scan(
			&item.ID, &item.RequestID, &userID, &item.Username, &item.UserEmail,
			&apiKeyID, &item.APIKeyName, &groupID, &item.GroupName,
			&item.Provider, &item.Endpoint, &item.Protocol, &item.Model, &item.PromptHash,
			&item.RedactedPreview, &item.PromptLength, &item.MessageCount, &item.Stage,
			&item.Result, &item.RiskLevel, &item.Action, &categories, &item.ConfigVersion,
			&item.LatencyMS, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		item.UserID = nullableInt64Value(userID)
		item.APIKeyID = nullableInt64Value(apiKeyID)
		item.GroupID = nullableInt64Ptr(groupID)
		_ = json.Unmarshal(categories, &item.Categories)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	pages := 0
	if total > 0 {
		pages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return &FilterRecordPage{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: pages}, nil
}

func buildFilterRecordWhere(filter EventFilter, firstIndex int) (string, []any) {
	filter = canonicalEventFilter(filter)
	clauses := []string{" WHERE TRUE"}
	args := make([]any, 0, 12)
	add := func(clause string, value any) {
		clauses = append(clauses, fmt.Sprintf(clause, firstIndex+len(args)))
		args = append(args, value)
	}
	if filter.Decision != "" {
		add(" AND r.result=$%d", filter.Decision)
	}
	if filter.RiskLevel != "" {
		add(" AND r.risk_level=$%d", filter.RiskLevel)
	}
	if filter.Endpoint != "" {
		add(" AND r.endpoint=$%d", filter.Endpoint)
	}
	if filter.GroupID != nil {
		add(" AND r.group_id=$%d", *filter.GroupID)
	}
	if filter.UserID != nil {
		add(" AND r.user_id=$%d", *filter.UserID)
	}
	if filter.APIKeyID != nil {
		add(" AND r.api_key_id=$%d", *filter.APIKeyID)
	}
	if filter.RequestID != "" {
		add(" AND r.request_id=$%d", filter.RequestID)
	}
	if filter.PromptHash != "" {
		add(" AND r.prompt_hash=$%d", filter.PromptHash)
	}
	if filter.Keyword != "" {
		index := firstIndex + len(args)
		clauses = append(clauses, fmt.Sprintf(` AND (
			r.request_id ILIKE $%[1]d OR r.prompt_hash ILIKE $%[1]d OR r.redacted_preview ILIKE $%[1]d
			OR r.username_snapshot ILIKE $%[1]d OR r.user_email_snapshot ILIKE $%[1]d
			OR r.api_key_name_snapshot ILIKE $%[1]d OR r.model ILIKE $%[1]d
		)`, index))
		args = append(args, "%"+strings.TrimSpace(filter.Keyword)+"%")
	}
	if filter.StartAt != nil {
		add(" AND r.created_at >= $%d", filter.StartAt.UTC())
	}
	if filter.EndAt != nil {
		add(" AND r.created_at <= $%d", filter.EndAt.UTC())
	}
	return strings.Join(clauses, ""), args
}
