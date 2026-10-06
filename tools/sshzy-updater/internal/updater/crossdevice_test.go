package updater

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestMaterializeUIFallsBackToCopyOnCrossDevice(t *testing.T) {
	var calls [][]string
	driver := &HostDriver{
		renameOverride: func(string, string) error { return fmt.Errorf("rename: %w", errCrossDevice) },
		runCommand: func(_ context.Context, name string, arguments ...string) ([]byte, error) {
			calls = append(calls, append([]string{name}, arguments...))
			return nil, nil
		},
	}
	if err := driver.materializeUI(context.Background(), "/state/prepared/op/ui", "/opt/sshzyu-ui/releases/0.2.13-r9"); err != nil {
		t.Fatalf("materializeUI: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected cp then rm, got %v", calls)
	}
	if calls[0][0] != "cp" || !strings.Contains(strings.Join(calls[0], " "), "-a") {
		t.Fatalf("first command should be cp -a, got %v", calls[0])
	}
	if calls[1][0] != "rm" || calls[1][1] != "-rf" {
		t.Fatalf("second command should be rm -rf, got %v", calls[1])
	}
}

func TestMaterializeUISucceedsWithoutFallbackWhenRenameWorks(t *testing.T) {
	renamed := false
	driver := &HostDriver{
		renameOverride: func(string, string) error { renamed = true; return nil },
		runCommand: func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("runCommand must not be called when rename succeeds")
			return nil, nil
		},
	}
	if err := driver.materializeUI(context.Background(), "/s", "/d"); err != nil {
		t.Fatalf("materializeUI: %v", err)
	}
	if !renamed {
		t.Fatal("expected rename to be attempted")
	}
}

func TestMaterializeUIPropagatesNonCrossDeviceError(t *testing.T) {
	ranCommand := false
	driver := &HostDriver{
		renameOverride: func(string, string) error { return errors.New("permission denied") },
		runCommand: func(context.Context, string, ...string) ([]byte, error) {
			ranCommand = true
			return nil, nil
		},
	}
	err := driver.materializeUI(context.Background(), "/s", "/d")
	if err == nil || err.Error() != "permission denied" {
		t.Fatalf("expected original error, got %v", err)
	}
	if ranCommand {
		t.Fatal("must not fall back on non cross-device errors")
	}
}

func TestMaterializeUISurfacesCopyFailure(t *testing.T) {
	driver := &HostDriver{
		renameOverride: func(string, string) error { return fmt.Errorf("rename: %w", errCrossDevice) },
		runCommand: func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name == "cp" {
				return nil, errors.New("cp failed")
			}
			return nil, nil
		},
	}
	err := driver.materializeUI(context.Background(), "/s", "/d")
	if !errors.Is(err, errCrossDevice) {
		t.Fatalf("expected rename error surfaced when cp fails, got %v", err)
	}
}
