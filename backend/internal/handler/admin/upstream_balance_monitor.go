package admin

import (
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/upstreammonitor"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"strconv"
)

func (h *AccountHandler) monitor(c *gin.Context) *upstreammonitor.Service {
	if h.upstreamBillingProbe == nil || h.upstreamBillingProbe.BalanceMonitor() == nil {
		response.Error(c, 503, "MONITOR_UNAVAILABLE")
		return nil
	}
	return h.upstreamBillingProbe.BalanceMonitor()
}
func monitorError(c *gin.Context, err error) {
	code := upstreammonitor.Code(err)
	status := http.StatusBadRequest
	switch code {
	case "MONITOR_NOT_FOUND", "MONITOR_CANDIDATE_NOT_FOUND":
		status = 404
	case "MONITOR_CONFIG_CHANGED", "MONITOR_ALREADY_ADDED", "MONITOR_QUERY_BUSY":
		status = 409
	case "MONITOR_INTERNAL_ERROR":
		status = 500
	}
	response.Error(c, status, code)
}
func monitorJSON(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		response.BadRequest(c, "MONITOR_INVALID_INPUT")
		return false
	}
	var extra any
	if decoder.Decode(&extra) == io.EOF {
		return true
	}
	response.BadRequest(c, "MONITOR_INVALID_INPUT")
	return false
}
func (h *AccountHandler) ListUpstreamBalances(c *gin.Context) {
	s := h.monitor(c)
	if s == nil {
		return
	}
	items, e := s.List(c.Request.Context())
	if e != nil {
		monitorError(c, e)
		return
	}
	response.Success(c, items)
}
func (h *AccountHandler) UpstreamBalanceCandidates(c *gin.Context) {
	s := h.monitor(c)
	if s == nil {
		return
	}
	items, e := s.Candidates(c.Request.Context())
	if e != nil {
		monitorError(c, e)
		return
	}
	response.Success(c, items)
}
func (h *AccountHandler) DiscoverUpstreamBalances(c *gin.Context) {
	s := h.monitor(c)
	if s == nil {
		return
	}
	var in struct {
		IDs []string `json:"candidate_ids"`
	}
	if !monitorJSON(c, &in) {
		return
	}
	items, e := s.Discover(c.Request.Context(), in.IDs)
	if e != nil {
		monitorError(c, e)
		return
	}
	response.Success(c, items)
}
func (h *AccountHandler) AddUpstreamBalance(c *gin.Context) {
	s := h.monitor(c)
	if s == nil {
		return
	}
	var in upstreammonitor.SiteInput
	if !monitorJSON(c, &in) {
		return
	}
	site, e := s.Add(c.Request.Context(), in)
	if e != nil {
		monitorError(c, e)
		return
	}
	response.Created(c, site)
}
func (h *AccountHandler) UpdateUpstreamBalance(c *gin.Context) {
	s := h.monitor(c)
	if s == nil {
		return
	}
	var in upstreammonitor.SiteInput
	if !monitorJSON(c, &in) {
		return
	}
	site, e := s.Update(c.Request.Context(), c.Param("id"), in)
	if e != nil {
		monitorError(c, e)
		return
	}
	response.Success(c, site)
}
func (h *AccountHandler) DeleteUpstreamBalance(c *gin.Context) {
	s := h.monitor(c)
	if s == nil {
		return
	}
	revision, e := strconv.ParseInt(c.Query("revision"), 10, 64)
	if e != nil || revision < 1 {
		response.BadRequest(c, "MONITOR_INVALID_INPUT")
		return
	}
	if e = s.Remove(c.Request.Context(), c.Param("id"), revision); e != nil {
		monitorError(c, e)
		return
	}
	response.Success(c, gin.H{"removed": true})
}
func (h *AccountHandler) RefreshUpstreamBalance(c *gin.Context) {
	s := h.monitor(c)
	if s == nil {
		return
	}
	site, e := s.Refresh(c.Request.Context(), c.Param("id"))
	if e != nil {
		monitorError(c, e)
		return
	}
	response.Success(c, site)
}
func (h *AccountHandler) UpstreamBalanceEvents(c *gin.Context) {
	s := h.monitor(c)
	if s == nil {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	events, e := s.Events(c.Request.Context(), c.Query("site_id"), c.Query("kind"), page)
	if e != nil {
		monitorError(c, e)
		return
	}
	response.Success(c, events)
}
func (h *AccountHandler) GetUpstreamBalanceFeishu(c *gin.Context) {
	s := h.monitor(c)
	if s == nil {
		return
	}
	cfg, e := s.Channel(c.Request.Context())
	if e != nil {
		monitorError(c, e)
		return
	}
	response.Success(c, cfg)
}
func (h *AccountHandler) SaveUpstreamBalanceFeishu(c *gin.Context) {
	s := h.monitor(c)
	if s == nil {
		return
	}
	var in upstreammonitor.ChannelInput
	if !monitorJSON(c, &in) {
		return
	}
	cfg, e := s.SaveChannel(c.Request.Context(), in)
	if e != nil {
		monitorError(c, e)
		return
	}
	response.Success(c, cfg)
}
func (h *AccountHandler) TestUpstreamBalanceFeishu(c *gin.Context) {
	s := h.monitor(c)
	if s == nil {
		return
	}
	var in struct {
		RequestID string `json:"request_id"`
		SiteID    string `json:"site_id"`
	}
	if !monitorJSON(c, &in) {
		return
	}
	result, e := s.TestNotification(c.Request.Context(), in.RequestID, in.SiteID)
	if e != nil {
		monitorError(c, e)
		return
	}
	response.Success(c, result)
}
