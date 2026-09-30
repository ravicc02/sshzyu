package service

import (
	"testing"
	"time"
)

// timeUtc 构造 UTC 时间(测试辅助)。
func timeUtc(year int, month time.Month, day, hour, min, sec int) time.Time {
	return time.Date(year, month, day, hour, min, sec, 0, time.UTC)
}

// TestLotteryUserTier 阶梯边界: 与次数解锁阈值一致($5/$55/$105/$155),封顶王者。
func TestLotteryUserTier(t *testing.T) {
	cases := []struct {
		name       string
		spentCents int64
		want       int64
	}{
		{"zero spent bronze", 0, 0},
		{"just below $5 bronze", 499, 0},
		{"exactly $5 silver", 500, 1},
		{"just below $55 silver", 5499, 1},
		{"exactly $55 gold", 5500, 2},
		{"just below $105 gold", 10499, 2},
		{"exactly $105 diamond", 10500, 3},
		{"just below $155 diamond", 15499, 3},
		{"exactly $155 king", 15500, 4},
		{"beyond $205 capped king", 20500, 4},
		{"large amount capped king", 100000, 4},
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
		{2, 5500},
		{3, 10500},
		{4, 15500},
		{5, -1}, // 超出最大阶梯
	}
	for _, tc := range cases {
		if got := LotteryTierThresholdCents(tc.tier); got != tc.want {
			t.Fatalf("LotteryTierThresholdCents(%d) = %d, want %d", tc.tier, got, tc.want)
		}
	}
}

// TestLotteryPrizeTierWeights 验证显式阶梯中奖概率与最低阶梯限制。
// 历史数据缺少 tier_weights 时仍兼容回退基础 weight。
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

// TestLotteryThresholdEntitlement 阈值边界: $5 解锁第 2 抽,此后每 $50 一次。
// 验收口径: $4.99 不解锁, $5 解锁, $54.99 不解锁第 3 抽, $55 解锁, $105 解锁第 4 抽。
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
		{"just below $55", 5499, 1},
		{"exactly $55", 5500, 2},
		{"just below $105", 10499, 2},
		{"exactly $105", 10500, 3},
		{"exactly $155", 15500, 4},
		{"exactly $205", 20500, 5},
		{"large amount $1000", 100000, 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LotteryThresholdEntitlement(tc.spentCents); got != tc.want {
				t.Fatalf("LotteryThresholdEntitlement(%d) = %d, want %d", tc.spentCents, got, tc.want)
			}
		})
	}
}

func TestLotterySpendWindow(t *testing.T) {
	if LotterySpendWindow != 24*time.Hour {
		t.Fatalf("LotterySpendWindow = %s, want 24h", LotterySpendWindow)
	}
}

func TestLotteryPointsWindow(t *testing.T) {
	base := timeUtc(2026, 10, 1, 0, 0, 0)
	cases := []struct {
		name    string
		now     time.Duration
		draws   []time.Duration
		start   time.Duration
		expires *time.Duration
	}{
		{"no draw", 30 * time.Hour, nil, 6 * time.Hour, nil},
		{"first draw", 12 * time.Hour, []time.Duration{10 * time.Hour}, 0, durationPtr(34 * time.Hour)},
		{"successful draw refresh", 35 * time.Hour, []time.Duration{10 * time.Hour, 33 * time.Hour}, 0, durationPtr(57 * time.Hour)},
		{"exact expiry excludes old spend", 57 * time.Hour, []time.Duration{10 * time.Hour, 33 * time.Hour}, 57 * time.Hour, durationPtr(57 * time.Hour)},
		{"new session cannot revive old spend", 60 * time.Hour, []time.Duration{10 * time.Hour, 33 * time.Hour, 59 * time.Hour}, 57 * time.Hour, durationPtr(83 * time.Hour)},
		{"draw exactly at expiry starts new session", 35 * time.Hour, []time.Duration{10 * time.Hour, 34 * time.Hour}, 34 * time.Hour, durationPtr(58 * time.Hour)},
		{"long expired", 90 * time.Hour, []time.Duration{10 * time.Hour}, 66 * time.Hour, durationPtr(34 * time.Hour)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			times := make([]time.Time, len(tc.draws))
			for i, at := range tc.draws {
				times[i] = base.Add(at)
			}
			start, expires := lotteryPointsWindow(base.Add(tc.now), base, times)
			if !start.Equal(base.Add(tc.start)) {
				t.Fatalf("start = %v, want %v", start, base.Add(tc.start))
			}
			if tc.expires == nil {
				if expires != nil {
					t.Fatalf("unexpected expires %v", expires)
				}
				return
			}
			if expires == nil || !expires.Equal(base.Add(*tc.expires)) {
				t.Fatalf("expires = %v, want %v", expires, base.Add(*tc.expires))
			}
		})
	}
}

func durationPtr(d time.Duration) *time.Duration { return &d }

func TestLotteryTierConfig(t *testing.T) {
	fixed := DefaultLotteryTierConfig()
	if !fixed.IsValid() {
		t.Fatal("default fixed tier config should be valid")
	}
	if got := fixed.Entitlement(15500); got != 4 {
		t.Fatalf("fixed entitlement at $155 = %d, want 4", got)
	}
	if got := fixed.UserTier(20500); got != LotteryTierKing {
		t.Fatalf("fixed tier at $205 = %d, want king", got)
	}

	custom := LotteryTierConfig{
		Mode:       LotteryTierModeCustom,
		Thresholds: []int64{500, 5500, 10500, 15500},
	}
	if !custom.IsValid() {
		t.Fatal("strictly increasing custom tier config should be valid")
	}
	if got := custom.Entitlement(10499); got != 2 {
		t.Fatalf("custom entitlement below $105 = %d, want 2", got)
	}
	if got := custom.Entitlement(15500); got != 4 {
		t.Fatalf("custom entitlement at $155 = %d, want 4", got)
	}
	if got := custom.NextThresholdCents(15500); got != -1 {
		t.Fatalf("custom max threshold next = %d, want -1", got)
	}

	invalid := LotteryTierConfig{
		Mode:       LotteryTierModeCustom,
		Thresholds: []int64{500, 500, 10500, 15500},
	}
	if invalid.IsValid() {
		t.Fatal("non-increasing custom thresholds must be invalid")
	}

	duplicateNames := LotteryTierConfig{
		Mode: LotteryTierModeCustom,
		Definitions: []LotteryTierDefinition{
			{Name: "青铜", ThresholdCents: 0},
			{Name: "白银", ThresholdCents: 500},
			{Name: "白银", ThresholdCents: 1000},
		},
	}
	if duplicateNames.IsValid() {
		t.Fatal("duplicate custom tier names must be invalid")
	}

	dynamic := LotteryTierConfig{
		Mode: LotteryTierModeCustom,
		Definitions: []LotteryTierDefinition{
			{Name: "起步", ThresholdCents: 0},
			{Name: "进阶", ThresholdCents: 100},
			{Name: "大师", ThresholdCents: 1000},
		},
	}
	if got := dynamic.Entitlement(1000); got != 2 {
		t.Fatalf("dynamic entitlement = %d, want 2", got)
	}
	if got := dynamic.TierName(2); got != "大师" {
		t.Fatalf("dynamic tier name = %q, want 大师", got)
	}
}

func TestLotteryNextThresholdCents(t *testing.T) {
	cases := []struct {
		entitlement int64
		want        int64
	}{
		{0, 500},   // 未解锁 -> 下一门槛 $5
		{1, 5500},  // 已解锁 1 -> 下一门槛 $55
		{2, 10500}, // 已解锁 2 -> 下一门槛 $105
		{4, 20500}, // 已解锁 4 -> 下一门槛 $205
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
		{0.005, 1}, // 四舍五入
		{-1.5, 0},  // 负值钳制为 0
	}
	for _, tc := range cases {
		if got := BalanceSpentFloatToCents(tc.spent); got != tc.want {
			t.Fatalf("BalanceSpentFloatToCents(%v) = %d, want %d", tc.spent, got, tc.want)
		}
	}
}

func TestClassifyDrawSource(t *testing.T) {
	status := func(firstRemaining, thresholdRemaining, manualRemaining int64) *LotteryUserStatus {
		return &LotteryUserStatus{
			firstRemaining:     firstRemaining,
			thresholdRemaining: thresholdRemaining,
			manualRemaining:    manualRemaining,
		}
	}
	cases := []struct {
		name   string
		status *LotteryUserStatus
		want   string
	}{
		{"first draw has priority", status(1, 3, 2), LotteryDrawSourceFirst},
		{"current window threshold draw", status(0, 1, 2), LotteryDrawSourceThreshold},
		{"manual draw after other pools exhausted", status(0, 0, 1), LotteryDrawSourceManual},
		{"expired threshold uses manual pool", status(0, 0, 1), LotteryDrawSourceManual},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyDrawSource(tc.status, false); got != tc.want {
				t.Fatalf("classifyDrawSource = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateLotteryTierWeightSumsForTierCount(t *testing.T) {
	prizes := []LotteryPrize{
		{ID: 1, Enabled: true, MinTier: 0, Weight: 100, TierWeights: map[string]int{"0": 100, "1": 0}},
	}
	if err := validateLotteryTierWeightSumsForTierCount(prizes, 2); err == nil {
		t.Fatal("an explicitly zero-weight second tier must be rejected")
	}
	if err := validateLotteryTierWeightSumsForTierCount(prizes, 1); err != nil {
		t.Fatalf("single-tier configuration should be valid: %v", err)
	}
}

func TestValidateLotteryTierWeightSums(t *testing.T) {
	valid := []LotteryPrize{
		{ID: 1, Enabled: true, MinTier: 0, Weight: 70, TierWeights: map[string]int{"0": 70, "1": 50, "2": 30, "3": 15, "4": 5}},
		{ID: 2, Enabled: true, MinTier: 0, Weight: 25, TierWeights: map[string]int{"0": 25, "1": 20, "2": 10, "3": 0, "4": 0}},
		{ID: 3, Enabled: true, MinTier: 0, Weight: 5, TierWeights: map[string]int{"0": 5, "1": 30, "2": 60, "3": 85, "4": 95}},
	}
	if err := validateLotteryTierWeightSums(valid); err != nil {
		t.Fatalf("valid 100%% tier weights rejected: %v", err)
	}

	invalid := append([]LotteryPrize(nil), valid...)
	invalid[2].TierWeights = cloneLotteryTierWeights(valid[2].TierWeights)
	invalid[2].TierWeights["2"] = 59
	if err := validateLotteryTierWeightSums(invalid); err == nil {
		t.Fatal("tier total of 99 must be rejected")
	}

	// 售罄奖品仍是配置的一部分；不能导致保存既有100%配置时失败。
	valid[1].Stock = 0
	if err := validateLotteryTierWeightSums(valid); err != nil {
		t.Fatalf("sold-out prize must remain part of configuration total: %v", err)
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
