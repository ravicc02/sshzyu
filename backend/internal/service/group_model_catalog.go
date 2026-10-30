package service

import (
	"context"
	"sort"
	"strings"
)

type groupModelCatalogKey struct {
	platform string
	name     string
}

// groupSupportedModels builds the public model catalog for one group from the
// current account-group assignments. This mirrors /v1/models: account mapping
// keys are the public model IDs, channel mapping supplies pricing and an
// optional cross-model target, and the group allowlist is the final filter.
func groupSupportedModels(
	ctx context.Context,
	accountRepo AccountRepository,
	routeRepo CompositeModelRouteRepository,
	pricingService *PricingService,
	channel *Channel,
	group *Group,
) ([]SupportedModel, error) {
	if channel == nil || group == nil {
		return nil, nil
	}

	channelModels := channel.SupportedModels()
	byKey := make(map[groupModelCatalogKey]SupportedModel, len(channelModels))
	for _, model := range channelModels {
		byKey[groupModelCatalogKey{platform: model.Platform, name: model.Name}] = model
	}

	accounts := []Account{}
	if accountRepo != nil && group.ID > 0 {
		var err error
		accounts, err = accountRepo.ListSchedulableByGroupID(ctx, group.ID)
		if err != nil {
			return nil, err
		}
	}

	models := make(map[groupModelCatalogKey]SupportedModel)
	add := func(platform, name, accountTarget string) {
		platform = strings.TrimSpace(platform)
		name = strings.TrimSpace(name)
		if platform == "" || name == "" || !groupModelAllowed(group, name) {
			return
		}
		key := groupModelCatalogKey{platform: platform, name: name}
		current, exists := models[key]
		if !exists {
			current = SupportedModel{Name: name, Platform: platform}
			if channelModel, ok := byKey[key]; ok {
				current.Pricing = channelModel.Pricing
				current.RouteTarget = channelModel.RouteTarget
			}
		}
		if current.RouteTarget == "" {
			target := strings.TrimSpace(accountTarget)
			if target != "" && !strings.EqualFold(target, name) {
				if _, wildcard := splitWildcardSuffix(target); !wildcard {
					current.RouteTarget = target
				}
			}
		}
		if current.Pricing == nil {
			if channelModel, ok := byKey[key]; ok {
				current.Pricing = channelModel.Pricing
			}
		}
		models[key] = current
	}

	hasAnyMapping := false
	for i := range accounts {
		account := &accounts[i]
		if group.Platform == PlatformComposite {
			if !isConcreteRequestPlatform(account.Platform) {
				continue
			}
		} else if account.Platform != group.Platform && !mixedListingAccountAllowed(group.Platform, account) {
			continue
		}
		if group.Platform == PlatformOpenAI && account.IsOpenAIPassthroughEnabled() {
			continue
		}

		platform := group.Platform
		if group.Platform == PlatformComposite {
			platform = account.Platform
		}
		for name, target := range account.GetModelMapping() {
			if account.Platform != platform && !mixedListingModelAllowed(platform, name) {
				continue
			}
			add(platform, name, target)
			hasAnyMapping = true
		}
	}

	// No account mapping is authoritative as a fallback to the channel catalog
	// for that platform. This preserves legacy groups while allowing account
	// mapping changes to take over immediately once configured.
	if !hasAnyMapping {
		for _, model := range channelModels {
			if group.Platform == PlatformComposite {
				if !isConcreteRequestPlatform(model.Platform) {
					continue
				}
			} else if model.Platform != group.Platform {
				continue
			}
			add(model.Platform, model.Name, model.RouteTarget)
		}
	}

	if group.Platform == PlatformOpenAI && len(accounts) > 0 {
		names := make([]string, 0, len(models))
		for key := range models {
			if key.platform == PlatformOpenAI {
				names = append(names, key.name)
			}
		}
		sort.Strings(names)
		for _, name := range supplementUnmappedOpenAIModels(accounts, names) {
			add(PlatformOpenAI, name, "")
		}
	}

	if group.Platform == PlatformComposite && routeRepo != nil {
		routes, err := routeRepo.ListByGroup(ctx, group.ID, false)
		if err != nil {
			return nil, err
		}
		for _, route := range routes {
			if !isConcreteRequestPlatform(route.TargetPlatform) {
				continue
			}
			add(route.TargetPlatform, route.PublicModel, route.UpstreamModel)
		}
	}

	out := make([]SupportedModel, 0, len(models))
	for _, model := range models {
		out = append(out, model)
	}
	if group.Platform == PlatformOpenAI {
		names := make([]string, 0, len(out))
		for i := range out {
			names = append(names, out[i].Name)
		}
		allowed := make(map[string]struct{}, len(names))
		for _, name := range filterRetiredOpenAIModelIDs(accounts, names) {
			allowed[name] = struct{}{}
		}
		filtered := out[:0]
		for _, model := range out {
			if _, ok := allowed[model.Name]; ok {
				filtered = append(filtered, model)
			}
		}
		out = filtered
	}
	fillGlobalPricingFallback(pricingService, out)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Platform != out[j].Platform {
			return out[i].Platform < out[j].Platform
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func groupModelAllowed(group *Group, model string) bool {
	return group == nil || !group.ModelAllowlistEnabled() || group.ModelAllowlist.Allows(model)
}
