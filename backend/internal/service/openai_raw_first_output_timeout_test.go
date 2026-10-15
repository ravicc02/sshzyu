//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
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

func TestRawChatFirstOutputGuard_MetadataDoesNotDisableDeadline(t *testing.T) {
	for _, prefix := range []string{": ping\n\n", "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n", "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n"} {
		t.Run(prefix, func(t *testing.T) {
			reader, writer := io.Pipe()
			defer writer.Close()
			watchdog := time.AfterFunc(300*time.Millisecond, func() { _ = writer.Close() })
			defer watchdog.Stop()
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{RawChatFirstOutputTimeoutSeconds: 1}}}
			resp := &http.Response{Header: http.Header{}, Body: reader}
			go func() { _, _ = io.WriteString(writer, prefix) }()
			started := time.Now()
			result, err := svc.streamRawChatCompletions(c, resp, rawChatCompletionsTestAccount(), "deepseek-v4-flash", "deepseek-v4-flash", "deepseek-v4-flash", nil, nil, time.Now().Add(-950*time.Millisecond), 10)
			require.Nil(t, result)
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.Equal(t, http.StatusGatewayTimeout, failover.StatusCode)
			require.Empty(t, rec.Body.String(), "metadata must remain retractable before failover")
			require.Less(t, time.Since(started), 500*time.Millisecond)
		})
	}
}

func TestRawChatFirstOutputGuard_StopsAfterSemanticOutput(t *testing.T) {
	for _, delta := range []string{`{"content":"ok"}`, `{"reasoning_content":"thinking"}`, `{"tool_calls":[{"index":0,"function":{"name":"lookup"}}]}`} {
		t.Run(delta, func(t *testing.T) {
			reader, writer := io.Pipe()
			defer reader.Close()
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{RawChatFirstOutputTimeoutSeconds: 1}}}
			go func() {
				defer writer.Close()
				_, _ = io.WriteString(writer, "data: {\"choices\":[{\"delta\":"+delta+"}]}\n\n")
				time.Sleep(150 * time.Millisecond)
				_, _ = io.WriteString(writer, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}()
			result, err := svc.streamRawChatCompletions(c, &http.Response{Header: http.Header{}, Body: reader}, rawChatCompletionsTestAccount(), "deepseek-v4-flash", "deepseek-v4-flash", "deepseek-v4-flash", nil, nil, time.Now().Add(-900*time.Millisecond), 10)
			require.NoError(t, err)
			require.NotNil(t, result.FirstTokenMs)
			require.Contains(t, rec.Body.String(), "[DONE]")
		})
	}
}

func TestRawChatFirstOutputTimeout_EffortOverrideAndDisabled(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{RawChatFirstOutputTimeoutSeconds: 30, RawChatHighEffortFirstOutputTimeoutSeconds: 90}}}
	require.Equal(t, 30*time.Second, svc.rawChatFirstOutputTimeout(nil))
	effort := "xhigh"
	require.Equal(t, 90*time.Second, svc.rawChatFirstOutputTimeout(&effort))
	svc.cfg.Gateway.RawChatFirstOutputTimeoutSeconds = 0
	require.Zero(t, svc.rawChatFirstOutputTimeout(&effort))
	require.False(t, rawChatChunkHasSemanticOutput(`{"choices":[{"delta":{"role":"assistant","content":""}}]}`))
	require.False(t, rawChatChunkHasSemanticOutput(`{"choices":[],"usage":{"total_tokens":3}}`))
	require.False(t, rawChatChunkHasSemanticOutput(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`))
	require.False(t, rawChatChunkHasSemanticOutput(`{"error":{"type":"api_error"}}`))
	require.False(t, rawChatChunkHasSemanticOutput(`{"choices":[{"delta":{"tool_calls":[{"index":0}],"function_call":{}}}]}`))
}

func TestRawChatFirstOutputGuard_PreservesMeteredUsageWithoutReplay(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{RawChatFirstOutputTimeoutSeconds: 1}}}
	go func() { _, _ = io.WriteString(writer, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2}}\n\n") }()
	result, err := svc.streamRawChatCompletions(c, &http.Response{Header: http.Header{}, Body: reader}, rawChatCompletionsTestAccount(), "deepseek-v4-flash", "deepseek-v4-flash", "deepseek-v4-flash", nil, nil, time.Now().Add(-950*time.Millisecond), 10)
	require.Error(t, err)
	var failover *UpstreamFailoverError
	require.False(t, errors.As(err, &failover), "metered attempts must not be replayed")
	require.Equal(t, 2, result.Usage.InputTokens)
	require.Nil(t, result.FirstTokenMs)
	require.Empty(t, rec.Body.String())
}

type rawHeaderDeadlineUpstream struct{ httpUpstreamRecorder }

func (u *rawHeaderDeadlineUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}
func TestRawChatFirstOutputGuard_BoundsHeadersAndKeepsHealthyStream(t *testing.T) {
	body := []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"OK"}],"stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	cfg := rawChatCompletionsTestConfig()
	cfg.Gateway.RawChatFirstOutputTimeoutSeconds = 1
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: &rawHeaderDeadlineUpstream{}}
	ctx := context.WithValue(context.Background(), rawChatFirstOutputBudgetKey{}, rawChatFirstOutputBudget{start: time.Now().Add(-950 * time.Millisecond), timeout: time.Second, model: "deepseek-v4-flash"})
	start := time.Now()
	resp, err := svc.sendCCUpstreamRequest(ctx, c, rawChatCompletionsTestAccount(), "https://example.com/v1/chat/completions", body, true, "lab-only", "", "")
	require.Nil(t, resp)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Equal(t, http.StatusGatewayTimeout, failover.StatusCode)
	require.Less(t, time.Since(start), 500*time.Millisecond)
}

func TestRawChatFirstOutputGuard_HeaderDeadlineStopsAfterHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	ctx := context.WithValue(context.Background(), rawChatFirstOutputBudgetKey{}, rawChatFirstOutputBudget{start: time.Now().Add(-950 * time.Millisecond), timeout: time.Second, model: "deepseek-v4-flash"})
	resp, err := svc.sendCCUpstreamRequest(ctx, c, rawChatCompletionsTestAccount(), "https://example.com/v1/chat/completions", []byte(`{"model":"deepseek-v4-flash","stream":true}`), true, "lab-only", "", "")
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, upstream.lastReq.Context().Err(), "header deadline must not cancel a healthy response body")
	require.NoError(t, resp.Body.Close())
	require.ErrorIs(t, upstream.lastReq.Context().Err(), context.Canceled)
}

func TestRawChatFirstOutputGuard_MultilineEvent(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{RawChatFirstOutputTimeoutSeconds: 1}}}
	body := "data: {\"choices\":[\n" + "data: {\"delta\":{\"content\":\"OK\"}}]}\n\n" + "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	result, err := svc.streamRawChatCompletions(c, &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, rawChatCompletionsTestAccount(), "deepseek-v4.1-flash", "deepseek-v4.1-flash", "deepseek-v4.1-flash", nil, nil, time.Now(), 10000)
	require.NoError(t, err)
	require.NotNil(t, result.FirstTokenMs)
	require.Contains(t, rec.Body.String(), "OK")
}
