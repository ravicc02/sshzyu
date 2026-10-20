package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestPostgresTraceCommentaryRehydratesOnlyOwnedPublicHandoffs(t *testing.T) {
	st := isolatedStore(t)
	s, _ := setupRun(t, st, &testExecutor{})
	ctx := context.Background()
	r, err := st.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	id := r.ID + "_model_0"
	op, err := st.Prepare(ctx, r, Operation{ID: id, RunID: r.ID, Kind: "text", Step: 0, RequestHash: "test", Quote: "0.02"})
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"status":"completed","output":[{"type":"reasoning","encrypted_content":"private-encrypted-test","summary":[{"type":"summary_text","text":"公开思考摘要"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"先确认内容，再调用工具。"}]},{"type":"function_call","call_id":"p1","name":"generate_image","arguments":"{}"}]}`)
	op.Status = "succeeded"
	op.Cost = "0.01000000"
	if err = st.FinishOperation(ctx, r, op); err != nil {
		t.Fatal(err)
	}
	if err = s.writeOutcome(id, Outcome{Cost: op.Cost, Body: raw}); err != nil {
		t.Fatal(err)
	}
	trace := []TraceStep{{ID: id, Kind: "reasoning", Status: "completed"}}
	s.PopulateTraceCommentary(ctx, 1, r.ConversationID, trace)
	if trace[0].Commentary != "先确认内容，再调用工具。" {
		t.Fatal("public commentary not restored")
	}
	encoded, _ := json.Marshal(trace)
	if bytes.Contains(encoded, []byte("private-")) || bytes.Contains(encoded, []byte("encrypted_content")) || bytes.Contains(encoded, []byte("arguments")) {
		t.Fatal("private provider content leaked")
	}
	foreign := []TraceStep{{ID: id, Kind: "reasoning", Status: "completed"}}
	s.PopulateTraceCommentary(ctx, 2, r.ConversationID, foreign)
	if foreign[0].Commentary != "" {
		t.Fatal("cross-owner commentary leaked")
	}
	final := json.RawMessage(`{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"最终回答。"}]}]}`)
	if err = s.writeOutcome(id, Outcome{Cost: op.Cost, Body: final}); err != nil {
		t.Fatal(err)
	}
	trace[0].Commentary = ""
	s.PopulateTraceCommentary(ctx, 1, r.ConversationID, trace)
	if trace[0].Commentary != "" {
		t.Fatal("final reply duplicated as stage commentary")
	}
}

type commentaryExecutor struct{ testExecutor }

func (f *commentaryExecutor) Execute(ctx context.Context, in Invocation) (Outcome, error) {
	out, err := f.testExecutor.Execute(ctx, in)
	if err != nil || in.Kind != "text" {
		return out, err
	}
	result, e := ParseResponse(out.Body)
	if e != nil || len(result.Calls) == 0 {
		return out, err
	}
	var value map[string]json.RawMessage
	_ = json.Unmarshal(out.Body, &value)
	var items []json.RawMessage
	_ = json.Unmarshal(value["output"], &items)
	prefix := jsonBytes(map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": "阶段说明：现在开始调用生图工具。"}}})
	value["output"] = jsonBytes(append([]json.RawMessage{prefix}, items...))
	out.Body = jsonBytes(value)
	if in.OnProgress != nil {
		if e = in.OnProgress(StreamUpdate{Text: "阶段说明：现在开始调用生图工具。", Summary: "用户需求已确认。"}); e != nil {
			return out, e
		}
	}
	return out, nil
}
func TestPostgresTraceCommentaryPersistsStagesWithoutDuplicatingFinalReply(t *testing.T) {
	st := isolatedStore(t)
	s, _ := setupRun(t, st, &testExecutor{})
	s.Exec = &commentaryExecutor{}
	ctx := context.Background()
	r, err := st.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	if r.Status != "completed" || r.Text != "图片已生成。" {
		t.Fatal("final answer mixed with stage preamble", r.Status, r.Text)
	}
	if len(r.Trace) < 3 || r.Trace[0].Commentary != "阶段说明：现在开始调用生图工具。" || r.Trace[0].Summary != "用户需求已确认。" || r.Trace[len(r.Trace)-1].Commentary != "" {
		t.Fatal("phase commentary missing or final duplicated")
	}
	messages, err := st.Messages(ctx, 1, r.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	last := messages[len(messages)-1]
	if len(last.Trace) == 0 || last.Trace[0].Commentary != r.Trace[0].Commentary {
		t.Fatal("refresh lost commentary")
	}
	events, err := allTraceEvents(ctx, st, 1, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(events)
	if !bytes.Contains(encoded, []byte("commentary")) {
		t.Fatal("SSE omitted phase commentary")
	}
}
