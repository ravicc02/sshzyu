package admin

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminLotteryHandler 抽奖管理端 HTTP handler。
//
// 包含活动/奖品配置、流水查询、发放重试、次数调整,以及仅面向本地
// 测试环境的辅助能力(消耗偏移、用户重置)。
type AdminLotteryHandler struct {
	lotteryService *service.LotteryService
}

// NewAdminLotteryHandler 构造抽奖管理端 handler。
func NewAdminLotteryHandler(lotteryService *service.LotteryService) *AdminLotteryHandler {
	return &AdminLotteryHandler{lotteryService: lotteryService}
}

// AdminUpdateActivityRequest PUT /admin/lottery/activity/:id 请求体(局部更新,nil 不改)。
type AdminUpdateActivityRequest struct {
	Name     *string `json:"name"`
	Status   *string `json:"status"`
	StartsAt *string `json:"starts_at"`
	EndsAt   *string `json:"ends_at"`
}

// AdminUpdatePrizeRequest PUT /admin/lottery/prizes/:id 请求体(局部更新,nil 不改)。
type AdminUpdatePrizeRequest struct {
	Name      *string  `json:"name"`
	PrizeType *string  `json:"prize_type"`
	Value     *float64 `json:"value"`
	Weight    *int     `json:"weight"`
	// MinTier 可中该奖的最低阶梯(0 青铜/1 白银/2 黄金/3 钻石/4 王者)。
	MinTier *int `json:"min_tier"`
	// TierWeights 按阶梯覆盖权重,key 为 tier 数字字符串(如 "0".."4")。
	TierWeights *map[string]int `json:"tier_weights"`
	Stock       *int            `json:"stock"`
	Enabled     *bool           `json:"enabled"`
	SortOrder   *int            `json:"sort_order"`
}

// AdminAdjustDrawsRequest POST /admin/lottery/users/:id/adjust 请求体。
type AdminAdjustDrawsRequest struct {
	Delta int `json:"delta" binding:"required"`
}

// AdminSetSpendOffsetRequest POST /admin/lottery/users/:id/spend-offset 请求体。
type AdminSetSpendOffsetRequest struct {
	// OffsetCents 本地测试消耗偏移(美分)。
	OffsetCents int64 `json:"offset_cents"`
}

// AdminListDraws GET /admin/lottery/draws?user_id=&page=&page_size=
func (h *AdminLotteryHandler) AdminListDraws(c *gin.Context) {
	filter := service.LotteryDrawListFilter{}
	if raw := c.Query("user_id"); raw != "" {
		uid, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || uid <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filter.UserID = &uid
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	records, result, err := h.lotteryService.AdminListDraws(c.Request.Context(), filter, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.LotteryDraw, 0, len(records))
	for i := range records {
		out = append(out, *dto.LotteryDrawFromService(&records[i]))
	}
	response.Paginated(c, out, result.Total, page, pageSize)
}

// AdminGetActivities GET /admin/lottery/activities
func (h *AdminLotteryHandler) AdminGetActivities(c *gin.Context) {
	activities, err := h.lotteryService.AdminListActivities(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]*dto.LotteryAdminActivity, 0, len(activities))
	for i := range activities {
		out = append(out, dto.LotteryAdminActivityFromService(&activities[i]))
	}
	response.Success(c, gin.H{"activities": out})
}

// AdminGetPrizes GET /admin/lottery/prizes?activity_id=
// 返回奖品原始配置(含停用),供管理页展示与编辑;不做概率换算。
func (h *AdminLotteryHandler) AdminGetPrizes(c *gin.Context) {
	raw := c.Query("activity_id")
	if raw == "" {
		response.BadRequest(c, "activity_id is required")
		return
	}
	activityID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || activityID <= 0 {
		response.BadRequest(c, "Invalid activity_id")
		return
	}
	prizes, err := h.lotteryService.AdminListPrizes(c.Request.Context(), activityID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]*dto.LotteryAdminPrize, 0, len(prizes))
	for i := range prizes {
		out = append(out, dto.LotteryAdminPrizeFromService(&prizes[i]))
	}
	response.Success(c, gin.H{"prizes": out})
}

// AdminUpdateActivity PUT /admin/lottery/activity/:id
func (h *AdminLotteryHandler) AdminUpdateActivity(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid activity id")
		return
	}
	var req AdminUpdateActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	input := service.LotteryActivityUpdateInput{Name: req.Name, Status: req.Status}
	if req.StartsAt != nil {
		t, perr := parseLotteryTimeParam(*req.StartsAt)
		if perr != nil {
			response.BadRequest(c, "Invalid starts_at: "+perr.Error())
			return
		}
		input.StartsAt = t
	}
	if req.EndsAt != nil {
		t, perr := parseLotteryTimeParam(*req.EndsAt)
		if perr != nil {
			response.BadRequest(c, "Invalid ends_at: "+perr.Error())
			return
		}
		input.EndsAt = t
	}
	if err := h.lotteryService.AdminUpdateActivity(c.Request.Context(), id, input); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"updated": true})
}

// AdminUpdatePrize PUT /admin/lottery/prizes/:id
func (h *AdminLotteryHandler) AdminUpdatePrize(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid prize id")
		return
	}
	var req AdminUpdatePrizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	input := service.LotteryPrizeUpdateInput{
		Name:        req.Name,
		PrizeType:   req.PrizeType,
		Value:       req.Value,
		Weight:      req.Weight,
		MinTier:     req.MinTier,
		TierWeights: req.TierWeights,
		Stock:       req.Stock,
		Enabled:     req.Enabled,
		SortOrder:   req.SortOrder,
	}
	if err := h.lotteryService.AdminUpdatePrize(c.Request.Context(), id, input); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"updated": true})
}

// AdminAdjustDraws POST /admin/lottery/users/:id/adjust
// 补发(正数)/回收(负数)抽奖次数。
func (h *AdminLotteryHandler) AdminAdjustDraws(c *gin.Context) {
	uid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || uid <= 0 {
		response.BadRequest(c, "Invalid user id")
		return
	}
	var req AdminAdjustDrawsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.lotteryService.AdminAdjustDraws(c.Request.Context(), uid, req.Delta); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"adjusted": true})
}

// AdminSetSpendOffset POST /admin/lottery/users/:id/spend-offset
// 本地测试辅助: 设置消耗偏移(美分),叠加在真实消耗之上验证阶梯解锁。
func (h *AdminLotteryHandler) AdminSetSpendOffset(c *gin.Context) {
	uid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || uid <= 0 {
		response.BadRequest(c, "Invalid user id")
		return
	}
	var req AdminSetSpendOffsetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.lotteryService.AdminSetSpendOffset(c.Request.Context(), uid, req.OffsetCents); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"updated": true})
}

// AdminRetryFulfillment POST /admin/lottery/draws/:id/retry-fulfillment
// 重试失败/待处理的奖品发放。
func (h *AdminLotteryHandler) AdminRetryFulfillment(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid draw id")
		return
	}
	record, err := h.lotteryService.AdminRetryFulfillment(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"draw": dto.LotteryDrawFromService(record)})
}

// AdminResetUser POST /admin/lottery/users/:id/reset
// 本地测试辅助: 清空用户抽奖状态与流水,重新从首抽开始。
func (h *AdminLotteryHandler) AdminResetUser(c *gin.Context) {
	uid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || uid <= 0 {
		response.BadRequest(c, "Invalid user id")
		return
	}
	if err := h.lotteryService.AdminResetUser(c.Request.Context(), uid); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"reset": true})
}

// parseLotteryTimeParam 解析活动时间参数(RFC3339)。
func parseLotteryTimeParam(raw string) (*time.Time, error) {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
