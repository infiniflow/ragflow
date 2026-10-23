//go:build integration
// +build integration

package dao

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

func assertModifyColumnTypesTablesAbsent(t *testing.T, db *gorm.DB, allowedTable string) {
	t.Helper()
	tables := append([]string{
		"dialog",
		"tenant_llm",
		"api_token",
		"canvas_template",
		"system_settings",
		"knowledgebase",
		"knowledge_compile_docs",
	}, tenantModelIDFieldTables()...)
	for _, table := range tables {
		if table == allowedTable {
			continue
		}
		if db.Migrator().HasTable(table) {
			t.Fatalf("%s already exists in this database; refusing to run modifyColumnTypes against a populated schema", table)
		}
	}
}

func knowledgeCompileBacklogFixture(count int) string {
	entries := make([]string, count)
	for i := range entries {
		entries[i] = fmt.Sprintf(`{"doc_id":"%032d","event_type":"doc_completed","variants":["structure"],"task_types":["Graph"]}`, i)
	}
	return "[" + strings.Join(entries, ",") + "]"
}

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
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get database connection pool: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("failed to close database connection pool: %v", err)
		}
	})

	ctx := context.Background()
	assertModifyColumnTypesTablesAbsent(t, db.WithContext(ctx), "dialog")

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
		if err := db.WithContext(context.Background()).Exec("DROP TABLE IF EXISTS dialog").Error; err != nil {
			t.Errorf("failed to drop dialog test table: %v", err)
		}
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

func TestMigrateKnowledgeCompileQueues_Integration(t *testing.T) {
	dsn := os.Getenv("RAGFLOW_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("RAGFLOW_TEST_MYSQL_DSN unset: this test needs a throwaway MySQL database")
	}

	db, err := gorm.Open(newMigrationAwareDialector(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get database connection pool: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("failed to close database connection pool: %v", err)
		}
	})
	ctx := context.Background()
	assertModifyColumnTypesTablesAbsent(t, db.WithContext(ctx), "knowledge_compile_docs")
	for _, model := range []interface{}{&entity.KnowledgeCompileBacklog{}, &entity.KnowledgeCompileInflight{}} {
		if db.Migrator().HasTable(model) {
			t.Fatalf("%T already exists; this test requires an empty database", model)
		}
	}
	if err = db.WithContext(ctx).Exec(`CREATE TABLE knowledge_compile_docs (
		dataset_id varchar(64) NOT NULL PRIMARY KEY,
		backlog_doc_ids longtext NOT NULL,
		inflight_doc_ids longtext NOT NULL
	)`).Error; err != nil {
		t.Fatalf("failed to create legacy knowledge_compile_docs: %v", err)
	}
	t.Cleanup(func() {
		for _, table := range []string{"knowledge_compile_docs", "knowledge_compile_docs_backlog", "knowledge_compile_docs_inflight"} {
			if err := db.WithContext(context.Background()).Migrator().DropTable(table); err != nil {
				t.Errorf("failed to drop %s test table: %v", table, err)
			}
		}
	})

	legacyBacklog := knowledgeCompileBacklogFixture(600)
	if len(legacyBacklog) <= 65535 {
		t.Fatalf("fixture is %d bytes, want more than TEXT's 65,535-byte limit", len(legacyBacklog))
	}
	if err = db.WithContext(ctx).Exec(
		"INSERT INTO knowledge_compile_docs (dataset_id, backlog_doc_ids, inflight_doc_ids) VALUES (?, ?, '[]')",
		"kb1", legacyBacklog,
	).Error; err != nil {
		t.Fatalf("failed to seed legacy backlog: %v", err)
	}
	if err := db.AutoMigrate(&entity.KnowledgeCompileBacklog{}, &entity.KnowledgeCompileInflight{}); err != nil {
		t.Fatal(err)
	}
	if err = migrateKnowledgeCompileQueues(ctx, db); err != nil {
		t.Fatalf("migrateKnowledgeCompileQueues failed: %v", err)
	}

	for _, column := range []string{"backlog_doc_ids", "inflight_doc_ids"} {
		if db.Migrator().HasColumn(&entity.KnowledgeCompileDataset{}, column) {
			t.Errorf("source column %s still exists", column)
		}
	}
	var entries []entity.KnowledgeCompileEntry
	if err := db.Table((entity.KnowledgeCompileBacklog{}).TableName()).Order("queue_position").Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	if len(entries) != 600 {
		t.Fatalf("migrated %d events, want 600", len(entries))
	}
	for i, entry := range entries {
		if entry.DocID != fmt.Sprintf("%032d", i) || entry.QueuePosition != int64(i+1) || entry.EventType != "doc_completed" || len(entry.TaskTypes) != 1 || entry.TaskTypes[0] != "Graph" {
			t.Fatalf("unexpected event %d: %+v", i, entry)
		}
	}
	for _, table := range []string{"knowledge_compile_docs_backlog", "knowledge_compile_docs_inflight"} {
		var count int64
		if err := db.Raw("SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND non_unique = 0", table).Scan(&count).Error; err != nil || count != 0 {
			t.Fatalf("%s must have no primary key or unique indexes: count=%d err=%v", table, count, err)
		}
	}
}
