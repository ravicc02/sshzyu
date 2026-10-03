package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/deployment"
	"github.com/gin-gonic/gin"
)

func TestDeploymentMaintenanceKeepsHealthAndUpdateStatusAccessible(t *testing.T) {
	gin.SetMode(gin.TestMode)
	path := filepath.Join(t.TempDir(), "maintenance.json")
	os.WriteFile(path, []byte(`{"active":true,"operation_id":"test"}`), 0600)
	router := gin.New()
	router.Use(DeploymentMaintenance(deployment.NewGate(path)))
	router.GET("/health", func(c *gin.Context) { c.Status(200) })
	router.GET("/api/v1/admin/system/version", func(c *gin.Context) { c.Status(200) })
	router.POST("/v1/responses", func(c *gin.Context) { c.Status(200) })
	for _, test := range []struct {
		method, path string
		expected     int
	}{
		{"GET", "/health", 200}, {"GET", "/api/v1/admin/system/version", 200}, {"POST", "/v1/responses", 503},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != test.expected {
			t.Fatalf("%s: %d", test.path, response.Code)
		}
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	router.ServeHTTP(response, request)
	if response.Header().Get("Retry-After") == "" {
		t.Fatal("maintenance has no retry hint")
	}
}
