//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/agent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/agentlimits"
	"net/http"
	"net/http/httptrace"
	"reflect"
	"strings"
	"testing"
)

func TestAgentPlatformPayload(t *testing.T) {
	p := &platformAgentExecutor{}
	endpoint, ctype, body, err := p.payload(agent.Invocation{Kind: "text", Model: "text", Input: []json.RawMessage{agent.UserItem("hello")}, Tools: true})
	if err != nil || endpoint != "/v1/responses" || ctype != "application/json" {
		t.Fatal(endpoint, err)
	}
	var v map[string]any
	_ = json.Unmarshal(body, &v)
	if v["parallel_tool_calls"] != false || v["max_output_tokens"] != float64(8192) || v["tools"] == nil {
		t.Fatal("missing safety configuration")
	}
	size, aspect := "4K", "16:9"
	endpoint, _, body, err = p.payload(agent.Invocation{Kind: "image", Model: "image", Args: &agent.ToolArgs{Prompt: "poster", Size: &size, Aspect: &aspect}})
	if err != nil || endpoint != "/v1/images/generations" {
		t.Fatal(endpoint, err)
	}
	_ = json.Unmarshal(body, &v)
	if v["size"] != "3840x2160" || v["n"] != float64(1) {
		t.Fatal("wrong dimensions")
	}
	endpoint, ctype, _, err = p.payload(agent.Invocation{Kind: "image", Model: "image", Args: &agent.ToolArgs{Prompt: "poster", Size: &size, Aspect: &aspect}, References: []agent.Asset{{Data: []byte("reference")}}})
	if err != nil || endpoint != "/v1/images/edits" || ctype[:19] != "multipart/form-data" {
		t.Fatal(endpoint, ctype, err)
	}
}
func TestAgentPlatformAccountingRecoveryNoDispatch(t *testing.T) {
	p := &platformAgentExecutor{}
	out, err := p.Recover(context.Background(), agent.Invocation{Kind: "text"}, agent.Outcome{Body: json.RawMessage(`{"status":"completed","output":[]}`), Cost: "0.1"})
	if err != nil || out.Cost != "0.1" {
		t.Fatal(err)
	}
}
func TestAgentResponseBound(t *testing.T) {
	w := &agentResponseWriter{headers: make(http.Header)}
	if _, err := w.Write(make([]byte, agentlimits.ImageResponseBytes+1)); err == nil || !w.overflow {
		t.Fatal("unbounded response")
	}
}

func TestAgentReasoningSummaryRequestIsPublicAndModelScoped(t *testing.T) {
	p := &platformAgentExecutor{}
	for _, model := range []string{"gpt-6.1-sol", "gpt-5.6-sol", "o3", "text", "gpt-4.1"} {
		_, _, body, err := p.payload(agent.Invocation{Kind: "text", Model: model, Input: []json.RawMessage{agent.UserItem("hello")}})
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err = json.Unmarshal(body, &value); err != nil {
			t.Fatal(err)
		}
		expected := model != "text" && model != "gpt-4.1"
		reasoning, ok := value["reasoning"].(map[string]any)
		if ok != expected || (ok && reasoning["summary"] != "auto") {
			t.Fatal("summary capability shape", model)
		}
	}
}

func TestAgentGatewayFailureRequiresPreSendProof(t *testing.T) {
	trace := &service.AgentInvocationTrace{}
	ctx := service.WithAgentInvocation(context.Background(), trace)
	trace.MarkDispatched()
	hooks := httptrace.ContextClientTrace(ctx)
	hooks.GetConn("provider.invalid:443")
	var failure *agent.InvocationFailure
	if err := agentGatewayFailure(trace); !errors.As(err, &failure) || failure.Dispatched || failure.Code != "AGENT_UPSTREAM_CONNECTION_FAILED" {
		t.Fatal("known connection failure not terminal", err)
	}
	hooks.WroteHeaders()
	if !errors.Is(agentGatewayFailure(trace), agent.ErrUnknown) {
		t.Fatal("post-write failure can be replayed")
	}
	if err := agentGatewayFailure(&service.AgentInvocationTrace{}); !errors.As(err, &failure) || failure.Dispatched {
		t.Fatal("preflight rejection changed")
	}
}

func TestAgentPayloadForcesAskUserWhenExplicitlyRequested(t *testing.T) {
	p := &platformAgentExecutor{}
	_, _, body, e := p.payload(agent.Invocation{Kind: "text", Model: "gpt-6.1-sol", ForceAskUser: true, Tools: true, ImageTools: true, Input: []json.RawMessage{agent.UserItem("请先问我几个问题")}})
	if e != nil {
		t.Fatal(e)
	}
	var value map[string]any
	_ = json.Unmarshal(body, &value)
	choice := value["tool_choice"].(map[string]any)
	if choice["type"] != "function" || choice["name"] != "ask_user" {
		t.Fatalf("unexpected tool choice: %#v", choice)
	}
}

func TestAgentPayloadUsesConfiguredReasoningEffort(t *testing.T) {
	p := &platformAgentExecutor{}
	_, _, body, e := p.payload(agent.Invocation{Kind: "text", Model: "gpt-6.1-sol", ReasoningEffort: "high", Input: []json.RawMessage{agent.UserItem("hello")}})
	if e != nil {
		t.Fatal(e)
	}
	var value map[string]any
	_ = json.Unmarshal(body, &value)
	reasoning := value["reasoning"].(map[string]any)
	if reasoning["effort"] != "high" || reasoning["summary"] != "auto" {
		t.Fatalf("unexpected reasoning payload: %#v", reasoning)
	}
}

func TestAgentPayloadCarriesVerifiedConversationMemory(t *testing.T) {
	p := &platformAgentExecutor{}
	m := &agent.ConversationMemory{ConversationID: "conv-test", ConfirmedImages: 1, Images: []agent.ImageMemory{{OperationID: "op-test", Status: "succeeded", AssetID: "asset-confirmed", Availability: "ready"}}}
	_, _, body, e := p.payload(agent.Invocation{Kind: "text", Model: "gpt-6.1-sol", Input: []json.RawMessage{agent.UserItem("刚才你生成了什么")}, Memory: m})
	if e != nil {
		t.Fatal(e)
	}
	var value map[string]any
	_ = json.Unmarshal(body, &value)
	instructions, _ := value["instructions"].(string)
	if !strings.Contains(instructions, "asset-confirmed") || !strings.Contains(instructions, "不能因为本轮未调用工具") {
		t.Fatal("missing verified memory instructions")
	}
	if !strings.Contains(instructions, "用户仅回顾结果时") {
		t.Fatal("recall may dispatch another paid image")
	}
}

func TestAgentAnsweredQuestionContinuesOriginalTask(t *testing.T) {
	p := &platformAgentExecutor{}
	_, _, body, err := p.payload(agent.Invocation{Kind: "text", Model: "text", ClarificationAnswered: true, Tools: true})
	if err != nil {
		t.Fatal(err)
	}
	var request map[string]any
	if err = json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	instructions, _ := request["instructions"].(string)
	if !strings.Contains(instructions, "继续完成原始请求") || !strings.Contains(instructions, "不要重复询问已回答字段") {
		t.Fatal("missing answer continuation contract")
	}
	if request["tool_choice"] != "auto" {
		t.Fatal("answered run must not force another question")
	}
}

func TestAgentCapabilitiesAndPayloadPreserveGPT61Maximum(t *testing.T) {
	p := &platformAgentExecutor{}
	key := &service.APIKey{Group: &service.Group{Platform: service.PlatformOpenAI}}
	caps, err := p.reasoningCapabilities(context.Background(), key, []string{"gpt-6.1-sol"})
	if err != nil || len(caps) != 1 {
		t.Fatal(caps, err)
	}
	if !reflect.DeepEqual(caps[0].ReasoningLevels, []string{"low", "medium", "high", "xhigh", "max"}) {
		t.Fatal(caps[0])
	}
	for _, effort := range []string{"xhigh", "max"} {
		in := agent.Invocation{Kind: "text", Model: "gpt-6.1-sol", ReasoningEffort: effort}
		_, _, body, err := p.invocationPayload(context.Background(), key, &in)
		if err != nil {
			t.Fatal(err)
		}
		var request struct {
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
		}
		if err = json.Unmarshal(body, &request); err != nil || request.Reasoning.Effort != effort {
			t.Fatal(request, err)
		}
	}
	in := agent.Invocation{Kind: "text", Model: "gpt-6.1-sol", ReasoningEffort: "none"}
	if _, _, _, err = p.invocationPayload(context.Background(), key, &in); err == nil {
		t.Fatal("unsupported GPT61 effort silently enabled")
	}
}

func TestAgentRuntimeModelIdentityUsesConfiguredIDWithoutClaimingProviderIdentity(t *testing.T) {
	p := &platformAgentExecutor{}
	for _, model := range []string{"deepseek-v4.1-flash", "gpt-6.1-sol", "custom-\"label\""} {
		_, _, body, err := p.payload(agent.Invocation{Kind: "text", Model: model})
		if err != nil {
			t.Fatal(err)
		}
		var request struct {
			Instructions string `json:"instructions"`
		}
		if err = json.Unmarshal(body, &request); err != nil {
			t.Fatal(err)
		}
		meta, _ := json.Marshal(map[string]string{"selected_model_id": model})
		if !strings.Contains(request.Instructions, string(meta)) || !strings.Contains(request.Instructions, "不能独立证明上游真实底层型号") {
			t.Fatal("missing bounded runtime identity context")
		}
	}
}
