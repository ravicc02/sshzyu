package service

import (
	"sort"
	"strings"
)

const groupProviderRateModelPrefix = "__provider_rate__:"

var groupProviderSortOrder = map[string]int{
	PlatformDeepseek: 10,
	PlatformZhipu:    20,
	PlatformKimi:     30,
	PlatformMiniMax:  40,
}

func modelProviderPlatform(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(model, "deepseek-"):
		return PlatformDeepseek
	case strings.HasPrefix(model, "glm-"):
		return PlatformZhipu
	case strings.HasPrefix(model, "kimi-"):
		return PlatformKimi
	case strings.HasPrefix(model, "minimax-"), strings.HasPrefix(model, "abab"):
		return PlatformMiniMax
	default:
		return ""
	}
}

func groupProviderModelPattern(platform string) string {
	if _, ok := groupProviderSortOrder[platform]; !ok {
		return ""
	}
	return groupProviderRateModelPrefix + platform
}

func providerFromGroupRateEntry(model string) (string, bool) {
	model = strings.TrimSpace(model)
	if !strings.HasPrefix(model, groupProviderRateModelPrefix) {
		return "", false
	}
	provider := strings.TrimPrefix(model, groupProviderRateModelPrefix)
	_, ok := groupProviderSortOrder[provider]
	return provider, ok
}

// GroupProviderRates derives provider-wide rates from a composite group's
// model pricing entries. Wildcard/provider entries take precedence over exact
// legacy entries for the same provider.
func GroupProviderRates(group *Group) []GroupProviderRate {
	if group == nil || group.Platform != PlatformComposite {
		return nil
	}
	exact := make(map[string]float64)
	providerEntries := make(map[string]float64)
	wildcard := make(map[string]float64)
	providersSeen := make(map[string]struct{})
	for i := range group.ModelPricing {
		entry := &group.ModelPricing[i]
		for _, pattern := range entry.Models {
			if provider, ok := providerFromGroupRateEntry(pattern); ok {
				providersSeen[provider] = struct{}{}
				if entry.RateMultiplier != nil && *entry.RateMultiplier > 0 {
					providerEntries[provider] = *entry.RateMultiplier
				}
				continue
			}
			provider := modelProviderPlatform(pattern)
			if provider == "" {
				continue
			}
			providersSeen[provider] = struct{}{}
			if entry.RateMultiplier == nil || *entry.RateMultiplier <= 0 {
				continue
			}
			if strings.HasSuffix(strings.TrimSpace(pattern), "*") {
				wildcard[provider] = *entry.RateMultiplier
			} else if _, exists := exact[provider]; !exists {
				exact[provider] = *entry.RateMultiplier
			}
		}
	}
	out := make([]GroupProviderRate, 0, len(wildcard)+len(exact))
	for provider, rate := range providerEntries {
		out = append(out, GroupProviderRate{Platform: provider, RateMultiplier: rate})
		delete(wildcard, provider)
		delete(exact, provider)
	}
	for provider, rate := range wildcard {
		out = append(out, GroupProviderRate{Platform: provider, RateMultiplier: rate})
		delete(exact, provider)
	}
	for provider, rate := range exact {
		out = append(out, GroupProviderRate{Platform: provider, RateMultiplier: rate})
	}
	seenOut := make(map[string]struct{}, len(out))
	for _, item := range out {
		seenOut[item.Platform] = struct{}{}
	}
	for provider := range providersSeen {
		if _, exists := seenOut[provider]; exists {
			continue
		}
		out = append(out, GroupProviderRate{Platform: provider, RateMultiplier: group.RateMultiplier})
	}
	sort.SliceStable(out, func(i, j int) bool {
		oi, ij := groupProviderSortOrder[out[i].Platform], groupProviderSortOrder[out[j].Platform]
		if oi != ij {
			return oi < ij
		}
		return out[i].Platform < out[j].Platform
	})
	return out
}
