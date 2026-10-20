package handler

import (
	"bytes"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/agent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/tidwall/gjson"
	"strings"
)

// Consume complete SSE frames independently of network/UTF-8 write boundaries.
// Only text and public reasoning summaries are projected; encrypted/analysis
// payloads remain in the private provider checkpoint.
func (w *agentResponseWriter) consumeStream(chunk []byte) {
	w.pending = append(w.pending, chunk...)
	for {
		cut := bytes.Index(w.pending, []byte("\n\n"))
		width := 2
		if cr := bytes.Index(w.pending, []byte("\r\n\r\n")); cr >= 0 && (cut < 0 || cr < cut) {
			cut = cr
			width = 4
		}
		if cut < 0 {
			return
		}
		frame := w.pending[:cut]
		w.pending = w.pending[cut+width:]
		var raw []byte
		for _, line := range bytes.Split(frame, []byte("\n")) {
			if bytes.HasPrefix(line, []byte("data:")) {
				data := bytes.TrimSpace(line[5:])
				if raw == nil {
					raw = data
				} else {
					raw = append(append(append([]byte(nil), raw...), '\n'), data...)
				}
			}
		}
		if len(raw) == 0 {
			continue
		}
		if w.chatState != nil {
			if bytes.Equal(raw, []byte("[DONE]")) {
				w.finishChatStream()
				continue
			}
			var chunk apicompat.ChatCompletionsChunk
			if json.Unmarshal(raw, &chunk) != nil || gjson.GetBytes(raw, "error").Exists() {
				w.streamErr = agent.ErrUnknown
				return
			}
			for _, event := range apicompat.ChatCompletionsChunkToResponsesEvents(&chunk, w.chatState) {
				data, _ := json.Marshal(event)
				w.consumeResponseData(data)
			}
			continue
		}
		if bytes.Equal(raw, []byte("[DONE]")) {
			continue
		}
		w.consumeResponseData(raw)
		if len(w.pending) == 0 {
			w.pending = nil
		}
	}
}
func (w *agentResponseWriter) finishChatStream() {
	if w.chatState.FinishReason == "" || w.chatState.ValidateToolCallArguments() != nil {
		w.streamErr = agent.ErrUnknown
		return
	}
	for _, event := range apicompat.FinalizeChatCompletionsResponsesStream(w.chatState) {
		data, _ := json.Marshal(event)
		w.consumeResponseData(data)
	}
}
func (w *agentResponseWriter) consumeResponseData(raw []byte) {
	var kind struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &kind) != nil {
		return
	}
	if kind.Type == "image_generation.completed" || kind.Type == "image_edit.completed" {
		release, err := agent.AcquireImageMemory(w.ctx)
		if err != nil {
			w.streamErr = err
			return
		}
		defer release()
	}
	if kind.Type == "image_generation.partial_image" || kind.Type == "image_edit.partial_image" {
		return
	}
	var event struct {
		Type     string            `json:"type"`
		Delta    string            `json:"delta"`
		Response json.RawMessage   `json:"response"`
		B64      string            `json:"b64_json"`
		URL      string            `json:"url"`
		Data     []json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return
	}
	changed := false
	switch event.Type {
	case "response.output_text.delta":
		w.streamText += event.Delta
		changed = true
	case "response.reasoning_summary_text.delta":
		w.streamSummary += event.Delta
		changed = true
	case "response.reasoning_summary_part.added":
		if w.streamSummary != "" {
			w.streamSummary += "\n\n"
		}
	case "response.completed", "response.incomplete", "response.failed":
		if json.Valid(event.Response) {
			w.completed = append(json.RawMessage(nil), event.Response...)
		}
	case "image_generation.completed", "image_edit.completed":
		if event.B64 != "" {
			w.completed, _ = json.Marshal(map[string]any{"data": []any{map[string]string{"b64_json": event.B64}}})
		} else if event.URL != "" {
			w.completed, _ = json.Marshal(map[string]any{"data": []any{map[string]string{"url": event.URL}}})
		} else if len(event.Data) > 0 {
			w.completed, _ = json.Marshal(map[string]any{"data": event.Data})
		}
	}
	if len(w.streamText) > 128<<10 || len(w.streamSummary) > 48<<10 {
		w.streamErr = agent.Invalid("AGENT_STREAM_LIMIT")
		return
	}
	if changed && w.onProgress != nil && w.streamErr == nil {
		w.streamErr = w.onProgress(agent.StreamUpdate{Text: w.streamText, Summary: w.streamSummary})
	}
}
func (w *agentResponseWriter) resultBody() json.RawMessage {
	if !strings.Contains(w.headers.Get("Content-Type"), "text/event-stream") {
		return bytes.TrimSpace(w.body.Bytes())
	}
	if len(w.completed) == 0 {
		return nil
	}
	// Some compatible providers emit summaries only as deltas and omit them from
	// response.completed. Retain the public stream rather than dropping it.
	if w.streamSummary != "" {
		parsed, err := agent.ParseResponse(w.completed)
		if err == nil && parsed.Summary == "" {
			var response map[string]json.RawMessage
			if json.Unmarshal(w.completed, &response) == nil {
				items := parsed.Items
				summary, _ := json.Marshal(map[string]any{"type": "reasoning", "summary": []any{map[string]string{"type": "summary_text", "text": w.streamSummary}}})
				items = append(items, summary)
				response["output"], _ = json.Marshal(items)
				normalized, _ := json.Marshal(response)
				return normalized
			}
		}
	}
	return w.completed
}
