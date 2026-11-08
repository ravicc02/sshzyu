package service

import (
	"time"
)

// SubscriptionCacheData represents cached subscription data
type SubscriptionCacheData struct {
	Status               string
	ExpiresAt            time.Time
	DailyUsage           float64
	WeeklyUsage          float64
	MonthlyUsage         float64
	DailyUsageTokens     int64
	WeeklyUsageTokens    int64
	MonthlyUsageTokens   int64
	DailyUsageRequests   int64
	WeeklyUsageRequests  int64
	MonthlyUsageRequests int64
	Version              int64
}
