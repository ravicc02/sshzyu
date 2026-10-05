//go:build linux

package updater

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupUsesPrivateFilesAndNeverRestoresDatabase(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	os.MkdirAll(bin, 0700)
	os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\ncase \"$*\" in\n*pg_dump*) printf 'synthetic-backup';;\n*pg_restore*) cat >/dev/null;;\n*) exit 1;;\nesac\n"), 0700)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	config := Config{DeploymentDir: filepath.Join(root, "deploy"), UIRoot: filepath.Join(root, "ui"), StateDir: filepath.Join(root, "state"), DockerConfig: filepath.Join(root, "docker-config")}
	for path, data := range map[string]string{
		filepath.Join(config.DeploymentDir, "docker-compose.yml"):     "synthetic-compose",
		filepath.Join(config.DeploymentDir, ".env"):                   "SYNTHETIC_FIXTURE=1",
		filepath.Join(config.DeploymentDir, "data/config.yaml"):       "synthetic: true",
		filepath.Join(config.UIRoot, "shared/assets/fw-cachebust.js"): "synthetic-bootstrap",
		filepath.Join(config.StateDir, "installed.json"):              "{}",
	} {
		atomicWrite(path, []byte(data), 0600)
	}
	driver := &HostDriver{config: config}
	operation := Operation{ID: strings.Repeat("a", 32)}
	path, err := driver.backup(context.Background(), operation)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"database.dump", "database.sha256", "compose.yml", "runtime.env", "config.yaml", "bootstrap.js", "installed.json", "snapshot.json"} {
		stat, err := os.Stat(filepath.Join(path, name))
		if err != nil || stat.Mode().Perm() != 0600 {
			t.Fatalf("unsafe or missing backup %s: %v", name, err)
		}
	}
	if _, err := driver.backup(context.Background(), operation); err == nil {
		t.Fatal("existing backup overwritten")
	}
	if _, err := command(context.Background(), "missing-sshzy-test-command"); err == nil {
		t.Fatal("command failure masked")
	}
}

func TestProductionDriverRejectsForeignHostAndPaths(t *testing.T) {
	config := Config{ExpectedHostname: "other-host"}
	if _, err := NewHostDriver(config, nil); err == nil {
		t.Fatal("foreign host accepted")
	}
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		t.Skip("host identity unavailable")
	}
	config.ExpectedHostname = hostname
	if _, err := NewHostDriver(config, nil); err == nil {
		t.Fatal("invalid paths accepted")
	}
	token := filepath.Join(t.TempDir(), "synthetic-control-token")
	os.WriteFile(token, []byte("unit-test-control"), 0600)
	config.StateDir, config.DeploymentDir = "/opt/sub2api-deploy/updater/state", "/opt/sub2api-deploy"
	config.UIRoot, config.ControlDir, config.DockerConfig = "/opt/sshzyu-ui", "/run/sshzy-updater", "/var/lib/sshzy-updater/docker"
	config.ControlTokenFile = token
	driver, err := NewHostDriver(config, nil)
	if err != nil || driver.ControlToken() != "unit-test-control" {
		t.Fatalf("valid driver configuration failed: %v", err)
	}
}
