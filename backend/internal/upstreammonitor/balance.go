package upstreammonitor

import (
	"context"
	"encoding/json"
	"github.com/shopspring/decimal"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func responseJSON(response *http.Response) (map[string]json.RawMessage, error) {
	if response == nil || response.Body == nil {
		return nil, Fail("MONITOR_EMPTY_RESPONSE")
	}
	defer response.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(response.Body, (256<<10)+1))
	if e != nil {
		return nil, Fail("MONITOR_RESPONSE_INTERRUPTED")
	}
	if len(raw) > 256<<10 {
		return nil, Fail("MONITOR_RESPONSE_TOO_LARGE")
	}
	if response.StatusCode == 429 {
		return nil, &Failure{Code: "MONITOR_RATE_LIMITED", Retryable: true, RetryAfter: retryAfter(response.Header.Get("Retry-After"))}
	}
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return nil, Fail("MONITOR_AUTH_REQUIRED")
	}
	if response.StatusCode == 404 {
		return nil, Fail("MONITOR_ENDPOINT_UNAVAILABLE")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &Failure{Code: "MONITOR_HTTP_ERROR", Retryable: response.StatusCode >= 500}
	}
	var data map[string]json.RawMessage
	if json.Unmarshal(raw, &data) != nil || data == nil {
		return nil, Fail("MONITOR_INVALID_RESPONSE")
	}
	if string(data["success"]) == "false" || string(data["code"]) == "false" {
		return nil, Fail("MONITOR_API_REJECTED")
	}
	if code := string(data["code"]); code != "" && code != "true" && code != "0" && code != "200" {
		return nil, Fail("MONITOR_API_REJECTED")
	}
	if _, ok := data["error"]; ok {
		return nil, Fail("MONITOR_API_REJECTED")
	}
	return data, nil
}
func retryAfter(raw string) time.Duration {
	if seconds, e := strconv.Atoi(raw); e == nil && seconds > 0 {
		if seconds > 3600 {
			seconds = 3600
		}
		return time.Duration(seconds) * time.Second
	}
	if when, e := http.ParseTime(raw); e == nil {
		d := time.Until(when)
		if d > time.Hour {
			return time.Hour
		}
		if d > 0 {
			return d
		}
	}
	return time.Minute
}
func str(raw json.RawMessage) string { var s string; _ = json.Unmarshal(raw, &s); return s }
func obj(raw json.RawMessage) map[string]json.RawMessage {
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	return m
}
func number(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "true" || string(raw) == "false" {
		return "", Fail("MONITOR_BALANCE_MISSING")
	}
	value := string(raw)
	if raw[0] == '"' {
		value = str(raw)
	}
	amount, e := ParseAmount(value)
	if e != nil {
		return "", Fail("MONITOR_BALANCE_MISSING")
	}
	return amount, nil
}
func safeCurrency(raw string) (string, error) {
	if len(raw) < 1 || len(raw) > 16 {
		return "", Fail("MONITOR_CURRENCY_MISSING")
	}
	for _, r := range raw {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r == '_') {
			return "", Fail("MONITOR_CURRENCY_MISSING")
		}
	}
	return raw, nil
}
func ParseSub2(data map[string]json.RawMessage) (Reading, error) {
	if string(data["isValid"]) == "false" {
		return Reading{}, Fail("MONITOR_AUTH_REQUIRED")
	}
	if str(data["mode"]) != "unrestricted" {
		return Reading{}, Fail("MONITOR_ACCOUNT_BALANCE_NOT_EXPOSED")
	}
	if sub, ok := data["subscription"]; ok && string(sub) != "null" {
		return Reading{}, Fail("MONITOR_ACCOUNT_BALANCE_NOT_EXPOSED")
	}
	amount, e := number(data["balance"])
	if e != nil && str(data["planName"]) == "钱包余额" {
		amount, e = number(data["remaining"])
	}
	if e != nil {
		return Reading{}, e
	}
	unit, e := safeCurrency(str(data["unit"]))
	if e != nil {
		return Reading{}, e
	}
	return Reading{Amount: amount, Currency: unit}, nil
}
func (s *Service) get(ctx context.Context, site Site, path string, credential Credential) (map[string]json.RawMessage, error) {
	if s.Fetcher == nil {
		return nil, Fail("MONITOR_TRANSPORT_UNAVAILABLE")
	}
	response, e := s.Fetcher.Get(ctx, site.AccountID, site.BaseURL, path, credential)
	if e != nil {
		return nil, e
	}
	return responseJSON(response)
}
func (s *Service) fetch(ctx context.Context, site Site) (Reading, error) {
	if site.Provider == "sub2api" {
		today := time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
		query := url.Values{"days": {"1"}, "start_date": {today}, "end_date": {today}}
		data, e := s.get(ctx, site, "/v1/usage?"+query.Encode(), Credential{UseAccountKey: true})
		if e != nil {
			return Reading{}, e
		}
		return ParseSub2(data)
	}
	var rawQuota json.RawMessage
	if site.authCipher != "" {
		token, e := unseal(s.master, site.authCipher)
		if e != nil {
			return Reading{}, e
		}
		data, e := s.get(ctx, site, "/api/user/self", Credential{AccessToken: token, UserID: site.UserID})
		if e != nil {
			return Reading{}, e
		}
		if string(data["success"]) != "true" {
			return Reading{}, Fail("MONITOR_AUTH_REQUIRED")
		}
		rawQuota = obj(data["data"])["quota"]
	} else {
		data, e := s.get(ctx, site, "/api/usage/token/", Credential{UseAccountKey: true})
		if e != nil {
			return Reading{}, e
		}
		rawQuota = obj(obj(data["data"])["user_quota"])["total_available"]
		if len(rawQuota) == 0 {
			return Reading{}, Fail("MONITOR_ACCOUNT_CREDENTIAL_REQUIRED")
		}
	}
	quota, e := number(rawQuota)
	if e != nil {
		return Reading{}, e
	}
	public, e := s.get(ctx, site, "/api/status", Credential{})
	if e != nil {
		return Reading{}, e
	}
	settings := obj(public["data"])
	factor, e := number(settings["quota_per_unit"])
	if e != nil {
		return Reading{}, Fail("MONITOR_CURRENCY_MISSING")
	}
	f, _ := decimal.NewFromString(factor)
	if !f.IsPositive() {
		return Reading{}, Fail("MONITOR_CURRENCY_MISSING")
	}
	unit := str(settings["quota_display_type"])
	if unit != "USD" {
		return Reading{}, Fail("MONITOR_CURRENCY_UNCONFIRMED")
	}
	q, _ := decimal.NewFromString(quota)
	return Reading{Amount: q.Div(f).Round(12).String(), Currency: unit}, nil
}
func (s *Service) Discover(ctx context.Context, ids []string) ([]Candidate, error) {
	if len(ids) > 10 {
		return nil, Fail("MONITOR_TOO_MANY_TARGETS")
	}
	candidates, e := s.Candidates(ctx)
	if e != nil {
		return nil, e
	}
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	for _, c := range candidates {
		if !wanted[c.ID] || len(c.Accounts) == 0 {
			continue
		}
		site := Site{BaseURL: c.BaseURL, AccountID: c.Accounts[0].ID}
		kind := ""
		data, err := s.get(ctx, site, "/api/status", Credential{})
		if err == nil {
			m := obj(data["data"])
			if len(m["quota_per_unit"]) > 0 {
				kind = "newapi"
			}
		}
		if kind == "" {
			data, err = s.get(ctx, site, "/api/v1/settings/public", Credential{})
			if err == nil {
				m := obj(data["data"])
				if len(m["site_name"]) > 0 {
					kind = "sub2api"
				}
			}
		}
		_ = s.event(ctx, nil, "identify", "info", "上游类型识别", map[string]any{"base_url": c.BaseURL, "provider_type": kind, "identified": kind != ""})
	}
	return s.Candidates(ctx)
}
func cleanName(name, base string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = base
	}
	if len([]rune(name)) > 120 {
		return string([]rune(name)[:120])
	}
	return name
}
