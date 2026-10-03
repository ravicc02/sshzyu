//go:build !linux

package updater

func availableSpace(string, uint64) error {
	return CodeError("LINUX_HOST_REQUIRED")
}
