package service

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"
)

const openAIModelRuntimeTTL = 10 * time.Minute
const openAIModelRuntimeLimit = 64

type openAIModelRuntimeStat struct {
	stats      *openAIAccountRuntimeStats
	observedAt time.Time
}

func (s *defaultOpenAIAccountScheduler) latencyAwareEnabled() bool {
	return s != nil && s.service != nil && s.service.cfg != nil && s.service.cfg.Gateway.OpenAIScheduler.LatencyAwareEnabled
}

func (s *defaultOpenAIAccountScheduler) runtimeSnapshot(accountID int64, model string) (float64, float64, bool) {
	if s == nil || s.stats == nil {
		return 0, 0, false
	}
	if s.latencyAwareEnabled() {
		return s.stats.snapshotModel(accountID, model, time.Now())
	}
	return s.stats.snapshot(accountID)
}

// Model observations stay bounded per account. Old observations are neutral so
// a recovered supplier gets another chance rather than a permanent penalty.
func (s *openAIAccountRuntimeStats) reportModel(accountID int64, model string, success bool, ttft *int, now time.Time) {
	model = strings.TrimSpace(model)
	if s == nil || accountID <= 0 || model == "" {
		return
	}
	account := s.loadOrCreate(accountID)
	account.modelMu.Lock()
	defer account.modelMu.Unlock()
	if account.models == nil {
		account.models = make(map[string]*openAIModelRuntimeStat)
	}
	stat := account.models[model]
	if stat == nil || now.Sub(stat.observedAt) >= openAIModelRuntimeTTL {
		if len(account.models) >= openAIModelRuntimeLimit && stat == nil {
			var oldest string
			var oldestAt time.Time
			for key, item := range account.models {
				if oldest == "" || item.observedAt.Before(oldestAt) {
					oldest, oldestAt = key, item.observedAt
				}
			}
			delete(account.models, oldest)
		}
		stat = &openAIModelRuntimeStat{stats: newOpenAIAccountRuntimeStats()}
		account.models[model] = stat
	}
	_, previousTTFT, hadTTFT := stat.stats.snapshot(accountID)
	stat.stats.report(accountID, success, ttft)
	// A healthy fast observation proves recovery; do not make a recovered
	// supplier spend many low-traffic probes paying off an old latency spike.
	if success && ttft != nil && *ttft > 0 && hadTTFT && float64(*ttft) < previousTTFT/2 {
		stat.stats.loadOrCreate(accountID).ttftEWMABits.Store(math.Float64bits(float64(*ttft)))
	}
	stat.observedAt = now
}

func (s *openAIAccountRuntimeStats) snapshotModel(accountID int64, model string, now time.Time) (float64, float64, bool) {
	if s == nil {
		return 0, 0, false
	}
	value, ok := s.accounts.Load(accountID)
	if !ok {
		return 0, 0, false
	}
	account := value.(*openAIAccountRuntimeStat)
	account.modelMu.Lock()
	defer account.modelMu.Unlock()
	stat := account.models[strings.TrimSpace(model)]
	if stat == nil || now.Sub(stat.observedAt) >= openAIModelRuntimeTTL {
		return 0, 0, false
	}
	return stat.stats.snapshot(accountID)
}

// Complete chat messages can move; server-owned responses and guardian bindings
// must retain their owner. Probe alternatives without changing the old binding.
func (s *defaultOpenAIAccountScheduler) tryLatencyAwareChatStickyEscape(ctx context.Context, req OpenAIAccountScheduleRequest, current *Account) *AccountSelectionResult {
	if !s.latencyAwareEnabled() || !s.service.openAIStickyEscapeConfig().enabled || req.DisableStickyEscape || req.PreserveStickyBinding || req.PreviousResponseID != "" || req.RequiredCapability != OpenAIEndpointCapabilityChatCompletions || (req.RequiredTransport != OpenAIUpstreamTransportHTTPSSE && req.RequiredTransport != OpenAIUpstreamTransportAny) {
		return nil
	}
	currentModel := ResolveOpenAIAccountUpstreamModelForRequest(current, req.RequestedModel, req.RequireCompact)
	currentError, currentTTFT, known := s.runtimeSnapshot(current.ID, currentModel)
	if !known || currentTTFT < 1000 {
		return nil
	}
	probe := req
	probe.PreserveStickyBinding = true
	probe.SessionHash = ""
	probe.StickyAccountID = 0
	probe.ExcludedIDs = cloneExcludedAccountIDs(req.ExcludedIDs)
	if probe.ExcludedIDs == nil {
		probe.ExcludedIDs = make(map[int64]struct{})
	}
	probe.ExcludedIDs[current.ID] = struct{}{}
	selection, _, _, _, err := s.selectByLoadBalance(ctx, probe)
	if err != nil || selection == nil || selection.Account == nil {
		return nil
	}
	other := selection.Account
	otherModel := ResolveOpenAIAccountUpstreamModelForRequest(other, req.RequestedModel, req.RequireCompact)
	otherError, otherTTFT, otherKnown := s.runtimeSnapshot(other.ID, otherModel)
	// Preserve ownership above; only movable full-history Chat requests reach here.
	// A sticky account can be healthy historically yet congested right now.
	if s.service.concurrencyService != nil {
		loads, loadErr := s.service.concurrencyService.GetAccountsLoadBatch(ctx, []AccountWithConcurrency{{ID: current.ID, MaxConcurrency: current.EffectiveLoadFactor()}, {ID: other.ID, MaxConcurrency: other.EffectiveLoadFactor()}})
		if loadErr == nil {
			currentTTFT = s.estimateFirstOutput(current, loads[current.ID], currentTTFT, known)
			otherTTFT = s.estimateFirstOutput(other, loads[other.ID], otherTTFT, otherKnown)
		}
	}
	if other.ID != current.ID && otherKnown && otherError <= currentError && currentTTFT >= 2*otherTTFT && currentTTFT-otherTTFT >= 500 {
		return selection
	}
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
	return nil
}

// Keep a small probe share for suppliers whose latency has recovered. A strict
// winner-takes-all policy can get stuck after one unlucky slow observation.
func (s *defaultOpenAIAccountScheduler) orderLatencyAwareCandidates(pool []openAIAccountCandidateScore, req OpenAIAccountScheduleRequest) []openAIAccountCandidateScore {
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].score > pool[j].score })
	if len(pool) < 2 {
		return pool
	}
	if req.UseUpstreamTokenCost && s.service.openAIWSSchedulerWeightsForRequest(context.Background()).UpstreamCost > 0 {
		return pool
	}
	rng := newOpenAISelectionRNG(deriveOpenAISelectionSeed(req) ^ 0xa0761d6478bd642f)
	if rng.nextFloat64() >= .10 {
		best := pool[0]
		equivalent := []int{0}
		for i := 1; i < len(pool); i++ {
			other := pool[i]
			if best.score-other.score <= .05 && other.account.Priority == best.account.Priority && other.errorRate <= best.errorRate+.01 && other.estimatedTTFT <= best.estimatedTTFT*1.2 {
				equivalent = append(equivalent, i)
			}
		}
		chosen := equivalent[int(rng.nextUint64()%uint64(len(equivalent)))]
		pool[0], pool[chosen] = pool[chosen], pool[0]
		return pool
	}
	best := pool[0]
	eligible := make([]int, 0, len(pool)-1)
	for i := 1; i < len(pool); i++ {
		other := pool[i]
		if other.account.Priority != best.account.Priority || other.errorRate > best.errorRate+.05 || other.loadInfo.LoadRate > best.loadInfo.LoadRate+20 || other.loadInfo.WaitingCount > best.loadInfo.WaitingCount || other.estimatedTTFT > best.estimatedTTFT*2 && other.loadInfo.CurrentConcurrency > best.loadInfo.CurrentConcurrency {
			continue
		}
		eligible = append(eligible, i)
	}
	if len(eligible) > 0 {
		i := eligible[int(rng.nextUint64()%uint64(len(eligible)))]
		pool[0], pool[i] = pool[i], pool[0]
	}
	return pool
}

// estimateFirstOutput is a pressure heuristic, not an upstream queue promise.
// The target starts shedding work before a generous configured hard cap is full.
// Unknown history gets a neutral 3s estimate so unused accounts can learn.
func (s *defaultOpenAIAccountScheduler) estimateFirstOutput(account *Account, load *AccountLoadInfo, ttft float64, known bool) float64 {
	if !known || ttft <= 0 || math.IsNaN(ttft) || math.IsInf(ttft, 0) {
		ttft = 3000
	}
	if load == nil {
		load = &AccountLoadInfo{}
	}
	active := load.CurrentConcurrency
	if local := s.service.localOpenAIDispatchCount(account.ID); local > active {
		active = local
	}
	target := s.service.cfg.Gateway.OpenAIScheduler.LatencyConcurrencyTarget
	if target <= 0 {
		target = 8
	}
	if account.Concurrency > 0 && target > account.Concurrency {
		target = account.Concurrency
	}
	pressure := float64(active) / float64(target)
	// Small hard-cap pools and externally reported pressure remain meaningful.
	utilization := math.Max(0, float64(load.LoadRate)/100)
	if account.Concurrency > 0 {
		utilization = math.Max(utilization, float64(active)/float64(account.Concurrency))
	}
	queue := float64(load.WaitingCount) / float64(target)
	return ttft * (1 + pressure*pressure + utilization*utilization + math.Max(0, queue))
}
