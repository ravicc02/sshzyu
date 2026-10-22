package agent

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type parallelWorkExecutor struct {
	testExecutor
	active, peak, calls, texts atomic.Int32
	started                    chan int
	release                    <-chan struct{}
	failIndex, unknownIndex    int
}

func (f *parallelWorkExecutor) Execute(_ context.Context, in Invocation) (Outcome, error) {
	if in.Kind != "image" {
		f.texts.Add(1)
		return adaptiveReply("unexpected model call"), nil
	}
	index, _ := strconv.Atoi(in.OperationID[strings.LastIndex(in.OperationID, "_")+1:])
	f.calls.Add(1)
	current := f.active.Add(1)
	defer f.active.Add(-1)
	for old := f.peak.Load(); current > old && !f.peak.CompareAndSwap(old, current); old = f.peak.Load() {
	}
	if f.started != nil {
		f.started <- index
	}
	if index == f.unknownIndex {
		return Outcome{Unknown: true, Cost: "0.05000000"}, errors.New("upstream connection interrupted")
	}
	if f.release != nil {
		<-f.release
	}
	if index == f.failIndex {
		return Outcome{}, &InvocationFailure{Code: "UPSTREAM_REJECTED", Dispatched: false}
	}
	return Outcome{Image: testImage(), Cost: "0.17000000"}, nil
}
func parallelFixture(t *testing.T, n int) (*Service, *parallelWorkExecutor, WorkPlan) {
	s, _, p := detailedWorkFixture(t, n)
	s.WorkConcurrency = 3
	f := &parallelWorkExecutor{failIndex: -1, unknownIndex: -1}
	s.Exec = f
	return s, f, p
}
func submitParallel(t *testing.T, s *Service, p WorkPlan) *Run {
	t.Helper()
	ctx := context.Background()
	ready, err := s.ReviewWork(ctx, 1, p.ID, "", p.Revision, p.WorkDraft)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SubmitWork(ctx, 1, p.ID, "", ready.Revision); err != nil {
		t.Fatal(err)
	}
	r, err := s.Store.Claim(ctx, "parallel-worker")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Work.Parallel {
		t.Fatal("parallel approval not persisted")
	}
	return r
}
func waitStarts(t *testing.T, ch <-chan int, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			t.Fatal("parallel request did not start")
		}
	}
}
func waitWork(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("parallel work did not finish")
	}
}

func TestPostgresParallelWorkUsesBoundedConcurrentImages(t *testing.T) {
	s, f, p := parallelFixture(t, 8)
	release := make(chan struct{})
	f.release = release
	f.started = make(chan int, 32)
	r := submitParallel(t, s, p)
	done := make(chan struct{})
	go func() { s.process(context.Background(), r); close(done) }()
	waitStarts(t, f.started, 3)
	if f.active.Load() != 3 {
		t.Fatal("images still serialized", f.active.Load())
	}
	close(release)
	waitWork(t, done)
	if r.Status != "completed" || f.calls.Load() != 8 || f.texts.Load() != 0 || f.peak.Load() != 3 || len(r.AssetIDs) != 8 || r.Spent != "1.36000000" {
		t.Fatal("parallel completion/accounting", r.Status, r.ErrorCode, f.calls.Load(), f.peak.Load(), r.Spent)
	}
	for i, item := range r.Work.Results {
		if item.ItemID != p.Items[i].ID || item.Status != "succeeded" {
			t.Fatal("result order/status lost")
		}
	}
	s.process(context.Background(), r)
	if f.calls.Load() != 8 {
		t.Fatal("terminal batch replayed")
	}
}
func TestPostgresParallelWorkItemFailureKeepsOtherResults(t *testing.T) {
	s, f, p := parallelFixture(t, 5)
	f.failIndex = 1
	r := submitParallel(t, s, p)
	s.process(context.Background(), r)
	if r.Status != "partial_success" || f.calls.Load() != 5 || len(r.AssetIDs) != 4 || r.Work.Results[1].Status != "failed" {
		t.Fatal("one failure stopped/discarded batch", r.Status, f.calls.Load(), len(r.AssetIDs))
	}
	if r.Spent != "0.68000000" {
		t.Fatal("failed item billed as successful", r.Spent)
	}
}
func TestPostgresParallelSingleItemRedoPreservesOtherOutputs(t *testing.T) {
	s, f, p := parallelFixture(t, 3)
	r := submitParallel(t, s, p)
	s.process(context.Background(), r)
	prior := append([]WorkResult(nil), r.Work.Results...)
	current, err := s.Store.WorkPlan(context.Background(), 1, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := s.ReviewWorkItems(context.Background(), 1, p.ID, "", current.Revision, p.WorkDraft, []string{p.Items[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	if ready.Quote != "0.20000000" || ready.ItemQuotes[0] != "0.00000000" || ready.ItemQuotes[2] != "0.00000000" {
		t.Fatal("single-item quote included others", ready.Quote)
	}
	next, err := s.SubmitWork(context.Background(), 1, p.ID, "", ready.Revision)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.SubmitWork(context.Background(), 1, p.ID, "", ready.Revision)
	if err != nil || again.ID != next.ID {
		t.Fatal("duplicate item approval not idempotent", err)
	}
	claimed, err := s.Store.Claim(context.Background(), "single-worker")
	if err != nil {
		t.Fatal(err)
	}
	s.process(context.Background(), claimed)
	if claimed.Status != "completed" || f.calls.Load() != 4 || f.texts.Load() != 0 || claimed.Spent != "0.17000000" {
		t.Fatal("redo charged/dispatched other items", claimed.Status, f.calls.Load(), claimed.Spent)
	}
	if claimed.Work.Results[0].AssetID != prior[0].AssetID || claimed.Work.Results[2].AssetID != prior[2].AssetID || claimed.Work.Results[1].AssetID == prior[1].AssetID {
		t.Fatal("unselected outputs replaced")
	}
	if _, err = s.ReviewWorkItems(context.Background(), 2, p.ID, "", ready.Revision, p.WorkDraft, []string{p.Items[0].ID}); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-user item review", err)
	}
	fresh, _ := s.Store.WorkPlan(context.Background(), 1, p.ID)
	if _, err = s.ReviewWorkItems(context.Background(), 1, p.ID, "", fresh.Revision, p.WorkDraft, []string{"foreign-item"}); err == nil {
		t.Fatal("invalid target accepted")
	}
}
func TestPostgresParallelCancellationRetainsAlreadyPaidImages(t *testing.T) {
	s, f, p := parallelFixture(t, 8)
	release := make(chan struct{})
	f.release = release
	f.started = make(chan int, 32)
	r := submitParallel(t, s, p)
	done := make(chan struct{})
	go func() { s.process(context.Background(), r); close(done) }()
	waitStarts(t, f.started, 3)
	if err := s.Store.Cancel(context.Background(), 1, r.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	waitWork(t, done)
	if r.Status != "partial_success" || !r.Cancel || f.calls.Load() != 3 || len(r.AssetIDs) != 3 || r.Spent != "0.51000000" {
		t.Fatal("cancellation lost or relaunched paid images", r.Status, r.Cancel, f.calls.Load(), len(r.AssetIDs), r.Spent)
	}
}
func TestPostgresParallelUnknownResultsDoNotReplayOrPermitRedo(t *testing.T) {
	s, f, p := parallelFixture(t, 6)
	release := make(chan struct{})
	f.release = release
	f.started = make(chan int, 32)
	f.unknownIndex = 0
	r := submitParallel(t, s, p)
	done := make(chan struct{})
	go func() { s.process(context.Background(), r); close(done) }()
	waitStarts(t, f.started, 3)
	until := time.Now().Add(5 * time.Second)
	for {
		snapshot, e := s.Store.Run(context.Background(), 1, r.ID)
		if e != nil {
			t.Fatal(e)
		}
		if snapshot.Work.Results[0].Status == "failed" && snapshot.ReconcileDeadline == nil {
			break
		}
		if time.Now().After(until) {
			t.Fatal("reconciliation timer waited for blocked sibling")
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(release)
	waitWork(t, done)
	if r.Status != "partial_success" || f.calls.Load() != 3 || len(r.AssetIDs) != 2 {
		t.Fatal("unknown outcome not isolated", r.Status, f.calls.Load(), len(r.AssetIDs))
	}
	s.process(context.Background(), r)
	if f.calls.Load() != 3 {
		t.Fatal("unknown operation was dispatched again")
	}
	past := time.Now().Add(-time.Second)
	r.ReconcileDeadline = &past
	s.finishReconciliationTimeout(r)
	current, _ := s.Store.WorkPlan(context.Background(), 1, p.ID)
	if _, err := s.ReviewWorkItems(context.Background(), 1, p.ID, "", current.Revision, p.WorkDraft, []string{p.Items[0].ID}); err == nil {
		t.Fatal("uncertain item re-approved")
	}
}
func TestAgentParallelWorkSelectionValidation(t *testing.T) {
	d := workDraft(3)
	d.Items[2].Prompt = ""
	if err := validateWorkSelection(d, []string{d.Items[0].ID}); err != nil {
		t.Fatal("unselected incomplete page blocked independent action", err)
	}
	for _, ids := range [][]string{{d.Items[2].ID}, {d.Items[0].ID, d.Items[0].ID}, {"not-in-plan"}} {
		if validateWorkSelection(d, ids) == nil {
			t.Fatal("invalid scope accepted", fmt.Sprint(ids))
		}
	}
}

func TestPostgresParallelRestartRecoversEveryPaidItemWithoutReplay(t *testing.T) {
	s, f, p := parallelFixture(t, 3)
	ctx := context.Background()
	r := submitParallel(t, s, p)
	for i, item := range r.Work.Plan.Items {
		r.Work.Results[i].Status = "running"
		r.startStep(fmt.Sprintf("%s_work_%d", r.ID, i), "tool", "generate_image", "running", traceArguments(workArgs(r.Work.Plan, item), r.Config.ImageModel))
	}
	r.Status = "executing_tool"
	if err := s.Store.Save(ctx, r, "work.item.started", r.Public()); err != nil {
		t.Fatal(err)
	}
	// Model a crash after all upstream responses were durably recorded but before
	// the coordinator committed any of their materialized asset IDs.
	for i := range p.Items {
		out := s.runWorkItem(ctx, workRunView(r), i)
		if out.Err != nil {
			t.Fatal(out.Err)
		}
	}
	if err := s.Store.Release(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB.ExecContext(ctx, `UPDATE agent_task_outbox SET ready_at=now() WHERE run_id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.Store.Claim(ctx, "restart-worker")
	if err != nil {
		t.Fatal(err)
	}
	fresh := New(s.Store, f, s.Root, true)
	fresh.process(ctx, recovered)
	if recovered.Status != "completed" || f.calls.Load() != 3 || len(recovered.AssetIDs) != 3 || recovered.Spent != "0.51000000" {
		t.Fatal("parallel restart lost/replayed paid outputs", recovered.Status, f.calls.Load(), recovered.Spent)
	}
	var mismatch int
	if err = s.Store.DB.QueryRowContext(ctx, `SELECT count(*) FROM agent_checkpoints c JOIN agent_operations o ON o.id=c.operation_id WHERE o.run_id=$1 AND c.reserved_bytes<>c.byte_size`, r.ID).Scan(&mismatch); err != nil || mismatch != 0 {
		t.Fatal("parallel reserve stranded", mismatch, err)
	}
}
