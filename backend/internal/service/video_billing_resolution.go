package service

import "strings"

const (
	VideoBillingResolution480P  = "480p"
	VideoBillingResolution768P  = "768p"
	VideoBillingResolution720P  = "720p"
	VideoBillingResolution1080P = "1080p"
	VideoBillingResolution2K    = "2k"
	VideoBillingResolution4K    = "4k"
)

// xAI 视频生成按秒计费，duration 请求参数允许 1-15 秒；未指定时上游默认生成 8 秒。
// 计费时长必须与上游实际消耗对齐，否则用户可通过拉长 duration 套利（提交时长由用户控制）。
const (
	VideoBillingMinDurationSeconds     = 1
	VideoBillingMaxDurationSeconds     = 15
	VideoBillingDefaultDurationSeconds = 8
)

// NormalizeVideoBillingDurationSecondsOrDefault 归一化计费用视频时长：
// 未指定（<=0）按上游默认 8 秒计，超出上游允许区间按边界收敛。
func NormalizeVideoBillingDurationSecondsOrDefault(durationSeconds int) int {
	if durationSeconds <= 0 {
		return VideoBillingDefaultDurationSeconds
	}
	if durationSeconds < VideoBillingMinDurationSeconds {
		return VideoBillingMinDurationSeconds
	}
	if durationSeconds > VideoBillingMaxDurationSeconds {
		return VideoBillingMaxDurationSeconds
	}
	return durationSeconds
}

// NormalizeVideoBillingDurationSecondsForModel preserves the xAI 1-15s contract
// while allowing generic async video providers to report longer generated clips.
func NormalizeVideoBillingDurationSecondsForModel(model string, durationSeconds int) int {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "grok-imagine-video") {
		return NormalizeVideoBillingDurationSecondsOrDefault(durationSeconds)
	}
	if durationSeconds <= 0 {
		return DefaultVideoAPIDurationSeconds
	}
	if durationSeconds > MaxVideoAPIDurationSeconds {
		return MaxVideoAPIDurationSeconds
	}
	return durationSeconds
}

// LookupVideoBillingResolution 归一化分辨率并报告是否为已知档位。
// 配置解析路径必须用它而不是 OrDefault：把无法识别的档位（如 "4k"、拼错的
// "1080i"）静默折算成 480p，会让管理员配的高分辨率单价被挂到低分辨率档上。
func LookupVideoBillingResolution(resolution string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(resolution)) {
	case "480", "480p", "sd":
		return VideoBillingResolution480P, true
	case "720", "720p", "hd":
		return VideoBillingResolution720P, true
	case "768", "768p":
		return VideoBillingResolution768P, true
	case "1080", "1080p", "full_hd", "full-hd", "fhd":
		return VideoBillingResolution1080P, true
	case "2k", "2048", "2048p":
		return VideoBillingResolution2K, true
	case "4k", "2160", "2160p":
		return VideoBillingResolution4K, true
	default:
		return "", false
	}
}

// NormalizeVideoBillingResolutionOrDefault 用于运行时计费：上游回传的分辨率
// 缺失或无法识别时按最低档兜底，保证请求仍可计费。
func NormalizeVideoBillingResolutionOrDefault(resolution string) string {
	if normalized, ok := LookupVideoBillingResolution(resolution); ok {
		return normalized
	}
	return VideoBillingResolution480P
}

func NormalizeVideoBillingResolutionForModel(model, resolution string) string {
	if normalized, ok := LookupVideoBillingResolution(resolution); ok {
		return normalized
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "grok-imagine-video") {
		return VideoBillingResolution480P
	}
	value := strings.ToLower(strings.TrimSpace(resolution))
	if value == "" {
		return VideoBillingResolution720P
	}
	return value
}
