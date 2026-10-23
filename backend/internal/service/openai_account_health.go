package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

const (
	openAIAccountHealthWindow           = 2 * time.Minute
	openAIAccountHealthCooldown         = 5 * time.Minute
	openAIAccountHealthFailureThreshold = 3
	openAIAccountHealthGatewayThreshold = 2
	openAIAccountHealthMaxEntries       = 8192
	openAIAccountHealthReason           = "openai_account_health_cooldown"
)

type openAIAccountHealthEntry struct {
	// Only the last threshold observations are needed for an exact rolling window.
	failures        [openAIAccountHealthFailureThreshold]time.Time
	gatewayFailures [openAIAccountHealthGatewayThreshold]time.Time
	blockedUntil    time.Time
	touchedAt       time.Time
}

type openAIAccountHealthCircuit struct {
	mu      sync.Mutex
	entries map[int64]openAIAccountHealthEntry
	// At saturation, keep all active cooldowns and conservatively reject unknown
	// accounts until the earliest entry expires. Never grow or evict a live block.
	overflowUntil time.Time
}

func (s *OpenAIGatewayService) getOpenAIAccountHealthCircuit() *openAIAccountHealthCircuit {
	s.openaiAccountHealthOnce.Do(func() {
		s.openaiAccountHealth = &openAIAccountHealthCircuit{entries: make(map[int64]openAIAccountHealthEntry)}
	})
	return s.openaiAccountHealth
}

func appendOpenAIHealthFailure(times []time.Time, now time.Time) int {
	copy(times, times[1:])
	times[len(times)-1] = now
	count := 0
	for _, at := range times {
		if !at.IsZero() && !at.After(now) && now.Sub(at) < openAIAccountHealthWindow {
			count++
		}
	}
	return count
}

func (c *openAIAccountHealthCircuit) record(accountID int64, status int, now time.Time) (tripped bool, until time.Time, count, threshold int) {
	if status == http.StatusServiceUnavailable {
		return false, time.Time{}, 0, 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, exists := c.entries[accountID]
	if exists && now.Before(entry.blockedUntil) {
		return false, entry.blockedUntil, 0, 0
	}
	if !exists && len(c.entries) >= openAIAccountHealthMaxEntries {
		var oldestID int64
		var oldest, earliestBlock time.Time
		for id, e := range c.entries {
			if now.Before(e.blockedUntil) {
				if earliestBlock.IsZero() || e.blockedUntil.Before(earliestBlock) {
					earliestBlock = e.blockedUntil
				}
				continue
			}
			if oldest.IsZero() || e.touchedAt.Before(oldest) {
				oldestID, oldest = id, e.touchedAt
			}
		}
		if oldest.IsZero() {
			c.overflowUntil = earliestBlock
			return false, earliestBlock, 0, 0
		}
		delete(c.entries, oldestID)
	}
	// Expired cooldowns start a fresh observation window. Older successful
	// requests never clear cooldowns or wipe failures from the rolling window.
	if !entry.blockedUntil.IsZero() {
		entry = openAIAccountHealthEntry{}
	}
	entry.touchedAt = now
	count = appendOpenAIHealthFailure(entry.failures[:], now)
	threshold = openAIAccountHealthFailureThreshold
	if status == http.StatusBadGateway {
		n := appendOpenAIHealthFailure(entry.gatewayFailures[:], now)
		if n >= openAIAccountHealthGatewayThreshold {
			count, threshold = n, openAIAccountHealthGatewayThreshold
		}
	}
	if count >= threshold {
		entry.blockedUntil = now.Add(openAIAccountHealthCooldown)
		tripped = true
	}
	c.entries[accountID] = entry
	return tripped, entry.blockedUntil, count, threshold
}

func (c *openAIAccountHealthCircuit) blocked(accountID int64, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, exists := c.entries[accountID]
	if !exists {
		return now.Before(c.overflowUntil)
	}
	return now.Before(entry.blockedUntil)
}

// Congestion is a temporary inability to serve this model, not evidence that
// the selected credential is unhealthy. Keep both account breakers consistent.
func isOpenAIAccountCongestionFailure(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var failure *UpstreamFailoverError
	if errors.As(err, &failure) {
		if failure.IsCredentialFailure() {
			return false
		}
		return failure.StatusCode == http.StatusServiceUnavailable || failure.IsOpenAICapacityShed()
	}
	var image *OpenAIImagesUpstreamError
	if errors.As(err, &image) {
		return image.StatusCode == http.StatusServiceUnavailable || isOpenAIRequestScopedCapacityShed(image.Message, []byte(image.Message))
	}
	return false
}

// Health reflects failures of the selected inference account. Request payload,
// credentials, cancellations and dedicated quota policies have their own paths.
func classifyOpenAIAccountHealthFailure(err error) (int, bool) {
	if isOpenAIAccountCongestionFailure(err) {
		return 0, false
	}
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return 0, false
	}
	var failure *UpstreamFailoverError
	if errors.As(err, &failure) {
		if failure.IsCredentialFailure() || failure.Scope == GatewayFailureScopeProvider || failure.Scope == GatewayFailureScopeRequest {
			return 0, false
		}
		status := failure.StatusCode
		if status < 500 || status > 599 {
			return 0, false
		}
		// Congestion was excluded above; remaining request-scoped failures
		// must not be promoted into credential/account failures either.
		if failure.RequestScopedTransient {
			return 0, false
		}
		return status, true
	}
	var image *OpenAIImagesUpstreamError
	if errors.As(err, &image) && image.StatusCode >= 500 && image.StatusCode <= 599 {
		return image.StatusCode, true
	}
	return 0, false
}

func (s *OpenAIGatewayService) observeOpenAIAccountHealthFailure(account *Account, err error) bool {
	if s == nil || account == nil || account.ID <= 0 || account.Platform != PlatformOpenAI {
		return false
	}
	if isOpenAIAccountCongestionFailure(err) {
		var failure *UpstreamFailoverError
		if errors.As(err, &failure) && failure.StatusCode == http.StatusServiceUnavailable {
			failure.RetryableOnSameAccount = false
		}
		return false
	}
	status, eligible := classifyOpenAIAccountHealthFailure(err)
	if !eligible {
		return false
	}
	tripped, until, count, threshold := s.getOpenAIAccountHealthCircuit().record(account.ID, status, time.Now())
	var failure *UpstreamFailoverError
	// Even the first gateway error moves the current request to another account.
	// A tripped account also suppresses explicit pool retries for other 5xx.
	if errors.As(err, &failure) && (status == http.StatusBadGateway || !until.IsZero()) {
		failure.RetryableOnSameAccount = false
	}
	if !tripped {
		return !until.IsZero()
	}
	s.BlockAccountScheduling(account, until, openAIAccountHealthReason)
	state := &TempUnschedState{
		UntilUnix: until.Unix(), TriggeredAtUnix: time.Now().Unix(), StatusCode: status,
		MatchedKeyword: openAIAccountHealthReason, RuleIndex: -1,
		TriggerCount: int64(count), TriggerThreshold: threshold, TriggerWindowMinutes: 2,
	}
	// Reuse the database/outbox and shared scheduling-cache contract. Local
	// exclusion is already installed, including if persistence is unavailable.
	repo := s.accountRepo
	if repo == nil && s.rateLimitService != nil {
		repo = s.rateLimitService.accountRepo
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if repo != nil {
		reason, _ := json.Marshal(state)
		if persistErr := repo.SetTempUnschedulable(ctx, account.ID, until, string(reason)); persistErr != nil {
			slog.Warn("openai_account_health_persist_failed", "account_id", account.ID, "error", persistErr)
		} else if s.rateLimitService != nil && s.rateLimitService.tempUnschedCache != nil {
			// Do not shorten an existing longer cooldown in the shared cache.
			cached, cacheErr := s.rateLimitService.tempUnschedCache.GetTempUnsched(ctx, account.ID)
			if cacheErr == nil && (cached == nil || cached.UntilUnix < state.UntilUnix) {
				if cacheErr := s.rateLimitService.tempUnschedCache.SetTempUnsched(ctx, account.ID, state); cacheErr != nil {
					slog.Warn("openai_account_health_cache_failed", "account_id", account.ID, "error", cacheErr)
				}
			}
		}
	}
	slog.Warn("openai_account_health_cooldown", "account_id", account.ID, "status", status, "failures", count, "threshold", threshold, "until", until)
	return true
}
