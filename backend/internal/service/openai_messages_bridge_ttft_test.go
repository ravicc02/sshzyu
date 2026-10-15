package service

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMessagesBridgeTTFTIgnoresMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
	r, w := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer w.Close()
		io.WriteString(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_t\"}}\n\n")
		time.Sleep(120 * time.Millisecond)
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"42\"}\n\n")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_t\",\"status\":\"completed\",\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\n\n")
	}()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	result, err := svc.handleAnthropicStreamingResponse(&http.Response{StatusCode: 200, Header: http.Header{}, Body: r}, c, &Account{ID: 1, Platform: PlatformOpenAI}, "model", "model", "model", time.Now())
	<-done
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.FirstTokenMs)
	require.GreaterOrEqual(t, *result.FirstTokenMs, 100, "created event is not first content")
}
