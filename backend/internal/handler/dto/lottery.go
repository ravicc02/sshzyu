// Package dto 抽奖(Lottery)HTTP DTO 与转换函数。
package dto

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// LotteryDrawUserRef 管理端中奖记录附带的中奖人标识。
type LotteryDrawUserRef struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Deleted  bool   `json:"deleted"`
}

// LotteryDraw 抽奖流水 DTO(用户端与管理端共用;不含内部敏感字段)。
type LotteryDraw struct {
	ID                 int64               `json:"id"`
	UserID             int64               `json:"user_id"`
	ActivityID         int64               `json:"activity_id"`
	PrizeID            *int64              `json:"prize_id"`
	PrizeName          string              `json:"prize_name"`
	PrizeType          string              `json:"prize_type"`
	PrizeValue         float64             `json:"prize_value"`
	RulesVersion       int                 `json:"rules_version"`
	Source             string              `json:"source"`
	BalanceSpentAtDraw float64             `json:"balance_spent_at_draw"`
	FulfillmentStatus  string              `json:"fulfillment_status"`
	FulfilledAt        *time.Time          `json:"fulfilled_at"`
	FulfillmentError   *string             `json:"fulfillment_error,omitempty"`
	CreatedAt          time.Time           `json:"created_at"`
	User               *LotteryDrawUserRef `json:"user,omitempty"`
}

// LotteryDrawFromService 领域流水转 DTO。
func LotteryDrawFromService(record *service.LotteryDrawRecord) *LotteryDraw {
	if record == nil {
		return nil
	}
	return &LotteryDraw{
		ID:                 record.ID,
		UserID:             record.UserID,
		ActivityID:         record.ActivityID,
		PrizeID:            record.PrizeID,
		PrizeName:          record.PrizeName,
		PrizeType:          record.PrizeType,
		PrizeValue:         record.PrizeValue,
		RulesVersion:       record.RulesVersion,
		Source:             record.Source,
		BalanceSpentAtDraw: record.BalanceSpentAtDraw,
		FulfillmentStatus:  record.FulfillmentStatus,
		FulfilledAt:        record.FulfilledAt,
		FulfillmentError:   record.FulfillmentError,
		CreatedAt:          record.CreatedAt,
		User:               drawUserRefFromService(record.User),
	}
}

// drawUserRefFromService 服务层中奖人摘要转 DTO(nil 安全)。
func drawUserRefFromService(summary *service.DrawUserSummary) *LotteryDrawUserRef {
	if summary == nil {
		return nil
	}
	return &LotteryDrawUserRef{
		ID:       summary.ID,
		Email:    summary.Email,
		Username: summary.Username,
		Deleted:  summary.Deleted,
	}
}

// LotteryAdminActivity 管理端活动 DTO(snake_case,与前端类型对齐)。
type LotteryAdminActivity struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	RulesVersion   int        `json:"rules_version"`
	TierMode       string     `json:"tier_mode"`
	TierThresholds []int64    `json:"tier_thresholds"`
	StartsAt       *time.Time `json:"starts_at"`
	EndsAt         *time.Time `json:"ends_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// LotteryAdminActivityFromService 服务层活动转管理端 DTO。
func LotteryAdminActivityFromService(a *service.LotteryActivity) *LotteryAdminActivity {
	if a == nil {
		return nil
	}
	return &LotteryAdminActivity{
		ID:             a.ID,
		Name:           a.Name,
		Status:         a.Status,
		RulesVersion:   a.RulesVersion,
		TierMode:       a.TierConfig().Mode,
		TierThresholds: append([]int64(nil), a.TierConfig().Thresholds...),
		StartsAt:       a.StartsAt,
		EndsAt:         a.EndsAt,
		CreatedAt:      a.CreatedAt,
		UpdatedAt:      a.UpdatedAt,
	}
}

// LotteryAdminPrize 管理端奖品原始配置 DTO(未做概率换算,供展示与编辑)。
type LotteryAdminPrize struct {
	ID          int64           `json:"id"`
	ActivityID  int64           `json:"activity_id"`
	Name        string          `json:"name"`
	PrizeType   string          `json:"prize_type"`
	Value       float64         `json:"value"`
	Weight      int             `json:"weight"`
	MinTier     int             `json:"min_tier"`
	TierWeights map[string]int  `json:"tier_weights"`
	Stock       int             `json:"stock"`
	StockIssued int             `json:"stock_issued"`
	Enabled     bool            `json:"enabled"`
	SortOrder   int             `json:"sort_order"`
}

// LotteryAdminPrizeFromService 服务层奖品转管理端 DTO。
func LotteryAdminPrizeFromService(p *service.LotteryPrize) *LotteryAdminPrize {
	if p == nil {
		return nil
	}
	weights := p.TierWeights
	if weights == nil {
		weights = map[string]int{}
	}
	return &LotteryAdminPrize{
		ID:          p.ID,
		ActivityID:  p.ActivityID,
		Name:        p.Name,
		PrizeType:   p.PrizeType,
		Value:       p.Value,
		Weight:      p.Weight,
		MinTier:     p.MinTier,
		TierWeights: weights,
		Stock:       p.Stock,
		StockIssued: p.StockIssued,
		Enabled:     p.Enabled,
		SortOrder:   p.SortOrder,
	}
}
