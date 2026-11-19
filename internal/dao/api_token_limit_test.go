package dao

import (
	"errors"
	"fmt"
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupAPITokenLimitDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&entity.Tenant{}, &entity.APIToken{}, &entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"tenant-1", "tenant-2"} {
		if err := db.Create(&entity.Tenant{ID: id, ParserIDs: ""}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestAPITokenLimitDefaultAndTenantScope(t *testing.T) {
	db := setupAPITokenLimitDB(t)
	for i := range DefaultAPITokenLimit {
		if err := NewAPITokenDAO().Create(t.Context(), db, &entity.APIToken{TenantID: "tenant-1", Token: fmt.Sprint(i)}); err != nil {
			t.Fatal(err)
		}
	}
	err := NewAPITokenDAO().Create(t.Context(), db, &entity.APIToken{TenantID: "tenant-1", Token: "overflow"})
	if !errors.Is(err, ErrAPITokenLimitReached) {
		t.Fatalf("expected quota error, got %v", err)
	}
	if err := NewAPITokenDAO().Create(t.Context(), db, &entity.APIToken{TenantID: "tenant-2", Token: "separate"}); err != nil {
		t.Fatalf("other tenant rejected: %v", err)
	}
	if _, err := NewAPITokenDAO().DeleteByTenantIDAndToken(t.Context(), db, "tenant-1", "0"); err != nil {
		t.Fatal(err)
	}
	if err := NewAPITokenDAO().Create(t.Context(), db, &entity.APIToken{TenantID: "tenant-1", Token: "replacement"}); err != nil {
		t.Fatalf("deleted key did not release capacity: %v", err)
	}
}

func TestAPITokenLimitDatabaseErrors(t *testing.T) {
	for _, table := range []string{"tenant", "api_token"} {
		t.Run(table, func(t *testing.T) {
			db := setupAPITokenLimitDB(t)
			if err := db.Migrator().DropTable(table); err != nil {
				t.Fatal(err)
			}
			err := NewAPITokenDAO().Create(t.Context(), db, &entity.APIToken{TenantID: "tenant-1", Token: "failed"})
			if err == nil || errors.Is(err, ErrAPITokenLimitReached) {
				t.Fatalf("expected database error, got %v", err)
			}
		})
	}
}
