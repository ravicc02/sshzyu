package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPostgresAgentMemoryKeepsImageAcrossAcknowledgementAndRestart(t *testing.T) {
	st := isolatedStore(t)
	ctx := context.Background()
	f := &testExecutor{}
	svc, created := setupRun(t, st, f)
	first, e := st.Claim(ctx, "image-worker")
	if e != nil {
		t.Fatal(e)
	}
	svc.process(ctx, first)
	_ = st.Release(ctx, first)
	imageRun, e := st.Run(ctx, 1, created.ID)
	if e != nil || imageRun.Status != "completed" || len(imageRun.AssetIDs) != 1 {
		t.Fatal(imageRun, e)
	}
	ack, e := svc.Submit(ctx, 1, created.ConversationID, "memory-ack", "127.0.0.1", "ok", nil, 1, "1")
	if e != nil {
		t.Fatal(e)
	}
	if ack.Memory == nil || ack.Memory.ConfirmedImages != 1 || ack.Memory.Images[0].AssetID != imageRun.AssetIDs[0] || ack.Memory.Images[0].Availability != "ready" {
		t.Fatal("missing verified image memory", ack.Memory)
	}
	active, e := st.Claim(ctx, "ack-worker")
	if e != nil {
		t.Fatal(e)
	}
	// A short ordinary reply has no image tool. It must not replace prior facts.
	active.Text = "好的！"
	active.Status = "completed"
	if e = st.Save(ctx, active, "run.completed", active.Public()); e != nil {
		t.Fatal(e)
	}
	_ = st.Release(ctx, active)
	restarted := New(st, &testExecutor{}, svc.Root, true)
	follow, e := restarted.Submit(ctx, 1, created.ConversationID, "memory-follow", "127.0.0.1", "刚才你给我生成了什么？", nil, 1, "1")
	if e != nil {
		t.Fatal(e)
	}
	if follow.Memory == nil || follow.Memory.ConfirmedImages != 1 || follow.Memory.Images[0].Status != "succeeded" || follow.Memory.Images[0].AssetID != imageRun.AssetIDs[0] {
		t.Fatal("acknowledgement erased image evidence", follow.Memory)
	}
	if !strings.Contains(string(jsonBytes(follow.History)), "好的") {
		t.Fatal("conversation text lost")
	}
	persisted, e := st.Run(ctx, 1, follow.ID)
	if e != nil || persisted.Memory == nil || persisted.Memory.Images[0].AssetID != imageRun.AssetIDs[0] {
		t.Fatal("memory not durable", e)
	}
	if _, ok := persisted.Public()["conversation_memory"]; ok {
		t.Fatal("private execution memory leaked")
	}
	if _, ok := persisted.Public()["memory"]; !ok {
		t.Fatal("missing public memory statistics")
	}
	if f.images != 1 {
		t.Fatal("memory lookup regenerated image")
	}
}
func TestPostgresAgentMemoryBackfillsLegacyAndKeepsUnavailableHistory(t *testing.T) {
	st := isolatedStore(t)
	ctx := context.Background()
	f := &testExecutor{}
	svc, created := setupRun(t, st, f)
	run, _ := st.Claim(ctx, "old-worker")
	svc.process(ctx, run)
	_ = st.Release(ctx, run)
	original, _ := st.Run(ctx, 1, created.ID)
	// Legacy snapshots have no conversation_memory. The ledger is the source.
	_, e := st.DB.Exec(`UPDATE agent_runs SET snapshot=snapshot-'conversation_memory'`)
	if e != nil {
		t.Fatal(e)
	}
	for _, state := range []string{"expired", "deleted", "purged"} {
		_, e = st.DB.Exec(`UPDATE agent_assets SET status=$1,expires_at=now()-interval '1 day' WHERE id=$2`, state, original.AssetIDs[0])
		if e != nil {
			t.Fatal(e)
		}
		_, m, e := svc.conversationContext(ctx, 1, created.ConversationID, "之前生成过吗", nil)
		if e != nil || m.ConfirmedImages != 1 || m.Images[0].Availability != state {
			t.Fatal(m, e)
		}
	}
	if _, e = st.ConversationMemory(ctx, 2, created.ConversationID); !errors.Is(e, ErrNotFound) {
		t.Fatal("foreign memory leak", e)
	}
	other, e := svc.Create(ctx, 1, "other", Config{ModelKeyID: 1, Model: "text", Budget: "1"}, "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	m, e := st.ConversationMemory(ctx, 1, other.ID)
	if e != nil || m.ConfirmedImages != 0 || len(m.Images) != 0 {
		t.Fatal("cross-conversation memory leak", m, e)
	}
}
func TestPostgresAgentMemoryDoesNotTrustAssistantClaims(t *testing.T) {
	st := isolatedStore(t)
	ctx := context.Background()
	f := &testExecutor{}
	svc, created := setupRun(t, st, f)
	r, _ := st.Claim(ctx, "fake-claim")
	r.Text = "已经生成图片 asset_not_real"
	r.Status = "completed"
	if e := st.Save(ctx, r, "run.completed", r.Public()); e != nil {
		t.Fatal(e)
	}
	_ = st.Release(ctx, r)
	_, m, e := svc.conversationContext(ctx, 1, created.ConversationID, "真的吗", nil)
	if e != nil || m.ConfirmedImages != 0 || len(m.Images) != 0 {
		t.Fatal("text converted into tool evidence", m, e)
	}
	instructions := MemoryInstructions(m)
	if strings.Contains(instructions, "asset_not_real") || !strings.Contains(instructions, "禁止编造 ID") {
		t.Fatal("unverified text entered trusted memory")
	}
}
func TestPostgresAgentMemoryBoundsContextWithoutDroppingCurrentQuestion(t *testing.T) {
	st := isolatedStore(t)
	ctx := context.Background()
	svc := New(st, &testExecutor{}, t.TempDir(), true)
	cv, e := svc.Create(ctx, 1, "long", Config{ModelKeyID: 1, Model: "text", Budget: "1"}, "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 20; i++ {
		m := Message{ID: ID("msg"), Role: "user", Text: strings.Repeat("较早的需求。", 2000), CreatedAt: time.Now().UTC()}
		b, _ := json.Marshal(m)
		if _, e = st.DB.Exec(`INSERT INTO agent_messages(id,conversation_id,role,content) VALUES($1,$2,'user',$3)`, m.ID, cv.ID, b); e != nil {
			t.Fatal(e)
		}
	}
	history, m, e := svc.conversationContext(ctx, 1, cv.ID, "只回答当前问题", nil)
	if e != nil || m.OmittedMessages < 1 || len(jsonBytes(history)) > 100000 {
		t.Fatal("unbounded history", len(jsonBytes(history)), m, e)
	}
	if !strings.Contains(string(history[len(history)-1]), "只回答当前问题") {
		t.Fatal("current question lost")
	}
	if !strings.Contains(string(history[0]), "较早的用户消息摘录") {
		t.Fatal("earlier requirements not retained")
	}
}
func TestPostgresAgentMemoryRetainsClarificationAnswers(t *testing.T) {
	st := isolatedStore(t)
	ctx := context.Background()
	svc := New(st, &askingExecutor{}, t.TempDir(), true)
	cv, e := svc.Create(ctx, 1, "questions", Config{ModelKeyID: 1, Model: "text", Budget: "1"}, "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	r, e := svc.Submit(ctx, 1, cv.ID, "ask", "127.0.0.1", "生成照片", nil, 1, "1")
	if e != nil {
		t.Fatal(e)
	}
	active, _ := st.Claim(ctx, "ask-worker")
	svc.process(ctx, active)
	_ = st.Release(ctx, active)
	waiting, _ := st.Run(ctx, 1, r.ID)
	if _, e = st.Answer(ctx, 1, r.ID, waiting.Question.ID, map[string]string{"subject": "白色小猫", "resolution": "2K"}); e != nil {
		t.Fatal(e)
	}
	active, _ = st.Claim(ctx, "resume-worker")
	svc.process(ctx, active)
	_ = st.Release(ctx, active)
	history, _, e := svc.conversationContext(ctx, 1, cv.ID, "继续刚才要求", nil)
	raw := string(jsonBytes(history))
	if e != nil || !strings.Contains(raw, "白色小猫") || !strings.Contains(raw, "2K") {
		t.Fatal("clarification answers forgotten", e)
	}
}

func TestPostgresAgentMemoryRejectsStaleContextBeforeDispatch(t *testing.T) {
	st := isolatedStore(t)
	ctx := context.Background()
	svc := New(st, &testExecutor{}, t.TempDir(), true)
	cv, e := svc.Create(ctx, 1, "stale", Config{ModelKeyID: 1, Model: "text", Budget: "1"}, "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	add := func(text string) {
		t.Helper()
		m := Message{ID: ID("msg"), Role: "user", Text: text}
		b, _ := json.Marshal(m)
		if _, err := st.DB.Exec(`INSERT INTO agent_messages(id,conversation_id,role,content) VALUES($1,$2,'user',$3)`, m.ID, cv.ID, b); err != nil {
			t.Fatal(err)
		}
	}
	add("旧消息")
	history, m, e := svc.conversationContext(ctx, 1, cv.ID, "下一条", nil)
	if e != nil {
		t.Fatal(e)
	}
	add("刚刚提交的新消息")
	run := &Run{ID: ID("run"), UserID: 1, ConversationID: cv.ID, ConfigVersion: 1, Config: cv.Config, Key: "stale", RequestHash: "stale", Status: "queued", History: history, Memory: m}
	if e = st.Submit(ctx, run); !errors.Is(e, ErrConflict) {
		t.Fatal("stale memory admitted", e)
	}
	var count int
	if e = st.DB.QueryRow(`SELECT count(*) FROM agent_operations`).Scan(&count); e != nil || count != 0 {
		t.Fatal("dispatched with stale memory", count, e)
	}
}
