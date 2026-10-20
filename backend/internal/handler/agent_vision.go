package handler

import (
	"encoding/base64"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/agent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"strings"
)

func expandAgentVisionInput(input []json.RawMessage, assets []agent.Asset) ([]json.RawMessage, int, error) {
	images := map[string]string{}
	for _, a := range assets {
		images[a.ID] = "data:" + a.MIME + ";base64," + base64.StdEncoding.EncodeToString(a.Data)
	}
	result := append([]json.RawMessage(nil), input...)
	seen := map[string]bool{}
	count := 0
	for i := len(input) - 1; i >= 0; i-- {
		var item map[string]json.RawMessage
		if json.Unmarshal(input[i], &item) != nil {
			continue
		}
		var role string
		_ = json.Unmarshal(item["role"], &role)
		if role != "user" {
			continue
		}
		var parts []map[string]json.RawMessage
		if json.Unmarshal(item["content"], &parts) != nil {
			continue
		}
		changed := false
		for j := len(parts) - 1; j >= 0; j-- {
			var typ, url string
			_ = json.Unmarshal(parts[j]["type"], &typ)
			_ = json.Unmarshal(parts[j]["image_url"], &url)
			if typ != "input_image" {
				continue
			}
			if !strings.HasPrefix(url, agent.ImageAssetPrefix) {
				return nil, 0, agent.Invalid("AGENT_INVALID_IMAGE_REFERENCE")
			}
			id := strings.TrimPrefix(url, agent.ImageAssetPrefix)
			changed = true
			if data, ok := images[id]; ok && !seen[id] {
				parts[j]["image_url"], _ = json.Marshal(data)
				seen[id] = true
				count++
			} else {
				raw, _ := json.Marshal(map[string]string{"type": "input_text", "text": "此历史图片未在本轮重新提供像素，不能据此编造图片内容。"})
				_ = json.Unmarshal(raw, &parts[j])
			}
		}
		if changed {
			item["content"], _ = json.Marshal(parts)
			result[i], _ = json.Marshal(item)
		}
	}
	return result, count, nil
}

// Budget images as reserved multimodal input, never as millions of base64 text
// tokens. Tariff resolution and final billing remain in the existing gateway.
func agentVisionQuoteSize(in agent.Invocation, body []byte) int {
	return max(0, len(body)-agentVisionEncodedBytes(in)) + service.AgentVisionInputReserve*len(in.VisionInputs)
}
func agentVisionEncodedBytes(in agent.Invocation) int {
	size := 0
	for _, a := range in.VisionInputs {
		size += len("data:"+a.MIME+";base64,") + base64.StdEncoding.EncodedLen(len(a.Data))
	}
	return size
}
