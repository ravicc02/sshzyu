package updater

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/pkg/release"
)

type peerKey struct{}

func decodeRequest(request *http.Request, value any) error {
	decoder := json.NewDecoder(io.LimitReader(request.Body, 32769))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		return CodeError("INVALID_REQUEST")
	}
	return nil
}

func (manager *Manager) Handler(token string) http.Handler {
	mux := http.NewServeMux()
	reply := func(response http.ResponseWriter, status int, value any) {
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("Cache-Control", "no-store")
		response.WriteHeader(status)
		_ = json.NewEncoder(response).Encode(value)
	}
	failure := func(response http.ResponseWriter, err error) {
		reply(response, http.StatusConflict, map[string]string{"error": errorCode(err, "UPDATE_REQUEST_REJECTED")})
	}
	mux.HandleFunc("GET /v1/capabilities", func(response http.ResponseWriter, request *http.Request) {
		var installed Installed
		err := readJSON(manager.config.StateDir+"/installed.json", &installed)
		reply(response, 200, map[string]any{"protocol": release.Protocol, "activation_enabled": manager.config.ActivationEnabled && manager.config.PaymentCallbacksReviewed,
			"bootstrapped": err == nil, "installed": installed, "active_operation": manager.currentOperation()})
	})
	mux.HandleFunc("GET /v1/releases", func(response http.ResponseWriter, request *http.Request) {
		targets, err := manager.releaseCatalog(request.Context())
		if err != nil {
			failure(response, err)
			return
		}
		reply(response, 200, map[string]any{"releases": targets})
	})
	mux.HandleFunc("POST /v1/prepare", func(response http.ResponseWriter, request *http.Request) {
		var input PrepareRequest
		if err := decodeRequest(request, &input); err != nil {
			failure(response, err)
			return
		}
		operation, err := manager.Prepare(request.Context(), input)
		if err != nil {
			failure(response, err)
			return
		}
		reply(response, 202, operation)
	})
	mux.HandleFunc("GET /v1/operations/{id}", func(response http.ResponseWriter, request *http.Request) {
		operation, err := manager.Status(request.PathValue("id"))
		if err != nil {
			failure(response, err)
			return
		}
		reply(response, 200, operation)
	})
	mux.HandleFunc("POST /v1/operations/{id}/activate", func(response http.ResponseWriter, request *http.Request) {
		var input ActivateRequest
		if err := decodeRequest(request, &input); err != nil {
			failure(response, err)
			return
		}
		operation, err := manager.Activate(request.PathValue("id"), input)
		if err != nil {
			failure(response, err)
			return
		}
		reply(response, 202, operation)
	})
	mux.HandleFunc("POST /v1/operations/{id}/cancel", func(response http.ResponseWriter, request *http.Request) {
		operation, err := manager.Cancel(request.PathValue("id"))
		if err != nil {
			failure(response, err)
			return
		}
		reply(response, 200, operation)
	})
	// Recovery is executed inside the daemon so that the in-memory manager and
	// the on-disk state stay consistent. Running it as a separate CLI process
	// rewrites disk only, leaving the daemon's `active` pointer stale.
	mux.HandleFunc("POST /v1/operations/{id}/recover", func(response http.ResponseWriter, request *http.Request) {
		var input RecoverRequest
		if err := decodeRequest(request, &input); err != nil {
			failure(response, err)
			return
		}
		id := request.PathValue("id")
		if err := manager.Recover(request.Context(), id, input.Decision); err != nil {
			failure(response, err)
			return
		}
		operation, err := manager.Status(id)
		if err != nil {
			failure(response, err)
			return
		}
		reply(response, 200, operation)
	})
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if allowed, exists := request.Context().Value(peerKey{}).(bool); exists && !allowed {
			reply(response, 403, map[string]string{"error": "CONTROL_PEER_REJECTED"})
			return
		}
		provided := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		if token == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			reply(response, 401, map[string]string{"error": "CONTROL_AUTH_REQUIRED"})
			return
		}
		mux.ServeHTTP(response, request)
	})
}
