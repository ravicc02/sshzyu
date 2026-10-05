//go:build linux

package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func maintenanceGID(t *testing.T, path string) int {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("unexpected stat type %T", info.Sys())
	}
	return int(stat.Gid)
}

func TestEnsureMaintenanceOwnershipCreatesFileWithGroup(t *testing.T) {
	dir := t.TempDir()
	gid := os.Getgid()
	driver := &HostDriver{config: Config{ControlDir: dir, SocketGID: gid}}
	if err := driver.ensureMaintenanceOwnership(); err != nil {
		t.Fatalf("ensureMaintenanceOwnership: %v", err)
	}
	path := filepath.Join(dir, "maintenance.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("maintenance file was not created: %v", err)
	}
	if info.Mode().Perm() != 0640 {
		t.Fatalf("mode = %v, want 0640", info.Mode().Perm())
	}
	if got := maintenanceGID(t, path); got != gid {
		t.Fatalf("group = %d, want %d", got, gid)
	}
}

func TestEnsureMaintenanceOwnershipRepairsStaleGroup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "maintenance.json")
	if err := os.WriteFile(path, []byte(`{"active": false, "operation_id": ""}`), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, -1, 0); err != nil {
		t.Skipf("cannot set an alternate group in this environment: %v", err)
	}
	gid := os.Getgid()
	if gid == 0 {
		t.Skip("running as root; group repair is a no-op")
	}
	driver := &HostDriver{config: Config{ControlDir: dir, SocketGID: gid}}
	if err := driver.ensureMaintenanceOwnership(); err != nil {
		t.Fatalf("ensureMaintenanceOwnership: %v", err)
	}
	if got := maintenanceGID(t, path); got != gid {
		t.Fatalf("group was not repaired: got %d, want %d", got, gid)
	}
}

// uiArchive 构建一个包含多条目（含目录）的 tar.gz，供 extractUI 测试使用。
func uiArchive(t *testing.T, entries []struct {
	name    string
	content string
	dir     bool
}) []byte {
	t.Helper()
	var buffer bytes.Buffer
	compressor := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(compressor)
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Mode: 0644}
		if entry.dir {
			header.Typeflag = tar.TypeDir
			header.Name = entry.name + "/"
			header.Mode = 0755
		} else {
			header.Typeflag = tar.TypeReg
			header.Size = int64(len(entry.content))
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if !entry.dir {
			if _, err := writer.Write([]byte(entry.content)); err != nil {
				t.Fatal(err)
			}
		}
	}
	writer.Close()
	compressor.Close()
	return buffer.Bytes()
}

func TestExtractUIProducesWorldReadableAssets(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "ui.tar.gz")
	payload := uiArchive(t, []struct {
		name    string
		content string
		dir     bool
	}{
		{"assets", "", true},
		{"assets/index-abc.js", "console.log(1)", false},
		{"index.html", "<!doctype html>", false},
	})
	if err := os.WriteFile(archive, payload, 0600); err != nil {
		t.Fatal(err)
	}
	// 模拟 daemon unit 的 UMask=0077：没有显式 chmod 时产物会被削成 700/600，
	// nginx worker（www-data）读不到，导致全站 /assets/* 404、前端白屏。
	previous := syscall.Umask(0o077)
	defer syscall.Umask(previous)

	releaseDir := filepath.Join(root, "release")
	if err := extractUI(archive, releaseDir); err != nil {
		t.Fatalf("extractUI: %v", err)
	}
	for path, want := range map[string]os.FileMode{
		releaseDir:                             0755,
		filepath.Join(releaseDir, "assets"):    0755,
		filepath.Join(releaseDir, "index.html"): 0644,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode().Perm() != want {
			t.Fatalf("%s mode = %v, want %v", path, info.Mode().Perm(), want)
		}
	}
}

func TestNormalizeUIPermissionsRepairsUmaskArtifacts(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "assets")
	if err := os.MkdirAll(assets, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(assets, 0700); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(assets, "index-abc.js")
	if err := os.WriteFile(entry, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(entry, 0600); err != nil {
		t.Fatal(err)
	}
	if err := normalizeUIPermissions(root); err != nil {
		t.Fatalf("normalizeUIPermissions: %v", err)
	}
	if info, err := os.Stat(assets); err != nil {
		t.Fatalf("stat assets: %v", err)
	} else if info.Mode().Perm() != 0755 {
		t.Fatalf("assets mode = %v, want 0755", info.Mode().Perm())
	}
	if info, err := os.Stat(entry); err != nil {
		t.Fatalf("stat entry: %v", err)
	} else if info.Mode().Perm() != 0644 {
		t.Fatalf("entry mode = %v, want 0644", info.Mode().Perm())
	}
}
