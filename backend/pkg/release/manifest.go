package release

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const Repository = "ravicc02/sshzyu"
const Image = "ghcr.io/ravicc02/sshzyu-backend"
const ClientContract = "sshzy-api-1"
const Protocol = 1
const MaxManifestSize = 512 * 1024

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-r([1-9][0-9]*)$`)
var shaPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)
var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var migrationPattern = regexp.MustCompile(`^[0-9]{3}[a-z]?_[a-z0-9_]+\.sql$`)
var entryPattern = regexp.MustCompile(`^index-[A-Za-z0-9_-]+\.js$`)

type Version struct {
	Major, Minor, Patch, Revision uint64
}

func ParseVersion(value string) (Version, error) {
	parts := versionPattern.FindStringSubmatch(value)
	if parts == nil {
		return Version{}, errors.New("invalid custom version")
	}
	numbers := make([]uint64, 4)
	for index, part := range parts[1:] {
		number, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return Version{}, errors.New("version component overflow")
		}
		numbers[index] = number
	}
	return Version{numbers[0], numbers[1], numbers[2], numbers[3]}, nil
}

func (version Version) Compare(other Version) int {
	left := []uint64{version.Major, version.Minor, version.Patch, version.Revision}
	right := []uint64{other.Major, other.Minor, other.Patch, other.Revision}
	for index, number := range left {
		if number < right[index] {
			return -1
		}
		if number > right[index] {
			return 1
		}
	}
	return 0
}

func (version Version) OfficialTag() string {
	return fmt.Sprintf("v%d.%d.%d", version.Major, version.Minor, version.Patch)
}

type Upstream struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Commit     string `json:"commit"`
}

type Backend struct {
	Image        string `json:"image"`
	Digest       string `json:"digest"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Version      string `json:"version"`
	Commit       string `json:"commit"`
	BuildType    string `json:"build_type"`
}

func (backend Backend) Reference() string {
	return backend.Image + "@" + backend.Digest
}

type UI struct {
	Asset           string `json:"asset"`
	Size            int64  `json:"size"`
	SHA256          string `json:"sha256"`
	Entry           string `json:"entry"`
	BootstrapSHA256 string `json:"bootstrap_sha256"`
	Contract        string `json:"contract"`
}

type Migration struct {
	Filename         string `json:"filename"`
	Checksum         string `json:"checksum"`
	Risk             string `json:"risk"`
	Description      string `json:"description"`
	NonTransactional bool   `json:"non_transactional"`
}

type Compatibility struct {
	MinUpdaterProtocol int      `json:"min_updater_protocol"`
	UIContracts        []string `json:"ui_contracts"`
	RollbackCompatible bool     `json:"rollback_compatible"`
}

type Verification struct {
	Ready       bool `json:"ready"`
	TestsPassed bool `json:"tests_passed"`
}

type Artifact struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	SchemaVersion int           `json:"schema_version"`
	Distribution  string        `json:"distribution"`
	Repository    string        `json:"repository"`
	Version       string        `json:"version"`
	Tag           string        `json:"tag"`
	SourceSHA     string        `json:"source_sha"`
	SigningKeyID  string        `json:"signing_key_id"`
	PublishedAt   string        `json:"published_at"`
	Upstream      Upstream      `json:"upstream"`
	Backend       Backend       `json:"backend"`
	UI            UI            `json:"ui"`
	Migrations    []Migration   `json:"migrations"`
	Compatibility Compatibility `json:"compatibility"`
	Verification  Verification  `json:"verification"`
	Updater       Artifact      `json:"updater"`
}

func (manifest Manifest) Validate() error {
	version, err := ParseVersion(manifest.Version)
	if err != nil {
		return err
	}
	if manifest.SchemaVersion != 1 || manifest.Distribution != "sshzy" || manifest.Repository != Repository ||
		manifest.Tag != "sshzy-v"+manifest.Version || !shaPattern.MatchString(manifest.SourceSHA) ||
		!regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`).MatchString(manifest.SigningKeyID) {
		return errors.New("invalid release identity")
	}
	if _, err := time.Parse(time.RFC3339, manifest.PublishedAt); err != nil {
		return errors.New("invalid publication time")
	}
	if manifest.Upstream.Repository != "Wei-Shaw/sub2api" || manifest.Upstream.Tag != version.OfficialTag() || !shaPattern.MatchString(manifest.Upstream.Commit) {
		return errors.New("invalid upstream baseline")
	}
	backend := manifest.Backend
	if backend.Image != Image || !strings.HasPrefix(backend.Digest, "sha256:") ||
		!hashPattern.MatchString(strings.TrimPrefix(backend.Digest, "sha256:")) ||
		backend.OS != "linux" || backend.Architecture != "amd64" ||
		backend.Version != manifest.Version || backend.Commit != manifest.SourceSHA || backend.BuildType != "release" {
		return errors.New("invalid backend identity")
	}
	ui := manifest.UI
	if ui.Asset != "sshzy-ui-"+manifest.Version+".tar.gz" || ui.Size < 1 || ui.Size > 64*1024*1024 ||
		!hashPattern.MatchString(ui.SHA256) || !hashPattern.MatchString(ui.BootstrapSHA256) ||
		!entryPattern.MatchString(ui.Entry) || ui.Contract != ClientContract {
		return errors.New("invalid UI identity")
	}
	compatibility := manifest.Compatibility
	if compatibility.MinUpdaterProtocol != Protocol || len(compatibility.UIContracts) != 1 || compatibility.UIContracts[0] != ClientContract {
		return errors.New("unsupported release compatibility")
	}
	if !manifest.Verification.Ready || !manifest.Verification.TestsPassed {
		return errors.New("release verification not passed")
	}
	if manifest.Updater.Name != "sshzy-updater-linux-amd64" || manifest.Updater.Size < 1 ||
		manifest.Updater.Size > 32*1024*1024 || !hashPattern.MatchString(manifest.Updater.SHA256) {
		return errors.New("invalid updater artifact")
	}
	previous := ""
	for _, migration := range manifest.Migrations {
		if !migrationPattern.MatchString(migration.Filename) || migration.Filename <= previous ||
			!hashPattern.MatchString(migration.Checksum) ||
			(migration.Risk != "backward-compatible" && migration.Risk != "manual") ||
			strings.TrimSpace(migration.Description) == "" {
			return errors.New("invalid migration ledger")
		}
		previous = migration.Filename
	}
	if len(manifest.Migrations) == 0 {
		return errors.New("migration ledger is empty")
	}
	return nil
}

func Sign(data []byte, privateKey ed25519.PrivateKey) []byte {
	return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, data)) + "\n")
}

func Verify(data, signature []byte, publicKey ed25519.PublicKey) (*Manifest, error) {
	if len(data) == 0 || len(data) > MaxManifestSize || len(signature) > 256 || len(publicKey) != ed25519.PublicKeySize {
		return nil, errors.New("invalid manifest size or trust key")
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(signature)))
	if err != nil || !ed25519.Verify(publicKey, data, decoded) {
		return nil, errors.New("release signature verification failed")
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, errors.New("invalid manifest JSON")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("trailing manifest data")
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func MigrationChecksum(content string) string {
	return Hash([]byte(strings.TrimSpace(content)))
}

func PendingMigrations(migrations []Migration, installed map[string]string) ([]Migration, error) {
	known := make(map[string]string, len(migrations))
	pending := make([]Migration, 0)
	for _, migration := range migrations {
		known[migration.Filename] = migration.Checksum
		if checksum, exists := installed[migration.Filename]; exists {
			if checksum != migration.Checksum {
				return nil, fmt.Errorf("historical migration changed: %s", migration.Filename)
			}
		} else {
			pending = append(pending, migration)
		}
	}
	for filename := range installed {
		if _, exists := known[filename]; !exists {
			return nil, fmt.Errorf("historical migration missing: %s", filename)
		}
	}
	return pending, nil
}
