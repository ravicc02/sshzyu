package handler

import (
	"context"
	"strings"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"go.uber.org/zap"
)

func prepareVideoAPICompletionBilling(
	ctx context.Context,
	h *OpenAIGatewayHandler,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	taskRequestID string,
	statusResult *service.OpenAIForwardResult,
) *service.OpenAIForwardResult {
	if h == nil || h.gatewayService == nil || apiKey == nil || statusResult == nil || statusResult.VideoCount <= 0 {
		return nil
	}
	taskRequestID = strings.TrimSpace(firstNonEmptyString(taskRequestID, statusResult.ResponseID))
	if taskRequestID == "" {
		return nil
	}
	pending, err := h.gatewayService.LoadGrokVideoPendingBilling(ctx, taskRequestID, subject.UserID, apiKey.ID)
	if err != nil {
		reqLog.Warn("video_api.pending_billing_load_failed", zap.String("request_id", taskRequestID), zap.Error(err))
	}
	model := firstNonEmptyString(statusResult.BillingModel, statusResult.Model)
	if pending != nil {
		model = firstNonEmptyString(model, pending.BillingModel, pending.Model, pending.OriginalModel)
	}
	if model == "" {
		reqLog.Error("video_api.billing_skipped_missing_model", zap.String("request_id", taskRequestID))
		return nil
	}
	mode := h.gatewayService.VideoAPIBillingMode(ctx, apiKey, model)
	if mode == service.BillingModeToken && statusResult.Usage.OutputTokens <= 0 {
		reqLog.Error("video_api.token_billing_skipped_missing_usage",
			zap.String("request_id", taskRequestID),
			zap.String("model", model),
		)
		return nil
	}
	claimed, err := h.gatewayService.ClaimGrokVideoBilling(ctx, taskRequestID, subject.UserID, apiKey.ID)
	if err != nil {
		reqLog.Warn("video_api.billing_claim_failed", zap.String("request_id", taskRequestID), zap.Error(err))
		return nil
	}
	if !claimed {
		return nil
	}

	merged := *statusResult
	merged.Model = model
	merged.BillingModel = model
	if pending != nil {
		if strings.TrimSpace(merged.UpstreamModel) == "" {
			merged.UpstreamModel = pending.UpstreamModel
		}
		if strings.TrimSpace(merged.VideoResolution) == "" {
			merged.VideoResolution = pending.VideoResolution
		}
		if merged.VideoDurationSeconds <= 0 {
			merged.VideoDurationSeconds = pending.VideoDurationSeconds
		}
		if e2e := service.GrokVideoE2EDuration(pending.CreatedAt, time.Now()); e2e > 0 {
			merged.Duration = e2e
		}
	}
	merged.VideoResolution = service.NormalizeVideoAPIResolutionOrDefault(merged.VideoResolution)
	if merged.VideoDurationSeconds <= 0 {
		merged.VideoDurationSeconds = service.DefaultVideoAPIDurationSeconds
	}
	merged.RequestID = service.StableVideoAPIBillingRequestID(firstNonEmptyString(merged.ResponseID, taskRequestID))
	merged.ResponseID = firstNonEmptyString(merged.ResponseID, taskRequestID)
	merged.VideoCount = 1
	merged.ImageCount = 0
	return &merged
}
