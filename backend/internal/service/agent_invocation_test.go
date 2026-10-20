//go:build unit

package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAgentBillingIdentityAndTrace(t *testing.T) {
	trace := &AgentInvocationTrace{OperationID: "run1_image1"}
	ctx := WithAgentInvocation(context.Background(), trace)
	if got := resolveUsageBillingRequestID(ctx, "upstream-changing-id"); got != "agent:run1_image1" {
		t.Fatal(got)
	}
	RecordAgentCharge(ctx, .17)
	cost, ok := trace.Charge()
	if !ok || cost != .17 {
		t.Fatal(cost, ok)
	}
	if got := resolveUsageBillingRequestID(context.Background(), "upstream-id"); got != "upstream-id" {
		t.Fatal("non-Agent billing changed", got)
	}
}

func TestAgentConnectionFailureBeforeWrite(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	trace := &AgentInvocationTrace{}
	ctx := WithAgentInvocation(context.Background(), trace)
	trace.MarkDispatched() // the existing conservative gateway-attempt marker
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+address, strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: time.Second}
	if response, err := client.Do(request); err == nil {
		response.Body.Close()
		t.Fatal("expected refused local connection")
	}
	if !trace.ConnectionFailedBeforeWrite() {
		t.Fatal("proven pre-send connection failure not classified")
	}
	RecordAgentCharge(ctx, .01)
	if trace.ConnectionFailedBeforeWrite() {
		t.Fatal("confirmed charge was ignored")
	}
}
func TestAgentPartialWriteRemainsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer server.Close()
	trace := &AgentInvocationTrace{}
	ctx := WithAgentInvocation(context.Background(), trace)
	trace.MarkDispatched()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if response, err := http.DefaultClient.Do(request); err == nil {
		response.Body.Close()
		t.Fatal("expected response disconnect")
	}
	// A failed fallback connect cannot erase evidence of the earlier write.
	httptrace.ContextClientTrace(ctx).GetConn("failed-fallback.invalid:443")
	if trace.ConnectionFailedBeforeWrite() {
		t.Fatal("possibly executed request was treated as safe failure")
	}
	if (&AgentInvocationTrace{Dispatched: true}).ConnectionFailedBeforeWrite() {
		t.Fatal("missing instrumentation was treated as proof")
	}
}

func TestAgentProxyConnectionFailureBeforeWrite(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	proxyURL, err := url.Parse("http://" + address)
	if err != nil {
		t.Fatal(err)
	}
	trace := &AgentInvocationTrace{}
	ctx := WithAgentInvocation(context.Background(), trace)
	trace.MarkDispatched()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://provider.invalid/v1/responses", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}, Timeout: time.Second}
	if response, err := client.Do(request); err == nil {
		response.Body.Close()
		t.Fatal("expected refused local proxy")
	}
	if !trace.ConnectionFailedBeforeWrite() {
		t.Fatal("proxy failure was classified as a potentially paid request")
	}
}

func TestAgentProxyTunnelDisconnectIsBeforeModelRequest(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			t.Error("unexpected proxy request")
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	trace := &AgentInvocationTrace{}
	request, err := http.NewRequestWithContext(WithAgentInvocation(context.Background(), trace), http.MethodPost, "https://provider.invalid/v1/responses", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	trace.MarkDispatched()
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}, Timeout: time.Second}
	if response, err := client.Do(request); err == nil {
		response.Body.Close()
		t.Fatal("expected proxy CONNECT disconnect")
	}
	if !trace.ConnectionFailedBeforeWrite() {
		t.Fatal("proxy CONNECT failure was treated as a potentially billed model request")
	}
}

func TestAgentThinkingBudgetRespectsPaidOutputLimit(t *testing.T) {
	req := &apicompat.AnthropicRequest{MaxTokens: 2048, Thinking: &apicompat.AnthropicThinking{Type: "enabled", BudgetTokens: 32768}}
	boundAgentThinkingTokens(WithAgentInvocation(context.Background(), &AgentInvocationTrace{}), req)
	if req.Thinking.BudgetTokens >= req.MaxTokens || req.Thinking.BudgetTokens < 1024 {
		t.Fatal(req.Thinking.BudgetTokens)
	}
	original := &apicompat.AnthropicRequest{MaxTokens: 2048, Thinking: &apicompat.AnthropicThinking{Type: "enabled", BudgetTokens: 32768}}
	boundAgentThinkingTokens(context.Background(), original)
	if original.Thinking.BudgetTokens != 32768 {
		t.Fatal("ordinary gateway request changed")
	}
}
