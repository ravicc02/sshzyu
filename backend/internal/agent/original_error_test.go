package agent

import (
	"context"
	"errors"
	"testing"
)

type originalErrorExecutor struct {
	adaptiveExecutor
	cost        string
	resultKnown bool
}

func (f *originalErrorExecutor) Execute(ctx context.Context, in Invocation) (Outcome, error) {
	if in.Kind != "image" {
		return f.adaptiveExecutor.Execute(ctx, in)
	}
	f.images++
	detail := &FailureDetail{StatusCode: 502, UpstreamStatusCode: 403, Message: "Insufficient balance"}
	if !f.resultKnown {
		detail.UpstreamStatusCode = 503
		detail.Message = "upstream disconnected"
	}
	return Outcome{Cost: f.cost, BillingPending: f.cost == "", Failure: detail}, &InvocationFailure{Code: "AGENT_UPSTREAM_REJECTED", Dispatched: true, ResultKnown: f.resultKnown, Detail: detail}
}

func TestPostgresOriginalErrorReturnsImmediatelyAndPreservesEvidence(t *testing.T) {
	for _, known := range []bool{true, false} {
		name := "uncertain_after_charge"
		if known {
			name = "definite_rejection"
		}
		t.Run(name, func(t *testing.T) {
			s, _, c := newAdaptive(t)
			ctx := context.Background()
			f := &originalErrorExecutor{resultKnown: known}
			if !known {
				f.cost = "0.17000000"
			}
			s.Exec = f
			f.choose = func(_ Invocation, n int) Outcome {
				if n == 1 {
					return adaptiveCall("cat", "generate_image", ToolArgs{Prompt: "cat"})
				}
				return adaptiveReply("should not resume the model")
			}
			r := adaptiveTurn(t, s, c, "original-error", "生成一张猫图")
			if r.Status != "failed" || r.ReconcileDeadline != nil || r.ErrorCode != "AGENT_UPSTREAM_REJECTED" || r.Failure == nil || r.Failure.Message == "" || f.images != 1 || f.turns != 1 {
				t.Fatal("error was hidden or left waiting", r.Status, r.ErrorCode, f.images, f.turns)
			}
			if known && (r.Failure.UpstreamStatusCode != 403 || r.Failure.Message != "Insufficient balance") {
				t.Fatal("original error lost")
			}
			if !known && r.ImageCost != "0.17000000" {
				t.Fatal("known charge was zeroed", r.ImageCost)
			}
			var status string
			var nullable bool
			if err := s.Store.DB.QueryRow(`SELECT status,cost IS NULL FROM agent_operations WHERE run_id=$1 AND kind='image'`, r.ID).Scan(&status, &nullable); err != nil {
				t.Fatal(err)
			}
			if known && (status != "failed" || !nullable) {
				t.Fatal("rejected request fabricated a fee or kept waiting", status, nullable)
			}
			if !known && status != "result_unknown" {
				t.Fatal("uncertainty erased", status)
			}
			var waiting int
			if err := s.Store.DB.QueryRow(`SELECT count(*) FROM agent_run_events WHERE run_id=$1 AND type='operation.reconciling'`, r.ID).Scan(&waiting); err != nil || waiting != 0 {
				t.Fatal("polling state emitted", waiting, err)
			}
			messages, err := s.Store.Messages(ctx, 1, c.ID)
			if err != nil || messages[len(messages)-1].Failure == nil || messages[len(messages)-1].Failure.Message != r.Failure.Message {
				t.Fatal("history lost the error", err)
			}
			saved, err := s.Store.Run(ctx, 1, r.ID)
			if err != nil || saved.Failure == nil || saved.Failure.Message != r.Failure.Message {
				t.Fatal("restart lost original error", err)
			}
			if _, err = s.Store.Claim(ctx, "another-worker"); !errors.Is(err, ErrNotFound) {
				t.Fatal("failed request still polls", err)
			}
			s.process(ctx, r)
			if f.images != 1 {
				t.Fatal("terminal failure redispatched")
			}
			if !known {
				if _, err = s.Retry(ctx, 1, r.ID, "unsafe-retry", "", c.Version, ""); SafeCode(err) != "AGENT_RETRY_UNSETTLED" {
					t.Fatal("uncertain call can be replayed", err)
				}
			}
		})
	}
}

type recoveryOriginalErrorExecutor struct{ adaptiveExecutor }

func (f *recoveryOriginalErrorExecutor) Execute(ctx context.Context, in Invocation) (Outcome, error) {
	if in.Kind != "image" {
		return f.adaptiveExecutor.Execute(ctx, in)
	}
	f.images++
	return Outcome{Cost: "0.17000000", Body: jsonBytes(map[string]any{"data": []any{map[string]string{"url": "https://fixture.invalid/image"}}})}, nil
}
func (f *recoveryOriginalErrorExecutor) Recover(_ context.Context, in Invocation, out Outcome) (Outcome, error) {
	if in.Kind != "image" {
		return out, nil
	}
	return out, &InvocationFailure{Code: "AGENT_IMAGE_DOWNLOAD_FAILED", Dispatched: true, Detail: &FailureDetail{Message: "image download returned HTTP 403"}}
}
func TestPostgresRecoveryFailureReturnsOriginalCauseWithoutWaiting(t *testing.T) {
	s, _, c := newAdaptive(t)
	f := &recoveryOriginalErrorExecutor{}
	s.Exec = f
	f.choose = func(_ Invocation, n int) Outcome {
		if n == 1 {
			return adaptiveCall("image", "generate_image", ToolArgs{Prompt: "cat"})
		}
		return adaptiveReply("should not resume")
	}
	r := adaptiveTurn(t, s, c, "download-error", "生成一张猫图")
	if r.Status != "failed" || r.ReconcileDeadline != nil || r.ErrorCode != "AGENT_IMAGE_DOWNLOAD_FAILED" || r.Failure == nil || r.Failure.Message != "image download returned HTTP 403" || r.ImageCost != "0.17000000" || f.images != 1 {
		t.Fatal("recovery error hidden or charge lost", r.Status, r.ErrorCode, r.ImageCost)
	}
	out, err := s.readOutcome(r.ID + "_image_0")
	if err != nil || len(out.Body) == 0 {
		t.Fatal("accepted image response lost", err)
	}
}
