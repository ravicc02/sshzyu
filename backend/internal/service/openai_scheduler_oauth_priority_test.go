package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func oauthPriorityTestService(t *testing.T, accounts []Account, advanced bool) (*OpenAIGatewayService, *dispatchBurstCache, *fairShareSessionCache) {
	t.Helper()
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	cfg := dispatchTestScheduler().service.cfg
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cfg.Gateway.OpenAIWS.LBTopK = 1
	slots := &dispatchBurstCache{active: map[int64]int{}, peak: map[int64]int{}}
	sessions := &fairShareSessionCache{bindings: map[string]int64{}}
	svc := &OpenAIGatewayService{cfg: cfg, accountRepo: fairShareAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}}, cache: sessions, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService(fmt.Sprint(advanced), "", "false"), concurrencyService: NewConcurrencyService(slots)}
	return svc, slots, sessions
}

func oauthPriorityMixedAccounts() []Account {
	accounts := fairShareAccounts()
	relay := accounts[0]
	relay.ID = 279
	relay.Type = AccountTypeAPIKey
	relay.Priority = 0
	relay.Concurrency = 500
	relay.Credentials = map[string]any{"model_mapping": map[string]any{"model": "model"}}
	return append(accounts, relay)
}

func TestOpenAIOAuthPriorityAppliesWithSubscriptionSwitchOff(t *testing.T) {
	for _, advanced := range []bool{true, false} {
		for _, plan := range []string{"", "free", "plus"} {
			t.Run(fmt.Sprintf("advanced=%v/plan=%s", advanced, plan), func(t *testing.T) {
				accounts := oauthPriorityMixedAccounts()
				accounts = []Account{accounts[1], accounts[3]}
				accounts[0].Priority = 99
				accounts[0].Credentials = map[string]any{"plan_type": plan, "model_mapping": map[string]any{"model": "model"}}
				svc, slots, _ := oauthPriorityTestService(t, accounts, advanced)
				svc.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.UpstreamCost = 100
				svc.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 100
				group := int64(4)
				chosen, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", "mixed", "model", nil, OpenAIUpstreamTransportAny, false)
				require.NoError(t, err)
				require.NotNil(t, chosen)
				require.True(t, chosen.Acquired)
				require.Equal(t, AccountTypeOAuth, chosen.Account.Type)
				chosen.ReleaseFunc()
				require.Zero(t, slots.active[11])
				require.Zero(t, slots.peak[279], "API key must not acquire before OAuth")
			})
		}
	}
}

func TestOpenAIOAuthPriorityDistributesInsideOAuthPool(t *testing.T) {
	svc, slots, _ := oauthPriorityTestService(t, oauthPriorityMixedAccounts(), true)
	group := int64(4)
	for _, account := range oauthPriorityMixedAccounts() {
		ms := 8000
		if account.Type == AccountTypeAPIKey {
			ms = 100
		}
		svc.ReportOpenAIAccountScheduleResult(&account, "model", true, &ms)
	}
	counts := map[int64]int{}
	for i := 0; i < 300; i++ {
		chosen, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", fmt.Sprint("oauth-user-", i), "model", nil, OpenAIUpstreamTransportAny, false)
		require.NoError(t, err)
		require.True(t, chosen.Acquired)
		require.Equal(t, AccountTypeOAuth, chosen.Account.Type)
		counts[chosen.Account.ID]++
		chosen.ReleaseFunc()
	}
	for _, id := range []int64{260, 11, 7} {
		require.Greater(t, counts[id], 60)
	}
	require.Zero(t, slots.peak[279])
	t.Logf("300 mixed-pool sessions: %v", counts)
}

func TestOpenAIOAuthPriorityFallsBackAfterEligibilityOrSlots(t *testing.T) {
	for _, advanced := range []bool{true, false} {
		for _, reason := range []string{"full", "cooldown", "unsupported_model", "outside_group", "disabled", "excluded", "quota"} {
			t.Run(fmt.Sprintf("advanced=%v/%s", advanced, reason), func(t *testing.T) {
				mixed := oauthPriorityMixedAccounts()
				accounts := []Account{mixed[1], mixed[3]}
				var excluded map[int64]struct{}
				switch reason {
				case "cooldown":
					until := time.Now().Add(5 * time.Minute)
					accounts[0].TempUnschedulableUntil = &until
				case "unsupported_model":
					accounts[0].Credentials = map[string]any{"model_mapping": map[string]any{"other": "other"}}
				case "outside_group":
					accounts[0].GroupIDs = []int64{67}
				case "disabled":
					accounts[0].Schedulable = false
				case "quota":
					accounts[0].Extra = map[string]any{"auto_pause_7d_threshold": 0.9, "codex_7d_used_percent": 100.0, "codex_7d_reset_at": time.Now().Add(time.Hour).Format(time.RFC3339), "codex_usage_updated_at": time.Now().Format(time.RFC3339)}
				case "excluded":
					excluded = map[int64]struct{}{11: {}}
				}
				svc, slots, _ := oauthPriorityTestService(t, accounts, advanced)
				if reason == "full" {
					slots.active[11] = accounts[0].Concurrency
				}
				group := int64(4)
				chosen, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", "fallback", "model", excluded, OpenAIUpstreamTransportAny, false)
				require.NoError(t, err)
				require.NotNil(t, chosen)
				require.True(t, chosen.Acquired)
				require.Equal(t, int64(279), chosen.Account.ID)
				chosen.ReleaseFunc()
			})
		}
	}
}

func TestOpenAIOAuthPriorityMigratesOrdinaryAPIKeySticky(t *testing.T) {
	for _, advanced := range []bool{true, false} {
		t.Run(fmt.Sprint(advanced), func(t *testing.T) {
			svc, slots, sessions := oauthPriorityTestService(t, oauthPriorityMixedAccounts(), advanced)
			group := int64(4)
			require.NoError(t, sessions.SetSessionAccountID(context.Background(), group, "openai:old-api-session", 279, time.Hour))
			chosen, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", "old-api-session", "model", nil, OpenAIUpstreamTransportHTTPSSE, false)
			require.NoError(t, err)
			require.True(t, chosen.Acquired)
			require.Equal(t, AccountTypeOAuth, chosen.Account.Type)
			id := chosen.Account.ID
			chosen.ReleaseFunc()
			binding, err := svc.getStickySessionAccountID(context.Background(), &group, "old-api-session")
			require.NoError(t, err)
			require.Equal(t, id, binding)
			next, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", "old-api-session", "model", nil, OpenAIUpstreamTransportHTTPSSE, false)
			require.NoError(t, err)
			require.Equal(t, id, next.Account.ID)
			next.ReleaseFunc()
			require.Zero(t, slots.peak[279])
		})
	}
}

func TestOpenAIOAuthPriorityDoesNotMoveProtectedSticky(t *testing.T) {
	for _, protection := range []string{"disable", "preserve", "guardian", "previous", "websocket"} {
		t.Run(protection, func(t *testing.T) {
			svc, slots, _ := oauthPriorityTestService(t, oauthPriorityMixedAccounts(), true)
			scheduler := svc.getOpenAIAccountScheduler(context.Background()).(*defaultOpenAIAccountScheduler)
			group := int64(4)
			req := OpenAIAccountScheduleRequest{GroupID: &group, Platform: PlatformOpenAI, OAuthPriority: true, RequestedModel: "model", RequiredCapability: OpenAIEndpointCapabilityResponses, SessionHash: "bound", StickyAccountID: 279}
			switch protection {
			case "disable":
				req.DisableStickyEscape = true
			case "preserve":
				req.PreserveStickyBinding = true
			case "guardian":
				req.GuardianParentAccountID = 279
			case "previous":
				req.PreviousResponseID = "resp_owner"
			case "websocket":
				req.RequiredTransport = OpenAIUpstreamTransportResponsesWebsocketV2Ingress
			}
			current := oauthPriorityMixedAccounts()[3]
			require.Nil(t, scheduler.tryOAuthPreferredStickyEscape(context.Background(), req, &current))
			require.Empty(t, slots.peak)
		})
	}
}

func TestOpenAIOAuthPriorityCongestionUsesHealthyRelayAndRecovers(t *testing.T) {
	accounts := oauthPriorityMixedAccounts()
	svc, _, _ := oauthPriorityTestService(t, accounts, true)
	scheduler := svc.getOpenAIAccountScheduler(context.Background()).(*defaultOpenAIAccountScheduler)
	for _, a := range accounts[:3] {
		scheduler.stats.reportModel(a.ID, "model", false, nil, time.Now(), true)
	}
	group := int64(4)
	chosen, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", "congested", "model", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.Equal(t, int64(279), chosen.Account.ID)
	chosen.ReleaseFunc()
	// Expire the soft window without waiting or changing the account's hard state.
	for _, a := range accounts[:3] {
		stat := scheduler.stats.loadOrCreate(a.ID)
		stat.modelMu.Lock()
		stat.models["model"].overloadUntil = time.Now().Add(-time.Second)
		stat.modelMu.Unlock()
	}
	recovered, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", "recovered", "model", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.Equal(t, AccountTypeOAuth, recovered.Account.Type)
	recovered.ReleaseFunc()
}

func TestOpenAIOAuthPriorityWeightedStickyDoesNotOverrideType(t *testing.T) {
	svc, _, _ := oauthPriorityTestService(t, oauthPriorityMixedAccounts(), true)
	scheduler := svc.getOpenAIAccountScheduler(context.Background()).(*defaultOpenAIAccountScheduler)
	group := int64(4)
	req := OpenAIAccountScheduleRequest{GroupID: &group, Platform: PlatformOpenAI, RequestedModel: "model", OAuthPriority: true, StickyWeighted: true, StickyAccountID: 279, SessionHash: "weighted"}
	chosen, _, err := scheduler.Select(context.Background(), req)
	require.NoError(t, err)
	require.True(t, chosen.Acquired)
	require.Equal(t, AccountTypeOAuth, chosen.Account.Type)
	chosen.ReleaseFunc()
}

func TestOpenAIOAuthPriorityLegacyFullStickyTriesOtherOAuthBeforeRelay(t *testing.T) {
	for _, batch := range []bool{true, false} {
		t.Run(fmt.Sprint(batch), func(t *testing.T) {
			accounts := oauthPriorityMixedAccounts()
			svc, slots, sessions := oauthPriorityTestService(t, accounts, false)
			svc.cfg.Gateway.Scheduling.LoadBatchEnabled = batch
			group := int64(4)
			slots.active[260] = accounts[0].Concurrency
			require.NoError(t, sessions.SetSessionAccountID(context.Background(), group, "openai:full-oauth", 260, time.Hour))
			chosen, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", "full-oauth", "model", nil, OpenAIUpstreamTransportAny, false)
			require.NoError(t, err)
			require.True(t, chosen.Acquired)
			require.Equal(t, AccountTypeOAuth, chosen.Account.Type)
			require.NotEqual(t, int64(260), chosen.Account.ID)
			chosen.ReleaseFunc()
			require.Zero(t, slots.peak[279])
		})
	}
}

func TestOpenAIOAuthPriorityAllFullWaitsOnOAuthWithoutRevivingCooldown(t *testing.T) {
	accounts := oauthPriorityMixedAccounts()
	svc, slots, _ := oauthPriorityTestService(t, accounts, true)
	group := int64(4)
	for _, a := range accounts {
		slots.active[a.ID] = a.Concurrency
	}
	chosen, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", "all-full", "model", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.False(t, chosen.Acquired)
	require.NotNil(t, chosen.WaitPlan)
	require.Equal(t, AccountTypeOAuth, chosen.Account.Type)
	for i := range accounts {
		until := time.Now().Add(5 * time.Minute)
		accounts[i].TempUnschedulableUntil = &until
	}
	blocked, _, _ := oauthPriorityTestService(t, accounts, true)
	selected, _, err := blocked.SelectAccountWithScheduler(context.Background(), &group, "", "all-blocked", "model", nil, OpenAIUpstreamTransportAny, false)
	require.Error(t, err)
	require.Nil(t, selected)
}

func TestOpenAIOAuthPriorityMigratingAPIKeyUsersStillSpread(t *testing.T) {
	svc, slots, sessions := oauthPriorityTestService(t, oauthPriorityMixedAccounts(), true)
	group := int64(4)
	counts := map[int64]int{}
	for i := 0; i < 300; i++ {
		key := fmt.Sprint("former-api-user-", i)
		require.NoError(t, sessions.SetSessionAccountID(context.Background(), group, "openai:"+key, 279, time.Hour))
		chosen, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", key, "model", nil, OpenAIUpstreamTransportHTTPSSE, false)
		require.NoError(t, err)
		require.True(t, chosen.Acquired)
		require.Equal(t, AccountTypeOAuth, chosen.Account.Type)
		counts[chosen.Account.ID]++
		chosen.ReleaseFunc()
	}
	for _, id := range []int64{260, 11, 7} {
		require.Greater(t, counts[id], 60)
	}
	require.Zero(t, slots.peak[279])
}
