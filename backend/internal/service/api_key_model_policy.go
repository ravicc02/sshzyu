package service

import (
	"context"
	"strings"
	"unicode"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Model restrictions are an intersection: a key can never widen its group.
func (k *APIKey) HasModelRestrictions() bool {
	return k != nil && (len(k.AllowedModels) > 0 || (k.Group != nil && k.Group.ModelAllowlistEnabled()))
}

func (k *APIKey) AllowsModel(model string) bool {
	if k == nil {
		return false
	}
	if k.Group != nil && !k.Group.ModelAllowlist.Allows(model) {
		return false
	}
	if len(k.AllowedModels) == 0 {
		return true
	}
	return (GroupModelAllowlist{Enabled: true, Models: k.AllowedModels}).Allows(model)
}

func (k *APIKey) FilterModels(models []string) []string {
	if k == nil || !k.HasModelRestrictions() {
		return models
	}
	out := make([]string, 0, len(models))
	for _, model := range models {
		if k.AllowsModel(model) {
			out = append(out, model)
		}
	}
	return out
}

func normalizeAPIKeyAllowedModels(models []string) ([]string, error) {
	if len(models) > 200 {
		return nil, infraerrors.BadRequest("API_KEY_MODELS_INVALID", "select at most 200 models")
	}
	out := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" || len(model) > 256 || strings.ContainsAny(model, "*?") || strings.ContainsFunc(model, unicode.IsControl) {
			return nil, infraerrors.BadRequest("API_KEY_MODELS_INVALID", "model restrictions must contain explicit model IDs")
		}
		identity := strings.ToLower(model)
		if _, ok := seen[identity]; ok {
			continue
		}
		seen[identity] = struct{}{}
		out = append(out, model)
	}
	return out, nil
}

// BindableGroup shares the create/update authorization rules with model discovery.
func (s *APIKeyService) BindableGroup(ctx context.Context, userID, groupID int64) (*Group, error) {
	if groupID <= 0 {
		return nil, ErrGroupNotAllowed
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group.Status != StatusActive || !s.canUserBindGroup(ctx, user, group) {
		return nil, ErrGroupNotAllowed
	}
	return group, nil
}
