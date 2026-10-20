//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/agent"
)

func usageFixture(t *testing.T) *AgentHandler {
	t.Helper()
	h := agentTestHandler(t)
	h.svc.Close()
	_, err := h.svc.Store.DB.Exec(`CREATE TABLE groups(id bigint PRIMARY KEY,name text,deleted_at timestamptz);
 CREATE TABLE usage_logs(id bigserial PRIMARY KEY,request_id text,user_id bigint,api_key_id bigint,group_id bigint,input_tokens bigint DEFAULT 0,output_tokens bigint DEFAULT 0,cache_read_tokens bigint DEFAULT 0,cache_creation_tokens bigint DEFAULT 0,image_count integer DEFAULT 0,actual_cost numeric(20,10),duration_ms integer,first_token_ms integer);
 INSERT INTO groups VALUES(11,'旧对话分组',now()),(12,'旧生图分组',NULL),(21,'实际计费分组',NULL),(99,'其他用户分组',NULL);`)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func usageRun(t *testing.T, h *AgentHandler) *agent.Run {
	t.Helper()
	ctx := context.Background()
	c, err := h.svc.Create(ctx, 1, "usage", agent.Config{ModelKeyID: 1, Model: "text-demo", ImageKeyID: 2, ImageModel: "image-demo", Budget: "1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	r, err := h.svc.Submit(ctx, 1, c.ID, "usage-test", "", "hello", nil, c.Version, "1")
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestAgentUsageCountsOwnedActualBillsOnceAndPreservesHistoricalGroups(t *testing.T) {
	h := usageFixture(t)
	r := usageRun(t, h)
	ctx := context.Background()
	db := h.svc.Store.DB
	_, err := db.Exec(`INSERT INTO agent_operations(id,run_id,kind,step,request_hash,quote,cost,status,billing_request_id) VALUES
 ('usage-text',$1,'text',0,'a',9,0.01,'succeeded','agent:usage-text'),
 ('usage-image',$1,'image',0,'b',9,0.17,'succeeded','agent:usage-image'),
 ('usage-pending',$1,'image',1,'c',9,NULL,'result_unknown','agent:usage-pending'),
 ('usage-prepared',$1,'image',2,'d',100,NULL,'prepared','agent:usage-prepared'),
 ('usage-fallback',$1,'text',1,'e',9,0.02,'succeeded','agent:usage-fallback');`, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO usage_logs(request_id,user_id,api_key_id,group_id,input_tokens,output_tokens,cache_read_tokens,cache_creation_tokens,image_count,actual_cost) VALUES
 ('agent:usage-text',1,1,21,100,200,50,10,0,0.0123456789),
 ('agent:usage-text',1,1,21,100,200,50,10,0,0.0123456789),
 ('agent:usage-image',1,2,12,9999,9999,0,0,2,0.1700000000),
 ('agent:usage-text',2,1,99,900000,900000,0,0,0,999),
 ('agent:usage-text',1,99,99,800000,800000,0,0,0,888),
 ('unrelated-request',1,1,21,700000,700000,0,0,0,777);`)
	if err == nil {
		_, err = db.Exec(`UPDATE agent_conversations SET deleted_at=now(),config=jsonb_set(config,'{model_api_key_id}','99'::jsonb) WHERE id=$1`, r.ConversationID)
	}
	if err != nil {
		t.Fatal(err)
	}
	result, err := h.usageSummary(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalCostUSD != "0.2023456789" || result.Text.Totals.CostUSD != "0.0323456789" || result.Image.Totals.CostUSD != "0.1700000000" {
		t.Fatal("quotes, foreign/non-Agent bills or duplicates affected actual cost", result)
	}
	if result.Text.Totals.Requests != 2 || result.Text.Totals.TotalTokens != 360 || result.Image.Totals.ImageCount != 2 || result.Image.Totals.Requests != 2 || result.Image.Totals.PendingRequests != 1 {
		t.Fatal("request/token/image count incorrect", result)
	}
	if len(result.Text.Groups) != 2 || result.Text.Groups[0].GroupName != "旧对话分组" || result.Text.Groups[1].GroupName != "实际计费分组" {
		t.Fatal("historical key binding/group lost", result.Text.Groups)
	}
	if result.Text.Totals.MissingUsageRecords != 1 || result.Image.Totals.MissingImageRecords != 1 {
		t.Fatal("missing telemetry hidden")
	}
	foreign, err := h.usageSummary(ctx, 2)
	if err != nil || len(foreign.Text.Groups) != 0 || len(foreign.Image.Groups) != 0 || foreign.TotalCostUSD != "0.0000000000" {
		t.Fatal("foreign operation ownership bypass", foreign, err)
	}
	var operations int
	db.QueryRow(`SELECT count(*) FROM agent_operations`).Scan(&operations)
	if operations != 5 {
		t.Fatal("statistics wrote billing data")
	}
}
func TestAgentUsageRetainsExpiredImageCountWithoutCountingRecoveryOrReuse(t *testing.T) {
	h := usageFixture(t)
	r := usageRun(t, h)
	ctx := context.Background()
	db := h.svc.Store.DB
	_, err := db.Exec(`INSERT INTO agent_operations(id,run_id,kind,step,request_hash,quote,cost,status,billing_request_id) VALUES('retained-image',$1,'image',0,'a',0.2,0.17,'succeeded','agent:retained-image')`, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO agent_assets(id,user_id,conversation_id,run_id,operation_id,storage_key,mime_type,byte_size,width,height,sha256,status,expires_at,filename) VALUES('asset-old',1,$2,$1,'retained-image','asset-old.image','image/png',4,1,1,'hash','purged',now()-interval '1 hour','old.png')`, r.ID, r.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`UPDATE agent_runs SET snapshot=snapshot || '{"recover_only":true}'::jsonb WHERE id=$1`, r.ID)
	if err != nil {
		t.Fatal(err)
	}

	first, err := h.usageSummary(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.usageSummary(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Image.Totals.ImageCount != 1 || first.Image.Totals.CostUSD != "0.1700000000" || first.Image.Totals.MissingImageRecords != 0 || second.Image.Totals.Requests != 1 {
		t.Fatal("recovery, expiry or metadata inflated usage", first, second)
	}
	if first.Image.Groups[0].GroupName != "旧生图分组" {
		t.Fatal("used current key group instead of run binding")
	}
	_, err = db.Exec(`DELETE FROM groups WHERE id=12`)
	if err != nil {
		t.Fatal(err)
	}
	missing, err := h.usageSummary(ctx, 1)
	if err != nil || missing.Image.Groups[0].GroupName != "历史分组 #12" {
		t.Fatal("deleted group fallback lost", err)
	}
}
func TestAgentUsageHTTPRequiresAuthAndReturnsReadOnlyEmptyStatistics(t *testing.T) {
	h := usageFixture(t)
	router := agentTestRouter(h)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/agent/usage", nil))
	if w.Code != 401 {
		t.Fatal("anonymous stats", w.Code)
	}
	request := httptest.NewRequest("GET", "/api/v1/agent/usage?user_id=2", nil)
	request.Header.Set("Authorization", "Bearer agent-local-smoke-token")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, request)
	var value agentUsageSummary
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &value) != nil || value.Scope != "all_agent_history" || value.TotalCostUSD != "0.0000000000" || value.Text.Groups == nil || value.Image.Groups == nil || w.Header().Get("Cache-Control") != "private, no-store, max-age=0" {
		t.Fatal("empty or private response contract", w.Code, w.Body.String())
	}
	h.ready = false
	w = httptest.NewRecorder()
	router.ServeHTTP(w, request)
	if w.Code != 503 {
		t.Fatal("unavailable stats masked as zero usage")
	}
}
