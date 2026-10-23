//go:build unit

package service

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBatchImageModelTierPricing_ListSubmitSettle(t *testing.T) {
	price := func(v float64) *float64 { return &v }
	for _, tc := range []struct {
		model, size, aspect string
		want                float64
	}{
		{"gpt-image-2", "1K", "1:1", .15},
		{"gpt-image-2", "2K", "1:1", .15},
		{"gpt-image-2.5-flare", "1K", "1:1", .17},
		{"gpt-image-2.5-flare", "2K", "1:1", .19},
		{"gpt-image-2.5-sunburst", "1024x1024", "", .21},
		{"gpt-image-2.5-sunburst", "1536x1024", "", .23},
		{"gpt-image-2.5-sunburst", "3840x2160", "", .25},
	} {
		t.Run(tc.model+"/"+tc.size, func(t *testing.T) {
			ctx := context.Background()
			svc, repo, _, _, _ := newTestBatchImagePublicService(true)
			group := &Group{ID: 18, Platform: PlatformOpenAI, AllowImageGeneration: true, AllowBatchImageGeneration: true, RateMultiplier: 1, BatchImageHoldMultiplier: 1}
			group.ModelPricing = []ChannelModelPricing{
				{Models: []string{"gpt-image-2"}, BillingMode: BillingModeImage, Intervals: []PricingInterval{{TierLabel: "1K", PerRequestPrice: price(.15)}, {TierLabel: "2K", PerRequestPrice: price(.15)}, {TierLabel: "4K", PerRequestPrice: price(.15)}}},
				{Models: []string{"gpt-image-2.5-flare"}, BillingMode: BillingModeImage, Intervals: []PricingInterval{{TierLabel: "1K", PerRequestPrice: price(.17)}, {TierLabel: "2K", PerRequestPrice: price(.19)}, {TierLabel: "4K", PerRequestPrice: price(.19)}}},
				{Models: []string{"gpt-image-2.5-sunburst"}, BillingMode: BillingModeImage, Intervals: []PricingInterval{
					{TierLabel: "1K", PerRequestPrice: price(.21)}, {TierLabel: "2K", PerRequestPrice: price(.23)}, {TierLabel: "4K", PerRequestPrice: price(.25)},
				}},
			}
			svc.GroupRepo = &publicBatchImageGroupRepo{groups: map[int64]*Group{18: group}}
			provider := &publicBatchImageProvider{name: BatchImageProviderOpenAI}
			svc.ProviderRegistry = NewBatchImageProviderRegistry(provider)
			account := testBatchImageMappedAccount(290, AccountTypeAPIKey, map[string]any{
				"gpt-image-2": "gpt-image-2", "gpt-image-2.5-flare": "gpt-image-2.5-flare", "gpt-image-2.5-sunburst": "gpt-image-2.5-sunburst", "unpriced": "unpriced",
			})
			account.Platform = PlatformOpenAI
			svc.AccountRepo = &publicBatchImageAccountRepo{accounts: []Account{account}}
			owner := BatchImageOwner{UserID: 11, APIKeyID: 22, GroupID: &group.ID}
			models, err := svc.ListModels(ctx, owner)
			require.NoError(t, err)
			require.Len(t, models.Data, 3)
			for _, model := range models.Data {
				require.Equal(t, []string{"1K", "2K", "4K"}, model.SupportedImageSizes)
			}
			req := validBatchImageSubmitRequest()
			req.Model, req.Provider, req.ImageSize, req.AspectRatio = tc.model, BatchImageProviderOpenAI, tc.size, tc.aspect
			batch, err := svc.Submit(ctx, owner, req, "")
			require.NoError(t, err)
			require.InDelta(t, 2*tc.want, batch.EstimatedCost, 1e-12)
			require.InDelta(t, 2*tc.want, batch.HoldAmount, 1e-12)
			job := repo.jobs[batch.ID]
			require.InDelta(t, tc.want, job.BaseUnitPrice, 1e-12)
			require.InDelta(t, tc.want, job.BillableUnitPrice, 1e-12)
			require.Equal(t, 1, job.PricingSnapshotVersion)
			// Later edits to either kind of price must not reprice an existing job.
			group.ModelPricing = nil
			group.ImagePrice1K, group.ImagePrice2K, group.ImagePrice4K = price(9), price(9), price(9)
			job.Status, job.SuccessCount, job.FailCount = BatchImageJobStatusSettling, 1, 1
			settlement := &BatchImageSettlementService{Repo: repo, BillingRepo: svc.BillingRepo, Pricing: svc.Pricing}
			result, err := settlement.Settle(ctx, job.BatchID)
			require.NoError(t, err)
			require.InDelta(t, tc.want, result.ActualCost, 1e-12)
			billing := svc.BillingRepo.(*fakeBatchImageBillingRepo)
			require.Len(t, billing.captures, 1)
			require.InDelta(t, tc.want, billing.captures[0].ActualAmount, 1e-12)
		})
	}
}

func TestBatchImageModelTierPricing_Compatibility(t *testing.T) {
	price := func(v float64) *float64 { return &v }
	for _, tc := range []struct {
		name   string
		card   ChannelModelPricing
		legacy *float64
		want   *float64
	}{
		{name: "legacy price", legacy: price(.12), want: price(.12)},
		{name: "model tier overrides default and legacy", card: ChannelModelPricing{BillingMode: BillingModeImage, PerRequestPrice: price(.17), Intervals: []PricingInterval{{TierLabel: "2K", PerRequestPrice: price(.23)}}}, legacy: price(.12), want: price(.23)},
		{name: "model default falls back to legacy", card: ChannelModelPricing{BillingMode: BillingModeImage, PerRequestPrice: price(.17)}, legacy: price(.12), want: price(.12)},
		{name: "missing tier uses legacy", card: ChannelModelPricing{BillingMode: BillingModeImage, Intervals: []PricingInterval{{TierLabel: "1K", PerRequestPrice: price(.17)}}}, legacy: price(.12), want: price(.12)},
		{name: "only matching tier is offered", card: ChannelModelPricing{BillingMode: BillingModeImage, Intervals: []PricingInterval{{TierLabel: "1K", PerRequestPrice: price(.17)}}}},
		{name: "unpriced model stays unavailable"},
		{name: "zero tier is explicit", card: ChannelModelPricing{BillingMode: BillingModeImage, PerRequestPrice: price(.17), Intervals: []PricingInterval{{TierLabel: "2K", PerRequestPrice: price(0)}}}, legacy: price(.12), want: price(0)},
		{name: "zero default without tier is unavailable", card: ChannelModelPricing{BillingMode: BillingModeImage, PerRequestPrice: price(0)}},
		{name: "request mode without tier falls back", card: ChannelModelPricing{BillingMode: BillingModePerRequest, PerRequestPrice: price(.17)}, legacy: price(.12), want: price(.12)},
		{name: "token image price is not per image", card: ChannelModelPricing{BillingMode: BillingModeToken, ImageOutputPrice: price(.00000017)}},
		{name: "negative price rejected", card: ChannelModelPricing{BillingMode: BillingModeImage, PerRequestPrice: price(-1)}},
		{name: "NaN rejected", card: ChannelModelPricing{BillingMode: BillingModeImage, PerRequestPrice: price(math.NaN())}},
		{name: "infinite price rejected", card: ChannelModelPricing{BillingMode: BillingModeImage, PerRequestPrice: price(math.Inf(1))}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _, _, _ := newTestBatchImagePublicService(true)
			group := &Group{ID: 18, Platform: PlatformOpenAI, AllowImageGeneration: true, AllowBatchImageGeneration: true, RateMultiplier: 1, ImagePrice2K: tc.legacy, BatchImageHoldMultiplier: 1}
			tc.card.Models = []string{"gpt-image-2"}
			group.ModelPricing = []ChannelModelPricing{tc.card}
			svc.GroupRepo = &publicBatchImageGroupRepo{groups: map[int64]*Group{18: group}}
			svc.ProviderRegistry = NewBatchImageProviderRegistry(&publicBatchImageProvider{name: BatchImageProviderOpenAI})
			owner := BatchImageOwner{UserID: 11, APIKeyID: 22, GroupID: &group.ID}
			models, err := svc.ListModels(context.Background(), owner)
			require.NoError(t, err)
			listed := false
			for _, model := range models.Data {
				if model.ID == "gpt-image-2" {
					for _, size := range model.SupportedImageSizes {
						listed = listed || size == "2K"
					}
				}
			}
			require.Equal(t, tc.want != nil, listed)
			req := validBatchImageSubmitRequest()
			req.Model, req.Provider, req.ImageSize = "gpt-image-2", BatchImageProviderOpenAI, "2K"
			batch, err := svc.Submit(context.Background(), owner, req, "")
			if tc.want == nil {
				require.ErrorIs(t, err, ErrBatchImageSettlementPricingMissing)
				return
			}
			require.NoError(t, err)
			require.InDelta(t, *tc.want*2, batch.EstimatedCost, 1e-12)
		})
	}
}
