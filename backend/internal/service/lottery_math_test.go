package service

import (
	"testing"
	"time"
)

// timeUtc 构造 UTC 时间(测试辅助)。
func timeUtc(year int, month time.Month, day, hour, min, sec int) time.Time {
	return time.Date(year, month, day, hour, min, sec, 0, time.UTC)
}

// TestLotteryUserTier 阶梯边界: 与次数解锁阈值一致($5/$15/$25/$35),封顶王者。
func TestLotteryUserTier(t *testing.T) {
	cases := []struct {
		name       string
		spentCents int64
		want       int64
	}{
		{"zero spent bronze", 0, 0},
		{"just below $5 bronze", 499, 0},
		{"exactly $5 silver", 500, 1},
		{"just below $15 silver", 1499, 1},
		{"exactly $15 gold", 1500, 2},
		{"just below $25 gold", 2499, 2},
		{"exactly $25 diamond", 2500, 3},
		{"exactly $35 king", 3500, 4},
		{"beyond $45 capped king", 4500, 4},
		{"large amount capped king", 10000, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LotteryUserTier(tc.spentCents); got != tc.want {
				t.Fatalf("LotteryUserTier(%d) = %d, want %d", tc.spentCents, got, tc.want)
			}
		})
	}
}

// TestLotteryTierThresholdCents 阶梯门槛与次数阈值一致。
func TestLotteryTierThresholdCents(t *testing.T) {
	cases := []struct {
		tier int64
		want int64
	}{
		{0, 0},
		{1, 500},
		{2, 1500},
		{3, 2500},
		{4, 3500},
		{5, -1}, // 超出最大阶梯
	}
	for _, tc := range cases {
		if got := LotteryTierThresholdCents(tc.tier); got != tc.want {
			t.Fatalf("LotteryTierThresholdCents(%d) = %d, want %d", tc.tier, got, tc.want)
		}
	}
}

// TestLotteryPrizeTierWeights 有效权重与阶梯可用性:
// tier_weights 显式配置优先(含 0 不可中),缺失回退基础 weight,min_tier 拦截低阶梯。
func TestLotteryPrizeTierWeights(t *testing.T) {
	p := LotteryPrize{
		Weight:  10,
		MinTier: 2,
		TierWeights: map[string]int{
			"0": 70, "1": 50, "2": 30, "3": 0, // "4" 缺失,回退 weight=10
		},
	}
	if got := p.EffectiveWeight(0); got != 70 {
		t.Fatalf("EffectiveWeight(0) = %d, want 70", got)
	}
	if got := p.EffectiveWeight(3); got != 0 {
		t.Fatalf("EffectiveWeight(3) = %d, want 0(显式不可中)", got)
	}
	if got := p.EffectiveWeight(4); got != 10 {
		t.Fatalf("EffectiveWeight(4) = %d, want 10(缺失回退 weight)", got)
	}
	if p.AvailableAtTier(1) {
		t.Fatal("tier 1 低于 min_tier=2, 应不可用")
	}
	if p.AvailableAtTier(3) {
		t.Fatal("tier 3 显式权重 0, 应不可用")
	}
	if !p.AvailableAtTier(4) {
		t.Fatal("tier 4 回退权重 10, 应可用")
	}
	// nil TierWeights: 全部回退基础权重。
	p2 := LotteryPrize{Weight: 10, MinTier: 0}
	if !p2.AvailableAtTier(0) || p2.EffectiveWeight(4) != 10 {
		t.Fatal("nil tier_weights 应回退基础权重")
	}
}

// TestLotteryThresholdEntitlement 阈值边界: $5 解锁第 2 抽,此后每 $10 一次。
// 验收口径: $4.99 不解锁, $5 解锁, $14.99 不解锁第 3 抽, $15 解锁, $25 解锁第 4 抽。
func TestLotteryThresholdEntitlement(t *testing.T) {
	cases := []struct {
		name       string
		spentCents int64
		want       int64
	}{
		{"zero spent", 0, 0},
		{"just below $5", 499, 0},
		{"exactly $5", 500, 1},
		{"mid range $10", 1000, 1},
		{"just below $15", 1499, 1},
		{"exactly $15", 1500, 2},
		{"just below $25", 2499, 2},
		{"exactly $25", 2500, 3},
		{"exactly $35", 3500, 4},
		{"large amount $100", 10000, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LotteryThresholdEntitlement(tc.spentCents); got != tc.want {
				t.Fatalf("LotteryThresholdEntitlement(%d) = %d, want %d", tc.spentCents, got, tc.want)
			}
		})
	}
}

func TestLotteryNextThresholdCents(t *testing.T) {
	cases := []struct {
		entitlement int64
		want        int64
	}{
		{0, 500},  // 未解锁 -> 下一门槛 $5
		{1, 1500}, // 已解锁 1 -> 下一门槛 $15
		{2, 2500}, // 已解锁 2 -> 下一门槛 $25
	}
	for _, tc := range cases {
		if got := LotteryNextThresholdCents(tc.entitlement); got != tc.want {
			t.Fatalf("LotteryNextThresholdCents(%d) = %d, want %d", tc.entitlement, got, tc.want)
		}
	}
}

// TestBalanceSpentFloatToCents 金额换算无浮点歧义。
func TestBalanceSpentFloatToCents(t *testing.T) {
	cases := []struct {
		spent float64
		want  int64
	}{
		{0, 0},
		{4.99, 499},
		{5.0, 500},
		{14.99, 1499},
		{15.0, 1500},
		{0.005, 1},   // 四舍五入
		{-1.5, 0},    // 负值钳制为 0
	}
	for _, tc := range cases {
		if got := BalanceSpentFloatToCents(tc.spent); got != tc.want {
			t.Fatalf("BalanceSpentFloatToCents(%v) = %d, want %d", tc.spent, got, tc.want)
		}
	}
}

func TestClassifyDrawSource(t *testing.T) {
	status := func(used, threshold, manual int64) *LotteryUserStatus {
		return &LotteryUserStatus{
			UsedDraws:            used,
			ThresholdEntitlement: threshold,
			ManualAdjustment:     manual,
		}
	}
	cases := []struct {
		name         string
		status       *LotteryUserStatus
		firstGranted bool
		want         string
	}{
		{"first draw consumed", status(0, 0, 0), true, LotteryDrawSourceFirst},
		{"threshold draw", status(1, 1, 0), true, LotteryDrawSourceThreshold},
		{"manual draw", status(2, 1, 1), true, LotteryDrawSourceManual},
		{"no first grant uses threshold directly", status(0, 1, 0), false, LotteryDrawSourceThreshold},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyDrawSource(tc.status, tc.firstGranted); got != tc.want {
				t.Fatalf("classifyDrawSource = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWeightedPick(t *testing.T) {
	if weightedPick(nil) != nil {
		t.Fatal("empty candidates should return nil")
	}
	// 全 0 权重应返回 nil
	zero := []LotteryPrize{{Weight: 0}, {Weight: 0}}
	if weightedPick(zero) != nil {
		t.Fatal("all-zero weights should return nil")
	}
	// 单一候选必中
	single := []LotteryPrize{{ID: 7, Weight: 5}}
	if got := weightedPick(single); got == nil || got.ID != 7 {
		t.Fatalf("single candidate should always be picked, got %+v", got)
	}
	// 多候选: 返回值必须属于候选集合
	multi := []LotteryPrize{{ID: 1, Weight: 1}, {ID: 2, Weight: 3}, {ID: 3, Weight: 6}}
	for i := 0; i < 200; i++ {
		got := weightedPick(multi)
		if got == nil || (got.ID != 1 && got.ID != 2 && got.ID != 3) {
			t.Fatalf("picked invalid prize %+v", got)
		}
	}
}

func TestValidateIdempotencyKey(t *testing.T) {
	if err := validateIdempotencyKey(""); err == nil {
		t.Fatal("empty key should be rejected")
	}
	long := make([]byte, 65)
	for i := range long {
		long[i] = 'a'
	}
	if err := validateIdempotencyKey(string(long)); err == nil {
		t.Fatal("oversized key should be rejected")
	}
	if err := validateIdempotencyKey("draw-abc-123"); err != nil {
		t.Fatalf("valid key should pass, got %v", err)
	}
}

func TestLotteryActivityIsOpen(t *testing.T) {
	now := timeUtc(2026, 9, 24, 12, 0, 0)
	start := timeUtc(2026, 9, 24, 10, 0, 0)
	end := timeUtc(2026, 9, 24, 14, 0, 0)

	open := &LotteryActivity{Status: LotteryActivityStatusActive, StartsAt: &start, EndsAt: &end}
	if !open.IsOpen(now) {
		t.Fatal("activity within window should be open")
	}
	if open.IsOpen(timeUtc(2026, 9, 24, 9, 0, 0)) {
		t.Fatal("activity should be closed before starts_at")
	}
	if open.IsOpen(timeUtc(2026, 9, 24, 15, 0, 0)) {
		t.Fatal("activity should be closed after ends_at")
	}
	paused := &LotteryActivity{Status: LotteryActivityStatusPaused}
	if paused.IsOpen(now) {
		t.Fatal("paused activity should be closed")
	}
}
