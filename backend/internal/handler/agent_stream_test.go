package handler

import (
	"bytes"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/agent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestAgentStreamFragmentedPublicProjection(t *testing.T) {
	var updates []agent.StreamUpdate
	w := &agentResponseWriter{headers: make(http.Header), onProgress: func(u agent.StreamUpdate) error { updates = append(updates, u); return nil }}
	w.headers.Set("Content-Type", "text/event-stream")
	data := "data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"正在整理需求\"}\r\n\r\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"你好\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"你好\"}]}]}}\n\n"
	for _, b := range []byte(data) {
		if _, e := w.Write([]byte{b}); e != nil {
			t.Fatal(e)
		}
	}
	if len(updates) != 2 || updates[1].Text != "你好" || updates[1].Summary != "正在整理需求" {
		t.Fatal(updates)
	}
	r, e := agent.ParseResponse(w.resultBody())
	if e != nil || r.Text != "你好" || r.Summary != "正在整理需求" {
		t.Fatal(r, e)
	}
}
func TestAgentStreamRequiresFinalEvidence(t *testing.T) {
	w := &agentResponseWriter{headers: make(http.Header)}
	w.headers.Set("Content-Type", "text/event-stream")
	_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"))
	if len(w.resultBody()) != 0 {
		t.Fatal("partial stream incorrectly became a completed request")
	}
}
func TestAgentImageStreamingAndPayloads(t *testing.T) {
	w := &agentResponseWriter{headers: make(http.Header)}
	w.headers.Set("Content-Type", "text/event-stream")
	_, _ = w.Write([]byte("data: {\"type\":\"image_generation.partial_image\",\"b64_json\":\"preview\"}\n\ndata: {\"type\":\"image_generation.completed\",\"b64_json\":\"final\"}\n\n"))
	if !bytes.Contains(w.resultBody(), []byte("final")) || bytes.Contains(w.resultBody(), []byte("preview")) {
		t.Fatal("wrong image evidence")
	}
	p := &platformAgentExecutor{}
	for _, kind := range []string{"text", "image"} {
		size, aspect := "1K", "1:1"
		_, _, raw, e := p.payload(agent.Invocation{Kind: kind, Model: "gpt-6.1-sol", Tools: true, Args: &agent.ToolArgs{Prompt: "cat", Size: &size, Aspect: &aspect}})
		if e != nil {
			t.Fatal(e)
		}
		var b map[string]any
		_ = json.Unmarshal(raw, &b)
		if b["stream"] != true {
			t.Fatal("non-streaming invocation", kind)
		}
		if kind == "text" && len(b["tools"].([]any)) != 7 {
			t.Fatal("expected communication, draft and inspection tools without image binding")
		}
	}
}

func TestAgentGeminiChatBridgeStreamsToolsAndRoundTripsResults(t *testing.T) {
	for _, tool := range []string{"ask_user", "generate_image", "save_work_plan"} {
		t.Run(tool, func(t *testing.T) {
			w := &agentResponseWriter{headers: make(http.Header), chatState: apicompat.NewChatCompletionsToResponsesStreamState("gemini-test")}
			w.headers.Set("Content-Type", "text/event-stream")
			arguments := `{"question":"主题是什么？","fields":[]}`
			if tool == "generate_image" {
				arguments = `{"prompt":"cat","model":null,"image_size":"1K","aspect_ratio":"1:1","reference_asset_ids":null}`
			}
			frame, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "call_1", "type": "function", "function": map[string]any{"name": tool, "arguments": arguments}}}}, "finish_reason": "tool_calls"}}})
			data := append([]byte("data: "), frame...)
			data = append(data, []byte("\n\ndata: [DONE]\n\n")...)
			for _, b := range data {
				_, err := w.Write([]byte{b})
				require.NoError(t, err)
			}
			parsed, err := agent.ParseResponse(w.resultBody())
			require.NoError(t, err)
			require.Len(t, parsed.Calls, 1)
			require.Equal(t, tool, parsed.Calls[0].Name)
			require.Equal(t, arguments, parsed.Calls[0].Arguments)
			history := append([]json.RawMessage{agent.UserItem("请协助我")}, parsed.Items...)
			history = append(history, json.RawMessage(`{"type":"function_call_output","call_id":"call_1","output":"{\"answer\":\"cat\"}"}`))
			p := &platformAgentExecutor{}
			_, _, body, err := p.payload(agent.Invocation{Kind: "text", Model: "gemini-test", Tools: true, ImageTools: true, Input: history})
			require.NoError(t, err)
			var request apicompat.ResponsesRequest
			require.NoError(t, json.Unmarshal(body, &request))
			chat, err := apicompat.ResponsesToChatCompletionsRequest(&request)
			require.NoError(t, err)
			require.Len(t, chat.Tools, 8)
			require.Equal(t, "tool", chat.Messages[len(chat.Messages)-1].Role)
			require.Equal(t, "call_1", chat.Messages[len(chat.Messages)-1].ToolCallID)
			claude, err := apicompat.ResponsesToAnthropicRequest(&request)
			require.NoError(t, err)
			require.Len(t, claude.Tools, 8)
		})
	}
}
func TestAgentGeminiChatBridgeRejectsUnfinishedOrMalformedCalls(t *testing.T) {
	for _, data := range []string{`data: {"choices":[{"delta":{"content":"partial"}}]}` + "\n\ndata: [DONE]\n\n", `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"ask_user","arguments":"{"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n"} {
		w := &agentResponseWriter{headers: make(http.Header), chatState: apicompat.NewChatCompletionsToResponsesStreamState("gemini-test")}
		w.headers.Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(data))
		require.Error(t, w.streamErr)
		require.Empty(t, w.completed)
	}
}
