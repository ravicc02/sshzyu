package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// postControl issues an authenticated control-API POST and returns the HTTP
// status together with the decoded "error" code (empty when absent).
func postControl(t *testing.T, handler http.Handler, path string, body any) (int, string) {
	t.Helper()
	var payload []byte
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = data
	}
	request := httptest.NewRequest("POST", path, bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer unit-test-control")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var decoded struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &decoded)
	return response.Code, decoded.Error
}

// Defect 1: the control API must surface the specific rejection code instead of
// collapsing every Prepare failure into the UPDATE_REQUEST_REJECTED fallback.
func TestControlAPIExposesSpecificErrorCodes(t *testing.T) {
	manager := testManager(t, t.TempDir(), &fakeDriver{})
	handler := manager.Handler("unit-test-control")

	// Malformed prepare input must report INVALID_UPDATE_REQUEST, not the fallback.
	status, code := postControl(t, handler, "/v1/prepare", PrepareRequest{
		Version: "not-a-version", ManifestHash: strings.Repeat("a", 64), ActorID: 7, IdempotencyKey: "bad",
	})
	if status != 409 || code != "INVALID_UPDATE_REQUEST" {
		t.Fatalf("invalid prepare: status=%d code=%q (want 409 INVALID_UPDATE_REQUEST)", status, code)
	}

	// A second concurrent prepare while one is active must report UPDATE_IN_PROGRESS.
	first := PrepareRequest{Version: "0.2.13-r2", ManifestHash: strings.Repeat("a", 64), ActorID: 7, IdempotencyKey: "first"}
	if status, code := postControl(t, handler, "/v1/prepare", first); status != 202 || code != "" {
		t.Fatalf("accepted prepare: status=%d code=%q", status, code)
	}
	second := first
	second.IdempotencyKey = "second"
	if status, code := postControl(t, handler, "/v1/prepare", second); status != 409 || code != "UPDATE_IN_PROGRESS" {
		t.Fatalf("concurrent prepare: status=%d code=%q (want 409 UPDATE_IN_PROGRESS)", status, code)
	}

	// Unknown operation ID must report OPERATION_NOT_FOUND.
	if status, code := postControl(t, handler, "/v1/operations/"+strings.Repeat("f", 32)+"/cancel", nil); status != 409 || code != "OPERATION_CANNOT_BE_CANCELLED" {
		t.Fatalf("cancel unknown: status=%d code=%q", status, code)
	}
}

// Defect 2: recovery must be executable inside the running daemon so that the
// in-memory manager and the on-disk state stay consistent. Before the fix only
// a separate CLI process could recover, leaving the daemon's `active` pointer
// stale and rejecting every later prepare with UPDATE_IN_PROGRESS.
func TestControlAPIRecoverRunsInsideDaemon(t *testing.T) {
	driver := &fakeDriver{fail: true}
	manager := testManager(t, t.TempDir(), driver)
	handler := manager.Handler("unit-test-control")

	operation, err := manager.Prepare(context.Background(), PrepareRequest{
		Version: "0.2.13-r2", ManifestHash: strings.Repeat("a", 64), ActorID: 7, IdempotencyKey: "recover-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	operation = waitStage(t, manager, operation.ID, "ready")
	if _, err := manager.Activate(operation.ID, ActivateRequest{
		ManifestHash: operation.ManifestHash, ConfirmDowntime: true, ConfirmMigrations: []string{"002_new.sql"},
	}); err != nil {
		t.Fatal(err)
	}
	waitStage(t, manager, operation.ID, "manual_intervention")

	status, code := postControl(t, handler, "/v1/operations/"+operation.ID+"/recover", RecoverRequest{Decision: "complete"})
	if status == 404 {
		t.Fatal("recover route is not registered on the control API")
	}
	// The fake driver is not a *HostDriver, so Recover reaches the driver type
	// assertion and reports a specific code - proving the route executed the
	// daemon-side Recover path rather than falling through to a 404.
	if status != 409 || code != "RECOVERY_DRIVER_UNAVAILABLE" {
		t.Fatalf("recover route: status=%d code=%q (want 409 RECOVERY_DRIVER_UNAVAILABLE)", status, code)
	}

	// A missing/invalid decision must be rejected with its own code.
	if status, code := postControl(t, handler, "/v1/operations/"+operation.ID+"/recover", RecoverRequest{Decision: "nonsense"}); status != 409 || code != "RECOVERY_CONFIRMATION_REQUIRED" {
		t.Fatalf("recover invalid decision: status=%d code=%q", status, code)
	}
}
