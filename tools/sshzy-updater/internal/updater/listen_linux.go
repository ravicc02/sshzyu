//go:build linux

package updater

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func Serve(ctx context.Context, config Config, manager *Manager, token string) error {
	if filepath.Dir(config.Socket) != config.ControlDir {
		return errors.New("socket must stay inside the control directory")
	}
	if err := os.MkdirAll(config.ControlDir, 0750); err != nil {
		return err
	}
	if err := os.Chown(config.ControlDir, -1, config.SocketGID); err != nil {
		return err
	}
	if err := os.Chmod(config.ControlDir, 0750); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(config.StateDir, "executor.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return errors.New("another updater owns the deployment lock")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if info, err := os.Lstat(config.Socket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return errors.New("control socket path is not a socket")
		}
		connection, err := net.DialTimeout("unix", config.Socket, time.Second)
		if err == nil {
			connection.Close()
			return errors.New("control socket is already active")
		}
		if err := os.Remove(config.Socket); err != nil {
			return err
		}
	}
	listener, err := net.Listen("unix", config.Socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(config.Socket, 0660); err != nil {
		return err
	}
	if err := os.Chown(config.Socket, -1, config.SocketGID); err != nil {
		return err
	}
	server := &http.Server{Handler: manager.Handler(token), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 3 * time.Minute, IdleTimeout: 30 * time.Second,
		MaxHeaderBytes: 8192}
	server.ConnContext = func(ctx context.Context, connection net.Conn) context.Context {
		allowed := false
		if unix, ok := connection.(*net.UnixConn); ok {
			raw, err := unix.SyscallConn()
			if err == nil {
				_ = raw.Control(func(descriptor uintptr) {
					credentials, err := syscall.GetsockoptUcred(int(descriptor), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
					allowed = err == nil && (credentials.Uid == 0 || credentials.Uid == config.ClientUID)
				})
			}
		}
		return context.WithValue(ctx, peerKey{}, allowed)
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
