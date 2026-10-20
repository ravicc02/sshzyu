package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type askingExecutor struct {
	testExecutor
	summary bool
}

func (f *askingExecutor) Execute(ctx context.Context, in Invocation) (Outcome, error) {
	f.calls++
	if in.OnProgress != nil {
		if e := in.OnProgress(StreamUpdate{Text: "正在确认", Summary: "需要补充图片主题和规格。"}); e != nil {
			return Outcome{}, e
		}
	}
	if f.calls == 1 {
		args := `{"question":"你希望生成什么照片？","fields":[{"id":"subject","label":"画面内容","options":[],"required":true},{"id":"resolution","label":"分辨率","options":["1K","2K","4K"],"required":true}]}`
		b, _ := json.Marshal(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "function_call", "call_id": "ask-1", "name": "ask_user", "arguments": args}}})
		return Outcome{Body: b, Cost: "0.01000000"}, nil
	}
	return Outcome{Body: json.RawMessage(`{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"已收到你的要求。"}]}]}`), Cost: "0.01000000"}, nil
}
func TestRequiresUserQuestionOnlyMatchesExplicitAskIntent(t *testing.T) {
	for _, text := range []string{"我想和你讨论一个想法，请先问我几个问题，帮我把思路理清楚。", "ask me a few questions first"} {
		if !RequiresUserQuestion(text) {
			t.Fatalf("missed explicit ask intent: %s", text)
		}
	}
	for _, text := range []string{"帮我分析这个想法", "生成一张照片", "你觉得这个方案怎么样"} {
		if RequiresUserQuestion(text) {
			t.Fatalf("false positive: %s", text)
		}
	}
}

func TestAskUserValidationAndPrivateProgressHash(t *testing.T) {
	a := AskUserArgs{Question: "主题？", Fields: []QuestionField{{ID: "subject", Label: "主题", Required: true}}}
	raw, _ := json.Marshal(a)
	if _, e := ParseAskUser(raw); e != nil {
		t.Fatal(e)
	}
	a.Fields = append(a.Fields, a.Fields[0])
	raw, _ = json.Marshal(a)
	if _, e := ParseAskUser(raw); e == nil {
		t.Fatal("duplicate field accepted")
	}
	first := Invocation{Kind: "text"}
	second := first
	second.OnProgress = func(StreamUpdate) error { return nil }
	if Hash(first) != Hash(second) {
		t.Fatal("callback affects durable operation identity")
	}
}
func TestPostgresAskUserWaitAnswerAndDuplicate(t *testing.T) {
	st := isolatedStore(t)
	f := &askingExecutor{}
	svc := New(st, f, t.TempDir(), true)
	ctx := context.Background()
	cv, e := svc.Create(ctx, 1, "澄清测试", Config{ModelKeyID: 1, Model: "text", Budget: "1"}, "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	r, e := svc.Submit(ctx, 1, cv.ID, "ask-test", "127.0.0.1", "生成一张照片", nil, 1, "1")
	if e != nil {
		t.Fatal(e)
	}
	claimed, e := st.Claim(ctx, "worker")
	if e != nil {
		t.Fatal(e)
	}
	svc.process(ctx, claimed)
	_ = st.Release(ctx, claimed)
	waiting, e := st.Run(ctx, 1, r.ID)
	if e != nil || waiting.Status != "waiting_user" || waiting.Question == nil || f.calls != 1 || f.images != 0 {
		t.Fatal(waiting, e)
	}
	if _, e = st.Claim(ctx, "worker"); !errors.Is(e, ErrNotFound) {
		t.Fatal("waiting task got redispatched", e)
	}
	qid := waiting.Question.ID
	if _, e = st.Answer(ctx, 2, r.ID, qid, map[string]string{"subject": "cat", "resolution": "1K"}); !errors.Is(e, ErrNotFound) {
		t.Fatal("foreign answer", e)
	}
	if _, e = st.Answer(ctx, 1, r.ID, qid, map[string]string{"subject": "cat"}); e == nil {
		t.Fatal("missing required answer")
	}
	answers := map[string]string{"subject": "cat", "resolution": "1K"}
	if _, e = st.Answer(ctx, 1, r.ID, qid, answers); e != nil {
		t.Fatal(e)
	}
	if _, e = st.Answer(ctx, 1, r.ID, qid, answers); e != nil {
		t.Fatal("idempotent duplicate", e)
	}
	if _, e = st.Answer(ctx, 1, r.ID, qid, map[string]string{"subject": "dog", "resolution": "1K"}); !errors.Is(e, ErrConflict) {
		t.Fatal("conflicting duplicate", e)
	}
	msgs, err := st.Messages(ctx, 1, r.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	var answerMessage Message
	count := 0
	for _, msg := range msgs {
		if msg.Kind == "question_answer" {
			answerMessage = msg
			count++
		}
	}
	if count != 1 || answerMessage.QuestionID != qid || len(answerMessage.Answers) != 2 || strings.Contains(answerMessage.Text, waiting.Question.Question) {
		t.Fatal("answer must be one typed record without repeating the question", answerMessage)
	}
	// Existing records are classified by their persisted completed ask_user trace.
	snapshot, err := st.Run(ctx, 1, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	var legacyText string
	for _, step := range snapshot.Trace {
		if step.Name == "ask_user" {
			legacyText = step.Summary
		}
	}
	if _, err = st.DB.ExecContext(ctx, `UPDATE agent_messages SET content=(content-'kind'-'question_id'-'answers')||jsonb_build_object('text',$2::text) WHERE id=$1`, answerMessage.ID, legacyText); err != nil {
		t.Fatal(err)
	}
	msgs, err = st.Messages(ctx, 1, r.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	count = 0
	for _, msg := range msgs {
		if msg.Kind == "question_answer" {
			count++
			if msg.Text != legacyText {
				t.Fatal("legacy body changed")
			}
		}
	}
	if count != 1 {
		t.Fatal("legacy answer presented as new request")
	}
	claimed, e = st.Claim(ctx, "worker2")
	if e != nil {
		t.Fatal(e)
	}
	svc.process(ctx, claimed)
	_ = st.Release(ctx, claimed)
	done, e := st.Run(ctx, 1, r.ID)
	if e != nil || done.Status != "completed" || done.Question != nil || f.calls != 2 || done.Spent != "0.02000000" {
		t.Fatal(done, e)
	}
	if done.Text != "已收到你的要求。" {
		t.Fatal("streamed text duplicated", done.Text)
	}
	if _, ok := done.Public()["answer_hash"]; ok {
		t.Fatal("answer fingerprint exposed")
	}
}
func TestPostgresConversationManagementAtomicOwnership(t *testing.T) {
	st := isolatedStore(t)
	ctx := context.Background()
	f := &testExecutor{}
	svc := New(st, f, t.TempDir(), true)
	cv, e := svc.Create(ctx, 1, "old", Config{ModelKeyID: 1, Model: "text", Budget: "1"}, "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	if e = st.Manage(ctx, 2, []string{cv.ID}, "delete", ""); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	if e = st.Manage(ctx, 1, []string{cv.ID, "missing"}, "archive", ""); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	got, _ := st.Conversation(ctx, 1, cv.ID)
	if got.ArchivedAt != nil {
		t.Fatal("partial bulk update")
	}
	for _, action := range []string{"rename", "archive", "restore"} {
		if e = st.Manage(ctx, 1, []string{cv.ID}, action, "new"); e != nil {
			t.Fatal(e)
		}
	}
	got, _ = st.Conversation(ctx, 1, cv.ID)
	if got.Title != "new" || got.ArchivedAt != nil {
		t.Fatal(got)
	}
	r, e := svc.Submit(ctx, 1, cv.ID, "active-test", "127.0.0.1", "test", nil, 1, "1")
	if e != nil {
		t.Fatal(e)
	}
	if e = st.Manage(ctx, 1, []string{cv.ID}, "delete", ""); !errors.Is(e, ErrBusy) {
		t.Fatal("deleted active task", e)
	}
	claimed, _ := st.Claim(ctx, "worker")
	claimed.Status = "cancelled"
	_ = st.Save(ctx, claimed, "run.cancelled", claimed.Public())
	_ = st.Release(ctx, claimed)
	if e = st.Manage(ctx, 1, []string{cv.ID}, "delete", ""); e != nil {
		t.Fatal(e)
	}
	if _, e = st.Conversation(ctx, 1, cv.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("deleted conversation visible")
	}
	if _, e = st.Run(ctx, 1, r.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("deleted run still readable", e)
	}
	var count int
	if e = st.DB.QueryRowContext(ctx, `SELECT count(*) FROM agent_runs WHERE id=$1`, r.ID).Scan(&count); e != nil || count != 1 {
		t.Fatal("billing audit removed", e)
	}
	list, e := st.Conversations(ctx, 1)
	if e != nil || len(list) != 0 {
		t.Fatal(list, e)
	}
}
func TestPostgresWaitingUserCancelAndTimeout(t *testing.T) {
	for _, cancel := range []bool{true, false} {
		st := isolatedStore(t)
		ctx := context.Background()
		f := &askingExecutor{}
		svc := New(st, f, t.TempDir(), true)
		cv, _ := svc.Create(ctx, 1, "wait", Config{ModelKeyID: 1, Model: "text", Budget: "1"}, "127.0.0.1")
		r, e := svc.Submit(ctx, 1, cv.ID, "wait-test", "127.0.0.1", "photo", nil, 1, "1")
		if e != nil {
			t.Fatal(e)
		}
		claimed, _ := st.Claim(ctx, "worker")
		svc.process(ctx, claimed)
		if cancel {
			_ = st.Release(ctx, claimed)
			if e = st.Cancel(ctx, 1, r.ID); e != nil {
				t.Fatal(e)
			}
			claimed, e = st.Claim(ctx, "worker2")
			if e != nil {
				t.Fatal(e)
			}
		} else {
			claimed.Deadline = time.Now().Add(-time.Second)
		}
		svc.process(ctx, claimed)
		done, _ := st.Run(ctx, 1, r.ID)
		expected := "failed"
		if cancel {
			expected = "cancelled"
		}
		if done.Status != expected || f.calls != 1 {
			t.Fatal(done.Status, expected, f.calls)
		}
	}
}

func TestPostgresStreamRecoveryDoesNotDuplicateText(t *testing.T) {
	st := isolatedStore(t)
	ctx := context.Background()
	f := &askingExecutor{}
	f.calls = 1
	svc := New(st, f, t.TempDir(), true)
	cv, e := svc.Create(ctx, 1, "stream recovery", Config{ModelKeyID: 1, Model: "text", Budget: "1"}, "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	created, e := svc.Submit(ctx, 1, cv.ID, "stream-recovery", "127.0.0.1", "hello", nil, 1, "1")
	if e != nil {
		t.Fatal(e)
	}
	old, e := st.Claim(ctx, "before-crash")
	if e != nil {
		t.Fatal(e)
	}
	in := Invocation{OperationID: old.ID + "_model_0", RunID: old.ID, Kind: "text", Binding: Binding{UserID: 1, KeyID: 1, GroupID: old.ModelGroupID, IP: old.IP}, Model: "text", Input: old.History, Tools: true}
	if _, _, e = svc.operation(ctx, old, in, 0); e != nil {
		t.Fatal(e)
	}
	old.StreamOperation = in.OperationID
	old.StreamPrefix = ""
	old.Text = "已收到"
	old.Status = "running"
	if e = st.Save(ctx, old, "model.delta", old.Public()); e != nil {
		t.Fatal(e)
	}
	_ = st.Release(ctx, old)
	_, _ = st.DB.Exec(`UPDATE agent_task_outbox SET ready_at=now()`)
	resumed, e := st.Claim(ctx, "after-crash")
	if e != nil {
		t.Fatal(e)
	}
	svc.process(ctx, resumed)
	done, e := st.Run(ctx, 1, created.ID)
	if e != nil || done.Status != "completed" || done.Text != "已收到你的要求。" || f.calls != 2 {
		t.Fatal(done, e, f.calls)
	}
}
