package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAnthropicMidConversationSystemPreservesToolTurn(t *testing.T) {
	const body = `{
		"model":"gpt-5.2","max_tokens":64,"system":"Use verified data.",
		"messages":[
			{"role":"user","content":"What is the invoice total?"},
			{"role":"system","content":[{"type":"text","text":"Read the invoice first."}]},
			{"role":"assistant","content":[{"type":"tool_use","id":"toolu_invoice","name":"Read","input":{"path":"invoice.txt"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_invoice","content":[{"type":"text","text":"Total: USD 137.42"}]}]},
			{"role":"system","content":"Report the total with its currency."}
		]
	}`
	var req AnthropicRequest
	require.NoError(t, json.Unmarshal([]byte(body), &req))

	t.Run("Responses", func(t *testing.T) {
		out, err := AnthropicToResponses(&req)
		require.NoError(t, err)
		input := gjson.ParseBytes(out.Input).Array()
		require.Len(t, input, 6)
		require.Equal(t, "developer", input[0].Get("role").String())
		require.Equal(t, "Use verified data.", input[0].Get("content.0.text").String())
		require.Equal(t, "user", input[1].Get("role").String())
		require.Equal(t, "What is the invoice total?", input[1].Get("content.0.text").String())
		require.Equal(t, "developer", input[2].Get("role").String())
		require.Equal(t, "Read the invoice first.", input[2].Get("content.0.text").String())
		require.Equal(t, "function_call", input[3].Get("type").String())
		require.Equal(t, "Read", input[3].Get("name").String())
		require.JSONEq(t, `{"path":"invoice.txt"}`, input[3].Get("arguments").String())
		require.Equal(t, "function_call_output", input[4].Get("type").String())
		require.Equal(t, "toolu_invoice", input[3].Get("call_id").String())
		require.Equal(t, "toolu_invoice", input[4].Get("call_id").String())
		require.Equal(t, "Total: USD 137.42", input[4].Get("output").String())
		require.Equal(t, "developer", input[5].Get("role").String())
		require.Equal(t, "Report the total with its currency.", input[5].Get("content.0.text").String())
	})

	t.Run("Chat Completions", func(t *testing.T) {
		out, err := AnthropicToChatCompletionsRequest(&req)
		require.NoError(t, err)
		messages := out.Messages
		require.Len(t, messages, 6)
		require.Equal(t, "system", messages[0].Role)
		require.JSONEq(t, `"Use verified data."`, string(messages[0].Content))
		require.Equal(t, "user", messages[1].Role)
		require.JSONEq(t, `"What is the invoice total?"`, string(messages[1].Content))
		require.Equal(t, "system", messages[2].Role)
		require.JSONEq(t, `"Read the invoice first."`, string(messages[2].Content))
		require.Equal(t, "assistant", messages[3].Role)
		require.Len(t, messages[3].ToolCalls, 1)
		require.Equal(t, "toolu_invoice", messages[3].ToolCalls[0].ID)
		require.Equal(t, "Read", messages[3].ToolCalls[0].Function.Name)
		require.JSONEq(t, `{"path":"invoice.txt"}`, messages[3].ToolCalls[0].Function.Arguments)
		require.Equal(t, "tool", messages[4].Role)
		require.Equal(t, "toolu_invoice", messages[4].ToolCallID)
		require.JSONEq(t, `"Total: USD 137.42"`, string(messages[4].Content))
		require.Equal(t, "system", messages[5].Role)
		require.JSONEq(t, `"Report the total with its currency."`, string(messages[5].Content))
	})
}
