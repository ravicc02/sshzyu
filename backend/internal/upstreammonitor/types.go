package upstreammonitor

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/shopspring/decimal"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultThreshold is expressed in the upstream account balance currency.
const DefaultThreshold = "5"

type Failure struct {
	Code       string
	Retryable  bool
	RetryAfter time.Duration
}

func (e *Failure) Error() string { return e.Code }
func Fail(code string) error     { return &Failure{Code: code} }
func Code(err error) string {
	var e *Failure
	if errors.As(err, &e) {
		return e.Code
	}
	return "MONITOR_INTERNAL_ERROR"
}

type Group struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
type Account struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Status   string `json:"status"`
}
type Candidate struct {
	ID        string    `json:"id"`
	BaseURL   string    `json:"base_url"`
	Provider  string    `json:"provider_type"`
	Accounts  []Account `json:"accounts"`
	Groups    []Group   `json:"groups"`
	Monitored bool      `json:"monitored"`
}
type Site struct {
	ID             string     `json:"id"`
	BaseURL        string     `json:"base_url"`
	Provider       string     `json:"provider_type"`
	AccountID      int64      `json:"account_id"`
	Name           string     `json:"display_name"`
	Enabled        bool       `json:"enabled"`
	WarningEnabled bool       `json:"warning_enabled"`
	Threshold      *string    `json:"threshold"`
	RechargeURL    string     `json:"recharge_url"`
	HasAccessToken bool       `json:"has_access_token"`
	UserID         string     `json:"upstream_user_id"`
	Amount         *string    `json:"amount"`
	Currency       string     `json:"currency"`
	Status         string     `json:"status"`
	ErrorCode      string     `json:"error_code"`
	LastSuccess    *time.Time `json:"last_success_at"`
	LastAttempt    *time.Time `json:"last_attempt_at"`
	NextCheck      *time.Time `json:"next_check_at"`
	AlertActive    bool       `json:"alert_active"`
	Revision       int64      `json:"revision"`
	Groups         []Group    `json:"groups"`
	authCipher     string
	failureCount   int
	recoveryCount  int
	episodeID      string
}
type SiteInput struct {
	CandidateID      string  `json:"candidate_id"`
	Provider         string  `json:"provider_type"`
	AccountID        int64   `json:"account_id"`
	Name             string  `json:"display_name"`
	Enabled          bool    `json:"enabled"`
	WarningEnabled   bool    `json:"warning_enabled"`
	Threshold        *string `json:"threshold"`
	RechargeURL      string  `json:"recharge_url"`
	AccessToken      string  `json:"access_token"`
	ClearAccessToken bool    `json:"clear_access_token"`
	UserID           string  `json:"upstream_user_id"`
	Revision         int64   `json:"revision"`
}
type Event struct {
	ID        int64           `json:"id"`
	SiteID    *string         `json:"site_id"`
	Kind      string          `json:"kind"`
	Level     string          `json:"level"`
	Message   string          `json:"message"`
	Details   json.RawMessage `json:"details"`
	CreatedAt time.Time       `json:"created_at"`
}
type EventsPage struct {
	Items    []Event `json:"items"`
	Total    int64   `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
}
type Delivery struct {
	ID          string  `json:"id"`
	SiteID      *string `json:"site_id"`
	EpisodeID   string  `json:"episode_id"`
	Revision    int64   `json:"site_revision"`
	Name        string  `json:"display_name"`
	Amount      string  `json:"amount"`
	Currency    string  `json:"currency"`
	RechargeURL string  `json:"recharge_url"`
	Test        bool    `json:"is_test"`
	Status      string  `json:"status"`
	Attempts    int     `json:"attempts"`
	MessageID   string  `json:"message_id"`
	ErrorCode   string  `json:"error_code"`
}
type Channel struct {
	AppID     string `json:"app_id"`
	ChatID    string `json:"chat_id"`
	HasSecret bool   `json:"has_secret"`
	Revision  int64  `json:"revision"`
	secret    string
}
type ChannelInput struct {
	AppID    string `json:"app_id"`
	ChatID   string `json:"chat_id"`
	Secret   string `json:"app_secret"`
	Revision int64  `json:"revision"`
}
type Credential struct {
	AccessToken   string
	UserID        string
	UseAccountKey bool
}
type Fetcher interface {
	Get(context.Context, int64, string, string, Credential) (*http.Response, error)
}
type Sender interface {
	Send(context.Context, Channel, Delivery) (string, error)
}

type Reading struct {
	Amount   string
	Currency string
}

func CanonicalURL(raw string) (string, error) {
	u, e := url.Parse(strings.TrimSpace(raw))
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", Fail("MONITOR_INVALID_URL")
	}
	u.Host = strings.ToLower(u.Host)
	if strings.HasSuffix(u.Host, ":443") {
		u.Host = strings.TrimSuffix(u.Host, ":443")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}
func RechargeURL(raw string) (string, error) {
	u, e := url.Parse(strings.TrimSpace(raw))
	if e != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return "", Fail("MONITOR_INVALID_RECHARGE_URL")
	}
	for key := range u.Query() {
		k := strings.ToLower(key)
		if strings.Contains(k, "token") || strings.Contains(k, "secret") || strings.Contains(k, "key") {
			return "", Fail("MONITOR_INVALID_RECHARGE_URL")
		}
	}
	return u.String(), nil
}
func Endpoint(base, path string) string {
	u, _ := url.Parse(base)
	p := strings.TrimRight(u.Path, "/")
	for _, s := range []string{"/anthropic", "/v1"} {
		if strings.HasSuffix(p, s) {
			p = strings.TrimSuffix(p, s)
			break
		}
	}
	u.Path = p + strings.SplitN(path, "?", 2)[0]
	u.RawQuery = ""
	if i := strings.IndexByte(path, '?'); i >= 0 {
		u.RawQuery = path[i+1:]
	}
	return u.String()
}
func ParseAmount(raw string) (string, error) {
	if idx := strings.IndexAny(raw, "eE"); idx >= 0 {
		exponent, err := strconv.Atoi(raw[idx+1:])
		if err != nil || exponent < -36 || exponent > 36 {
			return "", Fail("MONITOR_INVALID_AMOUNT")
		}
	}
	if len(raw) > 64 {
		return "", Fail("MONITOR_INVALID_AMOUNT")
	}
	d, e := decimal.NewFromString(raw)
	if e != nil || d.Exponent() < -12 || d.Abs().GreaterThan(decimal.New(1, 23)) {
		return "", Fail("MONITOR_INVALID_AMOUNT")
	}
	return d.String(), nil
}
func Below(amount, threshold string) bool {
	a, e := decimal.NewFromString(amount)
	b, f := decimal.NewFromString(threshold)
	return e == nil && f == nil && a.LessThan(b)
}
func seal(master, plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if master == "" {
		return "", Fail("MONITOR_SECRET_STORAGE_UNAVAILABLE")
	}
	key := sha256.Sum256([]byte("upstream-monitor-v1:" + master))
	block, e := aes.NewCipher(key[:])
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(block)
	if e != nil {
		return "", e
	}
	nonce := make([]byte, g.NonceSize())
	if _, e = io.ReadFull(rand.Reader, nonce); e != nil {
		return "", e
	}
	return base64.StdEncoding.EncodeToString(g.Seal(nonce, nonce, []byte(plain), nil)), nil
}
func unseal(master, encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	if master == "" {
		return "", Fail("MONITOR_SECRET_STORAGE_UNAVAILABLE")
	}
	key := sha256.Sum256([]byte("upstream-monitor-v1:" + master))
	block, _ := aes.NewCipher(key[:])
	g, _ := cipher.NewGCM(block)
	raw, e := base64.StdEncoding.DecodeString(encoded)
	if e != nil || len(raw) < g.NonceSize() {
		return "", Fail("MONITOR_SECRET_DECRYPT_FAILED")
	}
	plain, e := g.Open(nil, raw[:g.NonceSize()], raw[g.NonceSize():], nil)
	if e != nil {
		return "", Fail("MONITOR_SECRET_DECRYPT_FAILED")
	}
	return string(plain), nil
}
