package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Wei-Shaw/sub2api/pkg/agentlimits"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "golang.org/x/image/webp"
)

type Service struct {
	Store               *Store
	Exec                Executor
	Root                string
	Enabled             bool
	WorkConcurrency     int
	MinFreeDiskBytes    int64
	startupFailed       atomic.Bool
	cleanupAt           atomic.Int64
	maintenanceInterval time.Duration
	previewMu           sync.Mutex
	previews            map[string]previewEntry
	ReconcileTimeout    time.Duration
	owner               string
	stop                context.CancelFunc
	wg                  sync.WaitGroup
}

func New(s *Store, e Executor, root string, enabled bool) *Service {
	if s != nil && s.MaxStorageBytes == 0 {
		s.MaxStorageBytes = DefaultStorageBytes
	}
	return &Service{Store: s, Exec: e, Root: root, Enabled: enabled, WorkConcurrency: 3, ReconcileTimeout: DefaultReconcileTimeout, owner: ID("worker")}
}
func (s *Service) Start() {
	if s.Store == nil || s.Store.DB == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.stop = cancel
	// File maintenance continues even when accepting new Agent calls is disabled.
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		interval := s.maintenanceInterval
		if interval <= 0 {
			interval = time.Minute
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanupCtx, done := context.WithTimeout(ctx, 30*time.Second)
				err := s.Cleanup(cleanupCtx)
				done()
				if err != nil {
					slog.Error("agent cleanup failed", "error", SafeCode(err))
				} else {
					s.cleanupAt.Store(time.Now().Unix())
				}
			}
		}
	}()
	if !s.Enabled {
		return
	}
	if err := s.accountExistingCheckpoints(ctx); err != nil {
		s.startupFailed.Store(true)
		slog.Error("agent checkpoint accounting failed", "error", SafeCode(err))
		return
	}
	for worker := 0; worker < 4; worker++ {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					r, e := s.Store.Claim(ctx, s.owner)
					if errors.Is(e, ErrNotFound) {
						continue
					}
					if e != nil {
						slog.Error("agent claim failed", "error", SafeCode(e))
						continue
					}
					s.process(ctx, r)
					_ = s.Store.Release(context.WithoutCancel(ctx), r)
				}
			}
		}()
	}
}
func (s *Service) Close() {
	if s.stop != nil {
		s.stop()
	}
	s.wg.Wait()
}
func (s *Service) Create(ctx context.Context, user int64, title string, c Config, ip string) (Conversation, error) {
	if !s.Accepting() {
		return Conversation{}, ErrDisabled
	}
	if err := c.Validate(); err != nil {
		return Conversation{}, err
	}
	if _, e := s.Exec.Authorize(ctx, Binding{UserID: user, KeyID: c.ModelKeyID, IP: ip}, "text", c.Model); e != nil {
		return Conversation{}, e
	}
	if c.ImageKeyID > 0 {
		if _, e := s.Exec.Authorize(ctx, Binding{UserID: user, KeyID: c.ImageKeyID, IP: ip}, "image", c.ImageModel); e != nil {
			return Conversation{}, e
		}
	}
	c.Budget = ""
	title = strings.TrimSpace(title)
	if title == "" {
		title = "新对话"
	}
	if len([]rune(title)) > 100 {
		return Conversation{}, errors.New("title too long")
	}
	cv := Conversation{ID: ID("conv"), UserID: user, Title: title, Config: c, Version: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	return cv, s.Store.CreateConversation(ctx, cv)
}
func (s *Service) Submit(ctx context.Context, user int64, conv, key, ip, text string, assets []string, version int, budget string) (*Run, error) {
	return s.submit(ctx, user, conv, key, ip, text, assets, version, budget, nil)
}
func (s *Service) Retry(ctx context.Context, user int64, source, key, ip string, version int, budget string) (*Run, error) {
	old, err := s.Store.Run(ctx, user, source)
	if err != nil {
		return nil, err
	}
	if old.Work != nil {
		return nil, Invalid("AGENT_WORK_REVIEW_REQUIRED")
	}
	if old.RuntimeVersion < 2 {
		var expiredSource bool
		if err = s.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_assets WHERE user_id=$1 AND conversation_id=$2 AND expires_at<=now() AND (mime_type LIKE 'text/%' OR mime_type='application/json'))`, user, old.ConversationID).Scan(&expiredSource); err != nil {
			return nil, err
		}
		if expiredSource {
			return nil, Invalid("AGENT_RETRY_CONTEXT_UNAVAILABLE")
		}
	}

	if time.Since(old.CreatedAt) >= ContentRetention {
		return nil, Invalid("AGENT_RETRY_CONTEXT_UNAVAILABLE")
	}

	return s.submit(ctx, user, old.ConversationID, key, ip, old.Input, old.Attachments, version, budget, old)
}
func (s *Service) submit(ctx context.Context, user int64, conv, key, ip, text string, assets []string, version int, budget string, retry *Run) (*Run, error) {
	if !s.Accepting() {
		return nil, ErrDisabled
	}
	if key == "" || len(key) > 200 || len(text) == 0 || len(text) > 16000 || len(assets) > 8 {
		return nil, errors.New("invalid message")
	}
	requestHash := Hash([]any{conv, text, assets, version, budget})
	if retry != nil {
		requestHash = Hash([]any{"retry", retry.ID, conv, version, budget})
	}
	if old, e := s.Store.FindIdempotent(ctx, user, key, requestHash); old != nil || e != nil {
		return old, e
	}
	if retry != nil {
		if !retry.Terminal() {
			return nil, ErrBusy
		}
		unsettled, err := s.Store.Unsettled(ctx, retry.ID)
		if err != nil {
			return nil, err
		}
		if unsettled {
			return nil, Invalid("AGENT_RETRY_UNSETTLED")
		}
	}
	c, e := s.Store.Conversation(ctx, user, conv)
	if e != nil {
		return nil, e
	}
	if c.Version != version {
		return nil, ErrConflict
	}
	// Ignore historical caller/conversation budgets; billing and quote validation remain active.
	c.Config.Budget = ""
	mg, e := s.Exec.Authorize(ctx, Binding{UserID: user, KeyID: c.Config.ModelKeyID, IP: ip}, "text", c.Config.Model)
	if e != nil {
		return nil, e
	}
	var ig int64
	if c.Config.ImageKeyID > 0 {
		ig, e = s.Exec.Authorize(ctx, Binding{UserID: user, KeyID: c.Config.ImageKeyID, IP: ip}, "image", c.Config.ImageModel)
		if e != nil {
			return nil, e
		}
	}
	for _, id := range assets {
		a, e := s.Store.Asset(ctx, user, id)
		if e != nil || a.ConversationID != conv {
			return nil, ErrNotFound
		}
	}
	r := &Run{RuntimeVersion: 2, ID: ID("run"), UserID: user, ConversationID: conv, Config: c.Config, ConfigVersion: version, IP: ip, Key: key, RequestHash: requestHash, Input: text, Attachments: assets, ForceAskUser: RequiresUserQuestion(text), AssetIDs: []string{}, Status: "queued", ModelCost: "0.00000000", ImageCost: "0.00000000", Spent: "0.00000000", CreatedAt: time.Now().UTC(), Deadline: time.Now().Add(900 * time.Second), ModelGroupID: mg, ImageGroupID: ig}
	history, memory, e := s.conversationContext(ctx, user, conv, text, assets)
	if e != nil {
		return nil, e
	}
	r.StatusInquiry = ImageStatusInquiry(text)
	r.History = history
	r.Memory = memory
	if retry != nil {
		end := 0
		for i, raw := range retry.History {
			var item struct {
				Role string `json:"role"`
			}
			if json.Unmarshal(raw, &item) == nil && item.Role == "user" {
				end = i + 1
			}
		}
		if end == 0 {
			return nil, Invalid("AGENT_RETRY_CONTEXT_UNAVAILABLE")
		}
		r.RetryOf = retry.ID
		r.History = append([]json.RawMessage(nil), retry.History[:end]...)
		// Rehydrate the submitted attachment using today's adapter, including old
		// conversations created before multimodal input was implemented.
		if len(assets) > 0 {
			note := text + s.attachmentNote(ctx, user, conv, assets)
			for i := len(r.History) - 1; i >= 0; i-- {
				var item struct {
					Role    string          `json:"role"`
					Content json.RawMessage `json:"content"`
				}
				if json.Unmarshal(r.History[i], &item) != nil || item.Role != "user" {
					continue
				}
				var content string
				if json.Unmarshal(item.Content, &content) != nil {
					var parts []struct {
						Text string `json:"text"`
					}
					if json.Unmarshal(item.Content, &parts) == nil && len(parts) > 0 {
						content = parts[0].Text
					}
				}
				if strings.HasPrefix(content, text) {
					r.History[i] = s.imageUserItem(ctx, user, conv, note, assets)
					break
				}
			}
		}
		expected := ""
		if memory != nil {
			expected = memory.lastMessageID
		}
		if retry.Memory != nil {
			m := *retry.Memory
			r.Memory = &m
		} else {
			r.Memory = &ConversationMemory{ConversationID: conv}
		}
		r.Memory.lastMessageID = expected
		if retry.AnsweredQuestion != "" {
			msgs, err := s.Store.Messages(ctx, user, conv)
			if err != nil {
				return nil, err
			}
			for _, m := range msgs {
				if m.RunID == retry.ID && m.Kind == "question_answer" {
					r.History = append(r.History, UserItem("用户已经确认的补充信息："+m.Text))
				}
			}
			r.AnsweredQuestion = "retry"
			r.ForceAskUser = false
		}
	}
	if e = s.Store.Submit(ctx, r); e != nil {
		if old, f := s.Store.FindIdempotent(ctx, user, key, requestHash); old != nil || f != nil {
			return old, f
		}
		return nil, e
	}
	return r, nil
}
func (s *Service) process(parent context.Context, r *Run) {
	ctx, cancel := context.WithDeadline(parent, r.Deadline)
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				if e := s.Store.Heartbeat(ctx, r); e != nil {
					cancel()
					return
				}
				c, e := s.Store.Cancelled(ctx, r)
				if e != nil || c {
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		if recover() != nil {
			slog.Error("agent worker panic", "run_id", r.ID)
			s.beginReconciliation(r, "AGENT_WORKER_INTERRUPTED")
		}
	}()
	if r.Terminal() {
		return
	}
	// A result-unknown operation stays in the audit trail, but cannot hold the
	// user's execution slot beyond the persisted reconciliation deadline.
	if r.Status == "reconciling" {
		if r.ReconcileDeadline == nil {
			s.beginReconciliation(r, r.ErrorCode)
		}
		if r.Terminal() {
			return
		}
		if s.reconciliationExpired(r, time.Now()) {
			s.finishReconciliationTimeout(r)
			return
		}
		// Recovery only reuses existing operations and checkpoints. Dispatching
		// operations without a result remain unknown and never get reissued.
		r.Status = "running"
		if r.PendingCall != nil {
			r.Status = "executing_tool"
		}
	}
	if r.Status == "waiting_user" {
		if r.Cancel {
			s.finishStopped(r, true)
			return
		}
		if time.Now().After(r.Deadline) {
			s.fail(r, Invalid("AGENT_QUESTION_TIMEOUT"))
		}
		return
	}
	if r.Status == "queued" {
		r.Status = "running"
		if e := s.Store.Save(ctx, r, "run.started", r.Public()); e != nil {
			return
		}
	}
	if r.RecoverOnly {
		s.processImageRecovery(ctx, r)
		return
	}
	if r.Work != nil {
		s.processWork(ctx, r)
		return
	}
	maxTurns := 8
	if r.RuntimeVersion >= 2 {
		maxTurns = 24
	}
	for r.Turn < maxTurns || r.PendingCall != nil || (r.PendingTools != nil && len(r.PendingTools.Calls) > 0) {
		if c, e := s.Store.Cancelled(context.WithoutCancel(ctx), r); e != nil || c || ctx.Err() != nil {
			if e == nil {
				s.finishStopped(r, c)
			}
			return
		}
		if r.PendingCall == nil && r.PendingTools != nil {
			if len(r.PendingTools.Calls) == 0 {
				r.PendingTools = nil
			} else {
				batch := r.PendingTools
				call := batch.Calls[0]
				batch.Calls = batch.Calls[1:]
				if !s.processToolCall(ctx, r, batch.OperationID, call) {
					return
				}
				continue
			}
		}
		if r.PendingCall != nil {
			if r.PendingCall.Name == "ask_user" {
				return
			}
			call := *r.PendingCall
			args, e := ParseTool(json.RawMessage(call.Arguments))
			if e != nil {
				if r.RuntimeVersion >= 2 {
					if !s.returnToolFeedback(ctx, r, call, invalidArguments("generate_image")) {
						return
					}
					continue
				}
				s.fail(r, e)
				return
			}
			if (r.RuntimeVersion < 2 && r.ToolCount >= 2) || r.Config.ImageKeyID == 0 {
				s.fail(r, Invalid("AGENT_TOOL_NOT_ALLOWED"))
				return
			}
			if args.Model != nil && *args.Model != r.Config.ImageModel {
				if r.RuntimeVersion >= 2 {
					if !s.returnToolFeedback(ctx, r, call, Invalid("AGENT_MODEL_NOT_ALLOWED")) {
						return
					}
					continue
				}
				s.fail(r, Invalid("AGENT_MODEL_NOT_ALLOWED"))
				return
			}
			args.Model = stringPtr(r.Config.ImageModel)
			if args.Size == nil {
				args.Size = stringPtr("1K")
			}
			if args.Aspect == nil {
				args.Aspect = stringPtr("1:1")
			}
			refs := []Asset{}
			var referenceErr error
			for _, id := range args.References {
				a, e := s.loadAssetForRun(ctx, r.UserID, id, r.ID)
				if e != nil {
					referenceErr = e
					break
				}
				if a.ConversationID != r.ConversationID || !strings.HasPrefix(a.MIME, "image/") {
					referenceErr = ErrNotFound
					break
				}
				refs = append(refs, a)
			}
			if referenceErr != nil {
				if r.RuntimeVersion >= 2 {
					if !s.returnToolFeedback(ctx, r, call, referenceErr) {
						return
					}
					continue
				}
				s.fail(r, referenceErr)
				return
			}
			r.Status = "executing_tool"
			stepStatus, eventType := "running", "tool.execution.started"
			if r.ReconcileDeadline != nil {
				r.Status, stepStatus, eventType = "reconciling", "reconciling", "operation.reconciling"
			}
			toolStep := r.startStep(call.ToolID, "tool", call.Name, stepStatus, traceArguments(args, r.Config.ImageModel))
			var eventData any = map[string]any{"tool_call_id": call.ToolID, "prompt": args.Prompt}
			if eventType == "operation.reconciling" {
				eventData = r.Public()
			}
			if e = s.Store.Save(ctx, r, eventType, eventData); e != nil {
				return
			}
			in := Invocation{OperationID: r.ID + "_image_" + fmt.Sprint(r.ToolCount), RunID: r.ID, Kind: "image", Binding: Binding{UserID: r.UserID, KeyID: r.Config.ImageKeyID, GroupID: r.ImageGroupID, IP: r.IP}, Model: r.Config.ImageModel, Args: &args, References: refs}
			if r.RuntimeVersion >= 2 {
				if args.TaskID != nil {
					if err := s.attachProject(ctx, r, *args.TaskID); err != nil {
						if !s.returnToolFeedback(ctx, r, call, err) {
							return
						}
						continue
					}
				}
				grant, e := s.imageGrant(ctx, r, args)
				if e != nil {
					if !s.returnToolFeedback(ctx, r, call, e) {
						return
					}
					continue
				}
				in.GrantID = grant
			}
			op, out, e := s.operation(ctx, r, in, r.ToolCount)
			if e != nil {
				if r.RuntimeVersion >= 2 && (op.ID == "" || op.Status == "prepared") {
					if !s.returnToolFeedback(ctx, r, call, e) {
						return
					}
					continue
				}
				s.operationFailure(r, op, e)
				return
			}
			r.ReconcileDeadline = nil
			toolStep.Cost = op.Cost
			a, e := s.saveImage(ctx, r, op, out)
			if e != nil {
				s.beginReconciliation(r, "AGENT_ASSET_STORAGE_FAILED")
				return
			}
			r.completeStep(call.ToolID, op.Cost, "", []string{a.ID})
			r.History = append(r.History, toolOutput(call, a, op.Cost))
			r.AssetIDs = append(r.AssetIDs, a.ID)
			s.syncCosts(r)
			r.ToolCount++
			r.PendingCall = nil
			r.Status = "resuming_model"
			if e = s.Store.Save(context.WithoutCancel(ctx), r, "asset.created", a); e != nil {
				return
			}
			continue
		}
		if c, e := s.Store.Cancelled(context.WithoutCancel(ctx), r); e != nil || c || ctx.Err() != nil {
			if e == nil {
				s.finishStopped(r, c)
			}
			return
		}
		tools := r.Turn < maxTurns-1
		in := Invocation{OperationID: r.ID + "_model_" + fmt.Sprint(r.Turn), RunID: r.ID, Kind: "text", ClarificationAnswered: r.AnsweredQuestion != "", AccountID: r.ModelAccountID, Binding: Binding{UserID: r.UserID, KeyID: r.Config.ModelKeyID, GroupID: r.ModelGroupID, IP: r.IP}, Model: r.Config.Model, ReasoningEffort: r.Config.ReasoningEffort, ForceAskUser: r.ForceAskUser && r.Turn == 0 && r.AskCount == 0, Input: r.History, Memory: r.Memory, Tools: tools, ImageTools: r.Config.ImageKeyID > 0 && (r.RuntimeVersion >= 2 || (r.ToolCount < 2 && !r.WorkPlanning)) && !r.StatusInquiry}
		workContext, contextErr := s.workContext(ctx, r.UserID, r.ConversationID)
		if contextErr != nil {
			s.fail(r, contextErr)
			return
		}
		in.WorkContext = workContext
		in.WorkPlanning = r.WorkPlanning
		in.LegacyContract = r.RuntimeVersion < 2
		stepStatus, eventType := "running", "model.started"
		if r.ReconcileDeadline != nil {
			r.Status, stepStatus, eventType = "reconciling", "reconciling", "operation.reconciling"
		}
		modelStep := r.startStep(in.OperationID, "reasoning", r.Config.Model, stepStatus, nil)
		var eventData any = map[string]any{"run_id": r.ID}
		if eventType == "operation.reconciling" {
			eventData = r.Public()
		}
		if e := s.Store.Save(ctx, r, eventType, eventData); e != nil {
			return
		}
		if r.StreamOperation != in.OperationID {
			r.StreamOperation = in.OperationID
			r.StreamPrefix = r.Text
			if r.StreamPrefix != "" {
				r.StreamPrefix += "\n\n"
			}
		}
		textPrefix := r.StreamPrefix
		lastProgress := time.Time{}
		in.OnProgress = func(update StreamUpdate) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			r.Text = textPrefix + update.Text
			modelStep.Summary = update.Summary
			modelStep.Commentary = update.Text
			if time.Since(lastProgress) < 750*time.Millisecond {
				return nil
			}
			lastProgress = time.Now()
			return s.Store.Save(ctx, r, "model.delta", r.Public())
		}
		op, out, e := s.operation(ctx, r, in, r.Turn)
		if e != nil {
			s.operationFailure(r, op, e)
			return
		}
		r.ReconcileDeadline = nil
		modelStep.Cost = op.Cost
		r.ModelAccountID = out.AccountID
		result, parseErr := ParseResponse(out.Body)
		s.syncCosts(r)
		if r.RuntimeVersion >= 2 && errors.Is(parseErr, ErrToolBatchLimit) {
			r.completeStep(in.OperationID, op.Cost, result.Summary, nil)
			s.yieldProgress(ctx, r, "本轮请求的工具步骤过多，尚未执行这些步骤。已有方案和结果已保留，可以继续沟通并分步处理。")
			return
		}
		if r.RuntimeVersion < 2 && parseErr == nil && len(result.Calls) > 1 {
			parseErr = Invalid("AGENT_TOO_MANY_TOOL_CALLS")
		}
		if parseErr != nil {
			s.fail(r, parseErr)
			return
		}
		if r.ForceAskUser && len(result.Calls) == 0 {
			s.fail(r, Invalid("AGENT_QUESTION_REQUIRED"))
			return
		}
		if result.Summary == "" {
			result.Summary = modelStep.Summary
		}
		r.completeStep(in.OperationID, op.Cost, result.Summary, nil)
		// Public stage commentary is distinct from reasoning summaries and the
		// final reply. Retain it only when this stage hands off to a tool.
		for i := range r.Trace {
			if r.Trace[i].ID == in.OperationID {
				r.Trace[i].Commentary = ""
				if len(result.Calls) > 0 {
					r.Trace[i].Commentary = result.Text
				}
			}
		}
		r.History = append(r.History, result.Items...)
		r.Turn++
		r.Text = textPrefix + result.Text
		if len(result.Calls) == 0 {
			r.Status = "completed"
			r.ErrorCode = ""
			_ = s.Store.Save(context.WithoutCancel(ctx), r, "run.completed", r.Public())
			return
		}
		if !tools {
			if r.RuntimeVersion >= 2 {
				s.yieldProgress(ctx, r)
				return
			}
			s.fail(r, Invalid("AGENT_TOOL_LIMIT"))
			return
		}
		// Tool handoffs are represented by the question/trace. They are not
		// completed answers, particularly when image execution later fails.
		r.Text = ""
		r.StreamOperation = ""
		r.StreamPrefix = ""
		calls := append([]Call(nil), result.Calls...)
		for i := range calls {
			calls[i].ToolID = "tc_" + Hash(op.ID + calls[i].ID)[:32]
		}
		r.PendingTools = &ToolBatch{OperationID: op.ID, Calls: calls}
		r.Status = "resuming_model"
		// Persist the full ordered batch before any tool side effect. Reclaiming
		// this run resumes stable call IDs instead of asking/charging the model again.
		if e = s.Store.Save(ctx, r, "model.completed", r.Public()); e != nil {
			return
		}

	}
	if r.RuntimeVersion >= 2 {
		s.yieldProgress(ctx, r)
		return
	}
	s.fail(r, Invalid("AGENT_MAX_TURNS"))
}

// processToolCall processes one durably admitted call, serially. false means
// the run paused for a user, terminated, or lost its persistence fence.
func (s *Service) processToolCall(ctx context.Context, r *Run, parentOperation string, call Call) bool {
	var e error
	if r.RuntimeVersion >= 2 && oneOf(call.Name, "update_draft", "present_review", "inspect_assets", "get_job", "finish_task") {
		value, err := s.adaptiveTool(ctx, r, call)
		if err != nil {
			if !s.returnToolFeedback(ctx, r, call, err) {
				return false
			}
			return true
		}
		if err = s.Store.Tool(ctx, r, parentOperation, call, json.RawMessage(call.Arguments)); err != nil {
			return false
		}
		r.startStep(call.ToolID, "tool", call.Name, "running", nil)
		r.completeStep(call.ToolID, "0.00000000", "工具结果已返回模型。", nil)
		r.History = append(r.History, toolResult(call, value))
		r.Status = "resuming_model"
		if err = s.Store.Save(ctx, r, "work.plan.updated", r.Public()); err != nil {
			return false
		}
		return true
	}

	if r.ForceAskUser && call.Name != "ask_user" {
		s.fail(r, Invalid("AGENT_QUESTION_REQUIRED"))
		return false
	}
	if call.Name == "save_work_plan" {
		p, err := s.proposeWork(ctx, r, call)
		if err != nil {
			if r.RuntimeVersion >= 2 {
				if !s.returnToolFeedback(ctx, r, call, err) {
					return false
				}
				return true
			}
			s.fail(r, err)
			return false
		}
		if err = s.Store.Tool(ctx, r, parentOperation, call, p.WorkDraft); err != nil {
			return false
		}
		r.startStep(call.ToolID, "tool", "save_work_plan", "running", nil)
		r.completeStep(call.ToolID, "0.00000000", "方案已保存，等待用户审核配置卡。", nil)
		r.History = append(r.History, jsonBytes(map[string]any{"type": "function_call_output", "call_id": call.ID, "output": string(jsonBytes(map[string]string{"plan_id": p.ID, "status": "draft", "approval": "human_required"}))}))
		if r.RuntimeVersion >= 2 {
			r.Status = "resuming_model"
			if err = s.Store.Save(ctx, r, "work.plan.created", r.Public()); err != nil {
				return false
			}
			return true
		}
		r.Text = "方案已整理成配置卡。你可以修改内容、规格和参考图，核价后确认生成，也可以继续沟通。"
		r.Status = "completed"
		r.ErrorCode = ""
		_ = s.Store.Save(context.WithoutCancel(ctx), r, "work.plan.created", r.Public())
		return false
	}
	if call.Name == "ask_user" {
		args, err := ParseAskUser([]byte(call.Arguments))
		if err != nil {
			if r.RuntimeVersion >= 2 {
				if !s.returnToolFeedback(ctx, r, call, invalidArguments("ask_user")) {
					return false
				}
				return true
			}
			s.fail(r, err)
			return false
		}
		if r.AskCount >= 6 {
			if r.RuntimeVersion >= 2 {
				s.yieldProgress(ctx, r)
				return false
			}
			s.fail(r, Invalid("AGENT_QUESTION_LIMIT"))
			return false
		}
		if err = s.Store.Tool(ctx, r, parentOperation, call, args); err != nil {
			return false
		}
		s.deferToolsUntilAnswer(r)
		r.AskCount++
		r.PendingCall = &call
		r.Question = &UserQuestion{ID: call.ToolID, AskUserArgs: args, Deadline: r.Deadline}
		step := r.startStep(call.ToolID, "tool", "ask_user", "waiting_user", nil)
		step.Summary = args.Question
		r.Status = "waiting_user"
		_ = s.Store.Save(ctx, r, "user.question", r.Public())
		return false
	}
	if r.StatusInquiry {
		if r.RuntimeVersion >= 2 {
			if !s.returnToolFeedback(ctx, r, call, Invalid("AGENT_IMAGE_AUTHORIZATION_REQUIRED")) {
				return false
			}
			return true
		}
		s.fail(r, Invalid("AGENT_IMAGE_AUTHORIZATION_REQUIRED"))
		return false
	}
	if r.WorkPlanning && r.RuntimeVersion < 2 {
		s.fail(r, Invalid("AGENT_WORK_REVIEW_REQUIRED"))
		return false
	}
	if r.Config.ImageKeyID == 0 || (r.RuntimeVersion < 2 && r.ToolCount >= 2) {
		if r.RuntimeVersion >= 2 {
			if !s.returnToolFeedback(ctx, r, call, Invalid("AGENT_TOOL_NOT_ALLOWED")) {
				return false
			}
			return true
		}
		s.fail(r, Invalid("AGENT_TOOL_NOT_ALLOWED"))
		return false
	}
	if call.Name != "generate_image" {
		if !s.returnToolFeedback(ctx, r, call, &ToolFeedback{Code: "TOOL_UNAVAILABLE", Category: "capability", Message: "工具不可用，请使用当前提供的工具或普通回复"}) {
			return false
		}
		return true
	}
	args, e := ParseTool(json.RawMessage(call.Arguments))
	if e != nil {
		if r.RuntimeVersion >= 2 {
			if !s.returnToolFeedback(ctx, r, call, invalidArguments("generate_image")) {
				return false
			}
			return true
		}
		s.fail(r, e)
		return false
	}
	if r.RuntimeVersion >= 2 && (args.TaskID == nil || *args.TaskID == "") {
		p, err := s.imageReviewCard(ctx, r, call, args)
		if err != nil {
			if !s.returnToolFeedback(ctx, r, call, err) {
				return false
			}
			return true
		}
		r.startStep(call.ToolID, "tool", call.Name, "running", traceArguments(args, r.Config.ImageModel))
		r.completeStep(call.ToolID, "0.00000000", "已提供生成配置卡，等待用户授权。", nil)
		r.History = append(r.History, toolResult(call, map[string]any{"ok": true, "dispatched": false, "approval": "human_required", "plan_id": p.ID, "revision": p.Revision, "note": "已展示配置卡，核价后批准或为本任务授予限额样张许可。尚未生图。"}))
		r.Status = "resuming_model"
		if e = s.Store.Save(ctx, r, "work.plan.created", r.Public()); e != nil {
			return false
		}
		return true
	}
	if e = s.Store.Tool(ctx, r, parentOperation, call, args); e != nil {
		return false
	}
	r.PendingCall = &call
	r.startStep(call.ToolID, "tool", call.Name, "queued", traceArguments(args, r.Config.ImageModel))
	r.Status = "waiting_tool"
	if e = s.Store.Save(ctx, r, "tool.call.created", map[string]any{"tool_call_id": call.ToolID, "tool_name": call.Name}); e != nil {
		return false
	}
	return true
}

func (s *Service) deferToolsUntilAnswer(r *Run) {
	if r.PendingTools == nil {
		return
	}
	for _, next := range r.PendingTools.Calls {
		r.History = append(r.History, toolResult(next, map[string]any{"ok": false, "code": "USER_INPUT_REQUIRED", "dispatched": false, "message": "先等待用户回答 ask_user，再依据答案重新选择工具；此调用未执行。"}))
		r.startStep(next.ToolID, "tool", next.Name, "running", nil)
		r.completeStep(next.ToolID, "0.00000000", "等待用户补充信息，此调用未执行。", nil)
	}
	r.PendingTools = nil
}

func (s *Service) operation(ctx context.Context, r *Run, in Invocation, step int) (Operation, Outcome, error) {
	defer s.syncCosts(r)
	if in.ProjectID == "" {
		in.ProjectID = r.TaskID
	}
	var empty Outcome
	if existing, err := s.Store.LookupOperation(ctx, in.OperationID); err == nil {
		if existing.Status == "failed" {
			return existing, empty, Invalid(existing.ErrorCode)
		}
		if existing.Status != "prepared" && !matchesExistingRequest(r, in, existing) {
			return existing, empty, ErrConflict
		}
		if existing.Status == "prepared" {
			goto prepareNew
		}
		if r.ReconcileDeadline != nil {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, *r.ReconcileDeadline)
			defer cancel()
		}
		evidence, readErr := s.readOutcome(existing.ID)
		if SafeCode(readErr) == "ASSET_EXPIRED" {
			return existing, empty, readErr
		}
		if readErr != nil {
			evidence = Outcome{Unknown: true, BillingPending: true}
		}
		if existing.Status == "succeeded" && readErr == nil && !(existing.Kind == "image" && evidence.ImageFromBody && len(evidence.Image) == 0) {
			evidence.Cost = existing.Cost
			evidence.BillingPending = false
			return existing, evidence, nil
		}
		if recovery, ok := s.Exec.(interface {
			Recover(context.Context, Invocation, Outcome) (Outcome, error)
		}); ok {
			evidence, err = recovery.Recover(ctx, in, evidence)
		}
		if evidence.confirmed() {
			existing.Cost = evidence.Cost
			if saveErr := s.Store.FinishOperation(context.WithoutCancel(ctx), r, existing); saveErr != nil {
				return existing, evidence, ErrUnknown
			}
		}
		var permanent *ResultFailure
		if errors.As(err, &permanent) {
			existing.Status = "failed"
			existing.ErrorCode = permanent.Code
			existing.Cost = evidence.Cost
			_ = s.Store.FinishOperation(context.WithoutCancel(ctx), r, existing)
			return existing, evidence, err
		}
		if err != nil || ctx.Err() != nil || evidence.BillingPending || evidence.Unknown {
			return existing, empty, ErrUnknown
		}
		if err = s.writeOutcome(existing.ID, evidence); err != nil {
			return existing, empty, ErrUnknown
		}
		existing.Status = "succeeded"
		existing.Cost = evidence.Cost
		err = s.Store.FinishOperation(ctx, r, existing)
		return existing, evidence, err
	} else if !errors.Is(err, ErrNotFound) {
		return Operation{}, empty, err
	}

prepareNew:
	group, e := s.Exec.Authorize(ctx, in.Binding, in.Kind, in.Model)
	if e != nil {
		return Operation{}, empty, e
	}
	if group != in.Binding.GroupID {
		return Operation{}, empty, Invalid("AGENT_KEY_BINDING_CHANGED")
	}
	if e = s.loadVisionInputs(ctx, r, &in); e != nil {
		return Operation{}, empty, e
	}
	quote, e := s.Exec.Quote(ctx, in)
	if e != nil {
		return Operation{}, empty, e
	}
	if in.ApprovedQuote != "" && !WithinBudget("0", quote, in.ApprovedQuote) {
		return Operation{}, empty, Invalid("AGENT_PRICING_CHANGED")
	}
	referenceIDs := []string{}
	for _, a := range in.References {
		referenceIDs = append(referenceIDs, a.ID)
	}
	requestHash := operationRequestHash(in)
	if r.RuntimeVersion < 2 {
		requestHash = Hash(in)
	}
	op, e := s.Store.Prepare(ctx, r, Operation{ID: in.OperationID, RunID: r.ID, Kind: in.Kind, Step: step, RequestHash: requestHash, Quote: quote, GrantID: in.GrantID, ProjectID: in.ProjectID, ReferenceIDs: referenceIDs})
	if e != nil {
		return op, empty, e
	}

	if op.Status != "prepared" {
		return op, empty, ErrUnknown
	}
	if e = s.reserveCheckpoint(ctx, r.UserID, op); e != nil {
		op.Status, op.Cost, op.ErrorCode = "failed", "0.00000000", SafeCode(e)
		_ = s.Store.FinishOperation(context.WithoutCancel(ctx), r, op)
		return op, empty, e
	}
	if e = s.Store.Dispatch(ctx, r, op); e != nil {
		return op, empty, e
	}
	op.Status = "dispatching"
	in.CostLimit = op.Quote
	out, e := s.Exec.Execute(ctx, in)
	if out.confirmed() {
		op.Cost = out.Cost
		if saveErr := s.Store.FinishOperation(context.WithoutCancel(ctx), r, op); saveErr != nil {
			return op, out, ErrUnknown
		}
	}
	if e != nil {
		out.Unknown = true
		_ = s.writeOutcome(op.ID, out)
		var failure *InvocationFailure
		if errors.As(e, &failure) && !failure.Dispatched {
			op.Status = "failed"
			op.Cost = "0.00000000"
			op.ErrorCode = failure.Code
			_ = s.Store.FinishOperation(context.WithoutCancel(ctx), r, op)
			return op, out, e
		}
		op.Status = "result_unknown"
		op.ErrorCode = SafeCode(e)
		_ = s.Store.FinishOperation(context.WithoutCancel(ctx), r, op)
		return op, out, ErrUnknown
	}
	if e = s.writeOutcome(op.ID, out); e != nil {
		return op, out, ErrUnknown
	}
	if recovery, ok := s.Exec.(interface {
		Recover(context.Context, Invocation, Outcome) (Outcome, error)
	}); ok {
		out, e = recovery.Recover(ctx, in, out)
		if out.confirmed() {
			op.Cost = out.Cost
			if saveErr := s.Store.FinishOperation(context.WithoutCancel(ctx), r, op); saveErr != nil {
				return op, out, ErrUnknown
			}
		}
		if e != nil {
			var permanent *ResultFailure
			if errors.As(e, &permanent) {
				op.Status = "failed"
				op.ErrorCode = permanent.Code
				op.Cost = out.Cost
				_ = s.Store.FinishOperation(context.WithoutCancel(ctx), r, op)
				return op, out, e
			}
			return op, out, ErrUnknown
		}
		if e = s.writeOutcome(op.ID, out); e != nil {
			return op, out, ErrUnknown
		}
	}

	if out.Unknown || out.BillingPending {
		return op, out, ErrUnknown
	}
	if _, e = Amount(out.Cost); e != nil {
		return op, out, ErrUnknown
	}
	op.Status = "succeeded"
	op.Cost = out.Cost
	op.Output = jsonBytes(map[string]string{"checkpoint": op.ID})
	e = s.Store.FinishOperation(context.WithoutCancel(ctx), r, op)
	if e == nil && in.Kind == "image" {
		out.Body = nil
	}
	return op, out, e
}
func (s *Service) operationFailure(r *Run, op Operation, e error) {
	if errors.Is(e, ErrUnknown) || op.Status == "dispatching" || op.Status == "result_unknown" {
		s.beginReconciliation(r, "UPSTREAM_RESULT_UNKNOWN")
		return
	}
	s.fail(r, e)
}
func (s *Service) fail(r *Run, e error) {
	s.syncCosts(r)
	r.Status = "failed"
	if len(r.AssetIDs) > 0 {
		r.Status = "partial_success"
	}
	r.ErrorCode = SafeCode(e)
	r.pendingSteps("failed", r.ErrorCode)
	_ = s.Store.Save(context.Background(), r, "run."+r.Status, r.Public())
}
func (s *Service) finishStopped(r *Run, cancelled bool) {
	if cancelled {
		r.Cancel = true
	}
	if r.Work != nil {
		if err := s.restoreStoppedWork(r); err != nil {
			s.beginReconciliation(r, "UPSTREAM_RESULT_UNKNOWN")
			return
		}
	}
	s.syncCosts(r)
	unsettled, err := s.Store.Unsettled(context.Background(), r.ID)
	if err != nil || unsettled {
		s.beginReconciliation(r, "UPSTREAM_RESULT_UNKNOWN")
		return
	}

	// Restore a confirmed paid result before finalizing a cancelled run. Reading
	// its existing checkpoint is not permission to dispatch another request.
	if r.Work == nil {
		kind := "text"
		step := r.Turn
		if r.PendingCall != nil {
			kind = "image"
			step = r.ToolCount
		}
		opID := r.ID + "_" + map[string]string{"text": "model", "image": "image"}[kind] + "_" + fmt.Sprint(step)
		if op, err := s.Store.LookupOperation(context.Background(), opID); err == nil && op.Status == "succeeded" {
			if out, readErr := s.readOutcome(opID); readErr == nil {
				if kind == "image" {
					if a, saveErr := s.saveImage(context.Background(), r, op, out); saveErr == nil {
						r.completeStep(r.PendingCall.ToolID, op.Cost, "", []string{a.ID})
						r.AssetIDs = append(r.AssetIDs, a.ID)
						s.syncCosts(r)
						r.PendingCall = nil
						r.ToolCount++
					} else {
						s.beginReconciliation(r, "AGENT_ASSET_STORAGE_FAILED")
						return
					}
				} else {
					s.syncCosts(r)
					r.Turn++
					if parsed, parseErr := ParseResponse(out.Body); parseErr == nil {
						r.completeStep(opID, op.Cost, parsed.Summary, nil)
						if parsed.Text != "" {
							r.Text += parsed.Text
						}
					}
				}
			} else {
				s.beginReconciliation(r, "AGENT_RESULT_STORAGE_FAILED")
				return
			}
		}

	}
	r.Status = "expired"
	if cancelled {
		r.Status = "cancelled"
	}
	if len(r.AssetIDs) > 0 {
		r.Status = "partial_success"
	}
	r.ErrorCode = ""
	stepStatus := "expired"
	if cancelled {
		stepStatus = "cancelled"
	}
	r.pendingSteps(stepStatus, "")
	_ = s.Store.Save(context.Background(), r, "run."+r.Status, r.Public())
}
func stringPtr(s string) *string { return &s }
func (s *Service) writeOutcome(id string, out Outcome) error {
	if out.ImageFromBody && len(out.Body) > 0 {
		out.Image = nil
	}
	if len(out.Body) > 0 && !json.Valid(out.Body) {
		return Invalid("AGENT_INVALID_CHECKPOINT")
	}
	// Serialize small metadata, then stream large JSON/Base64 without a duplicate encoder buffer.
	header, err := json.Marshal(map[string]any{"image_from_body": out.ImageFromBody, "AccountID": out.AccountID, "Cost": out.Cost, "BillingPending": out.BillingPending, "Unknown": out.Unknown, "MIME": out.MIME, "Width": out.Width, "Height": out.Height})
	if err != nil {
		return err
	}
	body := out.Body
	if len(body) == 0 {
		body = []byte("null")
	}
	prefix := append(header[:len(header)-1], []byte(`,"Body":`)...)
	imageSize := int64(4)
	if len(out.Image) > 0 {
		imageSize = int64(base64.StdEncoding.EncodedLen(len(out.Image)) + 2)
	}
	size := int64(len(prefix)+len(body)+len(`,"Image":`)+1) + imageSize
	return s.storeCheckpointStream(context.Background(), id, size, len(out.Body) > 0 || len(out.Image) > 0, func(w io.Writer) error {
		for _, part := range [][]byte{prefix, body, []byte(`,"Image":`)} {
			if _, err := w.Write(part); err != nil {
				return err
			}
		}
		if len(out.Image) > 0 {
			if _, err := io.WriteString(w, `"`); err != nil {
				return err
			}
			encoder := base64.NewEncoder(base64.StdEncoding, w)
			if _, err := encoder.Write(out.Image); err != nil {
				return err
			}
			if err := encoder.Close(); err != nil {
				return err
			}
			if _, err := io.WriteString(w, `"`); err != nil {
				return err
			}
		} else if _, err := io.WriteString(w, "null"); err != nil {
			return err
		}
		_, err := io.WriteString(w, "}")
		return err
	})
}
func (s *Service) readOutcome(id string) (Outcome, error) {
	var out Outcome
	if s.Store != nil {
		if err := s.checkpointReadable(context.Background(), id); err != nil {
			return out, err
		}
	}
	b, e := os.ReadFile(filepath.Join(s.Root, filepath.Base(id)+".result.json"))
	if e == nil {
		e = json.Unmarshal(b, &out)
	}
	return out, e
}
func (s *Service) writePrivate(key string, b []byte) error {
	return s.writePrivateStream(key, func(w io.Writer) error { _, err := w.Write(b); return err })
}
func (s *Service) writePrivateStream(key string, write func(io.Writer) error) error {
	if key != filepath.Base(key) {
		return ErrNotFound
	}
	if e := os.MkdirAll(s.Root, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(s.Root, ".agent-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		e = write(f)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(name, filepath.Join(s.Root, key))
}
func imageInfo(data []byte) (string, int, int, error) {
	if len(data) == 0 || len(data) > agentlimits.GeneratedImageBytes {
		return "", 0, 0, errors.New("invalid image size")
	}
	cfg, format, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > agentlimits.ImageSide || cfg.Height > agentlimits.ImageSide || int64(cfg.Width)*int64(cfg.Height) > agentlimits.ImagePixels {
		return "", 0, 0, errors.New("invalid image")
	}
	if !oneOf(format, "png", "jpeg", "webp") {
		return "", 0, 0, errors.New("unsupported image")
	}
	return "image/" + format, cfg.Width, cfg.Height, nil
}
func (s *Service) Upload(ctx context.Context, user int64, conv string, b []byte) (Asset, error) {
	return s.UploadFile(ctx, user, conv, "reference.png", b)
}
func (s *Service) saveImage(ctx context.Context, r *Run, op Operation, out Outcome) (Asset, error) {
	id := "asset_" + Hash(op.ID)[:32]
	if existing, e := s.Store.AssetMeta(ctx, r.UserID, id); e == nil {
		if !existing.ContentAvailable {
			return existing, Invalid("ASSET_EXPIRED")
		}
		return existing, nil
	}
	return s.persistImage(ctx, r.UserID, r.ConversationID, r.ID, op.ID, id, out.Image)
}
func (s *Service) persistImage(ctx context.Context, user int64, conv, run, op, id string, b []byte) (Asset, error) {
	mime, w, h, e := imageInfo(b)
	if e != nil {
		return Asset{}, e
	}
	expiry := time.Now().Add(ContentRetention)
	if s.Store != nil && op != "" {
		var stored time.Time
		err := s.Store.DB.QueryRowContext(ctx, `SELECT content_expires_at FROM agent_checkpoints WHERE operation_id=$1 AND purged_at IS NULL AND content_expires_at>now()`, op).Scan(&stored)
		if err != nil {
			return Asset{}, Invalid("ASSET_EXPIRED")
		}
		expiry = stored
	}
	a := Asset{FileName: id + "." + strings.TrimPrefix(mime, "image/"), ID: id, UserID: user, ConversationID: conv, RunID: run, Key: id + ".image", MIME: mime, Size: int64(len(b)), Width: w, Height: h, SHA: HashBytes(b), CreatedAt: time.Now().UTC(), Expires: expiry, Status: "ready", ContentAvailable: true}
	if e = s.writePrivate(a.Key, b); e != nil {
		return a, e
	}
	if e = s.Store.PutAsset(ctx, a, op); e != nil {
		// This is a newly written, unregistered asset only, never a user backup.
		_ = os.Remove(filepath.Join(s.Root, a.Key))
		return a, e
	}
	return a, nil
}
func (s *Service) LoadAsset(ctx context.Context, user int64, id string) (Asset, error) {
	a, err := s.Store.Asset(ctx, user, id)
	if err != nil {
		return a, err
	}
	return s.readAsset(a)
}
func (s *Service) loadAssetForRun(ctx context.Context, user int64, id, run string) (Asset, error) {
	a, err := s.Store.assetForRun(ctx, user, id, run)
	if err != nil {
		return a, err
	}
	return s.readAsset(a)
}
func (s *Service) readAsset(a Asset) (Asset, error) {
	if a.Key != filepath.Base(a.Key) || (a.Key != a.ID+".image" && a.Key != a.ID+".attachment") {
		return a, ErrNotFound
	}
	var err error
	a.Data, err = os.ReadFile(filepath.Join(s.Root, a.Key))
	if err == nil && HashBytes(a.Data) != a.SHA {
		err = Invalid("AGENT_ASSET_INTEGRITY_FAILED")
	}
	return a, err
}

func (s *Service) Accepting() bool           { return s.Enabled && !s.startupFailed.Load() }
func (s *Service) CleanupLastSuccess() int64 { return s.cleanupAt.Load() }
