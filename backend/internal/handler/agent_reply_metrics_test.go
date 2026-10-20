//go:build unit

package handler

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/agent"
	"testing"
)

func TestAgentReplyMetricsUsesOwnedLedgerAndKnownTimings(t *testing.T) {
	h := agentTestHandler(t)
	h.svc.Close()
	ctx := context.Background()
	db := h.svc.Store.DB
	_, err := db.Exec(`CREATE TABLE usage_logs(request_id text,user_id bigint,api_key_id bigint,input_tokens bigint,output_tokens bigint,cache_read_tokens bigint,cache_creation_tokens bigint,duration_ms integer,first_token_ms integer)`)
	if err != nil {
		t.Fatal(err)
	}
	cv, err := h.svc.Create(ctx, 1, "metrics", agent.Config{ModelKeyID: 1, Model: "text-demo", ImageKeyID: 2, ImageModel: "image-demo", Budget: "1"}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	r, err := h.svc.Submit(ctx, 1, cv.ID, "metrics-input", "127.0.0.1", "hello", nil, 1, "1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO agent_operations(id,run_id,kind,step,request_hash,quote,billing_request_id) VALUES ('metric-text',$1,'text',0,'hash',0.1,'metric-text'),('metric-image',$1,'image',1,'hash',0.1,'metric-image');
 `, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO usage_logs VALUES('metric-text',1,1,100,200,50,10,19000,9000),('metric-image',1,2,999,999,0,0,5000,NULL),('metric-text',2,1,88888,88888,0,0,99999,1),('metric-text',1,99,77777,77777,0,0,99999,1)`)
	if err != nil {
		t.Fatal(err)
	}
	m := h.replyMetrics(ctx, 1, cv.ID)[r.ID]
	if m == nil || m.TotalTokens != 360 || m.OutputTokens != 200 || m.ElapsedMS == nil || *m.ElapsedMS != 24000 || m.GenerationMS == nil || *m.GenerationMS != 10000 {
		t.Fatalf("incorrect metrics: %#v", m)
	}
	if len(h.replyMetrics(ctx, 2, cv.ID)) != 0 {
		t.Fatal("foreign metrics exposed")
	}
	_, err = db.Exec(`UPDATE usage_logs SET first_token_ms=NULL WHERE user_id=1 AND api_key_id=1`)
	if err != nil {
		t.Fatal(err)
	}
	if h.replyMetrics(ctx, 1, cv.ID)[r.ID].GenerationMS != nil {
		t.Fatal("invented generation timing")
	}
}
