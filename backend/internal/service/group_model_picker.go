package service

import (
	"context"
	"sort"
	"strings"
)

// GetConfiguredGroupModels returns declared public IDs without the runtime's
// compatibility defaults. An empty group catalog stays empty in the editor.
func (s *GatewayService) GetConfiguredGroupModels(ctx context.Context, group *Group) ([]string, error) {
	if group == nil || group.ID <= 0 {
		return nil, ErrGroupNotAllowed
	}
	models := make(map[string]struct{})
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" || strings.ContainsAny(model, "*?") || !group.ModelAllowlist.Allows(model) {
			return
		}
		models[model] = struct{}{}
	}
	if s.accountRepo != nil {
		accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		for i := range accounts {
			account := &accounts[i]
			if group.Platform == PlatformComposite && !isConcreteRequestPlatform(account.Platform) {
				continue
			}
			if group.Platform != PlatformComposite && account.Platform != group.Platform && !mixedListingAccountAllowed(group.Platform, account) {
				continue
			}
			// Read only the saved mapping: GetModelMapping can inject runtime defaults.
			for model := range stringMappingFromRaw(account.Credentials["model_mapping"]) {
				if group.Platform != PlatformComposite && account.Platform != group.Platform && !mixedListingModelAllowed(group.Platform, model) {
					continue
				}
				add(model)
			}
		}
	}
	if group.Platform == PlatformComposite {
		routes, err := s.GetCompositeRouteModels(ctx, &group.ID, "", true)
		if err != nil {
			return nil, err
		}
		for _, model := range routes {
			add(model)
		}
	}
	if group.ModelAllowlistEnabled() {
		for _, model := range group.ModelAllowlist.Models {
			add(model)
		}
	}
	result := make([]string, 0, len(models))
	for model := range models {
		result = append(result, model)
	}
	sort.Strings(result)
	return result, nil
}
