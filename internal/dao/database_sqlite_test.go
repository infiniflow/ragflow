package dao

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func newSQLiteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// :memory: is per connection; a single pooled connection keeps the schema
	// visible to every statement.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// migrateSQLiteStore is what the desktop build runs instead of AutoMigrate: it
// has to work on both a fresh file and an existing one, because AutoMigrate's
// rebuild path fails on the latter.
func TestMigrateSQLiteStoreIsIdempotent(t *testing.T) {
	db := newSQLiteTestDB(t)
	ctx := context.Background()
	models := []interface{}{&entity.User{}, &entity.Tenant{}}

	for pass := 1; pass <= 2; pass++ {
		if err := migrateSQLiteStore(ctx, db, models); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
	}

	if !db.Migrator().HasTable(&entity.User{}) {
		t.Fatal("user table was not created")
	}
	if !db.Migrator().HasColumn(&entity.User{}, "last_login_tenant") {
		t.Fatal("user.last_login_tenant is missing")
	}
	// The index has to exist too, or the second pass would have failed on a
	// duplicate CREATE INDEX instead.
	if !db.Migrator().HasIndex(&entity.User{}, "idx_user_nickname") {
		t.Fatal("user nickname index is missing")
	}
}

// A column the model gained after the file was created is added; the rest of
// the table is left alone, which is all SQLite can do.
func TestMigrateSQLiteStoreAddsMissingColumns(t *testing.T) {
	db := newSQLiteTestDB(t)
	ctx := context.Background()

	if err := db.Exec(`CREATE TABLE user (id text primary key)`).Error; err != nil {
		t.Fatalf("seed table: %v", err)
	}
	if err := migrateSQLiteStore(ctx, db, []interface{}{&entity.User{}}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !db.Migrator().HasColumn(&entity.User{}, "email") {
		t.Fatal("email column was not added")
	}
	if !db.Migrator().HasColumn(&entity.User{}, "create_date") {
		t.Fatal("create_date column was not added")
	}
}
