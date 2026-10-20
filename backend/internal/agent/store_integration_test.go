package agent

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

type testExecutor struct {
	calls      int
	images     int
	unknown    bool
	failResume bool
}

func (f *testExecutor) Options(context.Context, int64, string) ([]KeyOption, error) { return nil, nil }
func (f *testExecutor) Models(context.Context, Binding, string) ([]string, error) {
	return []string{"text", "image"}, nil
}
func (f *testExecutor) Authorize(_ context.Context, b Binding, p, m string) (int64, error) {
	if b.UserID != 1 {
		return 0, ErrNotFound
	}
	if p == "image" && b.KeyID != 2 {
		return 0, ErrNotFound
	}
	if p == "text" && b.KeyID != 1 {
		return 0, ErrNotFound
	}
	return b.KeyID + 10, nil
}
func (f *testExecutor) Quote(_ context.Context, in Invocation) (string, error) {
	if in.Kind == "image" {
		return "0.20000000", nil
	}
	return "0.02000000", nil
}
func (f *testExecutor) Execute(_ context.Context, in Invocation) (Outcome, error) {
	f.calls++
	if in.Kind == "image" {
		f.images++
		if f.unknown {
			return Outcome{}, errors.New("transport failed")
		}
		return Outcome{Image: testImage(), Cost: "0.17000000"}, nil
	}
	if len(in.Input) > 1 {
		if f.failResume {
			return Outcome{}, errors.New("connection dropped")
		}
		return Outcome{Cost: "0.01000000", Body: json.RawMessage(`{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"图片已生成。"}]}]}`)}, nil
	}
	args := `{"prompt":"cat","model":null,"image_size":"1K","aspect_ratio":"1:1","reference_asset_ids":null}`
	body, _ := json.Marshal(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "function_call", "call_id": "call1", "name": "generate_image", "arguments": args}}})
	return Outcome{Cost: "0.01000000", Body: body}, nil
}
func testImage() []byte {
	var b bytes.Buffer
	m := image.NewRGBA(image.Rect(0, 0, 16, 16))
	m.Set(0, 0, color.RGBA{R: 40, G: 120, A: 255})
	_ = png.Encode(&b, m)
	return b.Bytes()
}
func isolatedStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("AGENT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AGENT_TEST_DATABASE_URL not set; isolated PostgreSQL only")
	}
	// Each case owns a distinct namespace, so fixture connections never reuse
	// relations or invalidate another case's schema during teardown.
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("isolated PostgreSQL tests require a URL")
	}
	schema := "agent_test_" + strings.ReplaceAll(ID("db"), "-", "")
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, e := sql.Open("postgres", parsed.String())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	if _, e = db.Exec(`CREATE SCHEMA ` + schema + `; SET search_path TO ` + schema + `; CREATE TABLE users(id bigint PRIMARY KEY); INSERT INTO users VALUES(1),(2);`); e != nil {
		t.Fatal(e)
	}
	db.SetMaxOpenConns(1)
	// Force search_path on every connection through the dedicated test DSN.
	b, e := os.ReadFile(filepath.Join("..", "..", "migrations", "250_agent_workspace.sql"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(b)); e != nil {
		t.Fatal(e)
	}
	extraMigration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "251_agent_conversation_management.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(extraMigration)); err != nil {
		t.Fatal(err)
	}
	fileMigration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "252_agent_file_attachments.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(fileMigration)); err != nil {
		t.Fatal(err)
	}
	mimeMigration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "253_agent_attachment_mime.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(mimeMigration)); err != nil {
		t.Fatal(err)
	}
	securityMigration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "254_agent_security_storage.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(securityMigration)); err != nil {
		t.Fatal(err)
	}
	workMigration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "255_agent_work_plans.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(workMigration)); err != nil {
		t.Fatal(err)
	}
	adaptiveMigration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "256_agent_adaptive_runtime.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(adaptiveMigration)); err != nil {
		t.Fatal(err)
	}

	return &Store{DB: db}
}
func setupRun(t *testing.T, store *Store, exec *testExecutor) (*Service, *Run) {
	t.Helper()
	s := New(store, exec, t.TempDir(), true)
	ctx := context.Background()
	c, e := s.Create(ctx, 1, "test", Config{ModelKeyID: 1, Model: "text", ImageKeyID: 2, ImageModel: "image", Budget: "1.00"}, "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.Submit(ctx, 1, c.ID, "msg-key", "127.0.0.1", "请生成一张图片", nil, 1, "1.00")
	if e != nil {
		t.Fatal(e)
	}
	// Simulate an already persisted v1 run: recovery must keep its old contract.
	r.RuntimeVersion = 1
	if _, e = store.DB.ExecContext(ctx, `UPDATE agent_runs SET snapshot=$1 WHERE id=$2`, jsonBytes(r), r.ID); e != nil {
		t.Fatal(e)
	}
	return s, r
}
func TestPostgresAgentLifecycle(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{}
	s, created := setupRun(t, st, f)
	ctx := context.Background()
	duplicate, e := s.Submit(ctx, 1, created.ConversationID, "msg-key", "127.0.0.1", "请生成一张图片", nil, 1, "1.00")
	if e != nil || duplicate.ID != created.ID {
		t.Fatalf("idempotency: %v", e)
	}
	if _, e = s.Submit(ctx, 1, created.ConversationID, "msg-key", "127.0.0.1", "different", nil, 1, "1.00"); !errors.Is(e, ErrConflict) {
		t.Fatal("missing conflict", e)
	}
	if _, e = s.Submit(ctx, 1, created.ConversationID, "other", "127.0.0.1", "other", nil, 1, "1.00"); !errors.Is(e, ErrBusy) {
		t.Fatal("missing active-run admission", e)
	}
	r, e := st.Claim(ctx, "test-worker")
	if e != nil {
		t.Fatal(e)
	}
	if r.IP != "127.0.0.1" {
		t.Fatal("lost source IP")
	}
	s.process(ctx, r)
	done, e := st.Run(ctx, 1, created.ID)
	if e != nil {
		t.Fatal(e)
	}
	if done.Status != "completed" {
		t.Fatalf("status=%s error=%s calls=%d", done.Status, done.ErrorCode, f.calls)
	}
	if f.images != 1 || done.Spent != "0.19000000" || len(done.AssetIDs) != 1 {
		t.Fatalf("charges/asset: %+v images=%d", done, f.images)
	}
	a, e := s.LoadAsset(ctx, 1, done.AssetIDs[0])
	if e != nil || !bytes.Equal(a.Data, testImage()) {
		t.Fatal("asset failed", e)
	}
	if _, e = s.LoadAsset(ctx, 2, a.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("asset ownership leak", e)
	}
	if _, e = st.Run(ctx, 2, done.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("run ownership leak", e)
	}
	events, e := st.Events(ctx, 1, done.ID, 0)
	if e != nil || len(events) < 4 {
		t.Fatal("events missing", e)
	}
	for i, ev := range events {
		if ev.Sequence != int64(i+1) {
			t.Fatal("non-monotonic event")
		}
	}
	if len(done.Public()) == 0 {
		t.Fatal("empty snapshot")
	}
	public, _ := json.Marshal(done.Public())
	if bytes.Contains(public, []byte("protocol_history")) || bytes.Contains(public, []byte("client_ip")) {
		t.Fatal("private protocol state leaked")
	}
	// Migration is replayable and does not touch existing source tables.
	b, _ := os.ReadFile(filepath.Join("..", "..", "migrations", "250_agent_workspace.sql"))
	if _, e = st.DB.Exec(string(b)); e != nil {
		t.Fatal(e)
	}
}
func TestPostgresUnknownDoesNotRegenerate(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{unknown: true}
	s, created := setupRun(t, st, f)
	ctx := context.Background()
	r, e := st.Claim(ctx, "worker1")
	if e != nil {
		t.Fatal(e)
	}
	s.process(ctx, r)
	if r.Status != "reconciling" || f.images != 1 {
		t.Fatalf("expected unknown result: %+v %d", r, f.images)
	}
	_ = st.Release(ctx, r)
	_, _ = st.DB.Exec(`UPDATE agent_runs SET lease_until=now()-interval '1 second';UPDATE agent_task_outbox SET ready_at=now()`)
	r, e = st.Claim(ctx, "worker2")
	if e != nil {
		t.Fatal(e)
	}
	s.process(ctx, r)
	if f.images != 1 {
		t.Fatal("reissued paid POST")
	}
	if e = st.Cancel(ctx, 1, created.ID); e != nil {
		t.Fatal(e)
	}
	r.Cancel = true
	s.finishStopped(r, true)
	check, e := st.Run(ctx, 1, created.ID)
	if e != nil || check.Status != "reconciling" {
		t.Fatal("cancel incorrectly finalized unknown", e)
	}
}
func TestPostgresLeaseFencing(t *testing.T) {
	st := isolatedStore(t)
	s, _ := setupRun(t, st, &testExecutor{})
	_ = s
	ctx := context.Background()
	old, e := st.Claim(ctx, "one")
	if e != nil {
		t.Fatal(e)
	}
	_, _ = st.DB.Exec(`UPDATE agent_runs SET lease_until=now()-interval '1 second'`)
	current, e := st.Claim(ctx, "two")
	if e != nil {
		t.Fatal(e)
	}
	if current.Fence <= old.Fence {
		t.Fatal("fence not increased")
	}
	old.Status = "completed"
	if e = st.Save(ctx, old, "run.completed", nil); !errors.Is(e, ErrConflict) {
		t.Fatal("old worker wrote state", e)
	}
}
func TestPostgresLegacyBudgetDoesNotBlockAndAssetsStayValidated(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{}
	s, r := setupRun(t, st, f)
	ctx := context.Background()
	claimed, e := st.Claim(ctx, "worker")
	if e != nil {
		t.Fatal(e)
	}
	claimed.Config.Budget = "0.01000000"
	s.process(ctx, claimed)
	done, e := st.Run(ctx, 1, r.ID)
	if e != nil || done.Status != "completed" || f.calls == 0 {
		t.Fatal("legacy budget blocked dispatch", e)
	}
	if _, e = s.Upload(ctx, 1, r.ConversationID, []byte("not an image")); e == nil {
		t.Fatal("invalid upload accepted")
	}
	a, e := s.Upload(ctx, 1, r.ConversationID, testImage())
	if e != nil {
		t.Fatal(e)
	}
	a.Expires = time.Now().Add(-time.Hour)
	if a.MIME != "image/png" {
		t.Fatal("wrong sniffed MIME")
	}
}

func TestPostgresRestartAfterPaidModel(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{}
	s, created := setupRun(t, st, f)
	ctx := context.Background()
	old, e := st.Claim(ctx, "before-crash")
	if e != nil {
		t.Fatal(e)
	}
	in := Invocation{OperationID: old.ID + "_model_0", RunID: old.ID, Kind: "text", Binding: Binding{UserID: old.UserID, KeyID: old.Config.ModelKeyID, GroupID: old.ModelGroupID, IP: old.IP}, Model: old.Config.Model, Input: old.History, Tools: true, ImageTools: true, AccountID: old.ModelAccountID}
	_, _, e = s.operation(ctx, old, in, 0)
	if e != nil {
		t.Fatal(e)
	}
	if f.calls != 1 {
		t.Fatal("missing initial call")
	}
	// Simulate death after accepted response/bill but before advancing the run.
	_ = st.Release(ctx, old)
	_, _ = st.DB.Exec(`UPDATE agent_task_outbox SET ready_at=now()`)
	restored, e := st.Claim(ctx, "after-crash")
	if e != nil {
		t.Fatal(e)
	}
	s.process(ctx, restored)
	done, e := st.Run(ctx, 1, created.ID)
	if e != nil || done.Status != "completed" || done.ReconcileDeadline != nil || f.calls != 3 || f.images != 1 {
		t.Fatalf("replayed upstream or lost result: %v status=%s calls=%d images=%d", e, done.Status, f.calls, f.images)
	}
}
func TestPostgresCancelBeforeDispatch(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{}
	s, created := setupRun(t, st, f)
	ctx := context.Background()
	if e := st.Cancel(ctx, 1, created.ID); e != nil {
		t.Fatal(e)
	}
	r, e := st.Claim(ctx, "cancel-worker")
	if e != nil {
		t.Fatal(e)
	}
	s.process(ctx, r)
	done, e := st.Run(ctx, 1, created.ID)
	if e != nil || done.Status != "cancelled" || f.calls != 0 {
		t.Fatal("cancel dispatched work", e)
	}
}
func TestPrivateAssetStorage(t *testing.T) {
	s := New(nil, nil, t.TempDir(), true)
	if _, _, _, err := imageInfo([]byte("not an image")); err == nil {
		t.Fatal("accepted corrupt image")
	}
	if err := s.writePrivate("../escape", []byte("x")); err == nil {
		t.Fatal("path traversal accepted")
	}
	if err := s.writePrivate("proof.json", []byte("private")); err != nil {
		t.Fatal(err)
	}
	if err := s.writeOutcome("op", Outcome{Cost: "0.17", Image: testImage()}); err != nil {
		t.Fatal(err)
	}
	proof, err := s.readOutcome("op")
	if err != nil || proof.Cost != "0.17" || !bytes.Equal(proof.Image, testImage()) {
		t.Fatal("checkpoint round trip", err)
	}
	if err := s.writeOutcome("op", proof); err != nil {
		t.Fatal("atomic replacement failed", err)
	}
}

func TestPostgresWorkerAndHistoryConfiguration(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{}
	s, created := setupRun(t, st, f)
	ctx := context.Background()
	list, err := st.Conversations(ctx, 1)
	if err != nil || len(list) != 1 {
		t.Fatal("conversation list", err)
	}
	active, err := st.Active(ctx, 1, created.ConversationID)
	if err != nil || active.ID != created.ID {
		t.Fatal(err)
	}
	if err = st.Configure(ctx, 1, created.ConversationID, 1, created.Config); !errors.Is(err, ErrConflict) {
		t.Fatal("changed active config", err)
	}
	s.Start()
	defer s.Close()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r, e := st.Run(ctx, 1, created.ID)
		if e != nil {
			t.Fatal(e)
		}
		if r.Terminal() {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	done, err := st.Run(ctx, 1, created.ID)
	if err != nil || done.Status != "completed" {
		t.Fatal("background worker failed", err)
	}
	if err = st.Configure(ctx, 1, created.ConversationID, 1, created.Config); err != nil {
		t.Fatal(err)
	}
	cv, err := st.Conversation(ctx, 1, created.ConversationID)
	if err != nil || cv.Version != 2 {
		t.Fatal("version", err)
	}
	r, err := s.Submit(ctx, 1, cv.ID, "followup", "127.0.0.1", "再解释一下", nil, 2, "1.00")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.History) < 3 {
		t.Fatal("lost previous messages")
	}
}
func TestPostgresPreparedOperationRecoveryAndIdentity(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{}
	s, _ := setupRun(t, st, f)
	ctx := context.Background()
	r, e := st.Claim(ctx, "worker")
	if e != nil {
		t.Fatal(e)
	}
	if e = st.Heartbeat(ctx, r); e != nil {
		t.Fatal(e)
	}
	in := Invocation{OperationID: r.ID + "_model_0", RunID: r.ID, Kind: "text", Binding: Binding{UserID: 1, KeyID: 1, GroupID: 11, IP: r.IP}, Model: "text", Input: r.History, Tools: true}
	op, e := st.Prepare(ctx, r, Operation{ID: in.OperationID, RunID: r.ID, Kind: "text", Step: 0, Quote: "0.02", RequestHash: Hash(in)})
	if e != nil {
		t.Fatal(e)
	}
	op, out, e := s.operation(ctx, r, in, 0)
	if e != nil || len(out.Body) == 0 || op.Status != "succeeded" || f.calls != 1 {
		t.Fatal("prepared recovery", e)
	}
	if _, _, e = s.operation(ctx, r, in, 0); e != nil || f.calls != 1 {
		t.Fatal("success replay", e)
	}
	bad := in
	bad.Model = "different"
	if _, _, e = s.operation(ctx, r, bad, 0); !errors.Is(e, ErrConflict) {
		t.Fatal("changed payload accepted", e)
	}
	if _, e = st.Events(ctx, 2, r.ID, 0); !errors.Is(e, ErrNotFound) {
		t.Fatal("cross user events", e)
	}
	if _, e = st.Conversation(ctx, 2, r.ConversationID); !errors.Is(e, ErrNotFound) {
		t.Fatal("cross user conversation", e)
	}
}
func TestPostgresInvalidConfigurationsAndReferences(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{}
	s, run := setupRun(t, st, f)
	ctx := context.Background()
	if _, e := s.Create(ctx, 1, "x", Config{ModelKeyID: 1, Model: "text", Budget: "100"}, "127.0.0.1"); e != nil {
		t.Fatal("legacy ceiling should be ignored", e)
	}
	if _, e := s.Create(ctx, 2, "x", run.Config, "127.0.0.1"); !errors.Is(e, ErrNotFound) {
		t.Fatal("foreign key", e)
	}
	if _, e := s.Submit(ctx, 1, run.ConversationID, "bad", "127.0.0.1", "test", []string{"foreign"}, 1, "1"); !errors.Is(e, ErrNotFound) {
		t.Fatal("invalid ref", e)
	}
	if _, e := s.Submit(ctx, 1, run.ConversationID, "bad", "127.0.0.1", "test", nil, 99, "1"); !errors.Is(e, ErrConflict) {
		t.Fatal("stale config", e)
	}
	if err := st.Cancel(ctx, 1, run.ID); err != nil {
		t.Fatal(err)
	}
	cancelled, claimErr := st.Claim(ctx, "cancel-worker")
	if claimErr != nil {
		t.Fatal(claimErr)
	}
	s.process(ctx, cancelled)
	fresh, err := s.Create(ctx, 1, "no budget", Config{ModelKeyID: 1, Model: "text"}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, e := s.Submit(ctx, 1, fresh.ID, "no-budget", "127.0.0.1", "test", nil, 1, "0"); e != nil {
		t.Fatal("legacy zero budget should be ignored", e)
	}
	s.Enabled = false
	if _, e := s.Create(ctx, 1, "x", run.Config, "127.0.0.1"); !errors.Is(e, ErrDisabled) {
		t.Fatal(e)
	}
	if _, e := s.Submit(ctx, 1, run.ConversationID, "x", "127.0.0.1", "test", nil, 1, "1"); !errors.Is(e, ErrDisabled) {
		t.Fatal(e)
	}
	if _, e := s.Upload(ctx, 1, run.ConversationID, testImage()); !errors.Is(e, ErrDisabled) {
		t.Fatal(e)
	}
}

type scriptedExecutor struct {
	testExecutor
	reply  json.RawMessage
	reject bool
}

func (f *scriptedExecutor) Execute(ctx context.Context, in Invocation) (Outcome, error) {
	f.calls++
	if f.reject {
		return Outcome{}, &InvocationFailure{Code: "AGENT_GATEWAY_REJECTED"}
	}
	return Outcome{Cost: "0.01000000", Body: f.reply}, nil
}
func TestPostgresTextResponsesAndDefiniteFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reply  string
		reject bool
		want   string
	}{
		{"plain", `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}]}`, false, "completed"},
		{"incomplete", `{"status":"incomplete","output":[]}`, false, "failed"},
		{"unknown tool", `{"status":"completed","output":[{"type":"function_call","call_id":"c","name":"shell","arguments":"{}"}]}`, false, "failed"},
		{"definite rejection", "", true, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := isolatedStore(t)
			f := &scriptedExecutor{reply: json.RawMessage(tc.reply), reject: tc.reject}
			svc := New(st, f, t.TempDir(), true)
			ctx := context.Background()
			cv, e := svc.Create(ctx, 1, "", Config{ModelKeyID: 1, Model: "text", Budget: "1"}, "127.0.0.1")
			if e != nil {
				t.Fatal(e)
			}
			created, e := svc.Submit(ctx, 1, cv.ID, "onlytext", "127.0.0.1", "hello", nil, 1, "")
			if e != nil {
				t.Fatal(e)
			}
			run, e := st.Claim(ctx, "worker")
			if e != nil {
				t.Fatal(e)
			}
			svc.process(ctx, run)
			done, e := st.Run(ctx, 1, created.ID)
			if e != nil || done.Status != tc.want || f.calls != 1 {
				t.Fatal("response handling", e, done.Status, f.calls)
			}
			if !tc.reject && done.ModelCost != "0.01000000" {
				t.Fatal("lost model charge on output failure")
			}
		})
	}
}

type pendingBillingExecutor struct {
	testExecutor
	settled bool
}

func (f *pendingBillingExecutor) Execute(ctx context.Context, in Invocation) (Outcome, error) {
	out, e := f.testExecutor.Execute(ctx, in)
	out.BillingPending = true
	return out, e
}
func (f *pendingBillingExecutor) Recover(_ context.Context, _ Invocation, out Outcome) (Outcome, error) {
	if !f.settled {
		return out, ErrUnknown
	}
	out.BillingPending = false
	return out, nil
}
func TestPostgresBillingRecoveryUsesOriginalResponse(t *testing.T) {
	st := isolatedStore(t)
	f := &pendingBillingExecutor{}
	svc := New(st, f, t.TempDir(), true)
	ctx := context.Background()
	cv, e := svc.Create(ctx, 1, "billing", Config{ModelKeyID: 1, Model: "text", ImageKeyID: 2, ImageModel: "image", Budget: "1"}, "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	created, e := svc.Submit(ctx, 1, cv.ID, "billing", "127.0.0.1", "generate", nil, 1, "1")
	if e != nil {
		t.Fatal(e)
	}
	// Restore an existing v1 billing checkpoint without switching its contract.
	created.RuntimeVersion = 1
	if _, e = st.DB.ExecContext(ctx, `UPDATE agent_runs SET snapshot=$1 WHERE id=$2`, jsonBytes(created), created.ID); e != nil {
		t.Fatal(e)
	}

	run, e := st.Claim(ctx, "w1")
	if e != nil {
		t.Fatal(e)
	}
	svc.process(ctx, run)
	if run.Status != "reconciling" || f.calls != 1 {
		t.Fatal("billing pending did not protect dispatch")
	}
	f.settled = true
	_ = st.Release(ctx, run)
	_, _ = st.DB.Exec(`UPDATE agent_task_outbox SET ready_at=now()`)
	run, e = st.Claim(ctx, "w2")
	if e != nil {
		t.Fatal(e)
	}
	svc.process(ctx, run)
	done, e := st.Run(ctx, 1, created.ID)
	if e != nil || done.Status != "completed" || f.calls != 3 || f.images != 1 {
		t.Fatalf("billing recovery replayed request or lost response: %v %+v calls=%d", e, done, f.calls)
	}
}
func TestPostgresAssetIntegrityAndLimit(t *testing.T) {
	st := isolatedStore(t)
	svc, run := setupRun(t, st, &testExecutor{})
	ctx := context.Background()
	if _, e := svc.Upload(ctx, 1, run.ConversationID, make([]byte, (10<<20)+1)); e == nil {
		t.Fatal("oversized upload")
	}
	if _, e := svc.Upload(ctx, 2, run.ConversationID, testImage()); !errors.Is(e, ErrNotFound) {
		t.Fatal("foreign upload", e)
	}
	a, e := svc.Upload(ctx, 1, run.ConversationID, testImage())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(svc.Root, a.Key), []byte("tampered"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.LoadAsset(ctx, 1, a.ID); e == nil {
		t.Fatal("tampering not detected")
	}
	if _, e = st.DB.Exec(`UPDATE agent_assets SET byte_size=$1`, storageQuota); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Upload(ctx, 1, run.ConversationID, testImage()); e == nil {
		t.Fatal("storage quota bypass")
	}
}

func TestPostgresManagedAssetCleanup(t *testing.T) {
	st := isolatedStore(t)
	svc, run := setupRun(t, st, &testExecutor{})
	ctx := context.Background()
	a, e := svc.Upload(ctx, 1, run.ConversationID, testImage())
	if e != nil {
		t.Fatal(e)
	}
	_, e = st.DB.Exec(`UPDATE agent_assets SET expires_at=now()-interval '1 day'`)
	if e != nil {
		t.Fatal(e)
	}
	if e = svc.Cleanup(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(svc.Root, a.Key)); !os.IsNotExist(e) {
		t.Fatal("unrelated active conversation prolonged content retention", e)
	}
	_, _ = st.DB.Exec(`UPDATE agent_runs SET status='cancelled'`)
	if e = svc.Cleanup(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(svc.Root, a.Key)); !os.IsNotExist(e) {
		t.Fatal("expired registered file not purged", e)
	}
	if _, e = svc.LoadAsset(ctx, 1, a.ID); SafeCode(e) != "ASSET_EXPIRED" {
		t.Fatal("purged asset readable", e)
	}
	if e = svc.Cleanup(ctx); e != nil {
		t.Fatal("cleanup not idempotent", e)
	}
}

func TestPostgresCheckpointRetentionOnlyRegisteredFiles(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{}
	svc, created := setupRun(t, st, f)
	ctx := context.Background()
	r, e := st.Claim(ctx, "retention-worker")
	if e != nil {
		t.Fatal(e)
	}
	svc.process(ctx, r)
	if r.Status != "completed" {
		t.Fatal(r.Status, r.ErrorCode)
	}
	checkpoint := filepath.Join(svc.Root, created.ID+"_model_0.result.json")
	if _, e = os.Stat(checkpoint); e != nil {
		t.Fatal("checkpoint missing", e)
	}
	unmanaged := filepath.Join(svc.Root, "keep-backup.txt")
	if e = os.WriteFile(unmanaged, []byte("must remain"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = svc.Cleanup(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(checkpoint); e != nil {
		t.Fatal("premature checkpoint cleanup", e)
	}
	if _, e = st.DB.Exec(`UPDATE agent_checkpoints SET content_expires_at=now()-interval '1 second'`); e != nil {
		t.Fatal(e)
	}
	if e = svc.Cleanup(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(checkpoint); !os.IsNotExist(e) {
		t.Fatal("terminal checkpoint not purged", e)
	}
	if _, e = os.Stat(unmanaged); e != nil {
		t.Fatal("unmanaged file deleted", e)
	}
	if e = svc.Cleanup(ctx); e != nil {
		t.Fatal("checkpoint cleanup retry", e)
	}
}

func TestPostgresCancelAfterConfirmedImageRetainsAsset(t *testing.T) {
	st := isolatedStore(t)
	f := &testExecutor{}
	svc, created := setupRun(t, st, f)
	ctx := context.Background()
	r, e := st.Claim(ctx, "cancel-after-image")
	if e != nil {
		t.Fatal(e)
	}
	r.PendingCall = &Call{ID: "image-call", Name: "generate_image", Arguments: `{"prompt":"cat"}`, ToolID: "tc"}
	r.Turn = 1
	size, aspect := "1K", "1:1"
	in := Invocation{OperationID: r.ID + "_image_0", RunID: r.ID, Kind: "image", Binding: Binding{UserID: 1, KeyID: 2, GroupID: 12, IP: r.IP}, Model: "image", Args: &ToolArgs{Prompt: "cat", Size: &size, Aspect: &aspect}}
	if _, _, e = svc.operation(ctx, r, in, 0); e != nil {
		t.Fatal(e)
	}
	if e = st.Cancel(ctx, 1, created.ID); e != nil {
		t.Fatal(e)
	}
	svc.finishStopped(r, true)
	done, e := st.Run(ctx, 1, created.ID)
	if e != nil || done.Status != "partial_success" || len(done.AssetIDs) != 1 || done.ImageCost != "0.17000000" {
		t.Fatal("lost paid image during cancellation", e, done)
	}
	if f.images != 1 {
		t.Fatal("regenerated")
	}
}

func TestPostgresCancelAfterPaidTextAndMissingResult(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirmed text", true: "missing checkpoint"}[missing], func(t *testing.T) {
			st := isolatedStore(t)
			f := &testExecutor{}
			svc, created := setupRun(t, st, f)
			ctx := context.Background()
			r, e := st.Claim(ctx, "cancel-text")
			if e != nil {
				t.Fatal(e)
			}
			in := Invocation{OperationID: r.ID + "_model_0", RunID: r.ID, Kind: "text", Binding: Binding{UserID: 1, KeyID: 1, GroupID: 11, IP: r.IP}, Model: "text", Input: r.History, Tools: true}
			if _, _, e = svc.operation(ctx, r, in, 0); e != nil {
				t.Fatal(e)
			}
			if missing {
				if e = os.Remove(filepath.Join(svc.Root, in.OperationID+".result.json")); e != nil {
					t.Fatal(e)
				}
			}
			if e = st.Cancel(ctx, 1, created.ID); e != nil {
				t.Fatal(e)
			}
			svc.finishStopped(r, true)
			done, e := st.Run(ctx, 1, created.ID)
			if e != nil {
				t.Fatal(e)
			}
			if missing {
				if done.Status != "reconciling" {
					t.Fatal("missing result treated as complete")
				}
			} else if done.Status != "cancelled" || done.ModelCost != "0.01000000" {
				t.Fatal("lost text bill", done)
			}
			if f.calls != 1 {
				t.Fatal("cancel repeated generation")
			}
		})
	}
}
