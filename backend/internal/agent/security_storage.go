package agent

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Wei-Shaw/sub2api/pkg/agentlimits"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

const storageQuota int64 = agentlimits.UserStorageBytes
const generatedAssetReserve int64 = agentlimits.GeneratedImageBytes

func (s *Service) syncCosts(r *Run) {
	if s.Store == nil {
		return
	}
	var model, image string
	err := s.Store.DB.QueryRowContext(context.Background(), `SELECT coalesce(sum(cost) FILTER(WHERE kind='text'),0)::text,coalesce(sum(cost) FILTER(WHERE kind='image'),0)::text FROM agent_operations WHERE run_id=$1`, r.ID).Scan(&model, &image)
	if err != nil {
		slog.Error("agent cost aggregation failed", "run_id", r.ID)
		return
	}
	r.ModelCost, r.ImageCost, r.Spent = model, image, AddAmount(model, image)
}

func storageUsed(ctx context.Context, tx *sql.Tx, user int64) (int64, error) {
	var used int64
	err := tx.QueryRowContext(ctx, `SELECT (SELECT coalesce(sum(byte_size),0) FROM agent_assets WHERE user_id=$1 AND status<>'purged') + (SELECT coalesce(sum(greatest(byte_size,reserved_bytes)),0) FROM agent_checkpoints WHERE user_id=$1 AND purged_at IS NULL)`, user).Scan(&used)
	return used, err
}

// Serialize all quota reservations and uploads on the same owning user row.
func (s *Service) reserveCheckpoint(ctx context.Context, user int64, op Operation) error {
	requested := int64(4 << 20)
	if op.Kind == "image" {
		requested = agentlimits.ImageResponseBytes + generatedAssetReserve
	}
	if err := s.checkDisk(ctx, requested); err != nil {
		return err
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, user).Scan(&user); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_checkpoints(operation_id,user_id,storage_key) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, op.ID, user, op.ID+".result.json")
	if err != nil {
		return err
	}
	var old int64
	if err = tx.QueryRowContext(ctx, `SELECT greatest(byte_size,reserved_bytes) FROM agent_checkpoints WHERE operation_id=$1 AND user_id=$2 AND purged_at IS NULL`, op.ID, user).Scan(&old); err != nil {
		return err
	}
	reserve := int64(4 << 20)
	if op.Kind == "image" {
		reserve = agentlimits.ImageResponseBytes + generatedAssetReserve
	}
	used, err := storageUsed(ctx, tx, user)
	if err != nil {
		return err
	}
	if used-old+reserve > storageQuota {
		return Invalid("AGENT_STORAGE_QUOTA_EXCEEDED")
	}
	if err = s.Store.reserveGlobalStorage(ctx, tx, reserve-old); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE agent_checkpoints SET reserved_bytes=greatest(byte_size,$2),updated_at=now() WHERE operation_id=$1`, op.ID, reserve)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) storeCheckpoint(ctx context.Context, id string, b []byte, hasContent bool) error {
	return s.storeCheckpointStream(ctx, id, int64(len(b)), hasContent, func(w io.Writer) error { _, err := w.Write(b); return err })
}
func (s *Service) storeCheckpointStream(ctx context.Context, id string, size int64, hasContent bool, write func(io.Writer) error) error {
	if id != filepath.Base(id) {
		return ErrNotFound
	}
	if s.Store == nil {
		return s.writePrivateStream(id+".result.json", write)
	} // isolated file validation only
	var user int64
	if err := s.Store.DB.QueryRowContext(ctx, `SELECT r.user_id FROM agent_operations o JOIN agent_runs r ON r.id=o.run_id WHERE o.id=$1`, id).Scan(&user); err != nil {
		return err
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, user).Scan(&user); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_checkpoints(operation_id,user_id,storage_key) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, id, user, id+".result.json")
	if err != nil {
		return err
	}
	var old, reserve int64
	if err = tx.QueryRowContext(ctx, `SELECT byte_size,reserved_bytes FROM agent_checkpoints WHERE operation_id=$1 AND purged_at IS NULL`, id).Scan(&old, &reserve); err != nil {
		return err
	}
	used, err := storageUsed(ctx, tx, user)
	if err != nil {
		return err
	}
	next := size
	if next < reserve {
		next = reserve
	}
	if old > reserve {
		reserve = old
	}
	if used-reserve+next > storageQuota {
		return Invalid("AGENT_STORAGE_QUOTA_EXCEEDED")
	}
	if err = s.Store.reserveGlobalStorage(ctx, tx, next-reserve); err != nil {
		return err
	}
	// Register a conservative size before writing. A crash can overcount, never undercount.
	result, err := tx.ExecContext(ctx, `UPDATE agent_checkpoints SET byte_size=greatest(byte_size,$2),reserved_bytes=greatest(reserved_bytes,$2),content_created_at=CASE WHEN $3 THEN coalesce(content_created_at,now()) ELSE content_created_at END,content_expires_at=CASE WHEN $3 THEN coalesce(content_expires_at,now()+interval '24 hours') ELSE content_expires_at END,updated_at=now() WHERE operation_id=$1 AND (content_expires_at IS NULL OR content_expires_at>now())`, id, size, hasContent)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return Invalid("ASSET_EXPIRED")
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return s.writePrivateStream(id+".result.json", write)
}

func (s *Service) accountExistingCheckpoints(ctx context.Context) error {
	last := ""
	for {
		rows, err := s.Store.DB.QueryContext(ctx, `SELECT operation_id,storage_key FROM agent_checkpoints WHERE purged_at IS NULL AND operation_id>$1 ORDER BY operation_id LIMIT 100`, last)
		if err != nil {
			return err
		}
		type entry struct{ id, key string }
		list := make([]entry, 0, 100)
		for rows.Next() {
			var x entry
			if err = rows.Scan(&x.id, &x.key); err != nil {
				rows.Close()
				return err
			}
			list = append(list, x)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(list) == 0 {
			return nil
		}
		for _, x := range list {
			if x.key != filepath.Base(x.key) || x.key != x.id+".result.json" {
				return ErrNotFound
			}
			info, e := os.Stat(filepath.Join(s.Root, x.key))
			if errors.Is(e, os.ErrNotExist) {
				continue
			}
			if e != nil {
				return e
			}
			if _, e = s.Store.DB.ExecContext(ctx, `UPDATE agent_checkpoints SET byte_size=greatest(byte_size,$2) WHERE operation_id=$1`, x.id, info.Size()); e != nil {
				return e
			}
		}
		last = list[len(list)-1].id
	}
}
