package agent

import (
	"strings"
	"testing"
)

func TestAgentConversationMemoryWithoutImagesStaysGeneral(t *testing.T) {
	text := MemoryInstructions(&ConversationMemory{ConversationID: "owned-conversation", MessageCount: 5})
	if !strings.Contains(text, "当前问题属于普通聊天时，直接回答") || !strings.Contains(text, "禁止编造 ID") {
		t.Fatal("missing generic memory grounding")
	}
	for _, phrase := range []string{"用户仅回顾结果时", "succeeded 图片操作证明图片曾经生成", "image_content_observed_by_model=false 表示"} {
		if strings.Contains(text, phrase) {
			t.Fatal("empty history injected visual-task instructions", phrase)
		}
	}
	if MemoryInstructions(nil) != "" {
		t.Fatal("nil memory unexpectedly adds instructions")
	}
}
func TestAgentConversationMemoryWithImagesRetainsProvenance(t *testing.T) {
	text := MemoryInstructions(&ConversationMemory{ConfirmedImages: 1, Images: []ImageMemory{{AssetID: "existing", Status: "succeeded", Availability: "expired", ObservedByModel: false}}})
	for _, phrase := range []string{"用户仅回顾结果时", "不能因为本轮未调用工具", "不代表图片未生成", "existing", "expired"} {
		if !strings.Contains(text, phrase) {
			t.Fatal("verified provenance rule missing", phrase)
		}
	}
}
