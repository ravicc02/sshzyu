package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/pkg/billingid"
	"time"
)

type Store struct {
	DB              *sql.DB
	MaxStorageBytes int64
}

func (s *Store) CreateConversation(ctx context.Context, c Conversation) error {
	b, _ := json.Marshal(c.Config)
	_, e := s.DB.ExecContext(ctx, `INSERT INTO agent_conversations(id,user_id,config,title) VALUES($1,$2,$3,$4)`, c.ID, c.UserID, b, c.Title)
	return e
}
func (s *Store) Conversation(ctx context.Context, user int64, id string) (Conversation, error) {
	var c Conversation
	var raw []byte
	e := s.DB.QueryRowContext(ctx, `SELECT id,user_id,title,config,version,created_at,updated_at,archived_at FROM agent_conversations WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`, id, user).Scan(&c.ID, &c.UserID, &c.Title, &raw, &c.Version, &c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt)
	if errors.Is(e, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &c.Config)
		c.Config.Budget = ""
	}
	return c, e
}
func (s *Store) Conversations(ctx context.Context, user int64) ([]Conversation, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,user_id,title,config,version,created_at,updated_at,archived_at FROM agent_conversations WHERE user_id=$1 AND deleted_at IS NULL ORDER BY updated_at DESC LIMIT 100`, user)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Conversation{}
	for rows.Next() {
		var c Conversation
		var raw []byte
		if e = rows.Scan(&c.ID, &c.UserID, &c.Title, &raw, &c.Version, &c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &c.Config); e != nil {
			return nil, e
		}
		c.Config.Budget = ""
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Store) Configure(ctx context.Context, user int64, id string, version int, cfg Config) error {
	cfg.Budget = ""
	b, _ := json.Marshal(cfg)
	result, e := s.DB.ExecContext(ctx, `UPDATE agent_conversations SET config=$1,version=version+1,updated_at=now() WHERE id=$2 AND user_id=$3 AND version=$4 AND deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM agent_runs WHERE conversation_id=$2 AND status NOT IN ('completed','partial_success','failed','cancelled','expired'))`, b, id, user, version)
	if e != nil {
		return e
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrConflict
	}
	return nil
}
func (s *Store) Messages(ctx context.Context, user int64, conv string) ([]Message, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT content,is_answer FROM (SELECT CASE WHEN m.role='assistant' THEN m.content || jsonb_build_object('run_status',r.status,'error_code',coalesce(r.snapshot->>'error_code',''),'failure',r.snapshot->'failure','can_recover_image',r.status IN ('failed','expired','partial_success','cancelled') AND r.snapshot->'pending_call'->>'name'='generate_image' AND EXISTS(SELECT 1 FROM agent_operations recovery_op JOIN agent_checkpoints cp ON cp.operation_id=recovery_op.id WHERE recovery_op.run_id=r.id AND recovery_op.kind='image' AND recovery_op.status IN ('succeeded','dispatching','accepted','result_unknown') AND cp.user_id=c.user_id AND cp.purged_at IS NULL AND cp.byte_size>0 AND cp.content_expires_at>now())) ELSE m.content END content,m.created_at,m.id,
 (m.role='user' AND EXISTS(SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(r.snapshot->'trace')='array' THEN r.snapshot->'trace' ELSE '[]'::jsonb END) t WHERE t->>'kind'='tool' AND t->>'name'='ask_user' AND t->>'status'='completed' AND t->>'summary'=m.content->>'text')) AS is_answer
 FROM agent_messages m JOIN agent_conversations c ON c.id=m.conversation_id LEFT JOIN agent_runs r ON r.id=m.run_id AND r.user_id=c.user_id WHERE c.user_id=$1 AND c.id=$2 AND c.deleted_at IS NULL
 AND NOT(m.role='assistant' AND EXISTS(SELECT 1 FROM agent_runs replacement WHERE replacement.user_id=c.user_id AND replacement.conversation_id=c.id AND replacement.snapshot->>'retry_of'=m.run_id AND replacement.status IN ('completed','partial_success')))
 ORDER BY m.created_at DESC,m.id DESC LIMIT 1000) recent ORDER BY created_at,id`, user, conv)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var b []byte
		var legacyAnswer bool
		if e = rows.Scan(&b, &legacyAnswer); e != nil {
			return nil, e
		}
		var m Message
		if e = json.Unmarshal(b, &m); e != nil {
			return nil, e
		}
		if legacyAnswer && m.Kind == "" {
			m.Kind = "question_answer"
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *Store) FindIdempotent(ctx context.Context, user int64, key, hash string) (*Run, error) {
	var id, old string
	e := s.DB.QueryRowContext(ctx, `SELECT id,request_hash FROM agent_runs WHERE user_id=$1 AND idempotency_key=$2`, user, key).Scan(&id, &old)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if old != hash {
		return nil, ErrConflict
	}
	return s.Run(ctx, user, id)
}
func (s *Store) Submit(ctx context.Context, r *Run) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	// Lock the owner, not only the conversation, so concurrent conversations cannot bypass the limit.
	var uid int64
	if e = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, r.UserID).Scan(&uid); e != nil {
		return e
	}
	var hash, id string
	e = tx.QueryRowContext(ctx, `SELECT id,request_hash FROM agent_runs WHERE user_id=$1 AND idempotency_key=$2`, r.UserID, r.Key).Scan(&id, &hash)
	if e == nil {
		if hash != r.RequestHash {
			return ErrConflict
		}
		return ErrConflict
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	var n int
	if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM agent_runs WHERE user_id=$1 AND status NOT IN ('completed','partial_success','failed','cancelled','expired')`, r.UserID).Scan(&n); e != nil {
		return e
	}
	if n > 0 {
		return ErrBusy
	}
	for _, id := range r.Attachments {
		var assetID string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM agent_assets WHERE id=$1 AND user_id=$2 AND conversation_id=$3 AND status='ready' AND expires_at>now()`, id, r.UserID, r.ConversationID).Scan(&assetID); err != nil {
			return ErrNotFound
		}
	}
	var version int
	if e = tx.QueryRowContext(ctx, `SELECT version FROM agent_conversations WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL FOR UPDATE`, r.ConversationID, r.UserID).Scan(&version); e != nil {
		return e
	}
	if version != r.ConfigVersion {
		return ErrConflict
	}

	// Do not admit a run whose memory was assembled before the previous reply
	// committed. Reject it before creating any operation or bill.
	var latestMessage string
	e = tx.QueryRowContext(ctx, `SELECT id FROM agent_messages WHERE conversation_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1`, r.ConversationID).Scan(&latestMessage)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	expectedMessage := ""
	if r.Memory != nil {
		expectedMessage = r.Memory.lastMessageID
	}
	if latestMessage != expectedMessage {
		return ErrConflict
	}
	if r.RetryOf != "" {
		var latest string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM agent_runs WHERE conversation_id=$1 AND user_id=$2 ORDER BY created_at DESC,id DESC LIMIT 1`, r.ConversationID, r.UserID).Scan(&latest); err != nil {
			return err
		}
		if latest != r.RetryOf {
			return Invalid("AGENT_RETRY_NOT_LATEST")
		}
	}
	b, _ := json.Marshal(r)
	_, e = tx.ExecContext(ctx, `INSERT INTO agent_runs(id,conversation_id,user_id,idempotency_key,request_hash,snapshot,status) VALUES($1,$2,$3,$4,$5,$6,$7)`, r.ID, r.ConversationID, r.UserID, r.Key, r.RequestHash, b, r.Status)
	if e != nil {
		return e
	}
	if r.Work != nil {
		if e = claimWork(ctx, tx, r); e != nil {
			return e
		}
	}
	if r.RetryOf == "" {
		m := Message{ID: ID("msg"), Role: "user", Text: r.Input, Assets: r.Attachments, RunID: r.ID, CreatedAt: time.Now().UTC()}
		mb, _ := json.Marshal(m)
		if _, e = tx.ExecContext(ctx, `INSERT INTO agent_messages(id,conversation_id,run_id,role,content) VALUES($1,$2,$3,$4,$5)`, m.ID, r.ConversationID, r.ID, m.Role, mb); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO agent_task_outbox(run_id) VALUES($1)`, r.ID); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE agent_conversations SET updated_at=now() WHERE id=$1`, r.ConversationID); e != nil {
		return e
	}
	return tx.Commit()
}
func scanRun(row interface{ Scan(...any) error }) (*Run, error) {
	var b []byte
	var user, fence, version int64
	var owner, status string
	var cancel bool
	e := row.Scan(&b, &user, &fence, &version, &owner, &status, &cancel)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	var r Run
	if e = json.Unmarshal(b, &r); e != nil {
		return nil, e
	}
	r.UserID = user
	r.Fence = fence
	r.Version = version
	r.Owner = owner
	r.Status = status
	r.Cancel = cancel
	return &r, nil
}

const runColumns = `snapshot,user_id,fence_token,version,owner,status,cancel_requested`

func (s *Store) Run(ctx context.Context, user int64, id string) (*Run, error) {
	return scanRun(s.DB.QueryRowContext(ctx, `SELECT `+runColumns+` FROM agent_runs WHERE id=$1 AND user_id=$2 AND EXISTS(SELECT 1 FROM agent_conversations c WHERE c.id=agent_runs.conversation_id AND c.user_id=$2 AND c.deleted_at IS NULL)`, id, user))
}
func (s *Store) Active(ctx context.Context, user int64, conv string) (*Run, error) {
	r, e := scanRun(s.DB.QueryRowContext(ctx, `SELECT `+runColumns+` FROM agent_runs WHERE user_id=$1 AND conversation_id=$2 ORDER BY (status NOT IN ('completed','partial_success','failed','cancelled','expired')) DESC,created_at DESC LIMIT 1`, user, conv))
	if errors.Is(e, ErrNotFound) {
		return nil, nil
	}
	return r, e
}
func (s *Store) Claim(ctx context.Context, owner string) (*Run, error) {
	q := `WITH candidate AS (SELECT r.id FROM agent_runs r JOIN agent_task_outbox o ON o.run_id=r.id WHERE NOT o.completed AND o.ready_at<=now() AND (r.lease_until IS NULL OR r.lease_until<now()) ORDER BY o.ready_at FOR UPDATE OF r SKIP LOCKED LIMIT 1) UPDATE agent_runs r SET owner=$1,fence_token=fence_token+1,lease_until=now()+interval '60 seconds' FROM candidate c WHERE r.id=c.id RETURNING ` + `r.snapshot,r.user_id,r.fence_token,r.version,r.owner,r.status,r.cancel_requested`
	return scanRun(s.DB.QueryRowContext(ctx, q, owner))
}
func (s *Store) Heartbeat(ctx context.Context, r *Run) error {
	res, e := s.DB.ExecContext(ctx, `UPDATE agent_runs SET lease_until=now()+interval '60 seconds' WHERE id=$1 AND owner=$2 AND fence_token=$3`, r.ID, r.Owner, r.Fence)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrConflict
	}
	return nil
}
func (s *Store) Cancelled(ctx context.Context, r *Run) (bool, error) {
	var c bool
	e := s.DB.QueryRowContext(ctx, `SELECT cancel_requested FROM agent_runs WHERE id=$1 AND owner=$2 AND fence_token=$3`, r.ID, r.Owner, r.Fence).Scan(&c)
	return c, e
}
func (s *Store) Cancel(ctx context.Context, user int64, id string) error {
	res, e := s.DB.ExecContext(ctx, `WITH cancelled AS (UPDATE agent_runs SET cancel_requested=true WHERE id=$1 AND user_id=$2 RETURNING id) UPDATE agent_task_outbox SET ready_at=now() WHERE run_id IN (SELECT id FROM cancelled)`, id, user)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrNotFound
	}
	return nil
}
func (s *Store) Save(ctx context.Context, r *Run, typ string, data any) error {
	r.finishWorkState()
	if r.Work != nil && r.Terminal() {
		data = r.Public()
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	seq := r.LastSequence
	if typ != "" {
		seq++
	}
	r.LastSequence = seq
	b, _ := json.Marshal(r)
	res, e := tx.ExecContext(ctx, `UPDATE agent_runs SET snapshot=$1,status=$2,version=version+1,updated_at=now() WHERE id=$3 AND fence_token=$4 AND owner=$5 AND version=$6`, b, r.Status, r.ID, r.Fence, r.Owner, r.Version)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrConflict
	}
	if typ != "" {
		payload, marshalErr := traceEventPayload(r, data)
		if marshalErr != nil {
			return marshalErr
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO agent_run_events(run_id,sequence,type,payload) VALUES($1,$2,$3,$4)`, r.ID, seq, typ, payload); e != nil {
			return e
		}
	}
	if r.Terminal() {
		if r.Status != "completed" {
			if _, e = tx.ExecContext(ctx, `UPDATE agent_execution_grants SET status='closed' WHERE id IN (SELECT go.grant_id FROM agent_grant_operations go JOIN agent_operations o ON o.id=go.operation_id WHERE o.run_id=$1)`, r.ID); e != nil {
				return e
			}
		}
		if _, e = tx.ExecContext(ctx, `UPDATE agent_checkpoints SET reserved_bytes=byte_size WHERE operation_id IN(SELECT id FROM agent_operations WHERE run_id=$1)`, r.ID); e != nil {
			return e
		}
		m := Message{ID: "msg_" + r.ID, Role: "assistant", Text: r.Text, Assets: r.AssetIDs, RunID: r.ID, Trace: r.Trace, RunStatus: r.Status, ErrorCode: r.ErrorCode, Failure: r.Failure, CreatedAt: time.Now().UTC()}
		mb, _ := json.Marshal(m)
		if _, e = tx.ExecContext(ctx, `INSERT INTO agent_messages(id,conversation_id,run_id,role,content) VALUES($1,$2,$3,$4,$5) ON CONFLICT(id) DO NOTHING`, m.ID, r.ConversationID, r.ID, m.Role, mb); e != nil {
			return e
		}
		if r.RecoverOnly {
			if _, e = tx.ExecContext(ctx, `UPDATE agent_messages SET content=$1::jsonb || jsonb_build_object('created_at',coalesce(content->'created_at',to_jsonb(created_at))) WHERE id=$2 AND run_id=$3`, mb, m.ID, r.ID); e != nil {
				return e
			}
		}
		if _, e = tx.ExecContext(ctx, `UPDATE agent_task_outbox SET completed=true WHERE run_id=$1`, r.ID); e != nil {
			return e
		}
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	r.Version++
	return nil
}
func (s *Store) Release(ctx context.Context, r *Run) error {
	var deadline any
	if r.Status == "waiting_user" {
		_, err := s.DB.ExecContext(ctx, `WITH released AS (UPDATE agent_runs SET lease_until=NULL,owner='' WHERE id=$1 AND owner=$2 AND fence_token=$3 RETURNING id) UPDATE agent_task_outbox o SET ready_at=$4 FROM released r WHERE o.run_id=r.id`, r.ID, r.Owner, r.Fence, r.Deadline)
		return err
	}
	if r.ReconcileDeadline != nil {
		deadline = *r.ReconcileDeadline
	}
	// Only the lease owner can reschedule. Wake up at the reconciliation
	// deadline even when the normal recovery backoff would run later.
	_, err := s.DB.ExecContext(ctx, `WITH released AS (
 UPDATE agent_runs SET lease_until=NULL,owner='' WHERE id=$1 AND owner=$2 AND fence_token=$3 RETURNING id
 ) UPDATE agent_task_outbox o SET ready_at=LEAST(now()+interval '30 seconds',coalesce($4::timestamptz,now()+interval '30 seconds')),attempts=attempts+1
 FROM released r WHERE o.run_id=r.id`, r.ID, r.Owner, r.Fence, deadline)
	return err
}
func (s *Store) Events(ctx context.Context, user int64, id string, after int64) ([]Event, error) {
	events, _, err := s.PollEvents(ctx, user, id, after)
	return events, err
}

// One bounded query per poll, including ownership and current run status.
func (s *Store) PollEvents(ctx context.Context, user int64, id string, after int64) ([]Event, string, error) {
	var raw []byte
	var status string
	var last int64
	// A reconnect must not aggregate hundreds of full snapshots into one Go allocation.
	// Bound both rows and uncompressed JSON bytes; always include one event to progress.
	err := s.DB.QueryRowContext(ctx, `SELECT r.status,coalesce((
 WITH next AS(SELECT sequence,type,payload,created_at FROM agent_run_events WHERE run_id=r.id AND sequence>$3 ORDER BY sequence LIMIT 4),
 bounded AS(SELECT *,row_number() OVER(ORDER BY sequence) AS ordinal,sum(octet_length(payload::text)) OVER(ORDER BY sequence) AS bytes FROM next)
 SELECT jsonb_agg(jsonb_build_object('sequence',sequence,'type',type,'data',payload,'created_at',created_at) ORDER BY sequence) FROM bounded WHERE ordinal=1 OR bytes<=524288),'[]'::jsonb),
 coalesce((SELECT max(sequence) FROM agent_run_events WHERE run_id=r.id),0)
 FROM agent_runs r JOIN agent_conversations c ON c.id=r.conversation_id WHERE r.id=$1 AND r.user_id=$2 AND c.user_id=$2 AND c.deleted_at IS NULL`, id, user, after).Scan(&status, &raw, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	var events []Event
	err = json.Unmarshal(raw, &events)
	if err == nil && len(events) > 0 {
		events[len(events)-1].HasMore = events[len(events)-1].Sequence < last
	}
	return events, status, err
}

func (s *Store) Prepare(ctx context.Context, r *Run, op Operation) (Operation, error) {
	var fence int64
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return op, e
	}
	defer tx.Rollback()
	if e = tx.QueryRowContext(ctx, `SELECT fence_token FROM agent_runs WHERE id=$1 AND owner=$2 FOR UPDATE`, r.ID, r.Owner).Scan(&fence); e != nil || fence != r.Fence {
		return op, ErrConflict
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO agent_operations(id,run_id,kind,step,request_hash,quote,billing_request_id) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(run_id,kind,step) DO NOTHING`, op.ID, r.ID, op.Kind, op.Step, op.RequestHash, op.Quote, billingid.AgentOperation(op.ID))
	if e != nil {
		return op, e
	}
	var hash string
	e = tx.QueryRowContext(ctx, `SELECT id,status,request_hash,quote::text,coalesce(cost,0)::text,coalesce(output,'null'::jsonb),coalesce(asset_id,''),error_code FROM agent_operations WHERE run_id=$1 AND kind=$2 AND step=$3`, r.ID, op.Kind, op.Step).Scan(&op.ID, &op.Status, &hash, &op.Quote, &op.Cost, &op.Output, &op.AssetID, &op.ErrorCode)
	if e != nil {
		return op, e
	}
	if hash != op.RequestHash {
		return op, ErrConflict
	}
	if e = reserveProject(ctx, tx, r, op); e != nil {
		return op, e
	}
	if e = reserveGrant(ctx, tx, r, op); e != nil {
		return op, e
	}
	for _, id := range op.ReferenceIDs {
		res, err := tx.ExecContext(ctx, `INSERT INTO agent_asset_holds(asset_id,run_id,until_at) SELECT id,$1,$2 FROM agent_assets WHERE id=$3 AND user_id=$4 AND conversation_id=$5 AND status='ready' AND expires_at>now() ON CONFLICT(asset_id,run_id) DO NOTHING`, r.ID, r.Deadline.Add(DefaultReconcileTimeout), id, r.UserID, r.ConversationID)
		if err != nil {
			return op, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			var held bool
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_asset_holds WHERE asset_id=$1 AND run_id=$2 AND until_at>now())`, id, r.ID).Scan(&held); err != nil {
				return op, err
			}
			if !held {
				return op, Invalid("ASSET_EXPIRED")
			}
		}
	}
	return op, tx.Commit()
}
func (s *Store) Dispatch(ctx context.Context, r *Run, op Operation) error {
	res, e := s.DB.ExecContext(ctx, `UPDATE agent_operations o SET status='dispatching',updated_at=now() FROM agent_runs r WHERE o.id=$1 AND o.status='prepared' AND r.id=o.run_id AND r.owner=$2 AND r.fence_token=$3 AND NOT r.cancel_requested AND r.lease_until>now()`, op.ID, r.Owner, r.Fence)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrConflict
	}
	return nil
}
func (s *Store) FinishOperation(ctx context.Context, r *Run, op Operation) error {
	res, e := s.DB.ExecContext(ctx, `UPDATE agent_operations o SET status=$1,cost=$2,output=$3,asset_id=nullif($4,''),error_code=$5,updated_at=now() FROM agent_runs r WHERE o.id=$6 AND r.id=o.run_id AND r.owner=$7 AND r.fence_token=$8`, op.Status, nullableCost(op.Cost), nullableJSON(op.Output), op.AssetID, op.ErrorCode, op.ID, r.Owner, r.Fence)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrConflict
	}
	if op.Status == "failed" || (op.Status == "succeeded" && op.Kind == "text") {
		_, e = s.DB.ExecContext(ctx, `UPDATE agent_checkpoints SET reserved_bytes=byte_size WHERE operation_id=$1`, op.ID)
	}
	return e
}
func nullableCost(cost string) any {
	if cost == "" {
		return nil
	}
	return cost
}
func nullableJSON(b json.RawMessage) any {
	if len(b) == 0 {
		return nil
	}
	return []byte(b)
}
func (s *Store) Tool(ctx context.Context, r *Run, op string, c Call, args any) error {
	b, _ := json.Marshal(args)
	hash := Hash(args)
	var old string
	e := s.DB.QueryRowContext(ctx, `INSERT INTO agent_tool_calls(id,run_id,model_operation_id,provider_call_id,arguments_hash,arguments) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(model_operation_id,provider_call_id) DO UPDATE SET provider_call_id=excluded.provider_call_id RETURNING arguments_hash`, c.ToolID, r.ID, op, c.ID, hash, b).Scan(&old)
	if e != nil {
		return e
	}
	if old != hash {
		return ErrConflict
	}
	return nil
}
func (s *Store) PutAsset(ctx context.Context, a Asset, operation string) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var user int64
	if e = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, a.UserID).Scan(&user); e != nil {
		return e
	}
	if operation != "" {
		if _, e = tx.ExecContext(ctx, `UPDATE agent_checkpoints SET reserved_bytes=byte_size WHERE operation_id=$1 AND user_id=$2`, operation, a.UserID); e != nil {
			return e
		}
	}
	bytes, e := storageUsed(ctx, tx, a.UserID)
	if e != nil {
		return e
	}
	if bytes+a.Size > storageQuota {
		return Invalid("AGENT_STORAGE_QUOTA_EXCEEDED")
	}
	if e = s.reserveGlobalStorage(ctx, tx, a.Size); e != nil {
		return e
	}
	status := a.Status
	if status == "" {
		status = "ready"
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO agent_assets(id,user_id,conversation_id,run_id,operation_id,storage_key,mime_type,byte_size,width,height,sha256,expires_at,filename,status) VALUES($1,$2,$3,nullif($4,''),nullif($5,''),$6,$7,$8,$9,$10,$11,$12,$13,$14) ON CONFLICT(operation_id) DO NOTHING`, a.ID, a.UserID, a.ConversationID, a.RunID, operation, a.Key, a.MIME, a.Size, a.Width, a.Height, a.SHA, a.Expires, a.FileName, status)
	if e != nil {
		return e
	}
	return tx.Commit()
}

func (s *Store) AssetMeta(ctx context.Context, user int64, id string) (Asset, error) {
	var a Asset
	err := s.DB.QueryRowContext(ctx, `SELECT a.id,a.user_id,a.conversation_id,coalesce(a.run_id,''),a.storage_key,a.mime_type,a.byte_size,a.width,a.height,a.sha256,a.expires_at,a.created_at,a.filename,a.status FROM agent_assets a JOIN agent_conversations c ON c.id=a.conversation_id WHERE a.id=$1 AND a.user_id=$2 AND c.user_id=$2 AND c.deleted_at IS NULL`, id, user).Scan(&a.ID, &a.UserID, &a.ConversationID, &a.RunID, &a.Key, &a.MIME, &a.Size, &a.Width, &a.Height, &a.SHA, &a.Expires, &a.CreatedAt, &a.FileName, &a.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	a.ContentAvailable = err == nil && a.Status == "ready" && time.Now().Before(a.Expires)
	if !a.ContentAvailable && oneOf(a.Status, "ready", "expired", "purged") {
		a.Status = "expired"
	}
	return a, err
}
func (s *Store) Asset(ctx context.Context, user int64, id string) (Asset, error) {
	a, err := s.AssetMeta(ctx, user, id)
	if err != nil {
		return a, err
	}
	if a.ContentAvailable {
		return a, nil
	}
	if a.Status == "expired" {
		return a, Invalid("ASSET_EXPIRED")
	}
	return a, ErrNotFound
}

// Internal holds only authorize the exact admitted run. They never change the
// asset's public expiry or permit another message in the same conversation.
func (s *Store) assetForRun(ctx context.Context, user int64, id, run string) (Asset, error) {
	a, err := s.Asset(ctx, user, id)
	if err == nil || SafeCode(err) != "ASSET_EXPIRED" {
		return a, err
	}
	var held bool
	e := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_asset_holds h JOIN agent_runs r ON r.id=h.run_id JOIN agent_assets a ON a.id=h.asset_id WHERE h.asset_id=$1 AND h.run_id=$2 AND r.user_id=$3 AND a.user_id=$3 AND a.status='ready' AND h.until_at>now() AND r.status NOT IN ('completed','partial_success','failed','cancelled','expired'))`, id, run, user).Scan(&held)
	if e != nil {
		return a, e
	}
	if !held {
		return a, err
	}
	return a, nil
}

func (s *Store) LookupOperation(ctx context.Context, id string) (Operation, error) {
	var op Operation
	e := s.DB.QueryRowContext(ctx, `SELECT id,run_id,kind,step,status,request_hash,quote::text,coalesce(cost,0)::text,coalesce(output,'null'::jsonb),coalesce(asset_id,''),error_code FROM agent_operations WHERE id=$1`, id).Scan(&op.ID, &op.RunID, &op.Kind, &op.Step, &op.Status, &op.RequestHash, &op.Quote, &op.Cost, &op.Output, &op.AssetID, &op.ErrorCode)
	if errors.Is(e, sql.ErrNoRows) {
		return op, ErrNotFound
	}
	return op, e
}
func (s *Store) Unsettled(ctx context.Context, run string) (bool, error) {
	var n int
	e := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM agent_operations WHERE run_id=$1 AND status IN ('dispatching','result_unknown','accepted')`, run).Scan(&n)
	return n > 0, e
}
