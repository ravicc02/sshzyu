package handler

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/agent"
)

type agentActivityKey struct{}

type agentInvocationLimits struct {
	FirstResponse time.Duration
	Idle          time.Duration
	Total         time.Duration
}

func defaultAgentInvocationLimits(kind string) agentInvocationLimits {
	if kind == "image" {
		// Image endpoints may return only a completed image, without streaming.
		return agentInvocationLimits{Total: 10 * time.Minute}
	}
	return agentInvocationLimits{FirstResponse: time.Minute, Idle: time.Minute, Total: 2 * time.Minute}
}

// Cancel the actual gateway request, rather than abandoning its goroutine and
// allowing it to keep running or charging after the run is reported as ended.
type agentInvocationWatchdog struct {
	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelCauseFunc
	totalCancel context.CancelFunc
	timer       *time.Timer
	deadline    time.Time
	idle        time.Duration
	started     bool
	closed      bool
}

func newAgentInvocationWatchdog(parent context.Context, kind string, limits agentInvocationLimits) *agentInvocationWatchdog {
	totalCode := "AGENT_MODEL_TIMEOUT"
	if kind == "image" {
		totalCode = "AGENT_IMAGE_TIMEOUT"
	}
	total, stop := context.WithTimeoutCause(parent, limits.Total, agent.Invalid(totalCode))
	ctx, cancel := context.WithCancelCause(total)
	w := &agentInvocationWatchdog{ctx: ctx, cancel: cancel, totalCancel: stop, idle: limits.Idle}
	if limits.FirstResponse > 0 {
		w.deadline = time.Now().Add(limits.FirstResponse)
		w.timer = time.AfterFunc(limits.FirstResponse, w.expire)
	}
	return w
}

func (w *agentInvocationWatchdog) expire() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.ctx.Err() != nil {
		return
	}
	// A reset can race an already executing AfterFunc. Recheck under the same
	// lock so an old callback cannot cancel a request that just made progress.
	if remaining := time.Until(w.deadline); remaining > 0 {
		w.timer.Reset(remaining)
		return
	}
	code := "AGENT_MODEL_FIRST_RESPONSE_TIMEOUT"
	if w.started {
		code = "AGENT_MODEL_IDLE_TIMEOUT"
	}
	w.cancel(agent.Invalid(code))
}

func (w *agentInvocationWatchdog) activity() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.timer == nil || w.ctx.Err() != nil {
		return
	}
	// Progress arriving after the deadline cannot revive an expired request.
	if !time.Now().Before(w.deadline) {
		code := "AGENT_MODEL_FIRST_RESPONSE_TIMEOUT"
		if w.started {
			code = "AGENT_MODEL_IDLE_TIMEOUT"
		}
		w.cancel(agent.Invalid(code))
		return
	}
	w.started = true
	w.deadline = time.Now().Add(w.idle)
	w.timer.Reset(w.idle)
}

func (w *agentInvocationWatchdog) close() {
	w.mu.Lock()
	w.closed = true
	if w.timer != nil {
		w.timer.Stop()
	}
	w.cancel(context.Canceled)
	w.totalCancel()
	w.mu.Unlock()
}

func agentInvocationContextFailure(ctx context.Context, dispatched bool) *agent.InvocationFailure {
	if ctx.Err() == nil {
		return nil
	}
	code := agent.SafeCode(context.Cause(ctx))
	message := ""
	switch code {
	case "AGENT_MODEL_FIRST_RESPONSE_TIMEOUT":
		message = "模型在 60 秒内未返回内容，本次请求已超时结束。"
	case "AGENT_MODEL_IDLE_TIMEOUT":
		message = "模型超过 60 秒未返回新内容，本次请求已超时结束。"
	case "AGENT_MODEL_TIMEOUT":
		message = "模型调用超过 120 秒，本次请求已超时结束。"
	case "AGENT_IMAGE_TIMEOUT":
		message = "图片生成超过 10 分钟，本次请求已超时结束。"
	default:
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code, message = "AGENT_RUN_TIMEOUT", "本次任务已超时结束，已返回的内容会保留。"
		} else {
			code, message = "AGENT_REQUEST_CANCELLED", "本次请求已停止。"
		}
	}
	return &agent.InvocationFailure{Code: code, Dispatched: dispatched, Detail: &agent.FailureDetail{Message: message}}
}

func (w *agentResponseWriter) activity() {
	if w.ctx == nil {
		return
	}
	if touch, ok := w.ctx.Value(agentActivityKey{}).(func()); ok {
		touch()
	}
}

func agentMeaningfulStreamEvent(kind string, delta string) bool {
	return (strings.HasSuffix(kind, ".delta") && delta != "") ||
		kind == "response.output_item.added" || kind == "response.output_item.done" ||
		kind == "response.completed"
}
