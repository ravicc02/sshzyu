package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

func projectCommitted(ctx context.Context, q draftQuery, plan string) (string, error) {
	var amount string
	err := q.QueryRowContext(ctx, `SELECT coalesce(sum(CASE WHEN o.status IN ('succeeded','failed') THEN coalesce(o.cost,0) ELSE greatest(coalesce(o.cost,0),o.quote) END),0)::text FROM agent_project_operations p JOIN agent_operations o ON o.id=p.operation_id WHERE p.plan_id=$1`, plan).Scan(&amount)
	return amount, err
}
func linkProjectRun(ctx context.Context, tx *sql.Tx, plan string, user int64, run string) error {
	var valid bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_work_plans p JOIN agent_runs r ON r.conversation_id=p.conversation_id WHERE p.id=$1 AND p.user_id=$2 AND r.id=$3 AND r.user_id=$2)`, plan, user, run).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return ErrNotFound
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO agent_project_operations(plan_id,operation_id) SELECT $1,id FROM agent_operations WHERE run_id=$2 AND kind='text' ON CONFLICT DO NOTHING`, plan, run)
	return err
}
func reserveProject(ctx context.Context, tx *sql.Tx, r *Run, op Operation) error {
	if op.ProjectID == "" {
		return nil
	}
	var planID string
	err := tx.QueryRowContext(ctx, `SELECT id FROM agent_work_plans WHERE id=$1 AND user_id=$2 AND conversation_id=$3 FOR UPDATE`, op.ProjectID, r.UserID, r.ConversationID).Scan(&planID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err = linkProjectRun(ctx, tx, op.ProjectID, r.UserID, r.ID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_project_operations(plan_id,operation_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, op.ProjectID, op.ID)
	if err != nil {
		return err
	}
	return nil
}
func (s *Service) attachProject(ctx context.Context, r *Run, plan string) error {
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = linkProjectRun(ctx, tx, plan, r.UserID, r.ID); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	r.TaskID = plan
	return nil
}

// Transient availability, filenames and retention timestamps do not describe
// paid request identity. Pixel hashes and IDs do; expires_at may change during
// an admitted hold without changing the already accepted request.
func operationRequestHash(in Invocation) string {
	var value map[string]any
	_ = json.Unmarshal(jsonBytes(in), &value)
	refs := []map[string]string{}
	for _, a := range in.References {
		refs = append(refs, map[string]string{"id": a.ID, "sha256": a.SHA})
	}
	value["References"] = refs
	return Hash(value)
}

func matchesExistingRequest(r *Run, in Invocation, op Operation) bool {
	if r.RuntimeVersion >= 2 {
		return op.RequestHash == operationRequestHash(in)
	}
	if op.RequestHash == Hash(in) {
		return true
	}
	// Old image hashes included mutable retention metadata. Recover only the
	// exact server-persisted image action; changes to prompt, binding or pixels'
	// asset identity remain rejected. Recovery never dispatches a new request.
	if in.Kind != "image" || in.Args == nil || in.Binding.UserID != r.UserID || in.Binding.KeyID != r.Config.ImageKeyID || in.Binding.GroupID != r.ImageGroupID || in.Binding.IP != r.IP || in.Model != r.Config.ImageModel {
		return false
	}
	var expected ToolArgs
	if r.Work != nil {
		if op.Step < 0 || op.Step >= len(r.Work.Plan.Items) {
			return false
		}
		expected = workArgs(r.Work.Plan, r.Work.Plan.Items[op.Step])
	} else {
		if r.PendingCall == nil || r.PendingCall.Name != "generate_image" {
			return false
		}
		var err error
		expected, err = ParseTool([]byte(r.PendingCall.Arguments))
		if err != nil {
			return false
		}
		expected.Model = stringPtr(r.Config.ImageModel)
		if expected.Size == nil {
			expected.Size = stringPtr("1K")
		}
		if expected.Aspect == nil {
			expected.Aspect = stringPtr("1:1")
		}
	}
	if Hash(expected) != Hash(*in.Args) || len(expected.References) != len(in.References) {
		return false
	}
	for i, id := range expected.References {
		if in.References[i].ID != id {
			return false
		}
	}
	return true
}
