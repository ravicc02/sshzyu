package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/agent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/gin-gonic/gin"
)

func (h *AgentHandler) ReviewWork(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	var in struct {
		Revision int             `json:"revision"`
		Draft    agent.WorkDraft `json:"draft"`
		ItemIDs  []string        `json:"item_ids,omitempty"`
	}
	if err := readAgentJSON(c, &in); err != nil {
		h.fail(c, err)
		return
	}
	p, err := h.svc.ReviewWorkItems(c.Request.Context(), u, c.Param("plan_id"), ip.GetSecurityClientIP(c, h.cfg.TrustForwardedIPForAPIKeyACL()), in.Revision, in.Draft, in.ItemIDs)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(200, p)
}
func (h *AgentHandler) SubmitWork(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	var in struct {
		Revision int `json:"revision"`
	}
	if err := readAgentJSON(c, &in); err != nil {
		h.fail(c, err)
		return
	}
	r, err := h.svc.SubmitWork(c.Request.Context(), u, c.Param("plan_id"), ip.GetSecurityClientIP(c, h.cfg.TrustForwardedIPForAPIKeyACL()), in.Revision)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(202, r.Public())
}

func (h *AgentHandler) CreateGrant(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	var in agent.GrantRequest
	if err := readAgentJSON(c, &in); err != nil {
		h.fail(c, err)
		return
	}
	g, err := h.svc.CreateGrant(c.Request.Context(), u, c.Param("plan_id"), ip.GetSecurityClientIP(c, h.cfg.TrustForwardedIPForAPIKeyACL()), in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(201, g)
}
func (h *AgentHandler) RevokeGrant(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	if err := h.svc.RevokeGrant(c.Request.Context(), u, c.Param("grant_id")); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(204)
}
func (h *AgentHandler) SaveDraft(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	var in struct {
		Revision int             `json:"revision"`
		Draft    agent.WorkDraft `json:"draft"`
	}
	if err := readAgentJSON(c, &in); err != nil {
		h.fail(c, err)
		return
	}
	p, err := h.svc.SaveDraft(c.Request.Context(), u, c.Param("plan_id"), in.Revision, in.Draft)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(200, p)
}
