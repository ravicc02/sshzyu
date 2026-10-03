package deployment

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGateTracksInflightAndFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "maintenance.json")
	os.WriteFile(path, []byte(`{"active":false,"operation_id":""}`), 0600)
	gate := NewGate(path)
	done, err := gate.Enter()
	if err != nil || gate.Inflight() != 1 {
		t.Fatal("request not tracked")
	}
	os.WriteFile(path, []byte(`{"active":true,"operation_id":"test"}`), 0600)
	if _, err := gate.Enter(); err == nil {
		t.Fatal("maintenance allowed new work")
	}
	done()
	done()
	if gate.Inflight() != 0 {
		t.Fatal("completion is not idempotent")
	}
	os.WriteFile(path, []byte(`invalid`), 0600)
	if _, err := gate.Enter(); err == nil {
		t.Fatal("invalid fence allowed work")
	}
	os.Remove(path)
	if _, err := gate.Enter(); err == nil {
		t.Fatal("missing configured fence allowed work")
	}
}

func TestWaitUntilOpenHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "maintenance.json")
	os.WriteFile(path, []byte(`{"active":true}`), 0600)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if NewGate(path).Wait(ctx) == nil {
		t.Fatal("wait ignored cancellation")
	}
	if NewGate("").Wait(context.Background()) != nil {
		t.Fatal("unconfigured development gate blocked")
	}
}

func TestBackgroundStartupWaitsForMaintenanceAndClosesCleanly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "maintenance.json")
	os.WriteFile(path, []byte(`{"active":true}`), 0600)
	previous := Default
	Default = NewGate(path)
	defer func() { Default.Close(); Default = previous }()
	started := make(chan struct{}, 2)
	StartWhenOpen(func() { started <- struct{}{} })
	select {
	case <-started:
		t.Fatal("background service started while fenced")
	case <-time.After(20 * time.Millisecond):
	}
	os.WriteFile(path, []byte(`{"active":false}`), 0600)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background service did not resume")
	}
	StartWhenOpen(func() { started <- struct{}{} })
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("normal startup was delayed")
	}
}
