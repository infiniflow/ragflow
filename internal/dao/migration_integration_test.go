//go:build integration
// +build integration

package dao

import (
	"context"
	"os"
	"testing"

	"gorm.io/gorm"
)

// TestModifyColumnTypesHealsLegacyTenantModelIDColumns_Integration reproduces
// the schema that blocks startup: tenant model id columns created while the Go
// entities declared them *int64, which leaves MySQL holding bigint. No
// migration step converts them back, so AutoMigrate ends up being the one that
// tries and fails.
//
// The specs name real tables, so this needs a throwaway database rather than a
// shared one. Finding any of those tables already present aborts before any DDL
// lands, which is what keeps the DROP in t.Cleanup from ever touching a live
// schema.
//
// Run with:
//
//	RAGFLOW_TEST_MYSQL_DSN='root:infini_rag_flow@tcp(127.0.0.1:3306)/ragflow_test?charset=utf8mb4&parseTime=True&loc=Local' \
//	  bash build.sh --test-integration ./internal/dao/...
func TestModifyColumnTypesHealsLegacyTenantModelIDColumns_Integration(t *testing.T) {
	dsn := os.Getenv("RAGFLOW_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("RAGFLOW_TEST_MYSQL_DSN unset: this test needs a throwaway MySQL database")
	}

	db, err := gorm.Open(newMigrationAwareDialector(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	ctx := context.Background()
	for _, table := range tenantModelIDFieldTables() {
		if db.WithContext(ctx).Migrator().HasTable(table) {
			t.Fatalf("%s already exists in this database; refusing to run against a live schema", table)
		}
	}

	create := `CREATE TABLE dialog (
		id varchar(32) NOT NULL,
		llm_id varchar(128) NOT NULL DEFAULT '',
		rerank_id varchar(128) NOT NULL DEFAULT '',
		tenant_llm_id bigint DEFAULT NULL,
		tenant_rerank_id bigint DEFAULT NULL,
		PRIMARY KEY (id)
	)`
	if err = db.WithContext(ctx).Exec(create).Error; err != nil {
		t.Fatalf("failed to create legacy dialog: %v", err)
	}
	t.Cleanup(func() {
		_ = db.WithContext(context.Background()).Exec("DROP TABLE IF EXISTS dialog").Error
	})

	// llm_id is empty on purpose: this row would not be backfilled even if the
	// version gate let populateTenantModelIDColumns through, so the type repair
	// is its only chance to stop holding a dead reference.
	if err = db.WithContext(ctx).Exec(
		"INSERT INTO dialog (id, llm_id, rerank_id, tenant_llm_id, tenant_rerank_id) VALUES ('d1', '', '', 3, 7)",
	).Error; err != nil {
		t.Fatalf("failed to seed dialog: %v", err)
	}

	if err = modifyColumnTypes(ctx, db); err != nil {
		t.Fatalf("modifyColumnTypes failed: %v", err)
	}

	for _, column := range []string{"tenant_llm_id", "tenant_rerank_id"} {
		columnType, _, ok, err := columnDefinition(ctx, db, "dialog", column)
		if err != nil {
			t.Fatalf("failed to read %s definition: %v", column, err)
		}
		if !ok {
			t.Fatalf("%s vanished from dialog", column)
		}
		if columnType != "varchar(32)" {
			t.Errorf("%s type = %q, want varchar(32)", column, columnType)
		}
	}

	var remaining int64
	if err = db.WithContext(ctx).Raw(
		"SELECT COUNT(1) FROM dialog WHERE tenant_llm_id IS NOT NULL OR tenant_rerank_id IS NOT NULL",
	).Scan(&remaining).Error; err != nil {
		t.Fatalf("failed to count remaining references: %v", err)
	}
	if remaining != 0 {
		t.Errorf("%d rows still carry an orphaned integer model id", remaining)
	}
}
