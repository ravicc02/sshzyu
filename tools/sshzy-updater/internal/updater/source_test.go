package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/release"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func signedFixture(t *testing.T) (release.Manifest, []byte, []byte, ed25519.PublicKey) {
	t.Helper()
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	manifest := release.Manifest{
		SchemaVersion: 1, Distribution: "sshzy", Repository: release.Repository, Version: "0.2.13-r3",
		Tag: "sshzy-v0.2.13-r3", SourceSHA: strings.Repeat("a", 40), SigningKeyID: "test-key", PublishedAt: "2026-10-03T00:00:00Z",
		Upstream:      release.Upstream{Repository: "Wei-Shaw/sub2api", Tag: "v0.2.13", Commit: strings.Repeat("b", 40)},
		Backend:       release.Backend{Image: release.Image, Digest: "sha256:" + strings.Repeat("c", 64), OS: "linux", Architecture: "amd64", Version: "0.2.13-r3", Commit: strings.Repeat("a", 40), BuildType: "release"},
		UI:            release.UI{Asset: "sshzy-ui-0.2.13-r3.tar.gz", Size: 4, SHA256: release.Hash([]byte("test")), Entry: "index-test.js", BootstrapSHA256: strings.Repeat("d", 64), Contract: release.ClientContract},
		Migrations:    []release.Migration{{Filename: "001_init.sql", Checksum: strings.Repeat("e", 64), Risk: "backward-compatible", Description: "Fixture migration"}},
		Compatibility: release.Compatibility{MinUpdaterProtocol: 1, UIContracts: []string{release.ClientContract}, RollbackCompatible: true},
		Verification:  release.Verification{Ready: true, TestsPassed: true},
		Updater:       release.Artifact{Name: "sshzy-updater-linux-amd64", Size: 4, SHA256: release.Hash([]byte("test"))},
	}
	data, _ := json.Marshal(manifest)
	return manifest, data, release.Sign(data, privateKey), publicKey
}

func TestPrivateSourceUsesAssetAPIAndPinnedSignature(t *testing.T) {
	manifest, data, signature, publicKey := signedFixture(t)
	metadata := githubRelease{ID: 7, Tag: manifest.Tag, Assets: []githubAsset{
		{ID: 1, Name: "release-manifest.json"}, {ID: 2, Name: "release-manifest.sig"},
		{ID: 3, Name: manifest.UI.Asset, Size: 4}, {ID: 4, Name: manifest.Updater.Name, Size: 4},
	}}
	metadataBytes, _ := json.Marshal(metadata)
	source := &GitHubSource{publicKey: publicKey, keyID: "test-key", token: "unit-test-token"}
	source.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "api.github.com" || request.Header.Get("Authorization") != "Bearer unit-test-token" {
			t.Fatal("private asset authentication or source changed")
		}
		var body []byte
		switch {
		case strings.HasSuffix(request.URL.Path, "/assets/1"):
			body = data
		case strings.HasSuffix(request.URL.Path, "/assets/2"):
			body = signature
		case strings.HasSuffix(request.URL.Path, "/assets/3"):
			body = []byte("test")
		case strings.Contains(request.URL.Path, "/tags/"):
			body = metadataBytes
		default:
			body, _ = json.Marshal([]githubRelease{metadata})
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}, Request: request}, nil
	})}
	target, err := source.Get(context.Background(), manifest.Version, release.Hash(data))
	if err != nil || target.Hash != release.Hash(data) {
		t.Fatalf("get failed: %v", err)
	}
	if _, err := source.Get(context.Background(), manifest.Version, strings.Repeat("f", 64)); err == nil {
		t.Fatal("target drift accepted")
	}
	targets, err := source.List(context.Background())
	if err != nil || len(targets) != 1 {
		t.Fatal(err)
	}
	if err := source.DownloadUI(context.Background(), target, filepath.Join(t.TempDir(), "ui.tar.gz")); err != nil {
		t.Fatal(err)
	}
	signature[0] ^= 1
	if _, err := source.Get(context.Background(), manifest.Version, ""); err == nil {
		t.Fatal("bad signature accepted")
	}
}

func TestCredentialAndPublicKeyErrorsDoNotLeakContents(t *testing.T) {
	root := t.TempDir()
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	data, _ := x509.MarshalPKIXPublicKey(publicKey)
	path := filepath.Join(root, "public.pem")
	os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: data}), 0600)
	if _, err := ReadPublicKey(path); err != nil {
		t.Fatal(err)
	}
	privateData, _ := x509.MarshalPKCS8PrivateKey(privateKey)
	privatePath := filepath.Join(root, "synthetic-test-private.pem")
	os.WriteFile(privatePath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateData}), 0600)
	if _, err := ReadPrivateKey(privatePath); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPrivateKey(path); err == nil {
		t.Fatal("public key accepted as signing key")
	}
	tokenPath := filepath.Join(root, "synthetic-test-token")
	os.WriteFile(tokenPath, []byte("invalid\nmultiline"), 0600)
	if _, err := readToken(tokenPath); err == nil {
		t.Fatal("invalid control token accepted")
	}
	source, err := NewGitHubSource(Config{PublicKeyFile: path, SigningKeyID: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest("GET", "https://evil.example/asset", nil)
	request.Header.Set("Authorization", "Bearer unit-test-token")
	if source.client.CheckRedirect(request, nil) == nil {
		t.Fatal("untrusted redirect accepted")
	}
	request.URL.Host = "release-assets.githubusercontent.com"
	if source.client.CheckRedirect(request, nil) != nil || request.Header.Get("Authorization") != "" {
		t.Fatal("CDN redirect leaked authorization")
	}
}
