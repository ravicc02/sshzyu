package updater

import (
	"context"
	"path/filepath"
	"reflect"

	"github.com/Wei-Shaw/sub2api/pkg/release"
)

func (manager *Manager) currentOperation() *Operation {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if operation, exists := manager.operations[manager.active]; exists {
		return &operation
	}
	var attention *Operation
	for _, operation := range manager.operations {
		if operation.Stage == "manual_intervention" && (attention == nil || operation.UpdatedAt.After(attention.UpdatedAt)) {
			copy := operation
			attention = &copy
		}
	}
	return attention
}

func (manager *Manager) releaseCatalog(ctx context.Context) ([]Release, error) {
	sourceTargets, err := manager.source.List(ctx)
	if err != nil {
		return nil, err
	}
	targets := append([]Release{}, sourceTargets...)
	var installed Installed
	validInstallation := readJSON(filepath.Join(manager.config.StateDir, "installed.json"), &installed) == nil &&
		installed.Manifest.Validate() == nil
	current, _ := release.ParseVersion(installed.Manifest.Version)
	for index := range targets {
		targets[index].RollbackAvailable, targets[index].InstalledAt = false, nil
		target := targets[index]
		version, err := release.ParseVersion(target.Manifest.Version)
		if !validInstallation || err != nil || version.Compare(current) >= 0 || target.Manifest.Validate() != nil ||
			!rollbackSchemaCompatible(installed.Manifest, target.Manifest) {
			continue
		}
		var history Installed
		if readJSON(filepath.Join(manager.config.StateDir, "history", target.Manifest.Version+".json"), &history) != nil ||
			history.ManifestHash != target.Hash || history.InstalledAt.IsZero() ||
			!reflect.DeepEqual(history.Manifest, target.Manifest) {
			continue
		}
		targets[index].RollbackAvailable = true
		targets[index].InstalledAt = &history.InstalledAt
	}
	return targets, nil
}

func rollbackSchemaCompatible(current, target release.Manifest) bool {
	if !current.Compatibility.RollbackCompatible || target.UI.Contract != current.UI.Contract {
		return false
	}
	targetMigrations := make(map[string]string, len(target.Migrations))
	for _, migration := range target.Migrations {
		targetMigrations[migration.Filename] = migration.Checksum
	}
	for _, migration := range current.Migrations {
		if checksum, exists := targetMigrations[migration.Filename]; exists {
			if checksum != migration.Checksum {
				return false
			}
			delete(targetMigrations, migration.Filename)
		} else if migration.Risk != "backward-compatible" || migration.NonTransactional {
			return false
		}
	}
	return len(targetMigrations) == 0
}
