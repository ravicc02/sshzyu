package release

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"strings"
	"testing"
)

func validManifest() Manifest {
	return Manifest{
		SchemaVersion: 1, Distribution: "sshzy", Repository: Repository,
		Version: "0.2.13-r2", Tag: "sshzy-v0.2.13-r2", SourceSHA: strings.Repeat("a", 40),
		SigningKeyID: "release-1", PublishedAt: "2026-10-03T00:00:00Z",
		Upstream:      Upstream{Repository: "Wei-Shaw/sub2api", Tag: "v0.2.13", Commit: strings.Repeat("b", 40)},
		Backend:       Backend{Image: Image, Digest: "sha256:" + strings.Repeat("c", 64), OS: "linux", Architecture: "amd64", Version: "0.2.13-r2", Commit: strings.Repeat("a", 40), BuildType: "release"},
		UI:            UI{Asset: "sshzy-ui-0.2.13-r2.tar.gz", Size: 1024, SHA256: strings.Repeat("d", 64), Entry: "index-abc.js", BootstrapSHA256: strings.Repeat("e", 64), Contract: ClientContract},
		Migrations:    []Migration{{Filename: "001_init.sql", Checksum: strings.Repeat("f", 64), Risk: "backward-compatible", Description: "Initial schema"}},
		Compatibility: Compatibility{MinUpdaterProtocol: 1, UIContracts: []string{ClientContract}, RollbackCompatible: true},
		Verification:  Verification{Ready: true, TestsPassed: true},
		Updater:       Artifact{Name: "sshzy-updater-linux-amd64", Size: 1024, SHA256: strings.Repeat("a", 64)},
	}
}

func TestVersionOrdering(t *testing.T) {
	for _, test := range []struct{ current, next string }{
		{"0.2.13-r1", "0.2.13-r2"}, {"0.2.13-r9", "0.2.13-r10"}, {"0.2.13-r99", "0.2.14-r1"},
	} {
		current, err := ParseVersion(test.current)
		if err != nil {
			t.Fatal(err)
		}
		next, err := ParseVersion(test.next)
		if err != nil || current.Compare(next) >= 0 || next.Compare(current) <= 0 || current.Compare(current) != 0 {
			t.Fatalf("invalid ordering: %s %s", test.current, test.next)
		}
	}
	for _, invalid := range []string{"0.2.13", "0.2.13-r0", "0.2.13-r01", "0.2.13-rx", "../0.2.13-r1", "01.2.13-r1", "0.2.13-r18446744073709551616"} {
		if _, err := ParseVersion(invalid); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
}

func TestManifestIdentityAndIntegrity(t *testing.T) {
	if err := validManifest().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Manifest){
		func(manifest *Manifest) { manifest.Repository = "Wei-Shaw/sub2api" },
		func(manifest *Manifest) { manifest.Backend.Image = "weishaw/sub2api" },
		func(manifest *Manifest) { manifest.Backend.Commit = strings.Repeat("b", 40) },
		func(manifest *Manifest) { manifest.Tag = "v0.2.13" },
		func(manifest *Manifest) { manifest.UI.Asset = "../ui.tar.gz" },
		func(manifest *Manifest) { manifest.UI.Entry = "../index.js" },
		func(manifest *Manifest) { manifest.Verification.TestsPassed = false },
		func(manifest *Manifest) { manifest.Migrations = append(manifest.Migrations, manifest.Migrations[0]) },
		func(manifest *Manifest) { manifest.Upstream.Tag = "v0.2.14" },
		func(manifest *Manifest) { manifest.Compatibility.MinUpdaterProtocol = 2 },
	} {
		manifest := validManifest()
		mutate(&manifest)
		if err := manifest.Validate(); err == nil {
			t.Fatal("accepted invalid manifest")
		}
	}
}

func TestDetachedSignatureFailsClosed(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(validManifest())
	signature := Sign(data, privateKey)
	if _, err := Verify(data, signature, publicKey); err != nil {
		t.Fatal(err)
	}
	tampered := append([]byte(nil), data...)
	tampered[len(tampered)-2] ^= 1
	if _, err := Verify(tampered, signature, publicKey); err == nil {
		t.Fatal("accepted tampered manifest")
	}
	if _, err := Verify(data, []byte("invalid"), publicKey); err == nil {
		t.Fatal("accepted invalid signature")
	}
	otherKey, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := Verify(data, signature, otherKey); err == nil {
		t.Fatal("accepted untrusted signer")
	}
	data = append(data, []byte("{}")...)
	if _, err := Verify(data, Sign(data, privateKey), publicKey); err == nil {
		t.Fatal("accepted trailing JSON")
	}
}

func TestMigrationChecksumAndLedger(t *testing.T) {
	if MigrationChecksum(" \r\nSELECT 1;\n\t") != MigrationChecksum("SELECT 1;") {
		t.Fatal("runner trim semantics differ")
	}
	manifest := validManifest()
	pending, err := PendingMigrations(manifest.Migrations, map[string]string{})
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending: %v %v", pending, err)
	}
	pending, err = PendingMigrations(manifest.Migrations, map[string]string{"001_init.sql": strings.Repeat("f", 64)})
	if err != nil || len(pending) != 0 {
		t.Fatal("replayed migration")
	}
	if _, err := PendingMigrations(manifest.Migrations, map[string]string{"001_init.sql": "wrong"}); err == nil {
		t.Fatal("accepted changed historical migration")
	}
	if _, err := PendingMigrations(manifest.Migrations, map[string]string{"002_missing.sql": strings.Repeat("f", 64)}); err == nil {
		t.Fatal("accepted missing historical migration")
	}
}
