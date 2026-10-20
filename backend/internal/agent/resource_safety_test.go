package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAgentCheckpointStreamingRoundTrip(t *testing.T) {
	s := New(nil, nil, t.TempDir(), false)
	for _, embedded := range []bool{false, true} {
		out := Outcome{Body: json.RawMessage(`{"data":[{"b64_json":"AQID"}]}`), Image: []byte{1, 2, 3}, Cost: "0.17", AccountID: 12, ImageFromBody: embedded}
		if err := s.writeOutcome("op", out); err != nil {
			t.Fatal(err)
		}
		got, err := s.readOutcome("op")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.Body, out.Body) || got.Cost != out.Cost || got.AccountID != out.AccountID || got.ImageFromBody != embedded {
			t.Fatal(got)
		}
		if embedded && len(got.Image) > 0 {
			t.Fatal("duplicate embedded image checkpoint")
		}
		if !embedded && !bytes.Equal(got.Image, out.Image) {
			t.Fatal("URL image omitted")
		}
		info, _ := os.Stat(filepath.Join(s.Root, "op.result.json"))
		if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			t.Fatal("public checkpoint")
		}
	}
}
func TestAgentStreamAssetIntegrityAndOwnership(t *testing.T) {
	store := isolatedStore(t)
	s, run := setupRun(t, store, &testExecutor{})
	ctx := context.Background()
	a, err := s.UploadFile(ctx, 1, run.ConversationID, "reference.png", testImage())
	if err != nil {
		t.Fatal(err)
	}
	meta, f, err := s.OpenAsset(ctx, 1, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil || !bytes.Equal(data, testImage()) || len(meta.Data) != 0 {
		t.Fatal("not an intact streamed file", err)
	}
	if _, f, err = s.OpenAsset(ctx, 2, a.ID); !errors.Is(err, ErrNotFound) || f != nil {
		t.Fatal("cross-user asset read", err)
	}
	if err = os.WriteFile(filepath.Join(s.Root, a.Key), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, f, err = s.OpenAsset(ctx, 1, a.ID); SafeCode(err) != "AGENT_ASSET_INTEGRITY_FAILED" || f != nil {
		t.Fatal("corrupt response sent", err)
	}
}
func TestAgentDisabledStillCleansFilesAndStaleStreamEvents(t *testing.T) {
	store := isolatedStore(t)
	s, r := setupRun(t, store, &testExecutor{})
	ctx := context.Background()
	a, err := s.UploadFile(ctx, 1, r.ConversationID, "a.png", testImage())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DB.Exec(`UPDATE agent_assets SET expires_at=now()-interval '1 hour' WHERE id=$1`, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DB.Exec(`UPDATE agent_runs SET status='completed' WHERE id=$1`, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DB.Exec(`INSERT INTO agent_run_events(run_id,sequence,type,payload,created_at)VALUES($1,901,'model.delta','{}',now()-interval '25 hours'),($1,902,'run.completed','{}',now()-interval '25 hours'),($1,903,'tool.call.created','{}',now()-interval '25 hours')`, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	s.Enabled = false
	s.maintenanceInterval = 10 * time.Millisecond
	s.Start()
	defer s.Close()
	until := time.Now().Add(3 * time.Second)
	for s.CleanupLastSuccess() == 0 && time.Now().Before(until) {
		time.Sleep(10 * time.Millisecond)
	}
	if s.CleanupLastSuccess() == 0 || s.Accepting() {
		t.Fatal("maintenance coupled to accepting work")
	}
	if _, err = os.Stat(filepath.Join(s.Root, a.Key)); !os.IsNotExist(err) {
		t.Fatal("expired content retained", err)
	}
	var n int
	if err = store.DB.QueryRow(`SELECT count(*) FROM agent_run_events WHERE run_id=$1 AND sequence IN(901,902,903)`, r.ID).Scan(&n); err != nil || n != 1 {
		t.Fatal("stale delta not compacted or final event lost", n, err)
	}
}
func TestAgentGlobalStorageCapAndDiskPressure(t *testing.T) {
	store := isolatedStore(t)
	s, r := setupRun(t, store, &testExecutor{})
	ctx := context.Background()
	store.MaxStorageBytes = int64(len(testImage()))
	if _, err := s.UploadFile(ctx, 1, r.ConversationID, "one.png", testImage()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UploadFile(ctx, 1, r.ConversationID, "two.png", testImage()); SafeCode(err) != "AGENT_STORAGE_CAPACITY" {
		t.Fatal(err)
	}
	s.MinFreeDiskBytes = 1 << 62
	if _, err := s.UploadFile(ctx, 1, r.ConversationID, "three.png", testImage()); SafeCode(err) != "AGENT_DISK_PRESSURE" {
		t.Fatal(err)
	}
	var n int
	store.DB.QueryRow(`SELECT count(*) FROM agent_assets`).Scan(&n)
	if n != 1 {
		t.Fatal("rejected upload wrote metadata", n)
	}
}
func TestAgentThumbnailBoundedPrivateAndExpires(t *testing.T) {
	store := isolatedStore(t)
	s, r := setupRun(t, store, &testExecutor{})
	ctx := context.Background()
	var raw bytes.Buffer
	if err := png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 1200, 900))); err != nil {
		t.Fatal(err)
	}
	a, err := s.UploadFile(ctx, 1, r.ConversationID, "large.png", raw.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.Thumbnail(ctx, 1, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	cfg, kind, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || kind != "jpeg" || cfg.Width > 896 || cfg.Height > 896 {
		t.Fatal(cfg, kind, err)
	}
	if _, err = s.Thumbnail(ctx, 2, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("thumbnail crossed users", err)
	}
	if _, err = store.DB.Exec(`UPDATE agent_assets SET expires_at=now()-interval '1 hour' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Thumbnail(ctx, 1, a.ID); SafeCode(err) != "ASSET_EXPIRED" {
		t.Fatal("cached thumbnail outlived source", err)
	}
}
func TestAgentImageMaterializationCancellation(t *testing.T) {
	release, err := AcquireImageMemory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if unlock, err := AcquireImageMemory(ctx); !errors.Is(err, context.DeadlineExceeded) || unlock != nil {
		t.Fatal(err)
	}
}
func BenchmarkAgentOutcomeCheckpoint(b *testing.B) {
	s := New(nil, nil, b.TempDir(), false)
	body := json.RawMessage(`{"data":[{"b64_json":"` + strings.Repeat("A", 16<<20) + `"}]}`)
	out := Outcome{Body: body, Cost: "0.17000000", ImageFromBody: true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.writeOutcome("op", out); err != nil {
			b.Fatal(err)
		}
	}
}

func TestAgentEventReplayHasByteBudgetWithoutDroppingTerminalEvent(t *testing.T) {
	store := isolatedStore(t)
	_, r := setupRun(t, store, &testExecutor{})
	ctx := context.Background()
	payload, _ := json.Marshal(map[string]string{"text": strings.Repeat("x", 300<<10)})
	if _, err := store.DB.Exec(`UPDATE agent_runs SET status='completed' WHERE id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec(`INSERT INTO agent_run_events(run_id,sequence,type,payload) VALUES($1,1,'model.delta',$2),($1,2,'model.delta',$2),($1,3,'run.completed','{}')`, r.ID, payload); err != nil {
		t.Fatal(err)
	}
	first, status, err := store.PollEvents(ctx, 1, r.ID, 0)
	if err != nil || status != "completed" || len(first) != 1 || !first[0].HasMore {
		t.Fatal(len(first), status, err)
	}
	next, _, err := store.PollEvents(ctx, 1, r.ID, first[0].Sequence)
	if err != nil || len(next) != 2 || next[0].Sequence != 2 || next[1].Type != "run.completed" || next[1].HasMore {
		t.Fatal(next, err)
	}
}
