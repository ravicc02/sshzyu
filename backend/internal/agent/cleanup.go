package agent

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

const ContentRetention = 24 * time.Hour

func (s *Service) checkpointReadable(ctx context.Context, id string) error {
	if id != filepath.Base(id) {
		return ErrNotFound
	}
	var available bool
	err := s.Store.DB.QueryRowContext(ctx, `SELECT purged_at IS NULL AND (content_expires_at>now() OR (content_expires_at IS NULL AND updated_at>now()-interval '24 hours')) FROM agent_checkpoints WHERE operation_id=$1`, id).Scan(&available)
	if err != nil {
		return err
	}
	if !available {
		return Invalid("ASSET_EXPIRED")
	}
	return nil
}

// Registered files only. Logical expiry applies on access, independently of
// this bounded physical cleanup. A failed unlink remains eligible next pass.
func (s *Service) Cleanup(ctx context.Context) error {
	s.expirePreviews()
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT id,user_id,storage_key FROM agent_assets WHERE expires_at<=now() AND status IN ('ready','deleted','expired','uploading') ORDER BY expires_at LIMIT 50`)
	if err != nil {
		return err
	}
	type candidate struct {
		id   string
		user int64
		key  string
	}
	list := []candidate{}
	for rows.Next() {
		var a candidate
		if err = rows.Scan(&a.id, &a.user, &a.key); err != nil {
			rows.Close()
			return err
		}
		list = append(list, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, a := range list {
		if a.key != filepath.Base(a.key) || (a.key != a.id+".image" && a.key != a.id+".attachment") {
			continue
		}
		tx, e := s.Store.DB.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		var user int64
		if e = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, a.user).Scan(&user); e != nil {
			tx.Rollback()
			return e
		}
		res, e := tx.ExecContext(ctx, `UPDATE agent_assets a SET status='expired' WHERE id=$1 AND expires_at<=now() AND status IN ('ready','deleted','expired','uploading') AND NOT EXISTS(SELECT 1 FROM agent_asset_holds h JOIN agent_runs r ON r.id=h.run_id WHERE h.asset_id=a.id AND h.until_at>now() AND r.user_id=a.user_id AND r.status NOT IN ('completed','partial_success','failed','cancelled','expired'))`, a.id)
		if e != nil {
			tx.Rollback()
			return e
		}
		n, _ := res.RowsAffected()
		if e = tx.Commit(); e != nil {
			return e
		}
		if n == 0 {
			continue
		}
		e = os.Remove(filepath.Join(s.Root, a.key))
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if _, e = s.Store.DB.ExecContext(ctx, `UPDATE agent_assets SET status='purged' WHERE id=$1 AND status='expired'`, a.id); e != nil {
			return e
		}
	}
	rows, err = s.Store.DB.QueryContext(ctx, `SELECT operation_id,storage_key FROM agent_checkpoints WHERE purged_at IS NULL AND coalesce(content_expires_at,updated_at+interval '24 hours')<=now() ORDER BY content_expires_at NULLS FIRST LIMIT 100`)
	if err != nil {
		return err
	}
	list = nil
	for rows.Next() {
		var a candidate
		if err = rows.Scan(&a.id, &a.key); err != nil {
			rows.Close()
			return err
		}
		list = append(list, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, a := range list {
		if a.key != filepath.Base(a.key) || a.key != a.id+".result.json" {
			continue
		}
		e := os.Remove(filepath.Join(s.Root, a.key))
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if _, err = s.Store.DB.ExecContext(ctx, `UPDATE agent_checkpoints SET purged_at=now(),byte_size=0,reserved_bytes=0 WHERE operation_id=$1 AND coalesce(content_expires_at,updated_at+interval '24 hours')<=now()`, a.id); err != nil {
			return err
		}
	}
	// Protocol history can contain extracted original text. Keep conversational
	// messages/summaries, IDs, prices and audit evidence, discard raw replay data.
	_, err = s.Store.DB.ExecContext(ctx, `WITH candidates AS (SELECT id FROM agent_runs WHERE created_at<now()-interval '24 hours' AND status IN ('completed','partial_success','failed','cancelled','expired') AND (snapshot ? 'protocol_history' OR snapshot ? 'pending_tools') ORDER BY created_at LIMIT 100) UPDATE agent_runs r SET snapshot=r.snapshot-'protocol_history'-'stream_prefix'-'pending_tools' FROM candidates c WHERE r.id=c.id`)
	if err != nil {
		return err
	}
	// Compact stale duplicate process events of terminal runs. Final
	// messages, snapshots, operation IDs and billing records remain authoritative.
	_, err = s.Store.DB.ExecContext(ctx, `WITH stale AS(SELECT e.run_id,e.sequence FROM agent_run_events e JOIN agent_runs r ON r.id=e.run_id WHERE e.type NOT IN ('run.completed','run.partial_success','run.failed','run.cancelled','run.expired') AND e.created_at<now()-interval '24 hours' AND r.status IN ('completed','partial_success','failed','cancelled','expired') ORDER BY e.created_at LIMIT 1000) DELETE FROM agent_run_events e USING stale s WHERE e.run_id=s.run_id AND e.sequence=s.sequence`)
	if err != nil {
		return err
	}
	_, err = s.Store.DB.ExecContext(ctx, `DELETE FROM agent_asset_holds h WHERE h.until_at<=now() OR EXISTS(SELECT 1 FROM agent_runs r WHERE r.id=h.run_id AND r.status IN ('completed','partial_success','failed','cancelled','expired'))`)
	return err
}
