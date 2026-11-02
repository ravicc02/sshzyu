package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type hostedOpenAIAccountRepository struct {
	db *sql.DB
}

func NewHostedOpenAIAccountRepository(db *sql.DB) service.HostedOpenAIAccountRepository {
	return &hostedOpenAIAccountRepository{db: db}
}

const hostedOpenAIAccountSelect = `
	SELECT
		h.id,
		h.user_id,
		h.account_id,
		h.group_id,
		h.subscription_id,
		h.proxy_id,
		h.account_name,
		h.model,
		h.models,
		h.supports_messages,
		h.status,
		h.account_email,
		h.plan_type,
		h.last_quota_refreshed_at,
		h.created_at,
		h.updated_at,
		COALESCE(NULLIF(u.username, ''), u.email, ''),
		COALESCE(u.email, ''),
		COALESCE(g.name, ''),
		COALESCE(p.name, '')
	FROM hosted_openai_accounts h
	JOIN users u ON u.id = h.user_id
	JOIN groups g ON g.id = h.group_id
	LEFT JOIN proxies p ON p.id = h.proxy_id
`

type hostedOpenAIAccountScanner interface {
	Scan(dest ...any) error
}

func scanHostedOpenAIAccount(scanner hostedOpenAIAccountScanner) (*service.HostedOpenAIAccountRecord, error) {
	var (
		record           service.HostedOpenAIAccountRecord
		subscriptionID   sql.NullInt64
		proxyID          sql.NullInt64
		lastQuotaRefresh sql.NullTime
		modelsRaw        []byte
	)
	if err := scanner.Scan(
		&record.ID,
		&record.UserID,
		&record.AccountID,
		&record.GroupID,
		&subscriptionID,
		&proxyID,
		&record.AccountName,
		&record.Model,
		&modelsRaw,
		&record.SupportsMessages,
		&record.Status,
		&record.AccountEmail,
		&record.PlanType,
		&lastQuotaRefresh,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.UserName,
		&record.UserEmail,
		&record.GroupName,
		&record.ProxyName,
	); err != nil {
		return nil, err
	}
	if subscriptionID.Valid {
		record.SubscriptionID = &subscriptionID.Int64
	}
	if proxyID.Valid {
		record.ProxyID = &proxyID.Int64
	}
	if lastQuotaRefresh.Valid {
		record.LastQuotaRefreshedAt = &lastQuotaRefresh.Time
	}
	if len(modelsRaw) > 0 {
		if err := json.Unmarshal(modelsRaw, &record.Models); err != nil {
			return nil, fmt.Errorf("decode hosted models: %w", err)
		}
	}
	if len(record.Models) == 0 && strings.TrimSpace(record.Model) != "" {
		record.Models = []string{record.Model}
	}
	return &record, nil
}

func (r *hostedOpenAIAccountRepository) Create(ctx context.Context, record *service.HostedOpenAIAccountRecord) error {
	if record == nil {
		return errors.New("hosted account record is nil")
	}
	modelsJSON, err := json.Marshal(record.Models)
	if err != nil {
		return fmt.Errorf("encode hosted models: %w", err)
	}
	query := `
		INSERT INTO hosted_openai_accounts (
			user_id, account_id, group_id, subscription_id, proxy_id, account_name,
			model, models, supports_messages, status, account_email, plan_type, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW())
		RETURNING id, created_at, updated_at
	`
	err = r.db.QueryRowContext(
		ctx,
		query,
		record.UserID,
		record.AccountID,
		record.GroupID,
		record.SubscriptionID,
		record.ProxyID,
		record.AccountName,
		record.Model,
		modelsJSON,
		record.SupportsMessages,
		record.Status,
		record.AccountEmail,
		record.PlanType,
	).Scan(&record.ID, &record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create hosted account binding: %w", err)
	}
	return nil
}

func (r *hostedOpenAIAccountRepository) GetByID(ctx context.Context, id int64) (*service.HostedOpenAIAccountRecord, error) {
	row := r.db.QueryRowContext(ctx, hostedOpenAIAccountSelect+` WHERE h.id = $1`, id)
	record, err := scanHostedOpenAIAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrHostedOpenAIAccountNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get hosted account binding: %w", err)
	}
	return record, nil
}

func (r *hostedOpenAIAccountRepository) GetByIDForUser(ctx context.Context, id, userID int64) (*service.HostedOpenAIAccountRecord, error) {
	row := r.db.QueryRowContext(ctx, hostedOpenAIAccountSelect+` WHERE h.id = $1 AND h.user_id = $2`, id, userID)
	record, err := scanHostedOpenAIAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrHostedOpenAIAccountNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get hosted account binding: %w", err)
	}
	return record, nil
}

func (r *hostedOpenAIAccountRepository) ListByUser(ctx context.Context, userID int64) ([]service.HostedOpenAIAccountRecord, error) {
	return r.query(ctx, hostedOpenAIAccountSelect+`
		WHERE h.user_id = $1 AND h.status <> 'archived'
		ORDER BY h.created_at DESC`, userID)
}

func (r *hostedOpenAIAccountRepository) List(ctx context.Context, search, status string) ([]service.HostedOpenAIAccountRecord, error) {
	query := hostedOpenAIAccountSelect + ` WHERE 1 = 1`
	args := make([]any, 0, 2)
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(` AND h.status = $%d`, len(args))
	}
	if search = strings.TrimSpace(search); search != "" {
		args = append(args, "%"+search+"%")
		query += fmt.Sprintf(` AND (u.email ILIKE $%d OR u.username ILIKE $%d OR h.account_email ILIKE $%d OR g.name ILIKE $%d)`,
			len(args), len(args), len(args), len(args))
	}
	query += ` ORDER BY h.updated_at DESC, h.id DESC`
	return r.query(ctx, query, args...)
}

func (r *hostedOpenAIAccountRepository) query(ctx context.Context, query string, args ...any) ([]service.HostedOpenAIAccountRecord, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list hosted account bindings: %w", err)
	}
	defer rows.Close()

	result := make([]service.HostedOpenAIAccountRecord, 0)
	for rows.Next() {
		record, scanErr := scanHostedOpenAIAccount(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan hosted account binding: %w", scanErr)
		}
		result = append(result, *record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hosted account bindings: %w", err)
	}
	return result, nil
}

func (r *hostedOpenAIAccountRepository) UpdateConfig(
	ctx context.Context,
	id int64,
	proxyID *int64,
	accountName string,
	models []string,
	supportsMessages bool,
) error {
	modelsJSON, err := json.Marshal(models)
	if err != nil {
		return fmt.Errorf("encode hosted models: %w", err)
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE hosted_openai_accounts
		SET proxy_id = $2, account_name = $3, model = $4, models = $5,
		    supports_messages = $6, updated_at = NOW()
		WHERE id = $1
	`, id, proxyID, accountName, models[0], modelsJSON, supportsMessages)
	if err != nil {
		return fmt.Errorf("update hosted account config: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrHostedOpenAIAccountNotFound
	}
	return nil
}

func (r *hostedOpenAIAccountRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE hosted_openai_accounts
		SET status = $2, updated_at = NOW()
		WHERE id = $1
	`, id, status)
	if err != nil {
		return fmt.Errorf("update hosted account status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrHostedOpenAIAccountNotFound
	}
	return nil
}

func (r *hostedOpenAIAccountRepository) UpdateQuotaMeta(ctx context.Context, id int64, planType string, refreshedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE hosted_openai_accounts
		SET plan_type = $2, last_quota_refreshed_at = $3, updated_at = NOW()
		WHERE id = $1
	`, id, planType, refreshedAt)
	if err != nil {
		return fmt.Errorf("update hosted quota metadata: %w", err)
	}
	return nil
}

func (r *hostedOpenAIAccountRepository) UpdateIdentity(ctx context.Context, id int64, email, planType string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE hosted_openai_accounts
		SET account_email = $2, plan_type = $3, updated_at = NOW()
		WHERE id = $1
	`, id, email, planType)
	if err != nil {
		return fmt.Errorf("update hosted account identity: %w", err)
	}
	return nil
}

func (r *hostedOpenAIAccountRepository) DeleteSubscription(ctx context.Context, subscriptionID int64) error {
	if subscriptionID <= 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM user_subscriptions WHERE id = $1`, subscriptionID)
	return err
}
