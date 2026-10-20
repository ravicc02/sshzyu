package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func multiToolReply(calls ...Call) Outcome {
	items := []any{}
	for _, call := range calls {
		items = append(items, map[string]any{"type": "function_call", "call_id": call.ID, "name": call.Name, "arguments": call.Arguments})
	}
	return Outcome{Cost: "0.01000000", Body: jsonBytes(map[string]any{"status": "completed", "output": items})}
}
func batchCall(id, name string, args any) Call {
	return Call{ID: id, Name: name, Arguments: string(jsonBytes(args))}
}
func batchQuestion() AskUserArgs {
	return AskUserArgs{Question: "请补充主讲人姓名", Fields: []QuestionField{{ID: "name", Label: "姓名", Required: true, Options: []string{}}}}
}

func TestAgentMultiToolResponseOrderValidationAndPrivacy(t *testing.T) {
	calls := []Call{batchCall("read", "get_job", map[string]string{"plan_id": "p1"}), batchCall("ask", "ask_user", batchQuestion())}
	parsed, err := ParseResponse(multiToolReply(calls...).Body)
	if err != nil || len(parsed.Calls) != 2 || parsed.Calls[0].ID != "read" || parsed.Calls[1].ID != "ask" {
		t.Fatal("ordered calls lost", err)
	}
	if _, err = ParseResponse(multiToolReply(calls[0], calls[0]).Body); err == nil {
		t.Fatal("duplicate IDs admitted")
	}
	large := []Call{}
	for i := 0; i <= MaxToolCallsPerResponse; i++ {
		large = append(large, batchCall(fmt.Sprint(i), "get_job", map[string]string{"plan_id": "p1"}))
	}
	if _, err = ParseResponse(multiToolReply(large...).Body); !errors.Is(err, ErrToolBatchLimit) {
		t.Fatal("missing finite batch limit", err)
	}
	r := Run{PendingTools: &ToolBatch{OperationID: "private-operation", Calls: calls}}
	b, _ := json.Marshal(r.Public())
	if strings.Contains(string(b), "pending_tools") || strings.Contains(string(b), "private-operation") {
		t.Fatal("private queue exposed")
	}
}

func TestPostgresMultiToolReadThenAskUser(t *testing.T) {
	s, f, c := newAdaptive(t)
	p := makeAdaptivePlan(t, s, f, c)
	start := f.turns
	f.choose = func(_ Invocation, n int) Outcome {
		if n == start+1 {
			return multiToolReply(batchCall("read", "get_job", map[string]string{"plan_id": p.ID}), batchCall("ask", "ask_user", batchQuestion()))
		}
		return adaptiveReply("已按姓名补充主讲人介绍。")
	}
	r := adaptiveTurn(t, s, c, "multi-read-question", "加一页主讲人介绍")
	if r.Status != "waiting_user" || r.ErrorCode != "" || r.Question == nil || f.turns != start+1 || f.images != 0 {
		t.Fatal("read/question batch failed", r.Status, r.ErrorCode, f.turns, f.images)
	}
	if len(r.Trace) != 3 || r.Trace[1].Name != "get_job" || r.Trace[2].Name != "ask_user" {
		t.Fatal("wrong execution order")
	}
	if _, err := s.Store.Answer(context.Background(), 1, r.ID, r.Question.ID, map[string]string{"name": "李四"}); err != nil {
		t.Fatal(err)
	}
	next, err := s.Store.Claim(context.Background(), "answer-worker")
	if err != nil {
		t.Fatal(err)
	}
	s.process(context.Background(), next)
	if next.Status != "completed" || f.turns != start+2 {
		t.Fatal("question did not resume normally", next.Status, f.turns)
	}
}

func TestPostgresMultiToolQuestionDefersLaterPaidCalls(t *testing.T) {
	s, f, c := newAdaptive(t)
	p := makeAdaptivePlan(t, s, f, c)
	start := f.turns
	grant, err := s.CreateGrant(context.Background(), 1, p.ID, "", GrantRequest{RequestID: "batch-question-scope", Revision: p.Revision, Purpose: "sample", MaxCount: 2, Minutes: 60})
	if err != nil {
		t.Fatal(err)
	}
	purpose := "sample"
	f.choose = func(in Invocation, n int) Outcome {
		if n == start+1 {
			return multiToolReply(batchCall("ask", "ask_user", batchQuestion()), batchCall("later-image", "generate_image", ToolArgs{Prompt: "应在回答后重新规划", TaskID: &p.ID, Purpose: &purpose}))
		}
		if !strings.Contains(string(jsonBytes(in.Input)), "USER_INPUT_REQUIRED") {
			t.Error("deferred call has no matching output")
		}
		return adaptiveReply("已收到补充信息，先更新方案。")
	}
	r := adaptiveTurn(t, s, c, "multi-question", "补充我的照片和介绍")
	if r.Status != "waiting_user" || f.images != 0 || r.PendingTools != nil {
		t.Fatal("queued image crossed human boundary", r.Status, f.images)
	}
	grants, _ := s.Store.Grants(context.Background(), 1, c.ID)
	if len(grants) != 1 || grants[0].ID != grant.ID || grants[0].UsedCount != 0 {
		t.Fatal("unexecuted image consumed scope")
	}
	if _, err = s.Store.Answer(context.Background(), 1, r.ID, r.Question.ID, map[string]string{"name": "王五"}); err != nil {
		t.Fatal(err)
	}
	next, err := s.Store.Claim(context.Background(), "answer-worker")
	if err != nil {
		t.Fatal(err)
	}
	s.process(context.Background(), next)
	if next.Status != "completed" || f.images != 0 {
		t.Fatal("pre-answer paid call was executed after resume")
	}
}

func TestPostgresMultiToolImageBatchRespectsExistingGrants(t *testing.T) {
	for _, limit := range []int{1, 2} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			s, f, c := newAdaptive(t)
			p := makeAdaptivePlan(t, s, f, c)
			start := f.turns
			_, err := s.CreateGrant(context.Background(), 1, p.ID, "", GrantRequest{RequestID: "multi-image-scope", Revision: p.Revision, Purpose: "sample", MaxCount: limit, Minutes: 60})
			if err != nil {
				t.Fatal(err)
			}
			purpose := "sample"
			f.choose = func(_ Invocation, n int) Outcome {
				if n == start+1 {
					return multiToolReply(batchCall("one", "generate_image", ToolArgs{Prompt: "first", TaskID: &p.ID, Purpose: &purpose}), batchCall("two", "generate_image", ToolArgs{Prompt: "second", TaskID: &p.ID, Purpose: &purpose}))
				}
				return adaptiveReply("已完成许可范围内的样张。")
			}
			r := adaptiveTurn(t, s, c, "multi-images", "按授权生成样张")
			if r.Status != "completed" || r.ErrorCode != "" || f.images != limit || len(r.AssetIDs) != limit {
				t.Fatal("scope or batch violated", r.Status, r.ErrorCode, f.images, len(r.AssetIDs))
			}
			var operations int
			if err = s.Store.DB.QueryRow(`SELECT count(*) FROM agent_operations WHERE run_id=$1 AND kind='image'`, r.ID).Scan(&operations); err != nil || operations != limit {
				t.Fatal("duplicate/unauthorized image operation", operations, err)
			}
		})
	}
}

func TestPostgresMultiToolBoundYieldsProgressWithoutDispatch(t *testing.T) {
	s, f, c := newAdaptive(t)
	p := makeAdaptivePlan(t, s, f, c)
	calls := []Call{}
	for i := 0; i <= MaxToolCallsPerResponse; i++ {
		calls = append(calls, batchCall(fmt.Sprint(i), "get_job", map[string]string{"plan_id": p.ID}))
	}
	f.choose = func(Invocation, int) Outcome { return multiToolReply(calls...) }
	r := adaptiveTurn(t, s, c, "batch-bound", "继续补充方案")
	if r.Status != "completed" || r.ErrorCode != "" || !strings.Contains(r.Text, "步骤过多") || f.images != 0 {
		t.Fatal("oversized batch did not yield safely", r.Status, r.ErrorCode)
	}
	saved, err := s.Store.WorkPlan(context.Background(), 1, p.ID)
	if err != nil || saved.Revision != p.Revision {
		t.Fatal("unexecuted batch mutated draft", err)
	}
}

func TestPostgresMultiToolDurableQueueRestartsWithoutModelReplay(t *testing.T) {
	s, f, c := newAdaptive(t)
	p := makeAdaptivePlan(t, s, f, c)
	ctx := context.Background()
	start := f.turns
	f.choose = func(Invocation, int) Outcome {
		return multiToolReply(batchCall("read", "get_job", map[string]string{"plan_id": p.ID}), batchCall("ask", "ask_user", batchQuestion()))
	}
	_, err := s.Submit(ctx, 1, c.ID, "persist-batch", "", "补充介绍", nil, c.Version, "")
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Store.Claim(ctx, "before-restart")
	if err != nil {
		t.Fatal(err)
	}
	in := Invocation{OperationID: r.ID + "_model_0", RunID: r.ID, Kind: "text", Binding: Binding{UserID: r.UserID, KeyID: r.Config.ModelKeyID, GroupID: r.ModelGroupID, IP: r.IP}, Model: r.Config.Model, Input: r.History, Tools: true, ImageTools: true}
	op, out, err := s.operation(ctx, r, in, 0)
	if err != nil {
		t.Fatal(err)
	}
	response, err := ParseResponse(out.Body)
	if err != nil {
		t.Fatal(err)
	}
	r.History = append(r.History, response.Items...)
	r.Turn = 1
	for i := range response.Calls {
		response.Calls[i].ToolID = "tc_" + Hash(op.ID + response.Calls[i].ID)[:32]
	}
	r.PendingTools = &ToolBatch{OperationID: op.ID, Calls: response.Calls}
	r.Status = "resuming_model"
	if err = s.Store.Save(ctx, r, "model.completed", r.Public()); err != nil {
		t.Fatal(err)
	}
	if err = s.Store.Release(ctx, r); err != nil {
		t.Fatal(err)
	}
	// Advance only the isolated fixture's recovery scheduler; production backoff stays unchanged.
	if _, err = s.Store.DB.ExecContext(ctx, `UPDATE agent_task_outbox SET ready_at=now() WHERE run_id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	next, err := s.Store.Claim(ctx, "after-restart")
	if err != nil {
		t.Fatal(err)
	}
	restarted := New(s.Store, f, s.Root, true)
	restarted.process(ctx, next)
	if next.Status != "waiting_user" || f.turns != start+1 || f.images != 0 {
		t.Fatal("restart replayed model or lost batch", next.Status, f.turns, f.images)
	}
	var count int
	if err = s.Store.DB.QueryRow(`SELECT count(*) FROM agent_operations WHERE run_id=$1 AND kind='text'`, r.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("paid model replay", count, err)
	}
}

func TestPostgresMultiToolCancellationPrecedesQueuedDispatch(t *testing.T) {
	s, f, c := newAdaptive(t)
	ctx := context.Background()
	_, err := s.Submit(ctx, 1, c.ID, "cancel-batch", "", "取消排队步骤", nil, c.Version, "")
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Store.Claim(ctx, "cancel-worker")
	if err != nil {
		t.Fatal(err)
	}
	r.PendingTools = &ToolBatch{OperationID: r.ID + "_model_0", Calls: []Call{batchCall("queued", "inspect_assets", map[string]any{"asset_ids": []string{}})}}
	r.Status = "resuming_model"
	if err = s.Store.Save(ctx, r, "model.completed", r.Public()); err != nil {
		t.Fatal(err)
	}
	if err = s.Store.Cancel(ctx, 1, r.ID); err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	if r.Status != "cancelled" || f.turns != 0 || f.images != 0 {
		t.Fatal("cancelled queue was executed", r.Status, f.turns, f.images)
	}
}

func TestPostgresMultiToolQueuePayloadExpiresWithoutDeletingAudit(t *testing.T) {
	s, f, c := newAdaptive(t)
	ctx := context.Background()
	r := adaptiveTurn(t, s, c, "ttl-queue", "普通聊天")
	queue := ToolBatch{OperationID: r.ID + "_model_0", Calls: []Call{batchCall("private", "inspect_assets", map[string]any{"asset_ids": []string{"old-private-input"}})}}
	_, err := s.Store.DB.ExecContext(ctx, `UPDATE agent_runs SET created_at=now()-interval '25 hours',snapshot=(snapshot-'protocol_history')||jsonb_build_object('pending_tools',$2::jsonb) WHERE id=$1`, r.ID, jsonBytes(queue))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	var retainsQueue bool
	var cost string
	if err = s.Store.DB.QueryRowContext(ctx, `SELECT snapshot ? 'pending_tools',coalesce(snapshot->>'spent_usd','') FROM agent_runs WHERE id=$1`, r.ID).Scan(&retainsQueue, &cost); err != nil {
		t.Fatal(err)
	}
	if retainsQueue || cost != r.Spent {
		t.Fatal("queue retained or accounting lost", retainsQueue, cost)
	}
	if f.turns != 1 {
		t.Fatal("cleanup invoked model")
	}
}
