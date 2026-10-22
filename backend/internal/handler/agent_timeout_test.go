package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/agent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func awaitAgentLimit(t *testing.T, ctx context.Context, want string) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("invocation did not stop")
	}
	failure := agentInvocationContextFailure(ctx, true)
	if failure == nil || failure.Code != want || failure.ResultKnown || !failure.Dispatched {
		t.Fatalf("lost interruption evidence: %#v", failure)
	}
}

func TestAgentInvocationDefaultLimits(t *testing.T) {
	text, image := defaultAgentInvocationLimits("text"), defaultAgentInvocationLimits("image")
	if text.FirstResponse != time.Minute || text.Idle != time.Minute || text.Total != 2*time.Minute {
		t.Fatal(text)
	}
	if image.FirstResponse != 0 || image.Idle != 0 || image.Total != 10*time.Minute {
		t.Fatal(image)
	}
}

func TestAgentInvocationFirstResponseTimeout(t *testing.T) {
	w := newAgentInvocationWatchdog(context.Background(), "text", agentInvocationLimits{FirstResponse: 40 * time.Millisecond, Idle: time.Second, Total: time.Second})
	defer w.close()
	awaitAgentLimit(t, w.ctx, "AGENT_MODEL_FIRST_RESPONSE_TIMEOUT")
	// Late bytes cannot restart a timed out request.
	w.activity()
	awaitAgentLimit(t, w.ctx, "AGENT_MODEL_FIRST_RESPONSE_TIMEOUT")
}

func TestAgentInvocationIdleTimeoutRetainsPartialTextAndCharge(t *testing.T) {
	w := newAgentInvocationWatchdog(context.Background(), "text", agentInvocationLimits{FirstResponse: time.Second, Idle: 40 * time.Millisecond, Total: 2 * time.Second})
	defer w.close()
	trace := &service.AgentInvocationTrace{}
	ctx := context.WithValue(service.WithAgentInvocation(w.ctx, trace), agentActivityKey{}, w.activity)
	var partial string
	engine := gin.New()
	engine.POST("/v1/responses", func(c *gin.Context) {
		trace.MarkDispatched()
		service.RecordAgentCharge(c.Request.Context(), 0.01)
		c.Header("Content-Type", "text/event-stream")
		_, _ = c.Writer.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial reply\"}\n\n"))
		<-c.Request.Context().Done()
	})
	p := &platformAgentExecutor{engine: engine}
	writer := p.invoke(ctx, &service.APIKey{Key: "fixture"}, "127.0.0.1", "/v1/responses", "application/json", []byte(`{}`), func(u agent.StreamUpdate) error { partial = u.Text; return nil })
	defer writer.releaseBuffers()
	awaitAgentLimit(t, ctx, "AGENT_MODEL_IDLE_TIMEOUT")
	cost, recorded := trace.Charge()
	if partial != "partial reply" || !recorded || cost != 0.01 {
		t.Fatal("partial content or charge lost", partial, cost, recorded)
	}
	if _, err := writer.Write([]byte("late output")); err == nil {
		t.Fatal("accepted data after timeout")
	}
}

func TestAgentInvocationHeartbeatsDoNotExtendThinking(t *testing.T) {
	w := newAgentInvocationWatchdog(context.Background(), "text", agentInvocationLimits{FirstResponse: 60 * time.Millisecond, Idle: time.Second, Total: 2 * time.Second})
	defer w.close()
	writer := &agentResponseWriter{ctx: context.WithValue(w.ctx, agentActivityKey{}, w.activity), headers: http.Header{"Content-Type": []string{"text/event-stream"}}}
	defer writer.releaseBuffers()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for w.ctx.Err() == nil {
		select {
		case <-w.ctx.Done():
		case <-ticker.C:
			_, _ = writer.Write([]byte(": ping\n\ndata: {\"type\":\"response.created\"}\n\n"))
		}
	}
	awaitAgentLimit(t, w.ctx, "AGENT_MODEL_FIRST_RESPONSE_TIMEOUT")
}

func TestAgentInvocationToolProgressStillHasTotalLimit(t *testing.T) {
	w := newAgentInvocationWatchdog(context.Background(), "text", agentInvocationLimits{FirstResponse: time.Second, Idle: time.Second, Total: 120 * time.Millisecond})
	defer w.close()
	writer := &agentResponseWriter{ctx: context.WithValue(w.ctx, agentActivityKey{}, w.activity), headers: http.Header{"Content-Type": []string{"text/event-stream"}}}
	defer writer.releaseBuffers()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for w.ctx.Err() == nil {
		select {
		case <-w.ctx.Done():
		case <-ticker.C:
			_, _ = writer.Write([]byte("data: {\"type\":\"response.function_call_arguments.delta\",\"delta\":\"x\"}\n\n"))
		}
	}
	if !w.started {
		t.Fatal("tool argument progress ignored")
	}
	awaitAgentLimit(t, w.ctx, "AGENT_MODEL_TIMEOUT")
}

func TestAgentInvocationProgressResetsIdleDeadline(t *testing.T) {
	w := newAgentInvocationWatchdog(context.Background(), "text", agentInvocationLimits{FirstResponse: time.Second, Idle: 200 * time.Millisecond, Total: 2 * time.Second})
	defer w.close()
	for i := 0; i < 8; i++ {
		w.activity()
		select {
		case <-w.ctx.Done():
			t.Fatal("active response timed out", context.Cause(w.ctx))
		case <-time.After(35 * time.Millisecond):
		}
	}
	awaitAgentLimit(t, w.ctx, "AGENT_MODEL_IDLE_TIMEOUT")
}

func TestAgentInvocationParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	w := newAgentInvocationWatchdog(parent, "text", defaultAgentInvocationLimits("text"))
	defer w.close()
	cancel()
	awaitAgentLimit(t, w.ctx, "AGENT_REQUEST_CANCELLED")
}

func TestAgentInvocationImageUsesSeparateTotalLimit(t *testing.T) {
	w := newAgentInvocationWatchdog(context.Background(), "image", agentInvocationLimits{Total: 40 * time.Millisecond})
	defer w.close()
	awaitAgentLimit(t, w.ctx, "AGENT_IMAGE_TIMEOUT")
}
