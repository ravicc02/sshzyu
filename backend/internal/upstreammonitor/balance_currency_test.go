package upstreammonitor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNewAPIBalanceDisplayCurrencies(t *testing.T) {
	for _, tt := range []struct {
		name, quota, settings, amount, currency string
	}{
		{"USD", "2500000", `{"quota_per_unit":500000,"quota_display_type":"USD"}`, "5", "USD"},
		{"USD ignores display rates", "2500000", `{"quota_per_unit":500000,"quota_display_type":"USD","usd_exchange_rate":7,"custom_currency_exchange_rate":8}`, "5", "USD"},
		{"CNY", "2500000", `{"quota_per_unit":500000,"quota_display_type":"CNY","usd_exchange_rate":7.2}`, "36", "CNY"},
		{"Seedance CNY", "2500000", `{"quota_per_unit":500000,"quota_display_type":"CNY","usd_exchange_rate":1}`, "5", "CNY"},
		{"custom HKD", "2500000", `{"quota_per_unit":500000,"quota_display_type":"CUSTOM","custom_currency_symbol":"HK$","custom_currency_exchange_rate":7.8,"usd_exchange_rate":7.2}`, "39", "HK$"},
		{"custom EUR", "2500000", `{"quota_per_unit":500000,"quota_display_type":"CUSTOM","custom_currency_symbol":"\u20ac","custom_currency_exchange_rate":0.92}`, "4.6", "\u20ac"},
		{"custom JPY", "2500000", `{"quota_per_unit":500000,"quota_display_type":"CUSTOM","custom_currency_symbol":"JPY","custom_currency_exchange_rate":150}`, "750", "JPY"},
		{"string numbers", `"2500000"`, `{"quota_per_unit":"500000","quota_display_type":"CNY","usd_exchange_rate":"7.2"}`, "36", "CNY"},
		{"normalized display type", "2500000", `{"quota_per_unit":500000,"quota_display_type":" cny ","usd_exchange_rate":1}`, "5", "CNY"},
		{"zero", "0", `{"quota_per_unit":500000,"quota_display_type":"CNY","usd_exchange_rate":7.2}`, "0", "CNY"},
		{"negative balance", "-2500000", `{"quota_per_unit":500000,"quota_display_type":"CNY","usd_exchange_rate":7.2}`, "-36", "CNY"},
		{"round once after conversion", "1", `{"quota_per_unit":3,"quota_display_type":"CNY","usd_exchange_rate":3}`, "1", "CNY"},
		{"fraction precision", "1", `{"quota_per_unit":3,"quota_display_type":"USD"}`, "0.333333333333", "USD"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNewAPIBalance(json.RawMessage(tt.quota), data(tt.settings))
			if err != nil || got.Amount != tt.amount || got.Currency != tt.currency {
				t.Fatalf("reading=%+v, error=%v", got, err)
			}
			if _, err := FeishuContent(Delivery{Name: "fixture", Amount: got.Amount, Currency: got.Currency, RechargeURL: "https://example.com"}); err != nil {
				t.Fatalf("currency cannot be used in an alert: %v", err)
			}
		})
	}
}

func TestNewAPIBalanceRejectsUnconfirmedConversions(t *testing.T) {
	for _, tt := range []struct{ name, quota, settings, code string }{
		{"missing balance", `null`, `{"quota_per_unit":500000,"quota_display_type":"USD"}`, "MONITOR_BALANCE_MISSING"},
		{"missing quota divisor", "1", `{"quota_display_type":"USD"}`, "MONITOR_QUOTA_CONVERSION_INVALID"},
		{"zero quota divisor", "1", `{"quota_per_unit":0,"quota_display_type":"USD"}`, "MONITOR_QUOTA_CONVERSION_INVALID"},
		{"negative quota divisor", "1", `{"quota_per_unit":-1,"quota_display_type":"USD"}`, "MONITOR_QUOTA_CONVERSION_INVALID"},
		{"missing CNY rate", "1", `{"quota_per_unit":500000,"quota_display_type":"CNY"}`, "MONITOR_CURRENCY_RATE_INVALID"},
		{"zero CNY rate", "1", `{"quota_per_unit":500000,"quota_display_type":"CNY","usd_exchange_rate":0}`, "MONITOR_CURRENCY_RATE_INVALID"},
		{"negative CNY rate", "1", `{"quota_per_unit":500000,"quota_display_type":"CNY","usd_exchange_rate":-7}`, "MONITOR_CURRENCY_RATE_INVALID"},
		{"nonfinite rate", "1", `{"quota_per_unit":500000,"quota_display_type":"CNY","usd_exchange_rate":"NaN"}`, "MONITOR_CURRENCY_RATE_INVALID"},
		{"unbounded exponent", "1", `{"quota_per_unit":500000,"quota_display_type":"CNY","usd_exchange_rate":1e1000000000}`, "MONITOR_CURRENCY_RATE_INVALID"},
		{"missing custom rate", "1", `{"quota_per_unit":500000,"quota_display_type":"CUSTOM","custom_currency_symbol":"EUR","usd_exchange_rate":7}`, "MONITOR_CURRENCY_RATE_INVALID"},
		{"missing custom symbol", "1", `{"quota_per_unit":500000,"quota_display_type":"CUSTOM","custom_currency_exchange_rate":1}`, "MONITOR_CURRENCY_SYMBOL_INVALID"},
		{"multiline custom symbol", "1", `{"quota_per_unit":500000,"quota_display_type":"CUSTOM","custom_currency_symbol":"USD\nCNY","custom_currency_exchange_rate":1}`, "MONITOR_CURRENCY_SYMBOL_INVALID"},
		{"unsafe custom symbol", "1", `{"quota_per_unit":500000,"quota_display_type":"CUSTOM","custom_currency_symbol":"<script>","custom_currency_exchange_rate":1}`, "MONITOR_CURRENCY_SYMBOL_INVALID"},
		{"long custom symbol", "1", `{"quota_per_unit":500000,"quota_display_type":"CUSTOM","custom_currency_symbol":"abcdefghijklmnopq","custom_currency_exchange_rate":1}`, "MONITOR_CURRENCY_SYMBOL_INVALID"},
		{"unknown currency", "1", `{"quota_per_unit":500000,"quota_display_type":"EUR","usd_exchange_rate":1}`, "MONITOR_CURRENCY_UNCONFIRMED"},
		{"legacy flag does not prove currency", "1", `{"quota_per_unit":500000,"display_in_currency":true,"usd_exchange_rate":7}`, "MONITOR_CURRENCY_UNCONFIRMED"},
		{"tokens are not money", "1", `{"quota_per_unit":500000,"quota_display_type":"TOKENS"}`, "MONITOR_CURRENCY_TOKENS_ONLY"},
		{"converted amount overflow", "1e23", `{"quota_per_unit":1,"quota_display_type":"CNY","usd_exchange_rate":2}`, "MONITOR_BALANCE_MISSING"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNewAPIBalance(json.RawMessage(tt.quota), data(tt.settings))
			if err == nil || Code(err) != tt.code || got != (Reading{}) {
				t.Fatalf("reading=%+v, code=%s, expected=%s", got, Code(err), tt.code)
			}
		})
	}
}

func TestNewAPICurrencyFetchKeepsCredentialsOnAccountEndpoint(t *testing.T) {
	for _, accountToken := range []bool{false, true} {
		t.Run(map[bool]string{false: "account key", true: "account token"}[accountToken], func(t *testing.T) {
			site := Site{Provider: "newapi", AccountID: 42, BaseURL: "https://example.com", UserID: "123"}
			s := New(nil, nil, nil, "fixture-master")
			if accountToken {
				var err error
				site.authCipher, err = seal(s.master, "fixture-account-token")
				if err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			s.Fetcher = fetchFunc(func(_ context.Context, id int64, base, path string, credential Credential) (*http.Response, error) {
				calls++
				if id != site.AccountID || base != site.BaseURL {
					t.Fatal("account binding changed")
				}
				var body string
				switch path {
				case "/api/status":
					if credential != (Credential{}) {
						t.Fatal("credential sent to public settings")
					}
					body = `{"success":true,"data":{"quota_per_unit":500000,"quota_display_type":"CNY","usd_exchange_rate":7.2}}`
				case "/api/user/self":
					if !accountToken || credential.AccessToken != "fixture-account-token" || credential.UserID != site.UserID || credential.UseAccountKey {
						t.Fatal("incorrect account token selection")
					}
					body = `{"success":true,"data":{"quota":2500000}}`
				case "/api/usage/token/":
					if accountToken || !credential.UseAccountKey || credential.AccessToken != "" {
						t.Fatal("incorrect account key selection")
					}
					body = `{"success":true,"data":{"user_quota":{"total_available":2500000}}}`
				default:
					t.Fatal("unexpected endpoint")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			got, err := s.fetch(context.Background(), site)
			if err != nil || got != (Reading{Amount: "36", Currency: "CNY"}) || calls != 2 {
				t.Fatalf("reading=%+v, error=%v, requests=%d", got, err, calls)
			}
		})
	}
}
