package updater

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/release"
)

type Config struct {
	StateDir                 string `json:"state_dir"`
	Socket                   string `json:"socket"`
	ControlTokenFile         string `json:"control_token_file"`
	PublicKeyFile            string `json:"public_key_file"`
	SigningKeyID             string `json:"signing_key_id"`
	GitHubTokenFile          string `json:"github_token_file"`
	DockerConfig             string `json:"docker_config"`
	DeploymentDir            string `json:"deployment_dir"`
	UIRoot                   string `json:"ui_root"`
	ControlDir               string `json:"control_dir"`
	ActivationEnabled        bool   `json:"activation_enabled"`
	PaymentCallbacksReviewed bool   `json:"payment_callbacks_reviewed"`
	ExpectedHostname         string `json:"expected_hostname"`
	SocketGID                int    `json:"socket_gid"`
	ClientUID                uint32 `json:"client_uid"`
}

type Release struct {
	Hash      string           `json:"manifest_hash"`
	Manifest  release.Manifest `json:"manifest"`
	ReleaseID int64            `json:"release_id"`
	UIAssetID int64            `json:"-"`
}

type Snapshot struct {
	Version         string            `json:"version"`
	Contract        string            `json:"contract"`
	ManifestHash    string            `json:"manifest_hash"`
	Image           string            `json:"image"`
	ImageID         string            `json:"image_id"`
	ContainerID     string            `json:"container_id"`
	ComposeHash     string            `json:"compose_hash"`
	EnvironmentHash string            `json:"environment_hash"`
	UICurrent       string            `json:"ui_current"`
	BootstrapHash   string            `json:"bootstrap_hash"`
	Ledger          map[string]string `json:"ledger"`
}

type Operation struct {
	ID             string              `json:"id"`
	Kind           string              `json:"kind"`
	ActorID        int64               `json:"actor_id"`
	IdempotencyKey string              `json:"-"`
	ManifestHash   string              `json:"manifest_hash"`
	Target         release.Manifest    `json:"target"`
	Snapshot       Snapshot            `json:"snapshot"`
	Pending        []release.Migration `json:"pending_migrations"`
	Stage          string              `json:"stage"`
	Error          string              `json:"error,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
}

type storedOperation struct {
	Operation
	Key string `json:"idempotency_key"`
}

type PrepareRequest struct {
	Version        string `json:"version"`
	ManifestHash   string `json:"manifest_hash"`
	ActorID        int64  `json:"actor_id"`
	IdempotencyKey string `json:"idempotency_key"`
	Kind           string `json:"kind,omitempty"`
}

type ActivateRequest struct {
	ManifestHash      string   `json:"manifest_hash"`
	ConfirmDowntime   bool     `json:"confirm_downtime"`
	ConfirmMigrations []string `json:"confirm_migrations"`
}

type Source interface {
	List(context.Context) ([]Release, error)
	Get(context.Context, string, string) (Release, error)
}

type Driver interface {
	Prepare(context.Context, Release, string) (Snapshot, []release.Migration, error)
	Activate(context.Context, Operation, func(string)) error
	Rollback(context.Context, Operation, func(string)) error
}

type Manager struct {
	config     Config
	source     Source
	driver     Driver
	mu         sync.Mutex
	operations map[string]Operation
	active     string
}

var operationPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var manifestHashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func terminal(stage string) bool {
	return stage == "completed" || stage == "cancelled" || stage == "failed" || stage == "rolled_back"
}

func NewManager(config Config, source Source, driver Driver) (*Manager, error) {
	if config.StateDir == "" || source == nil || driver == nil {
		return nil, errors.New("invalid updater configuration")
	}
	if err := os.MkdirAll(filepath.Join(config.StateDir, "operations"), 0700); err != nil {
		return nil, err
	}
	manager := &Manager{config: config, source: source, driver: driver, operations: map[string]Operation{}}
	files, err := filepath.Glob(filepath.Join(config.StateDir, "operations", "*.json"))
	if err != nil {
		return nil, err
	}
	for _, filename := range files {
		var stored storedOperation
		data, err := os.ReadFile(filename)
		if err != nil || json.Unmarshal(data, &stored) != nil || !operationPattern.MatchString(stored.ID) {
			return nil, errors.New("invalid persisted updater operation")
		}
		operation := stored.Operation
		operation.IdempotencyKey = stored.Key
		if !terminal(operation.Stage) {
			if manager.active != "" {
				return nil, errors.New("multiple unfinished updater operations")
			}
			if operation.Stage == "queued" || operation.Stage == "preflight" {
				operation.Stage, operation.Error = "failed", "PREPARATION_INTERRUPTED"
			} else {
				manager.active = operation.ID
			}
			if operation.Stage != "ready" && operation.Stage != "failed" {
				operation.Stage = "manual_intervention"
				if operation.Error == "" {
					operation.Error = "RECOVERY_REQUIRED"
				}
			}
		}
		manager.operations[operation.ID] = operation
		if err := manager.save(operation); err != nil {
			return nil, err
		}
	}
	return manager, nil
}

func atomicWrite(filename string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(filename), ".updater-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), filename)
}

func (manager *Manager) save(operation Operation) error {
	data, err := json.MarshalIndent(storedOperation{Operation: operation, Key: operation.IdempotencyKey}, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(manager.config.StateDir, "operations", operation.ID+".json"), data, 0600)
}

func (manager *Manager) setStage(id, stage, code string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	operation := manager.operations[id]
	operation.Stage, operation.Error, operation.UpdatedAt = stage, code, time.Now().UTC()
	if err := manager.save(operation); err != nil {
		operation.Stage, operation.Error = "manual_intervention", "STATE_WRITE_FAILED"
	}
	manager.operations[id] = operation
	if terminal(operation.Stage) && manager.active == id {
		manager.active = ""
	}
}

func (manager *Manager) Status(id string) (Operation, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	operation, exists := manager.operations[id]
	if !exists || !operationPattern.MatchString(id) {
		return Operation{}, errors.New("OPERATION_NOT_FOUND")
	}
	return operation, nil
}

func (manager *Manager) Prepare(ctx context.Context, request PrepareRequest) (Operation, error) {
	if _, err := release.ParseVersion(request.Version); err != nil || !manifestHashPattern.MatchString(request.ManifestHash) ||
		request.ActorID <= 0 || !keyPattern.MatchString(request.IdempotencyKey) {
		return Operation{}, errors.New("INVALID_UPDATE_REQUEST")
	}
	kind := request.Kind
	if kind == "" {
		kind = "update"
	}
	if kind != "update" && kind != "rollback" {
		return Operation{}, errors.New("INVALID_OPERATION_KIND")
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	for _, existing := range manager.operations {
		if existing.IdempotencyKey == request.IdempotencyKey && existing.ActorID == request.ActorID {
			if existing.ManifestHash != request.ManifestHash || existing.Target.Version != request.Version || existing.Kind != kind {
				return Operation{}, errors.New("IDEMPOTENCY_CONFLICT")
			}
			return existing, nil
		}
	}
	if manager.active != "" {
		return Operation{}, errors.New("UPDATE_IN_PROGRESS")
	}
	target, err := manager.source.Get(ctx, request.Version, request.ManifestHash)
	if err != nil {
		return Operation{}, errors.New("TARGET_VERIFICATION_FAILED")
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return Operation{}, err
	}
	operation := Operation{ID: hex.EncodeToString(random), Kind: kind, ActorID: request.ActorID,
		IdempotencyKey: request.IdempotencyKey, ManifestHash: target.Hash, Target: target.Manifest,
		Stage: "queued", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := manager.save(operation); err != nil {
		return Operation{}, errors.New("STATE_WRITE_FAILED")
	}
	manager.operations[operation.ID], manager.active = operation, operation.ID
	go manager.prepare(operation, target)
	return operation, nil
}

func (manager *Manager) prepare(operation Operation, target Release) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	manager.setStage(operation.ID, "preflight", "")
	snapshot, pending, err := manager.driver.Prepare(ctx, target, operation.ID)
	if err != nil {
		manager.setStage(operation.ID, "failed", errorCode(err, "PREPARE_FAILED"))
		return
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	current := manager.operations[operation.ID]
	if current.Stage == "cancelled" {
		return
	}
	current.Snapshot, current.Pending, current.Stage, current.UpdatedAt = snapshot, pending, "ready", time.Now().UTC()
	if err := manager.save(current); err != nil {
		current.Stage, current.Error = "failed", "STATE_WRITE_FAILED"
		manager.active = ""
	}
	manager.operations[current.ID] = current
}

func (manager *Manager) Activate(id string, request ActivateRequest) (Operation, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	operation, exists := manager.operations[id]
	if !exists || operation.Stage != "ready" || manager.active != id {
		return Operation{}, errors.New("OPERATION_NOT_READY")
	}
	if !manager.config.ActivationEnabled {
		return Operation{}, errors.New("ACTIVATION_DISABLED")
	}
	if !manager.config.PaymentCallbacksReviewed {
		return Operation{}, errors.New("PAYMENT_CALLBACK_REVIEW_REQUIRED")
	}
	expected := make([]string, 0, len(operation.Pending))
	for _, migration := range operation.Pending {
		if migration.Risk != "backward-compatible" || migration.NonTransactional {
			return Operation{}, errors.New("MANUAL_MIGRATION_REQUIRED")
		}
		expected = append(expected, migration.Filename)
	}
	confirmed := append([]string{}, request.ConfirmMigrations...)
	sort.Strings(confirmed)
	if !request.ConfirmDowntime || request.ManifestHash != operation.ManifestHash || !reflect.DeepEqual(expected, confirmed) {
		return Operation{}, errors.New("UPDATE_CONFIRMATION_REQUIRED")
	}
	operation.Stage, operation.UpdatedAt = "draining", time.Now().UTC()
	if err := manager.save(operation); err != nil {
		return Operation{}, errors.New("STATE_WRITE_FAILED")
	}
	manager.operations[id] = operation
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		target, err := manager.source.Get(ctx, operation.Target.Version, operation.ManifestHash)
		if err != nil || target.Hash != operation.ManifestHash {
			manager.setStage(id, "failed", "TARGET_CHANGED")
			return
		}
		stage := func(value string) { manager.setStage(id, value, "") }
		if operation.Kind == "rollback" {
			err = manager.driver.Rollback(ctx, operation, stage)
		} else {
			err = manager.driver.Activate(ctx, operation, stage)
		}
		if err != nil {
			manager.setStage(id, "manual_intervention", errorCode(err, "ACTIVATION_FAILED"))
			return
		}
		manager.setStage(id, "completed", "")
	}()
	return operation, nil
}

func (manager *Manager) Cancel(id string) (Operation, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	operation, exists := manager.operations[id]
	if !exists || operation.Stage != "ready" {
		return Operation{}, errors.New("OPERATION_CANNOT_BE_CANCELLED")
	}
	operation.Stage, operation.UpdatedAt = "cancelled", time.Now().UTC()
	if err := manager.save(operation); err != nil {
		return Operation{}, err
	}
	manager.operations[id], manager.active = operation, ""
	return operation, nil
}

type CodeError string

func (code CodeError) Error() string { return string(code) }

func errorCode(err error, fallback string) string {
	var code CodeError
	if errors.As(err, &code) {
		return string(code)
	}
	return fallback
}
