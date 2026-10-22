package agent

import (
	"time"
)

const DefaultReconcileTimeout = 2 * time.Minute
const ReconciliationTimeoutCode = "AGENT_RECONCILIATION_TIMEOUT"

// Legacy callers retain this entry point, but failures no longer schedule a
// user-visible polling phase. Operations/checkpoints retain their own evidence.
func (s *Service) beginReconciliation(r *Run, code string) {
	if code == "" || code == ReconciliationTimeoutCode {
		code = "UPSTREAM_RESULT_UNKNOWN"
	}
	s.fail(r, Invalid(code))
}

func (s *Service) reconciliationExpired(r *Run, now time.Time) bool {
	return r.ReconcileDeadline != nil && !now.Before(*r.ReconcileDeadline)
}

func (s *Service) finishReconciliationTimeout(r *Run) {
	code := r.ErrorCode
	if code == "" || code == ReconciliationTimeoutCode {
		code = "UPSTREAM_RESULT_UNKNOWN"
	}
	s.fail(r, Invalid(code))
}
