//go:build !linux

package updater

import "errors"

// 非 Linux 平台不运行生产 daemon；保留同名变量以便跨平台编译与测试。
var errCrossDevice error = errors.New("cross-device link not permitted")
