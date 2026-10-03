package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/release"
)

type GitHubSource struct {
	client    *http.Client
	token     string
	publicKey ed25519.PublicKey
	keyID     string
}

type githubAsset struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type githubRelease struct {
	ID         int64         `json:"id"`
	Tag        string        `json:"tag_name"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Assets     []githubAsset `json:"assets"`
}

func readToken(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 8192 {
		return "", errors.New("credential file unavailable")
	}
	token := strings.TrimSpace(string(data))
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return "", errors.New("credential file invalid")
	}
	return token, nil
}

func ReadPublicKey(path string) (ed25519.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(data)
	if block == nil || strings.TrimSpace(string(rest)) != "" {
		return nil, errors.New("invalid trust key PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("trust key must be Ed25519")
	}
	return key, nil
}

func ReadPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(data)
	if block == nil || strings.TrimSpace(string(rest)) != "" {
		return nil, errors.New("invalid signing key PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("signing key must be Ed25519")
	}
	return key, nil
}

func NewGitHubSource(config Config) (*GitHubSource, error) {
	key, err := ReadPublicKey(config.PublicKeyFile)
	if err != nil {
		return nil, err
	}
	token, err := readToken(config.GitHubTokenFile)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	client.CheckRedirect = func(request *http.Request, previous []*http.Request) error {
		host := request.URL.Host
		if len(previous) > 5 || request.URL.Scheme != "https" || request.URL.User != nil ||
			(host != "api.github.com" && host != "github.com" && host != "objects.githubusercontent.com" && host != "release-assets.githubusercontent.com") {
			return errors.New("asset redirect rejected")
		}
		if host != "api.github.com" {
			request.Header.Del("Authorization")
		}
		return nil
	}
	return &GitHubSource{client: client, token: token, publicKey: key, keyID: config.SigningKeyID}, nil
}

func (source *GitHubSource) request(ctx context.Context, suffix, accept string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+release.Repository+suffix, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", accept)
	request.Header.Set("User-Agent", "sshzy-updater")
	if source.token != "" {
		request.Header.Set("Authorization", "Bearer "+source.token)
	}
	response, err := source.client.Do(request)
	if err != nil {
		return nil, CodeError("RELEASE_SOURCE_UNAVAILABLE")
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, CodeError("RELEASE_SOURCE_UNAVAILABLE")
	}
	return response, nil
}

func (source *GitHubSource) bytes(ctx context.Context, suffix, accept string, limit int64) ([]byte, error) {
	response, err := source.request(ctx, suffix, accept)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.ContentLength > limit {
		return nil, CodeError("RELEASE_ASSET_TOO_LARGE")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, CodeError("RELEASE_ASSET_INVALID")
	}
	return data, nil
}

func (source *GitHubSource) verify(ctx context.Context, metadata githubRelease) (Release, error) {
	if metadata.ID <= 0 || metadata.Draft || metadata.Prerelease {
		return Release{}, CodeError("RELEASE_NOT_READY")
	}
	assets := map[string]githubAsset{}
	for _, asset := range metadata.Assets {
		if _, exists := assets[asset.Name]; exists || asset.ID <= 0 {
			return Release{}, CodeError("RELEASE_ASSET_INVALID")
		}
		assets[asset.Name] = asset
	}
	manifestAsset, manifestExists := assets["release-manifest.json"]
	signatureAsset, signatureExists := assets["release-manifest.sig"]
	if !manifestExists || !signatureExists {
		return Release{}, CodeError("RELEASE_NOT_READY")
	}
	data, err := source.bytes(ctx, fmt.Sprintf("/releases/assets/%d", manifestAsset.ID), "application/octet-stream", release.MaxManifestSize)
	if err != nil {
		return Release{}, err
	}
	signature, err := source.bytes(ctx, fmt.Sprintf("/releases/assets/%d", signatureAsset.ID), "application/octet-stream", 256)
	if err != nil {
		return Release{}, err
	}
	manifest, err := release.Verify(data, signature, source.publicKey)
	if err != nil || manifest.SigningKeyID != source.keyID || manifest.Tag != metadata.Tag {
		return Release{}, CodeError("RELEASE_SIGNATURE_INVALID")
	}
	uiAsset, exists := assets[manifest.UI.Asset]
	if !exists || uiAsset.Size != manifest.UI.Size {
		return Release{}, CodeError("RELEASE_UI_MISSING")
	}
	updaterAsset, exists := assets[manifest.Updater.Name]
	if !exists || updaterAsset.Size != manifest.Updater.Size {
		return Release{}, CodeError("RELEASE_UPDATER_MISSING")
	}
	return Release{Hash: release.Hash(data), Manifest: *manifest, ReleaseID: metadata.ID, UIAssetID: uiAsset.ID}, nil
}

func (source *GitHubSource) Get(ctx context.Context, version, hash string) (Release, error) {
	if _, err := release.ParseVersion(version); err != nil {
		return Release{}, err
	}
	data, err := source.bytes(ctx, "/releases/tags/sshzy-v"+version, "application/vnd.github+json", 2*1024*1024)
	if err != nil {
		return Release{}, err
	}
	var metadata githubRelease
	if json.Unmarshal(data, &metadata) != nil || metadata.Tag != "sshzy-v"+version {
		return Release{}, CodeError("RELEASE_IDENTITY_INVALID")
	}
	target, err := source.verify(ctx, metadata)
	if err != nil {
		return Release{}, err
	}
	if hash != "" && target.Hash != hash {
		return Release{}, CodeError("RELEASE_TARGET_CHANGED")
	}
	return target, nil
}

func (source *GitHubSource) List(ctx context.Context) ([]Release, error) {
	candidates := []githubRelease{}
	for page := 1; page <= 20; page++ {
		data, err := source.bytes(ctx, fmt.Sprintf("/releases?per_page=100&page=%d", page), "application/vnd.github+json", 8*1024*1024)
		if err != nil {
			return nil, err
		}
		var entries []githubRelease
		if json.Unmarshal(data, &entries) != nil {
			return nil, CodeError("RELEASE_SOURCE_INVALID")
		}
		for _, entry := range entries {
			if entry.Draft || entry.Prerelease || !strings.HasPrefix(entry.Tag, "sshzy-v") {
				continue
			}
			if _, err := release.ParseVersion(strings.TrimPrefix(entry.Tag, "sshzy-v")); err != nil {
				return nil, CodeError("RELEASE_SOURCE_INVALID")
			}
			candidates = append(candidates, entry)
		}
		if len(entries) < 100 {
			break
		}
		if page == 20 {
			return nil, CodeError("RELEASE_PAGINATION_LIMIT")
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		first, _ := release.ParseVersion(strings.TrimPrefix(candidates[left].Tag, "sshzy-v"))
		second, _ := release.ParseVersion(strings.TrimPrefix(candidates[right].Tag, "sshzy-v"))
		return first.Compare(second) > 0
	})
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}
	targets := make([]Release, 0, len(candidates))
	for _, entry := range candidates {
		target, err := source.verify(ctx, entry)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func (source *GitHubSource) DownloadUI(ctx context.Context, target Release, destination string) error {
	data, err := source.bytes(ctx, fmt.Sprintf("/releases/assets/%d", target.UIAssetID), "application/octet-stream", target.Manifest.UI.Size)
	if err != nil {
		return err
	}
	if int64(len(data)) != target.Manifest.UI.Size || release.Hash(data) != target.Manifest.UI.SHA256 {
		return CodeError("UI_CHECKSUM_MISMATCH")
	}
	return atomicWrite(destination, data, 0600)
}
