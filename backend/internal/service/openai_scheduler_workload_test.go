package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func workloadBody(effort string, stream bool, padding int) []byte {
	body := map[string]any{"model": "model", "stream": stream, "input": strings.Repeat("x", padding)}
	if effort != "" {
		body["reasoning_effort"] = effort
	}
	raw, _ := json.Marshal(body)
	return raw
}
func workloadContext(protocol, effort string, padding int) context.Context {
	return WithOpenAISchedulingWorkload(context.Background(), protocol, workloadBody(effort, true, padding))
}
func observeWorkload(stats *openAIAccountRuntimeStats, account int64, model string, class openAISchedulingWorkload, ms int, now time.Time) {
	stats.reportModel(account, model, true, &ms, now)
	stats.reportWorkloadLatency(account, model, class, &ms, now)
}

func TestOpenAIWorkloadClassificationIsFixedAndSeparatesCostShapes(t *testing.T) {
	base := classifyOpenAISchedulingWorkload("chat", workloadBody("high", true, 1))
	require.NotZero(t, base)
	for _, tc := range []struct {
		name, protocol, effort string
		stream                 bool
		size                   int
	}{
		{"responses", "responses", "high", true, 1},
		{"messages", "messages", "high", true, 1},
		{"websocket", "websocket", "high", true, 1},
		{"nonstream", "chat", "high", false, 1},
		{"xhigh", "chat", "xhigh", true, 1},
		{"long_context", "chat", "high", true, 200000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NotEqual(t, base, classifyOpenAISchedulingWorkload(tc.protocol, workloadBody(tc.effort, tc.stream, tc.size)))
		})
	}
	// Identifiers and arbitrary effort strings never become persistent map keys.
	classes := map[openAISchedulingWorkload]struct{}{}
	for i := 0; i < 1000; i++ {
		body := []byte(fmt.Sprintf(`{"model":"model","stream":true,"reasoning_effort":"unrecognized-%d","session_id":"private-%d","user":"user-%d"}`, i, i, i))
		classes[classifyOpenAISchedulingWorkload("chat", body)] = struct{}{}
	}
	require.Len(t, classes, 1)
	old := workloadContext("responses", "high", 1)
	require.Zero(t, openAISchedulingWorkloadFromContext(WithOpenAISchedulingWorkload(old, "arbitrary-client-path", []byte(`{}`))))
	require.Zero(t, openAISchedulingWorkloadFromContext(WithOpenAISchedulingWorkload(old, "chat", nil)))
	require.Zero(t, openAISchedulingWorkloadForPlatform(old, PlatformGrok))
	require.Zero(t, openAISchedulingWorkloadForPlatform(old, PlatformDeepseek))
}

func TestOpenAIWorkloadContinuationsDoNotPretendToHaveSmallContext(t *testing.T) {
	full := classifyOpenAISchedulingWorkload("responses", []byte(`{"model":"model","stream":true,"input":"short"}`))
	continuation := classifyOpenAISchedulingWorkload("responses", []byte(`{"model":"model","stream":true,"previous_response_id":"resp_private","input":"short"}`))
	require.NotEqual(t, full, continuation)
	require.Equal(t, continuation, classifyOpenAISchedulingWorkload("responses", []byte(`{"model":"model","stream":true,"conversation":{"id":"conv_private"},"input":"short"}`)))
}

func TestOpenAIWorkloadLiveLatencyClassesDoNotPoisonShortChat(t *testing.T) {
	svc, _, _ := oauthPriorityTestService(t, fairShareAccounts(), true)
	account := fairShareAccounts()[0]
	// Measured means from the read-only snapshot, paired with synthetic bodies.
	chat := workloadContext("chat", "", 1)
	high := workloadContext("responses", "high", 200000)
	xhigh := workloadContext("responses", "xhigh", 200000)
	for _, tc := range []struct {
		ctx context.Context
		ms  int
	}{{chat, 6590}, {high, 16660}, {xhigh, 21634}} {
		for n := 0; n < 3; n++ {
			svc.ReportOpenAIAccountScheduleResultForRequest(tc.ctx, &account, "model", true, &tc.ms)
		}
	}
	scheduler := svc.getOpenAIAccountScheduler(context.Background()).(*defaultOpenAIAccountScheduler)
	for _, tc := range []struct {
		ctx context.Context
		ms  float64
	}{{chat, 6590}, {high, 16660}, {xhigh, 21634}} {
		_, ms, known := scheduler.runtimeSnapshot(account.ID, "model", openAISchedulingWorkloadFromContext(tc.ctx))
		require.True(t, known)
		require.InDelta(t, tc.ms, ms, .001)
	}
	escape := openAIStickyEscapeConfig{enabled: true, ttftMs: 15000, errorRate: .5}
	_, _, _, oldEscape := scheduler.shouldEscapeStickyAccount(account.ID, escape, "model")
	require.True(t, oldEscape, "the legacy mixed aggregate makes short chat look too slow")
	_, _, _, newEscape := scheduler.shouldEscapeStickyAccountForWorkload(account.ID, escape, "model", openAISchedulingWorkloadFromContext(chat))
	require.False(t, newEscape, "long high/xhigh requests must not trigger chat latency escape")
}

func TestOpenAIWorkloadColdSamplesAndTTLNeverFallBackToMixedLatency(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	now := time.Now()
	class := openAISchedulingWorkloadFromContext(workloadContext("chat", "", 1))
	other := openAISchedulingWorkloadFromContext(workloadContext("responses", "high", 200000))
	for n := 0; n < 2; n++ {
		observeWorkload(stats, 1, "model", class, 1000, now)
	}
	_, _, known := stats.snapshotWorkload(1, "model", class, now)
	require.False(t, known, "fewer than three samples are neutral")
	_, _, mixedKnown := stats.snapshotModel(1, "model", now)
	require.True(t, mixedKnown)
	observeWorkload(stats, 1, "model", class, 1000, now)
	_, ms, known := stats.snapshotWorkload(1, "model", class, now)
	require.True(t, known)
	require.Equal(t, 1000.0, ms)
	for n := 0; n < 3; n++ {
		observeWorkload(stats, 1, "model", other, 30000, now.Add(9*time.Minute))
	}
	_, _, known = stats.snapshotWorkload(1, "model", class, now.Add(10*time.Minute+time.Second))
	require.False(t, known, "another class cannot keep stale latency fresh")
	_, ms, known = stats.snapshotWorkload(1, "model", other, now.Add(10*time.Minute+time.Second))
	require.True(t, known)
	require.Equal(t, 30000.0, ms)
}

func TestOpenAIWorkloadLatencyCacheIsBoundedWithoutEvictingCongestion(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	now := time.Now()
	stats.reportModel(1, "model", false, nil, now, true)
	for n := 1; n <= 20; n++ {
		ms := 1000 + n
		stats.reportWorkloadLatency(1, "model", openAISchedulingWorkload(n), &ms, now.Add(time.Duration(n)*time.Millisecond))
	}
	account := stats.loadOrCreate(1)
	account.modelMu.Lock()
	require.Len(t, account.models["model"].workloads, openAIWorkloadLatencyLimit)
	_, oldestExists := account.models["model"].workloads[1]
	_, latestExists := account.models["model"].workloads[20]
	account.modelMu.Unlock()
	require.False(t, oldestExists)
	require.True(t, latestExists)
	require.True(t, stats.modelOverloaded(1, "model", now.Add(time.Second)))
}

type workloadHealthRepo struct {
	fairShareAccountRepo
	cooldowns int
}

func (r *workloadHealthRepo) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	r.cooldowns++
	return nil
}
func TestOpenAIWorkloadKeepsAccountHealthAndModelCongestionShared(t *testing.T) {
	accounts := fairShareAccounts()
	svc, _, _ := oauthPriorityTestService(t, accounts, true)
	repo := &workloadHealthRepo{fairShareAccountRepo: fairShareAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}}}
	svc.accountRepo = repo
	account := &accounts[0]
	chat := workloadContext("chat", "", 1)
	high := workloadContext("responses", "high", 200000)
	for n := 0; n < 3; n++ {
		ms := 1000
		svc.ReportOpenAIAccountScheduleResultForRequest(chat, account, "model", true, &ms)
	}
	congestion := &UpstreamFailoverError{StatusCode: 503}
	svc.ReportOpenAIAccountScheduleResultForRequest(high, account, "model", false, nil, congestion)
	scheduler := svc.getOpenAIAccountScheduler(context.Background()).(*defaultOpenAIAccountScheduler)
	require.True(t, scheduler.stats.modelOverloaded(account.ID, "model", time.Now()))
	require.Zero(t, repo.cooldowns, "503 is still soft and model-scoped")
	errorRate, _, _ := scheduler.runtimeSnapshot(account.ID, "model", openAISchedulingWorkloadFromContext(chat))
	require.Positive(t, errorRate)
	svc.ReportOpenAIAccountScheduleResultForRequest(high, account, "model", false, nil, &UpstreamFailoverError{StatusCode: 502})
	svc.ReportOpenAIAccountScheduleResultForRequest(chat, account, "model", false, nil, &UpstreamFailoverError{StatusCode: 502})
	require.Equal(t, 1, repo.cooldowns, "changing workload must not reset the shared 502 rolling window")
	require.True(t, svc.isOpenAIAccountRequestRuntimeBlocked(account, "model"))
}

func TestOpenAIWorkloadRealSelectionUsesMatchingLatency(t *testing.T) {
	accounts := fairShareAccounts()[:2]
	for i := range accounts {
		accounts[i].Type = AccountTypeAPIKey
		accounts[i].Priority = 0
	}
	svc, _, _ := oauthPriorityTestService(t, accounts, true)
	chat := workloadContext("chat", "", 1)
	reasoning := workloadContext("responses", "xhigh", 200000)
	for _, tc := range []struct {
		ctx      context.Context
		msA, msB int
	}{{chat, 1000, 6000}, {reasoning, 30000, 2000}} {
		for n := 0; n < 3; n++ {
			svc.ReportOpenAIAccountScheduleResultForRequest(tc.ctx, &accounts[0], "model", true, &tc.msA)
			svc.ReportOpenAIAccountScheduleResultForRequest(tc.ctx, &accounts[1], "model", true, &tc.msB)
		}
	}
	group := int64(4)
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want int64
	}{{"chat", chat, accounts[0].ID}, {"reasoning", reasoning, accounts[1].ID}} {
		matching := 0
		for n := 0; n < 200; n++ {
			chosen, _, err := svc.SelectAccountWithScheduler(tc.ctx, &group, "", fmt.Sprintf("%s-user-%d", tc.name, n), "model", nil, OpenAIUpstreamTransportAny, false)
			require.NoError(t, err)
			require.True(t, chosen.Acquired)
			if chosen.Account.ID == tc.want {
				matching++
			}
			chosen.ReleaseFunc()
		}
		// Keep the existing recovery exploration share; matching latency must
		// determine the dominant selection, not forbid all probing.
		require.GreaterOrEqual(t, matching, 160, tc.name)
	}
}

func TestOpenAIWorkloadConcurrentReportsStayBounded(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				now := time.Now()
				key := openAISchedulingWorkload(1 + (worker+n)%16)
				observeWorkload(stats, 1, "model", key, 1000+worker, now)
				stats.snapshotWorkload(1, "model", key, now)
			}
		}(worker)
	}
	wg.Wait()
	account := stats.loadOrCreate(1)
	account.modelMu.Lock()
	defer account.modelMu.Unlock()
	require.LessOrEqual(t, len(account.models["model"].workloads), openAIWorkloadLatencyLimit)
	require.Len(t, account.models, 1)
}

func BenchmarkOpenAIWorkloadClassification(b *testing.B) {
	for _, size := range []int{1024, 128 * 1024, 1024 * 1024} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			body := workloadBody("xhigh", true, size)
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				classifyOpenAISchedulingWorkload("responses", body)
			}
		})
	}
}

func TestOpenAIWorkloadWebsocketTurnsDoNotInheritOldLatencyClass(t *testing.T) {
	first := workloadContext("websocket", "high", 1)
	next := WithOpenAISchedulingWorkload(first, "websocket", workloadBody("xhigh", true, 200000))
	require.NotEqual(t, openAISchedulingWorkloadFromContext(first), openAISchedulingWorkloadFromContext(next))
	// Wire size cannot reveal the server-side conversation held by a socket.
	require.Equal(t, classifyOpenAISchedulingWorkload("websocket", workloadBody("high", true, 1)), classifyOpenAISchedulingWorkload("websocket", workloadBody("high", true, 200000)))
	require.Zero(t, openAISchedulingWorkloadFromContext(WithOpenAISchedulingWorkload(first, "websocket", nil)))
}

func TestOpenAIWorkloadNormalizerConcurrentAliases(t *testing.T) {
	var wg sync.WaitGroup
	pairs := []struct{ input, want string }{{"HIGH", "high"}, {" x-high ", "xhigh"}, {"extra_high", "xhigh"}, {"MAX", "max"}, {" unknown ", ""}, {"", ""}}
	failures := make(chan string, 16)
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				for _, pair := range pairs {
					if got := NormalizeMaxReasoningEffort(pair.input); got != pair.want {
						failures <- pair.input
						return
					}
				}
			}
		}()
	}
	wg.Wait()
	close(failures)
	for value := range failures {
		t.Errorf("normalization changed under concurrency: %q", value)
	}
}
