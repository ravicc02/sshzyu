package updater

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/release"
)

type catalogSource struct {
	targets []Release
	err     error
}

func (source catalogSource) List(context.Context) ([]Release, error) {
	return source.targets, source.err
}
func (source catalogSource) Get(context.Context, string, string) (Release, error) {
	return Release{}, errors.New("unexpected lookup")
}

func catalogFixture(t *testing.T, version string) release.Manifest {
	t.Helper()
	manifest, _, _, _ := signedFixture(t)
	manifest.Version, manifest.Tag = version, "sshzy-v"+version
	manifest.Backend.Version = version
	manifest.UI.Asset = "sshzy-ui-" + version + ".tar.gz"
	return manifest
}

func TestReleaseCatalogUsesActualDeploymentHistoryForRollback(t *testing.T) {
	manager := testManager(t, t.TempDir(), &fakeDriver{})
	current := catalogFixture(t, "0.2.13-r4")
	old := catalogFixture(t, "0.2.13-r3")
	neverInstalled := catalogFixture(t, "0.2.13-r2")
	hash := strings.Repeat("b", 64)
	manager.source = catalogSource{targets: []Release{
		{Manifest: old, Hash: hash},
		{Manifest: neverInstalled, Hash: strings.Repeat("c", 64)},
		{Manifest: current, Hash: strings.Repeat("d", 64)},
	}}
	date := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	if err := writeJSON(filepath.Join(manager.config.StateDir, "installed.json"), Installed{Manifest: current}, 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(manager.config.StateDir, "history", old.Version+".json"), Installed{Manifest: old, ManifestHash: hash, InstalledAt: date}, 0600); err != nil {
		t.Fatal(err)
	}
	targets, err := manager.releaseCatalog(context.Background())
	if err != nil || len(targets) != 3 {
		t.Fatalf("unexpected catalog: %v", err)
	}
	if !targets[0].RollbackAvailable || targets[0].InstalledAt == nil || !targets[0].InstalledAt.Equal(date) {
		t.Fatal("actual previous installation not offered")
	}
	if targets[1].RollbackAvailable || targets[2].RollbackAvailable {
		t.Fatal("uninstalled or current version offered for rollback")
	}
}

func TestReleaseCatalogRejectsAlteredHistoryAndUnsafeSchema(t *testing.T) {
	for _, scenario := range []string{"hash", "manifest", "schema", "nontransactional", "disabled", "checksum"} {
		t.Run(scenario, func(t *testing.T) {
			manager := testManager(t, t.TempDir(), &fakeDriver{})
			current := catalogFixture(t, "0.2.13-r4")
			old := catalogFixture(t, "0.2.13-r3")
			hash := strings.Repeat("b", 64)
			history := Installed{Manifest: old, ManifestHash: hash, InstalledAt: time.Now().UTC()}
			switch scenario {
			case "hash":
				history.ManifestHash = strings.Repeat("c", 64)
			case "manifest":
				history.Manifest.SourceSHA = strings.Repeat("f", 40)
			case "schema", "nontransactional":
				risk := "manual"
				if scenario == "nontransactional" {
					risk = "backward-compatible"
				}
				current.Migrations = append(current.Migrations, release.Migration{
					Filename: "002_added.sql", Checksum: strings.Repeat("f", 64),
					Risk: risk, Description: "Fixture incompatible schema",
					NonTransactional: scenario == "nontransactional",
				})
			case "checksum":
				current.Migrations[0].Checksum = strings.Repeat("f", 64)
			case "disabled":
				current.Compatibility.RollbackCompatible = false
			}
			manager.source = catalogSource{targets: []Release{{Manifest: old, Hash: hash}}}
			if err := writeJSON(filepath.Join(manager.config.StateDir, "installed.json"), Installed{Manifest: current}, 0600); err != nil {
				t.Fatal(err)
			}
			if err := writeJSON(filepath.Join(manager.config.StateDir, "history", old.Version+".json"), history, 0600); err != nil {
				t.Fatal(err)
			}
			targets, err := manager.releaseCatalog(context.Background())
			if err != nil || targets[0].RollbackAvailable {
				t.Fatalf("unsafe rollback accepted: %v", err)
			}
		})
	}
}

func TestReleaseCatalogWithoutVerifiedHistoryFailsClosed(t *testing.T) {
	manager := testManager(t, t.TempDir(), &fakeDriver{})
	manager.source = catalogSource{targets: []Release{{
		Manifest: catalogFixture(t, "0.2.13-r3"), RollbackAvailable: true,
	}}}
	targets, err := manager.releaseCatalog(context.Background())
	if err != nil || targets[0].RollbackAvailable || targets[0].InstalledAt != nil {
		t.Fatal("missing local history accepted a source-supplied rollback flag")
	}
	manager.source = catalogSource{err: errors.New("source unavailable")}
	if _, err := manager.releaseCatalog(context.Background()); err == nil {
		t.Fatal("source failure masked as an empty verified catalog")
	}
}

func TestRollbackSchemaRequiresMatchingContractAndExistingTargetMigrations(t *testing.T) {
	current := catalogFixture(t, "0.2.13-r4")
	target := catalogFixture(t, "0.2.13-r3")
	target.UI.Contract = "different-contract"
	if rollbackSchemaCompatible(current, target) {
		t.Fatal("different UI contract accepted")
	}
	target.UI.Contract = current.UI.Contract
	target.Migrations = append(target.Migrations, release.Migration{Filename: "002_unknown.sql"})
	if rollbackSchemaCompatible(current, target) {
		t.Fatal("missing target migration accepted")
	}
}

func TestCapabilitiesDoNotHideManualIntervention(t *testing.T) {
	manager := testManager(t, t.TempDir(), &fakeDriver{})
	if manager.currentOperation() != nil {
		t.Fatal("inactive updater returned a historical operation")
	}
	manager.operations["failed"] = Operation{Stage: "failed", UpdatedAt: time.Now().UTC()}
	manager.operations["manual"] = Operation{ID: "manual", Stage: "manual_intervention", UpdatedAt: time.Now().UTC()}
	if attention := manager.currentOperation(); attention == nil || attention.ID != "manual" {
		t.Fatal("maintenance recovery requirement was hidden")
	}
}
