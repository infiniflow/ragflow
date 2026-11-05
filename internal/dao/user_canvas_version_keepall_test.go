package dao

import (
	"context"
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// TestCanvasVersionKeepAllRetainsUnpublished verifies the design decision to keep
// every version row (published or unpublished) instead of auto-trimming old draft
// versions. Inserting more draft versions than the old 20-row trim limit must
// retain them all rather than deleting the oldest.
func TestCanvasVersionKeepAllRetainsUnpublished(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const canvasID = "keepall"
	seedCanvasLimit(t, db, canvasID)

	d := NewUserCanvasVersionDAO()
	const n = 30
	oldestID := canvasID + "-v-0"
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%s-v-%d", canvasID, i)
		if _, err := d.SaveOrReplaceLatestTx(context.Background(), db, SaveOrReplaceLatestVersionOptions{
			NewID:        id,
			UserCanvasID: canvasID,
			Title:        ptrStr("draft"),
			DSL:          entity.JSONMap{"n": i}, // distinct DSL -> create branch
			Release:      false,                  // unpublished / draft
			ForceNew:     true,
		}); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	got := countVersions(t, db, canvasID)
	if got != n {
		t.Fatalf("keep-all: expected all %d unpublished versions retained, got %d (trimming still active?)\n", n, got)
	}
	// The oldest draft must still be reachable: keep-all never auto-deletes rows.
	var oldest entity.UserCanvasVersion
	if err := db.WithContext(context.Background()).Where("id = ?", oldestID).First(&oldest).Error; err != nil {
		t.Fatalf("oldest draft %s was auto-deleted: %v", oldestID, err)
	}
}

// TestCanvasVersionKeepAllRetainsBeyondOldTrim inserts past the old 20-row trim
// boundary plus one and confirms growth is unbounded (bounded only by K=64).
func TestCanvasVersionKeepAllRetainsBeyondOldTrim(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const canvasID = "keepall-extra"
	seedCanvasLimit(t, db, canvasID)

	d := NewUserCanvasVersionDAO()
	const n = 21
	for i := 0; i < n; i++ {
		if _, err := d.SaveOrReplaceLatestTx(context.Background(), db, SaveOrReplaceLatestVersionOptions{
			NewID:        fmt.Sprintf("%s-v-%d", canvasID, i),
			UserCanvasID: canvasID,
			Title:        ptrStr("draft"),
			DSL:          entity.JSONMap{"n": i},
			Release:      false,
			ForceNew:     true,
		}); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}
	if got := countVersions(t, db, canvasID); got != n {
		t.Fatalf("keep-all: expected %d retained, got %d (old 20-row trim still active?)\n", n, got)
	}
}
