package handler

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// LotteryHandler 抽奖用户端 HTTP handler。
type LotteryHandler struct {
	lotteryService *service.LotteryService
}

// NewLotteryHandler 构造抽奖用户端 handler。
func NewLotteryHandler(lotteryService *service.LotteryService) *LotteryHandler {
	return &LotteryHandler{lotteryService: lotteryService}
}

// LotteryStatusResponse GET /lottery/status 响应体。
type LotteryStatusResponse struct {
	ActivityOpen         bool    `json:"activity_open"`
	AvailableDraws       int64   `json:"available_draws"`
	UsedDraws            int64   `json:"used_draws"`
	FirstDrawGranted     bool    `json:"first_draw_granted"`
	ThresholdEntitlement int64   `json:"threshold_entitlement"`
	ManualAdjustment     int64   `json:"manual_adjustment"`
	BalanceSpent         float64 `json:"balance_spent"`
	NextThreshold        float64 `json:"next_threshold"`
	// CurrentTier 当前阶梯(0 青铜/1 白银/2 黄金/3 钻石/4 王者)。
	CurrentTier int64 `json:"current_tier"`
	// TierName 当前阶梯展示名。
	TierName        string     `json:"tier_name"`
	RulesVersion    int        `json:"rules_version"`
	PointsExpiresAt *time.Time `json:"points_expires_at"`
	ServerTime      time.Time  `json:"server_time"`
}

// LotteryDrawRequest POST /lottery/draw 请求体。
type LotteryDrawRequest struct {
	// IdempotencyKey 客户端生成的幂等键(如 UUID),同一请求重试必须复用。
	IdempotencyKey string `json:"idempotency_key" binding:"required"`
}

// GetActivity GET /api/v1/lottery/activity
// 返回当前活动、奖品列表与概率展示。
func (h *LotteryHandler) GetActivity(c *gin.Context) {
	view, err := h.lotteryService.GetUserActivityView(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"id":            view.Activity.ID,
		"name":          view.Activity.Name,
		"status":        view.Activity.Status,
		"rules_version": view.RulesVersion,
		"starts_at":     view.Activity.StartsAt,
		"ends_at":       view.Activity.EndsAt,
		"is_open":       view.IsOpen,
		"server_time":   view.ServerTime,
		"prizes":        view.Prizes,
		"tiers":         view.Tiers,
	})
}

// GetStatus GET /api/v1/lottery/status
// 返回用户抽奖资格、剩余次数、累计消耗与下一阈值。
func (h *LotteryHandler) GetStatus(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	status, err := h.lotteryService.GetUserStatus(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, LotteryStatusResponse{
		ActivityOpen:         status.ActivityOpen,
		AvailableDraws:       status.AvailableDraws,
		UsedDraws:            status.UsedDraws,
		FirstDrawGranted:     status.FirstDrawGranted,
		ThresholdEntitlement: status.ThresholdEntitlement,
		ManualAdjustment:     status.ManualAdjustment,
		BalanceSpent:         service.BalanceSpentCentsToFloat(status.BalanceSpentCents),
		NextThreshold:        service.BalanceSpentCentsToFloat(status.NextThresholdCents),
		CurrentTier:          status.CurrentTier,
		TierName:             status.TierName,
		RulesVersion:         status.RulesVersion,
		PointsExpiresAt:      status.PointsExpiresAt,
		ServerTime:           status.ServerTime,
	})
}

// Draw POST /api/v1/lottery/draw
// 执行抽奖。结果完全由服务端决定;同一 idempotency_key 重试返回已有结果。
func (h *LotteryHandler) Draw(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req LotteryDrawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	outcome, err := h.lotteryService.Draw(c.Request.Context(), subject.UserID, req.IdempotencyKey)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"draw":            dto.LotteryDrawFromService(outcome.Record),
		"remaining_draws": outcome.RemainingDraws,
	})
}

// GetRecords GET /api/v1/lottery/records
// 分页返回用户抽奖流水。
func (h *LotteryHandler) GetRecords(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	records, result, err := h.lotteryService.ListUserRecords(c.Request.Context(), subject.UserID, params)
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

// GetRewards GET /api/v1/lottery/rewards
// 分页返回用户中奖记录(不含"谢谢参与")。
func (h *LotteryHandler) GetRewards(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	records, result, err := h.lotteryService.ListUserRewards(c.Request.Context(), subject.UserID, params)
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
