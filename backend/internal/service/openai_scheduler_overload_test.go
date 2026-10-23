package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func overloadTestError(svc *OpenAIGatewayService, account *Account) *UpstreamFailoverError {
	return svc.newOpenAIAccountFailoverError(account, http.StatusServiceUnavailable, nil, []byte(`{"error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`), "Our servers are currently overloaded. Please try again later.", false, true)
}

func TestOpenAIOverloadDefersFastRelayToHealthyOAuthAcrossTopKAndSeeds(t *testing.T) {
	s := dispatchTestScheduler()
	s.service.cfg.Gateway.OpenAIWS.LBTopK = 1
	accounts := []*Account{{ID: 279, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Priority: 1, Concurrency: 500}, {ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Priority: 1, Concurrency: 10}, {ID: 260, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Priority: 2, Concurrency: 100}}
	fast, slow, pro := 1900, 13300, 5500
	now := time.Now()
	s.stats.reportModel(279, "gpt-6.1-sol", true, &fast, now)
	s.stats.reportModel(11, "gpt-6.1-sol", true, &slow, now)
	s.stats.reportModel(260, "gpt-6.1-sol", true, &pro, now)
	req := OpenAIAccountScheduleRequest{RequestedModel: "gpt-6.1-sol", SessionHash: "original"}
	require.Equal(t, int64(279), s.buildOpenAIAccountLoadPlan(context.Background(), req, accounts, nil).selectionOrder[0].account.ID)
	s.stats.reportModel(279, "gpt-6.1-sol", false, nil, now, true)
	for i := 0; i < 200; i++ {
		req.SessionHash = fmt.Sprint("new-request-", i)
		plan := s.buildOpenAIAccountLoadPlan(context.Background(), req, accounts, nil)
		require.NotEqual(t, int64(279), plan.selectionOrder[0].account.ID, "historical speed must not defeat active overload deferral")
		chosen, release := s.reserveDispatchCandidate(plan.selectionOrder, OpenAIAccountScheduleRequest{RequestedModel: "model"})
		require.NotEqual(t, int64(279), plan.selectionOrder[chosen].account.ID, "dynamic reservation must honor the same deferral")
		release()
	}
	require.Zero(t, s.service.localOpenAIDispatchCount(279))
}

func TestOpenAIOverloadDeferralIsModelScopedBoundedAndRecovers(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	now := time.Now()
	fast := 1500
	stats.reportModel(279, "affected", true, &fast, now)
	stats.reportModel(279, "other", true, &fast, now)
	stats.reportModel(279, "affected", false, nil, now, true)
	require.True(t, stats.modelOverloaded(279, "affected", now.Add(14*time.Second)))
	require.False(t, stats.modelOverloaded(279, "other", now.Add(time.Second)))
	require.False(t, stats.modelOverloaded(279, "affected", now.Add(15*time.Second)))
	// A success already in flight cannot erase the rejection before the backoff ends.
	stats.reportModel(279, "affected", true, &fast, now.Add(time.Second))
	require.True(t, stats.modelOverloaded(279, "affected", now.Add(2*time.Second)))
	stats.reportModel(279, "affected", false, nil, now.Add(2*time.Second), true)
	require.True(t, stats.modelOverloaded(279, "affected", now.Add(31*time.Second)))
	require.False(t, stats.modelOverloaded(279, "affected", now.Add(32*time.Second)))
	stats.reportModel(279, "affected", true, &fast, now.Add(33*time.Second))
	require.False(t, stats.modelOverloaded(279, "affected", now.Add(34*time.Second)))
	stats.reportModel(279, "affected", false, nil, now.Add(35*time.Second), true)
	require.True(t, stats.modelOverloaded(279, "affected", now.Add(94*time.Second)), "one success must not erase the recent overload window")
	require.False(t, stats.modelOverloaded(279, "affected", now.Add(95*time.Second)))
	stats.reportModel(279, "affected", true, &fast, now.Add(100*time.Second))
	stats.reportModel(279, "affected", false, nil, now.Add(101*time.Second), true)
	require.False(t, stats.modelOverloaded(279, "affected", now.Add(116*time.Second)), "a recovered window resets the backoff")
	for i := 0; i < 200; i++ {
		stats.reportModel(279, fmt.Sprint("bounded-", i), false, nil, now.Add(time.Duration(i)*time.Millisecond), true)
	}
	require.LessOrEqual(t, len(stats.loadOrCreate(279).models), openAIModelRuntimeLimit)
}

func TestOpenAIOverloadDeferralDoesNotEmptySingleOrAllDeferredPools(t *testing.T) {
	for _, count := range []int{1, 3} {
		s := dispatchTestScheduler()
		accounts := []*Account{}
		for i := 1; i <= count; i++ {
			a := &Account{ID: int64(i), Concurrency: 10}
			accounts = append(accounts, a)
			s.stats.reportModel(a.ID, "model", false, nil, time.Now(), true)
		}
		plan := s.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{RequestedModel: "model"}, accounts, nil)
		require.Len(t, plan.selectionOrder, count)
		chosen, release := s.reserveDispatchCandidate(plan.selectionOrder, OpenAIAccountScheduleRequest{RequestedModel: "model"})
		require.NotNil(t, plan.selectionOrder[chosen].account)
		release()
	}
}

func TestOpenAIOverloadFeedbackAfterOutputStillUpdatesModelRouting(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	svc := &OpenAIGatewayService{cfg: dispatchTestScheduler().service.cfg, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true")}
	account := &Account{ID: 279, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	svc.ObserveOpenAIAccountHealthFailure(context.Background(), account, overloadTestError(svc, account), "gpt-6.1-sol")
	scheduler := svc.getOpenAIAccountScheduler(context.Background()).(*defaultOpenAIAccountScheduler)
	require.True(t, scheduler.stats.modelOverloaded(account.ID, "gpt-6.1-sol", time.Now()))
	require.Nil(t, account.TempUnschedulableUntil, "soft deferral must not disable the account")
}

func TestOpenAIOverloadGatewayFailuresSwitchPoolsAndOAuth(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: dispatchTestScheduler().service.cfg}
	for _, tc := range []struct {
		name    string
		account *Account
		same    bool
	}{
		{"independent relay", &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, false},
		{"explicit pool", &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}}, false},
		{"oauth", &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure := overloadTestError(svc, tc.account)
			require.Equal(t, tc.same, failure.RetryableOnSameAccount)
			require.True(t, failure.RequestScopedTransient)
			require.True(t, failure.ShouldRetryNextAccount())
		})
	}
}

func TestOpenAIOverloadGatewayFailuresSwitchEvenWithLatencyAwareDisabled(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: dispatchTestScheduler().service.cfg}
	svc.cfg.Gateway.OpenAIScheduler.LatencyAwareEnabled = false
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	require.False(t, overloadTestError(svc, account).RetryableOnSameAccount)
}

func TestOpenAIOverloadAllowsUnboundResponsesEscapeButPreservesOwners(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	first := Account{ID: 279, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 500}
	other := Account{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 10, Credentials: map[string]any{"model_mapping": map[string]any{"model": "model"}}}
	cache := &upstreamCostTrackingConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{279: {AccountID: 279}, 11: {AccountID: 11}}}
	cfg := dispatchTestScheduler().service.cfg
	cfg.Gateway.OpenAIScheduler.StickyEscapeEnabled = true
	svc := &OpenAIGatewayService{cfg: cfg, accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{first, other}}, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"), concurrencyService: NewConcurrencyService(cache)}
	fast, slow := 1900, 13000
	svc.ReportOpenAIAccountScheduleResult(&first, "model", true, &fast)
	svc.ReportOpenAIAccountScheduleResult(&other, "model", true, &slow)
	svc.ReportOpenAIAccountScheduleResult(&first, "model", false, nil, overloadTestError(svc, &first))
	scheduler := svc.getOpenAIAccountScheduler(context.Background()).(*defaultOpenAIAccountScheduler)
	req := OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, RequestedModel: "model", RequiredCapability: OpenAIEndpointCapabilityResponses, RequiredTransport: OpenAIUpstreamTransportHTTPSSE, SessionHash: "session"}
	selection := scheduler.tryLatencyAwareChatStickyEscape(context.Background(), req, &first)
	require.NotNil(t, selection)
	require.Equal(t, int64(11), selection.Account.ID)
	selection.ReleaseFunc()
	for _, mode := range []string{"previous", "guardian", "preserved", "disabled", "websocket"} {
		protected := req
		switch mode {
		case "previous":
			protected.PreviousResponseID = "resp-owned"
		case "guardian":
			protected.GuardianParentAccountID = 279
		case "preserved":
			protected.PreserveStickyBinding = true
		case "disabled":
			protected.DisableStickyEscape = true
		case "websocket":
			protected.RequiredTransport = OpenAIUpstreamTransportResponsesWebsocketV2Ingress
		}
		require.Nil(t, scheduler.tryLatencyAwareChatStickyEscape(context.Background(), protected, &first), mode)
	}
}

func TestOpenAIOverloadConcurrentReportsAndSelection(t *testing.T) {
	s := dispatchTestScheduler()
	accounts := []*Account{{ID: 279, Concurrency: 500}, {ID: 11, Concurrency: 10}}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Repository results are request-scoped; Account's mapping memo is not shared.
			localAccounts := make([]*Account, len(accounts))
			for n, original := range accounts {
				copy := *original
				localAccounts[n] = &copy
			}
			for j := 0; j < 30; j++ {
				now := time.Now()
				s.stats.reportModel(279, "model", false, nil, now, true)
				s.stats.reportModel(11, "model", true, nil, now)
				plan := s.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{RequestedModel: "model", SessionHash: fmt.Sprint(i, j)}, localAccounts, nil)
				chosen, release := s.reserveDispatchCandidate(plan.selectionOrder, OpenAIAccountScheduleRequest{RequestedModel: "model"})
				if plan.selectionOrder[chosen].account.ID != 11 {
					t.Errorf("deferred relay selected despite healthy OAuth")
				}
				release()
			}
		}(i)
	}
	wg.Wait()
	require.Zero(t, s.service.localOpenAIDispatchCount(279))
	require.Zero(t, s.service.localOpenAIDispatchCount(11))
}

func TestOpenAIOverloadPostOutputFeedbackPreservesClientAndCredentialAttribution(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	for _, mode := range []string{"cancelled", "deadline", "provider-credential", "request-credential"} {
		svc := &OpenAIGatewayService{cfg: dispatchTestScheduler().service.cfg, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true")}
		account := &Account{ID: 279, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
		var failure error = overloadTestError(svc, account)
		switch mode {
		case "cancelled":
			failure = errors.Join(context.Canceled, failure)
		case "deadline":
			failure = errors.Join(context.DeadlineExceeded, failure)
		case "provider-credential":
			failure = &UpstreamFailoverError{Stage: GatewayFailureStageAccountAuth, Scope: GatewayFailureScopeProvider}
		case "request-credential":
			failure = &UpstreamFailoverError{Stage: GatewayFailureStageAccountAuth, Scope: GatewayFailureScopeRequest}
		}
		svc.ObserveOpenAIAccountHealthFailure(context.Background(), account, failure, "model")
		scheduler := svc.getOpenAIAccountScheduler(context.Background()).(*defaultOpenAIAccountScheduler)
		errorRate, _, known := scheduler.runtimeSnapshot(account.ID, "model")
		require.False(t, known, mode)
		require.Zero(t, errorRate, mode)
		require.False(t, scheduler.stats.modelOverloaded(account.ID, "model", time.Now()), mode)
	}
}

func TestOpenAIOverloadFullSelectionMovesFromRelayToEligibleOAuth(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	cfg := dispatchTestScheduler().service.cfg
	cfg.Gateway.OpenAIWS.LBTopK = 1
	accounts := []Account{
		{ID: 279, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 500, Priority: 1},
		{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 10, Priority: 1, Credentials: map[string]any{"model_mapping": map[string]any{"model": "model"}}},
	}
	cache := &dispatchBurstCache{active: map[int64]int{}, peak: map[int64]int{}}
	svc := &OpenAIGatewayService{cfg: cfg, accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"), concurrencyService: NewConcurrencyService(cache)}
	fast, slow := 1900, 13000
	svc.ReportOpenAIAccountScheduleResult(&accounts[0], "model", true, &fast)
	svc.ReportOpenAIAccountScheduleResult(&accounts[1], "model", true, &slow)
	group := int64(4)
	// OAuth is temporarily excluded for the first attempt, then becomes eligible.
	first, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", "", "model", map[int64]struct{}{11: {}}, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.True(t, first.Acquired)
	require.Equal(t, int64(279), first.Account.ID)
	first.ReleaseFunc()
	failure := overloadTestError(svc, first.Account)
	require.False(t, failure.RetryableOnSameAccount)
	svc.ReportOpenAIAccountScheduleResult(first.Account, "model", false, nil, failure)
	second, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", "", "model", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, second)
	require.True(t, second.Acquired)
	require.Equal(t, int64(11), second.Account.ID)
	second.ReleaseFunc()
	require.Zero(t, cache.active[279])
	require.Zero(t, cache.active[11])
	require.Equal(t, 500, accounts[0].Concurrency)
	require.Equal(t, 10, accounts[1].Concurrency)
}
