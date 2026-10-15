package service

import (
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestOpenAIModelQualityIsolationExpiryAndBounds(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	now := time.Now()
	slow, fast := 3000, 200
	stats.reportModel(1, "slow-model", false, &slow, now)
	stats.reportModel(1, "fast-model", true, &fast, now)
	e, ttft, ok := stats.snapshotModel(1, "fast-model", now)
	require.True(t, ok)
	require.Zero(t, e)
	require.Equal(t, float64(fast), ttft)
	e, _, ok = stats.snapshotModel(1, "unknown", now)
	require.False(t, ok)
	require.Zero(t, e)
	_, _, ok = stats.snapshotModel(1, "slow-model", now.Add(openAIModelRuntimeTTL))
	require.False(t, ok)
	stats.reportModel(1, "slow-model", true, &fast, now.Add(openAIModelRuntimeTTL))
	e, ttft, ok = stats.snapshotModel(1, "slow-model", now.Add(openAIModelRuntimeTTL))
	require.True(t, ok)
	require.Zero(t, e)
	require.Equal(t, float64(fast), ttft)
	for i := 0; i < 100; i++ {
		stats.reportModel(1, fmt.Sprint(i), true, &fast, now.Add(time.Duration(i)*time.Second))
	}
	account := stats.loadOrCreate(1)
	require.Len(t, account.models, openAIModelRuntimeLimit)
	require.Equal(t, 1, stats.size())
}

func TestOpenAIModelQualityConcurrent(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				ms := 200
				stats.reportModel(1, "model", true, &ms, time.Now())
				stats.snapshotModel(1, "model", time.Now())
			}
		}()
	}
	wg.Wait()
	_, ttft, ok := stats.snapshotModel(1, "model", time.Now())
	require.True(t, ok)
	require.Equal(t, 200.0, ttft)
}

func TestOpenAILatencyAwareMixedPoolAndCapacity(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	cfg := &config.Config{}
	cfg.Gateway.OpenAIScheduler.LatencyAwareEnabled = true
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights = config.GatewayOpenAIWSSchedulerScoreWeights{Priority: 1, Load: 1, Queue: .7, ErrorRate: .8, TTFT: .5}
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{cfg: cfg}, stats: newOpenAIAccountRuntimeStats()}
	slow, fast := 3000, 200
	now := time.Now()
	scheduler.stats.reportModel(1, "model", true, &slow, now)
	scheduler.stats.reportModel(2, "model", true, &fast, now)
	accounts := []*Account{{ID: 1, Concurrency: 2}, {ID: 2, Concurrency: 2}}
	plan := scheduler.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{RequestedModel: "model"}, accounts, map[int64]*AccountLoadInfo{1: {AccountID: 1}, 2: {AccountID: 2}})
	require.Len(t, plan.selectionOrder, 2)
	require.Equal(t, int64(2), plan.selectionOrder[0].account.ID)
	require.Equal(t, int64(1), plan.selectionOrder[1].account.ID)
	// With both in Top-K, lottery must not keep assigning to the known slow account.
	cfg.Gateway.OpenAIWS.LBTopK = 2
	fastSelections := 0
	for i := 0; i < 100; i++ {
		plan = scheduler.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{RequestedModel: "model", SessionHash: fmt.Sprint("probe-", i)}, accounts, map[int64]*AccountLoadInfo{1: {AccountID: 1}, 2: {AccountID: 2}})
		if plan.selectionOrder[0].account.ID == 2 {
			fastSelections++
		}
	}
	require.GreaterOrEqual(t, fastSelections, 80)
	require.Less(t, fastSelections, 100)
	// Congestion outweighs latency so the fast supplier is not overloaded.
	plan = scheduler.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{RequestedModel: "model"}, accounts, map[int64]*AccountLoadInfo{1: {AccountID: 1}, 2: {AccountID: 2, LoadRate: 100, WaitingCount: 10}})
	require.Equal(t, int64(1), plan.selectionOrder[0].account.ID)
	// Legacy mode consumes the original aggregate statistics.
	cfg.Gateway.OpenAIScheduler.LatencyAwareEnabled = false
	scheduler.stats.report(1, true, &slow)
	_, ttft, ok := scheduler.runtimeSnapshot(1, "unknown")
	require.True(t, ok)
	require.Equal(t, 3000.0, ttft)
	cfg.Gateway.OpenAIScheduler.LatencyAwareEnabled = true
	_, _, ok = scheduler.runtimeSnapshot(1, "unknown")
	require.False(t, ok)
}

func TestOpenAILatencyAwareSelectSaturatedFastAccount(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	slowAccount := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	fastAccount := slowAccount
	fastAccount.ID = 2
	cache := &upstreamCostTrackingConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{1: {AccountID: 1}, 2: {AccountID: 2, CurrentConcurrency: 1, LoadRate: 100}}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIScheduler.LatencyAwareEnabled = true
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights = config.GatewayOpenAIWSSchedulerScoreWeights{Load: 1, TTFT: .5}
	svc := &OpenAIGatewayService{cfg: cfg, accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{slowAccount, fastAccount}}, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"), concurrencyService: NewConcurrencyService(cache)}
	svc.ReportOpenAIAccountScheduleResult(&slowAccount, "gpt-test", true, func() *int { v := 3000; return &v }())
	svc.ReportOpenAIAccountScheduleResult(&fastAccount, "gpt-test", true, func() *int { v := 200; return &v }())
	group := int64(1)
	selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, slowAccount.ID, selection.Account.ID)
	require.Empty(t, cache.limits(fastAccount.ID))
	selection.ReleaseFunc()
	require.Equal(t, 1, cache.releaseCount(slowAccount.ID))
}

func TestOpenAILatencyAwareMappedModelFeedback(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	cfg := &config.Config{}
	cfg.Gateway.OpenAIScheduler.LatencyAwareEnabled = true
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights = config.GatewayOpenAIWSSchedulerScoreWeights{TTFT: 1}
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{cfg: cfg}, stats: newOpenAIAccountRuntimeStats()}
	accounts := []*Account{{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"public-model": "supplier-a"}}}, {ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"public-model": "supplier-b"}}}}
	slow, fast := 3000, 200
	scheduler.stats.reportModel(1, "supplier-a", true, &slow, time.Now())
	scheduler.stats.reportModel(2, "supplier-b", true, &fast, time.Now())
	plan := scheduler.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{RequestedModel: "public-model"}, accounts, nil)
	require.Equal(t, int64(2), plan.selectionOrder[0].account.ID)
}

func TestOpenAILatencyAwareChatStickyEscapePreservesOwners(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	slowAccount := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 2}
	fastAccount := slowAccount
	fastAccount.ID = 2
	cache := &upstreamCostTrackingConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{1: {AccountID: 1}, 2: {AccountID: 2}}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIScheduler.LatencyAwareEnabled = true
	cfg.Gateway.OpenAIScheduler.StickyEscapeEnabled = true
	cfg.Gateway.OpenAIWS.LBTopK = 2
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights = config.GatewayOpenAIWSSchedulerScoreWeights{Load: 1, TTFT: 1}
	svc := &OpenAIGatewayService{cfg: cfg, accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{slowAccount, fastAccount}}, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"), concurrencyService: NewConcurrencyService(cache)}
	slow, fast := 3000, 200
	svc.ReportOpenAIAccountScheduleResult(&slowAccount, "gpt-test", true, &slow)
	svc.ReportOpenAIAccountScheduleResult(&fastAccount, "gpt-test", true, &fast)
	scheduler := svc.getOpenAIAccountScheduler(context.Background()).(*defaultOpenAIAccountScheduler)
	req := OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, RequestedModel: "gpt-test", RequiredCapability: OpenAIEndpointCapabilityChatCompletions, RequiredTransport: OpenAIUpstreamTransportHTTPSSE, SessionHash: "same-chat"}
	selection := scheduler.tryLatencyAwareChatStickyEscape(context.Background(), req, &slowAccount)
	require.NotNil(t, selection)
	require.Equal(t, fastAccount.ID, selection.Account.ID)
	selection.ReleaseFunc()
	for _, protect := range []string{"owner", "guardian", "previous", "responses", "websocket"} {
		owned := req
		switch protect {
		case "owner":
			owned.DisableStickyEscape = true
		case "guardian":
			owned.PreserveStickyBinding = true
		case "previous":
			owned.PreviousResponseID = "resp_owner"
		case "responses":
			owned.RequiredCapability = OpenAIEndpointCapabilityResponses
		case "websocket":
			owned.RequiredTransport = OpenAIUpstreamTransportResponsesWebsocketV2Ingress
		}
		before := cache.totalAcquires()
		require.Nil(t, scheduler.tryLatencyAwareChatStickyEscape(context.Background(), owned, &slowAccount), protect)
		require.Equal(t, before, cache.totalAcquires(), protect)
	}
	// An alternative that merely produced a fast failure must not win on latency.
	for i := 0; i < 10; i++ {
		svc.ReportOpenAIAccountScheduleResult(&fastAccount, "gpt-test", false, &fast)
	}
	require.Nil(t, scheduler.tryLatencyAwareChatStickyEscape(context.Background(), req, &slowAccount))
}

func TestOpenAIModelQualityRapidRecoveryAfterSpike(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	slow, fast := 29500, 1800
	now := time.Now()
	stats.reportModel(1, "model", true, &slow, now)
	stats.reportModel(1, "model", true, &fast, now.Add(time.Second))
	_, ttft, ok := stats.snapshotModel(1, "model", now.Add(time.Second))
	require.True(t, ok)
	require.Equal(t, 1800.0, ttft)
}

func TestOpenAILatencyAwareExplorationDoesNotProbeUnhealthyOrCostConstrained(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	cfg := &config.Config{}
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{cfg: cfg}}
	pool := []openAIAccountCandidateScore{{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}, score: 3}, {account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}, score: 2, errorRate: .2}}
	for i := 0; i < 100; i++ {
		req := OpenAIAccountScheduleRequest{SessionHash: fmt.Sprint("probe-", i)}
		ordered := scheduler.orderLatencyAwareCandidates(append([]openAIAccountCandidateScore(nil), pool...), req)
		require.Equal(t, int64(1), ordered[0].account.ID)
	}
	pool[1].errorRate = 0
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.UpstreamCost = 1
	for i := 0; i < 100; i++ {
		req := OpenAIAccountScheduleRequest{SessionHash: fmt.Sprint("probe-", i), UseUpstreamTokenCost: true}
		ordered := scheduler.orderLatencyAwareCandidates(append([]openAIAccountCandidateScore(nil), pool...), req)
		require.Equal(t, int64(1), ordered[0].account.ID)
	}
}
