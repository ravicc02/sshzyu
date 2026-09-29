package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestAdminUserList_ParsesAPIKeyProvider verifies api_key_provider parsing:
// only known provider buckets are captured, anything else means "no filter".
func TestAdminUserList_ParsesAPIKeyProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"anthropic", "?api_key_provider=anthropic", "anthropic"},
		{"openai", "?api_key_provider=openai", "openai"},
		{"domestic", "?api_key_provider=domestic", "domestic"},
		{"other", "?api_key_provider=other", "other"},
		{"missing", "", ""},
		{"unknown ignored", "?api_key_provider=qwen", ""},
		{"whitespace trimmed", "?api_key_provider=%20domestic%20", "domestic"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &listUsersFilterStub{AdminService: newStubAdminService()}
			r := gin.New()
			h := NewUserHandler(stub, nil, nil, nil, nil, nil, nil)
			r.GET("/admin/users", h.List)

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/admin/users"+tc.query, nil)
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, tc.want, stub.captured.APIKeyProvider)
		})
	}
}

// TestKeyGroupProviderPlatformsUnit guards the domain mapping contract used by
// both the handler validation and the repository filter.
func TestKeyGroupProviderPlatformsUnit(t *testing.T) {
	include, excludeAll, ok := domain.KeyGroupProviderPlatforms("domestic")
	require.True(t, ok)
	require.False(t, excludeAll)
	require.ElementsMatch(t, []string{"kimi", "zhipu", "deepseek", "minimax"}, include)

	_, _, ok = domain.KeyGroupProviderPlatforms("qwen")
	require.False(t, ok, "unknown provider must not resolve")

	_, excludeAll, ok = domain.KeyGroupProviderPlatforms("other")
	require.True(t, ok)
	require.True(t, excludeAll, "other must be resolved by exclusion")

	// The exclusion list ("other" = everything not explicitly bucketed) must be
	// exactly the union of all include buckets: no overlap between buckets, and
	// every explicitly-classified platform is covered.
	union := make(map[string]struct{})
	for _, provider := range []string{"anthropic", "openai", "domestic"} {
		inc, excl, ok := domain.KeyGroupProviderPlatforms(provider)
		require.True(t, ok)
		require.False(t, excl)
		for _, p := range inc {
			_, dup := union[p]
			require.False(t, dup, "platform %s must not appear in two buckets", p)
			union[p] = struct{}{}
		}
	}
	excludedList := domain.KeyGroupProviderExcludePlatforms()
	require.Len(t, excludedList, len(union), "exclusion list must cover exactly the union of include buckets")
	for _, p := range excludedList {
		_, covered := union[p]
		require.True(t, covered, "platform %s in exclusion list is not part of any include bucket", p)
	}
}
