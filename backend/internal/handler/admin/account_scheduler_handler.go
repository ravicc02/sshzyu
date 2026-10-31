package admin

import (
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type AccountSchedulerHandler struct {
	service *service.AccountSchedulerService
}

func NewAccountSchedulerHandler(service *service.AccountSchedulerService) *AccountSchedulerHandler {
	return &AccountSchedulerHandler{service: service}
}

func (h *AccountSchedulerHandler) Overview(c *gin.Context) {
	overview, err := h.service.Overview(c.Request.Context())
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, overview)
}

func (h *AccountSchedulerHandler) UpdateSetting(c *gin.Context) {
	var input service.AccountSchedulerSettingInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	setting, err := h.service.UpdateSetting(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, service.ErrAccountSchedulerInvalidSetting) {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, setting)
}

func (h *AccountSchedulerHandler) Run(c *gin.Context) {
	var input service.AccountSchedulerRunInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	result, err := h.service.RunNow(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, service.ErrAccountSchedulerInvalidSetting) {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}
