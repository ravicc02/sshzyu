package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func fairShareGinContext(key int64) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if key > 0 {
		c.Set("api_key", &APIKey{ID: key, UserID: key + 100})
	}
	return c
}
func TestOpenAIFairShareSessionHashIsolatedByAuthenticatedKey(t *testing.T) {
	svc := &OpenAIGatewayService{}
	body := []byte(`{"model":"model","prompt_cache_key":"same-session","input":"same text","user_id":999}`)
	a, b := fairShareGinContext(1), fairShareGinContext(2)
	first := svc.GenerateSessionHash(a, body)
	second := svc.GenerateSessionHash(b, body)
	require.NotEmpty(t, first)
	require.NotEqual(t, first, second)
	require.Equal(t, first, svc.GenerateExplicitSessionHash(a, body))
	require.Equal(t, second, svc.GenerateExplicitSessionHash(b, body))
	require.Equal(t, first, svc.GenerateSessionHash(a, body))
	require.NotEqual(t, openAILegacySessionHashFromContext(a.Request.Context()), openAILegacySessionHashFromContext(b.Request.Context()))
	// The compatibility lookup must also remain tenant scoped, not read the old
	// global binding that caused different users to share one account pin.
	_, oldLegacy := deriveOpenAISessionHashes("same-session")
	require.NotEqual(t, oldLegacy, openAILegacySessionHashFromContext(a.Request.Context()))
}
func TestOpenAIFairShareHeaderAndFallbackScopingIsStable(t *testing.T) {
	svc := &OpenAIGatewayService{}
	a, b := fairShareGinContext(10), fairShareGinContext(11)
	a.Request.Header.Set("session_id", "default")
	b.Request.Header.Set("session_id", "default")
	require.NotEqual(t, svc.GenerateSessionHash(a, nil), svc.GenerateSessionHash(b, nil))
	a.Request.Header.Del("session_id")
	b.Request.Header.Del("session_id")
	require.NotEqual(t, svc.GenerateSessionHashWithFallback(a, nil, "fallback"), svc.GenerateSessionHashWithFallback(b, nil, "fallback"))
	require.Equal(t, svc.GenerateSessionHashWithFallback(a, nil, "fallback"), svc.GenerateSessionHashWithFallback(a, nil, "fallback"))
	require.Equal(t, DeriveSessionHashFromSeed("fallback"), svc.GenerateSessionHashWithFallback(fairShareGinContext(0), nil, "fallback"))
}

func TestOpenAIFairSharePreservesSeparateGrokAffinityContract(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c := fairShareGinContext(901)
	c.Set("api_key", &APIKey{ID: 901, Group: &Group{Platform: PlatformGrok}})
	c.Request.Header.Set("session_id", "grok-session")
	body := []byte(`{"model":"grok-4","prompt_cache_key":"body-session"}`)
	require.Equal(t, DeriveSessionHashFromSeed("grok-session"), svc.GenerateExplicitSessionHash(c, body))
	require.Equal(t, DeriveSessionHashFromSeed(grokStickyAffinitySeed("grok-session", body)), svc.GenerateSessionHash(c, body))
}
