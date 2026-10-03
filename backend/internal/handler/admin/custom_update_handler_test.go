package admin

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type handlerAgent struct{ calls int }

func (agent *handlerAgent) Call(context.Context, string, string, any, any) error {
	agent.calls++
	return nil
}

func TestCustomUpdateRejectsMachineAndCrossOriginBeforeHostCall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct{ method, origin string }{{"admin_api_key", ""}, {"jwt", "https://other.example"}} {
		agent := &handlerAgent{}
		handler := NewSystemHandler(service.NewUpdateService(nil, nil, "0.2.13-r3", "release").WithAgent(agent), nil)
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 1})
			c.Set("auth_method", test.method)
		})
		router.POST("/prepare", handler.PrepareCustomUpdate)
		request := httptest.NewRequest("POST", "/prepare", strings.NewReader(`{"version":"0.2.13-r3"}`))
		request.Header.Set("Origin", test.origin)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != 403 || agent.calls != 0 {
			t.Fatalf("unsafe host access: %d %d", response.Code, agent.calls)
		}
	}
}

func TestCustomRestartCannotUseLegacyExit(t *testing.T) {
	handler := NewSystemHandler(service.NewUpdateService(nil, nil, "0.2.13-r3", "release"), nil)
	router := gin.New()
	router.POST("/restart", handler.RestartService)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("POST", "/restart", nil))
	if response.Code != 409 {
		t.Fatal("legacy restart was not blocked")
	}
}
