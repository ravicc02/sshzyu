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

func TestChatResponsesBridgeTTFTIgnoresMetadata(t *testing.T) {
	for _, tc := range []struct{ name, preamble, output string }{
		{"text", `{"type":"response.created","response":{"id":"resp_t"}}`, `{"type":"response.output_text.delta","delta":"42"}`},
		{"reasoning", `{"type":"response.output_item.added","output_index":0,"item":{"id":"r","type":"reasoning","summary":[]}}`, `{"type":"response.reasoning_summary_text.delta","item_id":"r","delta":"compute"}`},
		{"tool", `{"type":"response.output_item.added","output_index":0,"item":{"id":"f","type":"function_call","call_id":"call_t","name":"lookup","arguments":""}}`, `{"type":"response.function_call_arguments.delta","item_id":"f","output_index":0,"delta":"{}"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
			r, w := io.Pipe()
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer w.Close()
				io.WriteString(w, "data: "+tc.preamble+"\n\n")
				io.WriteString(w, "data: {\"type\":\"response.in_progress\"}\n\n")
				time.Sleep(120 * time.Millisecond)
				io.WriteString(w, "data: "+tc.output+"\n\n")
				io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_t\",\"status\":\"completed\",\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\n\n")
			}()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			result, err := svc.handleChatStreamingResponse(&http.Response{StatusCode: 200, Header: http.Header{}, Body: r}, c, &Account{ID: 1, Platform: PlatformOpenAI}, "test-model", "test-model", "test-model", time.Now(), 0)
			<-done
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, result.FirstTokenMs)
			require.GreaterOrEqual(t, *result.FirstTokenMs, 100, "metadata must not make a stalled provider look fast")
			require.Equal(t, 2, result.Usage.InputTokens)
			require.Equal(t, 1, result.Usage.OutputTokens)
		})
	}
}
