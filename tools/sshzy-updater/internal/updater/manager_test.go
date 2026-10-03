package updater

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/release"
)

type fakeSource struct {
	target Release
}

func (source fakeSource) List(context.Context) ([]Release, error) {
	return []Release{source.target}, nil
}

func (source fakeSource) Get(_ context.Context, version, hash string) (Release, error) {
	if source.target.Manifest.Version != version || source.target.Hash != hash {
		return Release{}, errors.New("target changed")
	}
	return source.target, nil
}

type fakeDriver struct {
	activated int
	fail      bool
}

func (driver *fakeDriver) Prepare(context.Context, Release, string) (Snapshot, []release.Migration, error) {
	return Snapshot{Version: "0.2.13-r1", Contract: release.ClientContract}, []release.Migration{{Filename: "002_new.sql", Checksum: strings.Repeat("b", 64), Risk: "backward-compatible"}}, nil
}

func (driver *fakeDriver) Activate(_ context.Context, operation Operation, stage func(string)) error {
	driver.activated++
	stage("backing_up")
	if driver.fail {
		return errors.New("activation failed")
	}
	stage("verifying_site")
	return nil
}

func (driver *fakeDriver) Rollback(context.Context, Operation, func(string)) error {
	return nil
}

func testManager(t *testing.T, directory string, driver *fakeDriver) *Manager {
	t.Helper()
	manager, err := NewManager(Config{StateDir: directory, ActivationEnabled: true, PaymentCallbacksReviewed: true}, fakeSource{Release{
		Hash:     strings.Repeat("a", 64),
		Manifest: release.Manifest{Version: "0.2.13-r2"},
	}}, driver)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func waitStage(t *testing.T, manager *Manager, id, expected string) Operation {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		operation, err := manager.Status(id)
		if err != nil {
			t.Fatal(err)
		}
		if operation.Stage == expected {
			return operation
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("stage did not become %s", expected)
	return Operation{}
}

func TestPreparePersistsAndDoesNotActivate(t *testing.T) {
	directory := t.TempDir()
	driver := &fakeDriver{}
	manager := testManager(t, directory, driver)
	request := PrepareRequest{Version: "0.2.13-r2", ManifestHash: strings.Repeat("a", 64), ActorID: 7, IdempotencyKey: "request-1"}
	operation, err := manager.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := manager.Prepare(context.Background(), request)
	if err != nil || duplicate.ID != operation.ID {
		t.Fatal("prepare is not idempotent")
	}
	waitStage(t, manager, operation.ID, "ready")
	if driver.activated != 0 {
		t.Fatal("prepare activated release")
	}
	restarted := testManager(t, directory, driver)
	if _, err := restarted.Status(operation.ID); err != nil {
		t.Fatal("operation disappeared on restart")
	}
}

func TestActivationRequiresExactMigrationAndDowntimeConsent(t *testing.T) {
	driver := &fakeDriver{}
	manager := testManager(t, t.TempDir(), driver)
	operation, err := manager.Prepare(context.Background(), PrepareRequest{Version: "0.2.13-r2", ManifestHash: strings.Repeat("a", 64), ActorID: 7, IdempotencyKey: "request-2"})
	if err != nil {
		t.Fatal(err)
	}
	operation = waitStage(t, manager, operation.ID, "ready")
	for _, request := range []ActivateRequest{
		{ManifestHash: operation.ManifestHash},
		{ManifestHash: "wrong", ConfirmDowntime: true, ConfirmMigrations: []string{"002_new.sql"}},
		{ManifestHash: operation.ManifestHash, ConfirmDowntime: true},
	} {
		if _, err := manager.Activate(operation.ID, request); err == nil {
			t.Fatal("unconfirmed activation accepted")
		}
	}
	_, err = manager.Activate(operation.ID, ActivateRequest{ManifestHash: operation.ManifestHash, ConfirmDowntime: true, ConfirmMigrations: []string{"002_new.sql"}})
	if err != nil {
		t.Fatal(err)
	}
	waitStage(t, manager, operation.ID, "completed")
	if driver.activated != 1 {
		t.Fatal("activation executed more than once")
	}
}

func TestManagerRejectsUnknownAndConcurrentOperations(t *testing.T) {
	manager := testManager(t, t.TempDir(), &fakeDriver{})
	if _, err := manager.Status("../secrets"); err == nil {
		t.Fatal("accepted invalid operation ID")
	}
	request := PrepareRequest{Version: "0.2.13-r2", ManifestHash: strings.Repeat("a", 64), ActorID: 7, IdempotencyKey: "request-3"}
	operation, err := manager.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.IdempotencyKey = "other-request"
	if _, err := manager.Prepare(context.Background(), request); err == nil {
		t.Fatal("accepted concurrent operation")
	}
	waitStage(t, manager, operation.ID, "ready")
}
