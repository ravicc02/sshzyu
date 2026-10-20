package service

import (
	"context"
	"errors"
	"math"

	"github.com/shopspring/decimal"
)

// AgentCostBound accepts only pricing that can be bounded before dispatch.
// It never substitutes an image token rate for a per-image price. The input
// token ceiling uses one token per UTF-8 byte, including tool definitions.
func (s *OpenAIGatewayService) AgentCostBound(ctx context.Context, key *APIKey, model, kind string, inputBytes int) (string, error) {
	if key == nil || key.Group == nil || s.resolver == nil {
		return "", errors.New("AGENT_PRICING_UNAVAILABLE")
	}
	return s.agentCostBoundWithDeps(ctx, key, model, kind, inputBytes, s.inflightEstimateDeps())
}

// Billing source describes how an already configured request is charged; it is
// not itself evidence that the tariff is unbounded. Resolve every deterministic
// channel/account candidate and take the maximum, including model multipliers.
func (s *OpenAIGatewayService) agentCostBoundWithDeps(ctx context.Context, key *APIKey, model, kind string, inputBytes int, deps inflightEstimateDeps) (string, error) {
	if key == nil || key.Group == nil || s.resolver == nil || model == "" || inputBytes < 0 {
		return "", errors.New("AGENT_PRICING_UNAVAILABLE")
	}
	mapping := ChannelMappingResult{MappedModel: model}
	if deps.resolveMapping != nil && key.GroupID != nil {
		mapping = deps.resolveMapping(ctx, *key.GroupID, model)
	}
	// A model invented by an untrusted future response cannot be bounded here.
	if mapping.BillingModelSource == BillingModelSourceResponse {
		return "", errors.New("AGENT_UNBOUNDED_PRICING")
	}
	primary, fallbacks, upstream := inflightBillingModelCandidates(ctx, deps, key, model)
	models := append(append([]string{}, primary...), fallbacks...)
	if deps.accountMappedModels != nil {
		models = append(models, deps.accountMappedModels(ctx, key, upstream)...)
	}
	best := decimal.Zero
	found := false
	seen := map[string]bool{}
	var missing error
	for _, candidate := range models {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		quote, err := s.agentCostBoundForModel(ctx, key, candidate, kind, inputBytes, deps)
		if err != nil {
			if err.Error() == "AGENT_PRICING_UNAVAILABLE" || err.Error() == "AGENT_IMAGE_FIXED_PRICE_REQUIRED" {
				missing = err
				continue
			}
			return "", err
		}
		amount, err := decimal.NewFromString(quote)
		if err != nil {
			return "", errors.New("AGENT_UNBOUNDED_PRICING")
		}
		if !found || amount.GreaterThan(best) {
			best = amount
		}
		found = true
	}
	if !found {
		if missing != nil {
			return "", missing
		}
		return "", errors.New("AGENT_PRICING_UNAVAILABLE")
	}
	return best.StringFixed(8), nil
}

func (s *OpenAIGatewayService) agentCostBoundForModel(ctx context.Context, key *APIKey, model, kind string, inputBytes int, deps inflightEstimateDeps) (string, error) {
	resolved := s.resolver.Resolve(ctx, PricingInput{Model: model, GroupID: key.GroupID, Group: key.Group})
	if resolved == nil {
		return "", errors.New("AGENT_PRICING_UNAVAILABLE")
	}
	textRate, imageRate := deps.rates(ctx, key, model)
	rate := textRate
	if kind == "image" {
		rate = imageRate
	}
	if rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return "", errors.New("AGENT_UNBOUNDED_PRICING")
	}
	var bound float64
	// Image groups can intentionally have no channel/model card and use the
	// legacy fixed group tiers. Those prices are explicit per-image amounts,
	// unlike a catalog's token-based image_output_price.
	if kind == "image" && resolved.Mode == BillingModeToken {
		for _, tier := range []string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K} {
			price := key.Group.GetImagePrice(tier)
			if price == nil || *price < 0 || math.IsNaN(*price) || math.IsInf(*price, 0) {
				return "", errors.New("AGENT_IMAGE_FIXED_PRICE_REQUIRED")
			}
			bound = math.Max(bound, *price*rate)
		}
		if math.IsNaN(bound) || math.IsInf(bound, 0) {
			return "", errors.New("AGENT_UNBOUNDED_PRICING")
		}
		return decimal.NewFromFloat(bound).Mul(decimal.NewFromFloat(1.10)).RoundCeil(8).StringFixed(8), nil
	}
	switch resolved.Mode {
	case BillingModeImage, BillingModePerRequest:
		bound = maxPerRequestPrice(resolved) * rate
		if kind == "image" {
			for _, tier := range []string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K} {
				if price := key.Group.GetImagePrice(tier); price != nil && *price >= 0 {
					bound = math.Max(bound, *price*rate)
				}
			}
		}
		if bound == 0 && resolved.channelPricing != nil && resolved.channelPricing.PerRequestPrice == nil && len(resolved.RequestTiers) == 0 {
			return "", errors.New("AGENT_PRICING_UNAVAILABLE")
		}
	case BillingModeToken:
		if kind == "image" {
			return "", errors.New("AGENT_IMAGE_FIXED_PRICE_REQUIRED")
		}
		inputs := []int{0, inputBytes + 512}
		for _, iv := range resolved.Intervals {
			inputs = append(inputs, iv.MinTokens)
			if iv.MaxTokens != nil {
				inputs = append(inputs, *iv.MaxTokens)
			}
		}
		for _, n := range inputs {
			p := s.resolver.GetIntervalPricing(resolved, n)
			if p == nil {
				continue
			}
			inPrice := math.Max(p.InputPricePerToken, math.Max(p.CacheCreationPricePerToken, p.CacheReadPricePerToken))
			if kind == "vision" {
				inPrice = math.Max(inPrice, p.ImageInputPricePerToken)
			}
			inPrice = math.Max(inPrice, math.Max(p.InputPricePerTokenPriority, math.Max(p.CacheCreationPricePerTokenPriority, p.CacheReadPricePerTokenPriority)))
			outPrice := math.Max(p.OutputPricePerToken, p.OutputPricePerTokenPriority)
			inPrice *= math.Max(1, p.LongContextInputMultiplier)
			outPrice *= math.Max(1, p.LongContextOutputMultiplier)
			mult := 1.0
			if key.Group.ForceOpenAIFast {
				mult = math.Max(mult, 2)
			}
			if p.FastMultiplier != nil {
				mult = math.Max(mult, *p.FastMultiplier)
			}
			for _, v := range p.ReasoningEffortMultipliers {
				if v > mult {
					mult = v
				}
			}
			c := (float64(inputBytes+512)*inPrice + 2048*outPrice) * rate * mult
			if c > bound {
				bound = c
			}
		}
		if bound <= 0 && rate > 0 {
			return "", errors.New("AGENT_PRICING_UNAVAILABLE")
		}
	default:
		return "", errors.New("AGENT_UNBOUNDED_PRICING")
	}
	if resolved.channelPricing != nil && resolved.channelPricing.TimePricing != nil {
		peak := 1.0
		for _, period := range resolved.channelPricing.TimePricing.Periods {
			peak = math.Max(peak, period.Multiplier)
		}
		bound *= peak
	}
	if math.IsNaN(bound) || math.IsInf(bound, 0) || bound < 0 {
		return "", errors.New("AGENT_UNBOUNDED_PRICING")
	}
	// Guard against finite precision and small serialization overhead; dispatch
	// still uses the existing gateway tariff and records the actual charge.
	return decimal.NewFromFloat(bound).Mul(decimal.NewFromFloat(1.10)).RoundCeil(8).StringFixed(8), nil
}

// Recheck the selected account immediately before the paid request. Scheduler
// snapshots may change between the initial quote and account selection.
func (s *OpenAIGatewayService) CheckAgentSelectedAccountPrice(ctx context.Context, key *APIKey, account *Account, requested, forward, kind string, inputBytes int) error {
	return s.checkAgentSelectedAccountPrice(ctx, key, account, requested, forward, kind, inputBytes, s.inflightEstimateDeps())
}
func (s *OpenAIGatewayService) checkAgentSelectedAccountPrice(ctx context.Context, key *APIKey, account *Account, requested, forward, kind string, inputBytes int, deps inflightEstimateDeps) error {
	trace := AgentInvocationFromContext(ctx)
	if trace == nil {
		return nil
	}
	if trace.InputImageCount > 0 {
		if account == nil {
			return errors.New("AGENT_PRICING_UNAVAILABLE")
		}
		target := forward
		if target == "" {
			target = requested
		}
		target = account.GetMappedModel(target)
		if meta, ok := account.GetUpstreamModelMetadata(target); ok && len(meta.InputModalities) > 0 {
			if !stringSliceContains(normalizeCodexInputModalities(meta.InputModalities), "image") {
				return errors.New("AGENT_MODEL_VISION_UNSUPPORTED")
			}
		}
		kind = "vision"
		inputBytes = max(0, inputBytes-trace.VisionEncodedBytes) + AgentVisionInputReserve*trace.InputImageCount
	}
	if trace.CostLimit == "" {
		return nil
	}
	limit, err := decimal.NewFromString(trace.CostLimit)
	if err != nil || limit.IsNegative() {
		return errors.New("AGENT_UNBOUNDED_PRICING")
	}
	if account == nil {
		return errors.New("AGENT_PRICING_UNAVAILABLE")
	}
	if forward == "" {
		forward = requested
	}
	candidates := usageBillingModelCandidates(requested, forward, account.GetMappedModel(forward))
	found := false
	for _, model := range candidates {
		quote, err := s.agentCostBoundForModel(ctx, key, model, kind, inputBytes, deps)
		if err != nil {
			if err.Error() == "AGENT_PRICING_UNAVAILABLE" || err.Error() == "AGENT_IMAGE_FIXED_PRICE_REQUIRED" {
				continue
			}
			return err
		}
		amount, err := decimal.NewFromString(quote)
		if err != nil {
			return errors.New("AGENT_UNBOUNDED_PRICING")
		}
		if amount.GreaterThan(limit) {
			return errors.New("AGENT_PRICING_CHANGED")
		}
		found = true
	}
	if !found {
		return errors.New("AGENT_PRICING_UNAVAILABLE")
	}
	return nil
}

// The generic gateway uses its own scheduler candidates and rate resolver.
func (s *GatewayService) AgentCostBound(ctx context.Context, key *APIKey, model, kind string, inputBytes int) (string, error) {
	calculator := &OpenAIGatewayService{resolver: s.resolver}
	return calculator.agentCostBoundWithDeps(ctx, key, model, kind, inputBytes, s.inflightEstimateDeps())
}
func (s *GatewayService) CheckAgentSelectedAccountPrice(ctx context.Context, key *APIKey, account *Account, requested, forward, kind string, inputBytes int) error {
	calculator := &OpenAIGatewayService{resolver: s.resolver}
	return calculator.checkAgentSelectedAccountPrice(ctx, key, account, requested, forward, kind, inputBytes, s.inflightEstimateDeps())
}
