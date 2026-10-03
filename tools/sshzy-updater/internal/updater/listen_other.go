//go:build !linux

package updater

import (
	"context"
	"errors"
)

func Serve(context.Context, Config, *Manager, string) error {
	return errors.New("updater daemon requires Linux")
}
