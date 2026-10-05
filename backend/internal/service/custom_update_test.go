package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/release"
)

type customAgentStub struct {
	unavailable bool
	active      bool
}

func (agent customAgentStub) Call(_ context.Context, _ string, path string, _ any, output any) error {
	if agent.unavailable {
		return errors.New("offline")
	}
	var data string
	switch path {
	case "/v1/capabilities":
		data = `{"protocol":1,"activation_enabled":true,"bootstrapped":true,"installed":{"manifest":{"version":"0.2.13-r9","source_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}`
	case "/v1/releases":
		data = `{"releases":[{"manifest_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","manifest":{"version":"0.2.13-r10","source_sha":"cccccccccccccccccccccccccccccccccccccccc","repository":"ravicc02/sshzyu"}}]}`
	default:
		return errors.New("unexpected route")
	}
	if agent.active && path == "/v1/capabilities" {
		var payload map[string]any
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			return err
		}
		payload["active_operation"] = map[string]any{
			"id": "11111111111111111111111111111111", "kind": "update", "stage": "ready",
			"manifest_hash": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"target":        map[string]any{"version": "0.2.13-r10"}, "pending_migrations": []any{},
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		data = string(encoded)
	}
	return json.Unmarshal([]byte(data), output)
}

func TestCustomUpdatesCompareRevisionAndNeverInstallOfficial(t *testing.T) {
	svc := NewUpdateService(nil, nil, "0.2.13-r9", "release").WithAgent(customAgentStub{})
	info, err := svc.CheckUpdate(context.Background(), true)
	if err != nil || !info.HasUpdate || info.LatestVersion != "0.2.13-r10" || info.UpdateSource != release.Repository {
		t.Fatalf("invalid custom update: %#v %v", info, err)
	}
	if err := svc.PerformUpdate(context.Background()); err == nil {
		t.Fatal("custom binary update accepted")
	}
	if err := svc.Rollback(); err == nil {
		t.Fatal("custom binary rollback accepted")
	}
}

func TestCustomAgentFailureIsNotUpToDateOrOfficialFallback(t *testing.T) {
	svc := NewUpdateService(nil, nil, "0.2.13-r9", "release").WithAgent(customAgentStub{unavailable: true})
	info, err := svc.CheckUpdate(context.Background(), true)
	if err != nil || info.CheckStatus != "unknown" || info.CanUpdate || info.Warning == "" {
		t.Fatalf("agent failure masked: %#v %v", info, err)
	}
}

func TestCustomUpdateIncludesCurrentHostOperation(t *testing.T) {
	svc := NewUpdateService(nil, nil, "0.2.13-r9", "release").WithAgent(customAgentStub{active: true})
	info, err := svc.CheckUpdate(context.Background(), true)
	if err != nil || info.ActiveOperation == nil || info.ActiveOperation.Stage != "ready" ||
		info.ActiveOperation.Target.Version != "0.2.13-r10" {
		t.Fatalf("active host preparation omitted: %#v %v", info, err)
	}
}
