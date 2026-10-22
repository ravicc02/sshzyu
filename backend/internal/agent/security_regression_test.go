package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type chargedBrokenExecutor struct {
	testExecutor
	recoveries int
}

func (f *chargedBrokenExecutor) Execute(context.Context, Invocation) (Outcome, error) {
	f.calls++
	return Outcome{Cost: "0.01000000", BillingPending: false}, errors.New("stream interrupted after charge")
}
func (f *chargedBrokenExecutor) Recover(_ context.Context, _ Invocation, out Outcome) (Outcome, error) {
	f.recoveries++
	out.Cost = "0.01000000"
	out.BillingPending = false
	return out, ErrUnknown
}

func TestPostgresSecurityChargedFailureSurvivesMissingCheckpointAndTimeout(t *testing.T) {
	st := isolatedStore(t)
	s, created := setupRun(t, st, &testExecutor{})
	f := &chargedBrokenExecutor{}
	s.Exec = f
	ctx := context.Background()
	r, err := st.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	got, err := st.Run(ctx, 1, created.ID)
	if err != nil || got.Status != "failed" || AddAmount(got.Spent, "0") != "0.01000000" {
		t.Fatal("charge lost on stream failure", got, err)
	}
	checkpoint := filepath.Join(s.Root, r.ID+"_model_0.result.json")
	if err = os.Remove(checkpoint); err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	if f.recoveries != 0 || f.calls != 1 || AddAmount(r.Spent, "0") != "0.01000000" {
		t.Fatal("recovery must read ledger without replay", f.calls, f.recoveries, r.Spent)
	}
	deadline := time.Now().Add(-time.Second)
	r.ReconcileDeadline = &deadline
	s.process(ctx, r)
	got, err = st.Run(ctx, 1, r.ID)
	if err != nil || got.Status != "failed" || AddAmount(got.Spent, "0") != "0.01000000" {
		t.Fatal("timeout erased charge", got, err)
	}
	var cost, billingID string
	if err = st.DB.QueryRowContext(ctx, `SELECT cost::text,billing_request_id FROM agent_operations WHERE run_id=$1`, r.ID).Scan(&cost, &billingID); err != nil || AddAmount(cost, "0") != "0.01000000" || billingID != "agent:"+r.ID+"_model_0" {
		t.Fatal(cost, billingID, err)
	}
}
func TestPostgresSecurityStorageReservationStopsDispatchAndUpload(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{}
	s, _ := setupRun(t, st, f)
	ctx := context.Background()
	r, err := st.Claim(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	a := Asset{ID: ID("asset"), UserID: 1, ConversationID: r.ConversationID, Key: "quota.attachment", MIME: "text/plain", Size: storageQuota - (3 << 20), SHA: "test", Expires: time.Now().Add(time.Hour)}
	if err = st.PutAsset(ctx, a, ""); err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	if f.calls != 0 || r.ErrorCode != "AGENT_STORAGE_QUOTA_EXCEEDED" {
		t.Fatal("dispatched without checkpoint capacity", f.calls, r.ErrorCode)
	}
}
func TestPostgresSecurityCheckpointQuotaIncludesPrivateResults(t *testing.T) {
	st := isolatedStore(t)
	s, _ := setupRun(t, st, &testExecutor{})
	ctx := context.Background()
	r, _ := st.Claim(ctx, "worker")
	op, err := st.Prepare(ctx, r, Operation{ID: r.ID + "_model_0", Kind: "text", Step: 0, RequestHash: "test", Quote: "0.02"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.reserveCheckpoint(ctx, 1, op); err != nil {
		t.Fatal(err)
	}
	out := Outcome{Cost: "0", Body: json.RawMessage(`{"output":[]}`)}
	if err = s.writeOutcome(op.ID, out); err != nil {
		t.Fatal(err)
	}
	var bytes, reserve int64
	if err = st.DB.QueryRowContext(ctx, `SELECT byte_size,reserved_bytes FROM agent_checkpoints WHERE operation_id=$1`, op.ID).Scan(&bytes, &reserve); err != nil || bytes <= 0 || reserve < 4<<20 {
		t.Fatal(bytes, reserve, err)
	}
	a := Asset{ID: ID("asset"), UserID: 1, ConversationID: r.ConversationID, Key: "overflow.attachment", MIME: "text/plain", Size: storageQuota - (3 << 20), SHA: "test", Expires: time.Now().Add(time.Hour)}
	if err = st.PutAsset(ctx, a, ""); SafeCode(err) != "AGENT_STORAGE_QUOTA_EXCEEDED" {
		t.Fatal("upload ignored reserved checkpoint", err)
	}
}
func TestPostgresSecurityCleanupProgressesPastFirstHundred(t *testing.T) {
	st := isolatedStore(t)
	s, _ := setupRun(t, st, &testExecutor{})
	ctx := context.Background()
	r, _ := st.Claim(ctx, "worker")
	for i := 0; i < 101; i++ {
		op, err := st.Prepare(ctx, r, Operation{ID: fmt.Sprintf("%s_model_%d", r.ID, i), Kind: "text", Step: i, RequestHash: "test", Quote: "0"})
		if err != nil {
			t.Fatal(err)
		}
		if err = s.writeOutcome(op.ID, Outcome{Cost: "0"}); err != nil {
			t.Fatal(err)
		}
		op.Status = "succeeded"
		op.Cost = "0"
		if err = st.FinishOperation(ctx, r, op); err != nil {
			t.Fatal(err)
		}
	}
	r.Status = "completed"
	if err := st.Save(ctx, r, "run.completed", r.Public()); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `UPDATE agent_checkpoints SET updated_at=now()-interval '25 hours',content_expires_at=now()-interval '1 second' WHERE operation_id IN (SELECT id FROM agent_operations WHERE run_id=$1)`, r.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.Cleanup(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := st.DB.QueryRowContext(ctx, `SELECT count(*) FROM agent_checkpoints WHERE purged_at IS NULL`).Scan(&count); err != nil || count != 0 {
		t.Fatal("cleanup stuck on first page", count, err)
	}
	files, err := filepath.Glob(filepath.Join(s.Root, "*.result.json"))
	if err != nil || len(files) != 0 {
		t.Fatal(len(files), err)
	}
}
func TestPostgresSecuritySubscriptionsBoundAcrossStores(t *testing.T) {
	st := isolatedStore(t)
	_, r := setupRun(t, st, &testExecutor{})
	ctx := context.Background()
	other := &Store{DB: st.DB}
	first, err := st.Subscribe(ctx, 1, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := other.Subscribe(ctx, 1, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.Subscribe(ctx, 1, r.ID); SafeCode(err) != "AGENT_SUBSCRIPTION_LIMIT" {
		t.Fatal("unbounded subscriptions", err)
	}
	if _, err = other.Subscribe(ctx, 2, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign subscription", err)
	}
	st.Unsubscribe(ctx, first)
	third, err := other.Subscribe(ctx, 1, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.DB.ExecContext(ctx, `UPDATE agent_event_subscriptions SET expires_at=now()-interval '1 second' WHERE id=$1`, second); err != nil {
		t.Fatal(err)
	}
	fourth, err := st.Subscribe(ctx, 1, r.ID)
	if err != nil {
		t.Fatal("expired lease blocked capacity", err)
	}
	st.Unsubscribe(ctx, third)
	st.Unsubscribe(ctx, fourth)
}
