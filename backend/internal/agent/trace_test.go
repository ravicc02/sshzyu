package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPublicReasoningSummaryOnly(t *testing.T) {
	raw := json.RawMessage(`{"status":"completed","output":[{"type":"reasoning","encrypted_content":"private-encrypted-payload","content":[{"type":"reasoning_text","text":"private-chain"}],"summary":[{"type":"summary_text","text":"公开摘要"},{"type":"reasoning_text","text":"private-chain"}]},{"type":"message","content":[{"type":"output_text","text":"最终回复"}]}]}`)
	parsed, err := ParseResponse(raw)
	if err != nil || parsed.Summary != "公开摘要" || parsed.Text != "最终回复" {
		t.Fatal("public summary parsing", err)
	}
	r := &Run{ID: "run", History: parsed.Items}
	r.startStep("model-0", "reasoning", "text", "running", nil)
	r.completeStep("model-0", "0.01", parsed.Summary, nil)
	public, err := json.Marshal(r.Public())
	if err != nil || !bytes.Contains(public, []byte("公开摘要")) || bytes.Contains(public, []byte("private-")) || bytes.Contains(public, []byte("encrypted_content")) {
		t.Fatal("private protocol leaked", err)
	}
	noSummary, err := ParseResponse(json.RawMessage(`{"status":"completed","output":[{"type":"reasoning","encrypted_content":"private"}]}`))
	if err != nil || noSummary.Summary != "" {
		t.Fatal("invented summary")
	}
}
func TestReasoningSummaryBound(t *testing.T) {
	text := strings.Repeat("思", 13000)
	raw, _ := json.Marshal(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "reasoning", "summary": []any{map[string]string{"type": "summary_text", "text": text}}}}})
	parsed, err := ParseResponse(raw)
	if err != nil || len([]rune(parsed.Summary)) != 12000 {
		t.Fatal("unbounded or invalid UTF-8 summary", err)
	}
}
func TestTraceStepIdentityAndTerminalStates(t *testing.T) {
	r := &Run{ID: "run"}
	r.startStep("m0", "reasoning", "text", "running", nil)
	first := r.Trace[0].StartedAt
	r.pendingSteps("reconciling", "UPSTREAM_RESULT_UNKNOWN")
	r.startStep("m0", "reasoning", "text", "running", nil)
	if len(r.Trace) != 1 || r.Trace[0].Status != "reconciling" || !r.Trace[0].StartedAt.Equal(first) {
		t.Fatal("recovery duplicated step or reset clock")
	}
	r.completeStep("m0", "0.01", "摘要", nil)
	r.startStep("tool", "tool", "generate_image", "queued", traceArguments(ToolArgs{Prompt: "cat"}, "image"))
	queued := time.Now().Add(-time.Minute)
	r.Trace[1].StartedAt = queued
	r.startStep("tool", "tool", "generate_image", "running", nil)
	if !r.Trace[1].StartedAt.After(queued) {
		t.Fatal("execution clock includes queue time")
	}
	r.pendingSteps("failed", ReconciliationTimeoutCode)
	if r.Trace[0].Status != "completed" || r.Trace[0].Summary != "摘要" || r.Trace[1].Status != "failed" || r.Trace[1].EndedAt == nil {
		t.Fatal("failure changed confirmed steps")
	}
	fields, err := traceEventPayload(r, map[string]string{"tool_call_id": "tool"})
	if err != nil || !bytes.Contains(fields, []byte(`"run_id":"run"`)) || !bytes.Contains(fields, []byte(`"trace"`)) {
		t.Fatal("event trace envelope", err)
	}
	if _, err := traceEventPayload(r, nil); err != nil {
		t.Fatal(err)
	}
}

type traceExecutor struct{ testExecutor }

func (f *traceExecutor) Execute(ctx context.Context, in Invocation) (Outcome, error) {
	out, err := f.testExecutor.Execute(ctx, in)
	if err != nil || in.Kind != "text" {
		return out, err
	}
	var body map[string]json.RawMessage
	if err = json.Unmarshal(out.Body, &body); err != nil {
		return out, err
	}
	var output []json.RawMessage
	if err = json.Unmarshal(body["output"], &output); err != nil {
		return out, err
	}
	reasoning := json.RawMessage(`{"type":"reasoning","encrypted_content":"private-encrypted-payload","content":[{"type":"reasoning_text","text":"private-chain"}],"summary":[{"type":"summary_text","text":"已核对需求，准备下一步。"}]}`)
	body["output"], _ = json.Marshal(append([]json.RawMessage{reasoning}, output...))
	out.Body, err = json.Marshal(body)
	return out, err
}
func TestPostgresProcessTraceHistoryAndPrivacy(t *testing.T) {
	st := isolatedStore(t)
	f := &traceExecutor{}
	svc, _ := setupRun(t, st, &f.testExecutor)
	svc.Exec = f
	ctx := context.Background()
	r, err := st.Claim(ctx, "trace-worker")
	if err != nil {
		t.Fatal(err)
	}
	svc.process(ctx, r)
	if r.Status != "completed" || len(r.Trace) != 3 || f.calls != 3 || f.images != 1 {
		t.Fatalf("trace lifecycle: status=%s steps=%d calls=%d", r.Status, len(r.Trace), f.calls)
	}
	for _, step := range r.Trace {
		if step.Status != "completed" || step.EndedAt == nil || step.Cost == "" {
			t.Fatal("incomplete trace")
		}
	}
	if r.Trace[0].Summary != "已核对需求，准备下一步。" || r.Trace[2].Summary == "" {
		t.Fatal("lost public reasoning summaries")
	}
	tool := r.Trace[1]
	if tool.Name != "generate_image" || tool.Arguments == nil || tool.Arguments.Prompt != "cat" || tool.Arguments.Model != "image" || tool.Arguments.ImageSize != "1K" || tool.Arguments.AspectRatio != "1:1" || len(tool.AssetIDs) != 1 {
		t.Fatal("tool projection is not the executed input/result")
	}
	saved, err := st.Run(ctx, 1, r.ID)
	if err != nil || len(saved.Trace) != 3 {
		t.Fatal("snapshot trace not durable", err)
	}
	messages, err := st.Messages(ctx, 1, r.ConversationID)
	if err != nil || len(messages[len(messages)-1].Trace) != 3 {
		t.Fatal("history trace not durable", err)
	}
	public, _ := json.Marshal(messages[len(messages)-1])
	if bytes.Contains(public, []byte("private-")) || bytes.Contains(public, []byte("encrypted_content")) {
		t.Fatal("history exposed private protocol")
	}
	events, err := allTraceEvents(ctx, st, 1, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, event := range events {
		seen[event.Type] = true
		if bytes.Contains(event.Data, []byte("private-")) || bytes.Contains(event.Data, []byte("encrypted_content")) {
			t.Fatal("event exposed private reasoning")
		}
		if event.Type == "run.started" {
			continue
		}
		var payload struct {
			RunID string      `json:"run_id"`
			Trace []TraceStep `json:"trace"`
		}
		if err = json.Unmarshal(event.Data, &payload); err != nil || payload.RunID != r.ID || len(payload.Trace) == 0 {
			t.Fatal("trace missing from SSE payload", err)
		}
	}
	for _, kind := range []string{"model.started", "tool.call.created", "tool.execution.started", "asset.created", "run.completed"} {
		if !seen[kind] {
			t.Fatal("missing event", kind)
		}
	}
}
func TestPostgresTraceReconciliationAndTimeoutDoNotReplay(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{unknown: true}
	svc, _ := setupRun(t, st, f)
	ctx := context.Background()
	r, err := st.Claim(ctx, "trace-worker")
	if err != nil {
		t.Fatal(err)
	}
	svc.process(ctx, r)
	if len(r.Trace) != 2 || r.Trace[0].Status != "completed" || r.Trace[1].Status != "failed" || r.Trace[1].Cost != "" {
		t.Fatal("pending state is misleading")
	}
	start, calls := r.Trace[1].StartedAt, f.calls
	svc.process(ctx, r)
	if len(r.Trace) != 2 || !r.Trace[1].StartedAt.Equal(start) || f.calls != calls {
		t.Fatal("recovery duplicated tool invocation")
	}
	events, err := allTraceEvents(ctx, st, 1, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	executions := 0
	for _, event := range events {
		if event.Type == "tool.execution.started" {
			executions++
		}
	}
	if executions != 1 {
		t.Fatal("recovery was misrepresented as another tool execution")
	}
	expireTestReconciliation(t, st, r)
	svc.process(ctx, r)
	if r.Trace[0].Status != "completed" || r.Trace[1].Status != "failed" || r.Trace[1].ErrorCode != "UPSTREAM_RESULT_UNKNOWN" || r.Trace[1].EndedAt == nil || f.calls != calls {
		t.Fatal("timeout lost the tool audit trail")
	}
}
func TestPostgresTraceCancellationBeforeDispatch(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{}
	svc, _ := setupRun(t, st, f)
	ctx := context.Background()
	r, err := st.Claim(ctx, "trace-worker")
	if err != nil {
		t.Fatal(err)
	}
	r.startStep(r.ID+"_model_0", "reasoning", "text", "running", nil)
	if err = st.Save(ctx, r, "model.started", map[string]string{"run_id": r.ID}); err != nil {
		t.Fatal(err)
	}
	if err = st.Cancel(ctx, 1, r.ID); err != nil {
		t.Fatal(err)
	}
	svc.process(ctx, r)
	if r.Status != "cancelled" || r.Trace[0].Status != "cancelled" || r.Trace[0].EndedAt == nil || f.calls != 0 {
		t.Fatal("cancelled thinking state incorrect")
	}
}

// Audit assertions read all replay pages; live delivery retains bounded batches.
func allTraceEvents(ctx context.Context, store *Store, user int64, id string) ([]Event, error) {
	var all []Event
	var after int64
	for len(all) < 1000 {
		events, _, err := store.PollEvents(ctx, user, id, after)
		if err != nil {
			return nil, err
		}
		if len(events) == 0 {
			return all, nil
		}
		all = append(all, events...)
		after = events[len(events)-1].Sequence
		if !events[len(events)-1].HasMore {
			return all, nil
		}
	}
	return nil, errors.New("unexpectedly unbounded fixture replay")
}
