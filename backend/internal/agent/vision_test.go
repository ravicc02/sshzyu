package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAgentVisionImageNormalization(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2400, 1200))
	src.Set(20, 20, color.NRGBA{R: 255, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, src); err != nil {
		t.Fatal(err)
	}
	data, err := visionJPEG(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	normalized, format, err := image.Decode(bytes.NewReader(data))
	if err != nil || format != "jpeg" || normalized.Bounds().Dx() != 1536 || normalized.Bounds().Dy() != 768 {
		t.Fatal("incorrect image normalization", format, err)
	}
	r, g, blue, _ := normalized.At(100, 100).RGBA()
	if r < 64000 || g < 64000 || blue < 64000 {
		t.Fatal("transparent pixels must use white background")
	}
	if _, err := visionJPEG([]byte("not an image")); err == nil {
		t.Fatal("invalid image accepted")
	}
}
func TestAgentVisionReferencesUseLastOccurrenceAndStayPrivate(t *testing.T) {
	item := func(id string) json.RawMessage {
		return jsonBytes(map[string]any{"role": "user", "content": []map[string]string{{"type": "input_image", "image_url": ImageAssetPrefix + id}}})
	}
	in := Invocation{Kind: "text", Input: []json.RawMessage{item("a"), item("b"), item("c"), item("a")}, VisionInputs: []Asset{{ID: "a", Data: []byte("private pixels")}}}
	if got := ImageAssetIDs(in.Input); !reflect.DeepEqual(got, []string{"b", "c", "a"}) {
		t.Fatal(got)
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "VisionInputs") || bytes.Contains(raw, []byte(base64.StdEncoding.EncodeToString([]byte("private pixels")))) {
		t.Fatal("image bytes leaked into persisted invocation")
	}
}
func TestPostgresVisionContextIsolationAndHistoricalFiles(t *testing.T) {
	st := isolatedStore(t)
	ctx := context.Background()
	svc := New(st, &testExecutor{}, t.TempDir(), true)
	cv, err := svc.Create(ctx, 1, "vision", Config{ModelKeyID: 1, Model: "text", Budget: "1"}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	a, err := svc.UploadFile(ctx, 1, cv.ID, "image.png", testImage())
	if err != nil {
		t.Fatal(err)
	}
	r, err := svc.Submit(ctx, 1, cv.ID, "vision-turn", "127.0.0.1", "read the image", []string{a.ID}, 1, "1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(r.History[0]), ImageAssetPrefix+a.ID) || strings.Contains(string(r.History[0]), "base64") {
		t.Fatal("context must persist reference only")
	}
	in := Invocation{Kind: "text", Input: r.History}
	if err = svc.loadVisionInputs(ctx, r, &in); err != nil || len(in.VisionInputs) != 1 || len(in.VisionInputs[0].Data) == 0 || in.VisionInputs[0].MIME != "image/jpeg" {
		t.Fatal("image pixels not loaded", err)
	}
	// The following turn reattaches the earlier image even with no new upload.
	history, _, err := svc.conversationContext(ctx, 1, cv.ID, "what color?", nil)
	if err != nil {
		t.Fatal(err)
	}
	follow := Invocation{Kind: "text", Input: history}
	next := &Run{UserID: 1, ConversationID: cv.ID}
	if err = svc.loadVisionInputs(ctx, next, &follow); err != nil || len(follow.VisionInputs) != 1 {
		t.Fatal("follow-up lost image", err)
	}
	foreign := Invocation{Kind: "text", Input: r.History}
	if err = svc.loadVisionInputs(ctx, &Run{UserID: 2, ConversationID: cv.ID, Attachments: []string{a.ID}}, &foreign); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign user image accepted", err)
	}
	if err = svc.loadVisionInputs(ctx, &Run{UserID: 1, ConversationID: "other"}, &foreign); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign conversation accepted", err)
	}
	// Missing historical pixels are omitted; missing current uploads fail clearly.
	if err = os.Remove(filepath.Join(svc.Root, a.Key)); err != nil {
		t.Fatal(err)
	}
	follow.VisionInputs = nil
	if err = svc.loadVisionInputs(ctx, next, &follow); err != nil || len(follow.VisionInputs) != 0 {
		t.Fatal("expired history blocked new turn", err)
	}
	if err = svc.loadVisionInputs(ctx, r, &in); !errors.Is(err, ErrNotFound) {
		t.Fatal("missing current attachment accepted", err)
	}
}
