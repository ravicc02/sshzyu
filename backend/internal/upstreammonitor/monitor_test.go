package upstreammonitor

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func data(raw string) map[string]json.RawMessage {
	var d map[string]json.RawMessage
	_ = json.Unmarshal([]byte(raw), &d)
	return d
}
func TestAccountWalletParserRejectsQuotaAndMalformedResponses(t *testing.T) {
	for _, raw := range []string{`{}`, `{"mode":"quota_limited","remaining":999999}`, `{"mode":"unrestricted","isValid":false,"balance":0,"unit":"USD"}`, `{"mode":"unrestricted","remaining":10,"unit":"USD"}`, `{"mode":"unrestricted","balance":10,"unit":"USD","subscription":{}}`, `{"mode":"unrestricted","balance":"NaN","unit":"USD"}`, `{"mode":"unrestricted","balance":1e1000000000,"unit":"USD"}`} {
		if _, e := ParseSub2(data(raw)); e == nil {
			t.Fatal("accepted non-wallet response", raw)
		}
	}
	reading, e := ParseSub2(data(`{"mode":"unrestricted","balance":-0.00325001,"unit":"USD"}`))
	if e != nil || reading.Amount != "-0.00325001" {
		t.Fatal(reading, e)
	}
	if _, e = responseJSON(&http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`<html>not a balance</html>`))}); e == nil {
		t.Fatal("HTML accepted")
	}
}
func TestThresholdStrictComparisonAndBoundedAmounts(t *testing.T) {
	for _, pair := range [][2]string{{"10", "10"}, {"11", "10"}, {"0", "0"}} {
		if Below(pair[0], pair[1]) {
			t.Fatal(pair)
		}
	}
	if !Below("9.99", "10") || !Below("-0.00001", "0") {
		t.Fatal("low balance comparison failed")
	}
	for _, raw := range []string{"NaN", "Infinity", "1e1000000000", strings.Repeat("9", 100), "0.0000000000001"} {
		if _, e := ParseAmount(raw); e == nil {
			t.Fatal("unsafe amount", raw)
		}
	}
}
func TestSecretCipherNeverAppearsInPublicDTO(t *testing.T) {
	a, e := seal("master", "fixture-private-value")
	if e != nil {
		t.Fatal(e)
	}
	b, _ := seal("master", "fixture-private-value")
	if a == b || strings.Contains(a, "fixture-private-value") {
		t.Fatal("nonce missing or plaintext retained")
	}
	plain, e := unseal("master", a)
	if e != nil || plain != "fixture-private-value" {
		t.Fatal(e)
	}
	if _, e = unseal("wrong", a); e == nil {
		t.Fatal("accepted wrong encryption key")
	}
	raw, _ := json.Marshal(Channel{AppID: "cli_fixture", secret: "fixture-private-value", HasSecret: true})
	if strings.Contains(string(raw), "fixture-private-value") {
		t.Fatal("secret exposed")
	}
	raw, _ = json.Marshal(Site{authCipher: a})
	if strings.Contains(string(raw), a) {
		t.Fatal("credential ciphertext exposed")
	}
}
func TestCanonicalEndpointsAndRechargeSafety(t *testing.T) {
	base, e := CanonicalURL("https://EXAMPLE.com:443/team/v1/")
	if e != nil || base != "https://example.com/team/v1" {
		t.Fatal(base, e)
	}
	if Endpoint(base, "/v1/usage?days=1") != "https://example.com/team/v1/usage?days=1" {
		t.Fatal("prefix lost")
	}
	for _, raw := range []string{"javascript:alert(1)", "https://name:password@example.com", "https://example.com/?token=fixture"} {
		if _, e := RechargeURL(raw); e == nil {
			t.Fatal("unsafe recharge URL accepted", raw)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestFeishuUsesFourLinePostStableUUIDAndTokenCache(t *testing.T) {
	var tokens, messages atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "open.feishu.cn" {
			t.Fatal("unexpected origin")
		}
		raw, _ := io.ReadAll(req.Body)
		var in map[string]json.RawMessage
		_ = json.Unmarshal(raw, &in)
		body := `{"code":0,"tenant_access_token":"fixture-tenant-token","expire":7200}`
		if strings.Contains(req.URL.Path, "tenant_access_token") {
			tokens.Add(1)
			if str(in["app_secret"]) != "fixture-app-secret" {
				t.Fatal("missing credential")
			}
		} else {
			messages.Add(1)
			if req.Header.Get("Authorization") != "Bearer fixture-tenant-token" {
				t.Fatal("missing token")
			}
			if req.URL.Query().Get("receive_id_type") != "chat_id" || str(in["receive_id"]) != "oc_fixture" {
				t.Fatal("wrong recipient")
			}
			if str(in["uuid"]) == "" {
				t.Fatal("missing idempotency key")
			}
			var post map[string]struct {
				Title   string                `json:"title"`
				Content [][]map[string]string `json:"content"`
			}
			if json.Unmarshal([]byte(str(in["content"])), &post) != nil {
				t.Fatal("content is not a JSON string")
			}
			zh := post["zh_cn"]
			if zh.Title != "预警--" || len(zh.Content) != 3 || zh.Content[2][0]["tag"] != "a" || zh.Content[2][0]["text"] != "快速前往充值" {
				t.Fatal("template changed")
			}
			body = `{"code":0,"data":{"message_id":"om_fixture"}}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	sender := NewFeishu(client)
	channel := Channel{AppID: "cli_fixture", ChatID: "oc_fixture", secret: "fixture-app-secret"}
	d := Delivery{ID: uuid.NewString(), Name: "本地测试", Amount: "8.5", Currency: "USD", RechargeURL: "https://example.com"}
	for i := 0; i < 2; i++ {
		if id, e := sender.Send(context.Background(), channel, d); e != nil || id != "om_fixture" {
			t.Fatal(id, e)
		}
	}
	if tokens.Load() != 1 || messages.Load() != 2 {
		t.Fatal("token cache not reused")
	}
}
func TestFeishuRejectsBusinessErrorAndMissingReceiptWithoutLeakingBody(t *testing.T) {
	for _, body := range []string{`{"code":99991672,"msg":"fixture-private-value"}`, `{"code":0,"data":{}}`} {
		sender := NewFeishu(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			b := body
			if strings.Contains(r.URL.Path, "tenant_access_token") {
				b = `{"code":0,"tenant_access_token":"fixture","expire":7200}`
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(b))}, nil
		})})
		_, e := sender.Send(context.Background(), Channel{AppID: "cli_test", ChatID: "oc_test", secret: "fixture-private-value"}, Delivery{ID: uuid.NewString(), Name: "test", Amount: "1", Currency: "USD", RechargeURL: "https://example.com"})
		if e == nil || strings.Contains(e.Error(), "fixture-private-value") {
			t.Fatal("false success or secret leaked", e)
		}
	}
}
