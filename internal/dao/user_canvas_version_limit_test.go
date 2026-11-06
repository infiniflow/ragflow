package dao

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// seedCanvas creates the parent canvas row that SaveOrReplaceLatestTx locks.
func seedCanvasLimit(t *testing.T, db *gorm.DB, canvasID string) {
	t.Helper()
	if err := db.Create(&entity.UserCanvas{ID: canvasID, UserID: "user-1"}).Error; err != nil {
		t.Fatalf("seed canvas: %v", err)
	}
}

// seedVersions inserts n released version rows directly so the test starts at a
// known count without going through SaveOrReplaceLatestTx. All versions are kept
// (keep-all policy): no version row is ever auto-deleted on write, so count
// assertions stay exact regardless of release state.
func seedVersions(t *testing.T, db *gorm.DB, canvasID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		v := &entity.UserCanvasVersion{
			ID:           fmt.Sprintf("%s-v-%d", canvasID, i),
			UserCanvasID: canvasID,
			Release:      true,
			DSL:          entity.JSONMap{"n": i},
		}
		if err := db.Create(v).Error; err != nil {
			t.Fatalf("seed version %d: %v", i, err)
		}
	}
}

func countVersions(t *testing.T, db *gorm.DB, canvasID string) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&entity.UserCanvasVersion{}).Where("user_canvas_id = ?", canvasID).Count(&n).Error; err != nil {
		t.Fatalf("count versions: %v", err)
	}
	return n
}

// TestCanvasVersionLimit_RejectsAtCap verifies that once a canvas holds the
// maximum number of versions, a NEW version write (ForceNew) is rejected and NO
// row is added. This is the core off-by-one: the 64th version is allowed, the
// 65th is not.
func TestCanvasVersionLimit_RejectsAtCap(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const canvasID = "cap-full"
	seedCanvasLimit(t, db, canvasID)
	seedVersions(t, db, canvasID, MaxCanvasVersions)

	before := countVersions(t, db, canvasID)
	if before != MaxCanvasVersions {
		t.Fatalf("seed count = %d, want %d", before, MaxCanvasVersions)
	}

	d := NewUserCanvasVersionDAO()
	_, err = d.SaveOrReplaceLatestTx(context.Background(), db, SaveOrReplaceLatestVersionOptions{
		NewID:        canvasID + "-v-new",
		UserCanvasID: canvasID,
		Title:        ptrStr("new"),
		DSL:          entity.JSONMap{"n": 9999}, // distinct DSL -> create branch
		Release:      true,
		ForceNew:     true,
	})
	if err == nil {
		t.Fatalf("expected rejection when at cap, got nil error")
	}
	if !errors.Is(err, ErrCanvasVersionLimitExceeded) {
		t.Fatalf("cap rejection must return ErrCanvasVersionLimitExceeded, got: %v", err)
	}
	if !strings.Contains(err.Error(), "version limit reached") {
		t.Fatalf("cap error must surface the English hint, got: %v", err)
	}
	after := countVersions(t, db, canvasID)
	if after != before {
		t.Fatalf("rejection must not add a row: before=%d after=%d", before, after)
	}
}

// TestCanvasVersionLimit_AllowsBelowCap verifies that one version below the cap
// still accepts a new write (count becomes exactly the cap).
func TestCanvasVersionLimit_AllowsBelowCap(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const canvasID = "cap-below"
	seedCanvasLimit(t, db, canvasID)
	seedVersions(t, db, canvasID, MaxCanvasVersions-1)

	d := NewUserCanvasVersionDAO()
	_, err = d.SaveOrReplaceLatestTx(context.Background(), db, SaveOrReplaceLatestVersionOptions{
		NewID:        canvasID + "-v-new",
		UserCanvasID: canvasID,
		Title:        ptrStr("new"),
		DSL:          entity.JSONMap{"n": 9999},
		Release:      true,
		ForceNew:     true,
	})
	if err != nil {
		t.Fatalf("expected success below cap, got: %v", err)
	}
	after := countVersions(t, db, canvasID)
	if after != MaxCanvasVersions {
		t.Fatalf("count after allowed write = %d, want %d", after, MaxCanvasVersions)
	}
}

// TestCanvasVersionLimit_RefreshInPlaceAllowedAtCap verifies the cap only blocks
// NEW rows. An in-place DSL refresh (ForceNew=false, same DSL) does not consume
// the budget and must succeed even at the cap.
func TestCanvasVersionLimit_RefreshInPlaceAllowedAtCap(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const canvasID = "cap-refresh"
	seedCanvasLimit(t, db, canvasID)
	seedVersions(t, db, canvasID, MaxCanvasVersions)

	d := NewUserCanvasVersionDAO()
	_, err = d.SaveOrReplaceLatestTx(context.Background(), db, SaveOrReplaceLatestVersionOptions{
		NewID:        canvasID + "-v-new",
		UserCanvasID: canvasID,
		Title:        ptrStr("refreshed"),
		DSL:          entity.JSONMap{"n": 0}, // ignored: SameDSL forces in-place
		Release:      true,
		ForceNew:     false,
		SameDSL:      func(entity.JSONMap) bool { return true },
	})
	if err != nil {
		t.Fatalf("in-place refresh at cap should succeed, got: %v", err)
	}
	after := countVersions(t, db, canvasID)
	if after != MaxCanvasVersions {
		t.Fatalf("refresh must not add a row: count=%d want %d", after, MaxCanvasVersions)
	}
}

// TestCanvasVersionLimit_PerCanvasIsolation verifies the cap is enforced per
// canvas: a full canvas must not block a different (empty) canvas.
func TestCanvasVersionLimit_PerCanvasIsolation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const full = "iso-full"
	const empty = "iso-empty"
	seedCanvasLimit(t, db, full)
	seedCanvasLimit(t, db, empty)
	seedVersions(t, db, full, MaxCanvasVersions)

	d := NewUserCanvasVersionDAO()
	_, err = d.SaveOrReplaceLatestTx(context.Background(), db, SaveOrReplaceLatestVersionOptions{
		NewID:        empty + "-v-new",
		UserCanvasID: empty,
		Title:        ptrStr("new"),
		DSL:          entity.JSONMap{"n": 1},
		Release:      true,
		ForceNew:     true,
	})
	if err != nil {
		t.Fatalf("empty canvas should accept a write despite another canvas being full: %v", err)
	}
	if got := countVersions(t, db, empty); got != 1 {
		t.Fatalf("empty canvas count = %d, want 1", got)
	}
	if got := countVersions(t, db, full); got != MaxCanvasVersions {
		t.Fatalf("full canvas count changed = %d, want %d", got, MaxCanvasVersions)
	}
}

// TestCanvasVersionLimit_AliasPatchUnaffected verifies that the cap does not
// interfere with metadata-only version PATCH (alias edit), which never creates a
// version row and must succeed even when the canvas is at the cap.
func TestCanvasVersionLimit_AliasPatchUnaffected(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const canvasID = "cap-patch"
	seedCanvasLimit(t, db, canvasID)
	seedVersions(t, db, canvasID, MaxCanvasVersions)

	alias := "pinned"
	d := NewUserCanvasVersionDAO()
	if err := d.UpdateTx(context.Background(), db, canvasID+"-v-0", UserCanvasVersionPatch{Alias: &alias}); err != nil {
		t.Fatalf("alias patch at cap should succeed, got: %v", err)
	}
	if got := countVersions(t, db, canvasID); got != MaxCanvasVersions {
		t.Fatalf("alias patch must not add a row: count=%d want %d", got, MaxCanvasVersions)
	}
}
