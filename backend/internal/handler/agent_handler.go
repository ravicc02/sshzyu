package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/agent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/deployment"
	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	svc   *agent.Service
	cfg   *config.Config
	ready bool
}

func NewAgentHandler(db *sql.DB, keys *service.APIKeyService, gateway *service.GatewayService, openai *OpenAIGatewayHandler, generic *GatewayHandler, subs *service.SubscriptionService, cfg *config.Config, prices *service.ModelPricingResolver) *AgentHandler {
	enabled := os.Getenv("AGENT_ENABLED") == "true"
	dir := os.Getenv("AGENT_STORAGE_DIR")
	if dir == "" {
		dir = filepath.Join(cfg.Pricing.DataDir, "agent")
	}
	executor := newPlatformAgentExecutor(keys, gateway, openai, generic, subs, cfg, prices, db)
	svc := agent.New(&agent.Store{DB: db}, executor, dir, enabled)
	svc.MinFreeDiskBytes = agent.DefaultMinFreeBytes
	for name, target := range map[string]*int64{"AGENT_STORAGE_MAX_BYTES": &svc.Store.MaxStorageBytes, "AGENT_MIN_FREE_DISK_BYTES": &svc.MinFreeDiskBytes} {
		if raw := os.Getenv(name); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value <= 0 {
				svc.Enabled = false
			} else {
				*target = value
			}
		}
	}
	if value := os.Getenv("AGENT_RECONCILE_TIMEOUT_SECONDS"); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil && seconds >= 1 && seconds <= 900 {
			svc.ReconcileTimeout = time.Duration(seconds) * time.Second
		}
	}

	var ready bool
	if e := db.QueryRow(`SELECT to_regclass('public.agent_task_outbox') IS NOT NULL AND to_regclass('public.agent_checkpoints') IS NOT NULL AND to_regclass('public.agent_event_subscriptions') IS NOT NULL`).Scan(&ready); e != nil {
		ready = false
	}
	if !ready {
		svc.Enabled = false
	}
	if svc.Enabled {
		if e := os.MkdirAll(dir, 0700); e != nil {
			svc.Enabled = false
		}
	}

	if ready {
		deployment.StartWhenOpen(svc.Start)
	}
	return &AgentHandler{svc: svc, cfg: cfg, ready: ready}
}
func (h *AgentHandler) user(c *gin.Context) (int64, bool) {
	c.Header("Cache-Control", "private, no-store, max-age=0")
	c.Header("Pragma", "no-cache")
	s, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(401, gin.H{"code": "UNAUTHORIZED"})
		return 0, false
	}
	return s.UserID, true
}
func (h *AgentHandler) fail(c *gin.Context, err error) {
	var feedback *agent.ToolFeedback
	if errors.As(err, &feedback) {
		c.JSON(400, gin.H{"code": feedback.Code, "message": feedback.Message, "field_errors": feedback.Fields})
		return
	}
	if agent.SafeCode(err) == "ASSET_EXPIRED" {
		c.JSON(410, gin.H{"code": "ASSET_EXPIRED", "message": "文件内容已过24小时，请重新上传。历史记录仍然保留。"})
		return
	}

	status := 400
	code := agent.SafeCode(err)
	switch {
	case errors.Is(err, agent.ErrNotFound):
		status = 404
	case errors.Is(err, agent.ErrConflict), errors.Is(err, agent.ErrBusy):
		status = 409
	case code == "AGENT_DISK_PRESSURE", code == "AGENT_STORAGE_CAPACITY", code == "AGENT_STORAGE_UNAVAILABLE":
		status = 503
		c.Header("Retry-After", "30")
	case code == "AGENT_SUBSCRIPTION_LIMIT":
		status = 429
		c.Header("Retry-After", "2")
	case errors.Is(err, agent.ErrDisabled):
		status = 503
	}
	c.JSON(status, gin.H{"code": code, "message": agentErrorText(code)})
}
func agentErrorText(code string) string {
	switch code {
	case "AGENT_WORK_PLAN_INVALID":
		return "请检查配置卡字段。草稿最多32项，参考图可选；生成前需要补全内容与规格。"
	case "AGENT_WORK_REVIEW_REQUIRED":
		return "方案已变化或报价已过期，请重新保存并核价后提交。"
	case "AGENT_IMAGE_KEY_REQUIRED":
		return "方案已保留，请在设置中单独选择生图key及模型，再保存核价。"
	case "AGENT_RETRY_NOT_LATEST":
		return "此回复之后已有新消息，请重试最新一轮。"
	case "AGENT_RETRY_UNSETTLED":
		return "原调用未取得完整结果，无法直接重试。请发送新消息明确发起新的请求。"
	case "AGENT_RETRY_CONTEXT_UNAVAILABLE":
		return "该历史请求缺少可恢复的上下文，请重新发送消息。"
	case "AGENT_MODEL_VISION_UNSUPPORTED":
		return "当前对话模型不支持图片输入，请在设置中选择支持识图的模型后重试。"
	case "AGENT_VISION_IMAGE_TOO_LARGE":
		return "识图图片分辨率过大，请缩小或裁剪后重试。原文件仍保留。"
	case "AGENT_MODEL_REASONING_UNSUPPORTED":
		return "该模型不支持所选思考档位，请重新选择。"
	case "AGENT_TEXT_PROTOCOL_UNSUPPORTED":
		return "该供应商使用评分/分类协议，不提供对话或工具调用，请选择对话模型 key。"
	case "AGENT_MODEL_CATALOG_UNAVAILABLE":
		return "模型列表暂时无法读取，请稍后重新获取。"
	case "AGENT_SUBSCRIPTION_LIMIT":
		return "实时连接过多，请关闭重复页面后重试。"
	case "AGENT_DISK_PRESSURE", "AGENT_STORAGE_CAPACITY", "AGENT_STORAGE_UNAVAILABLE":
		return "Agent 存储空间暂时不足，请稍后再试；已有结果和费用记录会保留。"
	case "AGENT_STORAGE_QUOTA_EXCEEDED":
		return "Agent 存储额度不足，无法保存或预留结果空间；已有文件和费用记录已保留。"
	case "AGENT_IMAGE_RESULT_TOO_LARGE":
		return "上游已返回图片，但超过单图大小限制；响应与费用记录已保留，平台不会自动重发。"
	case "AGENT_IMAGE_RESULT_INVALID":
		return "上游返回的图片格式无效；响应与费用记录已保留，平台不会自动重发。"
	case "AGENT_IMAGE_RECOVERY_UNAVAILABLE":
		return "没有可恢复的已保存图片响应；本次操作不会重新生成图片。"
	case "AGENT_IMAGE_AUTHORIZATION_REQUIRED":
		return "请明确需要生成或修改的图片，状态查询不会发起新的生图请求。"
	case "AGENT_FILE_SIZE_LIMIT":
		return "文件不能为空，且不能超过 10 MiB。"
	case "AGENT_INVALID_FILE":
		return "文件内容与格式不符，或文件已损坏，请检查后重新上传。"
	case "AGENT_UNSUPPORTED_FILE":
		return "暂不支持此格式，请添加图片、PDF、文本或 DOCX/XLSX/PPTX 文件。"
	case "AGENT_DISABLED":
		return "Agent 尚未启用，请联系管理员。"
	case agent.ReconciliationTimeoutCode:
		return "请求未完成，本次调用已结束。"
	case "AGENT_UPSTREAM_CONNECTION_FAILED":
		return "上游连接失败，请检查当前账号的出站代理或网络。请求未发送到模型服务。"
	case "AGENT_QUESTION_REQUIRED":
		return "当前请求明确要求先提问，但模型没有返回可交互的问题，请重试。"
	case "AGENT_RUN_BUSY":
		return "已有任务正在执行，请先完成或停止该任务。"
	case "WORK_RESULT_UNKNOWN":
		return "部分图片调用未取得完整结果，已阻止重复生成。请发送新消息明确发起新的生成请求。"
	case "AGENT_CONFLICT":
		return "配置已变化或重复请求的内容不同，请刷新。"
	case "AGENT_UNBOUNDED_PRICING":
		return "当前模型的计费来源或价卡无法确定费用上界。"
	case "AGENT_PRICING_CHANGED":
		return "账号或价卡发生变化，请重新提交以获取新的费用预估。"
	case "AGENT_IMAGE_FIXED_PRICE_REQUIRED":
		return "首版只支持可提前确认按张价格的生图模型。"
	case "AGENT_AUTHORIZATION_DENIED":
		return "当前 key 的权限、IP、额度或订阅不允许调用。"
	case "AGENT_MODEL_NOT_ALLOWED":
		return "请选择该 key 实际可用的模型。"
	case "AGENT_UNSUPPORTED_GROUP":
		return "请选择平台支持的文本分组 key；生图 key 需要使用 OpenAI 生图分组。"
	}
	return "Agent 请求未完成，请检查配置或稍后查询任务。"
}
func readAgentJSON(c *gin.Context, v any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	b, e := io.ReadAll(c.Request.Body)
	if e != nil {
		return e
	}
	return agent.StrictJSON(b, v)
}
func (h *AgentHandler) Info(c *gin.Context) {
	c.JSON(200, gin.H{"enabled": h.svc.Accepting(), "asset_thumbnail_supported": true, "cleanup_last_success": h.svc.CleanupLastSuccess(), "history_available": h.ready, "conversation_memory": true, "reference_limit": 8, "supported_platforms": append(domain.PlatformIDsWhere(func(spec domain.PlatformSpec) bool { return spec.ID != service.PlatformTypeSafe }), service.PlatformComposite)})
}
func (h *AgentHandler) Options(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	purpose := c.DefaultQuery("purpose", "text")
	if purpose != "text" && purpose != "image" {
		h.fail(c, errors.New("invalid purpose"))
		return
	}
	opts, e := h.svc.Exec.Options(c.Request.Context(), u, purpose)
	if e != nil {
		h.fail(c, e)
		return
	}
	c.JSON(200, opts)
}
func (h *AgentHandler) Capabilities(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	id, e := strconv.ParseInt(c.Query("api_key_id"), 10, 64)
	if e != nil || id <= 0 {
		h.fail(c, errors.New("invalid key"))
		return
	}
	purpose := c.DefaultQuery("purpose", "text")
	if purpose != "text" && purpose != "image" {
		h.fail(c, errors.New("invalid purpose"))
		return
	}
	models, e := h.svc.Exec.Models(c.Request.Context(), agent.Binding{KeyID: id, UserID: u, IP: ip.GetSecurityClientIP(c, h.cfg.TrustForwardedIPForAPIKeyACL())}, purpose)
	if e != nil {
		h.fail(c, e)
		return
	}
	capabilities := []agent.ModelCapability{}
	if provider, ok := h.svc.Exec.(interface {
		ModelCapabilities(context.Context, agent.Binding, []string) ([]agent.ModelCapability, error)
	}); ok && purpose == "text" {
		capabilities, e = provider.ModelCapabilities(c.Request.Context(), agent.Binding{KeyID: id, UserID: u}, models)
		if e != nil {
			h.fail(c, e)
			return
		}
	}
	c.JSON(200, gin.H{"models": models, "model_capabilities": capabilities})
}
func (h *AgentHandler) List(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	if !h.ready {
		c.JSON(200, []any{})
		return
	}
	v, e := h.svc.Store.Conversations(c.Request.Context(), u)
	if e != nil {
		h.fail(c, e)
		return
	}
	c.JSON(200, v)
}
func (h *AgentHandler) Create(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	var in struct {
		Title  string       `json:"title"`
		Config agent.Config `json:"config"`
	}
	if e := readAgentJSON(c, &in); e != nil {
		h.fail(c, e)
		return
	}
	v, e := h.svc.Create(c.Request.Context(), u, in.Title, in.Config, ip.GetSecurityClientIP(c, h.cfg.TrustForwardedIPForAPIKeyACL()))
	if e != nil {
		h.fail(c, e)
		return
	}
	c.JSON(201, v)
}
func (h *AgentHandler) Detail(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	conv, e := h.svc.Store.Conversation(c.Request.Context(), u, c.Param("conversation_id"))
	if e != nil {
		h.fail(c, e)
		return
	}
	msgs, e := h.svc.Store.Messages(c.Request.Context(), u, conv.ID)
	if e != nil {
		h.fail(c, e)
		return
	}
	r, e := h.svc.Store.Active(c.Request.Context(), u, conv.ID)
	if e != nil {
		h.fail(c, e)
		return
	}
	var active any
	if r != nil {
		active = r.Public()
	}
	traces := make([][]agent.TraceStep, 0, len(msgs)+1)
	for i := range msgs {
		traces = append(traces, msgs[i].Trace)
	}
	if r != nil {
		traces = append(traces, r.Trace)
	}
	h.svc.PopulateTraceCommentary(c.Request.Context(), u, conv.ID, traces...)
	if r != nil {
		active = r.Public()
	}
	metrics := h.replyMetrics(c.Request.Context(), u, conv.ID)
	for i := range msgs {
		if msgs[i].Role == "assistant" {
			msgs[i].Metrics = metrics[msgs[i].RunID]
		}
	}
	plans, err := h.svc.Store.WorkPlans(c.Request.Context(), u, conv.ID)
	if err != nil {
		h.fail(c, err)
		return
	}
	grants, err := h.svc.Store.Grants(c.Request.Context(), u, conv.ID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(200, gin.H{"conversation": conv, "messages": msgs, "active_run": active, "work_plans": plans, "execution_grants": grants})
}
func (h *AgentHandler) Configure(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	if !h.svc.Enabled {
		h.fail(c, agent.ErrDisabled)
		return
	}
	var in struct {
		Config  agent.Config `json:"config"`
		Version int          `json:"config_version"`
	}
	if e := readAgentJSON(c, &in); e != nil {
		h.fail(c, e)
		return
	}
	if e := in.Config.Validate(); e != nil {
		h.fail(c, e)
		return
	}
	clientIP := ip.GetSecurityClientIP(c, h.cfg.TrustForwardedIPForAPIKeyACL())
	if _, e := h.svc.Exec.Authorize(c.Request.Context(), agent.Binding{KeyID: in.Config.ModelKeyID, UserID: u, IP: clientIP}, "text", in.Config.Model); e != nil {
		h.fail(c, e)
		return
	}
	if in.Config.ImageKeyID > 0 {
		if _, e := h.svc.Exec.Authorize(c.Request.Context(), agent.Binding{KeyID: in.Config.ImageKeyID, UserID: u, IP: clientIP}, "image", in.Config.ImageModel); e != nil {
			h.fail(c, e)
			return
		}
	}
	if e := h.svc.Store.Configure(c.Request.Context(), u, c.Param("conversation_id"), in.Version, in.Config); e != nil {
		h.fail(c, e)
		return
	}
	h.Detail(c)
}
func (h *AgentHandler) Submit(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	var in struct {
		Content string   `json:"content"`
		Assets  []string `json:"attachment_asset_ids"`
		Version int      `json:"config_version"`
		Budget  string   `json:"max_cost_usd"`
	}
	if e := readAgentJSON(c, &in); e != nil {
		h.fail(c, e)
		return
	}
	r, e := h.svc.Submit(c.Request.Context(), u, c.Param("conversation_id"), c.GetHeader("Idempotency-Key"), ip.GetSecurityClientIP(c, h.cfg.TrustForwardedIPForAPIKeyACL()), in.Content, in.Assets, in.Version, in.Budget)
	if e != nil {
		h.fail(c, e)
		return
	}
	c.JSON(202, r.Public())
}
func (h *AgentHandler) Retry(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	var in struct {
		Version int    `json:"config_version"`
		Budget  string `json:"max_cost_usd"`
	}
	if err := readAgentJSON(c, &in); err != nil {
		h.fail(c, err)
		return
	}
	r, err := h.svc.Retry(c.Request.Context(), u, c.Param("run_id"), c.GetHeader("Idempotency-Key"), ip.GetSecurityClientIP(c, h.cfg.TrustForwardedIPForAPIKeyACL()), in.Version, in.Budget)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(202, r.Public())
}
func (h *AgentHandler) GetRun(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	r, e := h.svc.Store.Run(c.Request.Context(), u, c.Param("run_id"))
	if e != nil {
		h.fail(c, e)
		return
	}
	h.svc.PopulateTraceCommentary(c.Request.Context(), u, r.ConversationID, r.Trace)
	public := r.Public()
	if r.Terminal() {
		if metrics := h.replyMetrics(c.Request.Context(), u, r.ConversationID)[r.ID]; metrics != nil {
			public["metrics"] = metrics
		}
	}
	c.JSON(200, public)
}
func (h *AgentHandler) Cancel(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	if e := h.svc.Store.Cancel(c.Request.Context(), u, c.Param("run_id")); e != nil {
		h.fail(c, e)
		return
	}
	h.GetRun(c)
}
func (h *AgentHandler) Events(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	id := c.Param("run_id")
	r, e := h.svc.Store.Run(c.Request.Context(), u, id)
	if e != nil {
		h.fail(c, e)
		return
	}
	after, _ := strconv.ParseInt(c.GetHeader("Last-Event-ID"), 10, 64)
	if after < 0 || after > r.LastSequence {
		h.fail(c, agent.ErrConflict)
		return
	}
	subscription, e := h.svc.Store.Subscribe(c.Request.Context(), u, id)
	if e != nil {
		h.fail(c, e)
		return
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		h.svc.Store.Unsubscribe(ctx, subscription)
	}()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	c.Status(200)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	started := time.Now()
	for {
		events, status, e := h.svc.Store.PollEvents(c.Request.Context(), u, id, after)
		if e != nil {
			return
		}
		for _, ev := range events {
			b, _ := json.Marshal(ev.Data)
			if _, e = fmt.Fprintf(c.Writer, "id: %d\nevent: %s\ndata: %s\n\n", ev.Sequence, ev.Type, b); e != nil {
				return
			}
			after = ev.Sequence
		}
		if len(events) == 0 {
			_, _ = io.WriteString(c.Writer, ": heartbeat\n\n")
		}
		c.Writer.Flush()
		current := agent.Run{Status: status}
		more := len(events) > 0 && events[len(events)-1].HasMore
		if (current.Terminal() || status == "waiting_user") && !more || time.Since(started) > 2*time.Minute {
			return
		}
		if more {
			continue
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
func (h *AgentHandler) Upload(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, (10<<20)+(64<<10))
	f, header, e := c.Request.FormFile("file")
	if e != nil {
		h.fail(c, e)
		return
	}
	defer f.Close()
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	b, e := io.ReadAll(io.LimitReader(f, (10<<20)+1))
	if e != nil {
		h.fail(c, e)
		return
	}
	a, e := h.svc.UploadFile(c.Request.Context(), u, c.Param("conversation_id"), header.Filename, b)
	if e != nil {
		h.fail(c, e)
		return
	}
	c.JSON(201, a)
}
func (h *AgentHandler) Asset(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	a, file, e := h.svc.OpenAsset(c.Request.Context(), u, c.Param("asset_id"))
	if e != nil {
		h.fail(c, e)
		return
	}
	defer file.Close()
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	disposition := "attachment"
	if strings.HasPrefix(a.MIME, "image/") {
		disposition = "inline"
	}
	name := a.FileName
	if name == "" {
		name = a.ID + "." + strings.TrimPrefix(a.MIME, "image/")
	}
	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	c.DataFromReader(200, a.Size, a.MIME, file, nil)
}
func (h *AgentHandler) DeleteAsset(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	res, e := h.svc.Store.DB.ExecContext(c.Request.Context(), `UPDATE agent_assets a SET status='deleted' WHERE id=$1 AND user_id=$2 AND NOT EXISTS(SELECT 1 FROM agent_runs r WHERE r.user_id=$2 AND r.conversation_id=a.conversation_id AND r.status NOT IN ('completed','partial_success','failed','cancelled','expired'))`, c.Param("asset_id"), u)
	if e != nil {
		h.fail(c, e)
		return
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		h.fail(c, agent.ErrConflict)
		return
	}
	c.Status(204)
}

// Shutdown hook is called by application cleanup before database connections close.
func (h *AgentHandler) Close() { h.svc.Close() }

func (h *AgentHandler) ManageConversations(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	var in struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"`
		Title  string   `json:"title"`
	}
	if e := readAgentJSON(c, &in); e != nil {
		h.fail(c, e)
		return
	}
	if e := h.svc.Store.Manage(c.Request.Context(), u, in.IDs, in.Action, in.Title); e != nil {
		h.fail(c, e)
		return
	}
	h.List(c)
}
func (h *AgentHandler) Answer(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	if !h.svc.Enabled {
		h.fail(c, agent.ErrDisabled)
		return
	}
	var in struct {
		QuestionID string            `json:"question_id"`
		Answers    map[string]string `json:"answers"`
		Assets     []string          `json:"attachment_asset_ids"`
	}
	if e := readAgentJSON(c, &in); e != nil {
		h.fail(c, e)
		return
	}
	r, e := h.svc.Store.AnswerWithAssets(c.Request.Context(), u, c.Param("run_id"), in.QuestionID, in.Answers, in.Assets)
	if e != nil {
		h.fail(c, e)
		return
	}
	c.JSON(200, r.Public())
}

func (h *AgentHandler) AssetMeta(c *gin.Context) {
	u, ok := h.user(c)
	if !ok {
		return
	}
	a, e := h.svc.Store.AssetMeta(c.Request.Context(), u, c.Param("asset_id"))
	if e != nil {
		h.fail(c, e)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, a)
}

func (h *AgentHandler) Thumbnail(c *gin.Context) {
	user, ok := h.user(c)
	if !ok {
		return
	}
	data, err := h.svc.Thumbnail(c.Request.Context(), user, c.Param("asset_id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(200, "image/jpeg", data)
}
