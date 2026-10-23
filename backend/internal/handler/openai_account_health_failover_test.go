//go:build unit

package handler

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayHandlerResponses_Health502And503SwitchBeforeRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"generic502", 502, `{"error":{"type":"server_error","message":"Bad gateway"}}`},
		{"generic503", 503, `{"error":{"type":"server_error","message":"Service unavailable"}}`},
		{"overloaded503", 503, `{"error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := newAstraProCapturedUpstream(&http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, astra200())
			handler := newOpenAIResponsesFailoverTestHandler(t, upstream)
			c, rec := newAstraProFailoverContext(t, astraProRequestBody)
			handler.Responses(c)
			_, accounts, _ := upstream.snapshot()
			require.Equal(t, []int64{1, 2}, accounts, "the second upstream call must use a different healthy account")
			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Body.String(), `"status":"completed"`)
		})
	}
}
