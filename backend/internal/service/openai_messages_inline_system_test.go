package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardAsAnthropic_InlineSystemKeepsTaskAndToolResult(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const lead = "Use verified data."
	const userTask = "What is the invoice total?"
	const beforeTool = "Read the invoice first."
	const afterTool = "Report the total with its currency."
	const toolID = "toolu_invoice"
	const toolOutput = "Total: USD 137.42"
	const upstreamModel = "gpt-5.3-codex"
	const cacheKey = "invoice-session"
	const priorResponse = "resp_previous_invoice"

	for _, tc := range []struct {
		name      string
		cached    bool
		long      bool
		codex     bool
		useSystem bool
	}{
		{name: "warm Responses with inline system", cached: true, useSystem: true},
		{name: "warm Responses without inline system", cached: true},
		{name: "long uncached Responses with inline system", long: true, useSystem: true},
		{name: "Codex forced instructions template", codex: true, useSystem: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages := []string{`{"role":"user","content":"` + userTask + `"}`}
			if tc.long {
				for range openAICompatAnthropicReplayMaxTailMessages {
					messages = append(messages, `{"role":"assistant","content":"Checking."}`, `{"role":"user","content":"Please continue."}`)
				}
			}
			if tc.useSystem {
				messages = append(messages, `{"role":"system","content":[{"type":"text","text":"`+beforeTool+`"}]}`)
			}
			messages = append(messages,
				`{"role":"assistant","content":[{"type":"tool_use","id":"`+toolID+`","name":"Read","input":{"path":"invoice.txt"}}]}`,
				`{"role":"user","content":[{"type":"tool_result","tool_use_id":"`+toolID+`","content":"`+toolOutput+`"}]}`,
			)
			if tc.useSystem {
				messages = append(messages, `{"role":"system","content":"`+afterTool+`"}`)
			}
			body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":64,"system":"` + lead +
				`","tools":[{"name":"Read","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}}],` +
				`"messages":[` + strings.Join(messages, ",") + `]}`)

			// Stub the model: the regression asserts the actual outbound request,
			// not whether a real model happens to produce the expected words.
			resp := openAICompatSSECompletedResponse("resp_invoice", upstreamModel)
			data, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			resp.Body = io.NopCloser(strings.NewReader(strings.Replace(string(data), `"text":"ok"`, `"text":"USD 137.42"`, 1)))
			upstream := &httpUpstreamRecorder{resp: resp}
			svc := &OpenAIGatewayService{
				httpUpstream: upstream,
				cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
			}
			account := &Account{
				ID:          1,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Concurrency: 1,
				Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://api.openai.com/v1"},
			}
			if tc.codex {
				account.Type = AccountTypeOAuth
				account.Credentials = map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"}
				svc.cfg.Gateway.ForcedCodexInstructionsTemplate = "Gateway prefix\n\n{{ .ExistingInstructions }}"
			}
			ctx := context.Background()
			if tc.cached {
				svc.bindOpenAICompatSessionResponseID(ctx, nil, account, cacheKey, priorResponse)
			}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			result, err := svc.ForwardAsAnthropic(ctx, c, account, body, cacheKey, upstreamModel)
			require.NoError(t, err)
			require.NotNil(t, result)

			sent := gjson.ParseBytes(upstream.lastBody)
			input := sent.Get("input").Array()
			callIndex := 1 // one leading gateway guard when replaying a delta
			if tc.useSystem {
				require.False(t, sent.Get("previous_response_id").Exists())
				require.Len(t, input, len(messages)+2) // top-level system + gateway guard
				require.Equal(t, "developer", input[0].Get("role").String())
				require.Equal(t, lead, input[0].Get("content.0.text").String())
				require.Equal(t, "user", input[2].Get("role").String())
				require.Equal(t, userTask, input[2].Get("content.0.text").String())
				callIndex = len(input) - 3
				require.Equal(t, "developer", input[callIndex-1].Get("role").String())
				require.Equal(t, beforeTool, input[callIndex-1].Get("content.0.text").String())
				require.Equal(t, "developer", input[callIndex+2].Get("role").String())
				require.Equal(t, afterTool, input[callIndex+2].Get("content.0.text").String())
			} else {
				require.Equal(t, priorResponse, sent.Get("previous_response_id").String())
				require.Len(t, input, 3)
			}
			if tc.codex {
				require.Equal(t, "Gateway prefix\n\n"+lead, sent.Get("instructions").String())
			}
			require.Equal(t, "function_call", input[callIndex].Get("type").String())
			require.Equal(t, "Read", input[callIndex].Get("name").String())
			require.Equal(t, toolID, input[callIndex].Get("call_id").String())
			require.Equal(t, "function_call_output", input[callIndex+1].Get("type").String())
			require.Equal(t, toolID, input[callIndex+1].Get("call_id").String())
			require.Equal(t, toolOutput, input[callIndex+1].Get("output").String())
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, "end_turn", gjson.GetBytes(rec.Body.Bytes(), "stop_reason").String())
			require.Equal(t, "USD 137.42", gjson.GetBytes(rec.Body.Bytes(), "content.0.text").String())
		})
	}
}
