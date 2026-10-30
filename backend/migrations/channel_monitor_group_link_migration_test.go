package migrations

import (
	"strings"
	"testing"
)

func TestChannelMonitorGroupLinkMigration(t *testing.T) {
	content, err := FS.ReadFile("259_channel_monitor_group_link.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS group_id BIGINT",
		"REFERENCES groups(id) ON DELETE SET NULL",
		"CREATE INDEX IF NOT EXISTS idx_channel_monitors_group_id",
		"SET group_id = g.id",
		"AND g.deleted_at IS NULL",
		"AND g.name = cm.group_name",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
}
