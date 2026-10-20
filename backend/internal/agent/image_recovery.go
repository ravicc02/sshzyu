package agent

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func ImageStatusInquiry(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if len([]rune(text)) > 100 {
		return false
	}
	for _, word := range []string{"重新生成", "再生成", "重试生图", "继续生成", "再试一次", "regenerate"} {
		if strings.Contains(text, word) {
			return false
		}
	}
	for _, word := range []string{"咋回事", "怎么回事", "为什么失败", "为啥失败", "为什么报错", "为啥报错", "生成失败了吗", "生成成功了吗", "图片呢", "图呢", "生成了吗", "扣费了吗", "扣钱了吗", "现在什么状态", "出了什么问题"} {
		if strings.Contains(text, word) {
			return true
		}
	}
	return oneOf(strings.Trim(text, "。？！!? "), "报错", "上游错误", "失败了", "出错了")
}

// QueueImageRecovery grants only decoding/storage of an accepted checkpoint.
// It cannot prepare a new operation, call a model or dispatch an image request.
func (s *Service) QueueImageRecovery(ctx context.Context, user int64, id string) (*Run, error) {
	if !s.Enabled {
		return nil, ErrDisabled
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, user).Scan(&user); err != nil {
		return nil, ErrNotFound
	}
	r, err := scanRun(tx.QueryRowContext(ctx, `SELECT `+runColumns+` FROM agent_runs WHERE id=$1 AND user_id=$2 AND EXISTS(SELECT 1 FROM agent_conversations c WHERE c.id=agent_runs.conversation_id AND c.user_id=$2 AND c.deleted_at IS NULL) FOR UPDATE`, id, user))
	if err != nil {
		return nil, err
	}
	if r.RecoverOnly && !r.Terminal() {
		return r, nil
	}
	if r.RecoveredAt != nil && r.Terminal() {
		return r, nil
	}
	if !r.Terminal() || r.Work != nil || r.PendingCall == nil || r.PendingCall.Name != "generate_image" {
		return nil, Invalid("AGENT_IMAGE_RECOVERY_UNAVAILABLE")
	}
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM agent_runs WHERE user_id=$1 AND status NOT IN ('completed','partial_success','failed','cancelled','expired')`, user).Scan(&active); err != nil {
		return nil, err
	}
	if active > 0 {
		return nil, ErrBusy
	}
	opID := fmt.Sprintf("%s_image_%d", r.ID, r.ToolCount)
	var key string
	err = tx.QueryRowContext(ctx, `SELECT cp.storage_key FROM agent_operations o JOIN agent_checkpoints cp ON cp.operation_id=o.id WHERE o.id=$1 AND o.run_id=$2 AND o.kind='image' AND o.status IN ('succeeded','dispatching','accepted','result_unknown') AND cp.user_id=$3 AND cp.purged_at IS NULL AND cp.byte_size>0 AND cp.content_expires_at>now()`, opID, r.ID, user).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) || key != opID+".result.json" {
		return nil, Invalid("AGENT_IMAGE_RECOVERY_UNAVAILABLE")
	}
	if err != nil {
		return nil, err
	}
	deadline := time.Now().UTC().Add(s.ReconcileTimeout)
	if s.ReconcileTimeout <= 0 {
		deadline = time.Now().UTC().Add(DefaultReconcileTimeout)
	}
	r.RecoverOnly = true
	r.ReconcileDeadline = &deadline
	r.Deadline = deadline
	r.Status = "reconciling"
	r.ErrorCode = ""
	r.Cancel = false
	r.Owner = ""
	r.Fence++
	r.LastSequence++
	r.Text = "正在从已保存的上游响应恢复图片，不会重新生成。"
	for i := range r.Trace {
		if r.Trace[i].ID == r.PendingCall.ToolID {
			r.Trace[i].Status = "reconciling"
			r.Trace[i].ErrorCode = ""
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE agent_runs SET snapshot=$1,status='reconciling',fence_token=$2,owner='',lease_until=NULL,cancel_requested=false,version=version+1,updated_at=now() WHERE id=$3`, jsonBytes(r), r.Fence, r.ID)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE agent_task_outbox SET ready_at=now(),completed=false WHERE run_id=$1`, r.ID); err != nil {
		return nil, err
	}
	payload, err := traceEventPayload(r, r.Public())
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_run_events(run_id,sequence,type,payload) VALUES($1,$2,'result.recovery.requested',$3)`, r.ID, r.LastSequence, payload); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	r.Version++
	return r, nil
}
func (s *Service) processImageRecovery(ctx context.Context, r *Run) {
	if r.PendingCall == nil || r.PendingCall.Name != "generate_image" || r.Work != nil {
		s.fail(r, Invalid("AGENT_IMAGE_RECOVERY_UNAVAILABLE"))
		return
	}
	opID := fmt.Sprintf("%s_image_%d", r.ID, r.ToolCount)
	op, err := s.Store.LookupOperation(ctx, opID)
	if err != nil || oneOf(op.Status, "prepared", "failed") {
		s.fail(r, Invalid("AGENT_IMAGE_RECOVERY_UNAVAILABLE"))
		return
	}
	out, err := s.readOutcome(opID)
	if err != nil {
		s.fail(r, Invalid("AGENT_RESULT_STORAGE_FAILED"))
		return
	}
	in := Invocation{Kind: "image", OperationID: opID, RunID: r.ID, Binding: Binding{UserID: r.UserID, KeyID: r.Config.ImageKeyID, GroupID: r.ImageGroupID, IP: r.IP}, Model: r.Config.ImageModel}
	if recoverer, ok := s.Exec.(interface {
		Recover(context.Context, Invocation, Outcome) (Outcome, error)
	}); ok {
		out, err = recoverer.Recover(ctx, in, out)
	}
	if err != nil {
		var permanent *ResultFailure
		if errors.As(err, &permanent) {
			op.Status = "failed"
			op.ErrorCode = permanent.Code
			op.Cost = out.Cost
			_ = s.Store.FinishOperation(context.WithoutCancel(ctx), r, op)
			s.fail(r, err)
		} else {
			s.beginReconciliation(r, "AGENT_RESULT_RECOVERY_PENDING")
		}
		return
	}
	if out.Unknown || out.BillingPending || len(out.Image) == 0 {
		s.beginReconciliation(r, "AGENT_RESULT_RECOVERY_PENDING")
		return
	}
	if err = s.writeOutcome(opID, out); err != nil {
		s.fail(r, err)
		return
	}
	op.Status = "succeeded"
	op.ErrorCode = ""
	op.Cost = out.Cost
	if err = s.Store.FinishOperation(context.WithoutCancel(ctx), r, op); err != nil {
		return
	}
	a, err := s.saveImage(context.WithoutCancel(ctx), r, op, out)
	if err != nil {
		s.fail(r, err)
		return
	}
	call := *r.PendingCall
	var previousEnd *time.Time
	for _, step := range r.Trace {
		if step.ID == call.ToolID {
			previousEnd = step.EndedAt
		}
	}
	r.completeStep(call.ToolID, op.Cost, "已从保存的上游响应恢复，未重新生成。", []string{a.ID})
	if previousEnd != nil {
		for i := range r.Trace {
			if r.Trace[i].ID == call.ToolID {
				r.Trace[i].EndedAt = previousEnd
			}
		}
	}
	found := false
	for _, id := range r.AssetIDs {
		if id == a.ID {
			found = true
		}
	}
	if !found {
		r.AssetIDs = append(r.AssetIDs, a.ID)
	}
	r.History = append(r.History, toolOutput(call, a, op.Cost))
	r.PendingCall = nil
	r.ToolCount++
	r.ReconcileDeadline = nil
	now := time.Now().UTC()
	r.RecoveredAt = &now
	r.Status = "completed"
	r.ErrorCode = ""
	r.Text = "图片已从保存的上游响应恢复，未重新请求生成，也未产生新的生图费用。"
	s.syncCosts(r)
	_ = s.Store.Save(context.WithoutCancel(ctx), r, "result.recovered", r.Public())
}
