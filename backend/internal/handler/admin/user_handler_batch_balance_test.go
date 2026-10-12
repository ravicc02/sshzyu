package admin

import (
	"bytes"
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

type batchBalanceHandlerStub struct {
	*stubAdminService
	calls int
}

func (stub *batchBalanceHandlerStub) BatchUpdateBalance(context.Context, []int64, float64, string, string) service.BatchBalanceUpdateResult {
	stub.calls++
	return service.BatchBalanceUpdateResult{Success: 1, Items: []service.BatchBalanceUpdateItem{{UserID: 4, Success: true}}}
}

func TestBatchBalanceRequiresKeyAndReplaysWithoutExecutingAgain(t *testing.T) {
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(newMemoryIdempotencyRepoStub(), service.DefaultIdempotencyConfig()))
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(nil) })
	gin.SetMode(gin.TestMode)
	stub := &batchBalanceHandlerStub{stubAdminService: newStubAdminService()}
	handler := NewUserHandler(stub, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/batch-balance", handler.BatchUpdateBalance)
	execute := func(key string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/batch-balance", bytes.NewBufferString(`{"user_ids":[4],"amount":10,"operation":"add"}`))
		request.Header.Set("Content-Type", "application/json")
		if key != "" {
			request.Header.Set("Idempotency-Key", key)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}
	require.Equal(t, 400, execute("").Code)
	require.Zero(t, stub.calls)
	require.Equal(t, 200, execute("batch-balance-test-key").Code)
	replay := execute("batch-balance-test-key")
	require.Equal(t, 200, replay.Code)
	require.Equal(t, "true", replay.Header().Get("X-Idempotency-Replayed"))
	require.Equal(t, 1, stub.calls)
}
