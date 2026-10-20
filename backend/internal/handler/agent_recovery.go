package handler

import "github.com/gin-gonic/gin"

func (h *AgentHandler) RecoverImage(c *gin.Context) {
	user, ok := h.user(c)
	if !ok {
		return
	}
	run, err := h.svc.QueueImageRecovery(c.Request.Context(), user, c.Param("run_id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(202, run.Public())
}
