package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const maxWorkConcurrency = 4

func containsItem(ids []string, id string) bool {
	for _, value := range ids {
		if value == id {
			return true
		}
	}
	return false
}
func workItemSelected(p WorkPlan, id string) bool {
	return len(p.TargetItemIDs) == 0 || containsItem(p.TargetItemIDs, id)
}
func validateWorkSelection(d WorkDraft, targets []string) error {
	if err := d.ValidateDraft(); err != nil {
		return err
	}
	if len(targets) == 0 {
		return d.Validate()
	}
	if len(targets) > MaxWorkItems {
		return invalidArguments("item_ids")
	}
	seen := map[string]bool{}
	subset := d
	subset.Items = nil
	for _, id := range targets {
		if seen[id] {
			return invalidArguments("item_ids")
		}
		seen[id] = true
		found := false
		for _, item := range d.Items {
			if item.ID == id {
				subset.Items = append(subset.Items, item)
				found = true
				break
			}
		}
		if !found {
			return invalidArguments("item_ids")
		}
	}
	return subset.Validate()
}

type workItemOutcome struct {
	Index int
	Op    Operation
	Out   Outcome
	Err   error
}

// Each worker owns an isolated Run view. Only the coordinator changes the real
// snapshot/trace, while operations keep the existing stable billing IDs/fence.
func (s *Service) runWorkItem(ctx context.Context, r *Run, index int) (result workItemOutcome) {
	result.Index = index
	defer func() {
		if recover() != nil {
			result.Err = ErrUnknown
		}
	}()
	w := r.Work
	item := w.Plan.Items[index]
	args := workArgs(w.Plan, item)
	refs := []Asset{}
	for _, id := range args.References {
		a, err := s.loadAssetForRun(ctx, r.UserID, id, r.ID)
		if err != nil {
			result.Err = err
			return
		}
		refs = append(refs, a)
	}
	in := Invocation{OperationID: fmt.Sprintf("%s_work_%d", r.ID, index), RunID: r.ID, Kind: "image", Binding: Binding{UserID: r.UserID, KeyID: r.Config.ImageKeyID, GroupID: r.ImageGroupID, IP: r.IP}, Model: r.Config.ImageModel, Args: &args, References: refs, ApprovedQuote: w.Plan.ItemQuotes[index]}
	result.Op, result.Out, result.Err = s.operation(ctx, r, in, index)
	return
}
func workRunView(r *Run) *Run {
	copy := *r
	copy.Trace = nil
	copy.AssetIDs = nil
	copy.Work = &WorkExecution{Plan: r.Work.Plan}
	return &copy
}
func workResultTerminal(status string) bool {
	return oneOf(status, "succeeded", "failed", "cancelled", "skipped")
}
func (s *Service) updateWorkProgress(r *Run) {
	success, failed, total := 0, 0, 0
	r.Work.Next = 0
	r.AssetIDs = nil
	for _, item := range r.Work.Results {
		if !workItemSelected(r.Work.Plan, item.ItemID) {
			continue
		}
		total++
		if workResultTerminal(item.Status) {
			r.Work.Next++
		}
		if item.Status == "succeeded" {
			success++
			if item.AssetID != "" && !containsItem(r.AssetIDs, item.AssetID) {
				r.AssetIDs = append(r.AssetIDs, item.AssetID)
			}
		}
		if oneOf(item.Status, "failed", "cancelled") {
			failed++
		}
	}
	r.Text = fmt.Sprintf("%s：已完成 %d / %d 项", r.Work.Plan.Title, success, total)
	if failed > 0 {
		r.Text += fmt.Sprintf("，%d 项未完成", failed)
	}
	r.Text += "。"
	s.syncCosts(r)
}
func (s *Service) releaseWorkReserve(r *Run, index int) error {
	_, err := s.Store.DB.ExecContext(context.Background(), `UPDATE agent_checkpoints SET reserved_bytes=byte_size WHERE operation_id=$1`, fmt.Sprintf("%s_work_%d", r.ID, index))
	return err
}
func (r *Run) markWorkStep(index int, status, code string, detail ...*FailureDetail) {
	id := fmt.Sprintf("%s_work_%d", r.ID, index)
	for i := range r.Trace {
		if r.Trace[i].ID == id {
			now := time.Now().UTC()
			r.Trace[i].Status = status
			r.Trace[i].ErrorCode = code
			if len(detail) > 0 {
				r.Trace[i].Failure = detail[0]
			}
			r.Trace[i].EndedAt = &now
			return
		}
	}
}

func (s *Service) processParallelWork(ctx context.Context, r *Run) {
	w := r.Work
	if len(w.Results) != len(w.Plan.Items) || len(w.Plan.ItemQuotes) != len(w.Plan.Items) {
		s.fail(r, ErrConflict)
		return
	}
	recovery, pending := []int{}, []int{}
	for i, item := range w.Results {
		if item.Status == "succeeded" {
			if err := s.releaseWorkReserve(r, i); err != nil {
				s.fail(r, err)
				return
			}
			continue
		}
		if !workItemSelected(w.Plan, item.ItemID) || workResultTerminal(item.Status) {
			continue
		}
		if oneOf(item.Status, "running", "reconciling") {
			recovery = append(recovery, i)
		} else {
			pending = append(pending, i)
		}
	}
	s.updateWorkProgress(r)
	// Finish admitted operations before allowing new requests after a restart or
	// an uncertain upstream result. Existing operations are never re-dispatched.
	if len(recovery) > 0 && !s.runWorkWave(ctx, r, recovery) {
		return
	}
	r.ReconcileDeadline = nil
	r.ErrorCode = ""
	if len(pending) > 0 && !s.runWorkWave(ctx, r, pending) {
		return
	}
	s.updateWorkProgress(r)
	failures := 0
	for _, item := range w.Results {
		if workItemSelected(w.Plan, item.ItemID) && oneOf(item.Status, "failed", "cancelled") {
			failures++
		}
	}
	r.Status = "completed"
	r.ErrorCode = ""
	if failures > 0 {
		r.Status = "failed"
		if len(r.AssetIDs) > 0 {
			r.Status = "partial_success"
		}
		r.ErrorCode = "AGENT_WORK_ITEMS_FAILED"
		if r.Failure != nil {
			for _, item := range r.Work.Results {
				if item.Failure != nil {
					r.ErrorCode = item.ErrorCode
					break
				}
			}
		}
	}
	if failures == 0 {
		r.Text += " 请在对应内容下预览，或单独重新生成。"
	}
	_ = s.Store.Save(context.WithoutCancel(ctx), r, "run."+r.Status, r.Public())
}

func (s *Service) runWorkWave(ctx context.Context, r *Run, indices []int) bool {
	limit := s.WorkConcurrency
	if limit < 1 {
		limit = 1
	}
	if limit > maxWorkConcurrency {
		limit = maxWorkConcurrency
	}
	workCtx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	results := make(chan workItemOutcome, limit)
	cursor, active := 0, 0
	stopped, unknown, cancelled := false, false, false
	checkState := func() error {
		value, err := s.Store.Cancelled(context.WithoutCancel(ctx), r)
		if err != nil {
			return err
		}
		if value || ctx.Err() != nil {
			cancelled = value
			stopped = true
			cancel()
		}
		return nil
	}
	for cursor < len(indices) || active > 0 {
		if err := checkState(); err != nil {
			return false
		}
		for !stopped && cursor < len(indices) && active < limit {
			index := indices[cursor]
			cursor++
			item := &r.Work.Results[index]
			item.Status = "running"
			item.ErrorCode = ""
			item.Failure = nil
			r.Status = "executing_tool"
			r.startStep(fmt.Sprintf("%s_work_%d", r.ID, index), "tool", "generate_image", "running", traceArguments(workArgs(r.Work.Plan, r.Work.Plan.Items[index]), r.Config.ImageModel))
			if err := s.Store.Save(ctx, r, "work.item.started", r.Public()); err != nil {
				return false
			}
			child := workRunView(r)
			active++
			workers.Add(1)
			go func(index int, view *Run) { defer workers.Done(); results <- s.runWorkItem(workCtx, view, index) }(index, child)
		}
		if active == 0 {
			break
		}
		select {
		case result := <-results:
			active--
			index := result.Index
			item := &r.Work.Results[index]
			persistCtx, done := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			// Even cancellation must retain an already confirmed/paid complete image.
			usable := result.Err == nil || (result.Op.Status == "succeeded" && result.Out.confirmed() && len(result.Out.Image) > 0)
			if usable {
				if result.Err != nil {
					result.Err = s.Store.FinishOperation(persistCtx, r, result.Op)
				}
				if result.Err == nil {
					asset, err := s.saveImage(persistCtx, r, result.Op, result.Out)
					if err == nil {
						*item = WorkResult{ItemID: item.ItemID, Status: "succeeded", AssetID: asset.ID}
						r.completeStep(fmt.Sprintf("%s_work_%d", r.ID, index), result.Op.Cost, "", []string{asset.ID})
						r.markWorkStep(index, "completed", "")
						if !WithinBudget("0", result.Op.Cost, r.Work.Plan.ItemQuotes[index]) {
							item.ErrorCode = "AGENT_PRICING_CHANGED"
							stopped = true
						}
						if err = s.releaseWorkReserve(r, index); err != nil {
							result.Err = ErrUnknown
						}
					} else {
						result.Err = ErrUnknown
					}
				}
			}
			if result.Err != nil {
				code := SafeCode(result.Err)
				item.Failure = FailureFor(result.Err)
				if r.Failure == nil {
					r.Failure = item.Failure
				}
				if errors.Is(result.Err, ErrUnknown) || oneOf(result.Op.Status, "dispatching", "accepted", "result_unknown") {
					item.Status = "failed"
					item.ErrorCode = code
					r.markWorkStep(index, "failed", code, item.Failure)
					unknown = true
					stopped = true
				} else {
					item.Status = "failed"
					item.ErrorCode = code
					if cancelled || ctx.Err() != nil {
						item.Status = "cancelled"
					}
					r.markWorkStep(index, item.Status, code, item.Failure)
					if result.Op.ID != "" && result.Op.Status == "prepared" {
						result.Op.Status = "failed"
						result.Op.Cost = "0.00000000"
						result.Op.ErrorCode = code
						_ = s.Store.FinishOperation(persistCtx, r, result.Op)
					}
					if code == "AGENT_UPSTREAM_REJECTED" {
						stopped = true
					}
					if oneOf(code, "AGENT_PRICING_CHANGED", "AGENT_KEY_BINDING_CHANGED", "AGENT_STORAGE_QUOTA_EXCEEDED", "AGENT_AUTHORIZATION_DENIED") {
						stopped = true
					}
				}
			}
			done()
			s.updateWorkProgress(r)
			event := "work.item.completed"
			if item.Status != "succeeded" {
				event = "work.item.updated"
			}
			if err := s.Store.Save(context.WithoutCancel(ctx), r, event, r.Public()); err != nil {
				return false
			}
		case <-time.After(250 * time.Millisecond):
			// The loop observes cancellation and stops admitting more work promptly.
		}
	}
	if err := checkState(); err != nil {
		return false
	}
	if unknown {
		code := "UPSTREAM_RESULT_UNKNOWN"
		for _, item := range r.Work.Results {
			if item.Status == "failed" && item.ErrorCode != "" {
				code = item.ErrorCode
				break
			}
		}
		s.fail(r, Invalid(code))
		return false
	}
	if cancelled || ctx.Err() != nil {
		s.finishStopped(r, cancelled)
		return false
	}
	if stopped {
		s.updateWorkProgress(r)
		r.Status = "failed"
		if len(r.AssetIDs) > 0 {
			r.Status = "partial_success"
		}
		r.ErrorCode = "AGENT_WORK_ITEMS_FAILED"
		if r.Failure != nil {
			for _, item := range r.Work.Results {
				if item.Failure != nil {
					r.ErrorCode = item.ErrorCode
					break
				}
			}
		}
		_ = s.Store.Save(context.WithoutCancel(ctx), r, "run."+r.Status, r.Public())
		return false
	}
	return true
}

func (s *Service) restoreParallelWork(r *Run) error {
	var first error
	for index, item := range r.Work.Results {
		if !workItemSelected(r.Work.Plan, item.ItemID) || item.Status == "succeeded" {
			continue
		}
		op, err := s.Store.LookupOperation(context.Background(), fmt.Sprintf("%s_work_%d", r.ID, index))
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		if oneOf(op.Status, "prepared", "failed") {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		result := s.runWorkItem(ctx, workRunView(r), index)
		if result.Err == nil {
			asset, e := s.saveImage(ctx, r, result.Op, result.Out)
			if e == nil {
				r.Work.Results[index] = WorkResult{ItemID: item.ItemID, Status: "succeeded", AssetID: asset.ID}
				r.completeStep(op.ID, op.Cost, "", []string{asset.ID})
				r.markWorkStep(index, "completed", "")
				_ = s.releaseWorkReserve(r, index)
			} else if first == nil {
				first = e
			}
		} else if first == nil {
			first = result.Err
		}
		cancel()
	}
	s.updateWorkProgress(r)
	return first
}
