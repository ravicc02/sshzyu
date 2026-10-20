package agent

import (
	"encoding/json"
	"testing"
)

func TestBindingsAndBudget(t *testing.T) {
	good := Config{ModelKeyID: 1, Model: "text", ImageKeyID: 2, ImageModel: "image", Budget: "1.00"}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Config{
		{ModelKeyID: 1, Model: "text", ImageKeyID: 1, ImageModel: "image", Budget: "1"},
	} {
		if bad.Validate() == nil {
			t.Fatalf("accepted invalid configuration: %+v", bad)
		}
	}
}
func TestConfigIgnoresLegacyBudgets(t *testing.T) {
	for _, legacy := range []string{"", "0", "-1", "NaN", "999999"} {
		if err := (Config{ModelKeyID: 1, Model: "text", Budget: legacy}).Validate(); err != nil {
			t.Fatal(legacy, err)
		}
	}
}
func TestToolArgumentsStrict(t *testing.T) {
	good := `{"prompt":"一张海报","model":null,"image_size":"1K","aspect_ratio":"16:9","reference_asset_ids":null}`
	if _, err := ParseTool(json.RawMessage(good)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		`{"prompt":"test","api_key":"secret"}`, `{"prompt":"one","prompt":"two"}`,
		`{"prompt":"x","image_size":"8K"}`, `{"prompt":"x","reference_asset_ids":["a","b"]}`,
		good + `{}`, `{"prompt":""}`,
	} {
		if _, err := ParseTool(json.RawMessage(bad)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
func TestResponseProtocol(t *testing.T) {
	response := json.RawMessage(`{"status":"completed","output":[{"type":"reasoning","encrypted_content":"opaque"},{"type":"function_call","call_id":"call1","name":"generate_image","arguments":"{\"prompt\":\"cat\"}"}]}`)
	out, err := ParseResponse(response)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 2 || len(out.Calls) != 1 {
		t.Fatal("lost protocol output")
	}
	if _, err := ParseResponse(json.RawMessage(`{"status":"incomplete","output":[]}`)); err == nil {
		t.Fatal("accepted incomplete response")
	}
}
func TestBudgetBoundary(t *testing.T) {
	if !WithinBudget("0.17", "0.03", "0.20") {
		t.Fatal("rejected exact boundary")
	}
	if WithinBudget("0.17", "0.03000001", "0.20") {
		t.Fatal("overspent")
	}
}

func TestToolSchemaAndInvalidProtocol(t *testing.T) {
	var v map[string]any
	if e := json.Unmarshal(ToolDefinition(), &v); e != nil {
		t.Fatal(e)
	}
	if v["strict"] != true {
		t.Fatal("schema not strict")
	}
	for _, raw := range []string{`{`, `{"status":"completed","output":[4]}`, `{"status":"completed","output":[{"type":"function_call","name":"generate_image"}]}`} {
		if _, e := ParseResponse(json.RawMessage(raw)); e == nil {
			t.Fatal("bad protocol accepted")
		}
	}
	for _, c := range []Config{{Model: "x", Budget: "1"}, {ModelKeyID: 1, Model: "x", ImageKeyID: 2, Budget: "1"}} {
		if c.Validate() == nil {
			t.Fatal("bad binding accepted")
		}
	}
	if SafeCode(nil) != "" || SafeCode(Invalid("SAFE_CODE")) != "SAFE_CODE" || SafeCode(Invalid("unsafe details")) != "AGENT_EXECUTION_FAILED" {
		t.Fatal("unsafe error disclosure")
	}
	if (&InvocationFailure{Code: "AGENT_GATEWAY_REJECTED"}).Error() != "AGENT_GATEWAY_REJECTED" {
		t.Fatal("error code")
	}
}

func TestToolInputByteLimitAndStrictEnvelope(t *testing.T) {
	if _, err := ParseTool(json.RawMessage(`{"prompt":"x","aspect_ratio":"21:9"}`)); err == nil {
		t.Fatal("unsupported aspect")
	}
	huge := make([]byte, 65537)
	if err := StrictJSON(huge, new(ToolArgs)); err == nil {
		t.Fatal("unbounded JSON")
	}
	if err := (Config{ModelKeyID: 1, Model: "", Budget: "1"}).Validate(); err == nil {
		t.Fatal("missing model")
	}
}

func TestAgentConfigAcceptsAllPlatformReasoningLevels(t *testing.T) {
	for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"} {
		cfg := Config{ModelKeyID: 1, Model: "text", Budget: "1", ReasoningEffort: effort}
		if err := cfg.Validate(); err != nil {
			t.Fatal(effort, err)
		}
	}
	cfg := Config{ModelKeyID: 1, Model: "text", Budget: "1", ReasoningEffort: "ultra"}
	if cfg.Validate() == nil {
		t.Fatal("unknown reasoning level accepted")
	}
}
