package admin

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLotteryNeedsAttentionRejectsInvalidQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, raw := range []string{"pending", "yes", "-1"} {
		t.Run(raw, func(t *testing.T) {
			r := gin.New()
			h := NewAdminLotteryHandler(nil)
			r.GET("/draws", h.AdminListDraws)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/draws?needs_attention="+raw, nil))
			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

type lotteryAttentionRepo struct {
	service.LotteryRepository
	filter service.LotteryDrawListFilter
	params pagination.PaginationParams
}

func (r *lotteryAttentionRepo) ListDrawsAdmin(_ context.Context, f service.LotteryDrawListFilter, p pagination.PaginationParams) ([]service.LotteryDrawRecord, *pagination.PaginationResult, error) {
	r.filter = f
	r.params = p
	return nil, &pagination.PaginationResult{Total: 9}, nil
}
func TestLotteryNeedsAttentionQueryReachesRepository(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		raw  string
		want bool
	}{{"", false}, {"true", true}, {"false", false}, {"1", true}} {
		t.Run(tc.raw, func(t *testing.T) {
			repo := &lotteryAttentionRepo{}
			h := NewAdminLotteryHandler(service.NewLotteryService(repo, nil, nil, nil))
			r := gin.New()
			r.GET("/draws", h.AdminListDraws)
			url := "/draws?page_size=1&user_id=42"
			if tc.raw != "" {
				url += "&needs_attention=" + tc.raw
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, tc.want, repo.filter.NeedsAttention)
			require.EqualValues(t, 42, *repo.filter.UserID)
			require.Equal(t, 1, repo.params.PageSize)
			require.Contains(t, w.Body.String(), `"total":9`)
		})
	}
}
