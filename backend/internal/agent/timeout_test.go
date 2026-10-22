package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

type interruptedExecutor struct {
	adaptiveExecutor
	started chan struct{}
	code    string
}

func (f *interruptedExecutor) Execute(ctx context.Context, in Invocation) (Outcome, error) {
	f.turns++
	if in.OnProgress != nil {
		if err := in.OnProgress(StreamUpdate{Text: "partial reply", Summary: "public summary"}); err != nil {
			return Outcome{}, err
		}
	}
	if f.started != nil {
		close(f.started)
	}
	if f.code == "AGENT_REQUEST_CANCELLED" {
		<-ctx.Done()
	}
	return Outcome{Cost: "0.01000000", Unknown: true}, &InvocationFailure{Code: f.code, Dispatched: true, Detail: &FailureDetail{Message: "request interrupted"}}
}

func TestPostgresModelTimeoutEndsWithoutReplayOrPolling(t *testing.T) {
	for _, code := range []string{"AGENT_MODEL_FIRST_RESPONSE_TIMEOUT", "AGENT_MODEL_IDLE_TIMEOUT", "AGENT_MODEL_TIMEOUT", "AGENT_IMAGE_TIMEOUT"} {
		t.Run(code, func(t *testing.T) {
			s, _, c := newAdaptive(t)
			f := &interruptedExecutor{code: code}
			s.Exec = f
			r := adaptiveTurn(t, s, c, "timeout-"+code, "hello")
			assertInterruptedRun(t, s, r, f, "failed", code)
		})
	}
}

func TestPostgresStopInterruptsBlockedModelPromptly(t *testing.T) {
	s, _, c := newAdaptive(t)
	f := &interruptedExecutor{code: "AGENT_REQUEST_CANCELLED", started: make(chan struct{})}
	s.Exec = f
	created, err := s.Submit(context.Background(), 1, c.ID, "stop-stalled", "", "hello", nil, c.Version, "5")
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Store.Claim(context.Background(), "stop-worker")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cleanup := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanup()
	done := make(chan struct{})
	go func() { defer close(done); s.process(ctx, r) }()
	select {
	case <-f.started:
	case <-ctx.Done():
		t.Fatal("model not started")
	}
	start := time.Now()
	if err = s.Store.Cancel(context.Background(), 1, created.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stop waited for lease heartbeat")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("stop too slow")
	}
	assertInterruptedRun(t, s, r, f, "cancelled", "")
}

func assertInterruptedRun(t *testing.T, s *Service, r *Run, f *interruptedExecutor, status, code string) {
	t.Helper()
	saved, err := s.Store.Run(context.Background(), 1, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != status || saved.ErrorCode != code || saved.ReconcileDeadline != nil || saved.Text != "partial reply" || saved.ModelCost != "0.01000000" {
		t.Fatal("interruption state lost", saved.Status, saved.ErrorCode, saved.Text, saved.ModelCost)
	}
	var opStatus string
	if err = s.Store.DB.QueryRow(`SELECT status FROM agent_operations WHERE run_id=$1`, r.ID).Scan(&opStatus); err != nil || opStatus != "result_unknown" {
		t.Fatal("lost no-replay evidence", opStatus, err)
	}
	var polling int
	if err = s.Store.DB.QueryRow(`SELECT count(*) FROM agent_run_events WHERE run_id=$1 AND type='operation.reconciling'`, r.ID).Scan(&polling); err != nil || polling != 0 {
		t.Fatal("entered polling", polling, err)
	}
	s.process(context.Background(), saved)
	if f.turns != 1 {
		t.Fatal("automatically replayed model", f.turns)
	}
	if _, err = s.Store.Claim(context.Background(), "next-worker"); !errors.Is(err, ErrNotFound) {
		t.Fatal("terminal run still queued", err)
	}
}
