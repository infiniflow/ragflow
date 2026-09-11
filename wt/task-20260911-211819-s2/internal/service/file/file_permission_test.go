package file

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func setupPermissionDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.File{},
		&entity.File2Document{},
		&entity.Document{},
		&entity.Knowledgebase{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	old := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = old })
}

// checkFileTeamPermissionStub is a minimal permission checker for tests
// that avoids importing the parent service package (which would create a
// cycle in the test binary).
func checkFileTeamPermissionStub(_ *dao.FileDAO, file *entity.File, userID string) bool {
	return file.TenantID == userID
}

func TestCheckFileTeamPermissionStub(t *testing.T) {
	setupPermissionDB(t)

	// Direct tenant match short-circuits before any DB lookup.
	if !checkFileTeamPermissionStub(nil, &entity.File{TenantID: "u1"}, "u1") {
		t.Error("file tenant match should be authorized")
	}

	// A file owned by another tenant is denied for a different user.
	if checkFileTeamPermissionStub(nil, &entity.File{TenantID: "other"}, "u1") {
		t.Error("file with different tenant should be denied")
	}
}
