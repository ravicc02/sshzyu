//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/png"
	"io"
	"net/http"
	"testing"
)

func agentResultPNG() []byte {
	var out bytes.Buffer
	_ = png.Encode(&out, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	return out.Bytes()
}
func TestAgentImageResultLargePNGDoesNotUseBatchScannerLimit(t *testing.T) {
	base := agentResultPNG()
	payload := make([]byte, 21<<20)
	kind := []byte("vpAg")
	var chunk bytes.Buffer
	_ = binary.Write(&chunk, binary.BigEndian, uint32(len(payload)))
	chunk.Write(kind)
	chunk.Write(payload)
	hash := crc32.NewIEEE()
	_, _ = hash.Write(kind)
	_, _ = hash.Write(payload)
	_ = binary.Write(&chunk, binary.BigEndian, hash.Sum32())
	data := append(append(append([]byte{}, base[:len(base)-12]...), chunk.Bytes()...), base[len(base)-12:]...)
	raw, _ := json.Marshal(map[string]any{"data": []any{map[string]string{"b64_json": base64.StdEncoding.EncodeToString(data)}}})
	got, err := ResolveAgentImageResult(context.Background(), raw)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("large generated image was rejected", err)
	}
	p := &OpenAIBatchImageProvider{}
	if _, err = p.decodeImageResponse(context.Background(), &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(raw))}); err == nil {
		t.Fatal("public JSONL batch limits changed")
	}
}
func TestAgentImageResultRejectsInvalidAndUnsafeResults(t *testing.T) {
	for _, raw := range []string{`{}`, `{"data":[]}`, `{"data":[{},{}]}`, `{"data":[{"b64_json":"invalid"}]}`, `{"data":[{"b64_json":"aGVsbG8="}]}`} {
		if _, err := ResolveAgentImageResult(context.Background(), json.RawMessage(raw)); !errors.Is(err, ErrAgentImageResultInvalid) {
			t.Fatal(raw, err)
		}
	}
	for _, url := range []string{"http://127.0.0.1/image.png", "https://127.0.0.1/image.png", "https://169.254.169.254/image.png", "https://u:p@example.com/image.png"} {
		raw, _ := json.Marshal(map[string]any{"data": []any{map[string]string{"url": url}}})
		if _, err := ResolveAgentImageResult(context.Background(), raw); err == nil {
			t.Fatal("SSRF image URL accepted")
		}
	}
}
