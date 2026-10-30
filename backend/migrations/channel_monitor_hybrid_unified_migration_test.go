package migrations

import (
	"strings"
	"testing"
)

func TestChannelMonitorHybridUnifiedMigration(t *testing.T) {
	content, err := FS.ReadFile("261_channel_monitor_hybrid_unified.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	for _, want := range []string{
		"probe_fallback_enabled BOOLEAN NOT NULL DEFAULT TRUE",
		"intelligence_enabled BOOLEAN NOT NULL DEFAULT TRUE",
		"SET value = 'hybrid'",
		"ON CONFLICT (key) DO NOTHING",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
}
