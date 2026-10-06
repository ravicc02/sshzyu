package service

import "testing"

func testRatePtr(v float64) *float64 { return &v }

// TestApplyModelRateMultiplier 覆盖「模型级倍率（替代语义）」的核心行为：
// 命中精确名 / 通配名时替代基准倍率，未命中或配置非法时原样返回。
func TestApplyModelRateMultiplier(t *testing.T) {
	svc := &GatewayService{}
	group := &Group{
		ModelPricing: []ChannelModelPricing{
			{Models: []string{"claude-opus-4-1"}, RateMultiplier: testRatePtr(1.35)},
			{Models: []string{"wild-*"}, RateMultiplier: testRatePtr(2.0)},
		},
	}
	apiKey := &APIKey{Group: group}

	t.Run("命中精确名则替代基准", func(t *testing.T) {
		if got := svc.applyModelRateMultiplier(apiKey, "claude-opus-4-1", 1.0); got != 1.35 {
			t.Fatalf("want 1.35, got %v", got)
		}
	})

	t.Run("同组未命中模型保持基准（不影响其他模型）", func(t *testing.T) {
		if got := svc.applyModelRateMultiplier(apiKey, "gpt-5", 1.0); got != 1.0 {
			t.Fatalf("want 1.0, got %v", got)
		}
	})

	t.Run("通配命中则替代", func(t *testing.T) {
		if got := svc.applyModelRateMultiplier(apiKey, "wild-thing", 1.0); got != 2.0 {
			t.Fatalf("want 2.0, got %v", got)
		}
	})

	t.Run("非法倍率(<=0)回落基准", func(t *testing.T) {
		g := &Group{ModelPricing: []ChannelModelPricing{{Models: []string{"m"}, RateMultiplier: testRatePtr(0)}}}
		if got := svc.applyModelRateMultiplier(&APIKey{Group: g}, "m", 1.0); got != 1.0 {
			t.Fatalf("want 1.0, got %v", got)
		}
	})

	t.Run("未配置 rate_multiplier 回落基准", func(t *testing.T) {
		g := &Group{ModelPricing: []ChannelModelPricing{{Models: []string{"m"}, InputPrice: testRatePtr(3)}}}
		if got := svc.applyModelRateMultiplier(&APIKey{Group: g}, "m", 1.0); got != 1.0 {
			t.Fatalf("want 1.0, got %v", got)
		}
	})

	t.Run("nil 分组 / 空模型名回落基准", func(t *testing.T) {
		if got := svc.applyModelRateMultiplier(&APIKey{}, "m", 1.0); got != 1.0 {
			t.Fatalf("nil group: want 1.0, got %v", got)
		}
		if got := svc.applyModelRateMultiplier(apiKey, "", 1.0); got != 1.0 {
			t.Fatalf("empty model: want 1.0, got %v", got)
		}
	})
}

// TestGroupPricingHasPricing 验证「仅配非价格字段（如 rate_multiplier）的条目」
// 不被视为含价格——这是 Resolve 不短路价格解析、避免错价的前提。
func TestGroupPricingHasPricing(t *testing.T) {
	t.Run("仅 rate_multiplier 视为不含价格", func(t *testing.T) {
		if groupPricingHasPricing(&ChannelModelPricing{Models: []string{"m"}, RateMultiplier: testRatePtr(1.5)}) {
			t.Fatal("只配 rate_multiplier 不应视为含价格")
		}
	})
	t.Run("含 input_price 视为含价格", func(t *testing.T) {
		if !groupPricingHasPricing(&ChannelModelPricing{Models: []string{"m"}, InputPrice: testRatePtr(1)}) {
			t.Fatal("含 input_price 应视为含价格")
		}
	})
	t.Run("含 intervals 视为含价格", func(t *testing.T) {
		if !groupPricingHasPricing(&ChannelModelPricing{Models: []string{"m"}, Intervals: []PricingInterval{{MinTokens: 0}}}) {
			t.Fatal("含 intervals 应视为含价格")
		}
	})
	t.Run("空条目不含价格", func(t *testing.T) {
		if groupPricingHasPricing(&ChannelModelPricing{Models: []string{"m"}}) {
			t.Fatal("空条目不应视为含价格")
		}
		if groupPricingHasPricing(nil) {
			t.Fatal("nil 不应视为含价格")
		}
	})
}
