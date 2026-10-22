package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/agent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func TestAgentOriginalUpstreamErrorSurvivesGatewayMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(agentFailureMiddleware())
	engine.POST("/v1/images/generations", func(c *gin.Context) {
		service.SetOpsUpstreamError(c, 403, "Insufficient balance", "")
		c.JSON(502, gin.H{"error": gin.H{"message": "upstream request failed"}})
	})
	p := &platformAgentExecutor{engine: engine}
	writer := p.invoke(context.Background(), &service.APIKey{Key: "fixture-platform-key"}, "127.0.0.1", "/v1/images/generations", "application/json", []byte(`{}`))
	defer writer.releaseBuffers()
	failure := agentHTTPFailure(writer, true, "fixture-platform-key")
	if failure.Code != "AGENT_UPSTREAM_REJECTED" || !failure.ResultKnown || !failure.Dispatched || failure.Detail == nil || failure.Detail.UpstreamStatusCode != 403 || failure.Detail.StatusCode != 502 || failure.Detail.Message != "Insufficient balance" {
		t.Fatal("original rejection was lost", failure)
	}
	if errors.Is(failure, agent.ErrUnknown) {
		t.Fatal("explicit rejection treated as a missing response")
	}
}

func TestAgentSuccessfulFailoverDoesNotExposeEarlierFailure(t *testing.T) {
	engine := gin.New()
	engine.Use(agentFailureMiddleware())
	engine.POST("/v1/responses", func(c *gin.Context) {
		service.SetOpsUpstreamError(c, 403, "Earlier rejected attempt", "")
		c.JSON(200, gin.H{"status": "completed", "output": []any{}})
	})
	p := &platformAgentExecutor{engine: engine}
	writer := p.invoke(context.Background(), &service.APIKey{Key: "fixture"}, "127.0.0.1", "/v1/responses", "application/json", []byte(`{}`))
	defer writer.releaseBuffers()
	if writer.failure != nil {
		t.Fatal("successful request inherited an older attempt's error")
	}
}

func TestAgentOriginalErrorIsRedactedBoundedAndKeepsReason(t *testing.T) {
	message := `Insufficient balance. api_key="tiny-fixture" Authorization: Bearer fixture-bearer; https://name:password@upstream.invalid/a?key=fixture-query sk-fixture-secret fixture-user-key ` + strings.Repeat("很", 1200) + "\n"
	safe := agentFailureMessage(message, "fixture-user-key")
	if !strings.Contains(safe, "Insufficient balance") || utf8.RuneCountInString(safe) > 1025 {
		t.Fatal("reason lost or unbounded")
	}
	for _, secret := range []string{"tiny-fixture", "fixture-bearer", "upstream.invalid", "fixture-query", "sk-fixture-secret", "fixture-user-key", "\n"} {
		if strings.Contains(safe, secret) {
			t.Fatal("credential/address leaked", secret)
		}
	}
}

func TestAgentFailureDoesNotPretendAmbiguousResponsesAreSafeToReplay(t *testing.T) {
	for _, status := range []int{400, 401, 403, 422, 429} {
		if !knownAgentRejection(status) {
			t.Fatal("known rejection not recognized", status)
		}
	}
	for _, status := range []int{408, 409, 500, 502, 503, 504} {
		if knownAgentRejection(status) {
			t.Fatal("ambiguous request can be replayed", status)
		}
	}
	writer := &agentResponseWriter{code: 502}
	_, _ = writer.body.WriteString(`{"error":{"message":"upstream disconnected"}}`)
	failure := agentHTTPFailure(writer, true, "")
	if failure.ResultKnown || !failure.Dispatched || failure.Detail.Message != "upstream disconnected" {
		t.Fatal("result uncertainty lost", failure)
	}
}

func TestAgentStreamingErrorPreservesProviderMessage(t *testing.T) {
	writer := &agentResponseWriter{headers: make(http.Header)}
	writer.headers.Set("Content-Type", "text/event-stream")
	writer.consumeStream([]byte("data: {\"type\":\"error\",\"message\":\"Insufficient balance\"}\n\n"))
	var failure *agent.InvocationFailure
	if !errors.As(writer.streamErr, &failure) || failure.Detail == nil || failure.Detail.Message != "Insufficient balance" {
		t.Fatal("stream error became a generic timeout", writer.streamErr)
	}
}

func TestAgentMissingResultRecoveryFailsBeforePollingBilling(t *testing.T) {
	// No database is configured: missing content must return immediately,
	// without trying to query billing or redispatch a provider request.
	p := &platformAgentExecutor{}
	_, err := p.Recover(context.Background(), agent.Invocation{Kind: "image"}, agent.Outcome{Unknown: true, BillingPending: true, Failure: &agent.FailureDetail{Message: "connection interrupted"}})
	var failure *agent.InvocationFailure
	if !errors.As(err, &failure) || failure.Detail == nil || failure.Detail.Message != "connection interrupted" {
		t.Fatal("missing response entered an impossible recovery path", err)
	}
}

func TestAgentSavedImageRecoveryDoesNotRequireAnotherResponse(t *testing.T) {
	p := &platformAgentExecutor{}
	saved := agent.Outcome{Image: []byte("fixture accepted pixels"), Cost: "0.17000000"}
	recovered, err := p.Recover(context.Background(), agent.Invocation{Kind: "image"}, saved)
	if err != nil || len(recovered.Image) == 0 || recovered.Cost != saved.Cost {
		t.Fatal("materialized checkpoint wrongly rejected", err)
	}
}

func TestAgentFinalRejectionCannotSettleEarlierAmbiguousAttempt(t *testing.T) {
	engine := gin.New()
	engine.Use(agentFailureMiddleware())
	engine.POST("/v1/images/generations", func(c *gin.Context) {
		c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{{UpstreamStatusCode: 503, Kind: "http_error"}, {UpstreamStatusCode: 403, Kind: "http_error"}})
		service.SetOpsUpstreamError(c, 403, "Insufficient balance", "")
		c.JSON(502, gin.H{"error": gin.H{"message": "upstream failed"}})
	})
	p := &platformAgentExecutor{engine: engine}
	writer := p.invoke(context.Background(), &service.APIKey{Key: "fixture"}, "127.0.0.1", "/v1/images/generations", "application/json", []byte(`{}`))
	defer writer.releaseBuffers()
	failure := agentHTTPFailure(writer, true, "")
	if failure.ResultKnown || failure.Detail.UpstreamStatusCode != 403 || failure.Detail.Message != "Insufficient balance" {
		t.Fatal("earlier uncertain request became replayable", failure)
	}
}
