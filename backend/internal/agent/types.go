// Package agent owns durable platform agent orchestration. Credentials and HTTP
// framework objects never cross its execution interface.
package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

var ErrNotFound = errors.New("AGENT_NOT_FOUND")
var ErrConflict = errors.New("AGENT_CONFLICT")
var ErrBusy = errors.New("AGENT_RUN_BUSY")
var ErrDisabled = errors.New("AGENT_DISABLED")
var ErrUnknown = errors.New("UPSTREAM_RESULT_UNKNOWN")
var ErrToolBatchLimit = errors.New("AGENT_TOOL_BATCH_LIMIT")

const MaxToolCallsPerResponse = 8

var ErrReplay = errors.New("EVENT_REPLAY_UNAVAILABLE")

func ID(prefix string) string { return prefix + "_" + uuid.NewString() }
func Hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

var amountPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,5})(\.[0-9]{1,8})?$`)

func Amount(s string) (decimal.Decimal, error) {
	if !amountPattern.MatchString(s) {
		return decimal.Zero, errors.New("invalid USD amount")
	}
	return decimal.NewFromString(s)
}
func WithinBudget(spent, quote, budget string) bool {
	a, e := Amount(spent)
	b, f := Amount(quote)
	c, g := Amount(budget)
	return e == nil && f == nil && g == nil && a.Add(b).LessThanOrEqual(c)
}
func AddAmount(a, b string) string {
	x, _ := Amount(a)
	y, _ := Amount(b)
	return x.Add(y).StringFixed(8)
}

type Config struct {
	ModelKeyID int64  `json:"model_api_key_id"`
	Model      string `json:"model"`
	ImageKeyID int64  `json:"image_api_key_id,omitempty"`
	ImageModel string `json:"image_model,omitempty"`
	// Deprecated: decoded for old snapshots only, never used as a spending limit.
	Budget          string `json:"default_max_cost_usd,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

func (c Config) Validate() error {
	if c.ModelKeyID <= 0 || strings.TrimSpace(c.Model) == "" || len(c.Model) > 128 {
		return errors.New("select a text key and model")
	}
	if c.ImageKeyID < 0 || c.ImageKeyID == c.ModelKeyID {
		return errors.New("image key must be selected separately")
	}
	if (c.ImageKeyID > 0) != (strings.TrimSpace(c.ImageModel) != "") {
		return errors.New("select an image key and model together")
	}
	if c.ReasoningEffort != "" && !oneOf(c.ReasoningEffort, "none", "minimal", "low", "medium", "high", "xhigh", "max") {
		return errors.New("invalid reasoning effort")
	}
	return nil
}

type Conversation struct {
	ID         string     `json:"id"`
	UserID     int64      `json:"-"`
	Title      string     `json:"title"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	Config     Config     `json:"config"`
	Version    int        `json:"config_version"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
type QuestionAnswer struct {
	Label string `json:"label"`
	Value string `json:"value"`
}
type ReplyMetrics struct {
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheTokens  int64  `json:"cache_tokens"`
	TotalTokens  int64  `json:"total_tokens"`
	ElapsedMS    *int64 `json:"elapsed_ms,omitempty"`
	GenerationMS *int64 `json:"generation_ms,omitempty"`
}
type Message struct {
	RunStatus       string           `json:"run_status,omitempty"`
	ErrorCode       string           `json:"error_code,omitempty"`
	CanRecoverImage bool             `json:"can_recover_image,omitempty"`
	Metrics         *ReplyMetrics    `json:"metrics,omitempty"`
	Kind            string           `json:"kind,omitempty"`
	QuestionID      string           `json:"question_id,omitempty"`
	Answers         []QuestionAnswer `json:"answers,omitempty"`
	ID              string           `json:"id"`
	Role            string           `json:"role"`
	Text            string           `json:"text"`
	Assets          []string         `json:"asset_ids,omitempty"`
	RunID           string           `json:"run_id,omitempty"`
	Trace           []TraceStep      `json:"trace,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
}
type ConversationDetail struct {
	Conversation
	Messages  []Message `json:"messages"`
	ActiveRun *Run      `json:"active_run,omitempty"`
}
type Run struct {
	TaskID             string         `json:"task_id,omitempty"`
	RuntimeVersion     int            `json:"runtime_version,omitempty"`
	ToolErrors         int            `json:"tool_errors,omitempty"`
	LastToolError      string         `json:"last_tool_error,omitempty"`
	RepeatedToolErrors int            `json:"repeated_tool_errors,omitempty"`
	RecoverOnly        bool           `json:"recover_only,omitempty"`
	RecoveredAt        *time.Time     `json:"recovered_at,omitempty"`
	StatusInquiry      bool           `json:"status_inquiry,omitempty"`
	WorkPlanning       bool           `json:"work_planning,omitempty"`
	Work               *WorkExecution `json:"work,omitempty"`
	RetryOf            string         `json:"retry_of,omitempty"`
	ID                 string         `json:"id"`
	ConversationID     string         `json:"conversation_id"`
	UserID             int64          `json:"-"`
	Config             Config         `json:"config"`
	ConfigVersion      int            `json:"config_version"`
	IP                 string         `json:"client_ip,omitempty"`
	Key                string         `json:"-"`
	RequestHash        string         `json:"-"`
	Status             string         `json:"status"`
	ErrorCode          string         `json:"error_code,omitempty"`
	Text               string         `json:"text"`
	StreamOperation    string         `json:"stream_operation,omitempty"`
	StreamPrefix       string         `json:"stream_prefix,omitempty"`
	Input              string         `json:"input"`
	Attachments        []string       `json:"attachment_asset_ids"`
	AssetIDs           []string       `json:"asset_ids"`
	// Protocol history is private; the HTTP snapshot always uses PublicRun.
	History           []json.RawMessage   `json:"protocol_history,omitempty"`
	Memory            *ConversationMemory `json:"conversation_memory,omitempty"`
	PendingTools      *ToolBatch          `json:"pending_tools,omitempty"`
	PendingCall       *Call               `json:"pending_call,omitempty"`
	Turn              int                 `json:"turn_count"`
	ToolCount         int                 `json:"tool_call_count"`
	ModelCost         string              `json:"model_cost"`
	ImageCost         string              `json:"image_cost"`
	Spent             string              `json:"spent_usd"`
	LastSequence      int64               `json:"last_event_sequence"`
	Deadline          time.Time           `json:"deadline"`
	ReconcileDeadline *time.Time          `json:"reconcile_deadline,omitempty"`
	Trace             []TraceStep         `json:"trace,omitempty"`
	Question          *UserQuestion       `json:"question,omitempty"`
	AskCount          int                 `json:"ask_count,omitempty"`
	AnsweredQuestion  string              `json:"answered_question,omitempty"`
	AnswerHash        string              `json:"answer_hash,omitempty"`
	ForceAskUser      bool                `json:"force_ask_user,omitempty"`
	CreatedAt         time.Time           `json:"created_at"`
	Version           int64               `json:"version"`
	Fence             int64               `json:"-"`
	Owner             string              `json:"-"`
	Cancel            bool                `json:"cancel_requested"`
	ModelAccountID    int64               `json:"model_account_id,omitempty"`
	ModelGroupID      int64               `json:"model_group_id"`
	ImageGroupID      int64               `json:"image_group_id"`
}

func (r Run) Terminal() bool {
	switch r.Status {
	case "completed", "partial_success", "failed", "cancelled", "expired":
		return true
	}
	return false
}
func (r Run) Public() map[string]any {
	b, _ := json.Marshal(r)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	delete(m, "client_ip")
	delete(m, "protocol_history")
	delete(m, "conversation_memory")
	if r.Memory != nil {
		m["memory"] = map[string]any{"message_count": r.Memory.MessageCount, "omitted_context_messages": r.Memory.OmittedMessages, "confirmed_image_count": r.Memory.ConfirmedImages}
	}
	delete(m, "pending_call")
	delete(m, "pending_tools")
	delete(m, "model_account_id")
	delete(m, "model_group_id")
	delete(m, "image_group_id")
	delete(m, "stream_operation")
	delete(m, "stream_prefix")
	delete(m, "answer_hash")
	delete(m, "force_ask_user")
	delete(m, "answered_question")
	return m
}

type Event struct {
	HasMore   bool            `json:"-"`
	Sequence  int64           `json:"sequence"`
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}
type Binding struct {
	KeyID   int64
	GroupID int64
	UserID  int64
	IP      string
}
type ModelCapability struct {
	ID                     string   `json:"id"`
	ReasoningLevels        []string `json:"reasoning_levels"`
	DefaultReasoningEffort string   `json:"default_reasoning_effort"`
}
type KeyOption struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	GroupName      string `json:"group_name"`
	Purpose        string `json:"purpose"`
	Platform       string `json:"platform,omitempty"`
	DisabledReason string `json:"disabled_reason,omitempty"`
}
type ToolArgs struct {
	TaskID     *string  `json:"task_id,omitempty"`
	Purpose    *string  `json:"purpose,omitempty"`
	Prompt     string   `json:"prompt"`
	Model      *string  `json:"model"`
	Size       *string  `json:"image_size"`
	Aspect     *string  `json:"aspect_ratio"`
	References []string `json:"reference_asset_ids"`
}
type ToolBatch struct {
	OperationID string `json:"operation_id"`
	Calls       []Call `json:"calls"`
}

type Call struct {
	ID        string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	ToolID    string `json:"tool_id"`
}
type Response struct {
	Summary string
	Items   []json.RawMessage
	Text    string
	Calls   []Call
}
type Invocation struct {
	LegacyContract bool   `json:"-"`
	ProjectID      string `json:"-"`
	GrantID        string `json:"-"`

	WorkPlanning          bool                     `json:"-"`
	ApprovedQuote         string                   `json:"-"`
	WorkContext           string                   `json:"-"`
	CostLimit             string                   `json:"-"`
	OnProgress            func(StreamUpdate) error `json:"-"`
	AccountID             int64
	OperationID           string
	Kind                  string
	Binding               Binding
	Model                 string
	ReasoningEffort       string  `json:"-"`
	ReasoningSupported    bool    `json:"-"`
	ForceAskUser          bool    `json:"-"`
	ClarificationAnswered bool    `json:"-"`
	VisionInputs          []Asset `json:"-"`
	FileInputs            []Asset `json:"-"`
	Input                 []json.RawMessage
	Memory                *ConversationMemory
	Tools                 bool
	ImageTools            bool
	Args                  *ToolArgs
	References            []Asset
	RunID                 string
}
type Outcome struct {
	ImageFromBody  bool `json:"image_from_body,omitempty"`
	AccountID      int64
	Body           json.RawMessage
	Cost           string
	BillingPending bool
	Unknown        bool
	Image          []byte
	MIME           string
	Width          int
	Height         int
}
type Executor interface {
	Options(context.Context, int64, string) ([]KeyOption, error)
	Models(context.Context, Binding, string) ([]string, error)
	Authorize(context.Context, Binding, string, string) (int64, error)
	Quote(context.Context, Invocation) (string, error)
	Execute(context.Context, Invocation) (Outcome, error)
}
type Asset struct {
	Status           string    `json:"status"`
	ContentAvailable bool      `json:"content_available"`
	ID               string    `json:"id"`
	UserID           int64     `json:"-"`
	ConversationID   string    `json:"conversation_id"`
	RunID            string    `json:"run_id,omitempty"`
	MIME             string    `json:"mime_type"`
	FileName         string    `json:"filename"`
	Size             int64     `json:"byte_size"`
	Width            int       `json:"width"`
	Height           int       `json:"height"`
	Key              string    `json:"-"`
	SHA              string    `json:"sha256"`
	Expires          time.Time `json:"expires_at"`
	CreatedAt        time.Time `json:"created_at"`
	Data             []byte    `json:"-"`
}
type Operation struct {
	ReferenceIDs []string

	ProjectID string

	GrantID string

	ID          string
	RunID       string
	Kind        string
	Step        int
	Status      string
	Quote       string
	Cost        string
	RequestHash string
	Output      json.RawMessage
	AssetID     string
	ErrorCode   string
}

func StrictJSON(raw []byte, dest any) error {
	if len(raw) > 64<<10 {
		return errors.New("JSON too large")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return errors.New("trailing JSON")
	}
	// Reject duplicate object keys (including nested objects).
	dec = json.NewDecoder(bytes.NewReader(raw))
	return checkJSON(dec)
}
func checkJSON(d *json.Decoder) error {
	tok, e := d.Token()
	if e != nil {
		return e
	}
	del, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	if del == '{' {
		seen := map[string]bool{}
		for d.More() {
			t, e := d.Token()
			if e != nil {
				return e
			}
			k, ok := t.(string)
			if !ok || seen[k] {
				return errors.New("duplicate JSON field")
			}
			seen[k] = true
			if e = checkJSON(d); e != nil {
				return e
			}
		}
	} else if del == '[' {
		for d.More() {
			if e = checkJSON(d); e != nil {
				return e
			}
		}
	}
	_, e = d.Token()
	return e
}
func ParseTool(raw json.RawMessage) (ToolArgs, error) {
	var a ToolArgs
	if err := StrictJSON(raw, &a); err != nil {
		return a, err
	}
	if strings.TrimSpace(a.Prompt) == "" || len(a.Prompt) > 8000 || len(a.References) > 1 {
		return a, errors.New("invalid image prompt or references")
	}
	if a.Size != nil && !oneOf(*a.Size, "1K", "2K", "4K") {
		return a, errors.New("invalid image size")
	}
	if a.Aspect != nil && !oneOf(*a.Aspect, "1:1", "16:9", "9:16", "4:3", "3:4") {
		return a, errors.New("invalid aspect ratio")
	}
	return a, nil
}
func oneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
func ParseResponse(raw json.RawMessage) (Response, error) {
	var v struct {
		Status string            `json:"status"`
		Output []json.RawMessage `json:"output"`
	}
	var out Response
	if err := json.Unmarshal(raw, &v); err != nil {
		return out, err
	}
	if v.Status != "completed" {
		return out, errors.New("AGENT_MODEL_INCOMPLETE")
	}
	out.Items = v.Output
	callIDs := map[string]bool{}
	for _, b := range v.Output {
		var it struct {
			Type      string `json:"type"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			Summary   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"summary"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(b, &it); err != nil {
			return out, err
		}
		if it.Type == "reasoning" {
			for _, summary := range it.Summary {
				if summary.Type == "summary_text" && strings.TrimSpace(summary.Text) != "" {
					if out.Summary != "" {
						out.Summary += "\n\n"
					}
					out.Summary += summary.Text
				}
			}
		}
		if it.Type == "function_call" {
			if it.CallID == "" || callIDs[it.CallID] || !allowedTool(it.Name) {
				return out, errors.New("AGENT_TOOL_NOT_ALLOWED")
			}
			callIDs[it.CallID] = true
			out.Calls = append(out.Calls, Call{ID: it.CallID, Name: it.Name, Arguments: it.Arguments})
		}
		if it.Type == "message" {
			for _, c := range it.Content {
				if c.Type == "output_text" {
					out.Text += c.Text
				}
			}
		}
	}
	if summary := []rune(out.Summary); len(summary) > 12000 {
		out.Summary = string(summary[:12000])
	}
	if len(out.Calls) > MaxToolCallsPerResponse {
		return out, ErrToolBatchLimit
	}
	return out, nil
}
func ToolDefinition() json.RawMessage {
	return json.RawMessage(`{"type":"function","name":"generate_image","description":"建议或执行一张生图/改图，可用于风格样张、模板或成品。task_id=null时展示配置卡等待真人审批；已有明确任务范围许可时传真实task_id和purpose(sample/output)。模型不能自批或扩大许可。无需参考图。","strict":true,"parameters":{"type":"object","additionalProperties":false,"properties":{"prompt":{"type":"string"},"model":{"type":["string","null"]},"image_size":{"type":["string","null"],"enum":["1K","2K","4K",null]},"aspect_ratio":{"type":["string","null"],"enum":["1:1","16:9","9:16","4:3","3:4",null]},"reference_asset_ids":{"type":["array","null"],"items":{"type":"string"},"maxItems":1},"task_id":{"type":["string","null"]},"purpose":{"type":["string","null"],"enum":["sample","output",null]}},"required":["prompt","model","image_size","aspect_ratio","reference_asset_ids","task_id","purpose"]}}`)
}

// Keep the old parameter contract for persisted v1 checkpoints.
func LegacyImageToolDefinition() json.RawMessage {
	var v map[string]any
	_ = json.Unmarshal(ToolDefinition(), &v)
	params := v["parameters"].(map[string]any)
	props := params["properties"].(map[string]any)
	delete(props, "task_id")
	delete(props, "purpose")
	params["required"] = []string{"prompt", "model", "image_size", "aspect_ratio", "reference_asset_ids"}
	v["description"] = "仅在用户明确要求生图或修改图片时生成一张图片，使用当前绑定模型和真实素材。"
	return jsonBytes(v)
}
func SafeCode(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,80}$`).MatchString(s) {
		return s
	}
	return "AGENT_EXECUTION_FAILED"
}
func jsonBytes(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func UserItem(text string) json.RawMessage {
	return jsonBytes(map[string]any{"role": "user", "content": text})
}
func toolOutput(c Call, asset Asset, cost string) json.RawMessage {
	return jsonBytes(map[string]any{"type": "function_call_output", "call_id": c.ID, "output": string(jsonBytes(map[string]any{"ok": true, "asset_id": asset.ID, "observed_by_model": false, "cost": cost}))})
}
func Invalid(code string) error { return fmt.Errorf("%s", code) }

func HashBytes(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

type InvocationFailure struct {
	Code       string
	Dispatched bool
}

func (e *InvocationFailure) Error() string { return e.Code }

func (o Outcome) confirmed() bool {
	if o.BillingPending || o.Cost == "" {
		return false
	}
	_, err := Amount(o.Cost)
	return err == nil
}

// ResultFailure means the provider response is known and retained, but cannot
// be delivered within the accepted image envelope. It is not a missing result.
type ResultFailure struct{ Code string }

func (e *ResultFailure) Error() string { return e.Code }
