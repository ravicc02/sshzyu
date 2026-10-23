package service

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// Fixed enums bound cardinality. No user, key, session, path or request body is
// retained in observations. Size is a coarse wire-size proxy, not a token count.
type openAISchedulingWorkload uint16
type openAISchedulingWorkloadContextKey struct{}

const (
	openAIWorkloadLatencyLimit   = 8
	openAIWorkloadTTFTMinSamples = 3
)

type openAIWorkloadLatency struct {
	ttft       float64
	samples    uint8
	observedAt time.Time
}

// WithOpenAISchedulingWorkload scopes latency observations to comparable inbound
// work. Call after HTTP request policy has been applied. WebSocket callers bind
// the first frame for selection and each turn's own frame for reporting.
func WithOpenAISchedulingWorkload(ctx context.Context, protocol string, body []byte) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAISchedulingWorkloadContextKey{}, classifyOpenAISchedulingWorkload(protocol, body))
}

func openAISchedulingWorkloadFromContext(ctx context.Context) openAISchedulingWorkload {
	if ctx == nil {
		return 0
	}
	workload, _ := ctx.Value(openAISchedulingWorkloadContextKey{}).(openAISchedulingWorkload)
	return workload
}

func openAISchedulingWorkloadForPlatform(ctx context.Context, platform string) openAISchedulingWorkload {
	if NormalizeOpenAICompatiblePlatform(platform) != PlatformOpenAI {
		return 0
	}
	return openAISchedulingWorkloadFromContext(ctx)
}

func classifyOpenAISchedulingWorkload(protocol string, body []byte) openAISchedulingWorkload {
	var endpoint uint16
	switch protocol {
	case "responses":
		endpoint = 1
	case "chat":
		endpoint = 2
	case "messages":
		endpoint = 3
	case "websocket":
		endpoint = 4
	default:
		return 0
	}
	if !gjson.ValidBytes(body) {
		return 0
	}
	stream := uint16(0)
	if protocol == "websocket" || gjson.GetBytes(body, "stream").Bool() {
		stream = 1
	}
	effort := uint16(0)
	if value := CanonicalRequestedReasoningEffort(body); value != nil {
		switch *value {
		case "minimal":
			effort = 1
		case "low":
			effort = 2
		case "medium":
			effort = 3
		case "high":
			effort = 4
		case "xhigh":
			effort = 5
		case "max":
			effort = 6
		default:
			effort = 7
		}
	} else if explicitRequestedReasoningEffortFromBody(body) != "" || protocol == "websocket" {
		effort = 7
	}
	if strings.EqualFold(gjson.GetBytes(body, "reasoning.mode").String(), "pro") {
		effort |= 8
	}
	size := uint16(1)
	switch {
	case len(body) >= 1024*1024:
		size = 4
	case len(body) >= 128*1024:
		size = 3
	case len(body) >= 16*1024:
		size = 2
	}
	// A short continuation frame can reference a large hidden upstream context.
	// Keep it separate rather than claiming that its true context is small.
	conversation := gjson.GetBytes(body, "conversation")
	if protocol == "websocket" || gjson.GetBytes(body, "previous_response_id").String() != "" || conversation.Type == gjson.String && conversation.String() != "" || conversation.Get("id").String() != "" {
		size = 0
	}
	return openAISchedulingWorkload(endpoint | stream<<3 | effort<<4 | size<<8)
}

// Existing account/model health remains shared across workload classes. Only
// successful, positive TTFT observations enter this bounded latency cache.
func (s *openAIAccountRuntimeStats) reportWorkloadLatency(accountID int64, model string, workload openAISchedulingWorkload, ttft *int, now time.Time) {
	if s == nil || accountID <= 0 || workload == 0 || ttft == nil || *ttft <= 0 {
		return
	}
	account := s.loadOrCreate(accountID)
	account.modelMu.Lock()
	defer account.modelMu.Unlock()
	stat := account.models[strings.TrimSpace(model)]
	if stat == nil || now.Sub(stat.observedAt) >= openAIModelRuntimeTTL {
		return
	}
	if stat.workloads == nil {
		stat.workloads = make(map[openAISchedulingWorkload]openAIWorkloadLatency)
	}
	value, exists := stat.workloads[workload]
	if !exists && len(stat.workloads) >= openAIWorkloadLatencyLimit {
		var oldestKey openAISchedulingWorkload
		var oldest time.Time
		for key, observation := range stat.workloads {
			if oldest.IsZero() || observation.observedAt.Before(oldest) {
				oldestKey, oldest = key, observation.observedAt
			}
		}
		delete(stat.workloads, oldestKey)
	}
	sample := float64(*ttft)
	if value.samples == 0 || now.Sub(value.observedAt) >= openAIModelRuntimeTTL {
		value = openAIWorkloadLatency{ttft: sample}
	}
	if value.samples > 0 {
		if sample < value.ttft/2 {
			value.ttft = sample
		} else {
			value.ttft = .2*sample + .8*value.ttft
		}
	}
	if value.samples < openAIWorkloadTTFTMinSamples {
		value.samples++
	}
	value.observedAt = now
	stat.workloads[workload] = value
}

func (s *openAIAccountRuntimeStats) snapshotWorkload(accountID int64, model string, workload openAISchedulingWorkload, now time.Time) (float64, float64, bool) {
	if s == nil {
		return 0, 0, false
	}
	raw, ok := s.accounts.Load(accountID)
	if !ok {
		return 0, 0, false
	}
	account := raw.(*openAIAccountRuntimeStat)
	account.modelMu.Lock()
	defer account.modelMu.Unlock()
	stat := account.models[strings.TrimSpace(model)]
	if stat == nil || now.Sub(stat.observedAt) >= openAIModelRuntimeTTL {
		return 0, 0, false
	}
	errorRate, _, _ := stat.stats.snapshot(accountID)
	value, ok := stat.workloads[workload]
	if !ok || value.samples < openAIWorkloadTTFTMinSamples || now.Sub(value.observedAt) >= openAIModelRuntimeTTL || value.ttft <= 0 || math.IsNaN(value.ttft) || math.IsInf(value.ttft, 0) {
		return errorRate, 0, false
	}
	return errorRate, value.ttft, true
}
