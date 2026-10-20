package agent

import (
	"context"
	"log/slog"
	"time"
)

const DefaultReconcileTimeout = 2 * time.Minute
const ReconciliationTimeoutCode = "AGENT_RECONCILIATION_TIMEOUT"

// Persist a single absolute deadline. Repeated recovery, cancellation and
// worker restarts cannot restart this timer. The original run deadline also
// bounds legacy snapshots that have no reconciliation deadline yet.
func (s *Service) beginReconciliation(r *Run, code string) {
	s.syncCosts(r)
	now := time.Now().UTC()
	if r.ReconcileDeadline == nil {
		window := s.ReconcileTimeout
		if window <= 0 {
			window = DefaultReconcileTimeout
		}
		deadline := now.Add(window)
		if !r.Deadline.IsZero() && r.Deadline.Before(deadline) {
			deadline = r.Deadline
		}
		r.ReconcileDeadline = &deadline
	}
	if s.reconciliationExpired(r, now) {
		s.finishReconciliationTimeout(r)
		return
	}
	r.Status = "reconciling"
	r.pendingSteps("reconciling", code)
	r.ErrorCode = code
	if r.ErrorCode == "" {
		r.ErrorCode = "UPSTREAM_RESULT_UNKNOWN"
	}
	if err := s.Store.Save(context.Background(), r, "operation.reconciling", r.Public()); err != nil {
		slog.Error("agent reconciliation state save failed", "run_id", r.ID, "error", SafeCode(err))
	}
}

func (s *Service) reconciliationExpired(r *Run, now time.Time) bool {
	return r.ReconcileDeadline != nil && !now.Before(*r.ReconcileDeadline)
}

func (s *Service) finishReconciliationTimeout(r *Run) {
	s.syncCosts(r)
	// End admission blocking, not the unresolved provider operation. Keep its
	// status, checkpoints and known charges intact; do not replay or refund it.
	r.Status = "failed"
	if len(r.AssetIDs) > 0 {
		r.Status = "partial_success"
	}
	r.ErrorCode = ReconciliationTimeoutCode
	r.pendingSteps("failed", ReconciliationTimeoutCode)
	if err := s.Store.Save(context.Background(), r, "run."+r.Status, r.Public()); err != nil {
		slog.Error("agent reconciliation timeout save failed", "run_id", r.ID, "error", SafeCode(err))
	}
}
