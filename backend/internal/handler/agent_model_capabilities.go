package handler

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/agent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (p *platformAgentExecutor) ModelCapabilities(ctx context.Context, b agent.Binding, models []string) ([]agent.ModelCapability, error) {
	key, err := p.key(ctx, b)
	if err != nil {
		return nil, err
	}
	return p.reasoningCapabilities(ctx, key, models)
}
func (p *platformAgentExecutor) reasoningCapabilities(ctx context.Context, key *service.APIKey, models []string) ([]agent.ModelCapability, error) {
	raw, err := p.gateway.BuildCodexModelsManifestForGroup(ctx, key.Group, "", models)
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Models []struct {
			Slug    string `json:"slug"`
			Default string `json:"default_reasoning_level"`
			Levels  []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	for _, id := range models {
		allowed[id] = true
	}
	result := []agent.ModelCapability{}
	for _, model := range manifest.Models {
		if !allowed[model.Slug] {
			continue
		}
		cap := agent.ModelCapability{ID: model.Slug, DefaultReasoningEffort: model.Default, ReasoningLevels: []string{}}
		seen := map[string]bool{}
		for _, level := range model.Levels {
			switch level.Effort {
			case "none", "minimal", "low", "medium", "high", "xhigh", "max":
				if !seen[level.Effort] {
					cap.ReasoningLevels = append(cap.ReasoningLevels, level.Effort)
					seen[level.Effort] = true
				}
			}
		}
		if !seen[cap.DefaultReasoningEffort] {
			cap.DefaultReasoningEffort = ""
			if len(cap.ReasoningLevels) > 0 {
				cap.DefaultReasoningEffort = cap.ReasoningLevels[0]
			}
		}
		result = append(result, cap)
	}
	return result, nil
}
func (p *platformAgentExecutor) invocationPayload(ctx context.Context, key *service.APIKey, in *agent.Invocation) (string, string, []byte, error) {
	if in.Kind == "text" {
		caps, err := p.reasoningCapabilities(ctx, key, []string{in.Model})
		if err != nil {
			return "", "", nil, err
		}
		if len(caps) > 0 {
			cap := caps[0]
			found := false
			for _, level := range cap.ReasoningLevels {
				if level == in.ReasoningEffort {
					found = true
				}
				if level != "none" {
					in.ReasoningSupported = true
				}
			}
			if !found {
				switch in.ReasoningEffort {
				case "none", "minimal", "xhigh", "max":
					return "", "", nil, agent.Invalid("AGENT_MODEL_REASONING_UNSUPPORTED")
				}
				in.ReasoningEffort = cap.DefaultReasoningEffort
			}
		}
	}
	return p.payload(*in)
}
