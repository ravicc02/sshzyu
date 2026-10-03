package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/release"
)

func (driver *HostDriver) ControlToken() string { return driver.controlToken }

func (driver *HostDriver) Bootstrap(ctx context.Context, manifestPath, signaturePath string) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return CodeError("MANIFEST_UNAVAILABLE")
	}
	signature, err := os.ReadFile(signaturePath)
	if err != nil {
		return CodeError("SIGNATURE_UNAVAILABLE")
	}
	manifest, err := release.Verify(data, signature, driver.source.publicKey)
	if err != nil || manifest.SigningKeyID != driver.config.SigningKeyID {
		return CodeError("RELEASE_SIGNATURE_INVALID")
	}
	path := filepath.Join(driver.config.StateDir, "installed.json")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return CodeError("BOOTSTRAP_ALREADY_EXISTS")
	}
	record := Installed{ManifestHash: release.Hash(data), Manifest: *manifest, InstalledAt: time.Now().UTC()}
	if _, err := driver.snapshotFor(ctx, &record); err != nil {
		return CodeError("BOOTSTRAP_VERIFICATION_FAILED")
	}
	if err := writeJSON(path, record, 0600); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(driver.config.StateDir, "history", manifest.Version+".json"), record, 0600); err != nil {
		return err
	}
	statePath := filepath.Join(driver.config.ControlDir, "maintenance.json")
	if _, err := os.Stat(statePath); errors.Is(err, os.ErrNotExist) {
		return driver.maintenance(false, "")
	}
	return nil
}

func (manager *Manager) Recover(ctx context.Context, id, decision string) error {
	if !manager.config.ActivationEnabled || !operationPattern.MatchString(id) ||
		(decision != "complete" && decision != "rollback") {
		return CodeError("RECOVERY_CONFIRMATION_REQUIRED")
	}
	operation, err := manager.Status(id)
	if err != nil || operation.Stage != "manual_intervention" {
		return CodeError("RECOVERY_NOT_REQUIRED")
	}
	driver, ok := manager.driver.(*HostDriver)
	if !ok {
		return CodeError("RECOVERY_DRIVER_UNAVAILABLE")
	}
	if _, err := manager.source.Get(ctx, operation.Target.Version, operation.ManifestHash); err != nil {
		return err
	}
	if decision == "complete" {
		if err := driver.waitHealthy(ctx, operation.Target); err != nil {
			return err
		}
		current, err := filepath.EvalSymlinks(filepath.Join(driver.config.UIRoot, "current"))
		if err != nil || current != filepath.Join(driver.config.UIRoot, "releases", operation.Target.Version) {
			return CodeError("UI_RECOVERY_INCOMPLETE")
		}
		if err := driver.verifyUI(operation.Target, current); err != nil {
			return err
		}
		record := Installed{ManifestHash: operation.ManifestHash, Manifest: operation.Target, InstalledAt: time.Now().UTC()}
		if err := writeJSON(filepath.Join(driver.config.StateDir, "history", record.Manifest.Version+".json"), record, 0600); err != nil {
			return err
		}
		if err := writeJSON(filepath.Join(driver.config.StateDir, "installed.json"), record, 0600); err != nil {
			return err
		}
		if err := driver.maintenance(false, ""); err != nil {
			return err
		}
		manager.setStage(id, "completed", "")
		return nil
	}
	for _, migration := range operation.Pending {
		if migration.Risk != "backward-compatible" || migration.NonTransactional {
			return CodeError("MANUAL_DATABASE_RECOVERY_REQUIRED")
		}
	}
	ledger, err := driver.ledger(ctx)
	if err != nil {
		return err
	}
	expected := map[string]string{}
	for _, migration := range operation.Target.Migrations {
		expected[migration.Filename] = migration.Checksum
	}
	for name, hash := range ledger {
		if operation.Snapshot.Ledger[name] != hash && expected[name] != hash {
			return CodeError("MANUAL_DATABASE_RECOVERY_REQUIRED")
		}
	}
	for name, hash := range operation.Snapshot.Ledger {
		if ledger[name] != hash {
			return CodeError("MANUAL_DATABASE_RECOVERY_REQUIRED")
		}
	}
	backup := filepath.Join(driver.config.DeploymentDir, "backups", "updater-"+operation.ID)
	var old Installed
	if readJSON(filepath.Join(backup, "installed.json"), &old) != nil || old.Manifest.Validate() != nil ||
		old.Manifest.Version != operation.Snapshot.Version {
		return CodeError("RECOVERY_BACKUP_INVALID")
	}
	compose, err := os.ReadFile(filepath.Join(backup, "compose.yml"))
	if err != nil || release.Hash(compose) != operation.Snapshot.ComposeHash {
		return CodeError("RECOVERY_BACKUP_INVALID")
	}
	environmentHash, err := fileHash(filepath.Join(driver.config.DeploymentDir, ".env"))
	if err != nil || environmentHash != operation.Snapshot.EnvironmentHash {
		return CodeError("DEPLOYMENT_DRIFT")
	}
	if err := driver.maintenance(true, operation.ID); err != nil {
		return err
	}
	active := filepath.Join(driver.config.DeploymentDir, "docker-compose.yml")
	if err := atomicWrite(active, compose, 0600); err != nil {
		return err
	}
	if _, err := driver.compose(ctx, active, "up", "-d", "--no-build", "--no-deps", "--force-recreate", "sub2api"); err != nil {
		return err
	}
	if err := driver.waitHealthy(ctx, old.Manifest); err != nil {
		return err
	}
	bootstrap, err := os.ReadFile(filepath.Join(backup, "bootstrap.js"))
	if err != nil || release.Hash(bootstrap) != operation.Snapshot.BootstrapHash {
		return CodeError("RECOVERY_BACKUP_INVALID")
	}
	if err := atomicWrite(filepath.Join(driver.config.UIRoot, "shared/assets/fw-cachebust.js"), bootstrap, 0644); err != nil {
		return err
	}
	if _, err := driver.run(ctx, "bash", filepath.Join(driver.config.UIRoot, "switch-release.sh"), old.Manifest.Version); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(driver.config.StateDir, "installed.json"), old, 0600); err != nil {
		return err
	}
	if err := driver.maintenance(false, ""); err != nil {
		return err
	}
	manager.setStage(id, "rolled_back", "")
	return nil
}
