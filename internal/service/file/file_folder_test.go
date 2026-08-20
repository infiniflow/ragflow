package file

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestFileService_CreateFolder_RejectsSlashInName(t *testing.T) {
	svc := testFileService()
	for _, name := range []string{"/", "a/b", "dir/sub/"} {
		_, err := svc.CreateFolder(context.Background(), "tenant1", name, "pf1", FileTypeFolder)
		if err == nil || !strings.Contains(err.Error(), `cannot contain "/"`) {
			t.Fatalf("CreateFolder(%q) error = %v, want slash validation error", name, err)
		}
	}
}

func TestFileService_MoveFiles_RejectsSlashInNewName(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Keep a single connection so the :memory: database is shared across
	// goroutines (sqlite :memory: is otherwise per-connection).
	if sqlDB, serr := db.DB(); serr == nil {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	}
	if err := db.AutoMigrate(&entity.File{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	old := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = old })

	folder := &entity.File{ID: "f1", ParentID: "pf1", TenantID: "tenant1", Name: "old", Type: FileTypeFolder}
	if err := db.Create(folder).Error; err != nil {
		t.Fatalf("seed folder: %v", err)
	}

	svc := testFileService()
	ok, msg := svc.MoveFiles(context.Background(), "tenant1", []string{"f1"}, "", "a/b")
	if ok || !strings.Contains(msg, `cannot contain "/"`) {
		t.Fatalf("MoveFiles rename = %v, %q, want slash validation error", ok, msg)
	}
}
