package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/release"
)

type BundleOptions struct {
	Root             string
	Output           string
	Digest           string
	KeyFile          string
	KeyID            string
	VerificationFile string
	PreviousManifest string
}

func Bundle(ctx context.Context, options BundleOptions) error {
	shaBytes, err := command(ctx, "git", "-C", options.Root, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	sha := strings.TrimSpace(string(shaBytes))
	dirty, err := command(ctx, "git", "-C", options.Root, "status", "--porcelain", "--untracked-files=normal", "--",
		"backend", "frontend", "scripts", "tools", ".github", "customizations.json", "upstream-baseline.json")
	if err != nil || strings.TrimSpace(string(dirty)) != "" {
		return errors.New("release input is not a clean committed checkout")
	}
	var baseline struct {
		Repository string `json:"upstream_repository"`
		Tag        string `json:"upstream_release"`
		Commit     string `json:"upstream_commit"`
		Version    string `json:"local_build_version"`
	}
	if readJSON(filepath.Join(options.Root, "upstream-baseline.json"), &baseline) != nil {
		return errors.New("invalid baseline")
	}
	versionBytes, err := os.ReadFile(filepath.Join(options.Root, "backend/cmd/server/VERSION"))
	if err != nil || strings.TrimSpace(string(versionBytes)) != baseline.Version {
		return errors.New("version does not match baseline")
	}
	var receipt struct {
		SourceSHA   string `json:"source_sha"`
		TestsPassed bool   `json:"tests_passed"`
	}
	if readJSON(options.VerificationFile, &receipt) != nil || receipt.SourceSHA != sha || !receipt.TestsPassed {
		return errors.New("successful verification receipt required")
	}
	dist := filepath.Join(options.Root, "backend/internal/web/dist")
	var info struct {
		Version   string `json:"version"`
		Commit    string `json:"commit"`
		Contract  string `json:"contract"`
		BuildType string `json:"build_type"`
	}
	if readJSON(filepath.Join(dist, "build-info.json"), &info) != nil || info.Version != baseline.Version ||
		info.Commit != sha || info.Contract != release.ClientContract || info.BuildType != "release" {
		return errors.New("UI was not built for this release")
	}
	if err := os.MkdirAll(options.Output, 0700); err != nil {
		return err
	}
	asset := "sshzy-ui-" + baseline.Version + ".tar.gz"
	archive := filepath.Join(options.Output, asset)
	if err := packUI(dist, archive); err != nil {
		return err
	}
	stat, err := os.Stat(archive)
	if err != nil {
		return err
	}
	hash, err := fileHash(archive)
	if err != nil {
		return err
	}
	bootstrap, err := os.ReadFile(filepath.Join(dist, "assets/fw-cachebust.js"))
	if err != nil {
		return err
	}
	match := regexp.MustCompile(`/assets/(index-[A-Za-z0-9_-]+\.js)`).FindSubmatch(bootstrap)
	if len(match) != 2 {
		return errors.New("UI bootstrap entry missing")
	}
	var reviews map[string]release.Migration
	if readJSON(filepath.Join(options.Root, "scripts/release/migration-reviews.json"), &reviews) != nil {
		return errors.New("migration reviews unavailable")
	}
	files, err := filepath.Glob(filepath.Join(options.Root, "backend/migrations", "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	migrations := make([]release.Migration, 0, len(files))
	for _, filename := range files {
		data, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		name := filepath.Base(filename)
		migration := release.Migration{Filename: name, Checksum: release.MigrationChecksum(string(data)),
			Risk: "manual", Description: "Historical migration; new applications require explicit review", NonTransactional: strings.HasSuffix(name, "_notx.sql")}
		if review, exists := reviews[name]; exists {
			if review.Checksum != migration.Checksum || review.Description == "" ||
				(review.Risk != "manual" && review.Risk != "backward-compatible") {
				return errors.New("migration review does not match the committed SQL")
			}
			migration.Risk, migration.Description = review.Risk, review.Description
		}
		migrations = append(migrations, migration)
	}
	if options.PreviousManifest != "" {
		var previous release.Manifest
		if readJSON(options.PreviousManifest, &previous) != nil || previous.Validate() != nil {
			return errors.New("invalid previous manifest")
		}
		current := map[string]string{}
		for _, migration := range migrations {
			current[migration.Filename] = migration.Checksum
		}
		for _, migration := range previous.Migrations {
			if current[migration.Filename] != migration.Checksum {
				return errors.New("historical migration changed or removed")
			}
		}
	}
	manifest := release.Manifest{SchemaVersion: 1, Distribution: "sshzy", Repository: release.Repository,
		Version: baseline.Version, Tag: "sshzy-v" + baseline.Version, SourceSHA: sha, SigningKeyID: options.KeyID,
		PublishedAt: time.Now().UTC().Format(time.RFC3339),
		Upstream:    release.Upstream{Repository: baseline.Repository, Tag: baseline.Tag, Commit: baseline.Commit},
		Backend: release.Backend{Image: release.Image, Digest: options.Digest, OS: "linux", Architecture: "amd64",
			Version: baseline.Version, Commit: sha, BuildType: "release"},
		UI:         release.UI{Asset: asset, Size: stat.Size(), SHA256: hash, Entry: string(match[1]), BootstrapSHA256: release.Hash(bootstrap), Contract: release.ClientContract},
		Migrations: migrations, Compatibility: release.Compatibility{MinUpdaterProtocol: release.Protocol,
			UIContracts: []string{release.ClientContract}, RollbackCompatible: true},
		Verification: release.Verification{Ready: true, TestsPassed: true}}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	updaterHash, err := fileHash(executable)
	if err != nil {
		return err
	}
	updaterInfo, err := os.Stat(executable)
	if err != nil {
		return err
	}
	manifest.Updater = release.Artifact{Name: "sshzy-updater-linux-amd64", Size: updaterInfo.Size(), SHA256: updaterHash}
	if err := manifest.Validate(); err != nil {
		return err
	}
	key, err := ReadPrivateKey(options.KeyFile)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := atomicWrite(filepath.Join(options.Output, "release-manifest.json"), data, 0644); err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(options.Output, "release-manifest.sig"), release.Sign(data, key), 0644); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(options.Output, "migration-report.json"), migrations, 0644); err != nil {
		return err
	}
	return atomicWrite(filepath.Join(options.Output, "checksums.txt"), []byte(hash+"  "+asset+"\n"+release.Hash(data)+"  release-manifest.json\n"+updaterHash+"  sshzy-updater-linux-amd64\n"), 0644)
}

func packUI(source, destination string) error {
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed := gzip.NewWriter(file)
	writer := tar.NewWriter(compressed)
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("UI symlinks are not allowed")
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(filepath.Base(relative), ".") || strings.HasSuffix(relative, ".map") {
			return errors.New("private UI artifacts are not allowed")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name, header.Mode = filepath.ToSlash(relative), 0644
		header.ModTime = time.Unix(0, 0)
		if err := writer.WriteHeader(header); err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, input)
		closeErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		writer.Close()
		compressed.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return compressed.Close()
}
