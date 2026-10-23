package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Upstream v0.2.18 drains regular disconnected clients for billing. Agent stop
// must retain its cancellation contract and close a stalled upstream promptly.
func TestAgentAnthropicCompatStreamStopClosesStalledUpstream(t *testing.T) {
	for _, mode := range []string{"context_cancel", "writer_disconnected"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(WithAgentInvocation(context.Background(), &AgentInvocationTrace{}))
			defer cancel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
			reader, writer := io.Pipe()
			t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
			stream := newAnthropicCompatStream(nil, c, &http.Response{Body: reader}, "test-agent-stop", "synthetic")
			defer stream.stop()
			if mode == "context_cancel" {
				cancel()
			} else {
				stream.markClientDisconnected()
			}
			done := make(chan bool, 1)
			go func() { _, ok := stream.nextLine(); done <- ok }()
			select {
			case ok := <-done:
				require.False(t, ok)
			case <-time.After(time.Second):
				t.Fatal("Agent stop continued draining stalled upstream")
			}
			require.True(t, stream.clientDisconnected)
			require.ErrorIs(t, stream.readErr, context.Canceled)
			closed := make(chan error, 1)
			go func() { _, err := writer.Write([]byte("late provider bytes")); closed <- err }()
			select {
			case err := <-closed:
				require.True(t, errors.Is(err, io.ErrClosedPipe), "upstream body was not closed")
			case <-time.After(time.Second):
				t.Fatal("upstream connection remains readable after Agent stop")
			}
		})
	}
}
