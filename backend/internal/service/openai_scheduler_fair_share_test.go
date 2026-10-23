package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func fairShareAccounts() []Account {
	return []Account{
		{ID: 260, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Priority: 2, Concurrency: 100, GroupIDs: []int64{4}, Credentials: map[string]any{"model_mapping": map[string]any{"model": "model"}}},
		{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Priority: 1, Concurrency: 10, GroupIDs: []int64{4}, Credentials: map[string]any{"model_mapping": map[string]any{"model": "model"}}},
		{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Priority: 2, Concurrency: 10, GroupIDs: []int64{4}, Credentials: map[string]any{"model_mapping": map[string]any{"model": "model"}}},
	}
}
func fairSharePlan(s *defaultOpenAIAccountScheduler, req OpenAIAccountScheduleRequest, accounts []Account, loads map[int64]*AccountLoadInfo) openAIAccountLoadPlan {
	ptrs := make([]*Account, len(accounts))
	for i := range accounts {
		a := accounts[i]
		ptrs[i] = &a
	}
	return s.buildOpenAIAccountLoadPlan(context.Background(), req, ptrs, loads)
}
func seedFairShareQuality(s *defaultOpenAIAccountScheduler) {
	for id, ms := range map[int64]int{260: 5000, 11: 15000, 7: 6500} {
		value := ms
		s.stats.reportModel(id, "model", true, &value, time.Now())
	}
}

func TestOpenAIFairShareThreeHealthyAccountsReceiveNewSessions(t *testing.T) {
	s := dispatchTestScheduler()
	s.service.cfg.Gateway.OpenAIWS.LBTopK = 1
	s.service.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = .5
	accounts := fairShareAccounts()
	seedFairShareQuality(s)
	counts := map[int64]int{}
	for i := 0; i < 3000; i++ {
		req := OpenAIAccountScheduleRequest{RequestedModel: "model", SessionHash: fmt.Sprintf("new-user-%d", i)}
		plan := fairSharePlan(s, req, accounts, nil)
		index, release := s.reserveDispatchCandidate(plan.selectionOrder, req)
		counts[plan.selectionOrder[index].account.ID]++
		release()
	}
	for _, a := range accounts {
		require.Greater(t, counts[a.ID], 650, "a slower healthy account must receive a meaningful share, not just probes")
		require.Less(t, counts[a.ID], 1400)
		require.Zero(t, s.service.localOpenAIDispatchCount(a.ID))
	}
	raw, _ := json.Marshal(counts)
	t.Logf("3000 new sessions, hard caps 100/10/10, TTFT 5s/15s/6.5s, TopK=1: %s", raw)
	require.Equal(t, 100, accounts[0].Concurrency)
}

func TestOpenAIFairShareMovesNewTrafficAwayFrom83Of100HotAccount(t *testing.T) {
	s := dispatchTestScheduler()
	accounts := fairShareAccounts()
	seedFairShareQuality(s)
	loads := map[int64]*AccountLoadInfo{260: {CurrentConcurrency: 83, LoadRate: 83}, 11: {}, 7: {}}
	counts := map[int64]int{}
	for i := 0; i < 400; i++ {
		req := OpenAIAccountScheduleRequest{RequestedModel: "model", SessionHash: fmt.Sprint("hot-", i)}
		p := fairSharePlan(s, req, accounts, loads)
		index, release := s.reserveDispatchCandidate(p.selectionOrder, req)
		counts[p.selectionOrder[index].account.ID]++
		release()
	}
	require.Zero(t, counts[260])
	require.Greater(t, counts[11], 80)
	require.Greater(t, counts[7], 80)
}

func TestOpenAIFairShareDoesNotOverrideCostOwnershipOrHealth(t *testing.T) {
	s := dispatchTestScheduler()
	accounts := fairShareAccounts()
	seedFairShareQuality(s)
	req := OpenAIAccountScheduleRequest{RequestedModel: "model", SessionHash: "safe"}
	p := fairSharePlan(s, req, accounts, nil)
	for _, mode := range []string{"previous", "guardian", "disabled", "preserved", "cost", "grok"} {
		guarded := req
		switch mode {
		case "previous":
			guarded.PreviousResponseID = "resp_owner"
		case "guardian":
			guarded.GuardianParentAccountID = 260
		case "disabled":
			guarded.DisableStickyEscape = true
		case "preserved":
			guarded.PreserveStickyBinding = true
		case "cost":
			guarded.UseUpstreamTokenCost = true
			s.service.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.UpstreamCost = 1
		case "grok":
			guarded.Platform = PlatformGrok
		}
		guardedPlan := fairSharePlan(s, guarded, accounts, nil)
		_, ok := s.fairShareDispatchCandidate(guardedPlan.selectionOrder, guarded)
		require.False(t, ok, mode)
	}
	s.stats.report(11, false, nil) // an attributable failure, unlike model congestion
	s.stats.reportModel(7, "model", false, nil, time.Now(), true)
	for i := 0; i < 100; i++ {
		req.SessionHash = fmt.Sprint(i)
		p = fairSharePlan(s, req, accounts, nil)
		index, release := s.reserveDispatchCandidate(p.selectionOrder, req)
		require.Equal(t, int64(260), p.selectionOrder[index].account.ID)
		release()
	}
}

func TestOpenAIFairShareDoesNotKeepRecoveredCongestionOutForTenMinutes(t *testing.T) {
	s := dispatchTestScheduler()
	accounts := fairShareAccounts()
	seedFairShareQuality(s)
	for i := 0; i < 5; i++ {
		s.stats.reportModel(11, "model", false, nil, time.Now(), true)
	}
	runtime := s.stats.loadOrCreate(11)
	runtime.modelMu.Lock()
	runtime.models["model"].overloadUntil = time.Now().Add(-time.Second)
	runtime.modelMu.Unlock()
	selected := 0
	for i := 0; i < 300; i++ {
		req := OpenAIAccountScheduleRequest{RequestedModel: "model", SessionHash: fmt.Sprint("recovered-", i)}
		p := fairSharePlan(s, req, accounts, nil)
		index, release := s.reserveDispatchCandidate(p.selectionOrder, req)
		if p.selectionOrder[index].account.ID == 11 {
			selected++
		}
		release()
	}
	require.Greater(t, selected, 30, "expired 503 soft avoidance must not become an account-health exclusion")
}

// Repository objects are request-local; Account's model-mapping memo is not shared
// across independent selectors in this concurrency fixture.
type fairShareAccountRepo struct{ schedulerTestOpenAIAccountRepo }

func (r fairShareAccountRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	a, err := r.schedulerTestOpenAIAccountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	copy := *a
	return &copy, nil
}
func (r fairShareAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, group int64, platform string) ([]Account, error) {
	out := []Account{}
	for _, a := range r.accounts {
		if a.Platform == platform && openAIStickyAccountMatchesGroup(&a, &group) {
			out = append(out, a)
		}
	}
	return out, nil
}

type fairShareSessionCache struct {
	GatewayCache
	mu       sync.Mutex
	bindings map[string]int64
}

func (c *fairShareSessionCache) GetSessionAccountID(_ context.Context, group int64, key string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bindings[fmt.Sprint(group, ":", key)], nil
}
func (c *fairShareSessionCache) SetSessionAccountID(_ context.Context, group int64, key string, id int64, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bindings[fmt.Sprint(group, ":", key)] = id
	return nil
}
func (c *fairShareSessionCache) DeleteSessionAccountID(_ context.Context, group int64, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.bindings, fmt.Sprint(group, ":", key))
	return nil
}
func (c *fairShareSessionCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func TestOpenAIFairShareRealSelectionSpreadsKeysAndKeepsEachSessionSticky(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	accounts := fairShareAccounts()
	cfg := dispatchTestScheduler().service.cfg
	cfg.Gateway.OpenAIWS.LBTopK = 1
	slots := &dispatchBurstCache{active: map[int64]int{}, peak: map[int64]int{}}
	sessions := &fairShareSessionCache{bindings: map[string]int64{}}
	svc := &OpenAIGatewayService{cfg: cfg, accountRepo: fairShareAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}}, cache: sessions, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"), concurrencyService: NewConcurrencyService(slots)}
	for i, ms := range []int{5000, 15000, 6500} {
		value := ms
		svc.ReportOpenAIAccountScheduleResult(&accounts[i], "model", true, &value)
	}
	group := int64(4)
	counts := map[int64]int{}
	for user := 0; user < 300; user++ {
		c := fairShareGinContext(int64(user + 1))
		hash := svc.GenerateSessionHash(c, []byte(`{"model":"model","prompt_cache_key":"shared-client-session","input":"same first message"}`))
		selected, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", hash, "model", nil, OpenAIUpstreamTransportAny, false)
		require.NoError(t, err)
		require.NotNil(t, selected)
		id := selected.Account.ID
		counts[id]++
		selected.ReleaseFunc()
		for n := 0; n < 2; n++ {
			next, decision, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", hash, "model", nil, OpenAIUpstreamTransportAny, false)
			require.NoError(t, err)
			require.Equal(t, id, next.Account.ID)
			require.True(t, decision.StickySessionHit)
			next.ReleaseFunc()
		}
	}
	for _, a := range accounts {
		require.Greater(t, counts[a.ID], 45)
		require.Zero(t, slots.active[a.ID])
		require.Zero(t, svc.localOpenAIDispatchCount(a.ID))
	}
	t.Logf("300 API keys sharing client session text, each followed by two sticky turns: %v", counts)
}

func TestOpenAIFairShareOrdinaryResponsesCanEscapeHotStickyButOwnersCannot(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	cfg := dispatchTestScheduler().service.cfg
	cfg.Gateway.OpenAIScheduler.StickyEscapeEnabled = true
	accounts := fairShareAccounts()
	sessions := &fairShareSessionCache{bindings: map[string]int64{}}
	var released []int64
	cache := schedulerTestConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{260: {CurrentConcurrency: 83, LoadRate: 83}, 11: {}, 7: {}}, releasedIDs: &released}
	svc := &OpenAIGatewayService{cfg: cfg, accountRepo: fairShareAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}}, cache: sessions, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"), concurrencyService: NewConcurrencyService(cache)}
	for i, ms := range []int{5000, 15000, 6500} {
		value := ms
		svc.ReportOpenAIAccountScheduleResult(&accounts[i], "model", true, &value)
	}
	group := int64(4)
	hash := "hot-session"
	require.NoError(t, svc.setStickySessionAccountID(context.Background(), &group, hash, 260, time.Hour))
	selected, decision, err := svc.SelectAccountWithSchedulerForCapability(context.Background(), &group, "", hash, "model", nil, OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityResponses, false, false, false)
	require.NoError(t, err)
	require.NotEqual(t, int64(260), selected.Account.ID)
	require.True(t, decision.StickySessionHit)
	rebound, _ := svc.getStickySessionAccountID(context.Background(), &group, hash)
	require.Equal(t, selected.Account.ID, rebound)
	selected.ReleaseFunc()
	scheduler := svc.getOpenAIAccountScheduler(context.Background()).(*defaultOpenAIAccountScheduler)
	for _, mode := range []string{"previous", "guardian", "preserved", "disabled", "socket"} {
		req := OpenAIAccountScheduleRequest{GroupID: &group, Platform: PlatformOpenAI, RequestedModel: "model", SessionHash: hash, RequiredCapability: OpenAIEndpointCapabilityResponses}
		switch mode {
		case "previous":
			req.PreviousResponseID = "resp_owner"
		case "guardian":
			req.GuardianParentAccountID = 260
		case "preserved":
			req.PreserveStickyBinding = true
		case "disabled":
			req.DisableStickyEscape = true
		case "socket":
			req.RequiredTransport = OpenAIUpstreamTransportResponsesWebsocketV2Ingress
		}
		require.Nil(t, scheduler.tryLatencyAwareChatStickyEscape(context.Background(), req, &accounts[0]), mode)
	}
}

// Reflect real acquired slots in load reads, including the probe's own slot.
type fairShareLiveLoadCache struct{ *dispatchBurstCache }

func (c fairShareLiveLoadCache) GetAccountsLoadBatch(_ context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[int64]*AccountLoadInfo{}
	for _, a := range accounts {
		load := 0
		if a.MaxConcurrency > 0 {
			load = c.active[a.ID] * 100 / a.MaxConcurrency
		}
		out[a.ID] = &AccountLoadInfo{AccountID: a.ID, CurrentConcurrency: c.active[a.ID], LoadRate: load}
	}
	return out, nil
}
func TestOpenAIFairShareStickyEscapeHandlesOneSlotAndUnknownLatency(t *testing.T) {
	for _, withHistory := range []bool{false, true} {
		t.Run(fmt.Sprint("history=", withHistory), func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
			cfg := dispatchTestScheduler().service.cfg
			cfg.Gateway.OpenAIScheduler.StickyEscapeEnabled = true
			accounts := fairShareAccounts()
			accounts = accounts[:2]
			accounts[1].Concurrency = 1
			sessions := &fairShareSessionCache{bindings: map[string]int64{}}
			slots := fairShareLiveLoadCache{&dispatchBurstCache{active: map[int64]int{260: 83}, peak: map[int64]int{}}}
			svc := &OpenAIGatewayService{cfg: cfg, accountRepo: fairShareAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}}, cache: sessions, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"), concurrencyService: NewConcurrencyService(slots)}
			if withHistory {
				for i, ms := range []int{5000, 15000} {
					value := ms
					svc.ReportOpenAIAccountScheduleResult(&accounts[i], "model", true, &value)
				}
			}
			group := int64(4)
			hash := "one-slot"
			require.NoError(t, svc.setStickySessionAccountID(context.Background(), &group, hash, 260, time.Hour))
			selected, _, err := svc.SelectAccountWithSchedulerForCapability(context.Background(), &group, "", hash, "model", nil, OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityResponses, false, false, false)
			require.NoError(t, err)
			require.Equal(t, int64(11), selected.Account.ID)
			require.True(t, selected.Acquired)
			selected.ReleaseFunc()
			require.Zero(t, slots.active[11])
			require.Equal(t, 83, slots.active[260])
		})
	}
}
func TestOpenAIFairShareNeverBorrowsOutsideGroupOrInventsCandidates(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	accounts := fairShareAccounts()
	accounts[0].GroupIDs = []int64{4, 67}
	slots := &dispatchBurstCache{active: map[int64]int{}, peak: map[int64]int{}}
	svc := &OpenAIGatewayService{cfg: dispatchTestScheduler().service.cfg, accountRepo: fairShareAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}}, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"), concurrencyService: NewConcurrencyService(slots)}
	group := int64(67)
	for i := 0; i < 20; i++ {
		selected, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", fmt.Sprint("single-", i), "model", nil, OpenAIUpstreamTransportAny, false)
		require.NoError(t, err)
		require.Equal(t, int64(260), selected.Account.ID)
		selected.ReleaseFunc()
	}
	emptyGroup := int64(99)
	selected, _, err := svc.SelectAccountWithScheduler(context.Background(), &emptyGroup, "", "none", "model", nil, OpenAIUpstreamTransportAny, false)
	require.Error(t, err)
	require.Nil(t, selected)
}
