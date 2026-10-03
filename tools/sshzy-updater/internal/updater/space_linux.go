//go:build linux

package updater

import "syscall"

func availableSpace(path string, minimum uint64) error {
	var status syscall.Statfs_t
	if syscall.Statfs(path, &status) != nil || status.Bavail*uint64(status.Bsize) < minimum {
		return CodeError("INSUFFICIENT_DISK_SPACE")
	}
	return nil
}
