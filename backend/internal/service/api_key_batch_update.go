package service

import (
	"context"
	"html"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type UpdateAPIKeyBatchRequest struct {
	Name          string
	GroupID       int64
	Quota         float64
	QuotaTokens   int64
	AllowedModels []string
}

type apiKeyBatchUpdater interface {
	UpdateBatchByUserID(context.Context, int64, string, UpdateAPIKeyBatchRequest) ([]APIKey, error)
}

func (s *APIKeyService) UpdateBatch(ctx context.Context, userID int64, batchID string, req UpdateAPIKeyBatchRequest) ([]APIKey, error) {
	batchID = strings.TrimSpace(batchID)
	if batchID == "" || len(batchID) > 64 {
		return nil, infraerrors.BadRequest("BATCH_ID_INVALID", "invalid batch id")
	}
	if req.GroupID <= 0 {
		return nil, infraerrors.BadRequest("BATCH_GROUP_REQUIRED", "group is required")
	}
	if err := validateAPIKeyLimit(req.Quota); err != nil {
		return nil, err
	}
	if req.QuotaTokens < 0 || (req.Quota <= 0 && req.QuotaTokens <= 0) || (req.Quota > 0 && req.QuotaTokens > 0) {
		return nil, infraerrors.BadRequest("BATCH_QUOTA_INVALID", "choose one positive balance or token quota")
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, infraerrors.BadRequest("BATCH_NAME_REQUIRED", "batch name is required")
	}
	if _, err := s.BindableGroup(ctx, userID, req.GroupID); err != nil {
		return nil, err
	}
	models, err := normalizeAPIKeyAllowedModels(req.AllowedModels)
	if err != nil {
		return nil, err
	}
	req.AllowedModels = models
	req.Name = html.EscapeString(truncateBatchName(req.Name, MaxBatchAPIKeys))
	repo, ok := s.apiKeyRepo.(apiKeyBatchUpdater)
	if !ok {
		return nil, infraerrors.InternalServer("BATCH_UPDATE_UNAVAILABLE", "batch update is unavailable")
	}
	keys, err := repo.UpdateBatchByUserID(ctx, userID, batchID, req)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		s.InvalidateAuthCacheByKey(ctx, key.Key)
	}
	return s.ListBatchKeys(ctx, userID, batchID)
}
