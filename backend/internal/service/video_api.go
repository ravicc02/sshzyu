package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	DefaultVideoAPICreatePath  = "/v1/videos"
	DefaultVideoAPIStatusPath  = "/v1/videos/{id}"
	DefaultVideoAPIContentPath = "/v1/videos/{id}/content"

	DefaultVideoAPIDurationSeconds = 5
	MaxVideoAPIDurationSeconds     = 300
)

var (
	defaultVideoAPITaskIDPaths = []string{
		"id", "request_id", "task_id", "video.id", "video.request_id", "video.task_id",
		"data.id", "data.request_id", "data.task_id", "data.video.id", "data.video.request_id", "data.video.task_id",
	}
	defaultVideoAPIStatusPaths = []string{
		"status", "state", "video.status", "data.status", "data.state", "data.video.status",
	}
	defaultVideoAPICompletedValues = []string{"completed", "complete", "succeeded", "success", "done"}
	defaultVideoAPIFailedValues    = []string{"failed", "failure", "error", "cancelled", "canceled", "expired"}
	defaultVideoAPIContentURLPaths = []string{
		"video.url", "url", "video_url", "output_url", "download_url",
		"data.video.url", "data.url", "data.video_url", "data.output_url", "data.download_url",
	}
	defaultVideoAPITokenPaths = []string{
		"usage.total_tokens", "usage.output_tokens", "total_tokens", "output_tokens",
		"data.usage.total_tokens", "data.usage.output_tokens", "data.total_tokens", "data.output_tokens",
	}
	defaultVideoAPIDurationPaths = []string{
		"video.duration", "duration", "seconds",
		"data.video.duration", "data.duration", "data.seconds",
	}
	defaultVideoAPIResolutionPaths = []string{
		"resolution", "video.resolution", "data.resolution", "data.video.resolution",
	}
)

// VideoAPIAccountConfig describes the OpenAI-compatible async video protocol.
// All fields are optional; defaults keep standard /v1/videos providers working
// without provider-specific code.
type VideoAPIAccountConfig struct {
	CreatePath             string   `json:"create_path"`
	StatusPath             string   `json:"status_path"`
	ContentPath            string   `json:"content_path"`
	AuthHeader             string   `json:"auth_header"`
	AuthPrefix             string   `json:"auth_prefix"`
	APIKeyQueryParam       string   `json:"api_key_query_param"`
	DefaultResolution      string   `json:"default_resolution"`
	DefaultDurationSeconds int      `json:"default_duration_seconds"`
	TaskIDPaths            []string `json:"task_id_paths"`
	StatusPaths            []string `json:"status_paths"`
	ProgressPaths          []string `json:"progress_paths"`
	CompletedValues        []string `json:"completed_values"`
	FailedValues           []string `json:"failed_values"`
	ContentURLPaths        []string `json:"content_url_paths"`
	TokenUsagePaths        []string `json:"token_usage_paths"`
	DurationSecondsPaths   []string `json:"duration_seconds_paths"`
	ResolutionPaths        []string `json:"resolution_paths"`
}

func DefaultVideoAPIAccountConfig() VideoAPIAccountConfig {
	return VideoAPIAccountConfig{
		CreatePath:             DefaultVideoAPICreatePath,
		StatusPath:             DefaultVideoAPIStatusPath,
		ContentPath:            DefaultVideoAPIContentPath,
		AuthHeader:             "Authorization",
		AuthPrefix:             "Bearer ",
		DefaultResolution:      VideoBillingResolution720P,
		DefaultDurationSeconds: DefaultVideoAPIDurationSeconds,
		TaskIDPaths:            append([]string(nil), defaultVideoAPITaskIDPaths...),
		StatusPaths:            append([]string(nil), defaultVideoAPIStatusPaths...),
		ProgressPaths:          []string{"progress", "data.progress"},
		CompletedValues:        append([]string(nil), defaultVideoAPICompletedValues...),
		FailedValues:           append([]string(nil), defaultVideoAPIFailedValues...),
		ContentURLPaths:        append([]string(nil), defaultVideoAPIContentURLPaths...),
		TokenUsagePaths:        append([]string(nil), defaultVideoAPITokenPaths...),
		DurationSecondsPaths:   append([]string(nil), defaultVideoAPIDurationPaths...),
		ResolutionPaths:        append([]string(nil), defaultVideoAPIResolutionPaths...),
	}
}

func (a *Account) GetVideoAPIAccountConfig() VideoAPIAccountConfig {
	if a == nil || a.Credentials == nil {
		return DefaultVideoAPIAccountConfig()
	}
	return NormalizeVideoAPIAccountConfig(a.Credentials["video_api_config"])
}

func NormalizeVideoAPIAccountConfig(raw any) VideoAPIAccountConfig {
	cfg := DefaultVideoAPIAccountConfig()
	if raw == nil {
		return cfg
	}
	if text, ok := raw.(string); ok {
		text = strings.TrimSpace(text)
		if text == "" {
			return cfg
		}
		var input VideoAPIAccountConfig
		if err := json.Unmarshal([]byte(text), &input); err != nil {
			return cfg
		}
		raw = input
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return cfg
	}
	var input VideoAPIAccountConfig
	if err := json.Unmarshal(data, &input); err != nil {
		return cfg
	}
	if value := strings.TrimSpace(input.CreatePath); value != "" {
		cfg.CreatePath = value
	}
	if value := strings.TrimSpace(input.StatusPath); value != "" {
		cfg.StatusPath = value
	}
	if value := strings.TrimSpace(input.ContentPath); value != "" {
		cfg.ContentPath = value
	}
	if value := strings.TrimSpace(input.AuthHeader); value != "" {
		cfg.AuthHeader = value
	}
	if input.AuthPrefix != "" {
		cfg.AuthPrefix = input.AuthPrefix
	}
	cfg.APIKeyQueryParam = strings.TrimSpace(input.APIKeyQueryParam)
	if value := normalizeVideoAPIResolution(input.DefaultResolution); value != "" {
		cfg.DefaultResolution = value
	}
	if input.DefaultDurationSeconds > 0 {
		cfg.DefaultDurationSeconds = normalizeVideoAPIDurationSeconds(input.DefaultDurationSeconds)
	}
	cfg.TaskIDPaths = normalizeVideoAPIPathList(input.TaskIDPaths, cfg.TaskIDPaths)
	cfg.StatusPaths = normalizeVideoAPIPathList(input.StatusPaths, cfg.StatusPaths)
	cfg.ProgressPaths = normalizeVideoAPIPathList(input.ProgressPaths, cfg.ProgressPaths)
	cfg.CompletedValues = normalizeVideoAPIValueList(input.CompletedValues, cfg.CompletedValues)
	cfg.FailedValues = normalizeVideoAPIValueList(input.FailedValues, cfg.FailedValues)
	cfg.ContentURLPaths = normalizeVideoAPIPathList(input.ContentURLPaths, cfg.ContentURLPaths)
	cfg.TokenUsagePaths = normalizeVideoAPIPathList(input.TokenUsagePaths, cfg.TokenUsagePaths)
	cfg.DurationSecondsPaths = normalizeVideoAPIPathList(input.DurationSecondsPaths, cfg.DurationSecondsPaths)
	cfg.ResolutionPaths = normalizeVideoAPIPathList(input.ResolutionPaths, cfg.ResolutionPaths)
	return cfg
}

func ValidateVideoAPIAccountConfig(raw any) error {
	if raw == nil {
		return nil
	}
	var input VideoAPIAccountConfig
	if text, ok := raw.(string); ok {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil
		}
		if err := json.Unmarshal([]byte(text), &input); err != nil {
			return fmt.Errorf("video API config must be a JSON object: %w", err)
		}
	} else {
		data, err := json.Marshal(raw)
		if err != nil {
			return fmt.Errorf("encode video API config: %w", err)
		}
		if err := json.Unmarshal(data, &input); err != nil {
			return fmt.Errorf("video API config must be a JSON object: %w", err)
		}
	}
	for name, value := range map[string]string{
		"create_path":  input.CreatePath,
		"status_path":  input.StatusPath,
		"content_path": input.ContentPath,
	} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.Contains(value, "://") || !strings.HasPrefix(value, "/") {
			return fmt.Errorf("video API %s must be a relative path beginning with /", name)
		}
	}
	if input.AuthHeader != "" && strings.ContainsAny(input.AuthHeader, "\r\n:") {
		return fmt.Errorf("video API auth_header is invalid")
	}
	if input.APIKeyQueryParam != "" && strings.ContainsAny(input.APIKeyQueryParam, "&#") {
		return fmt.Errorf("video API api_key_query_param is invalid")
	}
	return nil
}

func normalizeVideoAPIValueList(input, fallback []string) []string {
	if len(input) == 0 {
		return append([]string(nil), fallback...)
	}
	out := make([]string, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, value := range input {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return append([]string(nil), fallback...)
	}
	return out
}

func normalizeVideoAPIPathList(input, fallback []string) []string {
	if len(input) == 0 {
		return append([]string(nil), fallback...)
	}
	out := make([]string, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, value := range input {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return append([]string(nil), fallback...)
	}
	return out
}

type VideoAPIRequestInfo struct {
	Model             string
	Prompt            string
	Resolution        string
	DurationSeconds   int
	HasReferenceVideo bool
}

func ParseVideoAPIRequest(body []byte) (VideoAPIRequestInfo, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return VideoAPIRequestInfo{}, fmt.Errorf("request body must be valid JSON")
	}
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if model == "" {
		return VideoAPIRequestInfo{}, fmt.Errorf("model is required")
	}
	prompt := strings.TrimSpace(gjson.GetBytes(body, "prompt").String())
	if prompt == "" {
		return VideoAPIRequestInfo{}, fmt.Errorf("prompt is required")
	}
	duration, err := videoAPIRequestDurationSeconds(body)
	if err != nil {
		return VideoAPIRequestInfo{}, err
	}
	return VideoAPIRequestInfo{
		Model:             model,
		Prompt:            prompt,
		Resolution:        normalizeVideoAPIResolution(gjson.GetBytes(body, "resolution").String()),
		DurationSeconds:   duration,
		HasReferenceVideo: videoAPIRequestHasReferenceVideo(body),
	}, nil
}

func (r VideoAPIRequestInfo) GrokMediaRequestInfo() GrokMediaRequestInfo {
	return GrokMediaRequestInfo{
		Model:           r.Model,
		Prompt:          r.Prompt,
		Resolution:      r.Resolution,
		DurationSeconds: r.DurationSeconds,
		N:               1,
	}
}

func (r VideoAPIRequestInfo) ModerationBody() []byte {
	payload := map[string]string{}
	if prompt := strings.TrimSpace(r.Prompt); prompt != "" {
		payload["prompt"] = prompt
	}
	if len(payload) == 0 {
		return nil
	}
	out, _ := json.Marshal(payload)
	return out
}

func videoAPIRequestDurationSeconds(body []byte) (int, error) {
	seconds := int64(0)
	duration := int64(0)
	for _, path := range []string{"seconds", "duration"} {
		value := gjson.GetBytes(body, path)
		if !value.Exists() {
			continue
		}
		parsed, ok := videoAPIResultInt(value)
		if !ok {
			return 0, fmt.Errorf("%s must be a positive integer", path)
		}
		if path == "seconds" {
			seconds = parsed
		} else {
			duration = parsed
		}
	}
	if seconds > 0 && duration > 0 && seconds != duration {
		return 0, fmt.Errorf("seconds and duration must match when both are provided")
	}
	value := seconds
	if value <= 0 {
		value = duration
	}
	if value <= 0 {
		return DefaultVideoAPIDurationSeconds, nil
	}
	return normalizeVideoAPIDurationSeconds(int(value)), nil
}

func videoAPIRequestHasReferenceVideo(body []byte) bool {
	for _, path := range []string{
		"video", "video_url", "video_urls", "reference_video", "reference_videos", "references.video",
	} {
		value := gjson.GetBytes(body, path)
		if !value.Exists() {
			continue
		}
		if value.IsArray() {
			return len(value.Array()) > 0
		}
		if value.Type == gjson.String {
			return strings.TrimSpace(value.String()) != ""
		}
		return true
	}
	return false
}

func normalizeVideoAPIDurationSeconds(value int) int {
	if value <= 0 {
		return DefaultVideoAPIDurationSeconds
	}
	if value > MaxVideoAPIDurationSeconds {
		return MaxVideoAPIDurationSeconds
	}
	return value
}

func normalizeVideoAPIResolution(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "480", "sd":
		return VideoBillingResolution480P
	case "720", "hd":
		return VideoBillingResolution720P
	case "768":
		return VideoBillingResolution768P
	case "1080", "full_hd", "full-hd", "fhd":
		return VideoBillingResolution1080P
	case "2k", "2048p":
		return VideoBillingResolution2K
	case "4k", "2160p":
		return VideoBillingResolution4K
	default:
		return value
	}
}

func NormalizeVideoAPIResolutionOrDefault(value string) string {
	value = normalizeVideoAPIResolution(value)
	if value == "" {
		return VideoBillingResolution720P
	}
	return value
}

func (cfg VideoAPIAccountConfig) endpointPath(endpoint GrokMediaEndpoint) string {
	switch endpoint {
	case VideoAPIEndpointCreate:
		return cfg.CreatePath
	case VideoAPIEndpointStatus:
		return cfg.StatusPath
	case VideoAPIEndpointContent:
		return cfg.ContentPath
	default:
		return ""
	}
}

func buildVideoAPIURL(account *Account, cfg VideoAPIAccountConfig, endpoint GrokMediaEndpoint, requestID string) (string, error) {
	if account == nil {
		return "", fmt.Errorf("video API account is required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(account.GetOpenAIBaseURL()), "/")
	parsedBase, err := url.Parse(baseURL)
	if err != nil || parsedBase.Scheme == "" || parsedBase.Host == "" || parsedBase.User != nil ||
		parsedBase.RawQuery != "" || parsedBase.Fragment != "" {
		return "", fmt.Errorf("video API base URL is invalid")
	}
	if !strings.EqualFold(parsedBase.Scheme, "http") && !strings.EqualFold(parsedBase.Scheme, "https") {
		return "", fmt.Errorf("video API base URL must use http or https")
	}
	endpointPath := strings.TrimSpace(cfg.endpointPath(endpoint))
	if endpointPath == "" || strings.Contains(endpointPath, "://") {
		return "", fmt.Errorf("video API endpoint path is invalid")
	}
	requestID = strings.Trim(strings.TrimSpace(requestID), "/")
	endpointPath = strings.ReplaceAll(endpointPath, "{id}", url.PathEscape(requestID))
	endpointPath = strings.ReplaceAll(endpointPath, ":id", url.PathEscape(requestID))
	if strings.HasSuffix(parsedBase.Path, "/v1") && strings.HasPrefix(endpointPath, "/v1/") {
		endpointPath = strings.TrimPrefix(endpointPath, "/v1")
	}
	target := baseURL + "/" + strings.TrimLeft(endpointPath, "/")
	parsedTarget, err := url.Parse(target)
	if err != nil || parsedTarget.Scheme == "" || parsedTarget.Host == "" || parsedTarget.User != nil {
		return "", fmt.Errorf("video API endpoint URL is invalid")
	}
	return parsedTarget.String(), nil
}

type VideoAPIStatusMetadata struct {
	Status          string
	Progress        int
	Completed       bool
	Failed          bool
	Model           string
	TotalTokens     int
	Resolution      string
	DurationSeconds int
	ContentURL      string
}

func extractVideoAPIStatusMetadata(body []byte, cfg VideoAPIAccountConfig, pending *GrokVideoPendingBilling, requestID string) VideoAPIStatusMetadata {
	meta := VideoAPIStatusMetadata{}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return meta
	}
	meta.Status = strings.ToLower(strings.TrimSpace(firstVideoAPIString(body, cfg.StatusPaths)))
	if progress, ok := firstVideoAPIInt(body, cfg.ProgressPaths); ok {
		meta.Progress = progress
	}
	meta.Completed = videoAPIValueIn(meta.Status, cfg.CompletedValues)
	meta.Failed = videoAPIValueIn(meta.Status, cfg.FailedValues)
	meta.Model = strings.TrimSpace(firstVideoAPIString(body, []string{"model", "data.model"}))
	meta.ContentURL = strings.TrimSpace(firstVideoAPIString(body, cfg.ContentURLPaths))
	if meta.ContentURL != "" {
		meta.Completed = true
	}
	if tokens, ok := firstVideoAPIInt(body, cfg.TokenUsagePaths); ok {
		meta.TotalTokens = tokens
	}
	if usage, ok := extractOpenAIUsageFromJSONBytes(body); ok {
		if meta.TotalTokens <= 0 {
			meta.TotalTokens = usage.OutputTokens
		}
		if meta.TotalTokens <= 0 {
			meta.TotalTokens = usage.InputTokens + usage.OutputTokens
		}
	}
	meta.Resolution = normalizeVideoAPIResolution(firstVideoAPIString(body, cfg.ResolutionPaths))
	if duration, ok := firstVideoAPIInt(body, cfg.DurationSecondsPaths); ok {
		meta.DurationSeconds = normalizeVideoAPIDurationSeconds(duration)
	}
	if pending != nil {
		if meta.Model == "" {
			meta.Model = firstNonEmpty(pending.BillingModel, pending.Model, pending.OriginalModel)
		}
		if meta.Resolution == "" {
			meta.Resolution = pending.VideoResolution
		}
		if meta.DurationSeconds <= 0 {
			meta.DurationSeconds = pending.VideoDurationSeconds
		}
		if meta.Resolution == "" {
			meta.Resolution = cfg.DefaultResolution
		}
		if meta.DurationSeconds <= 0 {
			meta.DurationSeconds = cfg.DefaultDurationSeconds
		}
	}
	if strings.TrimSpace(meta.Model) == "" {
		meta.Model = strings.TrimSpace(requestID)
	}
	return meta
}

func firstVideoAPIString(body []byte, paths []string) string {
	for _, path := range paths {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value
		}
	}
	return ""
}

func firstVideoAPIInt(body []byte, paths []string) (int, bool) {
	for _, path := range paths {
		value := gjson.GetBytes(body, path)
		if !value.Exists() {
			continue
		}
		if parsed, ok := videoAPIResultInt(value); ok {
			return int(parsed), true
		}
	}
	return 0, false
}

func videoAPIResultInt(value gjson.Result) (int64, bool) {
	switch value.Type {
	case gjson.Number:
		return value.Int(), true
	case gjson.String:
		parsed, err := strconv.ParseInt(strings.TrimSpace(value.String()), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func videoAPIValueIn(value string, values []string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	for _, item := range values {
		if value == strings.ToLower(strings.TrimSpace(item)) {
			return true
		}
	}
	return false
}

func ExtractVideoAPICompletion(body []byte, cfg VideoAPIAccountConfig, pending *GrokVideoPendingBilling, requestID string) *OpenAIForwardResult {
	meta := extractVideoAPIStatusMetadata(body, cfg, pending, requestID)
	if !meta.Completed {
		return nil
	}
	return &OpenAIForwardResult{
		ResponseID:           strings.TrimSpace(requestID),
		Model:                meta.Model,
		BillingModel:         meta.Model,
		Usage:                OpenAIUsage{OutputTokens: meta.TotalTokens},
		VideoCount:           1,
		VideoResolution:      meta.Resolution,
		VideoDurationSeconds: meta.DurationSeconds,
	}
}

func StableVideoAPIBillingRequestID(taskRequestID string) string {
	taskRequestID = strings.TrimSpace(taskRequestID)
	if taskRequestID == "" {
		return ""
	}
	if strings.HasPrefix(taskRequestID, "video-api:") {
		return taskRequestID
	}
	return "video-api:" + taskRequestID
}

func rewriteVideoAPIContentURLs(body []byte, cfg VideoAPIAccountConfig, requestID, proxyURL string) []byte {
	if len(body) == 0 || !gjson.ValidBytes(body) || strings.TrimSpace(requestID) == "" || strings.TrimSpace(proxyURL) == "" {
		return body
	}
	out := body
	for _, path := range cfg.ContentURLPaths {
		value := gjson.GetBytes(out, path)
		if !value.Exists() || value.Type != gjson.String || strings.TrimSpace(value.String()) == "" {
			continue
		}
		rewritten, err := sjson.SetBytes(out, path, proxyURL)
		if err == nil {
			out = rewritten
		}
	}
	return out
}

func (s *OpenAIGatewayService) ForwardVideoAPI(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint GrokMediaEndpoint,
	requestID string,
	body []byte,
	contentType string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	if account == nil || !account.IsVideoAPI() {
		return nil, fmt.Errorf("video API account is required")
	}
	token, _, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, err
	}
	cfg := account.GetVideoAPIAccountConfig()
	if endpoint == VideoAPIEndpointContent {
		return s.forwardVideoAPIContent(ctx, c, account, cfg, token, requestID, body, startTime)
	}
	targetURL, err := buildVideoAPIURL(account, cfg, endpoint, requestID)
	if err != nil {
		return nil, err
	}
	requestInfo := VideoAPIRequestInfo{}
	if endpoint == VideoAPIEndpointCreate {
		requestInfo, err = ParseVideoAPIRequest(body)
		if err != nil {
			return nil, err
		}
		if upstreamModel := strings.TrimSpace(account.GetMappedModel(requestInfo.Model)); upstreamModel != "" && upstreamModel != requestInfo.Model {
			body, err = sjson.SetBytes(body, "model", upstreamModel)
			if err != nil {
				return nil, fmt.Errorf("rewrite video API mapped model: %w", err)
			}
		}
	}
	var bodyReader *bytes.Reader
	if endpoint.RequiresRequestBody() {
		bodyReader = bytes.NewReader(body)
	}
	var reader io.Reader = http.NoBody
	if bodyReader != nil {
		reader = bodyReader
	}
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	req, err := http.NewRequestWithContext(upstreamCtx, endpoint.httpMethod(), targetURL, reader)
	if err != nil {
		return nil, err
	}
	applyVideoAPIAuth(req, cfg, token)
	req.Header.Set("Accept", "application/json")
	if endpoint.RequiresRequestBody() {
		contentType = strings.TrimSpace(contentType)
		if contentType == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
	}
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return s.handleErrorResponse(ctx, resp, c, account, body, requestInfo.Model)
	}
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if endpoint == VideoAPIEndpointStatus {
		respBody = rewriteVideoAPIContentURLs(respBody, cfg, requestID, grokMediaContentProxyURL(c, requestID))
	}
	writeGrokMediaResponse(c, resp, respBody, s.responseHeaderFilter)

	result := &OpenAIForwardResult{
		RequestID:       firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("request-id")),
		UpstreamHeaders: resp.Header,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(startTime),
		Model:           requestInfo.Model,
		BillingModel:    requestInfo.Model,
		UpstreamModel:   strings.TrimSpace(account.GetMappedModel(requestInfo.Model)),
	}
	if result.UpstreamModel == "" {
		result.UpstreamModel = requestInfo.Model
	}
	if endpoint == VideoAPIEndpointCreate {
		result.ResponseID = firstVideoAPIString(respBody, cfg.TaskIDPaths)
		result.VideoResolution = requestInfo.Resolution
		if result.VideoResolution == "" {
			result.VideoResolution = cfg.DefaultResolution
		}
		result.VideoDurationSeconds = requestInfo.DurationSeconds
	} else if endpoint == VideoAPIEndpointStatus {
		meta := extractVideoAPIStatusMetadata(respBody, cfg, nil, requestID)
		result.ResponseID = firstNonEmpty(requestID, firstVideoAPIString(respBody, cfg.TaskIDPaths))
		if meta.Model != "" {
			result.Model = meta.Model
			result.BillingModel = meta.Model
		}
		if meta.Completed {
			result.VideoCount = 1
			result.Usage.OutputTokens = meta.TotalTokens
			result.VideoResolution = meta.Resolution
			result.VideoDurationSeconds = meta.DurationSeconds
		}
	}
	return result, nil
}

func applyVideoAPIAuth(req *http.Request, cfg VideoAPIAccountConfig, token string) {
	if req == nil {
		return
	}
	if strings.TrimSpace(cfg.APIKeyQueryParam) != "" {
		values := req.URL.Query()
		values.Set(cfg.APIKeyQueryParam, token)
		req.URL.RawQuery = values.Encode()
	}
	header := strings.TrimSpace(cfg.AuthHeader)
	if header == "" || strings.EqualFold(header, "none") {
		return
	}
	prefix := cfg.AuthPrefix
	if prefix == "" {
		prefix = "Bearer "
	}
	req.Header.Set(header, prefix+token)
}

func (s *OpenAIGatewayService) forwardVideoAPIContent(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	cfg VideoAPIAccountConfig,
	token, requestID string,
	body []byte,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	statusURL, err := buildVideoAPIURL(account, cfg, VideoAPIEndpointStatus, requestID)
	if err != nil {
		return nil, err
	}
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	statusReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodGet, statusURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	applyVideoAPIAuth(statusReq, cfg, token)
	statusReq.Header.Set("Accept", "application/json")
	account.ApplyHeaderOverrides(statusReq.Header)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	statusResp, err := s.httpUpstream.Do(statusReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = statusResp.Body.Close() }()
	if statusResp.StatusCode >= 400 {
		return s.handleErrorResponse(ctx, statusResp, c, account, body)
	}
	statusBody, err := ReadUpstreamResponseBody(statusResp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	meta := extractVideoAPIStatusMetadata(statusBody, cfg, nil, requestID)
	contentURL := strings.TrimSpace(meta.ContentURL)
	signedContent := contentURL != "" && !sameURLOrigin(contentURL, account.GetOpenAIBaseURL())
	if contentURL == "" {
		contentURL, err = buildVideoAPIURL(account, cfg, VideoAPIEndpointContent, requestID)
		if err != nil {
			return nil, err
		}
	}
	contentReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodGet, contentURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	contentReq.Header.Set("Accept", "*/*")
	if rangeHeader := strings.TrimSpace(c.GetHeader("Range")); rangeHeader != "" {
		contentReq.Header.Set("Range", rangeHeader)
	}
	if !signedContent {
		applyVideoAPIAuth(contentReq, cfg, token)
		account.ApplyHeaderOverrides(contentReq.Header)
	}
	contentResp, err := s.httpUpstream.Do(contentReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = contentResp.Body.Close() }()
	if contentResp.StatusCode >= 400 && contentResp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		return s.handleErrorResponse(ctx, contentResp, c, account, body)
	}
	if err := writeGrokMediaContentResponse(c, contentResp); err != nil {
		return nil, err
	}
	result := &OpenAIForwardResult{
		ResponseID:      strings.TrimSpace(requestID),
		UpstreamHeaders: contentResp.Header,
		ResponseHeaders: contentResp.Header.Clone(),
		Duration:        time.Since(startTime),
	}
	if meta.Completed {
		result.Model = meta.Model
		result.BillingModel = meta.Model
		result.VideoCount = 1
		result.Usage.OutputTokens = meta.TotalTokens
		result.VideoResolution = meta.Resolution
		result.VideoDurationSeconds = meta.DurationSeconds
	}
	return result, nil
}

func sameURLOrigin(rawA, rawB string) bool {
	a, errA := url.Parse(strings.TrimSpace(rawA))
	b, errB := url.Parse(strings.TrimSpace(rawB))
	if errA != nil || errB != nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

func (s *OpenAIGatewayService) ValidateVideoAPIRequestPricing(
	ctx context.Context,
	apiKey *APIKey,
	model, resolution string,
) error {
	if apiKey == nil || apiKey.Group == nil {
		return fmt.Errorf("video API group is required")
	}
	resolved := s.resolveOpenAIChannelPricing(ctx, model, apiKey)
	if resolved != nil && (resolved.Source == PricingSourceGroup || resolved.Source == PricingSourceChannel) {
		config := resolved.channelPricing
		if config != nil {
			switch resolved.Mode {
			case BillingModeToken:
				if videoAPIConfigHasTokenOutputPrice(config) {
					return nil
				}
			case BillingModePerRequest:
				if videoAPIConfigHasPerRequestPrice(config, "") {
					return nil
				}
			case BillingModeVideo:
				if videoAPIConfigHasPerRequestPrice(config, resolution) {
					return nil
				}
			}
		}
	}
	if apiKeyHasConfiguredVideoPrice(apiKey, model, normalizeVideoAPIResolution(resolution)) {
		return nil
	}
	if apiKeyHasConfiguredVideoPrice(apiKey, model, NormalizeVideoBillingResolutionOrDefault(resolution)) {
		return nil
	}
	return fmt.Errorf("video pricing is not configured for model %s at resolution %s", model, resolution)
}

func (s *OpenAIGatewayService) VideoAPIBillingMode(ctx context.Context, apiKey *APIKey, model string) BillingMode {
	if apiKey == nil || apiKey.Group == nil {
		return ""
	}
	resolved := s.resolveOpenAIChannelPricing(ctx, model, apiKey)
	if resolved != nil && (resolved.Source == PricingSourceGroup || resolved.Source == PricingSourceChannel) {
		switch resolved.Mode {
		case BillingModeToken, BillingModePerRequest, BillingModeVideo:
			return resolved.Mode
		}
	}
	if apiKeyHasConfiguredVideoPrice(apiKey, model, VideoBillingResolution480P) ||
		apiKey.Group.VideoPrice480P != nil || apiKey.Group.VideoPrice720P != nil || apiKey.Group.VideoPrice1080P != nil ||
		len(apiKey.Group.VideoModelPrices) > 0 {
		return BillingModeVideo
	}
	return ""
}

func videoAPIConfigHasTokenOutputPrice(config *ChannelModelPricing) bool {
	if config == nil {
		return false
	}
	if config.OutputPrice != nil {
		return true
	}
	for i := range config.Intervals {
		if config.Intervals[i].OutputPrice != nil {
			return true
		}
	}
	return false
}

func videoAPIConfigHasPerRequestPrice(config *ChannelModelPricing, resolution string) bool {
	if config == nil {
		return false
	}
	if config.PerRequestPrice != nil {
		return true
	}
	resolution = normalizeVideoAPIResolution(resolution)
	for i := range config.Intervals {
		interval := &config.Intervals[i]
		if interval.PerRequestPrice == nil {
			continue
		}
		if resolution == "" || normalizeVideoAPIResolution(interval.TierLabel) == resolution {
			return true
		}
	}
	return false
}
