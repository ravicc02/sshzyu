package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type updateAPIKeyBatchRequest struct {
	Name          string   `json:"name" binding:"required,max=100"`
	GroupID       int64    `json:"group_id" binding:"required,gt=0"`
	Quota         float64  `json:"quota" binding:"gte=0"`
	QuotaTokens   int64    `json:"quota_tokens" binding:"gte=0"`
	AllowedModels []string `json:"allowed_models" binding:"max=200"`
}

func (h *APIKeyHandler) UpdateBatch(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req updateAPIKeyBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	keys, err := h.apiKeyService.UpdateBatch(c.Request.Context(), subject.UserID, c.Param("batch_id"), service.UpdateAPIKeyBatchRequest{Name: req.Name, GroupID: req.GroupID, Quota: req.Quota, QuotaTokens: req.QuotaTokens, AllowedModels: req.AllowedModels})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]*dto.APIKey, 0, len(keys))
	for i := range keys {
		out = append(out, dto.APIKeyFromService(&keys[i]).Localized(c.GetHeader("Accept-Language")))
	}
	response.Success(c, gin.H{"items": out})
}
