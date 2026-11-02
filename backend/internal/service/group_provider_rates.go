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

// GroupProviderRates derives provider-wide channel rates from a composite
// group. Exact model entries are intentionally excluded: they are model-level
// overrides and must not be presented as the channel's rate.
func GroupProviderRates(group *Group) []GroupProviderRate {
	return GroupChannelRates(group)
}

// GroupChannelRates returns only channel-wide overrides. The provider marker is
// authoritative; a model wildcard such as deepseek-* is accepted as a legacy
// channel-wide entry. Exact model overrides are excluded.
func GroupChannelRates(group *Group) []GroupProviderRate {
	if group == nil || group.Platform != PlatformComposite {
		return nil
	}
	providerEntries := make(map[string]float64)
	wildcard := make(map[string]float64)
	for i := range group.ModelPricing {
		entry := &group.ModelPricing[i]
		for _, pattern := range entry.Models {
			if provider, ok := providerFromGroupRateEntry(pattern); ok {
				if entry.RateMultiplier != nil && *entry.RateMultiplier > 0 {
					providerEntries[provider] = *entry.RateMultiplier
				}
				continue
			}
			provider := modelProviderPlatform(pattern)
			if provider == "" || !strings.HasSuffix(strings.TrimSpace(pattern), "*") {
				continue
			}
			if entry.RateMultiplier == nil || *entry.RateMultiplier <= 0 {
				continue
			}
			wildcard[provider] = *entry.RateMultiplier
		}
	}
	out := make([]GroupProviderRate, 0, len(providerEntries)+len(wildcard))
	for provider, rate := range providerEntries {
		out = append(out, GroupProviderRate{Platform: provider, RateMultiplier: rate})
		delete(wildcard, provider)
	}
	for provider, rate := range wildcard {
		out = append(out, GroupProviderRate{Platform: provider, RateMultiplier: rate})
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

// GroupModelRateOverride returns a model-specific rate override only. Channel
// rates and wildcard provider entries are deliberately ignored so the admin UI
// can distinguish "channel rate" from "model rate".
func GroupModelRateOverride(group *Group, model string) *float64 {
	if group == nil {
		return nil
	}
	model = normalizeChannelPricingModelName(model)
	for i := range group.ModelPricing {
		entry := &group.ModelPricing[i]
		if entry.RateMultiplier == nil || *entry.RateMultiplier <= 0 {
			continue
		}
		for _, pattern := range entry.Models {
			if _, channelEntry := providerFromGroupRateEntry(pattern); channelEntry {
				continue
			}
			normalized := normalizeChannelPricingModelName(pattern)
			if strings.HasSuffix(normalized, "*") {
				continue
			}
			if normalized == model {
				rate := *entry.RateMultiplier
				return &rate
			}
		}
	}
	return nil
}
