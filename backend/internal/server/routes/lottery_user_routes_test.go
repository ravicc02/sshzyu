package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 用户抽奖接口必须注册在认证用户路由中。
// 未登录请求应被认证中间件拒绝，而不是落到 Gin 的框架级 404。
func TestUserLotteryRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Lottery: &handler.LotteryHandler{}}
	jwtAuth := servermiddleware.JWTAuthMiddleware(func(c *gin.Context) {
		c.AbortWithStatus(http.StatusUnauthorized)
	})
	auditLog := servermiddleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })

	RegisterUserRoutes(router.Group("/api/v1"), handlers, jwtAuth, auditLog, nil, nil)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/lottery/activity"},
		{http.MethodGet, "/api/v1/lottery/status"},
		{http.MethodPost, "/api/v1/lottery/draw"},
		{http.MethodGet, "/api/v1/lottery/records"},
		{http.MethodGet, "/api/v1/lottery/rewards"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tc.method, tc.path, nil)
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusUnauthorized, recorder.Code)
		})
	}
}
