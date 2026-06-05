package dao

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func setupUserCanvasVersionTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func TestGetByID_Success(t *testing.T) {
	db := setupUserCanvasVersionTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	dao := NewUserCanvasVersionDAO()

	expectedVersion := &entity.UserCanvasVersion{
		ID:           "version-1",
		UserCanvasID: "canvas-1",
		Release:      true,
	}

	if err := db.Create(expectedVersion).Error; err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	version, err := dao.GetByID("version-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if version == nil || version.ID != "version-1" {
		t.Fatalf("expected to find version-1, got %v", version)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	db := setupUserCanvasVersionTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	dao := NewUserCanvasVersionDAO()

	version, err := dao.GetByID("non-existent-id")
	if err == nil {
		t.Fatalf("expected error for non-existent id, got nil")
	}

	if version != nil {
		t.Fatalf("expected returned version to be nil, got %v", version)
	}
}

func TestGetLatestReleasedVersion(t *testing.T) {
	db := setupUserCanvasVersionTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	dao := NewUserCanvasVersionDAO()

	baseTime := time.Now().Unix()

	versions := []*entity.UserCanvasVersion{
		{ID: "v1", UserCanvasID: "canvas-1", Release: true, BaseModel: entity.BaseModel{UpdateTime: ptr(baseTime)}},
		{ID: "v2", UserCanvasID: "canvas-1", Release: false, BaseModel: entity.BaseModel{UpdateTime: ptr(baseTime + 100)}}, // Not released, should be ignored
		{ID: "v3", UserCanvasID: "canvas-1", Release: true, BaseModel: entity.BaseModel{UpdateTime: ptr(baseTime + 200)}},  // Latest released
		{ID: "v4", UserCanvasID: "canvas-2", Release: true, BaseModel: entity.BaseModel{UpdateTime: ptr(baseTime + 300)}},  // Different canvas
	}

	for _, v := range versions {
		if err := db.Create(v).Error; err != nil {
			t.Fatalf("failed to insert test data: %v", err)
		}
	}

	version, err := dao.GetLatestReleasedVersion("canvas-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if version == nil {
		t.Fatalf("expected to find a latest version")
	}

	if version.ID != "v3" {
		t.Fatalf("expected latest released version to be v3, got %s", version.ID)
	}
}

func TestGetLatestReleasedVersion_NotFound(t *testing.T) {
	db := setupUserCanvasVersionTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	dao := NewUserCanvasVersionDAO()

	// Insert only an unreleased version
	unreleased := &entity.UserCanvasVersion{
		ID:           "v1",
		UserCanvasID: "canvas-1",
		Release:      false,
		BaseModel: entity.BaseModel{
			UpdateTime: ptr(time.Now().Unix()),
		},
	}
	if err := db.Create(unreleased).Error; err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	version, err := dao.GetLatestReleasedVersion("canvas-1")
	if err == nil {
		t.Fatalf("expected error (not found), got nil")
	}

	if version != nil {
		t.Fatalf("expected returned version to be nil, got %v", version)
	}
}

// ptr returns a pointer to the given int64.
func ptr(v int64) *int64 { return &v }
