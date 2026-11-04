package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Lock batch membership and update the requested settings in one transaction.
// Credentials, counters, expiration and unrelated per-key limits stay intact.
func (r *apiKeyRepository) UpdateBatchByUserID(ctx context.Context, userID int64, batchID string, req service.UpdateAPIKeyBatchRequest) ([]service.APIKey, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	keys, err := tx.APIKey.Query().Where(apikey.UserIDEQ(userID), apikey.BatchIDEQ(batchID), apikey.DeletedAtIsNil()).Order(dbent.Asc(apikey.FieldID)).ForUpdate().All(ctx)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, service.ErrAPIKeyNotFound
	}
	updated := make([]service.APIKey, 0, len(keys))
	now := time.Now()
	for i, key := range keys {
		builder := tx.APIKey.UpdateOneID(key.ID).SetBatchName(req.Name).SetGroupID(req.GroupID).SetQuota(req.Quota).SetQuotaTokens(req.QuotaTokens).SetAllowedModels(append([]string{}, req.AllowedModels...)).SetUpdatedAt(now)
		if key.BatchName != req.Name {
			builder.SetName(service.BatchAPIKeyDisplayName(req.Name, i+1, len(keys)))
		}
		if key.Status == service.StatusActive || key.Status == service.StatusAPIKeyQuotaExhausted {
			exhausted := (req.Quota > 0 && key.QuotaUsed >= req.Quota) || (req.QuotaTokens > 0 && key.QuotaTokensUsed >= req.QuotaTokens)
			if exhausted {
				builder.SetStatus(service.StatusAPIKeyQuotaExhausted)
			} else {
				builder.SetStatus(service.StatusActive)
			}
		}
		saved, err := builder.Save(ctx)
		if err != nil {
			return nil, err
		}
		updated = append(updated, *apiKeyEntityToService(saved))
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return updated, nil
}
