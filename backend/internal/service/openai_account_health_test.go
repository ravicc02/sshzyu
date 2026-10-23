package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type accountHealthTestRepo struct {
	schedulerTestOpenAIAccountRepo
	calls      int
	until      time.Time
	reason     string
	persistErr error
	applyState bool
}

func (r *accountHealthTestRepo) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	r.calls++
	r.until, r.reason = until, reason
	if r.applyState && r.persistErr == nil {
		for i := range r.accounts {
			if r.accounts[i].ID == id {
				r.accounts[i].TempUnschedulableUntil = &until
				r.accounts[i].TempUnschedulableReason = reason
			}
		}
	}
	return r.persistErr
}

func TestOpenAIAccountHealthRollingWindowsAndExactCooldown(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c := svc.getOpenAIAccountHealthCircuit()
	now := time.Now()
	trip, _, _, _ := c.record(279, 502, now)
	require.False(t, trip)
	trip, until, count, threshold := c.record(279, 502, now.Add(90*time.Second))
	require.True(t, trip)
	require.Equal(t, 2, count)
	require.Equal(t, 2, threshold)
	require.Equal(t, now.Add(90*time.Second+5*time.Minute), until)
	require.True(t, c.blocked(279, until.Add(-time.Nanosecond)))
	trip, again, _, _ := c.record(279, 502, until.Add(-time.Second))
	require.False(t, trip)
	require.Equal(t, until, again, "late in-flight failures must not prolong cooldown")
	require.False(t, c.blocked(279, until))
	trip, _, _, _ = c.record(279, 502, until)
	require.False(t, trip, "expiry starts fresh window")
	trip, _, _, _ = c.record(280, 502, now)
	require.False(t, trip)
	trip, _, count, _ = c.record(280, 502, now.Add(openAIAccountHealthWindow))
	require.False(t, trip, "boundary observation is expired")
	require.Equal(t, 1, count)
	for i, status := range []int{500, 504, 500} {
		trip, _, _, threshold = c.record(281, status, now.Add(time.Duration(i)*time.Second))
		require.Equal(t, i == 2, trip)
	}
	require.Equal(t, 3, threshold)
}

func TestOpenAIAccountHealthClassifiesOnlyAttributableServerFailures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		eligible bool
	}{
		{"generic 502", &UpstreamFailoverError{StatusCode: 502}, true},
		{"generic 503", &UpstreamFailoverError{StatusCode: 503}, false},
		{"generic 500", &UpstreamFailoverError{StatusCode: 500}, true},
		{"overload", overloadTestError(&OpenAIGatewayService{}, &Account{}), false},
		{"image 503", &OpenAIImagesUpstreamError{StatusCode: 503}, false},
		{"bad request", &UpstreamFailoverError{StatusCode: 400}, false},
		{"quota", &UpstreamFailoverError{StatusCode: 429}, false},
		{"permission", &UpstreamFailoverError{StatusCode: 403}, false},
		{"provider", &UpstreamFailoverError{StatusCode: 503, Scope: GatewayFailureScopeProvider}, false},
		{"request", &UpstreamFailoverError{StatusCode: 503, Scope: GatewayFailureScopeRequest}, false},
		{"transient payload", &UpstreamFailoverError{StatusCode: 503, RequestScopedTransient: true}, false},
		{"auth", &UpstreamFailoverError{StatusCode: 503, Stage: GatewayFailureStageAccountAuth}, false},
		{"cancel", errors.Join(context.Canceled, &UpstreamFailoverError{StatusCode: 502}), false},
		{"deadline", errors.Join(context.DeadlineExceeded, &UpstreamFailoverError{StatusCode: 503}), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, eligible := classifyOpenAIAccountHealthFailure(tc.err)
			require.Equal(t, tc.eligible, eligible)
		})
	}
}

func TestOpenAIAccountHealthSuccessCannotResetFailuresOrActiveCooldown(t *testing.T) {
	for _, kind := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
		t.Run(kind, func(t *testing.T) {
			repo := &accountHealthTestRepo{}
			svc := &OpenAIGatewayService{accountRepo: repo}
			account := &Account{ID: 279, Platform: PlatformOpenAI, Type: kind}
			require.False(t, svc.ReportOpenAIAccountScheduleResult(account, "model-a", false, nil, &UpstreamFailoverError{StatusCode: 502}))
			svc.ReportOpenAIAccountScheduleResult(account, "model-a", true, nil)
			before := time.Now()
			failure := &UpstreamFailoverError{StatusCode: 502, RetryableOnSameAccount: true}
			require.True(t, svc.ReportOpenAIAccountScheduleResult(account, "model-b", false, nil, failure))
			require.False(t, failure.RetryableOnSameAccount)
			require.WithinDuration(t, before.Add(5*time.Minute), repo.until, time.Second)
			require.Equal(t, 1, repo.calls)
			var state TempUnschedState
			require.NoError(t, json.Unmarshal([]byte(repo.reason), &state))
			require.Equal(t, openAIAccountHealthReason, state.MatchedKeyword)
			require.Equal(t, int64(2), state.TriggerCount)
			svc.ReportOpenAIAccountScheduleResult(account, "model-c", true, nil)
			require.True(t, svc.isOpenAIAccountRequestRuntimeBlocked(account, "unrelated-model"), "cross-model cooldown survives success and stale account objects")
			require.Nil(t, svc.recheckSelectedOpenAIAccountFromDBBeforeProfit(context.Background(), account, nil, PlatformOpenAI, "model", false, OpenAIEndpointCapabilityResponses))
			require.Equal(t, 1, repo.calls)
		})
	}
}

func TestOpenAIAccountHealthPersistenceFailureStillBlocksStaleSnapshots(t *testing.T) {
	repo := &accountHealthTestRepo{persistErr: errors.New("database unavailable")}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 279, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	svc.ObserveOpenAIAccountHealthFailure(context.Background(), account, &UpstreamFailoverError{StatusCode: 502})
	require.True(t, svc.ObserveOpenAIAccountHealthFailure(context.Background(), account, &UpstreamFailoverError{StatusCode: 502}))
	require.Equal(t, 1, repo.calls)
	stale := *account
	require.Nil(t, stale.TempUnschedulableUntil)
	require.True(t, svc.isOpenAIAccountRequestRuntimeBlocked(&stale, "different-model"))
}

func TestOpenAIAccountHealthFullSelectionNeverFallsBackToCooledAccount(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	for _, advanced := range []string{"true", "false"} {
		t.Run("advanced="+advanced, func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			cfg := dispatchTestScheduler().service.cfg
			cfg.Gateway.OpenAIWS.LBTopK = 1
			accounts := []Account{
				{ID: 279, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 500, Priority: 1},
				{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 10, Priority: 10, Credentials: map[string]any{"model_mapping": map[string]any{"model": "model"}}},
			}
			repo := &accountHealthTestRepo{schedulerTestOpenAIAccountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}}
			cache := &dispatchBurstCache{active: map[int64]int{}, peak: map[int64]int{}}
			svc := &OpenAIGatewayService{cfg: cfg, accountRepo: repo, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService(advanced), concurrencyService: NewConcurrencyService(cache)}
			svc.ReportOpenAIAccountScheduleResult(&accounts[0], "model", false, nil, &UpstreamFailoverError{StatusCode: 502})
			svc.ReportOpenAIAccountScheduleResult(&accounts[0], "another-model", false, nil, &UpstreamFailoverError{StatusCode: 502})
			group := int64(4)
			for i := 0; i < 8; i++ {
				selected, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", "", "model", nil, OpenAIUpstreamTransportAny, false)
				require.NoError(t, err)
				require.NotNil(t, selected)
				require.Equal(t, int64(11), selected.Account.ID)
				if selected.ReleaseFunc != nil {
					selected.ReleaseFunc()
				}
			}
			svc.ReportOpenAIAccountScheduleResult(&accounts[1], "model", false, nil, &UpstreamFailoverError{StatusCode: 502})
			svc.ReportOpenAIAccountScheduleResult(&accounts[1], "model", false, nil, &UpstreamFailoverError{StatusCode: 502})
			selected, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", "", "model", nil, OpenAIUpstreamTransportAny, false)
			require.Error(t, err)
			require.Nil(t, selected, "all-cooled pool must not fail open")
			require.Zero(t, cache.active[279])
			require.Zero(t, cache.active[11])
			// Expire only the circuit clocks; the repository intentionally supplies
			// stale empty cooldown fields in this scenario.
			circuit := svc.getOpenAIAccountHealthCircuit()
			circuit.mu.Lock()
			for id, entry := range circuit.entries {
				entry.blockedUntil = time.Now().Add(-time.Second)
				circuit.entries[id] = entry
			}
			circuit.mu.Unlock()
			selected, _, err = svc.SelectAccountWithScheduler(context.Background(), &group, "", "", "model", nil, OpenAIUpstreamTransportAny, false)
			require.NoError(t, err)
			require.NotNil(t, selected, "expired cooldowns become eligible again")
			if selected.ReleaseFunc != nil {
				selected.ReleaseFunc()
			}
		})
	}
}

func TestOpenAIAccountHealthGatewayRetryPolicyDoesNotAlterQuotaOrPayloadPolicy(t *testing.T) {
	svc := &OpenAIGatewayService{}
	for _, kind := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		account := &Account{ID: 42, Platform: PlatformOpenAI, Type: kind, Credentials: map[string]any{"pool_mode": true}}
		for _, status := range []int{502, 503} {
			failure := svc.newOpenAIAccountFailoverError(account, status, nil, nil, "bad gateway", false, true)
			require.False(t, failure.RetryableOnSameAccount)
			require.True(t, failure.ShouldRetryNextAccount())
		}
		failure := svc.newOpenAIAccountFailoverError(account, 400, nil, nil, "bad payload", false, true)
		require.True(t, failure.RetryableOnSameAccount)
	}
}

func TestOpenAIAccountHealthConcurrentReportsAndBoundedMemory(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c := svc.getOpenAIAccountHealthCircuit()
	now := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			for n := 0; n < 50; n++ {
				c.record(id, 502, now)
				c.blocked(id, now)
			}
		}(int64(i + 1))
	}
	wg.Wait()
	for i := 1; i <= 16; i++ {
		require.True(t, c.blocked(int64(i), now))
	}
	// Fill with active blocks; saturated unknown accounts fail closed without
	// dropping existing blocks or allocating an unbounded map.
	for id := int64(17); id <= openAIAccountHealthMaxEntries; id++ {
		c.record(id, 502, now)
		c.record(id, 502, now)
	}
	trip, until, _, _ := c.record(openAIAccountHealthMaxEntries+1, 502, now)
	require.False(t, trip)
	require.Equal(t, now.Add(5*time.Minute), until)
	require.True(t, c.blocked(openAIAccountHealthMaxEntries+1, now))
	require.Len(t, c.entries, openAIAccountHealthMaxEntries)
	require.False(t, c.blocked(openAIAccountHealthMaxEntries+1, now.Add(5*time.Minute)))
	t.Logf("bounded to %d account records", len(c.entries))
}

// Simulate a new gateway process reading the persisted account state.
func TestOpenAIAccountHealthPersistedCooldownSurvivesGatewayRecreation(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	cfg := dispatchTestScheduler().service.cfg
	repo := &accountHealthTestRepo{applyState: true, schedulerTestOpenAIAccountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{{ID: 279, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 10}}}}
	svc := &OpenAIGatewayService{cfg: cfg, accountRepo: repo}
	account, _ := repo.GetByID(context.Background(), 279)
	svc.ReportOpenAIAccountScheduleResult(account, "model", false, nil, &UpstreamFailoverError{StatusCode: 502})
	require.True(t, svc.ReportOpenAIAccountScheduleResult(account, "model", false, nil, &UpstreamFailoverError{StatusCode: 502}))
	restarted := &OpenAIGatewayService{cfg: cfg, accountRepo: repo}
	group := int64(4)
	selected, _, err := restarted.SelectAccountWithScheduler(context.Background(), &group, "", "", "model", nil, OpenAIUpstreamTransportAny, false)
	require.Error(t, err)
	require.Nil(t, selected)
}

type healthStateTestCache struct {
	TempUnschedCache
	state  *TempUnschedState
	writes int
}

func (c *healthStateTestCache) GetTempUnsched(context.Context, int64) (*TempUnschedState, error) {
	return c.state, nil
}
func (c *healthStateTestCache) SetTempUnsched(_ context.Context, _ int64, state *TempUnschedState) error {
	c.state = state
	c.writes++
	return nil
}
func TestOpenAIAccountHealthSharesCooldownWithoutShorteningLongerCacheState(t *testing.T) {
	for _, longer := range []bool{false, true} {
		t.Run(fmtBoolForHealthTest(longer), func(t *testing.T) {
			repo := &accountHealthTestRepo{}
			cache := &healthStateTestCache{}
			if longer {
				cache.state = &TempUnschedState{UntilUnix: time.Now().Add(time.Hour).Unix()}
			}
			svc := &OpenAIGatewayService{accountRepo: repo, rateLimitService: &RateLimitService{accountRepo: repo, tempUnschedCache: cache}}
			account := &Account{ID: 279, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
			svc.ObserveOpenAIAccountHealthFailure(context.Background(), account, &UpstreamFailoverError{StatusCode: 502})
			require.True(t, svc.ObserveOpenAIAccountHealthFailure(context.Background(), account, &UpstreamFailoverError{StatusCode: 502}))
			require.Equal(t, 1, repo.calls)
			if longer {
				require.Zero(t, cache.writes)
				require.Greater(t, cache.state.UntilUnix, time.Now().Add(59*time.Minute).Unix())
			} else {
				require.Equal(t, 1, cache.writes)
				require.Equal(t, openAIAccountHealthReason, cache.state.MatchedKeyword)
			}
		})
	}
}
func fmtBoolForHealthTest(value bool) string {
	if value {
		return "longer-existing-cooldown"
	}
	return "new-cooldown"
}
