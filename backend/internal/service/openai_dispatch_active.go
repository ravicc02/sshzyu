package service

import (
	"math"
	"sync"
	"sync/atomic"
)

func (s *OpenAIGatewayService) localOpenAIDispatchCount(id int64) int {
	if s == nil {
		return 0
	}
	v, ok := s.openaiDispatchActive.Load(id)
	if !ok {
		return 0
	}
	return int(v.(*atomic.Int64).Load())
}
func (s *OpenAIGatewayService) trackOpenAIDispatch(id int64, release func()) func() {
	v, _ := s.openaiDispatchActive.LoadOrStore(id, &atomic.Int64{})
	active := v.(*atomic.Int64)
	active.Add(1)
	var once sync.Once
	return func() {
		once.Do(func() {
			active.Add(-1)
			if release != nil {
				release()
			}
		})
	}
}

// Reserve before Redis I/O so simultaneous selectors cannot all act on the same
// cached zero-load snapshot. Redis remains the authoritative hard-cap gate.
func (s *defaultOpenAIAccountScheduler) reserveDispatchCandidate(pool []openAIAccountCandidateScore) (int, func()) {
	s.service.openaiDispatchMu.Lock()
	defer s.service.openaiDispatchMu.Unlock()
	if len(pool) == 0 || pool[0].account == nil {
		return 0, func() {}
	}
	first := pool[0]
	target := s.service.cfg.Gateway.OpenAIScheduler.LatencyConcurrencyTarget
	if target <= 0 {
		target = 8
	}
	activeOf := func(candidate openAIAccountCandidateScore) int {
		active := 0
		if candidate.loadInfo != nil {
			active = candidate.loadInfo.CurrentConcurrency
		}
		if local := s.service.localOpenAIDispatchCount(candidate.account.ID); local > active {
			active = local
		}
		return active
	}
	if first.account.Concurrency > 0 && target > first.account.Concurrency {
		target = first.account.Concurrency
	}
	if activeOf(first) >= target {
		peer, peerCost := -1, math.MaxFloat64
		for i := 1; i < len(pool); i++ {
			candidate := pool[i]
			if candidate.account == nil || candidate.account.Priority != first.account.Priority || candidate.errorRate > first.errorRate+0.05 {
				continue
			}
			limit := target
			if candidate.account.Concurrency > 0 && limit > candidate.account.Concurrency {
				limit = candidate.account.Concurrency
			}
			if activeOf(candidate) >= limit {
				continue
			}
			cost := s.estimateFirstOutput(candidate.account, candidate.loadInfo, candidate.ttft, candidate.hasTTFT)
			if cost < peerCost {
				peer, peerCost = i, cost
			}
		}
		if peer >= 0 {
			return peer, s.service.trackOpenAIDispatch(pool[peer].account.ID, nil)
		}
	}
	best, bestScore := 0, -math.MaxFloat64
	for i, candidate := range pool {
		if candidate.account == nil {
			continue
		}
		score := candidate.score
		if candidate.estimatedTTFT > 0 {
			cost := s.estimateFirstOutput(candidate.account, candidate.loadInfo, candidate.ttft, candidate.hasTTFT)
			// Use projected TTFT as the dominant dispatch cost. Historical score
			// breaks close ties, but must not defeat a saturated soft target.
			score -= math.Log(math.Max(1, cost/candidate.estimatedTTFT))
			score -= math.Log(math.Max(1, cost/1000.0))
		}
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	return best, s.service.trackOpenAIDispatch(pool[best].account.ID, nil)
}
