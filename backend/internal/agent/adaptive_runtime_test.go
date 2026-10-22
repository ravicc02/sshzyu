package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type adaptiveExecutor struct {
	testExecutor
	turns  int
	choose func(Invocation, int) Outcome
	inputs []Invocation
}

func (f *adaptiveExecutor) Execute(ctx context.Context, in Invocation) (Outcome, error) {
	f.inputs = append(f.inputs, in)
	if in.Kind == "image" {
		return f.testExecutor.Execute(ctx, in)
	}
	f.turns++
	if f.choose != nil {
		return f.choose(in, f.turns), nil
	}
	return adaptiveReply("请告诉我想讨论的内容。"), nil
}
func adaptiveReply(text string) Outcome {
	return Outcome{Cost: "0.01000000", Body: jsonBytes(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": text}}}}})}
}
func adaptiveCall(id, name string, args any) Outcome {
	return Outcome{Cost: "0.01000000", Body: jsonBytes(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "function_call", "call_id": id, "name": name, "arguments": string(jsonBytes(args))}}})}
}
func newAdaptive(t *testing.T) (*Service, *adaptiveExecutor, Conversation) {
	t.Helper()
	st := isolatedStore(t)
	f := &adaptiveExecutor{}
	s := New(st, f, t.TempDir(), true)
	s.WorkConcurrency = 1
	c, err := s.Create(context.Background(), 1, "动态任务", Config{ModelKeyID: 1, Model: "text", ImageKeyID: 2, ImageModel: "image", Budget: "5"}, "")
	if err != nil {
		t.Fatal(err)
	}
	return s, f, c
}
func adaptiveTurn(t *testing.T, s *Service, c Conversation, key, text string) *Run {
	t.Helper()
	ctx := context.Background()
	_, err := s.Submit(ctx, 1, c.ID, key, "", text, nil, c.Version, "5")
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Store.Claim(ctx, "adaptive-worker")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	if err = s.Store.Release(ctx, r); err != nil {
		t.Fatal(err)
	}
	return r
}
func TestAgentAdaptiveDraftShapeAndDefinitions(t *testing.T) {
	draft := workDraft(10)
	draft.Size = ""
	draft.Aspect = ""
	for i := range draft.Items {
		draft.Items[i].Prompt = ""
	}
	if err := draft.ValidateDraft(); err != nil {
		t.Fatal("10-page outline rejected", err)
	}
	if SafeCode(draft.Validate()) != "DRAFT_INCOMPLETE" {
		t.Fatal("incomplete execution compiled")
	}
	draft.Size = "3K"
	var feedback *ToolFeedback
	if !errors.As(draft.ValidateDraft(), &feedback) || feedback.Fields[0].Path != "image_size" {
		t.Fatal("no actionable size feedback")
	}
	defs := append(AdaptiveToolDefinitions(), WorkPlanDefinition(), ToolDefinition())
	for _, raw := range defs {
		var v map[string]any
		if json.Unmarshal(raw, &v) != nil {
			t.Fatal("invalid tool schema")
		}
		parameters := v["parameters"].(map[string]any)
		if len(parameters["required"].([]any)) != len(parameters["properties"].(map[string]any)) {
			t.Fatal("incomplete strict schema")
		}
	}
}
func TestPostgresAdaptiveTenPageDraftFeedbackReviewAndChunks(t *testing.T) {
	s, f, c := newAdaptive(t)
	ctx := context.Background()
	outline := workDraft(10)
	outline.Size = ""
	for i := range outline.Items {
		outline.Items[i].Prompt = ""
	}
	f.choose = func(in Invocation, n int) Outcome {
		switch n {
		case 1:
			bad := outline
			bad.Size = "3K"
			return adaptiveCall("bad", "save_work_plan", bad)
		case 2:
			if !strings.Contains(string(jsonBytes(in.Input)), `DRAFT_FIELD_INVALID`) || !strings.Contains(string(jsonBytes(in.Input)), `"call_id":"bad"`) {
				t.Error("tool error/call ID not returned")
			}
			return adaptiveCall("draft", "save_work_plan", outline)
		default:
			return adaptiveReply("十页大纲已保存，我们可以继续讨论风格，不需要参考图。")
		}
	}
	r := adaptiveTurn(t, s, c, "outline", "做10页PPT，没有参考图，先保存大纲")
	if r.Status != "completed" || r.ErrorCode != "" || f.images != 0 {
		t.Fatalf("%s %s images=%d", r.Status, r.ErrorCode, f.images)
	}
	plans, err := s.Store.WorkPlans(ctx, 1, c.ID)
	if err != nil || len(plans) != 1 || len(plans[0].Items) != 10 {
		t.Fatal("outline lost", len(plans), err)
	}
	p := plans[0]
	if _, err = s.ReviewWork(ctx, 1, p.ID, "", p.Revision, p.WorkDraft); SafeCode(err) != "DRAFT_INCOMPLETE" {
		t.Fatal("incomplete draft dispatched", err)
	}
	p, err = s.SaveDraft(ctx, 1, p.ID, p.Revision, workDraft(10))
	if err != nil {
		t.Fatal(err)
	}
	ready, err := s.ReviewWork(ctx, 1, p.ID, "", p.Revision, p.WorkDraft)
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.SubmitWork(ctx, 1, p.ID, "", ready.Revision)
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.Store.Claim(ctx, "render")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, run)
	if run.Work.Next != 8 || run.Status != "queued" || f.images != 8 {
		t.Fatalf("chunk %d %s images=%d error=%s", run.Work.Next, run.Status, f.images, run.ErrorCode)
	}
	if err = s.Store.Release(ctx, run); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.DB.ExecContext(ctx, `UPDATE agent_task_outbox SET ready_at=now() WHERE run_id=$1`, job.ID); err != nil {
		t.Fatal(err)
	}
	run, err = s.Store.Claim(ctx, "render-2")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, run)
	if run.Status != "completed" || f.images != 10 || len(run.AssetIDs) != 10 || run.Spent != "1.70000000" {
		t.Fatalf("lost pages/cost across chunks: %s %s %d %s", run.Status, run.ErrorCode, f.images, run.Spent)
	}
	if f.turns != 3 {
		t.Fatal("render used paid model polling")
	}
	for i, result := range run.Work.Results {
		if result.ItemID != fmt.Sprint("page", i+1) || result.AssetID == "" {
			t.Fatal("unstable item binding")
		}
	}
}
func TestPostgresAdaptiveIncrementalDraftAndChatDoNotInvalidate(t *testing.T) {
	s, f, c := newAdaptive(t)
	ctx := context.Background()
	f.choose = func(_ Invocation, n int) Outcome {
		if n == 1 {
			return adaptiveCall("first", "save_work_plan", workDraft(3))
		}
		return adaptiveReply("可以继续讨论。")
	}
	adaptiveTurn(t, s, c, "first", "给我一个三页草稿")
	plans, _ := s.Store.WorkPlans(ctx, 1, c.ID)
	p := plans[0]
	adaptiveTurn(t, s, c, "chat", "你好，先聊一下")
	still, _ := s.Store.WorkPlan(ctx, 1, p.ID)
	if still.Revision != p.Revision || still.Status != "draft" {
		t.Fatal("ordinary chat invalidated draft")
	}
	material, err := s.Upload(ctx, 1, c.ID, testImage())
	if err != nil {
		t.Fatal(err)
	}
	item := p.Items[1]
	item.References = []string{material.ID}
	item.Content = "用户补充的奖状及说明"
	before := f.turns
	f.choose = func(in Invocation, n int) Outcome {
		if n == before+1 {
			return adaptiveCall("patch", "update_draft", DraftUpdate{PlanID: &p.ID, Revision: &p.Revision, Patch: DraftPatch{Items: &[]WorkItem{item}}})
		}
		return adaptiveReply("只修改第二页。")
	}
	run := adaptiveTurn(t, s, c, "patch", "第二页加上新素材，其他页面保持")
	if run.Status != "completed" {
		t.Fatal(run.Status, run.ErrorCode)
	}
	after, err := s.Store.WorkPlan(ctx, 1, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != 2 || len(after.Items) != 3 || Hash(after.Items[0]) != Hash(p.Items[0]) || Hash(after.Items[2]) != Hash(p.Items[2]) || Hash(after.ChangedItems) != Hash([]string{item.ID}) {
		t.Fatal("incremental change lost content or impact scope", after.ChangedItems)
	}
	var mutations int
	s.Store.DB.QueryRowContext(ctx, `SELECT count(*) FROM agent_draft_mutations WHERE plan_id=$1`, p.ID).Scan(&mutations)
	if mutations != 2 {
		t.Fatal("revision replay evidence missing")
	}
}
func TestPostgresAdaptiveAmbiguousRequestCanReplyWithoutForcedQuestion(t *testing.T) {
	s, f, c := newAdaptive(t)
	f.choose = func(in Invocation, _ int) Outcome {
		if in.ForceAskUser || !in.ImageTools {
			t.Error("business keyword forced tool/mode")
		}
		return adaptiveReply("可以。你希望讲什么主题、面对哪些听众？")
	}
	r := adaptiveTurn(t, s, c, "vague", "我想做PPT")
	if r.Status != "completed" || r.Question != nil || r.WorkPlanning {
		t.Fatal("vague request treated as fatal workflow", r.Status, r.ErrorCode)
	}
}
func TestPostgresAdaptiveRepeatedToolErrorsYieldProgress(t *testing.T) {
	s, f, c := newAdaptive(t)
	bad := workDraft(1)
	bad.Size = "invalid"
	f.choose = func(_ Invocation, n int) Outcome { return adaptiveCall(fmt.Sprint("bad", n), "save_work_plan", bad) }
	r := adaptiveTurn(t, s, c, "repeat", "先做个方案")
	if r.Status != "completed" || r.ErrorCode != "" || r.ToolErrors != 3 || f.turns != 3 || f.images != 0 || !strings.Contains(r.Text, "保留") {
		t.Fatal("tool repair not bounded", r.Status, r.ErrorCode, r.ToolErrors, f.turns)
	}
}
func makeAdaptivePlan(t *testing.T, s *Service, f *adaptiveExecutor, c Conversation) WorkPlan {
	t.Helper()
	start := f.turns
	f.choose = func(_ Invocation, n int) Outcome {
		if n == start+1 {
			return adaptiveCall("plan", "save_work_plan", workDraft(1))
		}
		return adaptiveReply("等待你的决定。")
	}
	adaptiveTurn(t, s, c, ID("msg"), "先讨论风格样张，不要自动生图")
	ps, err := s.Store.WorkPlans(context.Background(), 1, c.ID)
	if err != nil || len(ps) != 1 {
		t.Fatal("plan", err, len(ps))
	}
	return ps[0]
}
func TestPostgresAdaptiveImageRequiresHumanApprovalAndScopedGrant(t *testing.T) {
	s, f, c := newAdaptive(t)
	ctx := context.Background()
	f.choose = func(_ Invocation, n int) Outcome {
		if n == 1 {
			return adaptiveCall("single", "generate_image", ToolArgs{Prompt: "蓝白极简风格样张"})
		}
		return adaptiveReply("请在配置卡确认，尚未生成。")
	}
	run := adaptiveTurn(t, s, c, "card", "先展示一张蓝白风格样张的方案，批准后再生成")
	if f.images != 0 || len(run.AssetIDs) != 0 || run.Status != "completed" {
		t.Fatal("unapproved generation")
	}
	ps, _ := s.Store.WorkPlans(ctx, 1, c.ID)
	p := ps[0]
	request := GrantRequest{RequestID: "human-approval-001", Revision: p.Revision, Purpose: "sample", MaxCount: 1, Minutes: 60}
	grant, err := s.CreateGrant(ctx, 1, p.ID, "", request)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.CreateGrant(ctx, 1, p.ID, "", request)
	if err != nil || again.ID != grant.ID {
		t.Fatal("grant replay", err)
	}
	if _, err = s.CreateGrant(ctx, 2, p.ID, "", request); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign owner grant", err)
	}
	start := f.turns
	purpose := "sample"
	f.choose = func(_ Invocation, n int) Outcome {
		if n == start+1 {
			return adaptiveCall("sample", "generate_image", ToolArgs{Prompt: "蓝白极简风格样张", TaskID: &p.ID, Purpose: &purpose})
		}
		return adaptiveReply("样张已实际生成，请评价风格。")
	}
	generated := adaptiveTurn(t, s, c, "sample", "按我刚刚授予的许可生成一张样张")
	if generated.Status != "completed" || f.images != 1 || len(generated.AssetIDs) != 1 {
		t.Fatal("approved sample failed", generated.Status, generated.ErrorCode, f.images)
	}
	gs, err := s.Store.Grants(ctx, 1, c.ID)
	if err != nil || len(gs) != 1 || gs[0].UsedCount != 1 || gs[0].Status != "closed" || gs[0].Reserved != "0.20000000" {
		t.Fatal("scope ledger", gs, err)
	}
	start = f.turns
	f.choose = func(_ Invocation, n int) Outcome {
		if n == start+1 {
			return adaptiveCall("too-many", "generate_image", ToolArgs{Prompt: "再一张", TaskID: &p.ID, Purpose: &purpose})
		}
		return adaptiveReply("许可已用完，请重新授权。")
	}
	final := adaptiveTurn(t, s, c, "no-reset", "再考虑一下样张")
	if final.Status != "completed" || f.images != 1 || final.ToolErrors != 1 {
		t.Fatal("new run reset count/budget", final.Status, final.ErrorCode, f.images)
	}
}
func TestPostgresAdaptiveGrantReservationsAreAtomicAndBoundToModel(t *testing.T) {
	s, f, c := newAdaptive(t)
	ctx := context.Background()
	p := makeAdaptivePlan(t, s, f, c)
	g, err := s.CreateGrant(ctx, 1, p.ID, "", GrantRequest{RequestID: "atomic-grant-01", Revision: p.Revision, Purpose: "sample", MaxCount: 1, MaxCost: "0.25", Minutes: 60})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Submit(ctx, 1, c.ID, "reserve", "", "继续", nil, 1, "5")
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Store.Claim(ctx, "reserve-worker")
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.Store.Prepare(ctx, r, Operation{ID: fmt.Sprintf("%s_image_%d", r.ID, i), RunID: r.ID, Kind: "image", Step: i, RequestHash: fmt.Sprint(i), Quote: "0.20", GrantID: g.ID})
			results <- e
		}(i)
	}
	wg.Wait()
	close(results)
	succeeded := 0
	for e := range results {
		if e == nil {
			succeeded++
		} else if SafeCode(e) != "AGENT_GRANT_EXHAUSTED" {
			t.Fatal("unexpected reservation error", e)
		}
	}
	if succeeded != 1 || f.images != 0 {
		t.Fatal("grant over-reserved", succeeded)
	}
	other := *r
	other.Config.ImageModel = "other"
	if _, err = s.Store.Prepare(ctx, &other, Operation{ID: r.ID + "_image_9", RunID: r.ID, Kind: "image", Step: 9, RequestHash: "other-model", Quote: "0.01", GrantID: g.ID}); SafeCode(err) != "AGENT_GRANT_EXHAUSTED" {
		t.Fatal("model binding bypass", err)
	}
}
func TestPostgresAdaptiveExpiryPreservesMetadataWithoutConversationExemption(t *testing.T) {
	s, _, c := newAdaptive(t)
	ctx := context.Background()
	a, err := s.UploadFile(ctx, 1, c.ID, "note.txt", []byte("secret original file"))
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(a.Expires) > ContentRetention+time.Second || time.Until(a.Expires) < ContentRetention-time.Minute {
		t.Fatal("upload TTL")
	}
	_, err = s.Submit(ctx, 1, c.ID, "active", "", "你好", nil, 1, "5")
	if err != nil {
		t.Fatal(err)
	}
	s.Store.DB.ExecContext(ctx, `UPDATE agent_assets SET expires_at=now()-interval '1 second' WHERE id=$1`, a.ID)
	meta, err := s.Store.AssetMeta(ctx, 1, a.ID)
	if err != nil || meta.ContentAvailable || meta.FileName != "note.txt" {
		t.Fatal("metadata lost", err)
	}
	if _, err = s.LoadAsset(ctx, 1, a.ID); SafeCode(err) != "ASSET_EXPIRED" {
		t.Fatal("active conversation bypassed expiry", err)
	}
	if _, err = s.Store.AssetMeta(ctx, 2, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("expired metadata leaked to foreign owner", err)
	}
	if err = s.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(s.Root, a.Key)); !os.IsNotExist(err) {
		t.Fatal("expired bytes retained", err)
	}
	if _, err = s.Store.AssetMeta(ctx, 1, a.ID); err != nil {
		t.Fatal("cleanup deleted metadata", err)
	}
	note := s.attachmentNote(ctx, 1, c.ID, []string{a.ID})
	if strings.Contains(note, "secret original file") || !strings.Contains(note, "过期") {
		t.Fatal("expired original text returned", note)
	}
	replacement, err := s.UploadFile(ctx, 1, c.ID, "note.txt", []byte("new upload"))
	if err != nil || replacement.ID == a.ID || !replacement.ContentAvailable {
		t.Fatal("reupload did not create independent asset", err)
	}
}
func TestPostgresAdaptiveHoldIsBoundedToOneAlreadyAdmittedRun(t *testing.T) {
	s, _, c := newAdaptive(t)
	ctx := context.Background()
	a, err := s.Upload(ctx, 1, c.ID, testImage())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Submit(ctx, 1, c.ID, "hold", "", "继续", nil, 1, "5")
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Store.Claim(ctx, "held")
	if err != nil {
		t.Fatal(err)
	}
	s.Store.DB.ExecContext(ctx, `INSERT INTO agent_asset_holds(asset_id,run_id,until_at) VALUES($1,$2,now()+interval '1 minute')`, a.ID, r.ID)
	s.Store.DB.ExecContext(ctx, `UPDATE agent_assets SET expires_at=now()-interval '1 second' WHERE id=$1`, a.ID)
	if _, err = s.LoadAsset(ctx, 1, a.ID); SafeCode(err) != "ASSET_EXPIRED" {
		t.Fatal("hold opened public access")
	}
	if _, err = s.loadAssetForRun(ctx, 1, a.ID, r.ID); err != nil {
		t.Fatal("paid run lost exact input", err)
	}
	if _, err = s.loadAssetForRun(ctx, 1, a.ID, "another-run"); SafeCode(err) != "ASSET_EXPIRED" {
		t.Fatal("hold leaked to another task")
	}
	if err = s.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(s.Root, a.Key)); err != nil {
		t.Fatal("protected input removed")
	}
	s.Store.DB.ExecContext(ctx, `UPDATE agent_asset_holds SET until_at=now()-interval '1 second' WHERE run_id=$1`, r.ID)
	if err = s.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(s.Root, a.Key)); !os.IsNotExist(err) {
		t.Fatal("hold renewed content indefinitely")
	}
}
func TestPostgresAdaptiveExpiredCheckpointCannotRecoverOrRenewAsset(t *testing.T) {
	s, f, r := failedCheckpointFixture(t)
	ctx := context.Background()
	opID := r.ID + "_image_0"
	if _, err := s.Store.DB.ExecContext(ctx, `UPDATE agent_checkpoints SET content_created_at=now()-interval '25 hours',content_expires_at=now()-interval '1 hour' WHERE operation_id=$1`, opID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.readOutcome(opID); SafeCode(err) != "ASSET_EXPIRED" {
		t.Fatal("expired checkpoint read", err)
	}
	if _, err := s.QueueImageRecovery(ctx, 1, r.ID); err == nil {
		t.Fatal("expired checkpoint recovery admitted")
	}
	if _, err := s.persistImage(ctx, 1, r.ConversationID, r.ID, opID, "asset_expired", testImage()); SafeCode(err) != "ASSET_EXPIRED" {
		t.Fatal("expired result renewed", err)
	}
	messages, err := s.Store.Messages(ctx, 1, r.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if messages[len(messages)-1].CanRecoverImage {
		t.Fatal("UI advertises expired recovery")
	}
	if err = s.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(s.Root, opID+".result.json")); !os.IsNotExist(err) {
		t.Fatal("raw checkpoint retained")
	}
	op, err := s.Store.LookupOperation(ctx, opID)
	if err != nil || op.Cost != "0.17000000" || f.recoveries != 0 {
		t.Fatal("expiry erased ledger or invoked provider", op.Cost, err)
	}
}

func TestPostgresAdaptiveRevisionReusesUnchangedPaidPages(t *testing.T) {
	s, f, c := newAdaptive(t)
	ctx := context.Background()
	start := f.turns
	f.choose = func(_ Invocation, n int) Outcome {
		if n == start+1 {
			return adaptiveCall("plan", "save_work_plan", workDraft(3))
		}
		return adaptiveReply("先审核三页。")
	}
	adaptiveTurn(t, s, c, "initial", "准备三页")
	plans, _ := s.Store.WorkPlans(ctx, 1, c.ID)
	p := plans[0]
	ready, err := s.ReviewWork(ctx, 1, p.ID, "", p.Revision, p.WorkDraft)
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.SubmitWork(ctx, 1, p.ID, "", ready.Revision)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := s.Store.Claim(ctx, "render-old")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, rendered)
	s.Store.Release(ctx, rendered)
	if rendered.Status != "completed" || f.images != 3 {
		t.Fatal("initial render", rendered.Status, rendered.ErrorCode)
	}
	p, err = s.Store.WorkPlan(ctx, 1, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	item := p.Items[1]
	item.Content = "只更新第二页的内容"
	start = f.turns
	f.choose = func(_ Invocation, n int) Outcome {
		if n == start+1 {
			return adaptiveCall("edit", "update_draft", DraftUpdate{PlanID: &p.ID, Revision: &p.Revision, Patch: DraftPatch{Items: &[]WorkItem{item}}})
		}
		return adaptiveReply("第二页已更新，其他页面沿用。")
	}
	r := adaptiveTurn(t, s, c, "edit-completed", "只改第二页")
	if r.Status != "completed" || r.ErrorCode != "" {
		t.Fatal(r.Status, r.ErrorCode)
	}
	p, _ = s.Store.WorkPlan(ctx, 1, p.ID)
	if p.Status != "draft" || len(p.InheritedResults) != 2 {
		t.Fatal("unchanged results lost", p.Status, len(p.InheritedResults))
	}
	ready, err = s.ReviewWork(ctx, 1, p.ID, "", p.Revision, p.WorkDraft)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Quote != "0.20000000" {
		t.Fatal("charged all pages again", ready.Quote)
	}
	_, err = s.SubmitWork(ctx, 1, p.ID, "", ready.Revision)
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.Store.Claim(ctx, "render-edit")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, current)
	if current.Status != "completed" || f.images != 4 || current.Spent != "0.17000000" || len(current.AssetIDs) != 3 {
		t.Fatal("selective render failed", current.Status, current.ErrorCode, f.images, current.Spent)
	}
	if current.Work.Results[0].AssetID != rendered.Work.Results[0].AssetID || current.Work.Results[2].AssetID != rendered.Work.Results[2].AssetID {
		t.Fatal("unchanged outputs replaced")
	}
	original, err := s.Store.Run(ctx, 1, old.ID)
	if err != nil || original.Work.Plan.Items[1].Content != workDraft(3).Items[1].Content {
		t.Fatal("approved original manifest mutated")
	}
}
func TestPostgresAdaptiveIgnoresLegacyTaskAndGrantBudgets(t *testing.T) {
	s, f, c := newAdaptive(t)
	ctx := context.Background()
	p := makeAdaptivePlan(t, s, f, c)
	if _, err := s.Store.DB.ExecContext(ctx, `UPDATE agent_work_plans SET snapshot=jsonb_set(snapshot,'{task_budget_usd}','"0.15"'::jsonb) WHERE id=$1`, p.ID); err != nil {
		t.Fatal(err)
	}
	// Historical task and grant monetary limits must not block an authorized image.
	_, err := s.CreateGrant(ctx, 1, p.ID, "", GrantRequest{RequestID: "task-budget-001", Revision: p.Revision, Purpose: "sample", MaxCount: 1, MaxCost: "0.01", Minutes: 60})
	if err != nil {
		t.Fatal(err)
	}
	start := f.turns
	purpose := "sample"
	f.choose = func(_ Invocation, n int) Outcome {
		if n == start+1 {
			return adaptiveCall("budget", "generate_image", ToolArgs{Prompt: "样张", TaskID: &p.ID, Purpose: &purpose})
		}
		return adaptiveReply("已完成样张。")
	}
	r := adaptiveTurn(t, s, c, "budget-after-grant", "按许可做样张")
	if r.Status != "completed" || r.ErrorCode != "" || f.images != 1 || r.ToolErrors != 0 {
		t.Fatal("task total reset", r.Status, r.ErrorCode, f.images)
	}
	committed, err := projectCommitted(ctx, s.Store.DB, p.ID)
	if err != nil || WithinBudget("0", committed, "0.15") {
		t.Fatal("project ledger exceeded", committed, err)
	}
	grants, _ := s.Store.Grants(ctx, 1, c.ID)
	if grants[0].UsedCount != 1 {
		t.Fatal("non-dispatched budget denial consumed scope")
	}
}
func TestPostgresAdaptiveCheckpointExpiryDoesNotMoveOnRecovery(t *testing.T) {
	s, _, r := failedCheckpointFixture(t)
	ctx := context.Background()
	op := r.ID + "_image_0"
	var first time.Time
	if err := s.Store.DB.QueryRowContext(ctx, `SELECT content_expires_at FROM agent_checkpoints WHERE operation_id=$1`, op).Scan(&first); err != nil {
		t.Fatal(err)
	}
	outcome, err := s.readOutcome(op)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.writeOutcome(op, outcome); err != nil {
		t.Fatal(err)
	}
	var second time.Time
	s.Store.DB.QueryRowContext(ctx, `SELECT content_expires_at FROM agent_checkpoints WHERE operation_id=$1`, op).Scan(&second)
	if !first.Equal(second) {
		t.Fatal("recovery renewed raw content TTL")
	}
	a, err := s.persistImage(ctx, 1, r.ConversationID, r.ID, op, "asset_first_available", testImage())
	if err != nil {
		t.Fatal(err)
	}
	if !a.Expires.Equal(first) {
		t.Fatal("asset used recovery time instead of first available response")
	}
}

func TestAgentOperationIdentityIgnoresRetentionButBindsPixels(t *testing.T) {
	original := Invocation{Kind: "image", Model: "image", References: []Asset{{ID: "asset_a", SHA: "sha_a", Expires: time.Now(), Status: "ready", ContentAvailable: true}}}
	next := original
	next.References = append([]Asset{}, original.References...)
	next.References[0].Expires = time.Now().Add(time.Hour)
	next.References[0].Status = "expired"
	next.References[0].ContentAvailable = false
	if operationRequestHash(original) != operationRequestHash(next) {
		t.Fatal("logical expiry broke paid request recovery")
	}
	next.References[0].SHA = "different_pixels"
	if operationRequestHash(original) == operationRequestHash(next) {
		t.Fatal("different pixels share request identity")
	}
}
func TestAgentExpandedFileTextIsEphemeralAndJSONSafe(t *testing.T) {
	input := []json.RawMessage{UserItem("请分析：" + TextAssetPrefix + "asset_a")}
	original := string(jsonBytes(input))
	secret := "原始数据\n包含引号\"和反斜线\\"
	expanded := ExpandTextAssets(input, []Asset{{ID: "asset_a", Data: []byte(secret)}})
	var item map[string]any
	if json.Unmarshal(expanded[0], &item) != nil || !strings.Contains(item["content"].(string), secret) {
		t.Fatal("text expansion corrupts JSON")
	}
	if string(jsonBytes(input)) != original || strings.Contains(original, secret) {
		t.Fatal("text expansion mutated durable context")
	}
	expired := ExpandTextAssets(input, nil)
	if strings.Contains(string(jsonBytes(expired)), secret) || !strings.Contains(string(jsonBytes(expired)), "过期") {
		t.Fatal("expired file content reused")
	}
}

func TestPostgresAdaptiveFinishTaskClosesRemainingScopeWithoutSpending(t *testing.T) {
	s, f, c := newAdaptive(t)
	ctx := context.Background()
	p := makeAdaptivePlan(t, s, f, c)
	_, err := s.CreateGrant(ctx, 1, p.ID, "", GrantRequest{RequestID: "finish-task-grant", Revision: p.Revision, Purpose: "sample", MaxCount: 3, MaxCost: "1", Minutes: 60})
	if err != nil {
		t.Fatal(err)
	}
	start := f.turns
	f.choose = func(_ Invocation, n int) Outcome {
		if n == start+1 {
			return adaptiveCall("finish", "finish_task", map[string]string{"plan_id": p.ID})
		}
		return adaptiveReply("已结束任务，保留草稿。")
	}
	r := adaptiveTurn(t, s, c, "finish", "这个任务到这里，先结束")
	grants, err := s.Store.Grants(ctx, 1, c.ID)
	if err != nil || len(grants) != 1 || grants[0].Status != "closed" || grants[0].UsedCount != 0 || f.images != 0 {
		t.Fatal("finish did not revoke unused scope", grants, err)
	}
	p, err = s.Store.WorkPlan(ctx, 1, p.ID)
	if err != nil || p.TaskState != "closed" || len(p.Items) != 1 || r.Status != "completed" {
		t.Fatal("finish erased project", err)
	}
}

func TestPostgresAdaptivePresentReviewTargetsExistingPlanWithoutDispatch(t *testing.T) {
	s, f, c := newAdaptive(t)
	ctx := context.Background()
	f.choose = func(_ Invocation, n int) Outcome {
		if n == 1 {
			return adaptiveCall("create", "save_work_plan", workDraft(1))
		}
		return adaptiveReply("草稿已保存")
	}
	first := adaptiveTurn(t, s, c, "create-review", "先整理方案")
	plans, err := s.Store.WorkPlans(ctx, 1, c.ID)
	if err != nil || len(plans) != 1 {
		t.Fatal("missing original plan", err)
	}
	p := plans[0]
	if _, err = s.Store.DB.ExecContext(ctx, `UPDATE agent_work_plans SET status='submitted' WHERE id=$1`, p.ID); err != nil {
		t.Fatal(err)
	}
	f.turns = 0
	f.choose = func(_ Invocation, n int) Outcome {
		if n == 1 {
			return adaptiveCall("read", "get_job", map[string]string{"plan_id": p.ID})
		}
		if n == 2 {
			return adaptiveCall("show", "present_review", map[string]string{"plan_id": p.ID})
		}
		return adaptiveReply("已请求重新展示原配置卡")
	}
	r := adaptiveTurn(t, s, c, "show-review", "没看到，请再展示原配置卡")
	if r.Status != "completed" || f.images != 0 {
		t.Fatalf("unexpected execution: status=%s images=%d", r.Status, f.images)
	}
	found := false
	for _, step := range r.Trace {
		if step.Name == "get_job" && step.ReviewPlanID != "" {
			t.Fatal("read-only job query requested presentation")
		}
		if step.Name == "present_review" {
			found = step.ReviewPlanID == p.ID && step.Status == "completed"
		}
	}
	if !found {
		t.Fatal("existing plan presentation target missing")
	}
	for _, item := range r.History {
		var record map[string]any
		_ = json.Unmarshal(item, &record)
		if record["type"] == "function_call_output" && record["call_id"] == "show" {
			var result map[string]any
			_ = json.Unmarshal([]byte(record["output"].(string)), &result)
			if result["approval"] != "not_pending" || result["presentation"] != "requested" || result["dispatched"] != false {
				t.Fatal("submitted result was advertised as new approval", result)
			}
		}
	}
	after, err := s.Store.WorkPlans(ctx, 1, c.ID)
	if err != nil || len(after) != 1 || after[0].SourceRunID != first.ID || after[0].Revision != p.Revision || after[0].Status != "submitted" {
		t.Fatal("presentation duplicated or rewrote the original plan", err)
	}
	var public map[string]any
	if json.Unmarshal(jsonBytes(r.Public()), &public) != nil || !strings.Contains(string(jsonBytes(public["trace"])), p.ID) {
		t.Fatal("review target was not persisted in the public snapshot")
	}
}

func TestPostgresAdaptiveImageProposalRequestsReviewWithoutDispatch(t *testing.T) {
	s, f, c := newAdaptive(t)
	f.choose = func(_ Invocation, n int) Outcome {
		if n == 1 {
			return adaptiveCall("propose", "generate_image", map[string]any{"prompt": "cat", "model": nil, "image_size": "1K", "aspect_ratio": "1:1", "reference_asset_ids": nil, "task_id": nil, "purpose": nil})
		}
		return adaptiveReply("请先审核配置卡")
	}
	r := adaptiveTurn(t, s, c, "image-review", "先展示生图方案")
	plans, err := s.Store.WorkPlans(context.Background(), 1, c.ID)
	if err != nil || len(plans) != 1 || f.images != 0 {
		t.Fatal("proposal dispatched or lost its plan", f.images, err)
	}
	found := false
	for _, step := range r.Trace {
		if step.Name == "generate_image" && step.ReviewPlanID == plans[0].ID {
			found = true
		}
	}
	if !found {
		t.Fatal("image proposal omitted its explicit review target")
	}
}

func TestPostgresAdaptiveReviewTargetCannotCrossConversation(t *testing.T) {
	s, f, c := newAdaptive(t)
	f.choose = func(_ Invocation, n int) Outcome {
		if n == 1 {
			return adaptiveCall("create", "save_work_plan", workDraft(1))
		}
		return adaptiveReply("已保存")
	}
	adaptiveTurn(t, s, c, "private-plan", "保存方案")
	plans, err := s.Store.WorkPlans(context.Background(), 1, c.ID)
	if err != nil || len(plans) != 1 {
		t.Fatal(err)
	}
	other, err := s.Create(context.Background(), 1, "另一会话", c.Config, "")
	if err != nil {
		t.Fatal(err)
	}
	f.turns = 0
	f.choose = func(_ Invocation, n int) Outcome {
		if n == 1 {
			return adaptiveCall("foreign", "present_review", map[string]string{"plan_id": plans[0].ID})
		}
		return adaptiveReply("该方案不属于本会话")
	}
	r := adaptiveTurn(t, s, other, "foreign-review", "展示方案")
	for _, step := range r.Trace {
		if step.ReviewPlanID != "" {
			t.Fatal("foreign plan leaked into a presentation request")
		}
	}
	if f.images != 0 {
		t.Fatal("foreign review dispatched an image")
	}
}
