package service

import (
	"context"
	"database/sql"
	"github.com/Wei-Shaw/sub2api/internal/upstreammonitor"
	"net/http"
	"strings"
)

type balanceMonitorTransport struct {
	accounts AccountRepository
	tests    *AccountTestService
}

func (t *balanceMonitorTransport) Get(ctx context.Context, accountID int64, base, path string, credential upstreammonitor.Credential) (*http.Response, error) {
	a, e := t.accounts.GetByID(ctx, accountID)
	if e != nil || a == nil || a.Type != AccountTypeAPIKey {
		return nil, upstreammonitor.Fail("MONITOR_ACCOUNT_BINDING_CHANGED")
	}
	current, e := upstreammonitor.CanonicalURL(a.GetCredential("base_url"))
	if e != nil || current != base {
		return nil, upstreammonitor.Fail("MONITOR_ACCOUNT_BINDING_CHANGED")
	}
	target := upstreammonitor.Endpoint(base, path)
	validated, e := cnValidateProbeURL(t.tests.cfg, target)
	if e != nil {
		return nil, upstreammonitor.Fail("MONITOR_URL_POLICY_REJECTED")
	}
	req, e := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(ctx), http.MethodGet, validated, nil)
	if e != nil {
		return nil, upstreammonitor.Fail("MONITOR_INVALID_URL")
	}
	if credential.UseAccountKey || credential.AccessToken != "" {
		a.ApplyHeaderOverrides(req.Header)
		key := credential.AccessToken
		if credential.UseAccountKey {
			key = strings.TrimSpace(a.GetCredential("api_key"))
		}
		if key == "" || strings.ContainsAny(key, "\r\n\x00") {
			return nil, upstreammonitor.Fail("MONITOR_AUTH_REQUIRED")
		}
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Accept", "application/json")
	if credential.UserID != "" {
		req.Header.Set("New-Api-User", credential.UserID)
	}
	proxyURL := ""
	if a.ProxyID != nil {
		if a.Proxy == nil || a.Proxy.ID != *a.ProxyID {
			return nil, upstreammonitor.Fail("MONITOR_PROXY_UNAVAILABLE")
		}
		proxyURL = a.Proxy.URL()
	}
	response, e := t.tests.httpUpstream.Do(req, proxyURL, a.ID, maxInt(a.Concurrency, 1))
	if e != nil {
		return nil, &upstreammonitor.Failure{Code: "MONITOR_NETWORK_ERROR", Retryable: true}
	}
	return response, nil
}
func newUpstreamBalanceMonitor(db *sql.DB, accounts AccountRepository, tests *AccountTestService) *upstreammonitor.Service {
	if db == nil || accounts == nil || tests == nil || tests.httpUpstream == nil {
		return nil
	}
	master := ""
	if tests.cfg != nil {
		master = tests.cfg.JWT.Secret
	}
	return upstreammonitor.New(db, &balanceMonitorTransport{accounts: accounts, tests: tests}, upstreammonitor.NewFeishu(nil), master)
}
func (s *UpstreamBillingProbeService) BalanceMonitor() *upstreammonitor.Service {
	if s == nil {
		return nil
	}
	return s.balanceMonitor
}
