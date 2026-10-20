package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"net/http/httptrace"
	"sync"
)

// AgentInvocationTrace is private to an in-process, authenticated invocation.
// It forces inline accounting and exposes only confirmed charges to the agent.
type AgentInvocationTrace struct {
	CostLimit          string
	InputImageCount    int
	VisionEncodedBytes int
	ForceAskUser       bool
	OperationID        string
	RunID              string
	PinnedAccountID    int64
	AccountID          int64
	Dispatched         bool
	mu                 sync.Mutex
	cost               float64
	recorded           bool
	requestWritten     bool
	acquisitionStarted bool
	connectionObtained bool
}

// AgentVisionInputReserve reserves multimodal input capacity per normalized image.
const AgentVisionInputReserve = 65536

type agentInvocationKey struct{}

func WithAgentInvocation(ctx context.Context, t *AgentInvocationTrace) context.Context {
	ctx = context.WithValue(ctx, agentInvocationKey{}, t)
	if t == nil {
		return ctx
	}
	return httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GetConn:      func(string) { t.mu.Lock(); t.acquisitionStarted = true; t.mu.Unlock() },
		GotConn:      func(httptrace.GotConnInfo) { t.mu.Lock(); t.connectionObtained = true; t.mu.Unlock() },
		WroteHeaders: func() { t.markRequestWritten() },
		WroteRequest: func(httptrace.WroteRequestInfo) { t.markRequestWritten() },
	})
}
func AgentInvocationFromContext(ctx context.Context) *AgentInvocationTrace {
	if ctx == nil {
		return nil
	}
	t, _ := ctx.Value(agentInvocationKey{}).(*AgentInvocationTrace)
	return t
}
func RecordAgentCharge(ctx context.Context, cost float64) {
	if t := AgentInvocationFromContext(ctx); t != nil {
		t.mu.Lock()
		defer t.mu.Unlock()
		t.cost += cost
		t.recorded = true
	}
}
func (t *AgentInvocationTrace) Charge() (float64, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cost, t.recorded
}

func (t *AgentInvocationTrace) BindAccount(id int64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.PinnedAccountID > 0 && t.PinnedAccountID != id {
		return false
	}
	t.AccountID = id
	return true
}
func (t *AgentInvocationTrace) MarkDispatched() { t.mu.Lock(); t.Dispatched = true; t.mu.Unlock() }

// Prove failed connection acquisition (including HTTPS proxy CONNECT) rather than treating all network failures
// as harmless. Missing instrumentation, timeouts after writes and partial writes
// remain ambiguous, and a confirmed charge always prevents this classification.
func (t *AgentInvocationTrace) markRequestWritten() {
	t.mu.Lock()
	t.requestWritten = true
	t.mu.Unlock()
}
func (t *AgentInvocationTrace) ConnectionFailedBeforeWrite() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.acquisitionStarted && !t.connectionObtained && !t.requestWritten && !t.recorded
}

// Keep legacy manual-thinking budgets within the invocation's paid output cap.
// Adaptive providers retain their native effort mapping.
func boundAgentThinkingTokens(ctx context.Context, req *apicompat.AnthropicRequest) {
	if AgentInvocationFromContext(ctx) == nil || req == nil || req.Thinking == nil || req.Thinking.Type != "enabled" {
		return
	}
	if req.Thinking.BudgetTokens >= req.MaxTokens {
		req.Thinking.BudgetTokens = max(1024, req.MaxTokens-256)
	}
}
