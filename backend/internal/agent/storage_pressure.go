package agent

import (
	"context"
	"database/sql"
	"github.com/shirou/gopsutil/v4/disk"
	"os"
	"path/filepath"
)

const DefaultStorageBytes int64 = 6 << 30
const DefaultMinFreeBytes int64 = 8 << 30

// Serialize capacity reservations across uploads and runs, including processes.
// This lock changes only storage accounting, never generation concurrency.
func (s *Store) reserveGlobalStorage(ctx context.Context, tx *sql.Tx, growth int64) error {
	if growth <= 0 || s.MaxStorageBytes <= 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(79310255)`); err != nil {
		return err
	}
	var used int64
	err := tx.QueryRowContext(ctx, `SELECT (SELECT coalesce(sum(byte_size),0) FROM agent_assets WHERE status<>'purged')+(SELECT coalesce(sum(greatest(byte_size,reserved_bytes)),0) FROM agent_checkpoints WHERE purged_at IS NULL)`).Scan(&used)
	if err != nil {
		return err
	}
	if growth > s.MaxStorageBytes-used {
		return Invalid("AGENT_STORAGE_CAPACITY")
	}
	return nil
}
func (s *Service) checkDisk(ctx context.Context, reserve int64) error {
	if s.MinFreeDiskBytes <= 0 {
		return nil
	}
	path := s.Root
	for {
		if _, err := os.Stat(path); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return Invalid("AGENT_STORAGE_UNAVAILABLE")
		}
		path = parent
	}
	stat, err := disk.UsageWithContext(ctx, path)
	if err != nil {
		return Invalid("AGENT_STORAGE_UNAVAILABLE")
	}
	if int64(stat.Free)-reserve < s.MinFreeDiskBytes {
		return Invalid("AGENT_DISK_PRESSURE")
	}
	return nil
}
