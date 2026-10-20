//go:build unit

package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/agent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

type smokeAgentExecutor struct{}

func (*smokeAgentExecutor) Options(_ context.Context, _ int64, p string) ([]agent.KeyOption, error) {
	if p == "text" {
		return []agent.KeyOption{{ID: 1, Name: "文本测试 key", GroupName: "文本分组", Purpose: p}}, nil
	}
	return []agent.KeyOption{{ID: 2, Name: "生图测试 key", GroupName: "生图分组", Purpose: p}}, nil
}
func (*smokeAgentExecutor) Models(_ context.Context, _ agent.Binding, p string) ([]string, error) {
	if p == "text" {
		return []string{"text-demo"}, nil
	}
	return []string{"image-demo"}, nil
}
func (*smokeAgentExecutor) Authorize(_ context.Context, b agent.Binding, p, m string) (int64, error) {
	if b.UserID != 1 || p == "text" && b.KeyID != 1 || p == "image" && b.KeyID != 2 {
		return 0, agent.ErrNotFound
	}
	return b.KeyID + 10, nil
}
func (*smokeAgentExecutor) Quote(context.Context, agent.Invocation) (string, error) {
	return "0.20000000", nil
}
func (*smokeAgentExecutor) Execute(_ context.Context, in agent.Invocation) (agent.Outcome, error) {
	time.Sleep(700 * time.Millisecond)
	if in.Kind == "image" {
		canvas := image.NewRGBA(image.Rect(0, 0, 960, 540))
		for y := 0; y < 540; y++ {
			for x := 0; x < 960; x++ {
				canvas.SetRGBA(x, y, color.RGBA{R: uint8(220 + x/40), G: uint8(228 + y/60), B: 215, A: 255})
			}
		}
		var b bytes.Buffer
		_ = png.Encode(&b, canvas)
		return agent.Outcome{Cost: "0.17000000", Image: b.Bytes()}, nil
	}
	hasResult := false
	for _, raw := range in.Input {
		var v map[string]any
		_ = json.Unmarshal(raw, &v)
		if v["type"] == "function_call_output" {
			hasResult = true
		}
	}
	if !hasResult && in.Tools {
		args := `{"prompt":"A restrained product poster","model":null,"image_size":"1K","aspect_ratio":"16:9","reference_asset_ids":null}`
		b, _ := json.Marshal(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "function_call", "call_id": "call-smoke", "name": "generate_image", "arguments": args}}})
		return agent.Outcome{Cost: "0.01000000", Body: b}, nil
	}
	return agent.Outcome{Cost: "0.01000000", Body: json.RawMessage(`{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"图片已经生成，可在下方预览与下载。文本与生图分别按所选分组计费。"}]}]}`)}, nil
}
func agentTestHandler(t *testing.T, executors ...agent.Executor) *AgentHandler {
	t.Helper()
	dsn := os.Getenv("AGENT_HTTP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires isolated Agent HTTP test database")
	}
	db, e := sql.Open("postgres", dsn)
	if e != nil {
		t.Fatal(e)
	}
	db.SetMaxOpenConns(2)
	if _, e = db.Exec(`DROP SCHEMA IF EXISTS agent_http_test CASCADE;CREATE SCHEMA agent_http_test;CREATE TABLE agent_http_test.users(id bigint PRIMARY KEY);INSERT INTO agent_http_test.users VALUES(1),(2);`); e != nil {
		t.Fatal(e)
	}
	migration, e := os.ReadFile(filepath.Join("..", "..", "migrations", "250_agent_workspace.sql"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(migration)); e != nil {
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

	var executor agent.Executor = &smokeAgentExecutor{}
	if len(executors) > 0 {
		executor = executors[0]
	}
	svc := agent.New(&agent.Store{DB: db}, executor, t.TempDir(), true)
	svc.Start()
	t.Cleanup(func() { svc.Close(); db.Close() })
	return &AgentHandler{svc: svc, cfg: &config.Config{}, ready: true}
}
func agentTestRouter(h *AgentHandler) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,Last-Event-ID,Idempotency-Key")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		if c.GetHeader("Authorization") != "Bearer agent-local-smoke-token" {
			c.AbortWithStatus(401)
			return
		}
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1, Concurrency: 2})
		c.Next()
	})
	a := r.Group("/api/v1/agent")
	a.GET("/info", h.Info)
	a.GET("/usage", h.Usage)
	a.GET("/key-options", h.Options)
	a.GET("/capabilities", h.Capabilities)
	a.GET("/conversations", h.List)
	a.POST("/conversations", h.Create)
	a.POST("/conversations/manage", h.ManageConversations)
	a.GET("/conversations/:conversation_id", h.Detail)
	a.PATCH("/conversations/:conversation_id/config", h.Configure)
	a.POST("/conversations/:conversation_id/messages", h.Submit)
	a.POST("/conversations/:conversation_id/assets", h.Upload)
	a.GET("/runs/:run_id", h.GetRun)
	a.GET("/runs/:run_id/events", h.Events)
	a.POST("/runs/:run_id/cancel", h.Cancel)
	a.POST("/runs/:run_id/answer", h.Answer)
	a.POST("/runs/:run_id/recover-image", h.RecoverImage)
	a.POST("/work-plans/:plan_id/review", h.ReviewWork)
	a.POST("/work-plans/:plan_id/submit", h.SubmitWork)
	a.GET("/assets/:asset_id", h.Asset)
	a.GET("/assets/:asset_id/meta", h.AssetMeta)
	a.PUT("/work-plans/:plan_id/draft", h.SaveDraft)
	a.POST("/work-plans/:plan_id/grants", h.CreateGrant)
	a.DELETE("/grants/:grant_id", h.RevokeGrant)
	a.DELETE("/assets/:asset_id", h.DeleteAsset)
	return r
}
func TestAgentHTTPContract(t *testing.T) {
	h := agentTestHandler(t)
	router := agentTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/conversations", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatal("anonymous access")
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/agent/key-options?purpose=image", nil)
	req.Header.Set("Authorization", "Bearer agent-local-smoke-token")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte("api_key\"")) || w.Header().Get("Cache-Control") != "private, no-store, max-age=0" {
		t.Fatal("options failed or credentials leaked", w.Body.String())
	}
}
func TestAgentBrowserHarness(t *testing.T) {
	if os.Getenv("AGENT_BROWSER_HARNESS") != "true" {
		t.Skip("browser harness opt-in only")
	}
	h := agentTestHandler(t)
	router := agentTestRouter(h)
	var once sync.Once
	router.POST("/_test/shutdown", func(c *gin.Context) { c.Status(204); once.Do(func() { close(browserHarnessDone) }) })
	server := &http.Server{Addr: "127.0.0.1:18089", Handler: router, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = server.ListenAndServe() }()
	t.Cleanup(func() { _ = server.Close() })
	t.Log("Agent local browser test harness ready on 127.0.0.1:18089 (fake provider, isolated PostgreSQL)")
	select {
	case <-time.After(12 * time.Minute):
	case <-browserHarnessDone:
	}
}

var browserHarnessDone = make(chan struct{})

type traceHTTPExecutor struct{ smokeAgentExecutor }

func (e *traceHTTPExecutor) Execute(ctx context.Context, in agent.Invocation) (agent.Outcome, error) {
	out, err := e.smokeAgentExecutor.Execute(ctx, in)
	if err != nil || in.Kind != "text" {
		return out, err
	}
	var value map[string]json.RawMessage
	if err = json.Unmarshal(out.Body, &value); err != nil {
		return out, err
	}
	var output []json.RawMessage
	if err = json.Unmarshal(value["output"], &output); err != nil {
		return out, err
	}
	summary := json.RawMessage(`{"type":"reasoning","encrypted_content":"private-http-encrypted","summary":[{"type":"summary_text","text":"公开摘要，已确认用户需求。"}]}`)
	value["output"], _ = json.Marshal(append([]json.RawMessage{summary}, output...))
	out.Body, err = json.Marshal(value)
	return out, err
}
func TestAgentHTTPTraceContract(t *testing.T) {
	h := agentTestHandler(t, &traceHTTPExecutor{})
	router := agentTestRouter(h)
	call := func(method, path string, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer agent-local-smoke-token")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "http-trace-request")
		writer := httptest.NewRecorder()
		router.ServeHTTP(writer, req)
		return writer
	}
	w := call(http.MethodPost, "/api/v1/agent/conversations", []byte(`{"title":"trace","config":{"model_api_key_id":1,"model":"text-demo","image_api_key_id":2,"image_model":"image-demo","default_max_cost_usd":"1"}}`))
	if w.Code >= 300 {
		t.Fatal("create", w.Code, w.Body.String())
	}
	var cv agent.Conversation
	if err := json.Unmarshal(w.Body.Bytes(), &cv); err != nil {
		t.Fatal(err)
	}
	w = call(http.MethodPost, "/api/v1/agent/conversations/"+cv.ID+"/messages", []byte(`{"content":"generate a poster","config_version":1,"max_cost_usd":"1"}`))
	if w.Code != 202 {
		t.Fatal("submit", w.Code, w.Body.String())
	}
	var run struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	// This exercises the real persistent SSE handler until the worker completes.
	w = call(http.MethodGet, "/api/v1/agent/runs/"+run.ID+"/events", nil)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("model.started")) || !bytes.Contains(w.Body.Bytes(), []byte("work.plan.created")) || !bytes.Contains(w.Body.Bytes(), []byte(`"trace"`)) {
		t.Fatal("missing persistent process events", w.Code)
	}
	if bytes.Contains(w.Body.Bytes(), []byte("private-http")) || bytes.Contains(w.Body.Bytes(), []byte("encrypted_content")) {
		t.Fatal("SSE leaked private reasoning")
	}
	w = call(http.MethodGet, "/api/v1/agent/conversations/"+cv.ID, nil)
	var detail struct {
		Messages []agent.Message  `json:"messages"`
		Plans    []agent.WorkPlan `json:"work_plans"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &detail) != nil || len(detail.Messages) != 2 || len(detail.Messages[1].Trace) != 3 {
		t.Fatal("history trace contract")
	}
	if detail.Messages[1].Trace[0].Summary == "" || detail.Messages[1].Trace[1].Arguments == nil || len(detail.Messages[1].Trace[1].AssetIDs) != 0 || len(detail.Plans) != 1 {
		t.Fatal("trace fields missing from history")
	}
	if bytes.Contains(w.Body.Bytes(), []byte("private-http")) || bytes.Contains(w.Body.Bytes(), []byte("encrypted_content")) {
		t.Fatal("history leaked private reasoning")
	}
	// Human quote + approval creates a separate deterministic render job.
	draftBody, _ := json.Marshal(map[string]any{"revision": detail.Plans[0].Revision, "draft": detail.Plans[0].WorkDraft})
	w = call(http.MethodPost, "/api/v1/agent/work-plans/"+detail.Plans[0].ID+"/review", draftBody)
	if w.Code != 200 {
		t.Fatal("review", w.Code, w.Body.String())
	}
	var approved agent.WorkPlan
	if json.Unmarshal(w.Body.Bytes(), &approved) != nil {
		t.Fatal("review result")
	}
	submitBody, _ := json.Marshal(map[string]int{"revision": approved.Revision})
	w = call(http.MethodPost, "/api/v1/agent/work-plans/"+approved.ID+"/submit", submitBody)
	if w.Code != 202 {
		t.Fatal("approve", w.Code, w.Body.String())
	}
	if json.Unmarshal(w.Body.Bytes(), &run) != nil {
		t.Fatal("approved run")
	}
	w = call(http.MethodGet, "/api/v1/agent/runs/"+run.ID+"/events", nil)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("work.item.completed")) {
		t.Fatal("approved render did not finish", w.Code)
	}

}

type connectionFailureHTTPExecutor struct {
	smokeAgentExecutor
	calls atomic.Int32
}

func (e *connectionFailureHTTPExecutor) Execute(context.Context, agent.Invocation) (agent.Outcome, error) {
	e.calls.Add(1)
	return agent.Outcome{}, &agent.InvocationFailure{Code: "AGENT_UPSTREAM_CONNECTION_FAILED", Dispatched: false}
}
func TestAgentHTTPConnectionFailureDoesNotEnterReconciliation(t *testing.T) {
	executor := &connectionFailureHTTPExecutor{}
	h := agentTestHandler(t, executor)
	router := agentTestRouter(h)
	call := func(method, path, key string, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer agent-local-smoke-token")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		writer := httptest.NewRecorder()
		router.ServeHTTP(writer, req)
		return writer
	}
	w := call(http.MethodPost, "/api/v1/agent/conversations", "", []byte(`{"title":"connection","config":{"model_api_key_id":1,"model":"text-demo","default_max_cost_usd":"1"}}`))
	var cv agent.Conversation
	if w.Code >= 300 || json.Unmarshal(w.Body.Bytes(), &cv) != nil {
		t.Fatal("create failed")
	}
	message := []byte(`{"content":"hello","config_version":1,"max_cost_usd":"1"}`)
	w = call(http.MethodPost, "/api/v1/agent/conversations/"+cv.ID+"/messages", "first", message)
	var r agent.Run
	if w.Code != 202 || json.Unmarshal(w.Body.Bytes(), &r) != nil {
		t.Fatal("submit failed")
	}
	w = call(http.MethodGet, "/api/v1/agent/runs/"+r.ID+"/events", "", nil)
	if w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte("operation.reconciling")) {
		t.Fatal("known connection failure entered reconciliation")
	}
	w = call(http.MethodGet, "/api/v1/agent/runs/"+r.ID, "", nil)
	var done agent.Run
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &done) != nil || done.Status != "failed" || done.ErrorCode != "AGENT_UPSTREAM_CONNECTION_FAILED" || executor.calls.Load() != 1 {
		t.Fatal("not a terminal single attempt")
	}
	if w.Header().Get("Cache-Control") != "private, no-store, max-age=0" {
		t.Fatal("task snapshot can be cached")
	}
	w = call(http.MethodPost, "/api/v1/agent/conversations/"+cv.ID+"/messages", "second", message)
	if w.Code != 202 {
		t.Fatal("known failure still blocks new requests", w.Code)
	}
}

type questionHTTPExecutor struct{ smokeAgentExecutor }

func (f *questionHTTPExecutor) Execute(_ context.Context, in agent.Invocation) (agent.Outcome, error) {
	if in.OnProgress != nil {
		if e := in.OnProgress(agent.StreamUpdate{Summary: "需要先确定照片主题。"}); e != nil {
			return agent.Outcome{}, e
		}
	}
	var items []any
	if strings.HasSuffix(in.OperationID, "_model_0") {
		args := `{"question":"想生成什么照片？","fields":[{"id":"subject","label":"画面内容","options":[],"required":true}]}`
		items = []any{map[string]any{"type": "function_call", "call_id": "question-http", "name": "ask_user", "arguments": args}}
	} else {
		items = []any{map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": "已收到你的答案。"}}}}
	}
	b, _ := json.Marshal(map[string]any{"status": "completed", "output": items})
	return agent.Outcome{Cost: "0.01000000", Body: b}, nil
}
func TestAgentHTTPQuestionAndManagement(t *testing.T) {
	h := agentTestHandler(t, &questionHTTPExecutor{})
	router := agentTestRouter(h)
	call := func(path string, body any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent"+path, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer agent-local-smoke-token")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "http-question")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	w := call("/conversations", map[string]any{"title": "question", "config": agent.Config{ModelKeyID: 1, Model: "text-demo", Budget: "1"}})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var cv agent.Conversation
	_ = json.Unmarshal(w.Body.Bytes(), &cv)
	w = call("/conversations/"+cv.ID+"/messages", map[string]any{"content": "生成一张照片", "config_version": 1, "max_cost_usd": "1"})
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var run struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &run)
	var waiting *agent.Run
	for until := time.Now().Add(5 * time.Second); time.Now().Before(until); {
		waiting, _ = h.svc.Store.Run(context.Background(), 1, run.ID)
		if waiting != nil && waiting.Status == "waiting_user" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if waiting == nil || waiting.Question == nil {
		t.Fatal("missing persisted question")
	}
	w = call("/conversations/manage", map[string]any{"ids": []string{cv.ID}, "action": "delete"})
	if w.Code < 400 {
		t.Fatal("active conversation deleted")
	}
	w = call("/runs/"+run.ID+"/answer", map[string]any{"question_id": waiting.Question.ID, "answers": map[string]string{}})
	if w.Code < 400 {
		t.Fatal("missing answer accepted")
	}
	body := map[string]any{"question_id": waiting.Question.ID, "answers": map[string]string{"subject": "海边的猫"}}
	for i := 0; i < 2; i++ {
		w = call("/runs/"+run.ID+"/answer", body)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/runs/"+run.ID+"/events", nil)
	req.Header.Set("Authorization", "Bearer agent-local-smoke-token")
	events := httptest.NewRecorder()
	router.ServeHTTP(events, req)
	for _, typ := range []string{"model.delta", "user.question", "user.answered", "run.completed"} {
		if !bytes.Contains(events.Body.Bytes(), []byte(typ)) {
			t.Fatal("missing SSE lifecycle", typ)
		}
	}
	for _, action := range []string{"rename", "archive", "restore", "delete"} {
		w = call("/conversations/manage", map[string]any{"ids": []string{cv.ID}, "action": action, "title": "新标题"})
		if w.Code != 200 {
			t.Fatal(action, w.Code, w.Body.String())
		}
	}
	if bytes.Contains(w.Body.Bytes(), []byte(cv.ID)) {
		t.Fatal("deleted conversation still listed")
	}
}

func TestAgentHTTPFileAttachmentRoundTrip(t *testing.T) {
	h := agentTestHandler(t)
	r := agentTestRouter(h)
	cv, e := h.svc.Create(context.Background(), 1, "files", agent.Config{ModelKeyID: 1, Model: "text-demo", Budget: "1"}, "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("file", "报表.csv")
	_, _ = part.Write([]byte("year,total\n2026,100"))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/conversations/"+cv.ID+"/assets", &body)
	req.Header.Set("Authorization", "Bearer agent-local-smoke-token")
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var a agent.Asset
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	if a.FileName != "报表.csv" || a.MIME != "text/csv" {
		t.Fatal(a)
	}
	for _, suffix := range []string{"/meta", ""} {
		req = httptest.NewRequest(http.MethodGet, "/api/v1/agent/assets/"+a.ID+suffix, nil)
		req.Header.Set("Authorization", "Bearer agent-local-smoke-token")
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if suffix == "/meta" {
			if !strings.Contains(w.Body.String(), "报表.csv") || strings.Contains(w.Body.String(), "storage_key") {
				t.Fatal("bad private file metadata")
			}
		} else if w.Body.String() != "year,total\n2026,100" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") {
			t.Fatal("bad file download", w.Header())
		}
	}
}

func TestAgentImageRecoveryHTTPRequiresAuthenticationAndSavedResult(t *testing.T) {
	h := agentTestHandler(t)
	router := agentTestRouter(h)
	req := httptest.NewRequest("POST", "/api/v1/agent/runs/missing/recover-image", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatal("anonymous recovery", w.Code)
	}
	req = httptest.NewRequest("POST", "/api/v1/agent/runs/missing/recover-image", nil)
	req.Header.Set("Authorization", "Bearer agent-local-smoke-token")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatal("recovery disclosed unknown result", w.Code)
	}
}

func TestAgentHTTPExpiredMetadataAndBinaryHaveSeparateContracts(t *testing.T) {
	h := agentTestHandler(t)
	router := agentTestRouter(h)
	ctx := context.Background()
	conv, err := h.svc.Create(ctx, 1, "retention", agent.Config{ModelKeyID: 1, Model: "text-demo", Budget: "1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	a, err := h.svc.UploadFile(ctx, 1, conv.ID, "source.txt", []byte("private original body"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.svc.Store.DB.ExecContext(ctx, `UPDATE agent_assets SET expires_at=now()-interval '1 second' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	call := func(suffix string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/v1/agent/assets/"+a.ID+suffix, nil)
		r.Header.Set("Authorization", "Bearer agent-local-smoke-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	meta := call("/meta")
	if meta.Code != 200 || !bytes.Contains(meta.Body.Bytes(), []byte(`"content_available":false`)) || !bytes.Contains(meta.Body.Bytes(), []byte("source.txt")) {
		t.Fatal("metadata", meta.Code, meta.Body.String())
	}
	file := call("")
	if file.Code != 410 || !bytes.Contains(file.Body.Bytes(), []byte("ASSET_EXPIRED")) || bytes.Contains(file.Body.Bytes(), []byte("private original body")) {
		t.Fatal("expired file accessible", file.Code, file.Body.String())
	}
	if err = h.svc.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if meta = call("/meta"); meta.Code != 200 {
		t.Fatal("cleanup lost history")
	}
}
