package updater

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestControlAPIRequiresTokenAndPreservesOperationAcrossRequests(t *testing.T) {
	manager := testManager(t, t.TempDir(), &fakeDriver{})
	handler := manager.Handler("unit-test-control")
	request := httptest.NewRequest("GET", "/v1/releases", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 401 {
		t.Fatal("unauthenticated control access accepted")
	}
	input := PrepareRequest{Version: "0.2.13-r2", ManifestHash: strings.Repeat("a", 64), ActorID: 7, IdempotencyKey: "http-request"}
	data, _ := json.Marshal(input)
	request = httptest.NewRequest("POST", "/v1/prepare", bytes.NewReader(data))
	request.Header.Set("Authorization", "Bearer unit-test-control")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 202 {
		t.Fatalf("prepare status: %d", response.Code)
	}
	var operation Operation
	if json.Unmarshal(response.Body.Bytes(), &operation) != nil {
		t.Fatal("invalid operation response")
	}
	waitStage(t, manager, operation.ID, "ready")
	request = httptest.NewRequest("GET", "/v1/capabilities", nil)
	request.Header.Set("Authorization", "Bearer unit-test-control")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var capabilities struct {
		ActiveOperation *Operation `json:"active_operation"`
	}
	if json.Unmarshal(response.Body.Bytes(), &capabilities) != nil || capabilities.ActiveOperation == nil ||
		capabilities.ActiveOperation.ID != operation.ID || capabilities.ActiveOperation.Stage != "ready" {
		t.Fatal("active host preparation omitted from capabilities")
	}
	for _, path := range []string{"/v1/capabilities", "/v1/releases", "/v1/operations/" + operation.ID} {
		request = httptest.NewRequest("GET", path, nil)
		request.Header.Set("Authorization", "Bearer unit-test-control")
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != 200 {
			t.Fatalf("status for %s: %d", path, response.Code)
		}
	}
	request = httptest.NewRequest("POST", "/v1/operations/"+operation.ID+"/cancel", nil)
	request.Header.Set("Authorization", "Bearer unit-test-control")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal("cancellation failed")
	}
}
