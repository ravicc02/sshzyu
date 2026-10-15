package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func dispatchTestScheduler() *defaultOpenAIAccountScheduler {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIScheduler.LatencyAwareEnabled = true
	cfg.Gateway.OpenAIWS.LBTopK = 2
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights = config.GatewayOpenAIWSSchedulerScoreWeights{TTFT: 1}
	return &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{cfg: cfg}, stats: newOpenAIAccountRuntimeStats()}
}
func TestDispatchCostDivertsBeforeHardLimit(t *testing.T) {
	s := dispatchTestScheduler()
	fast, other := 2000, 3000
	s.stats.reportModel(1, "model", true, &fast, time.Now())
	s.stats.reportModel(2, "model", true, &other, time.Now())
	plan := s.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{RequestedModel: "model", SessionHash: "busy"}, []*Account{{ID: 1, Concurrency: 100}, {ID: 2, Concurrency: 100}}, map[int64]*AccountLoadInfo{1: {CurrentConcurrency: 80, LoadRate: 80}, 2: {}})
	require.Equal(t, int64(2), plan.selectionOrder[0].account.ID, "divert before hard cap")
}
func TestDispatchCostDistributesEquivalentAccounts(t *testing.T) {
	s := dispatchTestScheduler()
	ms := 2000
	for id := int64(1); id <= 2; id++ {
		s.stats.reportModel(id, "model", true, &ms, time.Now())
	}
	counts := map[int64]int{}
	for i := 0; i < 400; i++ {
		plan := s.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{RequestedModel: "model", SessionHash: fmt.Sprint("new-user-", i)}, []*Account{{ID: 1, Concurrency: 100}, {ID: 2, Concurrency: 100}}, map[int64]*AccountLoadInfo{1: {}, 2: {}})
		counts[plan.selectionOrder[0].account.ID]++
	}
	require.Greater(t, counts[1], 120)
	require.Greater(t, counts[2], 120, "equivalent sources need traffic beyond recovery probes")
}
func TestDispatchCostQueuesByAvailableCapacity(t *testing.T) {
	s := dispatchTestScheduler()
	ms := 2000
	for id := int64(1); id <= 2; id++ {
		s.stats.reportModel(id, "model", true, &ms, time.Now())
	}
	plan := s.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{RequestedModel: "model", SessionHash: "queue"}, []*Account{{ID: 1, Concurrency: 2}, {ID: 2, Concurrency: 10}}, map[int64]*AccountLoadInfo{1: {CurrentConcurrency: 2, WaitingCount: 40, LoadRate: 100}, 2: {CurrentConcurrency: 10, WaitingCount: 10, LoadRate: 100}})
	require.Equal(t, int64(2), plan.selectionOrder[0].account.ID)
}

// Deliberately stale batch snapshots reproduce many simultaneous users seeing
// the same idle account. Slot acquisition itself is atomic, like Redis.
type dispatchBurstCache struct {
	ConcurrencyCache
	mu           sync.Mutex
	active, peak map[int64]int
}

func (c *dispatchBurstCache) AcquireAccountSlot(_ context.Context, id int64, cap int, _ string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active[id] >= cap {
		return false, nil
	}
	c.active[id]++
	if c.active[id] > c.peak[id] {
		c.peak[id] = c.active[id]
	}
	return true, nil
}
func (c *dispatchBurstCache) ReleaseAccountSlot(_ context.Context, id int64, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active[id]--
	return nil
}
func (c *dispatchBurstCache) GetAccountsLoadBatch(_ context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	out := map[int64]*AccountLoadInfo{}
	for _, a := range accounts {
		out[a.ID] = &AccountLoadInfo{AccountID: a.ID}
	}
	return out, nil
}
func TestDispatchCostConcurrentBurstStaleSnapshot(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	cache := &dispatchBurstCache{active: map[int64]int{}, peak: map[int64]int{}}
	cfg := dispatchTestScheduler().service.cfg
	accounts := []Account{{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 100}, {ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 100}}
	svc := &OpenAIGatewayService{cfg: cfg, accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"), concurrencyService: NewConcurrencyService(cache)}
	first, second := 2000, 3000
	svc.ReportOpenAIAccountScheduleResult(&accounts[0], "model", true, &first)
	svc.ReportOpenAIAccountScheduleResult(&accounts[1], "model", true, &second)
	start := make(chan struct{})
	selections := make(chan *AccountSelectionResult, 40)
	errs := make(chan error, 40)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			group := int64(1)
			selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &group, "", "", "model", nil, OpenAIUpstreamTransportAny, false)
			if err != nil {
				errs <- err
				return
			}
			selections <- selection
		}(i)
	}
	close(start)
	wg.Wait()
	close(selections)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	counts := map[int64]int{}
	n := 0
	for selection := range selections {
		require.NotNil(t, selection)
		require.True(t, selection.Acquired)
		counts[selection.Account.ID]++
		n++
		selection.ReleaseFunc()
		selection.ReleaseFunc()
	}
	require.Equal(t, 40, n)
	require.Greater(t, counts[1], 0)
	require.Greater(t, counts[2], 0)
	require.Less(t, counts[1], 35)
	for _, a := range accounts {
		require.Zero(t, svc.localOpenAIDispatchCount(a.ID))
		require.Zero(t, cache.active[a.ID])
		require.LessOrEqual(t, cache.peak[a.ID], a.Concurrency)
	}
	encoded, _ := json.Marshal(counts)
	t.Logf("40 simultaneous users / deliberately stale Redis batch: routes=%s", encoded)
}
func TestDispatchCostReservationReleaseOnAcquireFailure(t *testing.T) {
	var released atomic.Int64
	s := dispatchTestScheduler().service
	release := s.trackOpenAIDispatch(1, func() { released.Add(1) })
	require.Equal(t, 1, s.localOpenAIDispatchCount(1))
	release()
	release()
	require.Zero(t, s.localOpenAIDispatchCount(1))
	require.Equal(t, int64(1), released.Load())
}

func TestDispatchCostShedsHistoricallyFastAccountAfterSoftTarget(t *testing.T) {
	s := dispatchTestScheduler()
	fast, slow := 500, 5000
	s.stats.reportModel(1, "model", true, &fast, time.Now())
	s.stats.reportModel(2, "model", true, &slow, time.Now())
	accounts := []*Account{{ID: 1, Concurrency: 100}, {ID: 2, Concurrency: 100}}
	chosen := []int64{}
	releases := []func(){}
	for i := 0; i < 20; i++ {
		plan := s.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{RequestedModel: "model", SessionHash: fmt.Sprintf("shed-%d", i)}, accounts, map[int64]*AccountLoadInfo{1: {}, 2: {}})
		index, release := s.reserveDispatchCandidate(plan.selectionOrder)
		chosen = append(chosen, plan.selectionOrder[index].account.ID)
		releases = append(releases, release)
	}
	for _, release := range releases {
		release()
	}
	fastCount := 0
	for _, id := range chosen {
		if id == 1 {
			fastCount++
		}
	}
	require.Less(t, fastCount, 16, "historically fastest account must shed traffic after its soft target")
	require.Greater(t, 20-fastCount, 0)
}
