//go:build unit

package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/agent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/tidwall/gjson"
)

func visionReference(id string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{"role": "user", "content": []map[string]string{{"type": "input_text", "text": "read image"}, {"type": "input_image", "image_url": agent.ImageAssetPrefix + id}}})
	return raw
}
func TestAgentVisionPayloadHasPixelsAcrossProtocols(t *testing.T) {
	pixels := []byte("synthetic pixels")
	in := agent.Invocation{Kind: "text", Model: "vision-model", Input: []json.RawMessage{visionReference("a")}, VisionInputs: []agent.Asset{{ID: "a", MIME: "image/jpeg", Data: pixels}}}
	_, _, body, err := (&platformAgentExecutor{}).payload(in)
	if err != nil {
		t.Fatal(err)
	}
	data := gjson.GetBytes(body, "input.0.content.1.image_url").String()
	want := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(pixels)
	if data != want {
		t.Fatal("actual image bytes missing")
	}
	for _, platform := range domain.ConcretePlatformIDs() {
		if platform == "typesafe" {
			continue
		}
		t.Run(platform, func(t *testing.T) {
			converted, _, err := prepareAgentTextBody(body, platform, "", false)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(converted, []byte(want)) {
				t.Fatal("protocol converter dropped image input")
			}
			size := agentVisionQuoteSize(in, converted)
			if size < 65536 || size > 65536+len(converted) {
				t.Fatal("invalid image quote size", size)
			}
		})
	}
	// Large image payload is not counted as base64 text tokens.
	in.VisionInputs[0].Data = bytes.Repeat([]byte{0xff}, 512<<10)
	_, _, body, err = (&platformAgentExecutor{}).payload(in)
	if err != nil {
		t.Fatal(err)
	}
	size := agentVisionQuoteSize(in, body)
	if size < 65536 || size > 100000 || len(body) < 600000 {
		t.Fatal("base64 inflated text quote", size, len(body))
	}
}
func TestAgentVisionExpansionDropsUnavailableAndDuplicateReferences(t *testing.T) {
	input := []json.RawMessage{visionReference("old"), visionReference("a"), visionReference("a")}
	original := string(input[1])
	result, count, err := expandAgentVisionInput(input, []agent.Asset{{ID: "a", MIME: "image/jpeg", Data: []byte("pixels")}})
	if err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if strings.Contains(string(result[0]), "input_image") || strings.Contains(string(result[1]), "input_image") || !strings.Contains(string(result[2]), "data:image/jpeg") {
		t.Fatal("duplicate or absent reference not replaced")
	}
	if string(input[1]) != original {
		t.Fatal("persistent history mutated")
	}
	bad := json.RawMessage(`{"role":"user","content":[{"type":"input_image","image_url":"http://127.0.0.1/private"}]}`)
	if _, _, err = expandAgentVisionInput([]json.RawMessage{bad}, nil); err == nil {
		t.Fatal("external image URL accepted")
	}
}
