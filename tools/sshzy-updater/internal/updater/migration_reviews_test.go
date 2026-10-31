package updater

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/release"
)

func TestRepositoryMigrationReviewsMatchCommittedSQL(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate repository root")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "../../../.."))
	reviewData, err := os.ReadFile(filepath.Join(root, "scripts/release/migration-reviews.json"))
	if err != nil {
		t.Fatal(err)
	}
	var reviews map[string]release.Migration
	if err := json.Unmarshal(reviewData, &reviews); err != nil {
		t.Fatal(err)
	}
	for name, review := range reviews {
		content, err := command(context.Background(), "git", "-C", root, "show", "HEAD:backend/migrations/"+name)
		if err != nil {
			t.Errorf("%s: read committed SQL: %v", name, err)
			continue
		}
		if checksum := release.MigrationChecksum(string(content)); checksum != review.Checksum {
			t.Errorf("%s: review checksum %s, committed SQL checksum %s", name, review.Checksum, checksum)
		}
	}
}
