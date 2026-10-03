//go:build linux

package updater

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/release"
)

func TestHostLifecycleWithIsolatedFilesystemAndSimulatedCommands(t *testing.T) {
	root := t.TempDir()
	config := Config{StateDir: filepath.Join(root, "state"), DeploymentDir: filepath.Join(root, "deploy"),
		UIRoot: filepath.Join(root, "ui"), ControlDir: filepath.Join(root, "control"), DockerConfig: filepath.Join(root, "docker"), SocketGID: os.Getgid(), ActivationEnabled: true}
	target, _, _, _ := signedFixture(t)
	old := target
	old.Version, old.Tag, old.Backend.Version, old.UI.Asset = "0.2.13-r2", "sshzy-v0.2.13-r2", "0.2.13-r2", "sshzy-ui-0.2.13-r2.tar.gz"
	old.Backend.Digest = "sha256:" + strings.Repeat("f", 64)
	bootstrap := []byte("await import('/assets/index-test.js');")
	old.UI.BootstrapSHA256, target.UI.BootstrapSHA256 = release.Hash(bootstrap), release.Hash(bootstrap)
	target.Migrations = append(target.Migrations, release.Migration{Filename: "002_add.sql", Checksum: strings.Repeat("f", 64), Risk: "backward-compatible", Description: "Optional fixture field"})
	makeUI := func(path string, manifest release.Manifest) {
		t.Helper()
		os.MkdirAll(filepath.Join(path, "assets"), 0755)
		os.WriteFile(filepath.Join(path, "index.html"), []byte("<html></html>"), 0644)
		os.WriteFile(filepath.Join(path, "assets/index-test.js"), []byte("export{}"), 0644)
		os.WriteFile(filepath.Join(path, "assets/fw-cachebust.js"), bootstrap, 0644)
		writeJSON(filepath.Join(path, "build-info.json"), map[string]string{"version": manifest.Version, "commit": manifest.SourceSHA, "contract": release.ClientContract}, 0644)
	}
	oldDirectory := filepath.Join(config.UIRoot, "releases", old.Version)
	makeUI(oldDirectory, old)
	os.MkdirAll(filepath.Join(config.UIRoot, "shared/assets"), 0755)
	os.WriteFile(filepath.Join(config.UIRoot, "shared/assets/fw-cachebust.js"), bootstrap, 0644)
	if err := os.Symlink(oldDirectory, filepath.Join(config.UIRoot, "current")); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(config.DeploymentDir, 0700)
	os.WriteFile(filepath.Join(config.DeploymentDir, ".env"), []byte("SYNTHETIC_FIXTURE=1\n"), 0600)
	os.WriteFile(filepath.Join(config.DeploymentDir, "docker-compose.yml"), []byte("services:\n  sub2api:\n    image: "+old.Backend.Reference()+"\n"), 0600)
	record := Installed{ManifestHash: strings.Repeat("b", 64), Manifest: old, InstalledAt: time.Now()}
	writeJSON(filepath.Join(config.StateDir, "installed.json"), record, 0600)
	writeJSON(filepath.Join(config.StateDir, "history", old.Version+".json"), record, 0600)
	sourceDirectory := filepath.Join(root, "package")
	makeUI(sourceDirectory, target)
	archive := filepath.Join(root, "package.tar.gz")
	if err := packUI(sourceDirectory, archive); err != nil {
		t.Fatal(err)
	}
	archiveData, _ := os.ReadFile(archive)
	target.UI.Size, target.UI.SHA256 = int64(len(archiveData)), release.Hash(archiveData)
	current := old
	running := true
	ledger := map[string]string{"001_init.sql": old.Migrations[0].Checksum}
	source := &GitHubSource{client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(archiveData))), Request: request}, nil
	})}}
	driver := &HostDriver{config: config, source: source}
	driver.runtimeProbe = func(context.Context) (map[string]any, error) {
		var fence struct {
			Active bool `json:"active"`
		}
		readJSON(filepath.Join(config.ControlDir, "maintenance.json"), &fence)
		return map[string]any{"protocol": float64(1), "active": fence.Active, "inflight": float64(0), "version": current.Version, "commit": current.SourceSHA}, nil
	}
	driver.runCommand = func(_ context.Context, name string, arguments ...string) ([]byte, error) {
		joined := strings.Join(arguments, " ")
		switch {
		case name == "rsync":
			os.WriteFile(filepath.Join(config.UIRoot, "shared/assets/index-test.js"), []byte("export{}"), 0644)
		case name == "bash":
			os.Remove(filepath.Join(config.UIRoot, "current"))
			os.Symlink(filepath.Join(config.UIRoot, "releases", arguments[len(arguments)-1]), filepath.Join(config.UIRoot, "current"))
		case strings.Contains(joined, "inspect --format {{.State.Running}}"):
			return []byte(strconv.FormatBool(running)), nil
		case strings.Contains(joined, "inspect --format {{.State.ExitCode}}"):
			return []byte("0"), nil
		case strings.Contains(joined, "image inspect"):
			data, _ := json.Marshal([]any{map[string]any{"Architecture": "amd64", "Os": "linux", "Config": map[string]any{"Labels": map[string]string{"org.opencontainers.image.version": target.Version, "org.opencontainers.image.revision": target.SourceSHA}}}})
			return data, nil
		case strings.Contains(joined, "inspect sub2api"):
			data, _ := json.Marshal([]any{map[string]any{"ID": "fixture-container", "Image": "fixture-image", "Config": map[string]any{"Image": current.Backend.Reference(), "Labels": map[string]string{"org.opencontainers.image.version": current.Version, "org.opencontainers.image.revision": current.SourceSHA}}, "State": map[string]bool{"Running": running}}})
			return data, nil
		case strings.Contains(joined, "SELECT filename,checksum"):
			rows := ""
			for _, migration := range target.Migrations {
				if hash, exists := ledger[migration.Filename]; exists {
					rows += migration.Filename + "|" + hash + "\n"
				}
			}
			return []byte(rows), nil
		case strings.Contains(joined, "config --format json"):
			var file string
			for index, argument := range arguments {
				if argument == "-f" {
					file = arguments[index+1]
				}
			}
			data, _ := os.ReadFile(file)
			image := old.Backend.Reference()
			if strings.Contains(string(data), target.Backend.Reference()) {
				image = target.Backend.Reference()
			}
			output, _ := json.Marshal(map[string]any{"services": map[string]any{"sub2api": map[string]any{"image": image}}})
			return output, nil
		case strings.Contains(joined, "stop --signal SIGTERM --timeout -1"):
			running = false
		case strings.Contains(joined, "up -d"):
			var file string
			for index, argument := range arguments {
				if argument == "-f" {
					file = arguments[index+1]
				}
			}
			content, _ := os.ReadFile(file)
			if strings.Contains(string(content), target.Backend.Reference()) {
				current = target
			} else {
				current = old
			}
			running = true
			ledger["002_add.sql"] = target.Migrations[1].Checksum
		}
		return nil, nil
	}
	driver.backupOverride = func(_ context.Context, operation Operation) (string, error) {
		directory := filepath.Join(config.DeploymentDir, "backups", "updater-"+operation.ID)
		os.MkdirAll(directory, 0700)
		for input, output := range map[string]string{filepath.Join(config.DeploymentDir, "docker-compose.yml"): "compose.yml", filepath.Join(config.StateDir, "installed.json"): "installed.json", filepath.Join(config.UIRoot, "shared/assets/fw-cachebust.js"): "bootstrap.js"} {
			data, _ := os.ReadFile(input)
			atomicWrite(filepath.Join(directory, output), data, 0600)
		}
		return directory, nil
	}
	id := strings.Repeat("a", 32)
	if err := driver.maintenance(false, ""); err != nil {
		t.Fatal(err)
	}
	before, pending, err := driver.Prepare(context.Background(), Release{Hash: strings.Repeat("c", 64), Manifest: target, UIAssetID: 3}, id)
	if err != nil || len(pending) != 1 {
		t.Fatalf("prepare: %v", err)
	}
	if current.Version != old.Version || !running {
		t.Fatal("prepare changed live backend")
	}
	operation := Operation{ID: id, ManifestHash: strings.Repeat("c", 64), Snapshot: before, Target: target, Pending: pending}
	if err := driver.Activate(context.Background(), operation, func(string) {}); err != nil {
		t.Fatal(err)
	}
	var installed Installed
	readJSON(filepath.Join(config.StateDir, "installed.json"), &installed)
	if installed.Manifest.Version != target.Version || current.Version != target.Version || !running {
		t.Fatal("activation not persisted")
	}
	if _, err := driver.snapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	operation.Stage = "manual_intervention"
	manager, err := NewManager(config, fakeSource{Release{Hash: operation.ManifestHash, Manifest: target}}, driver)
	if err != nil {
		t.Fatal(err)
	}
	manager.operations[operation.ID], manager.active = operation, operation.ID
	manager.save(operation)
	driver.maintenance(true, operation.ID)
	if err := manager.Recover(context.Background(), operation.ID, "complete"); err != nil {
		t.Fatal(err)
	}
	status, _ := manager.Status(operation.ID)
	if status.Stage != "completed" {
		t.Fatal("recovery completion not persisted")
	}
	manager.operations[operation.ID], manager.active = operation, operation.ID
	manager.save(operation)
	if err := manager.Recover(context.Background(), operation.ID, "rollback"); err != nil {
		t.Fatal(err)
	}
	if current.Version != old.Version {
		t.Fatal("recovery did not restore old custom version")
	}
}
