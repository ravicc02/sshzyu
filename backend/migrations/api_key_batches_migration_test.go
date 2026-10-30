package migrations

import (
	"strings"
	"testing"
)

func TestAPIKeyBatchesMigration(t *testing.T) {
	content, err := FS.ReadFile("260_api_key_batches.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS batch_id VARCHAR(64)",
		"ADD COLUMN IF NOT EXISTS batch_name VARCHAR(100)",
		"CREATE INDEX IF NOT EXISTS idx_api_keys_user_batch",
		"WHERE batch_id IS NOT NULL AND deleted_at IS NULL",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
}
