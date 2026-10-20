//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAgentGeminiNativeToolsAndNoRetry(t *testing.T) {
	for _, status := range []int{200, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			raw := `data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"ask_user","args":{"question":"主题？","fields":[]}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":6}}` + "\n\ndata: [DONE]\n\n"
			if status != 200 {
				raw = `{"error":{"message":"unavailable"}}`
			}
			upstream := &geminiCompatHTTPUpstreamStub{response: &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(raw))}}
			svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
			account := &Account{ID: 102, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
			body := []byte(`{"model":"gemini-2.5-flash","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"ask"}],"tools":[{"type":"function","function":{"name":"ask_user","parameters":{"type":"object","properties":{"question":{"type":"string"}}}}}]}`)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			ctx := WithAgentInvocation(context.Background(), &AgentInvocationTrace{})
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(body)).WithContext(ctx)
			result, err := svc.ForwardAsChatCompletions(ctx, c, account, body)
			require.Equal(t, 1, upstream.calls)
			if status == 200 {
				require.NoError(t, err)
				require.Equal(t, 12, result.Usage.InputTokens)
				require.Contains(t, rec.Body.String(), `"name":"ask_user"`)
				require.Contains(t, rec.Body.String(), "[DONE]")
			} else {
				require.Error(t, err)
			}
		})
	}
}
func TestAgentAntigravityNativeCallIsNotReplayed(t *testing.T) {
	upstream := &mockSmartRetryUpstream{responses: []*http.Response{{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"unavailable"}`))}}, errors: []error{nil}}
	account := &Account{ID: 1, Platform: PlatformAntigravity, Type: AccountTypeOAuth}
	svc := &AntigravityGatewayService{}
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{ctx: WithAgentInvocation(context.Background(), &AgentInvocationTrace{}), account: account, httpUpstream: upstream, accessToken: "test-token", body: []byte(`{}`), action: "streamGenerateContent"})
	require.NoError(t, err)
	require.Equal(t, 503, result.resp.StatusCode)
	require.Equal(t, 1, upstream.callIdx)
	result.resp.Body.Close()
}
func TestAgentAntigravityTruncationRetainsUsage(t *testing.T) {
	svc := &AntigravityGatewayService{}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	ctx := WithAgentInvocation(context.Background(), &AgentInvocationTrace{})
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil).WithContext(ctx)
	body := `data: {"response":{"candidates":[{"content":{"parts":[{"text":"partial"}]}}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":2}}}` + "\n\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	result, err := svc.handleResponsesStreamingFromAntigravity(c, resp, time.Now(), "gemini-test")
	require.Error(t, err)
	require.NotNil(t, result)
	require.Greater(t, result.usage.InputTokens, 0)
	require.NotContains(t, rec.Body.String(), `"type":"response.completed"`)
}
