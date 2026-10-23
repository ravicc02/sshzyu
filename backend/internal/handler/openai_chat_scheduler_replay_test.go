//go:build unit

package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The production audit found repeated 503 attempts on 7/11 and successful
// streaming traffic with a committed HTTP 200. These are synthetic protocol
// fixtures for that error shape; no production request text or key is copied.
const chatReplayRequest = `{"model":"gpt-6.1-sol","messages":[{"role":"user","content":"synthetic scheduler verification"}],"stream":true,"reasoning_effort":"high"}`
const chatReplaySuccess = "data: {\"id\":\"synthetic-success\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-6.1-sol\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"healthy-result\"}}]}\n\n" +
	"data: {\"id\":\"synthetic-success\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-6.1-sol\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n" + "data: [DONE]\n\n"

// Read blocks without sleeping and is released by the real first-output guard.
// The body also makes it possible to assert that abandoned upstreams are closed.
type chatReplayBody struct {
	prefix *strings.Reader
	eof    bool
	closed chan struct{}
	once   sync.Once
}

func newChatReplayBody(prefix string, eof bool) *chatReplayBody {
	return &chatReplayBody{prefix: strings.NewReader(prefix), eof: eof, closed: make(chan struct{})}
}
func (b *chatReplayBody) Read(p []byte) (int, error) {
	if b.prefix.Len() > 0 {
		return b.prefix.Read(p)
	}
	if !b.eof {
		<-b.closed
	}
	return 0, io.EOF
}
func (b *chatReplayBody) Close() error { b.once.Do(func() { close(b.closed) }); return nil }

type chatReplayUpstream struct {
	service.HTTPUpstream
	answers   []*http.Response
	context   *gin.Context
	keepalive bool
	calls     []int64
	bodies    [][]byte
}

func (u *chatReplayUpstream) Do(req *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	u.calls = append(u.calls, id)
	body, _ := io.ReadAll(req.Body)
	u.bodies = append(u.bodies, body)
	if len(u.calls) == 1 && u.keepalive {
		u.context.Header("Content-Type", "text/event-stream")
		_, _ = u.context.Writer.WriteString(": scheduler-keepalive\n\n")
		u.context.Writer.Flush()
	}
	if len(u.answers) == 0 {
		return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"server_error","message":"Service unavailable"}}`))}, nil
	}
	answer := u.answers[0]
	u.answers = u.answers[1:]
	return answer, nil
}
func chatReplayStream(body io.ReadCloser) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: body}
}
func chatReplay503() *http.Response {
	return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"server_error","code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`))}
}

func newChatReplayHandler(t *testing.T, upstream *chatReplayUpstream) (*OpenAIGatewayHandler, *gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	accounts := []service.Account{}
	for i, id := range []int64{7, 11, 260} {
		accounts = append(accounts, service.Account{ID: id, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 10, Priority: i, GroupIDs: []int64{4}, Credentials: map[string]any{"api_key": "synthetic-test-key", "base_url": "https://api.openai.com", "model_mapping": map[string]any{"gpt-6.1-sol": "gpt-6.1-sol"}}, Extra: map[string]any{"openai_responses_supported": false}})
	}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.RawChatFirstOutputTimeoutSeconds = 1
	cfg.Gateway.OpenAIScheduler.LatencyAwareEnabled = true
	gateway := service.NewOpenAIGatewayService(openAIImagesFailoverAccountRepo{accounts: accounts}, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil)
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	handler := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billing, service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
	handler.maxAccountSwitches = 10 // first-output budget is independent of this larger switch cap
	requestCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	t.Cleanup(cancel)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(chatReplayRequest)).WithContext(requestCtx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	upstream.context = c
	group := int64(4)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 99, GroupID: &group, Group: &service.Group{ID: group, Platform: service.PlatformOpenAI}, User: &service.User{ID: 100}})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 100, Concurrency: 0})
	return handler, c, rec
}

func TestOpenAIChatReplay503MovesThroughCandidatesWithoutRepeating(t *testing.T) {
	upstream := &chatReplayUpstream{answers: []*http.Response{chatReplay503(), chatReplay503(), chatReplayStream(io.NopCloser(strings.NewReader(chatReplaySuccess)))}}
	handler, c, rec := newChatReplayHandler(t, upstream)
	handler.ChatCompletions(c)
	require.Equal(t, []int64{7, 11, 260}, upstream.calls)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, strings.Count(rec.Body.String(), "healthy-result"))
	require.Len(t, upstream.bodies, 3)
	require.JSONEq(t, string(upstream.bodies[0]), string(upstream.bodies[1]))
	require.JSONEq(t, string(upstream.bodies[1]), string(upstream.bodies[2]))
}

func TestOpenAIChatReplayFirstOutputTimeoutAfterKeepaliveCanSwitch(t *testing.T) {
	stalled := newChatReplayBody(": upstream-heartbeat\n\n", false)
	upstream := &chatReplayUpstream{keepalive: true, answers: []*http.Response{chatReplayStream(stalled), chatReplayStream(io.NopCloser(strings.NewReader(chatReplaySuccess)))}}
	handler, c, rec := newChatReplayHandler(t, upstream)
	handler.ChatCompletions(c)
	require.Equal(t, []int64{7, 11}, upstream.calls)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), ": scheduler-keepalive")
	require.Equal(t, 1, strings.Count(rec.Body.String(), "healthy-result"))
	require.NotContains(t, rec.Body.String(), "first_output_timeout")
	select {
	case <-stalled.closed:
	default:
		t.Fatal("abandoned upstream was not closed")
	}
}

func TestOpenAIChatReplayFirstOutputTimeoutStopsAfterOneSwitch(t *testing.T) {
	a, b, cbody := newChatReplayBody("", false), newChatReplayBody("", false), newChatReplayBody("", false)
	upstream := &chatReplayUpstream{answers: []*http.Response{chatReplayStream(a), chatReplayStream(b), chatReplayStream(cbody)}}
	handler, c, rec := newChatReplayHandler(t, upstream)
	handler.ChatCompletions(c)
	require.Equal(t, []int64{7, 11}, upstream.calls, "no third slow upstream, even when maxAccountSwitches is 10")
	// The existing gateway maps the client response to an upstream_error, while
	// operations evidence must retain the actual upstream timeout status.
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "upstream_error")
	require.NotContains(t, rec.Body.String(), "healthy-result")
	status, ok := c.Get(service.OpsUpstreamStatusCodeKey)
	require.True(t, ok)
	require.Equal(t, http.StatusGatewayTimeout, status)
	for _, body := range []*chatReplayBody{a, b} {
		select {
		case <-body.closed:
		default:
			t.Fatal("timed-out upstream was not closed")
		}
	}
}

func TestOpenAIChatReplaySemanticOutputNeverReplays(t *testing.T) {
	partial := "data: {\"id\":\"synthetic-partial\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-6.1-sol\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial-result\"}}]}\n\n"
	upstream := &chatReplayUpstream{answers: []*http.Response{chatReplayStream(newChatReplayBody(partial, true)), chatReplayStream(io.NopCloser(strings.NewReader(chatReplaySuccess)))}}
	handler, c, rec := newChatReplayHandler(t, upstream)
	handler.ChatCompletions(c)
	require.Equal(t, []int64{7}, upstream.calls)
	require.Equal(t, 1, strings.Count(rec.Body.String(), "partial-result"))
	require.NotContains(t, rec.Body.String(), "healthy-result")
}

func TestOpenAIChatReplayUsageEvidenceNeverReplays(t *testing.T) {
	usage := "data: {\"id\":\"synthetic-usage\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-6.1-sol\",\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2,\"total_tokens\":6}}\n\n"
	upstream := &chatReplayUpstream{keepalive: true, answers: []*http.Response{chatReplayStream(newChatReplayBody(usage, false)), chatReplayStream(io.NopCloser(strings.NewReader(chatReplaySuccess)))}}
	handler, c, rec := newChatReplayHandler(t, upstream)
	handler.ChatCompletions(c)
	require.Equal(t, []int64{7}, upstream.calls, "already metered usage forbids replay even without semantic output")
	require.NotContains(t, rec.Body.String(), "healthy-result")
}
