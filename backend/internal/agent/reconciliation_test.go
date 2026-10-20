package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestReconciliationDeadlineBoundary(t *testing.T) {
	s := New(nil, nil, "", false)
	if s.ReconcileTimeout != 2*time.Minute {
		t.Fatal("unexpected default timeout")
	}
	now := time.Now()
	if s.reconciliationExpired(&Run{}, now) {
		t.Fatal("missing deadline is not expired")
	}
	r := &Run{ReconcileDeadline: &now}
	if s.reconciliationExpired(r, now.Add(-time.Nanosecond)) || !s.reconciliationExpired(r, now) || !s.reconciliationExpired(r, now.Add(time.Nanosecond)) {
		t.Fatal("deadline boundary")
	}
}

func expireTestReconciliation(t *testing.T, st *Store, r *Run) {
	t.Helper()
	past := time.Now().Add(-time.Second)
	r.ReconcileDeadline = &past
	if err := st.Save(context.Background(), r, "", nil); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresReconciliationTimeoutReleasesAdmission(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{unknown: true}
	svc, created := setupRun(t, st, f)
	ctx := context.Background()
	r, err := st.Claim(ctx, "original-worker")
	if err != nil {
		t.Fatal(err)
	}
	before := time.Now()
	svc.process(ctx, r)
	if r.Status != "reconciling" || r.ReconcileDeadline == nil {
		t.Fatal("missing bounded recovery state")
	}
	deadline := *r.ReconcileDeadline
	if deadline.Before(before.Add(119*time.Second)) || deadline.After(time.Now().Add(121*time.Second)) {
		t.Fatal("unexpected reconciliation window")
	}
	saved, err := st.Run(ctx, 1, r.ID)
	if err != nil || saved.ReconcileDeadline == nil || !saved.ReconcileDeadline.Equal(deadline) {
		t.Fatal("deadline not persisted", err)
	}
	calls, spent := f.calls, r.Spent
	svc.process(ctx, r)
	if !r.ReconcileDeadline.Equal(deadline) || f.calls != calls {
		t.Fatal("recovery extended deadline or replayed request")
	}
	if err := st.Cancel(ctx, 1, r.ID); err != nil {
		t.Fatal(err)
	}
	svc.finishStopped(r, true)
	if !r.ReconcileDeadline.Equal(deadline) {
		t.Fatal("cancel reset the timer")
	}
	expireTestReconciliation(t, st, r)
	svc.process(ctx, r)
	if r.Status != "failed" || r.ErrorCode != ReconciliationTimeoutCode || r.Spent != spent || f.calls != calls {
		t.Fatalf("timeout lost state or replayed: status=%s code=%s calls=%d", r.Status, r.ErrorCode, f.calls)
	}
	op, err := st.LookupOperation(ctx, r.ID+"_image_0")
	if err != nil || op.Status != "result_unknown" {
		t.Fatal("unresolved provider audit trail was overwritten", err)
	}
	var complete bool
	if err := st.DB.QueryRow(`SELECT completed FROM agent_task_outbox WHERE run_id=$1`, r.ID).Scan(&complete); err != nil || !complete {
		t.Fatal("terminal outbox not completed", err)
	}
	var eventCode string
	if err := st.DB.QueryRow(`SELECT payload->>'error_code' FROM agent_run_events WHERE run_id=$1 AND type='run.failed' ORDER BY sequence DESC LIMIT 1`, r.ID).Scan(&eventCode); err != nil || eventCode != ReconciliationTimeoutCode {
		t.Fatal("timeout not emitted to SSE", err)
	}
	// The same idempotent request remains the original failure; a distinct new
	// message can acquire the user's slot immediately through the real SQL index.
	old, err := svc.Submit(ctx, 1, created.ConversationID, "msg-key", "127.0.0.1", "请生成一张图片", nil, 1, "1.00")
	if err != nil || old.ID != r.ID {
		t.Fatal("old request was regenerated", err)
	}
	next, err := svc.Submit(ctx, 1, created.ConversationID, "new-message", "127.0.0.1", "新的对话", nil, 1, "1.00")
	if err != nil || next.ID == r.ID {
		t.Fatal("user admission not released", err)
	}
	if f.calls != calls {
		t.Fatal("admission triggered an upstream replay")
	}
}

func TestPostgresReconciliationTimeoutKeepsAssetsAndCharges(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{failResume: true}
	svc, _ := setupRun(t, st, f)
	ctx := context.Background()
	r, err := st.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	svc.process(ctx, r)
	if r.Status != "reconciling" || len(r.AssetIDs) != 1 {
		t.Fatal("expected saved image and uncertain final response")
	}
	assetID, spent, imageCost, calls := r.AssetIDs[0], r.Spent, r.ImageCost, f.calls
	expireTestReconciliation(t, st, r)
	svc.process(ctx, r)
	if r.Status != "partial_success" || r.ErrorCode != ReconciliationTimeoutCode || r.Spent != spent || r.ImageCost != imageCost || f.calls != calls {
		t.Fatal("timeout changed paid results")
	}
	if _, err := svc.LoadAsset(ctx, 1, assetID); err != nil {
		t.Fatal("saved image became unavailable", err)
	}
	messages, err := st.Messages(ctx, 1, r.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	last := messages[len(messages)-1]
	if len(last.Assets) != 1 || last.Assets[0] != assetID {
		t.Fatal("partial result not retained in history")
	}
	op, err := st.LookupOperation(ctx, r.ID+"_model_1")
	if err != nil || op.Status != "result_unknown" {
		t.Fatal("unknown operation disappeared", err)
	}
}

func TestPostgresLegacyReconciliationUsesOriginalDeadline(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{unknown: true}
	svc, _ := setupRun(t, st, f)
	ctx := context.Background()
	r, err := st.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	svc.process(ctx, r)
	calls := f.calls
	// Model a snapshot from before this fix: no new reconciliation field, and
	// the original overall task deadline already expired while the worker was off.
	r.ReconcileDeadline = nil
	r.Deadline = time.Now().Add(-time.Hour)
	if err := st.Save(ctx, r, "", nil); err != nil {
		t.Fatal(err)
	}
	restarted := New(st, f, svc.Root, true)
	restarted.process(ctx, r)
	if r.Status != "failed" || r.ErrorCode != ReconciliationTimeoutCode || f.calls != calls {
		t.Fatal("legacy task was renewed or dispatched")
	}
	if !r.ReconcileDeadline.Equal(r.Deadline) {
		t.Fatal("legacy recovery ignored the original run limit")
	}
}

func TestPostgresReconciliationReleaseSchedulesDeadlineAndFences(t *testing.T) {
	st := isolatedStore(t)
	svc, _ := setupRun(t, st, &testExecutor{unknown: true})
	ctx := context.Background()
	r, err := st.Claim(ctx, "first")
	if err != nil {
		t.Fatal(err)
	}
	svc.process(ctx, r)
	deadline := time.Now().Add(2 * time.Second).Truncate(time.Microsecond)
	r.ReconcileDeadline = &deadline
	if err := st.Save(ctx, r, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := st.Release(ctx, r); err != nil {
		t.Fatal(err)
	}
	var ready time.Time
	if err := st.DB.QueryRow(`SELECT ready_at FROM agent_task_outbox WHERE run_id=$1`, r.ID).Scan(&ready); err != nil || ready.After(deadline) {
		t.Fatal("normal backoff delayed timeout", err)
	}
	if _, err := st.DB.Exec(`UPDATE agent_task_outbox SET ready_at=now() WHERE run_id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	current, err := st.Claim(ctx, "second")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DB.QueryRow(`SELECT ready_at FROM agent_task_outbox WHERE run_id=$1`, r.ID).Scan(&ready); err != nil {
		t.Fatal(err)
	}
	svc.finishReconciliationTimeout(r) // stale owner must not finalize the task
	if err := st.Release(ctx, r); err != nil {
		t.Fatal(err)
	}
	saved, err := st.Run(ctx, 1, r.ID)
	if err != nil || saved.Terminal() || saved.Fence != current.Fence {
		t.Fatal("stale worker finalized task", err)
	}
	var after time.Time
	if err := st.DB.QueryRow(`SELECT ready_at FROM agent_task_outbox WHERE run_id=$1`, r.ID).Scan(&after); err != nil || !after.Equal(ready) {
		t.Fatal("stale worker rescheduled current lease", err)
	}
}

type deadlineRecoveryExecutor struct {
	pendingBillingExecutor
	block bool
}

func (f *deadlineRecoveryExecutor) Recover(ctx context.Context, in Invocation, out Outcome) (Outcome, error) {
	if !f.block {
		return f.pendingBillingExecutor.Recover(ctx, in, out)
	}
	<-ctx.Done()
	return out, ctx.Err()
}
func TestPostgresReconciliationCancelsRecoveryAtDeadline(t *testing.T) {
	st := isolatedStore(t)
	f := &deadlineRecoveryExecutor{}
	svc := New(st, f, t.TempDir(), true)
	ctx := context.Background()
	cv, err := svc.Create(ctx, 1, "recovery", Config{ModelKeyID: 1, Model: "text", Budget: "1"}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Submit(ctx, 1, cv.ID, "recover", "127.0.0.1", "hello", nil, 1, "1"); err != nil {
		t.Fatal(err)
	}
	r, err := st.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	svc.process(ctx, r)
	if r.Status != "reconciling" || f.calls != 1 {
		t.Fatal("expected pending billing")
	}
	f.block = true
	deadline := time.Now().Add(50 * time.Millisecond)
	r.ReconcileDeadline = &deadline
	if err := st.Save(ctx, r, "", nil); err != nil {
		t.Fatal(err)
	}
	svc.process(ctx, r)
	if r.Status != "failed" || r.ErrorCode != ReconciliationTimeoutCode || f.calls != 1 {
		t.Fatal("recovery exceeded cutoff or repeated execution")
	}
	if time.Since(deadline) > time.Second {
		t.Fatal("recovery did not honor cancellation")
	}
	if _, err := st.Claim(ctx, "other"); !errors.Is(err, ErrNotFound) {
		t.Fatal("timed-out task is still claimable", err)
	}
}
