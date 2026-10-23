package service

import (
	"math"
	"time"
)

// New-session admission uses bounded quality bias, instead of a permanent
// winner-takes-all TTFT race. Existing sticky/owner selection still happens first.
// Called under openaiDispatchMu so the reservation includes simultaneous arrivals.
func (s *defaultOpenAIAccountScheduler) fairShareDispatchCandidate(pool []openAIAccountCandidateScore, req OpenAIAccountScheduleRequest) (int, bool) {
	if len(pool) < 2 || req.DisableStickyEscape || req.PreserveStickyBinding || req.GuardianParentAccountID > 0 || (req.PreviousResponseID != "" && !req.PreviousResponseCanMove) || (req.Platform != "" && req.Platform != PlatformOpenAI) {
		return 0, false
	}
	if pool[0].costPriorityActive {
		return 0, false
	}
	healthy := openAIHasHealthyCandidate(pool)
	minError := math.MaxFloat64
	rates := make([]float64, len(pool))
	for i, c := range pool {
		if c.account == nil || (healthy && c.overloaded) {
			rates[i] = math.MaxFloat64
			continue
		}
		rates[i] = c.errorRate
		// Congestion changes model ranking, not account health. The global history
		// excludes 503 and prevents its EWMA from becoming a second long-lived ban.
		if s.stats != nil {
			if _, exists := s.stats.accounts.Load(c.account.ID); exists {
				rates[i], _, _ = s.stats.snapshot(c.account.ID)
			}
		}
		if rates[i] < minError {
			minError = rates[i]
		}
	}
	eligible := make([]int, 0, len(pool))
	costs := make([]float64, len(pool))
	fastest := math.MaxFloat64
	for i, c := range pool {
		if c.account == nil || (healthy && c.overloaded) || rates[i] > minError+.05 {
			continue
		}
		if req.RequireCompact && openAICompactSupportTier(c.account) != openAICompactSupportTier(pool[0].account) {
			continue
		}
		if c.account.Concurrency > 0 && s.liveDispatchConcurrency(c) >= c.account.Concurrency {
			continue
		}
		cost := s.estimateFirstOutput(c.account, c.loadInfo, c.ttft, c.hasTTFT)
		if cost <= 0 || math.IsNaN(cost) || math.IsInf(cost, 0) {
			continue
		}
		costs[i] = cost
		eligible = append(eligible, i)
		if cost < fastest {
			fastest = cost
		}
	}
	if len(eligible) < 2 {
		return 0, false
	}
	// Do not spread work onto a severely slow/stalled supplier just for balance.
	cohort := eligible[:0]
	minScore, maxScore := math.MaxFloat64, -math.MaxFloat64
	for _, i := range eligible {
		if costs[i] > fastest*4 {
			continue
		}
		cohort = append(cohort, i)
		if pool[i].score < minScore {
			minScore = pool[i].score
		}
		if pool[i].score > maxScore {
			maxScore = pool[i].score
		}
	}
	if len(cohort) < 2 {
		return 0, false
	}
	weights := make([]float64, len(cohort))
	sum := 0.0
	now := time.Now()
	for n, i := range cohort {
		c := pool[i]
		target := s.fairShareSoftTarget(c.account)
		active := s.liveDispatchConcurrency(c)
		waiting, load := 0, 0
		if c.loadInfo != nil {
			waiting = c.loadInfo.WaitingCount
			load = c.loadInfo.LoadRate
		}
		pressure := math.Max(float64(active)/float64(target), float64(load)/100) + math.Max(0, float64(waiting))/float64(target)
		quality := .75 + .25*math.Sqrt(fastest/costs[i])
		bias := 1.0
		if maxScore > minScore {
			bias = .85 + .3*clamp01((c.score-minScore)/(maxScore-minScore))
		}
		quota := .75 + .25*openAIQuotaHeadroomFactor(c.account, now)
		capacity := float64(target) / 8
		weight := capacity * quality * bias * quota / (1 + 4*pressure)
		weights[n] = weight
		sum += weight
	}
	if sum <= 0 || math.IsNaN(sum) || math.IsInf(sum, 0) {
		return 0, false
	}
	rng := newOpenAISelectionRNG(deriveOpenAISelectionSeed(req) ^ 0xe7037ed1a0b428db)
	pick := rng.nextFloat64() * sum
	for n, i := range cohort {
		pick -= weights[n]
		if pick <= 0 {
			return i, true
		}
	}
	return cohort[len(cohort)-1], true
}

func (s *defaultOpenAIAccountScheduler) fairShareSoftTarget(account *Account) int {
	target := s.service.cfg.Gateway.OpenAIScheduler.LatencyConcurrencyTarget
	if target <= 0 {
		target = 8
	}
	// High configured hard caps do not grant a disproportionate share of all
	// idle users. Real spare capacity remains available via pressure/fallback.
	if target > 8 {
		target = 8
	}
	if account != nil && account.Concurrency > 0 && target > account.Concurrency {
		target = account.Concurrency
	}
	return target
}
func (s *defaultOpenAIAccountScheduler) liveDispatchConcurrency(c openAIAccountCandidateScore) int {
	active := 0
	if c.loadInfo != nil {
		active = c.loadInfo.CurrentConcurrency
	}
	if local := s.service.localOpenAIDispatchCount(c.account.ID); local > active {
		active = local
	}
	return active
}
