//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Cache preflight must reserve the amount that the customized gateway actually bills.
func TestGeminiCachedContentQuotePreservesModelOverrideAndDisplayMultiplier(t *testing.T) {
	for _, peak := range []bool{false, true} {
		t.Run(map[bool]string{false: "off peak", true: "peak"}[peak], func(t *testing.T) {
			svc, usageRepo := newGeminiCacheBillingGatewayForTest(t)
			groupID := int64(33)
			override := 0.25
			key := &APIKey{ID: 821, GroupID: &groupID, User: &User{ID: 621}, Group: &Group{
				ID: groupID, Platform: PlatformGemini, RateMultiplier: 2, Hydrated: true, SubscriptionType: SubscriptionTypeSubscription,
				PeakRateEnabled: peak, PeakStart: "00:00", PeakEnd: "23:59", PeakRateMultiplier: 3,
				ModelPricing: []ChannelModelPricing{{Models: []string{"gemini-3.8-flash"}, RateMultiplier: &override}},
			}}
			input := &RecordUsageInput{Result: &ForwardResult{RequestID: "cache-custom-rate", Model: "gemini-3.8-flash", Usage: ClaudeUsage{InputTokens: 1000}, CacheStorageTokenHours: 24000}, APIKey: key, User: key.User, Account: &Account{ID: 713, Platform: PlatformGemini}, PricingAt: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)}
			quote := svc.QuoteUsageCost(context.Background(), input)
			factor := 1.0
			if peak {
				factor = 3
			}
			require.InDelta(t, (1000*0.00000075+24000*0.0000005)*override*factor, quote, 1e-12)
			require.NoError(t, svc.RecordUsage(context.Background(), input))
			require.InDelta(t, quote, usageRepo.lastLog.ActualCost, 1e-12)
			require.InDelta(t, override*factor, usageRepo.lastLog.RateMultiplier, 1e-12)
			require.NotNil(t, usageRepo.lastLog.RateMultiplierWithoutModel)
			require.InDelta(t, 2*factor, *usageRepo.lastLog.RateMultiplierWithoutModel, 1e-12)
		})
	}
}
