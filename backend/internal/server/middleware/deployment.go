package middleware

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/pkg/deployment"
	"github.com/Wei-Shaw/sub2api/pkg/release"
	"github.com/gin-gonic/gin"
)

func DeploymentMaintenance(gate *deployment.Gate) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/health" || path == "/health/deployment" || strings.HasPrefix(path, "/api/v1/admin/system/") {
			c.Next()
			return
		}
		contract := c.GetHeader("X-Sshzy-Client-Contract")
		if contract != "" && contract != release.ClientContract && c.Request.Method != http.MethodGet && strings.HasPrefix(path, "/api/") {
			AbortWithError(c, 409, "CLIENT_UPGRADE_REQUIRED", "Refresh the page before continuing")
			return
		}
		done, err := gate.Enter()
		if err != nil {
			c.Header("Retry-After", "30")
			AbortWithError(c, 503, "DEPLOYMENT_MAINTENANCE", "The site is temporarily undergoing an approved update")
			return
		}
		defer done()
		c.Next()
	}
}
