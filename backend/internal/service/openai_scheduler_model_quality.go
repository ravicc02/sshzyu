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
	workloads      map[openAISchedulingWorkload]openAIWorkloadLatency
	stats          *openAIAccountRuntimeStats
	observedAt     time.Time
	overloadUntil  time.Time
	lastOverloadAt time.Time
	overloadCount  int
}

func (s *defaultOpenAIAccountScheduler) latencyAwareEnabled() bool {
	return s != nil && s.service != nil && s.service.cfg != nil && s.service.cfg.Gateway.OpenAIScheduler.LatencyAwareEnabled
}

func (s *defaultOpenAIAccountScheduler) runtimeSnapshot(accountID int64, model string, workload ...openAISchedulingWorkload) (float64, float64, bool) {
	if s == nil || s.stats == nil {
		return 0, 0, false
	}
	if s.latencyAwareEnabled() {
		if len(workload) > 0 && workload[0] != 0 {
			return s.stats.snapshotWorkload(accountID, model, workload[0], time.Now())
		}
		return s.stats.snapshotModel(accountID, model, time.Now())
	}
	return s.stats.snapshot(accountID)
}

// Model observations stay bounded per account. Old observations are neutral so
// a recovered supplier gets another chance rather than a permanent penalty.
func (s *openAIAccountRuntimeStats) reportModel(accountID int64, model string, success bool, ttft *int, now time.Time, overloaded ...bool) {
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
	if !success && len(overloaded) > 0 && overloaded[0] {
		if now.Sub(stat.lastOverloadAt) > time.Minute {
			stat.overloadCount = 0
		}
		stat.overloadCount++
		delay := 15 * time.Second * time.Duration(1<<min(stat.overloadCount-1, 2))
		stat.overloadUntil = now.Add(delay)
		stat.lastOverloadAt = now
	} else if success && !now.Before(stat.overloadUntil) && now.Sub(stat.lastOverloadAt) > time.Minute {
		stat.overloadUntil = time.Time{}
		stat.lastOverloadAt = time.Time{}
		stat.overloadCount = 0
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

// A soft, model-scoped deferral never disables an account or changes Redis hard caps.
// Callers retain deferred candidates as fallback when no healthy alternative exists.
func (s *openAIAccountRuntimeStats) modelOverloaded(accountID int64, model string, now time.Time) bool {
	if s == nil {
		return false
	}
	value, ok := s.accounts.Load(accountID)
	if !ok {
		return false
	}
	account := value.(*openAIAccountRuntimeStat)
	account.modelMu.Lock()
	defer account.modelMu.Unlock()
	stat := account.models[strings.TrimSpace(model)]
	return stat != nil && now.Sub(stat.observedAt) < openAIModelRuntimeTTL && now.Before(stat.overloadUntil)
}

// Complete chat messages can move; server-owned responses and guardian bindings
// must retain their owner. Probe alternatives without changing the old binding.
func (s *defaultOpenAIAccountScheduler) tryLatencyAwareChatStickyEscape(ctx context.Context, req OpenAIAccountScheduleRequest, current *Account) *AccountSelectionResult {
	if !s.latencyAwareEnabled() || !s.service.openAIStickyEscapeConfig().enabled || req.DisableStickyEscape || req.PreserveStickyBinding || req.GuardianParentAccountID > 0 || req.PreviousResponseID != "" || (req.RequiredTransport != OpenAIUpstreamTransportHTTPSSE && req.RequiredTransport != OpenAIUpstreamTransportAny) {
		return nil
	}
	if req.RequiredCapability != OpenAIEndpointCapabilityChatCompletions && req.RequiredCapability != OpenAIEndpointCapabilityResponses {
		return nil
	}
	currentModel := ResolveOpenAIAccountUpstreamModelForRequest(current, req.RequestedModel, req.RequireCompact)
	currentError, currentTTFT, known := s.runtimeSnapshot(current.ID, currentModel, req.workload)
	overloaded := s.stats.modelOverloaded(current.ID, currentModel, time.Now())
	var currentLoad *AccountLoadInfo
	pressure := false
	if s.service.concurrencyService != nil {
		loads, err := s.service.concurrencyService.GetAccountsLoadBatch(ctx, []AccountWithConcurrency{{ID: current.ID, MaxConcurrency: current.EffectiveLoadFactor()}})
		if err == nil {
			currentLoad = loads[current.ID]
			pressure = s.liveDispatchConcurrency(openAIAccountCandidateScore{account: current, loadInfo: currentLoad}) >= s.fairShareSoftTarget(current)
		}
	}
	if req.RequiredCapability == OpenAIEndpointCapabilityResponses && !overloaded && req.UseUpstreamTokenCost && s.service.openAIWSSchedulerWeightsForRequest(ctx).UpstreamCost > 0 {
		return nil // preserve an explicitly selected upstream-cost policy
	}
	// Ordinary full-history Responses may escape a hot account. Server-owned
	// continuations and task/socket bindings were excluded above.
	if req.RequiredCapability == OpenAIEndpointCapabilityResponses && !overloaded && !pressure {
		return nil
	}
	if !overloaded && !pressure && (!known || currentTTFT < 1000) {
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
	otherError, otherTTFT, otherKnown := s.runtimeSnapshot(other.ID, otherModel, req.workload)
	if s.service.concurrencyService != nil {
		loads, loadErr := s.service.concurrencyService.GetAccountsLoadBatch(ctx, []AccountWithConcurrency{{ID: other.ID, MaxConcurrency: other.EffectiveLoadFactor()}})
		if loadErr == nil {
			otherLoad := loads[other.ID]
			currentTTFT = s.estimateFirstOutput(current, currentLoad, currentTTFT, known)
			otherTTFT = s.estimateFirstOutput(other, otherLoad, otherTTFT, otherKnown)
			currentActive := s.liveDispatchConcurrency(openAIAccountCandidateScore{account: current, loadInfo: currentLoad})
			otherActive := s.liveDispatchConcurrency(openAIAccountCandidateScore{account: other, loadInfo: otherLoad})
			if selection.Acquired && otherActive > 0 {
				otherActive--
			} // exclude our newly acquired slot
			if pressure && selection.Acquired && other.ID != current.ID && !s.stats.modelOverloaded(other.ID, otherModel, time.Now()) && otherError <= currentError+.05 && otherActive < s.fairShareSoftTarget(other) && otherActive+1 < currentActive {
				return selection
			}
		}
	}
	if other.ID != current.ID && overloaded && !s.stats.modelOverloaded(other.ID, otherModel, time.Now()) {
		return selection
	}
	if req.RequiredCapability == OpenAIEndpointCapabilityChatCompletions && other.ID != current.ID && !overloaded && otherKnown && otherError <= currentError && currentTTFT >= 2*otherTTFT && currentTTFT-otherTTFT >= 500 {
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
	sort.SliceStable(pool, func(i, j int) bool {
		if pool[i].overloaded != pool[j].overloaded {
			return !pool[i].overloaded
		}
		return pool[i].score > pool[j].score
	})
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
			if other.overloaded == best.overloaded && best.score-other.score <= .05 && other.account.Priority == best.account.Priority && other.errorRate <= best.errorRate+.01 && other.estimatedTTFT <= best.estimatedTTFT*1.2 {
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
		if other.overloaded != best.overloaded || other.account.Priority != best.account.Priority || other.errorRate > best.errorRate+.05 || other.loadInfo.LoadRate > best.loadInfo.LoadRate+20 || other.loadInfo.WaitingCount > best.loadInfo.WaitingCount || other.estimatedTTFT > best.estimatedTTFT*2 && other.loadInfo.CurrentConcurrency > best.loadInfo.CurrentConcurrency {
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
