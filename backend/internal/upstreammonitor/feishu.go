package upstreammonitor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Feishu uses only the fixed official origin. Credentials never become URLs.
type Feishu struct {
	client     *http.Client
	mu         sync.Mutex
	token      string
	tokenKey   string
	tokenUntil time.Time
}

func NewFeishu(client *http.Client) *Feishu {
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	copy := *client
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Feishu{client: &copy}
}
func (f *Feishu) request(ctx context.Context, path, token string, body any) (map[string]json.RawMessage, error) {
	raw, e := json.Marshal(body)
	if e != nil {
		return nil, Fail("MONITOR_FEISHU_PAYLOAD_INVALID")
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, "https://open.feishu.cn"+path, bytes.NewReader(raw))
	if e != nil {
		return nil, Fail("MONITOR_FEISHU_PAYLOAD_INVALID")
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, e := f.client.Do(req)
	if e != nil {
		return nil, &Failure{Code: "MONITOR_FEISHU_NETWORK_ERROR", Retryable: true}
	}
	defer resp.Body.Close()
	raw, e = io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if e != nil || len(raw) > 64<<10 {
		return nil, &Failure{Code: "MONITOR_FEISHU_RESPONSE_INVALID", Retryable: true}
	}
	if resp.StatusCode == 429 {
		return nil, &Failure{Code: "MONITOR_FEISHU_RATE_LIMITED", Retryable: true, RetryAfter: retryAfter(resp.Header.Get("Retry-After"))}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &Failure{Code: "MONITOR_FEISHU_HTTP_" + strconv.Itoa(resp.StatusCode), Retryable: resp.StatusCode >= 500}
	}
	var result map[string]json.RawMessage
	if json.Unmarshal(raw, &result) != nil {
		return nil, &Failure{Code: "MONITOR_FEISHU_RESPONSE_INVALID", Retryable: true}
	}
	var code int
	if len(result["code"]) == 0 || json.Unmarshal(result["code"], &code) != nil {
		return nil, Fail("MONITOR_FEISHU_RESPONSE_INVALID")
	}
	if code != 0 {
		return nil, &Failure{Code: "FEISHU_API_" + strconv.Itoa(code), Retryable: code == 99991400 || code == 99991401}
	}
	return result, nil
}
func (f *Feishu) accessToken(ctx context.Context, c Channel) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	hash := sha256.Sum256([]byte(c.AppID + "\x00" + c.secret))
	key := hex.EncodeToString(hash[:])
	if f.tokenKey == key && time.Now().Before(f.tokenUntil) {
		return f.token, nil
	}
	result, e := f.request(ctx, "/open-apis/auth/v3/tenant_access_token/internal", "", map[string]string{"app_id": c.AppID, "app_secret": c.secret})
	if e != nil {
		return "", e
	}
	token := str(result["tenant_access_token"])
	var expiry int
	_ = json.Unmarshal(result["expire"], &expiry)
	if token == "" || expiry <= 0 {
		return "", Fail("MONITOR_FEISHU_AUTH_INVALID")
	}
	ttl := time.Duration(expiry)*time.Second - time.Minute
	if ttl <= 0 {
		ttl = time.Second
	}
	f.tokenKey = key
	f.token = token
	f.tokenUntil = time.Now().Add(ttl)
	return token, nil
}
func FeishuContent(d Delivery) (string, error) {
	if _, e := RechargeURL(d.RechargeURL); e != nil {
		return "", e
	}
	if _, e := ParseAmount(d.Amount); e != nil {
		return "", e
	}
	if _, e := safeCurrency(d.Currency); e != nil {
		return "", e
	}
	post := map[string]any{"zh_cn": map[string]any{"title": "预警--", "content": [][]map[string]string{{{"tag": "text", "text": "上游：【" + d.Name + "】"}}, {{"tag": "text", "text": "余额：" + d.Amount + " " + d.Currency}}, {{"tag": "a", "text": "快速前往充值", "href": d.RechargeURL}}}}}
	raw, e := json.Marshal(post)
	return string(raw), e
}
func (f *Feishu) Send(ctx context.Context, c Channel, d Delivery) (string, error) {
	if c.AppID == "" || c.secret == "" || c.ChatID == "" {
		return "", Fail("MONITOR_FEISHU_NOT_CONFIGURED")
	}
	content, e := FeishuContent(d)
	if e != nil {
		return "", e
	}
	token, e := f.accessToken(ctx, c)
	if e != nil {
		return "", e
	}
	result, e := f.request(ctx, "/open-apis/im/v1/messages?receive_id_type=chat_id", token, map[string]string{"receive_id": c.ChatID, "msg_type": "post", "content": content, "uuid": d.ID})
	if e != nil {
		return "", e
	}
	message := str(obj(result["data"])["message_id"])
	if message == "" {
		return "", &Failure{Code: "MONITOR_FEISHU_RECEIPT_MISSING", Retryable: true}
	}
	return message, nil
}
