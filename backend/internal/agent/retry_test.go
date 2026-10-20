package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPostgresAgentRetryIsolatedIdempotentAndNoDuplicateUser(t *testing.T) {
	st := isolatedStore(t)
	svc, _ := setupRun(t, st, &testExecutor{})
	ctx := context.Background()
	source, err := st.Claim(ctx, "retry-test")
	if err != nil {
		t.Fatal(err)
	}
	svc.process(ctx, source)
	if source.Status != "completed" {
		t.Fatal(source.Status)
	}
	if _, err = svc.Retry(ctx, 2, source.ID, "foreign", "127.0.0.1", 1, "1"); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign retry accepted", err)
	}
	retried, err := svc.Retry(ctx, 1, source.ID, "retry-once", "127.0.0.1", 1, "1")
	if err != nil {
		t.Fatal(err)
	}
	if retried.ID == source.ID || retried.RetryOf != source.ID || len(retried.History) != 1 || strings.Contains(string(retried.History[0]), "图片已生成") {
		t.Fatal("retry must start from original input, excluding old outcome")
	}
	duplicate, err := svc.Retry(ctx, 1, source.ID, "retry-once", "127.0.0.1", 1, "1")
	if err != nil || duplicate.ID != retried.ID {
		t.Fatal("retry idempotency lost", err)
	}
	var users int
	err = st.DB.QueryRow(`SELECT count(*) FROM agent_messages WHERE conversation_id=$1 AND role='user'`, source.ConversationID).Scan(&users)
	if err != nil || users != 1 {
		t.Fatal("retry duplicated user message", users, err)
	}
	claimed, err := st.Claim(ctx, "retry-test-2")
	if err != nil {
		t.Fatal(err)
	}
	svc.process(ctx, claimed)
	if claimed.Status != "completed" {
		t.Fatal(claimed.Status)
	}
	messages, err := st.Messages(ctx, 1, source.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	assistants := 0
	for _, m := range messages {
		if m.Role == "assistant" {
			assistants++
			if m.RunID != claimed.ID {
				t.Fatal("superseded answer still included in model context")
			}
		}
	}
	if assistants != 1 {
		t.Fatal("replacement answer missing")
	}
	var stored int
	err = st.DB.QueryRow(`SELECT count(*) FROM agent_messages WHERE conversation_id=$1 AND role='assistant'`, source.ConversationID).Scan(&stored)
	if err != nil || stored != 2 {
		t.Fatal("original evidence deleted", stored, err)
	}
	if _, err = svc.Retry(ctx, 1, source.ID, "stale-retry", "127.0.0.1", 1, "1"); SafeCode(err) != "AGENT_RETRY_NOT_LATEST" {
		t.Fatal("stale response retry accepted", err)
	}
}
func TestPostgresAgentRetryBlocksUnsettledCalls(t *testing.T) {
	st := isolatedStore(t)
	svc, _ := setupRun(t, st, &testExecutor{})
	ctx := context.Background()
	source, err := st.Claim(ctx, "retry-pending")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Retry(ctx, 1, source.ID, "while-running", "127.0.0.1", 1, "1"); !errors.Is(err, ErrBusy) {
		t.Fatal("active retry accepted", err)
	}
	svc.process(ctx, source)
	_, err = st.DB.Exec(`UPDATE agent_operations SET status='result_unknown' WHERE run_id=$1`, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Retry(ctx, 1, source.ID, "uncertain-retry", "127.0.0.1", 1, "1"); SafeCode(err) != "AGENT_RETRY_UNSETTLED" {
		t.Fatal("uncertain paid call replayed", err)
	}
}

func TestPostgresAgentRetryRehydratesLegacyImageInput(t *testing.T) {
	st := isolatedStore(t)
	svc := New(st, &testExecutor{}, t.TempDir(), true)
	ctx := context.Background()
	cv, err := svc.Create(ctx, 1, "legacy image", Config{ModelKeyID: 1, Model: "text", Budget: "1"}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	a, err := svc.UploadFile(ctx, 1, cv.ID, "image.png", testImage())
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Submit(ctx, 1, cv.ID, "legacy-image", "127.0.0.1", "read image", []string{a.ID}, 1, "1")
	if err != nil {
		t.Fatal(err)
	}
	source, err := st.Claim(ctx, "legacy-image-test")
	if err != nil {
		t.Fatal(err)
	}
	source.History = []json.RawMessage{UserItem("read image - only attachment metadata")}
	source.Status = "failed"
	if err = st.Save(ctx, source, "run.failed", source.Public()); err != nil {
		t.Fatal(err)
	}
	retried, err := svc.Retry(ctx, 1, source.ID, "legacy-retry", "127.0.0.1", 1, "1")
	if err != nil {
		t.Fatal(err)
	}
	ids := ImageAssetIDs(retried.History)
	if len(ids) != 1 || ids[0] != a.ID {
		t.Fatal("legacy retry still sends metadata without pixels", ids)
	}
}
