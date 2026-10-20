//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/agent"
	"net/http/httptest"
	"testing"
	"time"
)

type workHTTPExecutor struct{ smokeAgentExecutor }

func (e *workHTTPExecutor) Execute(ctx context.Context, in agent.Invocation) (agent.Outcome, error) {
	if in.Kind == "image" {
		return e.smokeAgentExecutor.Execute(ctx, in)
	}
	for _, raw := range in.Input {
		var item struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &item) == nil && item.Type == "function_call_output" {
			return agent.Outcome{Cost: "0.01000000", Body: json.RawMessage(`{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"请审核草稿。"}]}]}`)}, nil
		}
	}
	args := `{"title":"学习经验","brief":"面向新生","style":"蓝白模板","image_size":"1K","aspect_ratio":"16:9","shared_reference_asset_ids":[],"items":[{"id":"p1","title":"目标","content":"确定学习目标","prompt":"清晰的标题和三个要点","reference_asset_ids":[]},{"id":"p2","title":"行动","content":"练习与复盘","prompt":"双列要点布局","reference_asset_ids":[]}]}`
	b, _ := json.Marshal(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "function_call", "call_id": "plan-http", "name": "save_work_plan", "arguments": args}}})
	return agent.Outcome{Cost: "0.01000000", Body: b}, nil
}
func TestAgentHTTPWorkReviewSubmitContract(t *testing.T) {
	h := agentTestHandler(t, &workHTTPExecutor{})
	router := agentTestRouter(h)
	call := func(method, path string, body any, auth bool) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "/api/v1/agent"+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "http-work-message")
		if auth {
			req.Header.Set("Authorization", "Bearer agent-local-smoke-token")
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	if w := call("POST", "/work-plans/unknown/submit", map[string]any{"revision": 1}, false); w.Code != 401 {
		t.Fatal("anonymous execution", w.Code)
	}
	c, err := h.svc.Create(context.Background(), 1, "HTTP配置卡", agent.Config{ModelKeyID: 1, Model: "text-demo", ImageKeyID: 2, ImageModel: "image-demo", Budget: "1.00"}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	w := call("POST", "/conversations/"+c.ID+"/messages", map[string]any{"content": "按明确的内容制作2页幻灯片：目标和行动，1K，16:9，先保存配置卡。", "config_version": 1, "max_cost_usd": "1.00", "attachment_asset_ids": []string{}}, true)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var plans []agent.WorkPlan
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		plans, err = h.svc.Store.WorkPlans(context.Background(), 1, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(plans) == 1 {
			r, _ := h.svc.Store.Run(context.Background(), 1, plans[0].SourceRunID)
			if r != nil && r.Terminal() {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(plans) != 1 {
		t.Fatal("no HTTP workflow card")
	}
	p := plans[0]
	w = call("POST", "/work-plans/"+p.ID+"/review", map[string]any{"revision": p.Revision, "draft": p.WorkDraft}, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var ready agent.WorkPlan
	if err = json.Unmarshal(w.Body.Bytes(), &ready); err != nil {
		t.Fatal(err)
	}
	w = call("POST", "/work-plans/"+p.ID+"/submit", map[string]any{"revision": ready.Revision, "approved": true}, true)
	if w.Code != 400 {
		t.Fatal("unknown approval field accepted", w.Code)
	}
	w = call("POST", "/work-plans/"+p.ID+"/submit", map[string]any{"revision": ready.Revision}, true)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var run map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &run)
	if _, ok := run["protocol_history"]; ok {
		t.Fatal("private protocol exposed")
	}
	if _, ok := run["client_ip"]; ok {
		t.Fatal("private IP exposed")
	}
	w = call("POST", "/work-plans/"+p.ID+"/submit", map[string]any{"revision": ready.Revision}, true)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var duplicate map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &duplicate)
	if duplicate["id"] != run["id"] {
		t.Fatal("duplicate human submit")
	}

	executionDeadline := time.Now().Add(8 * time.Second)
	for {
		snapshot, e := h.svc.Store.Run(context.Background(), 1, run["id"].(string))
		if e != nil {
			t.Fatal(e)
		}
		if snapshot.Status == "completed" {
			break
		}
		if time.Now().After(executionDeadline) {
			t.Fatal("batch did not finish", snapshot.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if w = call("POST", "/work-plans/"+p.ID+"/review", map[string]any{"revision": ready.Revision, "draft": p.WorkDraft, "item_ids": []string{"p2"}}, false); w.Code != 401 {
		t.Fatal("anonymous item approval", w.Code)
	}
	w = call("POST", "/work-plans/"+p.ID+"/review", map[string]any{"revision": ready.Revision, "draft": p.WorkDraft, "item_ids": []string{"p2"}}, true)
	if w.Code != 200 {
		t.Fatal("single-item HTTP review", w.Code, w.Body.String())
	}
	var selected agent.WorkPlan
	if err = json.Unmarshal(w.Body.Bytes(), &selected); err != nil {
		t.Fatal(err)
	}
	if len(selected.TargetItemIDs) != 1 || selected.TargetItemIDs[0] != "p2" || selected.ItemQuotes[0] != "0.00000000" {
		t.Fatal("selection not bound to review")
	}
}
