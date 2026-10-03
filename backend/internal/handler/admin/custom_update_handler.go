package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/deployment"
	"github.com/Wei-Shaw/sub2api/pkg/release"
	"github.com/gin-gonic/gin"
)

var customOperationPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (handler *SystemHandler) WithUpdateAuthorization(totp *service.TotpService, users *service.UserService) *SystemHandler {
	handler.updateTotp, handler.updateUsers = totp, users
	return handler
}

func (handler *SystemHandler) customService(c *gin.Context, write bool) *service.UpdateService {
	svc, ok := handler.updateSvc.(*service.UpdateService)
	if !ok || !svc.IsCustomDistribution() || svc.Agent() == nil {
		middleware2.AbortWithError(c, 503, "UPDATER_NOT_CONFIGURED", "The custom updater requires initial installation")
		return nil
	}
	if write {
		subject, ok := middleware2.GetAuthSubjectFromContext(c)
		if !ok || subject.UserID <= 0 || c.GetString("auth_method") != "jwt" {
			middleware2.AbortWithError(c, 403, "UPDATE_HUMAN_ADMIN_REQUIRED", "A verified administrator session is required")
			return nil
		}
		origin := c.GetHeader("Origin")
		if origin != "" {
			parsed, err := url.Parse(origin)
			expected := os.Getenv("SSHZY_UPDATE_SITE_ORIGIN")
			allowed := err == nil && parsed.User == nil && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == "" &&
				(parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host == c.Request.Host
			if expected != "" {
				allowed = err == nil && origin == expected
			}
			if !allowed {
				middleware2.AbortWithError(c, 403, "UPDATE_ORIGIN_REJECTED", "Update requests must originate from this site")
				return nil
			}
		}
	}
	return svc
}

func decodeUpdateInput(c *gin.Context, input any) bool {
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 32769))
	decoder.DisallowUnknownFields()
	if decoder.Decode(input) != nil || decoder.Decode(new(any)) != io.EOF {
		middleware2.AbortWithError(c, 400, "INVALID_UPDATE_REQUEST", "Invalid update request")
		return false
	}
	return true
}

func (handler *SystemHandler) forwardUpdate(c *gin.Context, svc *service.UpdateService, method, path string, input any, accepted bool) {
	var output json.RawMessage
	if err := svc.Agent().Call(c.Request.Context(), method, path, input, &output); err != nil {
		code := err.Error()
		if len(code) > 96 || strings.Trim(code, "ABCDEFGHIJKLMNOPQRSTUVWXYZ_0123456789") != "" {
			code = "UPDATER_UNAVAILABLE"
		}
		middleware2.AbortWithError(c, 409, code, "The custom update operation could not proceed")
		return
	}
	if accepted {
		c.JSON(http.StatusAccepted, gin.H{"code": 0, "data": output})
	} else {
		response.Success(c, output)
	}
}

func (handler *SystemHandler) CustomReleases(c *gin.Context) {
	svc := handler.customService(c, false)
	if svc == nil {
		return
	}
	targets, err := svc.CustomReleases(c.Request.Context())
	if err != nil {
		middleware2.AbortWithError(c, 503, "CUSTOM_SOURCE_UNAVAILABLE", "Unable to verify custom releases")
		return
	}
	response.Success(c, gin.H{"releases": targets})
}

func (handler *SystemHandler) PrepareCustomUpdate(c *gin.Context) {
	svc := handler.customService(c, true)
	if svc == nil {
		return
	}
	if _, err := svc.CustomCapabilities(c.Request.Context()); err != nil {
		middleware2.AbortWithError(c, 503, "UPDATER_BOOTSTRAP_REQUIRED", "The updater must match this installation")
		return
	}
	var input struct {
		Version      string `json:"version"`
		ManifestHash string `json:"manifest_hash"`
	}
	if !decodeUpdateInput(c, &input) {
		return
	}
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	key := c.GetHeader("Idempotency-Key")
	kind := "update"
	if strings.HasSuffix(c.FullPath(), "/rollback") {
		kind = "rollback"
	}
	handler.forwardUpdate(c, svc, http.MethodPost, "/v1/prepare", gin.H{
		"version": input.Version, "manifest_hash": input.ManifestHash, "actor_id": subject.UserID,
		"idempotency_key": key, "kind": kind,
	}, true)
}

func (handler *SystemHandler) CustomOperation(c *gin.Context) {
	svc := handler.customService(c, false)
	if svc == nil {
		return
	}
	id := c.Param("id")
	if !customOperationPattern.MatchString(id) {
		middleware2.AbortWithError(c, 400, "INVALID_OPERATION_ID", "Invalid operation identifier")
		return
	}
	handler.forwardUpdate(c, svc, http.MethodGet, "/v1/operations/"+id, nil, false)
}

func (handler *SystemHandler) ActivateCustomUpdate(c *gin.Context) {
	svc := handler.customService(c, true)
	if svc == nil {
		return
	}
	id := c.Param("id")
	if !customOperationPattern.MatchString(id) {
		middleware2.AbortWithError(c, 400, "INVALID_OPERATION_ID", "Invalid operation identifier")
		return
	}
	if handler.updateTotp == nil || handler.updateUsers == nil {
		middleware2.AbortWithError(c, 503, "STEP_UP_UNAVAILABLE", "Recent two-factor verification is required")
		return
	}
	if !middleware2.EnforceStepUpAlways(c, handler.updateTotp, handler.updateUsers) {
		return
	}
	var input struct {
		ManifestHash      string   `json:"manifest_hash"`
		ConfirmDowntime   bool     `json:"confirm_downtime"`
		ConfirmMigrations []string `json:"confirm_migrations"`
	}
	if !decodeUpdateInput(c, &input) {
		return
	}
	handler.forwardUpdate(c, svc, http.MethodPost, "/v1/operations/"+id+"/activate", input, true)
}

func (handler *SystemHandler) CancelCustomUpdate(c *gin.Context) {
	svc := handler.customService(c, true)
	if svc == nil {
		return
	}
	id := c.Param("id")
	if !customOperationPattern.MatchString(id) {
		middleware2.AbortWithError(c, 400, "INVALID_OPERATION_ID", "Invalid operation identifier")
		return
	}
	handler.forwardUpdate(c, svc, http.MethodPost, "/v1/operations/"+id+"/cancel", nil, false)
}

func (handler *SystemHandler) DeploymentState(c *gin.Context) {
	if !service.ValidControlAuthorization(c.GetHeader("Authorization")) {
		c.Status(http.StatusUnauthorized)
		return
	}
	svc, ok := handler.updateSvc.(*service.UpdateService)
	if !ok {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	state, inflight, err := deployment.Default.Status()
	info := svc.LocalVersionInfo()
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	c.JSON(200, gin.H{"protocol": release.Protocol, "active": state.Active, "operation_id": state.OperationID,
		"inflight": inflight, "version": info.CurrentVersion, "commit": info.Commit, "build_type": info.BuildType})
}
