//go:build linux

package updater

import "syscall"

// errCrossDevice 对应 rename(2) 在源与目标位于不同挂载点时返回的 EXDEV。
// 跨两个独立 bind mount 的 rename 会返回它，即使 stat 报告的设备号相同
// （bind mount 保留 st_dev，VFS 按 vfsmount 判定）。
var errCrossDevice error = syscall.EXDEV
