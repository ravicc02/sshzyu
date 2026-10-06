package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/release"
	"gopkg.in/yaml.v3"
)

type Installed struct {
	ManifestHash string           `json:"manifest_hash"`
	Manifest     release.Manifest `json:"manifest"`
	InstalledAt  time.Time        `json:"installed_at"`
}

type HostDriver struct {
	config         Config
	source         *GitHubSource
	controlToken   string
	runCommand     func(context.Context, string, ...string) ([]byte, error)
	runtimeProbe   func(context.Context) (map[string]any, error)
	backupOverride func(context.Context, Operation) (string, error)
	renameOverride func(string, string) error
}

func (driver *HostDriver) run(ctx context.Context, name string, arguments ...string) ([]byte, error) {
	if driver.runCommand != nil {
		return driver.runCommand(ctx, name, arguments...)
	}
	return command(ctx, name, arguments...)
}

// rename 默认使用 os.Rename（同一挂载点内为原子操作）；测试可注入 renameOverride。
func (driver *HostDriver) rename(source, target string) error {
	if driver.renameOverride != nil {
		return driver.renameOverride(source, target)
	}
	return os.Rename(source, target)
}

// materializeUI 把 prepared 的 UI 产物落到目标 release 目录。
// 源（state/prepared/<id>/ui）与目标（ui_root/releases/<version>）可能位于不同的
// vfsmount——例如 /opt/sub2api-deploy 与 /opt/sshzyu-ui 是两个独立 bind mount，
// 此时 rename(2) 返回 EXDEV，即便 stat 的设备号相同。因此 rename 失败且属跨设备
// 错误时，回退到复制 + 删除源；其它错误照常上抛。
func (driver *HostDriver) materializeUI(ctx context.Context, source, target string) error {
	err := driver.rename(source, target)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errCrossDevice) {
		return err
	}
	if _, copyErr := driver.run(ctx, "cp", "-a", source, target); copyErr != nil {
		return err
	}
	if _, removeErr := driver.run(ctx, "rm", "-rf", source); removeErr != nil {
		return removeErr
	}
	return nil
}

func NewHostDriver(config Config, source *GitHubSource) (*HostDriver, error) {
	hostname, err := os.Hostname()
	if err != nil || config.ExpectedHostname == "" || hostname != config.ExpectedHostname {
		return nil, errors.New("production host identity mismatch")
	}
	for _, path := range []string{config.StateDir, config.DeploymentDir, config.UIRoot, config.ControlDir, config.DockerConfig} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return nil, errors.New("updater paths must be canonical absolute paths")
		}
	}
	if config.DeploymentDir != "/opt/sub2api-deploy" || config.UIRoot != "/opt/sshzyu-ui" ||
		!strings.HasPrefix(config.StateDir, config.DeploymentDir+"/updater/") || config.ControlDir != "/run/sshzy-updater" {
		return nil, errors.New("unexpected production deployment paths")
	}
	token, err := readToken(config.ControlTokenFile)
	if err != nil || token == "" {
		return nil, errors.New("control credential unavailable")
	}
	return &HostDriver{config: config, source: source, controlToken: token}, nil
}

func command(ctx context.Context, name string, arguments ...string) ([]byte, error) {
	process := exec.CommandContext(ctx, name, arguments...)
	var output bytes.Buffer
	process.Stdout = &output
	process.Stderr = io.Discard
	if err := process.Run(); err != nil {
		return nil, CodeError("HOST_COMMAND_FAILED")
	}
	return output.Bytes(), nil
}

func fileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 32*1024*1024+1))
	if err != nil || len(data) > 32*1024*1024 {
		return "", CodeError("DEPLOYMENT_FILE_INVALID")
	}
	return release.Hash(data), nil
}

func readJSON(path string, destination any) error {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 8*1024*1024 {
		return CodeError("DEPLOYMENT_RECORD_UNAVAILABLE")
	}
	if json.Unmarshal(data, destination) != nil {
		return CodeError("DEPLOYMENT_RECORD_INVALID")
	}
	return nil
}

func writeJSON(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, data, mode)
}

func (driver *HostDriver) docker(ctx context.Context, arguments ...string) ([]byte, error) {
	return driver.run(ctx, "docker", append([]string{"--config", driver.config.DockerConfig}, arguments...)...)
}

func (driver *HostDriver) ledger(ctx context.Context) (map[string]string, error) {
	output, err := driver.docker(ctx, "exec", "sub2api-postgres", "sh", "-c", `exec psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -c 'SELECT filename,checksum FROM schema_migrations ORDER BY filename'`)
	if err != nil {
		return nil, err
	}
	ledger := map[string]string{}
	for _, row := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		columns := strings.Split(row, "|")
		if len(columns) != 2 || !manifestHashPattern.MatchString(columns[1]) {
			return nil, CodeError("MIGRATION_LEDGER_INVALID")
		}
		if _, exists := ledger[columns[0]]; exists {
			return nil, CodeError("MIGRATION_LEDGER_INVALID")
		}
		ledger[columns[0]] = columns[1]
	}
	return ledger, nil
}

func (driver *HostDriver) snapshot(ctx context.Context) (Snapshot, error) {
	return driver.snapshotFor(ctx, nil)
}

func (driver *HostDriver) snapshotFor(ctx context.Context, supplied *Installed) (Snapshot, error) {
	var installed Installed
	if supplied != nil {
		installed = *supplied
	} else if err := readJSON(filepath.Join(driver.config.StateDir, "installed.json"), &installed); err != nil {
		return Snapshot{}, CodeError("BOOTSTRAP_REQUIRED")
	}
	if installed.Manifest.Validate() != nil || !manifestHashPattern.MatchString(installed.ManifestHash) {
		return Snapshot{}, CodeError("BOOTSTRAP_IDENTITY_INVALID")
	}
	output, err := driver.docker(ctx, "inspect", "sub2api")
	if err != nil {
		return Snapshot{}, err
	}
	var containers []struct {
		ID     string
		Image  string
		Config struct {
			Image  string
			Labels map[string]string
		}
		State struct{ Running bool }
	}
	if json.Unmarshal(output, &containers) != nil || len(containers) != 1 || !containers[0].State.Running {
		return Snapshot{}, CodeError("BACKEND_NOT_RUNNING")
	}
	container := containers[0]
	if container.Config.Image != installed.Manifest.Backend.Reference() ||
		container.Config.Labels["org.opencontainers.image.revision"] != installed.Manifest.SourceSHA ||
		container.Config.Labels["org.opencontainers.image.version"] != installed.Manifest.Version {
		return Snapshot{}, CodeError("INSTALLED_IDENTITY_MISMATCH")
	}
	current, err := filepath.EvalSymlinks(filepath.Join(driver.config.UIRoot, "current"))
	if err != nil || current != filepath.Join(driver.config.UIRoot, "releases", installed.Manifest.Version) {
		return Snapshot{}, CodeError("UI_IDENTITY_MISMATCH")
	}
	var info struct {
		Version  string `json:"version"`
		Commit   string `json:"commit"`
		Contract string `json:"contract"`
	}
	if readJSON(filepath.Join(current, "build-info.json"), &info) != nil || info.Version != installed.Manifest.Version ||
		info.Commit != installed.Manifest.SourceSHA || info.Contract != release.ClientContract {
		return Snapshot{}, CodeError("UI_IDENTITY_MISMATCH")
	}
	composeHash, err := fileHash(filepath.Join(driver.config.DeploymentDir, "docker-compose.yml"))
	if err != nil {
		return Snapshot{}, err
	}
	environmentHash, err := fileHash(filepath.Join(driver.config.DeploymentDir, ".env"))
	if err != nil {
		return Snapshot{}, err
	}
	bootstrapHash, err := fileHash(filepath.Join(driver.config.UIRoot, "shared/assets/fw-cachebust.js"))
	if err != nil || bootstrapHash != installed.Manifest.UI.BootstrapSHA256 {
		return Snapshot{}, CodeError("BOOTSTRAP_IDENTITY_MISMATCH")
	}
	ledger, err := driver.ledger(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Version: info.Version, Contract: info.Contract, ManifestHash: installed.ManifestHash,
		Image: container.Config.Image, ImageID: container.Image, ContainerID: container.ID,
		ComposeHash: composeHash, EnvironmentHash: environmentHash, UICurrent: current,
		BootstrapHash: bootstrapHash, Ledger: ledger}, nil
}

func replaceBackendImage(data []byte, image string) ([]byte, error) {
	var document yaml.Node
	if yaml.Unmarshal(data, &document) != nil || len(document.Content) != 1 {
		return nil, CodeError("COMPOSE_INVALID")
	}
	find := func(node *yaml.Node, key string) *yaml.Node {
		for index := 0; index+1 < len(node.Content); index += 2 {
			if node.Content[index].Value == key {
				return node.Content[index+1]
			}
		}
		return nil
	}
	services := find(document.Content[0], "services")
	if services == nil {
		return nil, CodeError("COMPOSE_INVALID")
	}
	backend := find(services, "sub2api")
	if backend == nil {
		return nil, CodeError("COMPOSE_INVALID")
	}
	target := find(backend, "image")
	if target == nil || target.Kind != yaml.ScalarNode {
		return nil, CodeError("COMPOSE_INVALID")
	}
	return replaceScalarValue(data, target.Line, target.Column, target.Value, image)
}

// replaceScalarValue replaces the scalar beginning at the 1-based line/column with
// newValue, editing the original bytes so the rest of the document keeps its exact
// formatting (multi-line block scalars, comments, quoting). Re-serializing the whole
// YAML tree instead would normalize that formatting and trip the activation guard,
// which renders the candidate and the active file and compares them for equality.
func replaceScalarValue(data []byte, line, column int, oldValue, newValue string) ([]byte, error) {
	if line < 1 || column < 1 {
		return nil, CodeError("COMPOSE_INVALID")
	}
	offset, currentLine := 0, 1
	for offset < len(data) && currentLine < line {
		if data[offset] == '\n' {
			currentLine++
		}
		offset++
	}
	if currentLine != line {
		return nil, CodeError("COMPOSE_INVALID")
	}
	start := offset + column - 1
	if start < 0 || start > len(data) {
		return nil, CodeError("COMPOSE_INVALID")
	}
	if start < len(data) && (data[start] == '"' || data[start] == '\'') {
		quote := data[start]
		end := start + 1
		for end < len(data) && data[end] != '\n' && data[end] != quote {
			end++
		}
		if end >= len(data) || data[end] != quote {
			return nil, CodeError("COMPOSE_INVALID")
		}
		body := data[start+1 : end]
		if quote == '"' {
			body = bytes.ReplaceAll(body, []byte(`\"`), []byte{'"'})
		} else {
			body = bytes.ReplaceAll(body, []byte("''"), []byte{'\''})
		}
		if string(body) != oldValue {
			return nil, CodeError("COMPOSE_INVALID")
		}
		replaced := make([]byte, 0, len(data)-len(body)+len(newValue))
		replaced = append(replaced, data[:start+1]...)
		replaced = append(replaced, newValue...)
		replaced = append(replaced, data[end:]...)
		return replaced, nil
	}
	if end := start + len(oldValue); end <= len(data) && string(data[start:end]) == oldValue {
		replaced := make([]byte, 0, len(data)-len(oldValue)+len(newValue))
		replaced = append(replaced, data[:start]...)
		replaced = append(replaced, newValue...)
		replaced = append(replaced, data[end:]...)
		return replaced, nil
	}
	end := start
	for end < len(data) && data[end] != '\n' {
		end++
	}
	index := bytes.Index(data[start:end], []byte(oldValue))
	if index < 0 {
		return nil, CodeError("COMPOSE_INVALID")
	}
	absolute := start + index
	replaced := make([]byte, 0, len(data)-len(oldValue)+len(newValue))
	replaced = append(replaced, data[:absolute]...)
	replaced = append(replaced, newValue...)
	replaced = append(replaced, data[absolute+len(oldValue):]...)
	return replaced, nil
}

func extractUI(archivePath, destination string) error {
	if err := os.Mkdir(destination, 0755); err != nil {
		return CodeError("UI_RELEASE_ALREADY_EXISTS")
	}
	// Mkdir/OpenFile 的 perm 会被进程 umask 削减（daemon unit 为 UMask=0077），
	// 会让 UI 产物变成 700/600 而 nginx worker 读不到，故创建后显式 chmod。
	if err := os.Chmod(destination, 0755); err != nil {
		return err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return CodeError("UI_ARCHIVE_INVALID")
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	total := int64(0)
	for entries := 0; ; entries++ {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil || entries > 10000 {
			return CodeError("UI_ARCHIVE_INVALID")
		}
		name := strings.TrimPrefix(header.Name, "./")
		if name == "" || name == "." {
			if header.Typeflag == tar.TypeDir {
				continue
			}
			return CodeError("UI_ARCHIVE_INVALID")
		}
		clean := filepath.Clean(name)
		if filepath.IsAbs(name) || strings.Contains(name, "\\") || clean != strings.TrimSuffix(name, "/") ||
			clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return CodeError("UI_ARCHIVE_PATH_INVALID")
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			return CodeError("UI_ARCHIVE_ENTRY_INVALID")
		}
		path := filepath.Join(destination, clean)
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
			if err := os.Chmod(path, 0755); err != nil {
				return err
			}
			continue
		}
		total += header.Size
		if header.Size < 0 || header.Size > 32*1024*1024 || total > 256*1024*1024 {
			return CodeError("UI_ARCHIVE_TOO_LARGE")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return CodeError("UI_ARCHIVE_DUPLICATE")
		}
		if err := output.Chmod(0644); err != nil {
			output.Close()
			return err
		}
		written, copyErr := io.CopyN(output, reader, header.Size)
		closeErr := output.Close()
		if copyErr != nil || closeErr != nil || written != header.Size {
			return CodeError("UI_ARCHIVE_INVALID")
		}
	}
	return nil
}

// normalizeUIPermissions 幂等地把 UI 静态资源树规范为目录 0755 / 文件 0644。
// daemon 以 UMask=0077 运行，extractUI 与 rsync -a 创建的产物会被削成 700/600，
// nginx worker（www-data）读不到，导致全站 /assets/* 404、前端白屏。
func normalizeUIPermissions(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		mode := os.FileMode(0644)
		if entry.IsDir() {
			mode = 0755
		}
		return os.Chmod(path, mode)
	})
}

func (driver *HostDriver) verifyUI(manifest release.Manifest, directory string) error {
	data, err := os.ReadFile(filepath.Join(directory, "assets/fw-cachebust.js"))
	if err != nil || release.Hash(data) != manifest.UI.BootstrapSHA256 || !strings.Contains(string(data), "/assets/"+manifest.UI.Entry) {
		return CodeError("UI_BOOTSTRAP_INVALID")
	}
	var info struct {
		Version  string `json:"version"`
		Commit   string `json:"commit"`
		Contract string `json:"contract"`
	}
	if readJSON(filepath.Join(directory, "build-info.json"), &info) != nil ||
		info.Version != manifest.Version || info.Commit != manifest.SourceSHA || info.Contract != release.ClientContract {
		return CodeError("UI_BUILD_IDENTITY_INVALID")
	}
	if _, err := os.Stat(filepath.Join(directory, "index.html")); err != nil {
		return CodeError("UI_INDEX_MISSING")
	}
	if _, err := os.Stat(filepath.Join(directory, "assets", manifest.UI.Entry)); err != nil {
		return CodeError("UI_ENTRY_MISSING")
	}
	return filepath.WalkDir(filepath.Join(directory, "assets"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() == "fw-cachebust.js" {
			return nil
		}
		relative, err := filepath.Rel(filepath.Join(directory, "assets"), path)
		if err != nil {
			return err
		}
		shared := filepath.Join(driver.config.UIRoot, "shared/assets", relative)
		if _, err := os.Stat(shared); errors.Is(err, os.ErrNotExist) {
			return nil
		}
		expected, err := fileHash(path)
		if err != nil {
			return err
		}
		actual, err := fileHash(shared)
		if err != nil || actual != expected {
			return CodeError("SHARED_ASSET_COLLISION")
		}
		return nil
	})
}

func (driver *HostDriver) compose(ctx context.Context, filename string, arguments ...string) ([]byte, error) {
	base := []string{"compose", "--project-directory", driver.config.DeploymentDir, "--env-file", filepath.Join(driver.config.DeploymentDir, ".env"), "-f", filename}
	return driver.docker(ctx, append(base, arguments...)...)
}

func (driver *HostDriver) candidate(ctx context.Context, manifest release.Manifest, id string) (string, error) {
	active := filepath.Join(driver.config.DeploymentDir, "docker-compose.yml")
	data, err := os.ReadFile(active)
	if err != nil {
		return "", err
	}
	next, err := replaceBackendImage(data, manifest.Backend.Reference())
	if err != nil {
		return "", err
	}
	path := filepath.Join(driver.config.StateDir, "prepared", id, "compose.yml")
	if err := atomicWrite(path, next, 0600); err != nil {
		return "", err
	}
	original, err := driver.compose(ctx, active, "config", "--format", "json")
	if err != nil {
		return "", err
	}
	candidate, err := driver.compose(ctx, path, "config", "--format", "json")
	if err != nil {
		return "", err
	}
	var first, second map[string]any
	if json.Unmarshal(original, &first) != nil || json.Unmarshal(candidate, &second) != nil {
		return "", CodeError("COMPOSE_INVALID")
	}
	firstServices, ok := first["services"].(map[string]any)
	if !ok {
		return "", CodeError("COMPOSE_INVALID")
	}
	secondServices, ok := second["services"].(map[string]any)
	if !ok {
		return "", CodeError("COMPOSE_INVALID")
	}
	firstBackend, firstOK := firstServices["sub2api"].(map[string]any)
	secondBackend, secondOK := secondServices["sub2api"].(map[string]any)
	if !firstOK || !secondOK || secondBackend["image"] != manifest.Backend.Reference() {
		return "", CodeError("COMPOSE_INVALID")
	}
	secondBackend["image"] = firstBackend["image"]
	if !reflect.DeepEqual(first, second) {
		return "", CodeError("COMPOSE_UNEXPECTED_CHANGE")
	}
	return path, nil
}

func (driver *HostDriver) Prepare(ctx context.Context, target Release, id string) (Snapshot, []release.Migration, error) {
	snapshot, err := driver.snapshot(ctx)
	if err != nil {
		return Snapshot{}, nil, err
	}
	current, _ := release.ParseVersion(snapshot.Version)
	next, _ := release.ParseVersion(target.Manifest.Version)
	if current.Compare(next) == 0 {
		return Snapshot{}, nil, CodeError("ALREADY_INSTALLED")
	}
	ledger := snapshot.Ledger
	if next.Compare(current) < 0 {
		var history Installed
		if readJSON(filepath.Join(driver.config.StateDir, "history", target.Manifest.Version+".json"), &history) != nil || history.ManifestHash != target.Hash {
			return Snapshot{}, nil, CodeError("ROLLBACK_NOT_DEPLOYED")
		}
		var installed Installed
		if readJSON(filepath.Join(driver.config.StateDir, "installed.json"), &installed) != nil || !installed.Manifest.Compatibility.RollbackCompatible {
			return Snapshot{}, nil, CodeError("ROLLBACK_SCHEMA_INCOMPATIBLE")
		}
		targetNames := map[string]bool{}
		for _, migration := range target.Manifest.Migrations {
			targetNames[migration.Filename] = true
		}
		ledger = map[string]string{}
		for _, migration := range installed.Manifest.Migrations {
			if targetNames[migration.Filename] {
				ledger[migration.Filename] = snapshot.Ledger[migration.Filename]
			} else if migration.Risk != "backward-compatible" || migration.NonTransactional || snapshot.Ledger[migration.Filename] != migration.Checksum {
				return Snapshot{}, nil, CodeError("ROLLBACK_SCHEMA_INCOMPATIBLE")
			}
		}
	}
	pending, err := release.PendingMigrations(target.Manifest.Migrations, ledger)
	if err != nil {
		return Snapshot{}, nil, CodeError("MIGRATION_LEDGER_MISMATCH")
	}
	if next.Compare(current) < 0 && len(pending) != 0 {
		return Snapshot{}, nil, CodeError("ROLLBACK_SCHEMA_INCOMPATIBLE")
	}
	directory := filepath.Join(driver.config.StateDir, "prepared", id)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return Snapshot{}, nil, err
	}
	if _, err := driver.docker(ctx, "pull", target.Manifest.Backend.Reference()); err != nil {
		return Snapshot{}, nil, err
	}
	if err := driver.verifyImage(ctx, target.Manifest); err != nil {
		return Snapshot{}, nil, err
	}
	archive := filepath.Join(directory, "ui.tar.gz")
	if err := driver.source.DownloadUI(ctx, target, archive); err != nil {
		return Snapshot{}, nil, err
	}
	uiDirectory := filepath.Join(directory, "ui")
	if err := extractUI(archive, uiDirectory); err != nil {
		return Snapshot{}, nil, err
	}
	if err := driver.verifyUI(target.Manifest, uiDirectory); err != nil {
		return Snapshot{}, nil, err
	}
	if _, err := driver.candidate(ctx, target.Manifest, id); err != nil {
		return Snapshot{}, nil, err
	}
	if err := availableSpace(driver.config.DeploymentDir, 2*1024*1024*1024); err != nil {
		return Snapshot{}, nil, err
	}
	if _, err := driver.runtime(ctx); err != nil {
		return Snapshot{}, nil, CodeError("MAINTENANCE_PROTOCOL_UNAVAILABLE")
	}
	return snapshot, pending, nil
}

func (driver *HostDriver) verifyImage(ctx context.Context, manifest release.Manifest) error {
	output, err := driver.docker(ctx, "image", "inspect", manifest.Backend.Reference())
	if err != nil {
		return err
	}
	var images []struct {
		Architecture string
		Os           string
		Config       struct{ Labels map[string]string }
	}
	if json.Unmarshal(output, &images) != nil || len(images) != 1 || images[0].Architecture != "amd64" || images[0].Os != "linux" ||
		images[0].Config.Labels["org.opencontainers.image.version"] != manifest.Version ||
		images[0].Config.Labels["org.opencontainers.image.revision"] != manifest.SourceSHA {
		return CodeError("IMAGE_IDENTITY_INVALID")
	}
	return nil
}

func (driver *HostDriver) runtime(ctx context.Context) (map[string]any, error) {
	if driver.runtimeProbe != nil {
		return driver.runtimeProbe(ctx)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:8080/health/deployment", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+driver.controlToken)
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return nil, CodeError("BACKEND_UNAVAILABLE")
	}
	defer response.Body.Close()
	var status map[string]any
	if response.StatusCode != 200 || json.NewDecoder(io.LimitReader(response.Body, 8192)).Decode(&status) != nil ||
		status["protocol"] != float64(release.Protocol) {
		return nil, CodeError("MAINTENANCE_PROTOCOL_UNAVAILABLE")
	}
	return status, nil
}

func (driver *HostDriver) maintenance(active bool, id string) error {
	path := filepath.Join(driver.config.ControlDir, "maintenance.json")
	if err := writeJSON(path, map[string]any{"active": active, "operation_id": id}, 0640); err != nil {
		return err
	}
	return os.Chown(path, -1, driver.config.SocketGID)
}

// ensureMaintenanceOwnership forces the maintenance file to stay readable by the
// backend container. The backend runs as the SocketGID group and reads this file
// through a fail-closed gate: if it cannot read the file it treats the site as
// under maintenance and returns 503 for every business request. The file must
// therefore exist with the group set even when no operation is in flight.
func (driver *HostDriver) ensureMaintenanceOwnership() error {
	path := filepath.Join(driver.config.ControlDir, "maintenance.json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := writeJSON(path, map[string]any{"active": false, "operation_id": ""}, 0640); err != nil {
			return err
		}
	}
	if err := os.Chmod(path, 0640); err != nil {
		return err
	}
	return os.Chown(path, -1, driver.config.SocketGID)
}

func (driver *HostDriver) backup(ctx context.Context, operation Operation) (string, error) {
	if driver.backupOverride != nil {
		return driver.backupOverride(ctx, operation)
	}
	if err := os.MkdirAll(filepath.Join(driver.config.DeploymentDir, "backups"), 0700); err != nil {
		return "", CodeError("BACKUP_DIRECTORY_UNAVAILABLE")
	}
	path := filepath.Join(driver.config.DeploymentDir, "backups", "updater-"+operation.ID)
	if err := os.Mkdir(path, 0700); err != nil {
		return "", CodeError("BACKUP_ALREADY_EXISTS")
	}
	for source, name := range map[string]string{
		filepath.Join(driver.config.DeploymentDir, "docker-compose.yml"):     "compose.yml",
		filepath.Join(driver.config.DeploymentDir, ".env"):                   "runtime.env",
		filepath.Join(driver.config.UIRoot, "shared/assets/fw-cachebust.js"): "bootstrap.js",
		filepath.Join(driver.config.StateDir, "installed.json"):              "installed.json",
	} {
		data, err := os.ReadFile(source)
		if err != nil {
			return "", err
		}
		if err := atomicWrite(filepath.Join(path, name), data, 0600); err != nil {
			return "", err
		}
	}
	dump, err := os.OpenFile(filepath.Join(path, "database.dump"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	process := exec.CommandContext(ctx, "docker", "--config", driver.config.DockerConfig, "exec", "sub2api-postgres",
		"sh", "-c", `exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc`)
	process.Stdout, process.Stderr = dump, io.Discard
	runErr := process.Run()
	closeErr := dump.Close()
	if runErr != nil || closeErr != nil {
		return "", CodeError("DATABASE_BACKUP_FAILED")
	}
	input, err := os.Open(filepath.Join(path, "database.dump"))
	if err != nil {
		return "", err
	}
	defer input.Close()
	verify := exec.CommandContext(ctx, "docker", "--config", driver.config.DockerConfig, "exec", "-i", "sub2api-postgres", "pg_restore", "--list")
	verify.Stdin, verify.Stdout, verify.Stderr = input, io.Discard, io.Discard
	if verify.Run() != nil {
		return "", CodeError("DATABASE_BACKUP_INVALID")
	}
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, input); err != nil {
		return "", err
	}
	if err := atomicWrite(filepath.Join(path, "database.sha256"), []byte(hex.EncodeToString(hash.Sum(nil))+"  database.dump\n"), 0600); err != nil {
		return "", err
	}
	if err := writeJSON(filepath.Join(path, "snapshot.json"), operation.Snapshot, 0600); err != nil {
		return "", err
	}
	if config, err := os.ReadFile(filepath.Join(driver.config.DeploymentDir, "data/config.yaml")); err == nil {
		if err := atomicWrite(filepath.Join(path, "config.yaml"), config, 0600); err != nil {
			return "", err
		}
	}
	return path, nil
}

func (driver *HostDriver) Activate(ctx context.Context, operation Operation, stage func(string)) (activationError error) {
	current, err := driver.snapshot(ctx)
	if err != nil || !reflect.DeepEqual(current, operation.Snapshot) {
		return CodeError("DEPLOYMENT_DRIFT")
	}
	candidate, err := driver.candidate(ctx, operation.Target, operation.ID)
	if err != nil {
		return err
	}
	if err := driver.maintenance(true, operation.ID); err != nil {
		return err
	}
	stopped, changed := false, false
	defer func() {
		if activationError == nil {
			return
		}
		if changed {
			return
		}
		if stopped {
			status, err := driver.docker(context.WithoutCancel(ctx), "inspect", "--format", "{{.State.Running}}", "sub2api")
			if err != nil || strings.TrimSpace(string(status)) != "false" {
				return
			}
			if _, err := driver.docker(context.WithoutCancel(ctx), "start", "sub2api"); err != nil {
				return
			}
			var installed Installed
			if readJSON(filepath.Join(driver.config.StateDir, "installed.json"), &installed) != nil ||
				driver.waitHealthy(context.WithoutCancel(ctx), installed.Manifest) != nil {
				return
			}
		}
		_ = driver.maintenance(false, "")
	}()
	deadline := time.Now().Add(5 * time.Minute)
	for {
		if ctx.Err() != nil || time.Now().After(deadline) {
			return CodeError("DRAIN_TIMEOUT")
		}
		status, err := driver.runtime(ctx)
		if err != nil {
			return err
		}
		if status["active"] == true && status["inflight"] == float64(0) {
			break
		}
		select {
		case <-ctx.Done():
			return CodeError("DRAIN_TIMEOUT")
		case <-time.After(time.Second):
		}
	}
	stage("stopping_backend")
	stopped = true
	shutdownContext, shutdownCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer shutdownCancel()
	if _, err := driver.docker(shutdownContext, "stop", "--signal", "SIGTERM", "--timeout", "-1", "sub2api"); err != nil {
		return CodeError("BACKEND_DRAIN_TIMEOUT")
	}
	shutdownDeadline := time.Now().Add(5 * time.Minute)
	for {
		running, err := driver.docker(ctx, "inspect", "--format", "{{.State.Running}}", "sub2api")
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(running)) == "false" {
			break
		}
		if time.Now().After(shutdownDeadline) || ctx.Err() != nil {
			return CodeError("BACKEND_DRAIN_TIMEOUT")
		}
		select {
		case <-ctx.Done():
			return CodeError("BACKEND_DRAIN_TIMEOUT")
		case <-time.After(time.Second):
		}
	}
	state, err := driver.docker(ctx, "inspect", "--format", "{{.State.ExitCode}}", "sub2api")
	if err != nil || strings.TrimSpace(string(state)) != "0" {
		return CodeError("BACKEND_DID_NOT_DRAIN")
	}
	stage("backing_up")
	backup, err := driver.backup(ctx, operation)
	if err != nil {
		return err
	}
	stage("revalidating")
	ledger, err := driver.ledger(ctx)
	if err != nil || !reflect.DeepEqual(ledger, operation.Snapshot.Ledger) {
		return CodeError("MIGRATION_LEDGER_DRIFT")
	}
	next, err := os.ReadFile(candidate)
	if err != nil {
		return err
	}
	activeCompose := filepath.Join(driver.config.DeploymentDir, "docker-compose.yml")
	if err := atomicWrite(activeCompose, next, 0600); err != nil {
		return err
	}
	changed = true
	stage("activating_backend")
	if _, err := driver.compose(ctx, activeCompose, "up", "-d", "--no-build", "--no-deps", "--force-recreate", "sub2api"); err != nil {
		return err
	}
	stage("verifying_backend")
	if err := driver.waitHealthy(ctx, operation.Target); err != nil {
		return err
	}
	stage("activating_ui")
	source := filepath.Join(driver.config.StateDir, "prepared", operation.ID, "ui")
	target := filepath.Join(driver.config.UIRoot, "releases", operation.Target.Version)
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		if err := driver.materializeUI(ctx, source, target); err != nil {
			return err
		}
		// 产物可能被 umask 削成 700/600（prepared 解包与跨设备复制都会如此），
		// 切版前先规范化，避免 nginx worker 读不到。
		if err := normalizeUIPermissions(target); err != nil {
			return err
		}
	}
	if err := driver.verifyUI(operation.Target, target); err != nil {
		return err
	}
	if _, err := driver.run(ctx, "rsync", "-a", "--ignore-existing", "--exclude=/fw-cachebust.js", target+"/assets/", driver.config.UIRoot+"/shared/assets/"); err != nil {
		return err
	}
	// rsync -a 会把目标 shared/assets 目录及新文件的权限同步为源权限，
	// 同步后强制规范化整个 shared 累积库，确保 nginx worker 可读。
	if err := normalizeUIPermissions(filepath.Join(driver.config.UIRoot, "shared")); err != nil {
		return err
	}
	bootstrap, err := os.ReadFile(filepath.Join(target, "assets/fw-cachebust.js"))
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(driver.config.UIRoot, "shared/assets/fw-cachebust.js"), bootstrap, 0644); err != nil {
		return err
	}
	if _, err := driver.run(ctx, "bash", filepath.Join(driver.config.UIRoot, "switch-release.sh"), operation.Target.Version); err != nil {
		old, readErr := os.ReadFile(filepath.Join(backup, "bootstrap.js"))
		if readErr == nil {
			_ = atomicWrite(filepath.Join(driver.config.UIRoot, "shared/assets/fw-cachebust.js"), old, 0644)
		}
		return CodeError("UI_SWITCH_FAILED")
	}
	// switch-release.sh 内部用 cp -rn 把 release assets 补入 shared，
	// cp 同样受进程 umask 削减，切版后再兜底规范化一次。
	if err := normalizeUIPermissions(filepath.Join(driver.config.UIRoot, "shared")); err != nil {
		return err
	}
	stage("verifying_site")
	if _, err := driver.run(ctx, "curl", "--fail", "--silent", "--show-error", "--max-time", "15", "https://sshzyu.com/health"); err != nil {
		return err
	}
	record := Installed{ManifestHash: operation.ManifestHash, Manifest: operation.Target, InstalledAt: time.Now().UTC()}
	if err := writeJSON(filepath.Join(driver.config.StateDir, "history", record.Manifest.Version+".json"), record, 0600); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(driver.config.StateDir, "installed.json"), record, 0600); err != nil {
		return err
	}
	return driver.maintenance(false, "")
}

func (driver *HostDriver) waitHealthy(ctx context.Context, manifest release.Manifest) error {
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		status, err := driver.runtime(ctx)
		if err == nil && status["version"] == manifest.Version && status["commit"] == manifest.SourceSHA && status["active"] == true {
			ledger, err := driver.ledger(ctx)
			if err != nil {
				return err
			}
			for _, migration := range manifest.Migrations {
				if ledger[migration.Filename] != migration.Checksum {
					return CodeError("MIGRATION_VERIFICATION_FAILED")
				}
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return CodeError("BACKEND_HEALTH_FAILED")
		case <-time.After(time.Second):
		}
	}
	return CodeError("BACKEND_HEALTH_FAILED")
}

func (driver *HostDriver) Rollback(ctx context.Context, operation Operation, stage func(string)) error {
	if len(operation.Pending) != 0 {
		return CodeError("ROLLBACK_SCHEMA_INCOMPATIBLE")
	}
	var history Installed
	if readJSON(filepath.Join(driver.config.StateDir, "history", operation.Target.Version+".json"), &history) != nil ||
		history.ManifestHash != operation.ManifestHash {
		return CodeError("ROLLBACK_NOT_DEPLOYED")
	}
	return driver.Activate(ctx, operation, stage)
}
