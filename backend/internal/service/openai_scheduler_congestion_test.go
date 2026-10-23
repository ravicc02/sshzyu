package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAICongestion503IsModelScopedWithoutAccountPenalty(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	for _, postOutput := range []bool{false, true} {
		name := "before-output"
		if postOutput {
			name = "after-output"
		}
		t.Run(name, func(t *testing.T) {
			repo := &accountHealthTestRepo{}
			svc := &OpenAIGatewayService{cfg: dispatchTestScheduler().service.cfg, accountRepo: repo, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true")}
			account := &Account{ID: 279, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
			fast := 1500
			svc.ReportOpenAIAccountScheduleResult(account, "busy-model", true, &fast)
			svc.ReportOpenAIAccountScheduleResult(account, "healthy-model", true, &fast)
			for i := 0; i < 20; i++ {
				failure := &UpstreamFailoverError{StatusCode: 503, RetryableOnSameAccount: true}
				if postOutput {
					require.False(t, svc.ObserveOpenAIAccountHealthFailure(context.Background(), account, failure, "busy-model"))
				} else {
					require.False(t, svc.ReportOpenAIAccountScheduleResult(account, "busy-model", false, nil, failure))
				}
				require.False(t, failure.RetryableOnSameAccount)
			}
			scheduler := svc.getOpenAIAccountScheduler(context.Background()).(*defaultOpenAIAccountScheduler)
			require.True(t, scheduler.stats.modelOverloaded(account.ID, "busy-model", time.Now()))
			require.False(t, scheduler.stats.modelOverloaded(account.ID, "healthy-model", time.Now()))
			globalError, _, _ := scheduler.stats.snapshot(account.ID)
			require.Zero(t, globalError, "congestion must not contaminate account-wide error history")
			otherError, _, _ := scheduler.runtimeSnapshot(account.ID, "healthy-model")
			require.Zero(t, otherError)
			require.Zero(t, repo.calls)
			require.Empty(t, svc.getOpenAIAccountHealthCircuit().entries)
			require.False(t, svc.isOpenAIAccountRequestRuntimeBlocked(account, "busy-model"))
			require.False(t, svc.isOpenAIAccountRequestRuntimeBlocked(account, "healthy-model"))
		})
	}
}

func TestOpenAICongestionDoesNotFeedEitherAccountBreaker(t *testing.T) {
	for _, failure := range []error{
		&UpstreamFailoverError{StatusCode: 503},
		&OpenAIImagesUpstreamError{StatusCode: 503},
		overloadTestError(&OpenAIGatewayService{}, &Account{}),
		newOpenAIUpstreamFailoverError(502, nil, []byte(`{"error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded"}}`), "Our servers are currently overloaded", false),
	} {
		_, eligible := classifyOpenAIAccountHealthFailure(failure)
		require.False(t, eligible)
		encoded, err := json.Marshal(OpenAIAPIKeyHealthBreakerSettings{Enabled: true, WindowMinutes: 2, FailureThreshold: 2, CooldownMinutes: 5})
		require.NoError(t, err)
		repo := &openAIAPIKeyHealthAccountRepo{}
		cache := &openAIAPIKeyHealthCacheStub{tripped: true}
		legacy := NewRateLimitService(repo, nil, &config.Config{}, nil, cache)
		legacy.SetSettingService(NewSettingService(&openAIAPIKeyHealthSettingRepo{value: string(encoded)}, &config.Config{}))
		legacy.SetOpenAIAPIKeyHealthCache(cache)
		for i := 0; i < 20; i++ {
			require.False(t, legacy.ObserveOpenAIAPIKeyHealthFailure(context.Background(), openAIHealthPoolAccount(), failure))
		}
		require.Zero(t, cache.recordCalls)
		require.Zero(t, cache.setCalls)
		require.Zero(t, repo.setCalls)
	}
}

func TestOpenAICongestionDoesNotAccumulateOrResetHardFailureWindows(t *testing.T) {
	c := (&OpenAIGatewayService{}).getOpenAIAccountHealthCircuit()
	now := time.Now()
	trip, _, _, _ := c.record(279, 502, now)
	require.False(t, trip)
	for i := 1; i <= 20; i++ {
		trip, _, _, _ = c.record(279, 503, now.Add(time.Duration(i)*time.Second))
		require.False(t, trip)
	}
	trip, until, count, threshold := c.record(279, 502, now.Add(30*time.Second))
	require.True(t, trip)
	require.Equal(t, 2, count)
	require.Equal(t, 2, threshold)
	require.Equal(t, now.Add(330*time.Second), until)
	c.record(280, 500, now)
	for i := 1; i <= 20; i++ {
		trip, _, _, _ = c.record(280, 503, now.Add(time.Duration(i)*time.Second))
		require.False(t, trip)
	}
	trip, _, _, _ = c.record(280, 504, now.Add(25*time.Second))
	require.False(t, trip)
	trip, _, count, threshold = c.record(280, 500, now.Add(30*time.Second))
	require.True(t, trip)
	require.Equal(t, 3, count)
	require.Equal(t, 3, threshold)
}

func TestOpenAICongestion503DoesNotUseLegacyHardModelCooldown(t *testing.T) {
	svc := &OpenAIGatewayService{rateLimitService: NewRateLimitService(transientCooldownAccountRepo{}, nil, &config.Config{}, nil, nil)}
	account := &Account{ID: 279, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	for i := 0; i < 20; i++ {
		require.False(t, svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusServiceUnavailable, nil, []byte(`{"error":{"message":"Service unavailable"}}`), "busy-model"))
	}
	require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "busy-model"))
	require.False(t, svc.isOpenAIAccountRequestRuntimeBlocked(account, "healthy-model"))
}

func TestOpenAICongestionAllBusyCandidatesRemainUsable(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	accounts := []Account{
		{ID: 279, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 500, Priority: 1},
		{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 10, Priority: 1, Credentials: map[string]any{"model_mapping": map[string]any{"model": "model"}}},
	}
	repo := &accountHealthTestRepo{schedulerTestOpenAIAccountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}}
	cache := &dispatchBurstCache{active: map[int64]int{}, peak: map[int64]int{}}
	svc := &OpenAIGatewayService{cfg: dispatchTestScheduler().service.cfg, accountRepo: repo, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"), concurrencyService: NewConcurrencyService(cache)}
	svc.cfg.Gateway.OpenAIWS.LBTopK = 1
	for i := range accounts {
		for n := 0; n < 10; n++ {
			require.False(t, svc.ReportOpenAIAccountScheduleResult(&accounts[i], "model", false, nil, &UpstreamFailoverError{StatusCode: 503}))
		}
	}
	group := int64(4)
	selected, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", "", "model", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.True(t, selected.Acquired)
	if selected.ReleaseFunc != nil {
		selected.ReleaseFunc()
	}
	require.Zero(t, repo.calls)
	require.Zero(t, cache.active[279])
	require.Zero(t, cache.active[11])
}

func TestOpenAICongestionSlowSuccessDoesNotTripHealth(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	svc := &OpenAIGatewayService{cfg: dispatchTestScheduler().service.cfg, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true")}
	account := &Account{ID: 279, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	slow := 20000
	for i := 0; i < 20; i++ {
		require.False(t, svc.ReportOpenAIAccountScheduleResult(account, "model", true, &slow))
	}
	scheduler := svc.getOpenAIAccountScheduler(context.Background()).(*defaultOpenAIAccountScheduler)
	rate, ttft, known := scheduler.runtimeSnapshot(account.ID, "model")
	require.True(t, known)
	require.Zero(t, rate)
	require.InDelta(t, float64(slow), ttft, 1)
	require.False(t, scheduler.stats.modelOverloaded(account.ID, "model", time.Now()))
	require.False(t, svc.isOpenAIAccountRequestRuntimeBlocked(account, "model"))
}
