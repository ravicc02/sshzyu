package service

import (
	"context"
	"errors"
	"time"
)

// Health and hard eligibility gates run before type preference. Within each
// health tier, OAuth is preferred regardless of whether plan_type was imported.
// The existing paid-subscription switch refines ordering inside the OAuth tier.
func (s *defaultOpenAIAccountScheduler) oauthPriorityPools(req OpenAIAccountScheduleRequest, accounts []*Account) [][]*Account {
	var buckets [6][]*Account
	now := time.Now()
	for _, account := range accounts {
		if account == nil {
			continue
		}
		tier := 2
		if account.IsOpenAIOAuth() {
			tier = 1
			if !req.SubscriptionPriority || account.IsOpenAIChatGPTSubscription() {
				tier = 0
			}
		}
		model := ResolveOpenAIAccountUpstreamModelForRequest(account, req.RequestedModel, req.RequireCompact)
		if s.stats != nil && s.stats.modelOverloaded(account.ID, model, now) {
			tier += 3
		}
		buckets[tier] = append(buckets[tier], account)
	}
	pools := make([][]*Account, 0, len(buckets))
	for _, pool := range buckets {
		if len(pool) > 0 {
			pools = append(pools, pool)
		}
	}
	return pools
}

func (s *defaultOpenAIAccountScheduler) selectOAuthPriorityPools(ctx context.Context, req OpenAIAccountScheduleRequest, accounts []*Account, loads map[int64]*AccountLoadInfo, budget *openAISelectionProbeBudget, filters openAISelectionFilterStats) (*AccountSelectionResult, int, int, float64, error) {
	// Preserve invalid-binding cleanup even when per-tier sticky IDs are masked.
	if req.StickyWeighted && req.StickyAccountID > 0 && req.SessionHash != "" && !req.PreserveStickyBinding && !req.oauthOnly {
		present := false
		for _, account := range accounts {
			if account.ID == req.StickyAccountID {
				present = true
				break
			}
		}
		if !present {
			account, _ := s.service.getSchedulableAccount(ctx, req.StickyAccountID)
			if account != nil && !s.service.openAIAccountMatchesSchedulingGroup(account, req.GroupID) {
				_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, req.SessionHash)
				req.StickyAccountID = 0
			}
		}
	}
	pools := s.oauthPriorityPools(req, accounts)
	attempts := make([]openAIAccountLoadSelectionAttempt, 0, len(pools))
	requests := make([]OpenAIAccountScheduleRequest, 0, len(pools))
	for _, pool := range pools {
		scoped := req
		// A weighted sticky fallback must not jump into a different type/health tier.
		contains := func(id int64) bool {
			for _, a := range pool {
				if a.ID == id {
					return true
				}
			}
			return false
		}
		if !contains(scoped.StickyAccountID) {
			scoped.StickyAccountID = 0
		}
		if !contains(scoped.StickyPreviousAccountID) {
			scoped.StickyPreviousAccountID = 0
		}
		attempt := s.trySelectByLoadBalancePool(ctx, scoped, pool, loads, budget)
		if attempt.result != nil {
			return attempt.result, attempt.candidateCount, attempt.topK, attempt.loadSkew, nil
		}
		if attempt.err != nil && !attempt.noCompactCandidates {
			return nil, attempt.candidateCount, attempt.topK, attempt.loadSkew, attempt.err
		}
		attempts = append(attempts, attempt)
		requests = append(requests, scoped)
	}
	// All pools were unable to acquire. Prefer waiting on the highest eligible
	// tier, and never invent an eligible candidate or revive a hard-cooled account.
	var lastErr error
	for i, attempt := range attempts {
		if attempt.err != nil {
			lastErr = attempt.err
			continue
		}
		result, count, topK, skew, err := s.finishLoadBalanceSelectionFallback(ctx, requests[i], attempt, budget, filters)
		if result != nil && err == nil {
			return result, count, topK, skew, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = ErrNoAvailableAccounts
	}
	return nil, 0, 0, 0, lastErr
}

// Ordinary API-key affinity is a cache preference, not server-side ownership.
// Probe OAuth without mutating the binding; adopt it only after acquiring a slot.
func (s *defaultOpenAIAccountScheduler) tryOAuthPreferredStickyEscape(ctx context.Context, req OpenAIAccountScheduleRequest, current *Account) *AccountSelectionResult {
	if !req.OAuthPriority || NormalizeOpenAICompatiblePlatform(req.Platform) != PlatformOpenAI || current == nil || current.IsOpenAIOAuth() || req.DisableStickyEscape || req.PreserveStickyBinding || req.GuardianParentAccountID > 0 || req.PreviousResponseID != "" || (req.RequiredTransport != OpenAIUpstreamTransportAny && req.RequiredTransport != OpenAIUpstreamTransportHTTPSSE) {
		return nil
	}
	probe := req
	probe.oauthOnly = true
	probe.StickyAccountID = 0
	probe.StickyPreviousAccountID = 0
	selected, _, _, _, err := s.selectByLoadBalance(ctx, probe)
	if err == nil && selected != nil && selected.Acquired && selected.Account != nil && selected.Account.IsOpenAIOAuth() {
		model := ResolveOpenAIAccountUpstreamModelForRequest(selected.Account, req.RequestedModel, req.RequireCompact)
		if !s.stats.modelOverloaded(selected.Account.ID, model, time.Now()) {
			return selected
		}
	}
	if selected != nil && selected.ReleaseFunc != nil {
		selected.ReleaseFunc()
	}
	return nil
}

// The legacy scheduler uses the same OAuth-first admission contract. Reuse its
// own eligibility/DB/slot checks; a preferred-pool probe never binds a session.
func (s *OpenAIGatewayService) tryLegacyOAuthPreference(ctx context.Context, group *int64, platform, session, model string, excluded map[int64]struct{}, compact bool, capability OpenAIEndpointCapability, tokenCost bool) (*AccountSelectionResult, error) {
	if NormalizeOpenAICompatiblePlatform(platform) != PlatformOpenAI || preserveOpenAIGuardianParentBinding(ctx, session) {
		return nil, nil
	}
	// Keep a usable OAuth binding warm, but do not let an invalid or full one
	// bypass the other OAuth candidates and jump straight to a mixed score pool.
	if session != "" && s.cache != nil {
		if id, err := s.getStickySessionAccountID(ctx, group, session); err == nil && id > 0 {
			account := s.tryStickySessionHit(ctx, group, platform, session, model, excluded, compact, id, capability)
			if account != nil && account.IsOpenAIOAuth() {
				if slot, slotErr := s.tryAcquireAccountSlot(ctx, account.ID, account.Concurrency); slotErr == nil && slot != nil && slot.Acquired {
					selected, selectErr := s.newAcquiredSelectionResult(ctx, account, slot.ReleaseFunc)
					if selectErr != nil {
						return nil, selectErr
					}
					_ = s.refreshStickySessionTTL(ctx, group, session, openaiStickySessionTTL)
					return markStickySessionHit(selected, true), nil
				}
			}
		}
	}
	accounts, err := s.listSchedulableAccounts(ctx, group, platform)
	if err != nil {
		return nil, err
	}
	preferredExclusions := cloneExcludedAccountIDs(excluded)
	if preferredExclusions == nil {
		preferredExclusions = make(map[int64]struct{})
	}
	count := 0
	for _, account := range accounts {
		if !account.IsOpenAIOAuth() {
			preferredExclusions[account.ID] = struct{}{}
			continue
		}
		if _, skip := preferredExclusions[account.ID]; !skip {
			count++
		}
	}
	attempts := count
	if s.concurrencyService != nil && s.schedulingConfig().LoadBatchEnabled && attempts > 1 {
		// The batch implementation already probes every eligible OAuth candidate.
		attempts = 1
	}
	for i := 0; i < attempts; i++ {
		selected, err := s.selectAccountWithLoadAwarenessPool(ctx, group, platform, "", model, preferredExclusions, compact, capability, tokenCost)
		if err != nil {
			if errors.Is(err, ErrNoAvailableAccounts) || errors.Is(err, ErrNoAvailableCompactAccounts) {
				return nil, nil
			}
			return nil, err
		}
		if selected == nil || selected.Account == nil {
			return nil, nil
		}
		if selected.Acquired {
			if session != "" {
				_ = s.bindOpenAIStickySessionDuringSelection(ctx, group, session, selected.Account.ID)
			}
			return selected, nil
		}
		if selected.ReleaseFunc != nil {
			selected.ReleaseFunc()
		}
		preferredExclusions[selected.Account.ID] = struct{}{}
	}
	return nil, nil
}
