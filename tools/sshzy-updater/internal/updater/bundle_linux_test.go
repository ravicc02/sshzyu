//go:build linux

package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/release"
)

func TestBundleBuildsSignedMatchingArtifactsAndRejectsUnreviewedInputs(t *testing.T) {
	root := t.TempDir()
	executables := filepath.Join(root, "mock-bin")
	os.MkdirAll(executables, 0700)
	os.WriteFile(filepath.Join(executables, "git"), []byte("#!/bin/sh\ncase \"$*\" in\n*rev-parse*) printf '"+strings.Repeat("a", 40)+"\\n';;\n*status*) exit 0;;\n*) exit 1;;\nesac\n"), 0700)
	t.Setenv("PATH", executables+":"+os.Getenv("PATH"))
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	keyBytes, _ := x509.MarshalPKCS8PrivateKey(privateKey)
	keyPath := filepath.Join(root, "synthetic-signing-key.pem")
	os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}), 0600)
	writeJSON(filepath.Join(root, "upstream-baseline.json"), map[string]string{
		"upstream_repository": "Wei-Shaw/sub2api", "upstream_release": "v0.2.13",
		"upstream_commit": strings.Repeat("b", 40), "local_build_version": "0.2.13-r3",
	}, 0600)
	os.MkdirAll(filepath.Join(root, "backend/cmd/server"), 0700)
	os.WriteFile(filepath.Join(root, "backend/cmd/server/VERSION"), []byte("0.2.13-r3\n"), 0600)
	dist := filepath.Join(root, "backend/internal/web/dist")
	os.MkdirAll(filepath.Join(dist, "assets"), 0755)
	os.WriteFile(filepath.Join(dist, "index.html"), []byte("<html></html>"), 0644)
	os.WriteFile(filepath.Join(dist, "assets/index-test.js"), []byte("export{}"), 0644)
	os.WriteFile(filepath.Join(dist, "assets/fw-cachebust.js"), []byte("await import('/assets/index-test.js');"), 0644)
	writeJSON(filepath.Join(dist, "build-info.json"), map[string]string{
		"version": "0.2.13-r3", "commit": strings.Repeat("a", 40), "contract": release.ClientContract, "build_type": "release",
	}, 0644)
	receipt := filepath.Join(root, "verification.json")
	writeJSON(receipt, map[string]any{"source_sha": strings.Repeat("a", 40), "tests_passed": true}, 0600)
	os.MkdirAll(filepath.Join(root, "backend/migrations"), 0700)
	os.WriteFile(filepath.Join(root, "backend/migrations/001_init.sql"), []byte("SELECT 1;\n"), 0600)
	writeJSON(filepath.Join(root, "scripts/release/migration-reviews.json"), map[string]release.Migration{
		"001_init.sql": {Checksum: release.MigrationChecksum("SELECT 1;"), Risk: "backward-compatible", Description: "Synthetic schema fixture"},
	}, 0600)
	options := BundleOptions{Root: root, Output: filepath.Join(root, "bundle"), Digest: "sha256:" + strings.Repeat("c", 64),
		KeyFile: keyPath, KeyID: "test-key", VerificationFile: receipt}
	if err := Bundle(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(options.Output, "release-manifest.json"))
	signature, _ := os.ReadFile(filepath.Join(options.Output, "release-manifest.sig"))
	manifest, err := release.Verify(data, signature, publicKey)
	if err != nil || manifest.Version != "0.2.13-r3" || manifest.Migrations[0].Checksum != release.MigrationChecksum("SELECT 1;") {
		t.Fatalf("bundle identity invalid: %v", err)
	}
	if _, err := os.Stat(filepath.Join(options.Output, manifest.UI.Asset)); err != nil {
		t.Fatal(err)
	}
	if err := Bundle(context.Background(), options); err == nil {
		t.Fatal("bundle was overwritten")
	}
	options.Output = filepath.Join(root, "rejected")
	writeJSON(receipt, map[string]any{"source_sha": strings.Repeat("a", 40), "tests_passed": false}, 0600)
	if err := Bundle(context.Background(), options); err == nil {
		t.Fatal("failed test receipt accepted")
	}
	writeJSON(receipt, map[string]any{"source_sha": strings.Repeat("a", 40), "tests_passed": true}, 0600)
	writeJSON(filepath.Join(root, "scripts/release/migration-reviews.json"), map[string]release.Migration{
		"001_init.sql": {Checksum: strings.Repeat("f", 64), Risk: "backward-compatible", Description: "Stale review"},
	}, 0600)
	if err := Bundle(context.Background(), options); err == nil {
		t.Fatal("stale migration review accepted")
	}
}
