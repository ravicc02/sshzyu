package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type FieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}
type ToolFeedback struct {
	Code     string       `json:"code"`
	Category string       `json:"category"`
	Message  string       `json:"message"`
	Fields   []FieldError `json:"field_errors,omitempty"`
}

func (e *ToolFeedback) Error() string { return e.Code }
func allowedTool(name string) bool {
	return oneOf(name, "generate_image", "ask_user", "save_work_plan", "update_draft", "present_review", "inspect_assets", "get_job", "finish_task")
}
func functionDefinition(name, description string, properties map[string]any) json.RawMessage {
	required := []string{}
	for k := range properties {
		required = append(required, k)
	}
	// Stable encoding including required order is important for quote/request hashes.
	sortStrings(required)
	return jsonBytes(map[string]any{"type": "function", "name": name, "description": description, "strict": true, "parameters": map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}})
}
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
func draftProperties() map[string]any {
	str := map[string]any{"type": "string"}
	refs := map[string]any{"type": "array", "items": str, "maxItems": 8}
	item := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"id": str, "title": str, "content": str, "prompt": str, "reference_asset_ids": refs}, "required": []string{"id", "title", "content", "prompt", "reference_asset_ids"}}
	return map[string]any{"title": str, "brief": str, "style": str, "image_size": map[string]any{"type": "string", "enum": []string{"", "1K", "2K", "4K"}}, "aspect_ratio": map[string]any{"type": "string", "enum": []string{"", "1:1", "16:9", "9:16", "4:3", "3:4"}}, "shared_reference_asset_ids": refs, "items": map[string]any{"type": "array", "items": item, "maxItems": MaxWorkItems}}
}
func draftDefinition(name, description string) json.RawMessage {
	return functionDefinition(name, description, draftProperties())
}
func AdaptiveToolDefinitions() []json.RawMessage {
	props := draftProperties()
	for k, v := range props { // null means keep; [] explicitly clears a list.
		obj := v.(map[string]any)
		obj = cloneObject(obj)
		obj["type"] = []string{obj["type"].(string), "null"}
		if enum, ok := obj["enum"].([]string); ok {
			vals := []any{nil}
			for _, s := range enum {
				vals = append(vals, s)
			}
			obj["enum"] = vals
		}
		props[k] = obj
	}
	props["remove_item_ids"] = map[string]any{"type": []string{"array", "null"}, "items": map[string]any{"type": "string"}, "maxItems": MaxWorkItems}
	required := []string{}
	for k := range props {
		required = append(required, k)
	}
	sortStrings(required)
	patch := map[string]any{"type": "object", "additionalProperties": false, "properties": props, "required": required}
	return []json.RawMessage{
		functionDefinition("update_draft", "增量保存目标、风格、素材和逐项内容。plan_id=null创建项目，revision=null；修改时使用真实plan_id及revision。null字段保持不变，items按稳定id合并，remove_item_ids明确删除。草稿允许不完整，无参考图、10页大纲都合法。不会付费或自批执行。", map[string]any{"plan_id": map[string]any{"type": []string{"string", "null"}}, "revision": map[string]any{"type": []string{"integer", "null"}}, "patch": patch}),
		functionDefinition("present_review", "展示已有草稿的配置卡和缺失字段；用户可以修改、核价、批准或继续讨论。未完成的草稿也可展示，不批准费用。", map[string]any{"plan_id": map[string]any{"type": "string"}}),
		functionDefinition("inspect_assets", "查看当前会话素材元数据及是否过期，不读取像素、不进行OCR或Office解析。observed_by_model=false；只有本轮真实附带的视觉输入才能声明看过。过期素材须重新上传。", map[string]any{"asset_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 24}}),
		functionDefinition("finish_task", "结束当前项目的自主执行许可。仅在目标已完成或用户取消时调用；关闭所有剩余许可，不更改已有结果、未知操作或账本，不批准新的费用。", map[string]any{"plan_id": map[string]any{"type": "string"}}),
		functionDefinition("get_job", "查询已有审核稿与后台作业的结果和状态，不生成、不重发。长作业由后台推进，无需循环轮询。", map[string]any{"plan_id": map[string]any{"type": "string"}}),
	}
}
func cloneObject(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

type DraftPatch struct {
	Title            *string     `json:"title"`
	Brief            *string     `json:"brief"`
	Style            *string     `json:"style"`
	Size             *string     `json:"image_size"`
	Aspect           *string     `json:"aspect_ratio"`
	SharedReferences *[]string   `json:"shared_reference_asset_ids"`
	Items            *[]WorkItem `json:"items"`
	Remove           []string    `json:"remove_item_ids"`
}
type DraftUpdate struct {
	PlanID   *string    `json:"plan_id"`
	Revision *int       `json:"revision"`
	Patch    DraftPatch `json:"patch"`
}

func (d DraftPatch) apply(p WorkDraft) WorkDraft {
	if d.Title != nil {
		p.Title = *d.Title
	}
	if d.Brief != nil {
		p.Brief = *d.Brief
	}
	if d.Style != nil {
		p.Style = *d.Style
	}
	if d.Size != nil {
		p.Size = *d.Size
	}
	if d.Aspect != nil {
		p.Aspect = *d.Aspect
	}
	if d.SharedReferences != nil {
		p.SharedReferences = *d.SharedReferences
	}
	items := append([]WorkItem{}, p.Items...)
	if d.Items != nil {
		for _, item := range *d.Items {
			found := false
			for i, old := range items {
				if item.ID == old.ID {
					items[i] = item
					found = true
					break
				}
			}
			if !found {
				items = append(items, item)
			}
		}
	}
	remove := map[string]bool{}
	for _, id := range d.Remove {
		remove[id] = true
	}
	p.Items = []WorkItem{}
	for _, item := range items {
		if !remove[item.ID] {
			p.Items = append(p.Items, item)
		}
	}
	return p
}
func normalizeDraft(p *WorkDraft) {
	if p.Items == nil {
		p.Items = []WorkItem{}
	}
	if p.SharedReferences == nil {
		p.SharedReferences = []string{}
	}
	for i := range p.Items {
		if p.Items[i].References == nil {
			p.Items[i].References = []string{}
		}
	}
}

type draftQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Service) validateDraftAssets(ctx context.Context, user int64, conv string, p WorkDraft) error {
	return validateDraftAssetsQuery(ctx, s.Store.DB, user, conv, p)
}
func validateDraftAssetsQuery(ctx context.Context, q draftQuery, user int64, conv string, p WorkDraft) error {
	if err := p.ValidateDraft(); err != nil {
		return err
	}
	ids := append([]string{}, p.SharedReferences...)
	for _, item := range p.Items {
		ids = append(ids, item.References...)
	}
	for _, id := range ids {
		var found bool
		if err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_assets a JOIN agent_conversations c ON c.id=a.conversation_id WHERE a.id=$1 AND a.user_id=$2 AND a.conversation_id=$3 AND a.mime_type LIKE 'image/%' AND c.user_id=$2 AND c.deleted_at IS NULL)`, id, user, conv).Scan(&found); err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
	}
	return nil
}
func (s *Service) updateDraft(ctx context.Context, r *Run, call Call) (WorkPlan, error) {
	var in DraftUpdate
	if err := StrictJSON([]byte(call.Arguments), &in); err != nil {
		return WorkPlan{}, invalidArguments("patch")
	}
	if len(in.Patch.Remove) > MaxWorkItems {
		return WorkPlan{}, invalidArguments("remove_item_ids")
	}
	// Reject duplicate upserts rather than silently taking the last.
	if in.Patch.Items != nil {
		seen := map[string]bool{}
		for _, item := range *in.Patch.Items {
			if seen[item.ID] {
				return WorkPlan{}, invalidArguments("patch.items.id")
			}
			seen[item.ID] = true
		}
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return WorkPlan{}, err
	}
	defer tx.Rollback()
	var owner int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, r.UserID).Scan(&owner); err != nil {
		return WorkPlan{}, err
	}
	var saved []byte
	var argumentsHash string
	err = tx.QueryRowContext(ctx, `SELECT snapshot,arguments_hash FROM agent_draft_mutations WHERE tool_id=$1 AND user_id=$2`, call.ToolID, r.UserID).Scan(&saved, &argumentsHash)
	if err == nil {
		var p WorkPlan
		err = json.Unmarshal(saved, &p)
		if err != nil {
			return p, err
		}
		if argumentsHash != Hash(in) || p.ConversationID != r.ConversationID {
			return p, ErrConflict
		}
		r.TaskID = p.ID
		return p, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return WorkPlan{}, err
	}
	// A stale worker may not mutate a plan after losing its fence.
	var fence int64
	if err = tx.QueryRowContext(ctx, `SELECT fence_token FROM agent_runs WHERE id=$1 AND user_id=$2 AND owner=$3 AND lease_until>now() FOR UPDATE`, r.ID, r.UserID, r.Owner).Scan(&fence); err != nil || fence != r.Fence {
		return WorkPlan{}, ErrConflict
	}
	p := WorkPlan{ID: "plan_" + Hash(r.ID + call.ID)[:32], ConversationID: r.ConversationID, SourceRunID: r.ID, Revision: 1, Status: "draft", ImageKeyID: r.Config.ImageKeyID, ImageModel: r.Config.ImageModel, ConfigVersion: r.ConfigVersion, Quote: "0.00000000", ItemQuotes: []string{}, WorkDraft: WorkDraft{Items: []WorkItem{}, SharedReferences: []string{}}}
	create := in.PlanID == nil || *in.PlanID == ""
	if !create {
		if in.Revision == nil {
			return p, invalidArguments("revision")
		}
		var priorRunStatus string
		var priorResults []byte
		err = tx.QueryRowContext(ctx, `SELECT p.snapshot,p.revision,p.status,coalesce(r.status,''),coalesce(r.snapshot->'work'->'results','[]'::jsonb) FROM agent_work_plans p LEFT JOIN agent_runs r ON r.id=p.run_id WHERE p.id=$1 AND p.user_id=$2 AND p.conversation_id=$3 FOR UPDATE OF p`, *in.PlanID, r.UserID, r.ConversationID).Scan(&saved, &p.Revision, &p.Status, &priorRunStatus, &priorResults)
		if errors.Is(err, sql.ErrNoRows) {
			return p, ErrNotFound
		}
		if err != nil {
			return p, err
		}
		rev, status := p.Revision, p.Status
		if err = json.Unmarshal(saved, &p); err != nil {
			return p, err
		}
		p.Revision, p.Status = rev, status
		if rev != *in.Revision || (!oneOf(status, "draft", "ready") && !(status == "submitted" && oneOf(priorRunStatus, "completed", "partial_success", "failed", "cancelled", "expired"))) {
			return p, &ToolFeedback{Code: "DRAFT_REVISION_CONFLICT", Category: "conflict", Message: "草稿修订已变化或该版本已提交，请get_job后更新可编辑修订；执行中的已批准清单不可修改"}
		}
		if status == "submitted" {
			if err = json.Unmarshal(priorResults, &p.InheritedResults); err != nil {
				return p, err
			}
		}
		p.Revision++
	}
	before := p.WorkDraft
	p.WorkDraft = in.Patch.apply(p.WorkDraft)
	normalizeDraft(&p.WorkDraft)
	if err = validateDraftAssetsQuery(ctx, tx, r.UserID, r.ConversationID, p.WorkDraft); err != nil {
		return p, err
	}
	p.SourceRunID = r.ID
	p.Status = "draft"
	p.QuotedAt = nil
	p.Quote = "0.00000000"
	p.ItemQuotes = []string{}
	p.ChangedItems = changedItems(before, p.WorkDraft)
	p.InheritedResults = unchangedResults(p.InheritedResults, p.ChangedItems)
	p.Results = p.InheritedResults
	p.RunID = ""
	p.ExecutionStatus = ""

	if create {
		_, err = tx.ExecContext(ctx, `INSERT INTO agent_work_plans(id,user_id,conversation_id,source_run_id,revision,status,snapshot) VALUES($1,$2,$3,$4,$5,'draft',$6)`, p.ID, r.UserID, r.ConversationID, r.ID, p.Revision, jsonBytes(p))
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE agent_work_plans SET source_run_id=$1,revision=$2,status='draft',run_id=NULL,snapshot=$3,updated_at=now() WHERE id=$4`, r.ID, p.Revision, jsonBytes(p), p.ID)
	}
	if err != nil {
		return p, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_draft_mutations(tool_id,user_id,plan_id,revision,snapshot,arguments_hash) VALUES($1,$2,$3,$4,$5,$6)`, call.ToolID, r.UserID, p.ID, p.Revision, jsonBytes(p), Hash(in))
	if err != nil {
		return p, err
	}
	if err = linkProjectRun(ctx, tx, p.ID, r.UserID, r.ID); err != nil {
		return p, err
	}
	if err = tx.Commit(); err != nil {
		return p, err
	}
	r.TaskID = p.ID
	return p, nil
}
func changedItems(a, b WorkDraft) []string {
	out := []string{}
	prior := map[string]string{}
	global := a.Style != b.Style || a.Brief != b.Brief || a.Size != b.Size || a.Aspect != b.Aspect || Hash(a.SharedReferences) != Hash(b.SharedReferences)
	for _, item := range a.Items {
		prior[item.ID] = Hash(item)
	}
	for _, item := range b.Items {
		if global || prior[item.ID] != Hash(item) {
			out = append(out, item.ID)
		}
		delete(prior, item.ID)
	}
	for id := range prior {
		out = append(out, id)
	}
	sortStrings(out)
	return out
}
func invalidArguments(path string) error {
	return &ToolFeedback{Code: "TOOL_ARGUMENTS_INVALID", Category: "validation", Message: "工具参数格式不符合契约，请修正参数、询问用户或说明缺失信息", Fields: []FieldError{{Path: path, Message: "检查工具定义中的类型、允许值和必填字段"}}}
}
func (s *Service) adaptiveTool(ctx context.Context, r *Run, call Call) (any, error) {
	switch call.Name {
	case "finish_task":
		var in struct {
			PlanID string `json:"plan_id"`
		}
		if StrictJSON([]byte(call.Arguments), &in) != nil {
			return nil, invalidArguments("plan_id")
		}
		return s.finishTask(ctx, r, in.PlanID)
	case "update_draft":
		p, err := s.updateDraft(ctx, r, call)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "plan_id": p.ID, "revision": p.Revision, "draft": p.WorkDraft, "changed_item_ids": p.ChangedItems, "missing_fields": p.MissingFields(), "approval": "human_required"}, nil
	case "present_review", "get_job":
		var in struct {
			PlanID string `json:"plan_id"`
		}
		if StrictJSON([]byte(call.Arguments), &in) != nil {
			return nil, invalidArguments("plan_id")
		}
		p, err := s.Store.WorkPlan(ctx, r.UserID, in.PlanID)
		if err != nil {
			return nil, err
		}
		if p.ConversationID != r.ConversationID {
			return nil, ErrNotFound
		}
		plans, err := s.Store.WorkPlans(ctx, r.UserID, r.ConversationID)
		if err != nil {
			return nil, err
		}
		for _, plan := range plans {
			if plan.ID == p.ID {
				p = plan
			}
		}
		approval := "human_required"
		if p.Status != "draft" && p.Status != "ready" {
			approval = "not_pending"
		}
		presentation := "not_requested"
		if call.Name == "present_review" {
			presentation = "requested"
		}
		return map[string]any{"ok": true, "plan": p, "missing_fields": p.MissingFields(), "approval": approval, "presentation": presentation, "dispatched": false, "observed_by_model": false}, nil
	case "inspect_assets":
		var in struct {
			IDs []string `json:"asset_ids"`
		}
		if StrictJSON([]byte(call.Arguments), &in) != nil || len(in.IDs) > 24 {
			return nil, invalidArguments("asset_ids")
		}
		assets := []Asset{}
		for _, id := range in.IDs {
			a, err := s.Store.AssetMeta(ctx, r.UserID, id)
			if err != nil || a.ConversationID != r.ConversationID {
				return nil, ErrNotFound
			}
			assets = append(assets, a)
		}
		return map[string]any{"ok": true, "assets": assets, "observed_by_model": false, "note": "仅元数据。content_available=false的原文件无法再读取，需重新上传；历史摘要不代表重新读取。"}, nil
	}
	return nil, &ToolFeedback{Code: "TOOL_UNAVAILABLE", Category: "capability", Message: "该工具不可用，请使用当前提供的工具或普通回复"}
}
func toolResult(call Call, result any) json.RawMessage {
	return jsonBytes(map[string]any{"type": "function_call_output", "call_id": call.ID, "output": string(jsonBytes(result))})
}
func feedbackFor(err error) (*ToolFeedback, bool) {
	var f *ToolFeedback
	if errors.As(err, &f) {
		return f, true
	}
	code := SafeCode(err)
	switch code {
	case "AGENT_NOT_FOUND":
		return &ToolFeedback{Code: "ASSET_NOT_AVAILABLE", Category: "permission", Message: "资源不存在或不属于当前用户/会话，请使用真实已关联资源"}, true
	case "ASSET_EXPIRED":
		return &ToolFeedback{Code: code, Category: "expired", Message: "原文件已过24小时，不能读取。请用户重新上传；只能参考历史摘要，不能声称重新看过文件"}, true
	case "AGENT_TOOL_ARGUMENTS_INVALID", "AGENT_QUESTION_INVALID", "AGENT_QUESTION_LIMIT", "AGENT_TOOL_NOT_ALLOWED", "AGENT_MODEL_NOT_ALLOWED", "AGENT_IMAGE_AUTHORIZATION_REQUIRED", "AGENT_WORK_REVIEW_REQUIRED", "AGENT_UNBOUNDED_PRICING", "AGENT_STORAGE_QUOTA_EXCEEDED", "AGENT_GRANT_REQUIRED", "AGENT_GRANT_EXHAUSTED", "AGENT_KEY_BINDING_CHANGED", "AGENT_PRICING_CHANGED":
		return &ToolFeedback{Code: code, Category: "configuration", Message: "当前能力或授权不足。尚未派发生图，可向用户说明、调整草稿或请求在配置卡授权"}, true
	}
	if errors.Is(err, ErrNotFound) {
		return &ToolFeedback{Code: "ASSET_NOT_AVAILABLE", Category: "permission", Message: "资源不可用，请使用当前会话属于用户的真实资源"}, true
	}
	return nil, false
}
func (s *Service) returnToolFeedback(ctx context.Context, r *Run, call Call, err error) bool {
	f, ok := feedbackFor(err)
	if !ok {
		s.fail(r, err)
		return false
	}
	r.ToolErrors++
	signature := f.Code + ":" + call.Name + ":" + call.Arguments
	if signature == r.LastToolError {
		r.RepeatedToolErrors++
	} else {
		r.RepeatedToolErrors = 1
		r.LastToolError = signature
	}
	r.PendingCall = nil
	r.startStep(call.ToolID, "tool", call.Name, "running", nil)
	r.completeStep(call.ToolID, "0.00000000", f.Message, nil)
	r.History = append(r.History, toolResult(call, map[string]any{"ok": false, "error": f, "execution": map[string]any{"dispatched": false, "image_cost_usd": "0.00000000"}, "recoverable": true, "suggested_actions": []string{"repair_arguments", "ask_user", "respond"}}))
	r.Status = "resuming_model"
	if r.ToolErrors >= 6 || r.RepeatedToolErrors >= 3 {
		r.Status = "completed"
		r.Text = "当前操作仍有待补充或调整的信息：" + f.Message + "。已有草稿和结果已保留，你可以继续沟通后再执行。"
		r.ErrorCode = ""
	}
	if e := s.Store.Save(ctx, r, "tool.feedback", r.Public()); e != nil {
		return false
	}
	return !r.Terminal()
}
func (s *Service) yieldProgress(ctx context.Context, r *Run, message ...string) {
	r.Status = "completed"
	r.ErrorCode = ""
	r.Text = "本轮处理已达到安全上限，已有草稿和结果已保留。请继续告诉我需要补充或调整的内容。"
	if len(message) > 0 {
		r.Text = message[0]
	}
	_ = s.Store.Save(context.WithoutCancel(ctx), r, "run.completed", r.Public())
}

// SaveDraft is a human edit, separate from the strict quotation/approval endpoint.
func (s *Service) SaveDraft(ctx context.Context, user int64, id string, rev int, draft WorkDraft) (WorkPlan, error) {
	if !s.Enabled {
		return WorkPlan{}, ErrDisabled
	}
	normalizeDraft(&draft)
	p, err := s.Store.WorkPlan(ctx, user, id)
	if err != nil {
		return p, err
	}
	if err = s.validateDraftAssets(ctx, user, p.ConversationID, draft); err != nil {
		return p, err
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return p, err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, user).Scan(&user); err != nil {
		return p, err
	}
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM agent_runs WHERE user_id=$1 AND status NOT IN ('completed','partial_success','failed','cancelled','expired')`, user).Scan(&active); err != nil {
		return p, err
	}
	if active > 0 {
		return p, ErrBusy
	}
	p.ChangedItems = changedItems(p.WorkDraft, draft)
	p.InheritedResults = unchangedResults(p.InheritedResults, p.ChangedItems)
	p.WorkDraft = draft
	p.Status = "draft"
	p.Revision = rev + 1
	p.Quote = "0.00000000"
	p.ItemQuotes = []string{}
	p.QuotedAt = nil
	res, err := tx.ExecContext(ctx, `UPDATE agent_work_plans p SET snapshot=$1,revision=$2,status='draft',updated_at=now() FROM agent_conversations c WHERE p.id=$3 AND p.user_id=$4 AND p.revision=$5 AND p.status IN ('draft','ready') AND c.id=p.conversation_id AND c.user_id=$4 AND c.deleted_at IS NULL`, jsonBytes(p), p.Revision, id, user, rev)
	if err != nil {
		return p, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return p, ErrConflict
	}
	return p, tx.Commit()
}

func (s *Service) finishTask(ctx context.Context, r *Run, plan string) (any, error) {
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var user int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, r.UserID).Scan(&user); err != nil {
		return nil, err
	}
	var valid bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_work_plans p JOIN agent_runs r ON r.conversation_id=p.conversation_id WHERE p.id=$1 AND p.user_id=$2 AND r.id=$3 AND r.user_id=$2 AND r.owner=$4 AND r.fence_token=$5 AND r.lease_until>now())`, plan, r.UserID, r.ID, r.Owner, r.Fence).Scan(&valid); err != nil {
		return nil, err
	}
	if !valid {
		return nil, ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `UPDATE agent_execution_grants SET status='closed' WHERE plan_id=$1 AND user_id=$2`, plan, r.UserID); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE agent_work_plans SET snapshot=jsonb_set(snapshot,'{task_state}','"closed"'::jsonb),updated_at=now() WHERE id=$1 AND user_id=$2`, plan, r.UserID); err != nil {
		return nil, err
	}
	if err = linkProjectRun(ctx, tx, plan, r.UserID, r.ID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	r.TaskID = plan
	return map[string]any{"ok": true, "plan_id": plan, "task_state": "closed", "execution_grants": "closed", "note": "已有产物、未知操作和计费证据保留。后续生图需要真人重新授权；结束任务不会把未知操作标成成功。"}, nil
}
