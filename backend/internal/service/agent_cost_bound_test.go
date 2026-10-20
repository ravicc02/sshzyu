//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func agentPricingFixture(t *testing.T, source string, mapping map[string]string, pricing []ChannelModelPricing) (*OpenAIGatewayService, *APIKey) {
	t.Helper()
	gid := int64(100)
	repo := &mockChannelRepository{
		listAllFn: func(context.Context) ([]Channel, error) {
			return []Channel{{ID: 1, Status: StatusActive, GroupIDs: []int64{gid}, BillingModelSource: source, ModelMapping: map[string]map[string]string{PlatformOpenAI: mapping}, ModelPricing: pricing}}, nil
		},
		getGroupPlatformsFn: func(context.Context, []int64) (map[int64]string, error) {
			return map[int64]string{gid: PlatformOpenAI}, nil
		},
	}
	// Synthetic names use explicit channel cards rather than the production
	// fallback resolver's known model-family matching.
	pricing = append(pricing,
		ChannelModelPricing{Platform: PlatformOpenAI, Models: []string{"text-model"}, BillingMode: BillingModeToken, InputPrice: testPtrFloat64(3e-6), OutputPrice: testPtrFloat64(15e-6)},
		ChannelModelPricing{Platform: PlatformOpenAI, Models: []string{"expensive-model"}, BillingMode: BillingModeToken, InputPrice: testPtrFloat64(30e-6), OutputPrice: testPtrFloat64(150e-6)},
	)
	cs := NewChannelService(repo, nil, nil, nil, nil)
	bs := newTestBillingServiceForResolver()
	bs.fallbackPrices["text-model"] = &ModelPricing{InputPricePerToken: 3e-6, OutputPricePerToken: 15e-6}
	bs.fallbackPrices["expensive-model"] = &ModelPricing{InputPricePerToken: 30e-6, OutputPricePerToken: 150e-6}
	resolver := NewModelPricingResolver(cs, bs)
	s := &OpenAIGatewayService{channelService: cs, resolver: resolver, billingService: bs, cfg: &config.Config{}}
	key := &APIKey{ID: 6, UserID: 1, User: &User{ID: 1}, GroupID: &gid, Group: &Group{ID: gid, Platform: PlatformOpenAI, RateMultiplier: 1}}
	return s, key
}
func TestAgentCostBoundSupportsConfiguredBillingSources(t *testing.T) {
	for _, source := range []string{BillingModelSourceUpstream, BillingModelSourceRequested, BillingModelSourceChannelMapped} {
		t.Run(source, func(t *testing.T) {
			s, key := agentPricingFixture(t, source, nil, nil)
			quote, err := s.AgentCostBound(context.Background(), key, "text-model", "text", 1000)
			require.NoError(t, err)
			amount, err := decimal.NewFromString(quote)
			require.NoError(t, err)
			require.True(t, amount.IsPositive())
		})
	}
}
func TestAgentCostBoundIncludesDeterministicChannelAlias(t *testing.T) {
	s, key := agentPricingFixture(t, BillingModelSourceChannelMapped, map[string]string{"public-model": "expensive-model"}, nil)
	quote, err := s.AgentCostBound(context.Background(), key, "public-model", "text", 1000)
	require.NoError(t, err)
	amount, _ := decimal.NewFromString(quote)
	require.True(t, amount.GreaterThan(decimal.NewFromFloat(.3)), quote)
}
func TestAgentCostBoundStillRejectsResponseDeclaredPricing(t *testing.T) {
	s, key := agentPricingFixture(t, BillingModelSourceResponse, nil, nil)
	_, err := s.AgentCostBound(context.Background(), key, "text-model", "text", 1000)
	require.EqualError(t, err, "AGENT_UNBOUNDED_PRICING")
}
func TestAgentCostBoundFixedImageUnderUpstreamBilling(t *testing.T) {
	price := .17
	s, key := agentPricingFixture(t, BillingModelSourceUpstream, nil, []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"gpt-image-2"}, BillingMode: BillingModeImage, PerRequestPrice: &price}})
	quote, err := s.AgentCostBound(context.Background(), key, "gpt-image-2", "image", 1000)
	require.NoError(t, err)
	require.Equal(t, "0.18700000", quote)
}

func TestAgentCostBoundIncludesPricierAccountMapping(t *testing.T) {
	s, key := agentPricingFixture(t, BillingModelSourceUpstream, nil, nil)
	baseline, err := s.AgentCostBound(context.Background(), key, "text-model", "text", 1000)
	require.NoError(t, err)
	deps := s.inflightEstimateDeps()
	deps.accountMappedModels = func(context.Context, *APIKey, string) []string { return []string{"expensive-model"} }
	quote, err := s.agentCostBoundWithDeps(context.Background(), key, "text-model", "text", 1000, deps)
	require.NoError(t, err)
	a, _ := decimal.NewFromString(quote)
	b, _ := decimal.NewFromString(baseline)
	require.True(t, a.GreaterThan(b.Mul(decimal.NewFromInt(5))))
}
func TestAgentCostBoundRetainsNoPriceAndTokenImageRejections(t *testing.T) {
	s, key := agentPricingFixture(t, BillingModelSourceUpstream, nil, nil)
	_, err := s.AgentCostBound(context.Background(), key, "unknown-image", "image", 1000)
	require.EqualError(t, err, "AGENT_IMAGE_FIXED_PRICE_REQUIRED")
	_, err = s.AgentCostBound(context.Background(), key, "unknown-text", "text", 1000)
	require.EqualError(t, err, "AGENT_PRICING_UNAVAILABLE")
}

func TestAgentCostBoundExistingGPT61And124KGroupConfiguration(t *testing.T) {
	s, key := agentPricingFixture(t, BillingModelSourceUpstream, nil, nil)
	bs := NewBillingService(&config.Config{}, nil)
	s.billingService = bs
	s.resolver = NewModelPricingResolver(s.channelService, bs)
	key.Group.RateMultiplier = .25
	text, err := s.AgentCostBound(context.Background(), key, "gpt-6.1-sol", "text", 5000)
	require.NoError(t, err)
	v, _ := decimal.NewFromString(text)
	require.True(t, v.IsPositive())
	require.True(t, v.LessThan(decimal.NewFromInt(1)), text)
	key.Group.RateMultiplier = 1
	price := .13
	key.Group.ImagePrice1K = &price
	key.Group.ImagePrice2K = &price
	key.Group.ImagePrice4K = &price
	image, err := s.AgentCostBound(context.Background(), key, "gpt-image-2", "image", 500)
	require.NoError(t, err)
	require.Equal(t, "0.14300000", image)
}
func TestAgentSelectedAccountPriceRecheck(t *testing.T) {
	s, key := agentPricingFixture(t, BillingModelSourceUpstream, nil, nil)
	quote, err := s.AgentCostBound(context.Background(), key, "text-model", "text", 1000)
	require.NoError(t, err)
	ctx := WithAgentInvocation(context.Background(), &AgentInvocationTrace{CostLimit: quote})
	cheap := &Account{Credentials: map[string]any{"model_mapping": map[string]any{"text-model": "text-model"}}}
	require.NoError(t, s.CheckAgentSelectedAccountPrice(ctx, key, cheap, "text-model", "text-model", "text", 1000))
	costly := &Account{Credentials: map[string]any{"model_mapping": map[string]any{"text-model": "expensive-model"}}}
	require.EqualError(t, s.CheckAgentSelectedAccountPrice(ctx, key, costly, "text-model", "text-model", "text", 1000), "AGENT_PRICING_CHANGED")
}

func TestAgentAllPlatformsUseGenericTariffAndSelectedAccountRecheck(t *testing.T) {
	base, key := agentPricingFixture(t, BillingModelSourceRequested, nil, nil)
	generic := &GatewayService{cfg: base.cfg, resolver: base.resolver, billingService: base.billingService, channelService: base.channelService}
	for _, platform := range append(domain.ConcretePlatformIDs(), PlatformComposite) {
		t.Run(platform, func(t *testing.T) {
			key.Group.Platform = platform
			amount, err := generic.AgentCostBound(context.Background(), key, "text-model", "text", 1200)
			require.NoError(t, err)
			require.NotEqual(t, "0.00000000", amount)
			ctx := WithAgentInvocation(context.Background(), &AgentInvocationTrace{CostLimit: amount})
			cheap := &Account{Platform: platform, Credentials: map[string]any{"model_mapping": map[string]any{"text-model": "text-model"}}}
			require.NoError(t, generic.CheckAgentSelectedAccountPrice(ctx, key, cheap, "text-model", "text-model", "text", 1200))
		})
	}
}

func TestAgentVisionSelectedAccountCapabilityAndQuote(t *testing.T) {
	s, key := agentPricingFixture(t, BillingModelSourceRequested, nil, nil)
	quote, err := s.AgentCostBound(context.Background(), key, "text-model", "vision", 70000)
	require.NoError(t, err)
	trace := &AgentInvocationTrace{CostLimit: quote, InputImageCount: 1, VisionEncodedBytes: 2000000 - (70000 - AgentVisionInputReserve)}
	ctx := WithAgentInvocation(context.Background(), trace)
	account := &Account{}
	// Unknown metadata must not reject otherwise valid native image providers.
	require.NoError(t, s.CheckAgentSelectedAccountPrice(ctx, key, account, "text-model", "text-model", "text", 2000000))
	require.NoError(t, json.Unmarshal([]byte(`{"upstream_model_metadata":{"source":"upstream","models":{"text-model":{"id":"text-model","input_modalities":["text"]}}}}`), &account.Extra))
	require.EqualError(t, s.CheckAgentSelectedAccountPrice(ctx, key, account, "text-model", "text-model", "text", 2000000), "AGENT_MODEL_VISION_UNSUPPORTED")
	require.NoError(t, json.Unmarshal([]byte(`{"upstream_model_metadata":{"source":"upstream","models":{"text-model":{"id":"text-model","input_modalities":["text","image"]}}}}`), &account.Extra))
	require.NoError(t, s.CheckAgentSelectedAccountPrice(ctx, key, account, "text-model", "text-model", "text", 2000000))
}

func TestAgentVisionCostBoundIncludesImageInputTariff(t *testing.T) {
	s, key := agentPricingFixture(t, BillingModelSourceRequested, nil, []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"vision-tariff"}, BillingMode: BillingModeToken, InputPrice: testPtrFloat64(1e-6), OutputPrice: testPtrFloat64(1e-6), ImageInputPrice: testPtrFloat64(50e-6)}})
	text, err := s.AgentCostBound(context.Background(), key, "vision-tariff", "text", 70000)
	require.NoError(t, err)
	vision, err := s.AgentCostBound(context.Background(), key, "vision-tariff", "vision", 70000)
	require.NoError(t, err)
	textCost, err := decimal.NewFromString(text)
	require.NoError(t, err)
	imageCost, err := decimal.NewFromString(vision)
	require.NoError(t, err)
	require.True(t, imageCost.GreaterThan(textCost.Mul(decimal.NewFromInt(10))), "image-specific input tariff must be reserved")
}
