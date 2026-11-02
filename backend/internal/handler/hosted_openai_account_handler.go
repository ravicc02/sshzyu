package handler

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type HostedOpenAIAccountHandler struct {
	service *service.HostedOpenAIAccountService
}

func NewHostedOpenAIAccountHandler(service *service.HostedOpenAIAccountService) *HostedOpenAIAccountHandler {
	return &HostedOpenAIAccountHandler{service: service}
}

type hostedOpenAIAuthURLRequest struct {
	ProxyID     *int64 `json:"proxy_id"`
	RedirectURI string `json:"redirect_uri"`
}

type hostedOpenAIAccountConfigRequest struct {
	Model            string `json:"model"`
	ProxyID          *int64 `json:"proxy_id"`
	SupportsMessages *bool  `json:"supports_messages"`
}

type hostedOpenAICreateRequest struct {
	SessionID        string `json:"session_id"`
	Code             string `json:"code"`
	State            string `json:"state"`
	CallbackURL      string `json:"callback_url"`
	RedirectURI      string `json:"redirect_uri"`
	Model            string `json:"model"`
	ProxyID          *int64 `json:"proxy_id"`
	SupportsMessages bool   `json:"supports_messages"`
}

type hostedOpenAIReauthorizeRequest struct {
	SessionID   string `json:"session_id"`
	Code        string `json:"code"`
	State       string `json:"state"`
	CallbackURL string `json:"callback_url"`
	RedirectURI string `json:"redirect_uri"`
}

type hostedOpenAIStatusRequest struct {
	Status string `json:"status"`
}

func (h *HostedOpenAIAccountHandler) GenerateAuthURL(c *gin.Context) {
	userID, ok := currentHostedUserID(c)
	if !ok {
		return
	}
	_ = userID
	var req hostedOpenAIAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.service.GenerateAuthURL(c.Request.Context(), req.ProxyID, req.RedirectURI)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *HostedOpenAIAccountHandler) Options(c *gin.Context) {
	if _, ok := currentHostedUserID(c); !ok {
		return
	}
	result, err := h.service.Options(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *HostedOpenAIAccountHandler) List(c *gin.Context) {
	userID, ok := currentHostedUserID(c)
	if !ok {
		return
	}
	items, err := h.service.ListForUser(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *HostedOpenAIAccountHandler) Create(c *gin.Context) {
	userID, ok := currentHostedUserID(c)
	if !ok {
		return
	}
	var req hostedOpenAICreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	code, state := hostedOAuthCodeAndState(req.Code, req.State, req.CallbackURL)
	if code == "" || state == "" {
		response.BadRequest(c, "OpenAI callback URL must contain code and state")
		return
	}
	item, err := h.service.Create(c.Request.Context(), userID, service.CreateHostedOpenAIAccountInput{
		SessionID:        strings.TrimSpace(req.SessionID),
		Code:             code,
		State:            state,
		RedirectURI:      strings.TrimSpace(req.RedirectURI),
		Model:            strings.TrimSpace(req.Model),
		ProxyID:          req.ProxyID,
		SupportsMessages: req.SupportsMessages,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, item)
}

func (h *HostedOpenAIAccountHandler) Update(c *gin.Context) {
	userID, ok := currentHostedUserID(c)
	if !ok {
		return
	}
	id, ok := hostedPathID(c)
	if !ok {
		return
	}
	var req hostedOpenAIAccountConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.service.UpdateForUser(c.Request.Context(), userID, id, service.UpdateHostedOpenAIAccountInput{
		Model:            req.Model,
		ProxyID:          req.ProxyID,
		SupportsMessages: req.SupportsMessages,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *HostedOpenAIAccountHandler) RefreshQuota(c *gin.Context) {
	userID, ok := currentHostedUserID(c)
	if !ok {
		return
	}
	id, ok := hostedPathID(c)
	if !ok {
		return
	}
	item, err := h.service.RefreshQuotaForUser(c.Request.Context(), userID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *HostedOpenAIAccountHandler) Reauthorize(c *gin.Context) {
	userID, ok := currentHostedUserID(c)
	if !ok {
		return
	}
	id, ok := hostedPathID(c)
	if !ok {
		return
	}
	var req hostedOpenAIReauthorizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	code, state := hostedOAuthCodeAndState(req.Code, req.State, req.CallbackURL)
	if code == "" || state == "" {
		response.BadRequest(c, "OpenAI callback URL must contain code and state")
		return
	}
	item, err := h.service.ReauthorizeForUser(c.Request.Context(), userID, id, service.ReauthorizeHostedOpenAIAccountInput{
		SessionID:   strings.TrimSpace(req.SessionID),
		Code:        code,
		State:       state,
		RedirectURI: strings.TrimSpace(req.RedirectURI),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *HostedOpenAIAccountHandler) AdminList(c *gin.Context) {
	items, err := h.service.ListForAdmin(c.Request.Context(), c.Query("search"), c.Query("status"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *HostedOpenAIAccountHandler) AdminUpdate(c *gin.Context) {
	id, ok := hostedPathID(c)
	if !ok {
		return
	}
	var req hostedOpenAIAccountConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.service.UpdateForAdmin(c.Request.Context(), id, service.UpdateHostedOpenAIAccountInput{
		Model:            req.Model,
		ProxyID:          req.ProxyID,
		SupportsMessages: req.SupportsMessages,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *HostedOpenAIAccountHandler) AdminRefreshQuota(c *gin.Context) {
	id, ok := hostedPathID(c)
	if !ok {
		return
	}
	item, err := h.service.RefreshQuotaForAdmin(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *HostedOpenAIAccountHandler) AdminSetStatus(c *gin.Context) {
	id, ok := hostedPathID(c)
	if !ok {
		return
	}
	var req hostedOpenAIStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.service.SetStatusForAdmin(c.Request.Context(), id, req.Status)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func currentHostedUserID(c *gin.Context) (int64, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return 0, false
	}
	return subject.UserID, true
}

func hostedPathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid hosted account ID")
		return 0, false
	}
	return id, true
}

func hostedOAuthCodeAndState(code, state, callbackURL string) (string, string) {
	code = strings.TrimSpace(code)
	state = strings.TrimSpace(state)
	callbackURL = strings.TrimSpace(callbackURL)
	if callbackURL == "" {
		return code, state
	}
	parsed, err := url.Parse(callbackURL)
	if err != nil {
		return code, state
	}
	query := parsed.Query()
	if code == "" {
		code = strings.TrimSpace(query.Get("code"))
	}
	if state == "" {
		state = strings.TrimSpace(query.Get("state"))
	}
	return code, state
}
