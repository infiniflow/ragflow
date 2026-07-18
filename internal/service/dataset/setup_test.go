package dataset

import (
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupServiceTestDB initializes an in-memory SQLite database for tests.
func setupServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err = db.AutoMigrate(
		&entity.Document{},
		&entity.Knowledgebase{},
		&entity.Task{},
		&entity.IngestionTask{},
		&entity.IngestionTaskLog{},
		&entity.File2Document{},
		&entity.File{},
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.API4Conversation{},
		&entity.Connector{},
		&entity.Connector2Kb{},
		&entity.SyncLogs{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
		&entity.UserCanvas{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// pushServiceDB swaps dao.DB for the test and restores after.
func pushServiceDB(t *testing.T, testDB *gorm.DB) {
	t.Helper()
	oldDB := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = oldDB })
}

func sptr(s string) *string { return &s }
