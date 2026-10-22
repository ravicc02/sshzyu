package agent

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"
)

// A grant can only be created/revoked by authenticated human REST endpoints.
// Model tools only consume these immutable limits. Count reservations survive unknown results and new runs. Costs are recorded
// for accounting only; legacy max_cost is retained for schema compatibility.
type ExecutionGrant struct {
	ID        string    `json:"id"`
	PlanID    string    `json:"plan_id"`
	KeyID     int64     `json:"image_api_key_id"`
	Model     string    `json:"image_model"`
	Purpose   string    `json:"purpose"`
	MaxCount  int       `json:"max_count"`
	UsedCount int       `json:"used_count"`
	MaxCost   string    `json:"max_image_cost_usd,omitempty"`
	Reserved  string    `json:"reserved_image_cost_usd"`
	Status    string    `json:"status"`
	Expires   time.Time `json:"expires_at"`
}
type GrantRequest struct {
	RequestID string `json:"request_id"`
	Revision  int    `json:"revision"`
	Purpose   string `json:"purpose"`
	MaxCount  int    `json:"max_count"`
	MaxCost   string `json:"max_image_cost_usd,omitempty"`
	Minutes   int    `json:"expires_in_minutes"`
}

const grantColumns = `g.id,g.plan_id,g.key_id,g.model,g.purpose,g.max_count,g.used_count,g.max_cost::text,g.reserved_cost::text,g.status,g.expires_at`

func scanGrant(row interface{ Scan(...any) error }) (ExecutionGrant, error) {
	var g ExecutionGrant
	err := row.Scan(&g.ID, &g.PlanID, &g.KeyID, &g.Model, &g.Purpose, &g.MaxCount, &g.UsedCount, &g.MaxCost, &g.Reserved, &g.Status, &g.Expires)
	if err == nil && !time.Now().Before(g.Expires) {
		g.Status = "closed"
	}
	g.MaxCost = ""
	return g, err
}
func (s *Store) Grants(ctx context.Context, user int64, conv string) ([]ExecutionGrant, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+grantColumns+` FROM agent_execution_grants g JOIN agent_work_plans p ON p.id=g.plan_id WHERE g.user_id=$1 AND p.user_id=$1 AND p.conversation_id=$2 ORDER BY g.created_at DESC LIMIT 24`, user, conv)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExecutionGrant{}
	for rows.Next() {
		g, e := scanGrant(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
func (s *Service) CreateGrant(ctx context.Context, user int64, plan, ip string, in GrantRequest) (ExecutionGrant, error) {
	if !s.Enabled {
		return ExecutionGrant{}, ErrDisabled
	}
	// Kept only because existing schemas require a positive legacy max_cost.
	in.MaxCost = "0.00000001"
	if len(in.RequestID) < 8 || len(in.RequestID) > 100 || !oneOf(in.Purpose, "sample", "output") || in.MaxCount < 1 || in.MaxCount > MaxWorkItems || in.Minutes < 1 || in.Minutes > 1440 {
		return ExecutionGrant{}, invalidArguments("grant")
	}
	p, err := s.Store.WorkPlan(ctx, user, plan)
	if err != nil {
		return ExecutionGrant{}, err
	}
	c, err := s.Store.Conversation(ctx, user, p.ConversationID)
	if err != nil {
		return ExecutionGrant{}, err
	}
	if c.Config.ImageKeyID == 0 {
		return ExecutionGrant{}, Invalid("AGENT_IMAGE_KEY_REQUIRED")
	}
	if _, err = s.Exec.Authorize(ctx, Binding{UserID: user, KeyID: c.Config.ImageKeyID, IP: ip}, "image", c.Config.ImageModel); err != nil {
		return ExecutionGrant{}, err
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return ExecutionGrant{}, err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, user).Scan(&user); err != nil {
		return ExecutionGrant{}, err
	}
	id := "grant_" + Hash([]any{user, plan, in.RequestID})[:32]
	hash := Hash(in)
	var old string
	err = tx.QueryRowContext(ctx, `SELECT request_hash FROM agent_execution_grants WHERE id=$1 AND user_id=$2`, id, user).Scan(&old)
	if err == nil {
		if old != hash {
			return ExecutionGrant{}, ErrConflict
		}
		return scanGrant(tx.QueryRowContext(ctx, `SELECT `+grantColumns+` FROM agent_execution_grants g WHERE g.id=$1`, id))
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ExecutionGrant{}, err
	}
	var rev, version int
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT p.revision,p.status,c.version FROM agent_work_plans p JOIN agent_conversations c ON c.id=p.conversation_id WHERE p.id=$1 AND p.user_id=$2 AND c.user_id=$2 AND c.deleted_at IS NULL FOR UPDATE OF p,c`, plan, user).Scan(&rev, &status, &version); err != nil {
		return ExecutionGrant{}, ErrNotFound
	}
	if rev != in.Revision || version != c.Version || !oneOf(status, "draft", "ready") {
		return ExecutionGrant{}, ErrConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_execution_grants(id,request_hash,user_id,plan_id,key_id,model,purpose,max_count,max_cost,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, hash, user, plan, c.Config.ImageKeyID, c.Config.ImageModel, in.Purpose, in.MaxCount, in.MaxCost, time.Now().UTC().Add(time.Duration(in.Minutes)*time.Minute))
	if err != nil {
		return ExecutionGrant{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE agent_work_plans SET snapshot=jsonb_set(snapshot,'{task_state}','"active"'::jsonb) WHERE id=$1 AND user_id=$2`, plan, user); err != nil {
		return ExecutionGrant{}, err
	}
	g, err := scanGrant(tx.QueryRowContext(ctx, `SELECT `+grantColumns+` FROM agent_execution_grants g WHERE id=$1`, id))
	if err != nil {
		return g, err
	}
	return g, tx.Commit()
}
func (s *Service) RevokeGrant(ctx context.Context, user int64, id string) error {
	if !s.Enabled {
		return ErrDisabled
	}
	res, err := s.Store.DB.ExecContext(ctx, `UPDATE agent_execution_grants g SET status='closed' FROM agent_work_plans p JOIN agent_conversations c ON c.id=p.conversation_id WHERE g.id=$1 AND g.user_id=$2 AND g.plan_id=p.id AND p.user_id=$2 AND c.user_id=$2 AND c.deleted_at IS NULL`, id, user)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrNotFound
	}
	return nil
}
func (s *Service) imageGrant(ctx context.Context, r *Run, args ToolArgs) (string, error) {
	opID := r.ID + "_image_" + strconv.Itoa(r.ToolCount)
	var grant string
	// A grant already consumed by this stable operation remains usable only for
	// recovery, even after expiry or revocation. operation() never redispatches it.
	err := s.Store.DB.QueryRowContext(ctx, `SELECT go.grant_id FROM agent_grant_operations go JOIN agent_operations o ON o.id=go.operation_id WHERE o.id=$1 AND o.run_id=$2 AND o.status<>'prepared'`, opID, r.ID).Scan(&grant)
	if err == nil {
		return grant, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if args.TaskID == nil || args.Purpose == nil || !oneOf(*args.Purpose, "sample", "output") {
		return "", Invalid("AGENT_GRANT_REQUIRED")
	}
	err = s.Store.DB.QueryRowContext(ctx, `SELECT g.id FROM agent_execution_grants g JOIN agent_work_plans p ON p.id=g.plan_id WHERE g.plan_id=$1 AND g.user_id=$2 AND p.user_id=$2 AND p.conversation_id=$3 AND g.key_id=$4 AND g.model=$5 AND g.purpose=$6 AND g.status='open' AND g.expires_at>now() AND g.used_count<g.max_count ORDER BY g.created_at,g.id LIMIT 1`, *args.TaskID, r.UserID, r.ConversationID, r.Config.ImageKeyID, r.Config.ImageModel, *args.Purpose).Scan(&grant)
	if errors.Is(err, sql.ErrNoRows) {
		return "", Invalid("AGENT_GRANT_REQUIRED")
	}
	return grant, err
}
func reserveGrant(ctx context.Context, tx *sql.Tx, r *Run, op Operation) error {
	if op.Kind != "image" || r.Work != nil || r.RuntimeVersion < 2 {
		return nil
	}
	if op.GrantID == "" {
		return reserveDirectImage(ctx, tx, r, op)
	}
	var existing string
	err := tx.QueryRowContext(ctx, `SELECT grant_id FROM agent_grant_operations WHERE operation_id=$1`, op.ID).Scan(&existing)
	if err == nil {
		if existing != op.GrantID {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE agent_execution_grants g SET used_count=used_count+1,reserved_cost=reserved_cost+$1::numeric,status=CASE WHEN used_count+1>=max_count THEN 'closed' ELSE 'open' END FROM agent_work_plans p WHERE g.id=$2 AND g.plan_id=p.id AND g.user_id=$3 AND p.user_id=$3 AND p.conversation_id=$4 AND g.key_id=$5 AND g.model=$6 AND g.status='open' AND g.expires_at>now() AND g.used_count<g.max_count`, op.Quote, op.GrantID, r.UserID, r.ConversationID, r.Config.ImageKeyID, r.Config.ImageModel)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return Invalid("AGENT_GRANT_EXHAUSTED")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_grant_operations(operation_id,grant_id,reserved_cost) VALUES($1,$2,$3)`, op.ID, op.GrantID, op.Quote)
	return err
}
func (s *Service) imageReviewCard(ctx context.Context, r *Run, call Call, args ToolArgs) (WorkPlan, error) {
	if args.Model != nil && *args.Model != r.Config.ImageModel {
		return WorkPlan{}, Invalid("AGENT_MODEL_NOT_ALLOWED")
	}
	size, aspect := "1K", "1:1"
	if args.Size != nil {
		size = *args.Size
	}
	if args.Aspect != nil {
		aspect = *args.Aspect
	}
	draft := WorkDraft{Title: "配置卡", Brief: r.Input, Size: size, Aspect: aspect, SharedReferences: []string{}, Items: []WorkItem{{ID: "image1", Title: "图片", Prompt: args.Prompt, References: args.References}}}
	if len(draft.Brief) > 4000 {
		draft.Brief = "按用户已沟通的需求生成图片"
	}
	wrapped := call
	wrapped.Arguments = string(jsonBytes(draft))
	return s.proposeWork(ctx, r, wrapped)
}
