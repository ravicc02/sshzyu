//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAPIKeyAuthSnapshotPreservesModelRateMultiplier 验证模型级计费倍率（隐式配置）
// 能随鉴权缓存快照往返存活——否则线上改了倍率也不生效（历史同类陷阱）。
func TestAPIKeyAuthSnapshotPreservesModelRateMultiplier(t *testing.T) {
	groupID := int64(51)
	rate := 1.35
	apiKey := &APIKey{
		ID: 83, UserID: 41, GroupID: &groupID, Key: "sk-rate-multiplier-roundtrip", Status: StatusActive,
		User: &User{ID: 41, Status: StatusActive},
		Group: &Group{
			ID: groupID, Name: "rate-multiplier-roundtrip", Platform: PlatformAnthropic, Status: StatusActive,
			ModelPricing: []ChannelModelPricing{
				{Models: []string{"claude-sonnet-4"}, RateMultiplier: &rate},
			},
		},
	}
	svc := &APIKeyService{}

	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: svc.snapshotFromAPIKey(context.Background(), apiKey)})
	require.NoError(t, err)
	var cached APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &cached))

	materialized, used, err := svc.applyAuthCacheEntry(apiKey.Key, &cached)
	require.NoError(t, err)
	require.True(t, used)
	require.NotNil(t, materialized.Group)
	require.Len(t, materialized.Group.ModelPricing, 1)
	require.NotNil(t, materialized.Group.ModelPricing[0].RateMultiplier, "倍率必须随快照往返存活")
	require.InDelta(t, rate, *materialized.Group.ModelPricing[0].RateMultiplier, 1e-12)
}

// TestResolve_OnlyRateMultiplierDoesNotShortCircuitPricing 验证「仅配模型级倍率、
// 未配任何价格」的分组条目不会短路价格解析——否则会跳过渠道/全局价格表导致错价。
func TestResolve_OnlyRateMultiplierDoesNotShortCircuitPricing(t *testing.T) {
	bs := newTestBillingServiceForResolver()
	r := NewModelPricingResolver(nil, bs)
	rate := 1.35
	const model = "claude-sonnet-4"

	t.Run("仅配倍率的条目应回落到全局价格表", func(t *testing.T) {
		group := &Group{
			ID: 60, Platform: PlatformAnthropic,
			ModelPricing: []ChannelModelPricing{{Models: []string{"*"}, RateMultiplier: &rate}},
		}
		withRateOnly := r.Resolve(context.Background(), PricingInput{Model: model, Group: group})
		baseline := r.Resolve(context.Background(), PricingInput{Model: model})
		require.NotEqual(t, PricingSourceGroup, withRateOnly.Source,
			"只配倍率的条目不得短路到分组定价")
		require.Equal(t, baseline.Source, withRateOnly.Source,
			"应与未配置该条目时走同一条价格解析链路")
		require.NotNil(t, withRateOnly.BasePricing, "应回落到全局价格表取得基础定价")
		require.InDelta(t, 3e-6, withRateOnly.BasePricing.InputPricePerToken, 1e-12)
	})

	t.Run("配了价格的条目仍按分组定价", func(t *testing.T) {
		inputPrice := 1e-6
		group := &Group{
			ID: 61, Platform: PlatformAnthropic,
			ModelPricing: []ChannelModelPricing{
				{Models: []string{"*"}, InputPrice: &inputPrice, RateMultiplier: &rate},
			},
		}
		resolved := r.Resolve(context.Background(), PricingInput{Model: model, Group: group})
		require.Equal(t, PricingSourceGroup, resolved.Source,
			"含价格的条目仍应短路到分组定价")
		require.NotNil(t, resolved.BasePricing)
		require.InDelta(t, inputPrice, resolved.BasePricing.InputPricePerToken, 1e-12)
	})
}