package service

import (
	"encoding/json"
	apicompat "github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

// Only text, thinking or actual tool parameters count; role/start/usage/signatures do not.
func openAIAnthropicEventHasContent(event *apicompat.AnthropicStreamEvent) bool {
	if event == nil {
		return false
	}
	if event.Type == "content_block_delta" && event.Delta != nil {
		d := event.Delta
		return d.Text != "" || d.Thinking != "" || d.PartialJSON != ""
	}
	if event.Type == "content_block_start" && event.ContentBlock != nil {
		b := event.ContentBlock
		return b.Text != "" || b.Thinking != ""
	}
	return false
}
func openAIAnthropicDataHasContent(data string) bool {
	var event apicompat.AnthropicStreamEvent
	return json.Unmarshal([]byte(data), &event) == nil && openAIAnthropicEventHasContent(&event)
}
