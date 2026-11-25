//go:build !ee

package dao

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCompletionLogMigrationIsNoop(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := autoMigrateRuntimeModels(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable("completion_log") {
		t.Fatal("community migration created completion logs")
	}
}
