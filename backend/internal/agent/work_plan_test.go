package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type workExecutor struct {
	testExecutor
	draft        WorkDraft
	askTurns     int
	textTurns    int
	priceChanged bool
	inputs       []Invocation
}

func (f *workExecutor) Quote(ctx context.Context, in Invocation) (string, error) {
	if in.Kind == "image" && f.priceChanged {
		return "0.30000000", nil
	}
	return f.testExecutor.Quote(ctx, in)
}
func (f *workExecutor) Execute(ctx context.Context, in Invocation) (Outcome, error) {
	f.inputs = append(f.inputs, in)
	if in.Kind == "image" {
		return f.testExecutor.Execute(ctx, in)
	}
	f.textTurns++
	if f.textTurns > f.askTurns+1 {
		return Outcome{Cost: "0.01000000", Body: jsonBytes(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": "草稿已保留，你可以继续讨论或审核配置卡。"}}}}})}, nil
	}
	name := "save_work_plan"
	args := jsonBytes(f.draft)
	if f.textTurns <= f.askTurns {
		name = "ask_user"
		args = jsonBytes(AskUserArgs{Question: "请确认主题及内容安排", Fields: []QuestionField{{ID: "topic", Label: "主题", Options: []string{}, Required: true}}})
	}
	return Outcome{Cost: "0.01000000", Body: jsonBytes(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "function_call", "call_id": fmt.Sprint("call", f.textTurns), "name": name, "arguments": string(args)}}})}, nil
}
func workDraft(n int) WorkDraft {
	p := WorkDraft{Title: "经验分享", Brief: "给新生分享学习方法", Style: "简洁的白底蓝色模板", Size: "1K", Aspect: "16:9", SharedReferences: []string{}, Items: []WorkItem{}}
	for i := 0; i < n; i++ {
		p.Items = append(p.Items, WorkItem{ID: fmt.Sprint("page", i+1), Title: fmt.Sprint("第", i+1, "页"), Content: "先写清目标，再分解行动，最后复盘。", Prompt: "清晰标题、三列要点、留白充足。", References: []string{}})
	}
	return p
}
func workFixture(t *testing.T, n, asks int) (*Service, *workExecutor, WorkPlan) {
	t.Helper()
	st := isolatedStore(t)
	f := &workExecutor{draft: workDraft(n), askTurns: asks}
	s := New(st, f, t.TempDir(), true)
	s.WorkConcurrency = 1
	ctx := context.Background()
	c, err := s.Create(ctx, 1, "视觉方案", Config{ModelKeyID: 1, Model: "text", ImageKeyID: 2, ImageModel: "image", Budget: "1.00"}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Submit(ctx, 1, c.ID, "plan-request", "127.0.0.1", "我想做ppt", nil, 1, "1.00")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= asks; i++ {
		r, e := st.Claim(ctx, "worker")
		if e != nil {
			t.Fatal(e)
		}
		s.process(ctx, r)
		_ = st.Release(ctx, r)
		if i < asks {
			if r.Status != "waiting_user" || f.images != 0 {
				t.Fatalf("expected clarification, got %s %s", r.Status, r.ErrorCode)
			}
			if _, e = st.Answer(ctx, 1, r.ID, r.Question.ID, map[string]string{"topic": "学习经验，三页，面向新生，16:9，按模板风格"}); e != nil {
				t.Fatal(e)
			}
		} else if r.Status != "completed" {
			t.Fatalf("plan creation failed %s %s", r.Status, r.ErrorCode)
		}
	}
	plans, err := st.WorkPlans(ctx, 1, c.ID)
	if err != nil || len(plans) != 1 {
		t.Fatalf("plans %v %v", plans, err)
	}
	if f.images != 0 {
		t.Fatal("planning generated a paid image")
	}
	return s, f, plans[0]
}
func detailedWorkFixture(t *testing.T, n int) (*Service, *workExecutor, WorkPlan) {
	// Avoid the explicit vague-request question guard for fully specified content.
	t.Helper()
	st := isolatedStore(t)
	f := &workExecutor{draft: workDraft(n)}
	s := New(st, f, t.TempDir(), true)
	s.WorkConcurrency = 1
	ctx := context.Background()
	c, err := s.Create(ctx, 1, "详细方案", Config{ModelKeyID: 1, Model: "text", ImageKeyID: 2, ImageModel: "image", Budget: "1.00"}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Submit(ctx, 1, c.ID, "detailed", "127.0.0.1", "按以下详细内容做三页幻灯片：目标、行动、复盘。使用1K，16:9，自行拟定配文，先给我配置卡。", nil, 1, "1.00")
	if err != nil {
		t.Fatal(err)
	}
	r, err := st.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	_ = st.Release(ctx, r)
	if r.Status != "completed" {
		t.Fatalf("%s %s", r.Status, r.ErrorCode)
	}
	ps, err := st.WorkPlans(ctx, 1, c.ID)
	if err != nil || len(ps) != 1 {
		t.Fatalf("%v %v", ps, err)
	}
	return s, f, ps[0]
}
func TestPostgresWorkPlanClarifiesThenReviewsAndExecutes(t *testing.T) {
	s, f, p := workFixture(t, 3, 2)
	ctx := context.Background()
	if f.textTurns != 4 {
		t.Fatal("multi-round answers did not resume original run")
	}
	p.Items[1].Content = "用户最终修改后的第二页内容"
	ready, err := s.ReviewWork(ctx, 1, p.ID, "127.0.0.1", p.Revision, p.WorkDraft)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Revision != 2 || ready.Quote != "0.60000000" || f.images != 0 {
		t.Fatalf("review dispatched or quote wrong: %+v", ready)
	}
	run, err := s.SubmitWork(ctx, 1, p.ID, "127.0.0.1", ready.Revision)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.SubmitWork(ctx, 1, p.ID, "127.0.0.1", ready.Revision)
	if err != nil || replay.ID != run.ID {
		t.Fatal("duplicate submit not idempotent", err)
	}
	claimed, err := s.Store.Claim(ctx, "work-worker")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, claimed)
	if claimed.Status != "completed" || f.images != 3 || len(claimed.AssetIDs) != 3 || claimed.Spent != "0.51000000" {
		t.Fatalf("execution %s %s images=%d cost=%s", claimed.Status, claimed.ErrorCode, f.images, claimed.Spent)
	}
	if !strings.Contains(f.inputs[len(f.inputs)-2].Args.Prompt, "用户最终修改后的第二页内容") {
		t.Fatal("edited content not transmitted")
	}
	if _, err = s.Retry(ctx, 1, run.ID, "unsafe-retry", "127.0.0.1", 1, "1.00"); SafeCode(err) != "AGENT_WORK_REVIEW_REQUIRED" {
		t.Fatal("generic retry bypass", err)
	}
	plans, err := s.Store.WorkPlans(ctx, 1, p.ConversationID)
	if err != nil || len(plans[0].Results) != 3 || plans[0].ExecutionStatus != "completed" {
		t.Fatalf("history lost results %+v %v", plans, err)
	}
	s.process(ctx, claimed)
	if f.images != 3 {
		t.Fatal("completed job regenerated")
	}
}
func TestPostgresWorkPlanOwnershipRevisionAndSupersession(t *testing.T) {
	s, _, p := detailedWorkFixture(t, 2)
	ctx := context.Background()
	if _, err := s.Store.WorkPlan(ctx, 2, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-owner plan", err)
	}
	if _, err := s.ReviewWork(ctx, 2, p.ID, "", 1, p.WorkDraft); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-owner edit", err)
	}
	if _, err := s.SubmitWork(ctx, 2, p.ID, "", 1); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-owner execution", err)
	}
	ready, err := s.ReviewWork(ctx, 1, p.ID, "", 1, p.WorkDraft)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReviewWork(ctx, 1, p.ID, "", 1, p.WorkDraft); !errors.Is(err, ErrConflict) {
		t.Fatal("stale edit", err)
	}
	if _, err = s.SubmitWork(ctx, 1, p.ID, "", 1); err == nil {
		t.Fatal("stale revision executed")
	}
	if _, err = s.Submit(ctx, 1, p.ConversationID, "continue", "", "第二页加入奖状素材，继续完善方案", nil, 1, "1.00"); err != nil {
		t.Fatal(err)
	}
	current, err := s.Store.WorkPlan(ctx, 1, p.ID)
	if err != nil || current.Status != "ready" {
		t.Fatal("ordinary communication invalidated existing approval", err)
	}
	if _, err = s.SubmitWork(ctx, 1, p.ID, "", ready.Revision); err == nil {
		t.Fatal("superseded plan executed")
	}
}
func TestPostgresWorkPlanMergesSharedAndPageMaterials(t *testing.T) {
	s, f, p := detailedWorkFixture(t, 2)
	ctx := context.Background()
	template, err := s.Upload(ctx, 1, p.ConversationID, testImage())
	if err != nil {
		t.Fatal(err)
	}
	material, err := s.Upload(ctx, 1, p.ConversationID, testImage())
	if err != nil {
		t.Fatal(err)
	}
	p.SharedReferences = []string{template.ID}
	p.Items[1].References = []string{template.ID, material.ID}
	ready, err := s.ReviewWork(ctx, 1, p.ID, "", 1, p.WorkDraft)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SubmitWork(ctx, 1, p.ID, "", ready.Revision)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Store.Claim(ctx, "work-worker")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	if r.Status != "completed" {
		t.Fatal(r.Status, r.ErrorCode)
	}
	last := f.inputs[len(f.inputs)-1]
	if len(last.References) != 2 || last.References[0].ID != template.ID || last.References[1].ID != material.ID {
		t.Fatal("shared template replaced page material")
	}
}
func TestPostgresWorkPlanRejectsForeignMaterialWithoutBudget(t *testing.T) {
	s, _, p := detailedWorkFixture(t, 2)
	ctx := context.Background()
	p.SharedReferences = []string{"foreign-asset"}
	if _, err := s.ReviewWork(ctx, 1, p.ID, "", 1, p.WorkDraft); !errors.Is(err, ErrNotFound) {
		t.Fatal("unowned reference", err)
	}
	p.SharedReferences = []string{}
	p.Items = workDraft(8).Items
	if _, err := s.ReviewWork(ctx, 1, p.ID, "", 1, p.WorkDraft); err != nil {
		t.Fatal("quote above legacy budget rejected", err)
	}
}
func TestPostgresWorkPlanStopsOnHigherPrice(t *testing.T) {
	s, f, p := detailedWorkFixture(t, 2)
	ctx := context.Background()
	ready, err := s.ReviewWork(ctx, 1, p.ID, "", 1, p.WorkDraft)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SubmitWork(ctx, 1, p.ID, "", ready.Revision)
	if err != nil {
		t.Fatal(err)
	}
	f.priceChanged = true
	r, err := s.Store.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	if f.images != 0 || r.ErrorCode != "AGENT_PRICING_CHANGED" {
		t.Fatalf("price changed but dispatched: %d %s", f.images, r.ErrorCode)
	}
}
func TestPostgresWorkPlanUnknownDoesNotRegenerate(t *testing.T) {
	s, f, p := detailedWorkFixture(t, 2)
	ctx := context.Background()
	ready, err := s.ReviewWork(ctx, 1, p.ID, "", 1, p.WorkDraft)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SubmitWork(ctx, 1, p.ID, "", ready.Revision)
	if err != nil {
		t.Fatal(err)
	}
	f.unknown = true
	r, err := s.Store.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	if r.Status != "failed" || f.images != 1 || r.ReconcileDeadline != nil {
		t.Fatalf("%s %s %d", r.Status, r.ErrorCode, f.images)
	}
	s.process(ctx, r)
	if f.images != 1 {
		t.Fatal("unknown operation reissued")
	}
	deadline := time.Now().Add(-time.Second)
	r.ReconcileDeadline = &deadline
	s.process(ctx, r)
	if !r.Terminal() || f.images != 1 {
		t.Fatal("reconciliation did not terminate safely")
	}
}
func TestWorkPlanStrictSchemaAndReferences(t *testing.T) {
	p := workDraft(2)
	p.SharedReferences = []string{"template"}
	p.Items[0].References = []string{"template", "award"}
	refs := workReferences(p, p.Items[0])
	if len(refs) != 2 || refs[0] != "template" || refs[1] != "award" {
		t.Fatal(refs)
	}
	raw := strings.TrimSuffix(string(jsonBytes(p)), "}") + `,"approved":true}`
	var value WorkDraft
	if StrictJSON([]byte(raw), &value) == nil {
		t.Fatal("model approved its own plan")
	}
	p.Items[1].ID = p.Items[0].ID
	if p.Validate() == nil {
		t.Fatal("duplicate item ids")
	}
	if !vagueVisualRequest("我想做ppt") || !requiresWorkPlan("一组图片，保持同一个风格") {
		t.Fatal("vague workflow not guarded")
	}
}

func TestPostgresWorkPlanAcceptsMaterialsWithClarification(t *testing.T) {
	st := isolatedStore(t)
	ctx := context.Background()
	f := &workExecutor{draft: workDraft(2), askTurns: 1}
	s := New(st, f, t.TempDir(), true)
	s.WorkConcurrency = 1
	c, err := s.Create(ctx, 1, "追加素材", Config{ModelKeyID: 1, Model: "text", ImageKeyID: 2, ImageModel: "image", Budget: "1.00"}, "")
	if err != nil {
		t.Fatal(err)
	}
	template, err := s.Upload(ctx, 1, c.ID, testImage())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Submit(ctx, 1, c.ID, "topic-template", "", "按模板制作两页幻灯片，主题是学习经验。先问清具体需要。", []string{template.ID}, 1, "1.00")
	if err != nil {
		t.Fatal(err)
	}
	r, err := st.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	_ = st.Release(ctx, r)
	if r.Question == nil {
		t.Fatal("missing question", r.ErrorCode)
	}
	material, err := s.Upload(ctx, 1, c.ID, testImage())
	if err != nil {
		t.Fatal(err)
	}
	qid := r.Question.ID
	answers := map[string]string{"topic": "面向新生，第二页加入我补充的照片，1K，16:9"}
	if _, err = st.AnswerWithAssets(ctx, 1, r.ID, qid, answers, []string{"unowned"}); !errors.Is(err, ErrNotFound) {
		t.Fatal("unowned answer asset accepted", err)
	}
	resumed, err := st.AnswerWithAssets(ctx, 1, r.ID, qid, answers, []string{material.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonBytes(resumed.History)), material.ID) {
		t.Fatal("added material omitted from model input")
	}
	if _, err = st.AnswerWithAssets(ctx, 1, r.ID, qid, answers, []string{material.ID}); err != nil {
		t.Fatal("answer replay", err)
	}
	f.draft.SharedReferences = []string{template.ID}
	f.draft.Items[1].References = []string{material.ID}
	r, err = st.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	plans, err := st.WorkPlans(ctx, 1, c.ID)
	if err != nil || len(plans) != 1 || plans[0].Items[1].References[0] != material.ID {
		t.Fatal("mid-conversation material missing", err)
	}
	if f.images != 0 {
		t.Fatal("clarification dispatched image without approval")
	}
	msgs, err := st.Messages(ctx, 1, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range msgs {
		if m.Kind == "question_answer" {
			found = len(m.Assets) == 1 && m.Assets[0] == material.ID
		}
	}
	if !found {
		t.Fatal("clarification material missing from saved receipt")
	}
}
func TestPostgresWorkPlanQuoteExpiryAndConfigurationChange(t *testing.T) {
	s, _, p := detailedWorkFixture(t, 1)
	ctx := context.Background()
	ready, err := s.ReviewWork(ctx, 1, p.ID, "", 1, p.WorkDraft)
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-11 * time.Minute)
	ready.QuotedAt = &past
	if _, err = s.Store.DB.ExecContext(ctx, `UPDATE agent_work_plans SET snapshot=$1 WHERE id=$2`, jsonBytes(ready), p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SubmitWork(ctx, 1, p.ID, "", ready.Revision); SafeCode(err) != "AGENT_WORK_REVIEW_REQUIRED" {
		t.Fatal("expired quote executed", err)
	}
	ready, err = s.ReviewWork(ctx, 1, p.ID, "", ready.Revision, p.WorkDraft)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Store.Conversation(ctx, 1, p.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Store.Configure(ctx, 1, c.ID, c.Version, c.Config); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SubmitWork(ctx, 1, p.ID, "", ready.Revision); !errors.Is(err, ErrConflict) {
		t.Fatal("changed key/configuration executed old approval", err)
	}
}

func TestPostgresWorkPlanCancellationRetainsPaidCheckpoint(t *testing.T) {
	s, f, p := detailedWorkFixture(t, 2)
	ctx := context.Background()
	ready, err := s.ReviewWork(ctx, 1, p.ID, "", 1, p.WorkDraft)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SubmitWork(ctx, 1, p.ID, "", ready.Revision)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Store.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	args := workArgs(ready, ready.Items[0])
	opID := fmt.Sprintf("%s_work_0", r.ID)
	_, _, err = s.operation(ctx, r, Invocation{OperationID: opID, RunID: r.ID, Kind: "image", Binding: Binding{UserID: r.UserID, KeyID: r.Config.ImageKeyID, GroupID: r.ImageGroupID, IP: r.IP}, Model: r.Config.ImageModel, Args: &args, References: []Asset{}, ApprovedQuote: ready.ItemQuotes[0]}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Store.Cancel(ctx, 1, r.ID); err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	if r.Status != "partial_success" || len(r.AssetIDs) != 1 || f.images != 1 || r.Work.Results[0].Status != "succeeded" {
		t.Fatalf("cancel lost a paid page or replayed: %s %s %d %+v", r.Status, r.ErrorCode, f.images, r.Work.Results)
	}
}
