package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type checkpointOnlyExecutor struct {
	testExecutor
	recoveries int
}

func (f *checkpointOnlyExecutor) Execute(context.Context, Invocation) (Outcome, error) {
	panic("image recovery must never execute a new paid operation")
}
func (f *checkpointOnlyExecutor) Recover(_ context.Context, _ Invocation, out Outcome) (Outcome, error) {
	f.recoveries++
	var body struct {
		Data []struct {
			B64 string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Body, &body); err != nil || len(body.Data) != 1 {
		return out, ErrUnknown
	}
	data, err := base64.StdEncoding.DecodeString(body.Data[0].B64)
	if err != nil {
		return out, err
	}
	out.Image = data
	out.ImageFromBody = true
	return out, nil
}
func failedCheckpointFixture(t *testing.T) (*Service, *checkpointOnlyExecutor, *Run) {
	t.Helper()
	st := isolatedStore(t)
	s, _ := setupRun(t, st, &testExecutor{})
	f := &checkpointOnlyExecutor{}
	s.Exec = f
	ctx := context.Background()
	r, err := st.Claim(ctx, "original-worker")
	if err != nil {
		t.Fatal(err)
	}
	op, err := st.Prepare(ctx, r, Operation{ID: r.ID + "_image_0", RunID: r.ID, Kind: "image", Step: 0, RequestHash: "original", Quote: "0.20"})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Dispatch(ctx, r, op); err != nil {
		t.Fatal(err)
	}
	op.Status = "dispatching"
	op.Cost = "0.17000000"
	if err = st.FinishOperation(ctx, r, op); err != nil {
		t.Fatal(err)
	}
	out := Outcome{Cost: op.Cost, Body: jsonBytes(map[string]any{"data": []any{map[string]string{"b64_json": base64.StdEncoding.EncodeToString(testImage())}}})}
	if err = s.writeOutcome(op.ID, out); err != nil {
		t.Fatal(err)
	}
	r.PendingCall = &Call{ID: "provider-call", ToolID: "tc-failed-image", Name: "generate_image", Arguments: `{"prompt":"cat","image_size":"4K","aspect_ratio":"16:9"}`}
	r.startStep(r.PendingCall.ToolID, "tool", "generate_image", "running", nil)
	r.Text = "现在开始生成。"
	deadline := time.Now().Add(-time.Second)
	r.ReconcileDeadline = &deadline
	s.finishReconciliationTimeout(r)
	_ = st.Release(ctx, r)
	return s, f, r
}
func TestPostgresImageRecoveryRestoresWithoutGenerationOrNewBilling(t *testing.T) {
	s, f, old := failedCheckpointFixture(t)
	ctx := context.Background()
	messages, err := s.Store.Messages(ctx, 1, old.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	originalTime := messages[len(messages)-1].CreatedAt
	if !messages[len(messages)-1].CanRecoverImage {
		t.Fatal("known saved response not recoverable in history")
	}
	if _, err = s.QueueImageRecovery(ctx, 2, old.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-owner recovery", err)
	}
	queued, err := s.QueueImageRecovery(ctx, 1, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.QueueImageRecovery(ctx, 1, old.ID)
	if err != nil || replay.ID != queued.ID {
		t.Fatal("queue not idempotent", err)
	}
	r, err := s.Store.Claim(ctx, "recovery-worker")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	if r.Status != "completed" || r.RecoveredAt == nil || len(r.AssetIDs) != 1 || r.Spent != "0.17000000" || f.recoveries != 1 {
		t.Fatalf("%s %s %s %+v", r.Status, r.ErrorCode, r.Spent, r.AssetIDs)
	}
	after, err := s.QueueImageRecovery(ctx, 1, r.ID)
	if err != nil || after.ID != r.ID || f.recoveries != 1 {
		t.Fatal("completed recovery replayed", err)
	}
	var n int
	if err = s.Store.DB.QueryRowContext(ctx, `SELECT count(*) FROM agent_operations WHERE run_id=$1`, r.ID).Scan(&n); err != nil || n != 1 {
		t.Fatal("new paid operation added", n, err)
	}
	messages, err = s.Store.Messages(ctx, 1, r.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	m := messages[len(messages)-1]
	if !m.CreatedAt.Equal(originalTime) {
		t.Fatal("recovery changed original message time")
	}
	if len(m.Assets) != 1 || m.ErrorCode != "" || m.RunStatus != "completed" || m.CanRecoverImage {
		t.Fatal("history not repaired", m, err)
	}
	stored, err := s.readOutcome(r.ID + "_image_0")
	if err != nil || len(stored.Image) != 0 || len(stored.Body) == 0 || !stored.ImageFromBody {
		t.Fatal("duplicate image stored in checkpoint", err)
	}
}
func TestPostgresImageCapacityAllowsExistingResultsAndRemainsBounded(t *testing.T) {
	st := isolatedStore(t)
	s, _ := setupRun(t, st, &testExecutor{})
	ctx := context.Background()
	r, err := st.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	a := Asset{ID: ID("asset"), UserID: 1, ConversationID: r.ConversationID, Key: "history.attachment", MIME: "text/plain", Size: 40 << 20, SHA: "test", Expires: time.Now().Add(time.Hour)}
	if err = st.PutAsset(ctx, a, ""); err != nil {
		t.Fatal(err)
	}
	op, err := st.Prepare(ctx, r, Operation{ID: r.ID + "_image_0", Kind: "image", Step: 0, RequestHash: "test", Quote: "0.20"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.reserveCheckpoint(ctx, 1, op); err != nil {
		t.Fatal("normal 4K image blocked by historical results", err)
	}
	full := Asset{ID: ID("asset"), UserID: 1, ConversationID: r.ConversationID, Key: "overflow.attachment", MIME: "text/plain", Size: storageQuota, SHA: "test", Expires: time.Now().Add(time.Hour)}
	if err = st.PutAsset(ctx, full, ""); SafeCode(err) != "AGENT_STORAGE_QUOTA_EXCEEDED" {
		t.Fatal("quota became unbounded", err)
	}
}

type inquiryExecutor struct {
	testExecutor
	imageTools bool
}

func (f *inquiryExecutor) Execute(ctx context.Context, in Invocation) (Outcome, error) {
	f.imageTools = in.ImageTools
	if in.Kind == "image" {
		return f.testExecutor.Execute(ctx, in)
	}
	args := `{"prompt":"cat","model":null,"image_size":"1K","aspect_ratio":"1:1","reference_asset_ids":null}`
	return Outcome{Cost: "0.01", Body: jsonBytes(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": "现在开始生成。"}}}, map[string]string{"type": "function_call", "call_id": "bad-call", "name": "generate_image", "arguments": args}}})}, nil
}
func TestPostgresImageStatusInquiryDoesNotAuthorizeAnotherImage(t *testing.T) {
	st := isolatedStore(t)
	s, _ := setupRun(t, st, &testExecutor{})
	ctx := context.Background()
	// Mark the initial fixture complete, then submit the user's diagnostic query.
	first, err := st.Claim(ctx, "first")
	if err != nil {
		t.Fatal(err)
	}
	first.Status = "completed"
	if err = st.Save(ctx, first, "run.completed", first.Public()); err != nil {
		t.Fatal(err)
	}
	_ = st.Release(ctx, first)
	f := &inquiryExecutor{}
	s.Exec = f
	_, err = s.Submit(ctx, 1, first.ConversationID, "diagnostic", "", "咋回事", nil, 1, "1.00")
	if err != nil {
		t.Fatal(err)
	}
	r, err := st.Claim(ctx, "inquiry")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	if f.imageTools || f.images != 0 || r.Status != "completed" || r.ErrorCode != "" || r.Text == "" {
		t.Fatal("diagnostic query triggered image retry or kept false promise", r.ErrorCode, r.Text)
	}
	if !ImageStatusInquiry("咋回事") || ImageStatusInquiry("请重新生成一张图片") {
		t.Fatal("inquiry intent guard")
	}
}

type invalidImageExecutor struct{ testExecutor }

func (f *invalidImageExecutor) Execute(ctx context.Context, in Invocation) (Outcome, error) {
	if in.Kind != "image" {
		return f.testExecutor.Execute(ctx, in)
	}
	f.calls++
	f.images++
	return Outcome{Cost: "0.17000000", Body: json.RawMessage(`{"data":[]}`)}, nil
}
func (f *invalidImageExecutor) Recover(_ context.Context, in Invocation, out Outcome) (Outcome, error) {
	if in.Kind == "image" {
		return out, &ResultFailure{Code: "AGENT_IMAGE_RESULT_INVALID"}
	}
	return out, nil
}
func TestPostgresImageInvalidResultFailsWithoutUnknownWaitAndKeepsCharge(t *testing.T) {
	st := isolatedStore(t)
	s, _ := setupRun(t, st, &testExecutor{})
	f := &invalidImageExecutor{}
	s.Exec = f
	ctx := context.Background()
	r, err := st.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	if r.Status != "failed" || r.ErrorCode != "AGENT_IMAGE_RESULT_INVALID" || r.Spent != "0.18000000" || f.images != 1 || r.Text != "" {
		t.Fatalf("%s %s %s %d", r.Status, r.ErrorCode, r.Spent, f.images)
	}
	op, err := st.LookupOperation(ctx, r.ID+"_image_0")
	if err != nil || op.Status != "failed" || op.Cost != "0.17000000" {
		t.Fatal("charge lost", op, err)
	}
	if _, err = s.readOutcome(op.ID); err != nil {
		t.Fatal("accepted response lost", err)
	}
}
