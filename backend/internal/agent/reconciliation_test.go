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

func TestPostgresInvocationFailureImmediatelyReleasesAdmission(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{unknown: true}
	svc, created := setupRun(t, st, f)
	ctx := context.Background()
	r, err := st.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	svc.process(ctx, r)
	if r.Status != "failed" || r.ErrorCode != "UPSTREAM_RESULT_UNKNOWN" || r.ReconcileDeadline != nil {
		t.Fatal("failure still waits for reconciliation", r.Status, r.ErrorCode)
	}
	calls := f.calls
	if err = st.Release(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err = st.Claim(ctx, "other"); !errors.Is(err, ErrNotFound) {
		t.Fatal("failed request remains queued", err)
	}
	old, err := svc.Submit(ctx, 1, created.ConversationID, "msg-key", "127.0.0.1", "请生成一张图片", nil, 1, "1.00")
	if err != nil || old.ID != r.ID || f.calls != calls {
		t.Fatal("idempotent submission repeated dispatch", err)
	}
	if _, err = svc.Retry(ctx, 1, r.ID, "unsafe-retry", "127.0.0.1", 1, "1.00"); SafeCode(err) != "AGENT_RETRY_UNSETTLED" {
		t.Fatal("unknown request is replayable", err)
	}
	next, err := svc.Submit(ctx, 1, created.ConversationID, "new-message", "127.0.0.1", "新的对话", nil, 1, "1.00")
	if err != nil || next.ID == r.ID {
		t.Fatal("failed run kept user's slot", err)
	}
}

func TestPostgresImmediateFailureKeepsAssetsAndCharges(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{failResume: true}
	svc, _ := setupRun(t, st, f)
	ctx := context.Background()
	r, err := st.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	svc.process(ctx, r)
	if r.Status != "partial_success" || len(r.AssetIDs) != 1 || r.ReconcileDeadline != nil {
		t.Fatal("saved image lost or waiting", r.Status, r.ErrorCode)
	}
	asset, spent, cost, calls := r.AssetIDs[0], r.Spent, r.ImageCost, f.calls
	svc.process(ctx, r)
	if r.Spent != spent || r.ImageCost != cost || f.calls != calls {
		t.Fatal("terminal failure changed bills or replayed")
	}
	if _, err := svc.LoadAsset(ctx, 1, asset); err != nil {
		t.Fatal("paid image unavailable", err)
	}
	messages, err := st.Messages(ctx, 1, r.ConversationID)
	if err != nil || len(messages[len(messages)-1].Assets) != 1 {
		t.Fatal("partial output lost from history", err)
	}
	op, err := st.LookupOperation(ctx, r.ID+"_model_1")
	if err != nil || op.Status != "result_unknown" {
		t.Fatal("unsettled financial evidence erased", err)
	}
}

func TestPostgresLegacyReconciliationClosesWithoutRenewingOrReplaying(t *testing.T) {
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
	// Restore a persisted legacy active snapshot, even with plenty of time left.
	r.Status = "reconciling"
	r.ReconcileDeadline = nil
	r.Deadline = time.Now().Add(time.Hour)
	if err = st.Save(ctx, r, "", nil); err != nil {
		t.Fatal(err)
	}
	svc.process(ctx, r)
	if r.Status != "failed" || r.ErrorCode != "UPSTREAM_RESULT_UNKNOWN" || r.ReconcileDeadline != nil || f.calls != calls {
		t.Fatal("legacy waiter renewed or repeated request", r.Status, r.ErrorCode)
	}
}

func TestPostgresLegacyRecoveryReleaseStillFencesStaleWorkers(t *testing.T) {
	st := isolatedStore(t)
	svc, _ := setupRun(t, st, &testExecutor{})
	ctx := context.Background()
	r, err := st.Claim(ctx, "first")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second).Truncate(time.Microsecond)
	r.Status = "reconciling"
	r.ReconcileDeadline = &deadline
	if err = st.Save(ctx, r, "", nil); err != nil {
		t.Fatal(err)
	}
	if err = st.Release(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err = st.DB.Exec(`UPDATE agent_task_outbox SET ready_at=now() WHERE run_id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	current, err := st.Claim(ctx, "second")
	if err != nil {
		t.Fatal(err)
	}
	var before time.Time
	if err = st.DB.QueryRow(`SELECT ready_at FROM agent_task_outbox WHERE run_id=$1`, r.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	svc.finishReconciliationTimeout(r)
	_ = st.Release(ctx, r)
	saved, err := st.Run(ctx, 1, r.ID)
	if err != nil || saved.Terminal() || saved.Fence != current.Fence {
		t.Fatal("stale worker finalized task", err)
	}
	var after time.Time
	if err = st.DB.QueryRow(`SELECT ready_at FROM agent_task_outbox WHERE run_id=$1`, r.ID).Scan(&after); err != nil || !after.Equal(before) {
		t.Fatal("stale worker rescheduled lease", err)
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
func TestPostgresFailureDoesNotReenterRecoveryWorker(t *testing.T) {
	st := isolatedStore(t)
	f := &deadlineRecoveryExecutor{}
	svc := New(st, f, t.TempDir(), true)
	ctx := context.Background()
	cv, err := svc.Create(ctx, 1, "failure", Config{ModelKeyID: 1, Model: "text"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Submit(ctx, 1, cv.ID, "failure", "", "hello", nil, 1, ""); err != nil {
		t.Fatal(err)
	}
	r, err := st.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	svc.process(ctx, r)
	if r.Status != "failed" || f.calls != 1 || r.ReconcileDeadline != nil {
		t.Fatal("billing failure kept run active", r.Status)
	}
	f.block = true
	start := time.Now()
	svc.process(ctx, r)
	if time.Since(start) > time.Second || f.calls != 1 {
		t.Fatal("terminal request entered recovery or dispatched")
	}
	if _, err = st.Claim(ctx, "other"); !errors.Is(err, ErrNotFound) {
		t.Fatal("failure still claimable", err)
	}
}
