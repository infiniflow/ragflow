//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

//go:build integration
// +build integration

package dao

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"
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

	wantType := fmt.Sprintf("varchar(%d)", common.GroupModelRefWidth)
	for _, column := range []string{"tenant_llm_id", "tenant_rerank_id"} {
		columnType, _, ok, err := columnDefinition(ctx, db, "dialog", column)
		if err != nil {
			t.Fatalf("failed to read %s definition: %v", column, err)
		}
		if !ok {
			t.Fatalf("%s vanished from dialog", column)
		}
		if columnType != wantType {
			t.Errorf("%s type = %q, want %q", column, columnType, wantType)
		}
	}

	// The type repair and the width migration own the same columns. A repair
	// that converged on a narrower type would be undone here and then redone on
	// the next startup, which is what made every boot issue DDL on the same
	// columns. Both must be no-ops from this point on.
	if err = migrateColumnWidths(ctx, db); err != nil {
		t.Fatalf("migrateColumnWidths failed: %v", err)
	}
	if err = modifyColumnTypes(ctx, db); err != nil {
		t.Fatalf("second modifyColumnTypes failed: %v", err)
	}
	for _, column := range []string{"tenant_llm_id", "tenant_rerank_id"} {
		columnType, _, _, err := columnDefinition(ctx, db, "dialog", column)
		if err != nil {
			t.Fatalf("failed to re-read %s definition: %v", column, err)
		}
		if columnType != wantType {
			t.Errorf("%s type after a full pass = %q, want %q", column, columnType, wantType)
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

// TestRunMigrationsKeepsModelReferencesWhenWidening_Integration runs the whole
// startup migration pass over the schema that predates gp- group references: the
// model-reference column is still varchar(32) and already holds a 32-character
// model id. Widening it to varchar(35) changes the column type, and a type change
// is what triggers the orphan sweep, so a sweep keyed on the new width alone
// would erase every valid legacy id. The pass must keep that id, and it must keep
// the gp- reference the wider column exists for.
func TestRunMigrationsKeepsModelReferencesWhenWidening_Integration(t *testing.T) {
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
	if db.WithContext(ctx).Migrator().HasTable("dialog") {
		t.Skip("test database already contains dialog; use a throwaway database")
	}

	// tenant_rerank_id starts wider than tenant_llm_id because a varchar(32)
	// column cannot store the 35-character reference this test protects.
	create := `CREATE TABLE dialog (
		id varchar(32) NOT NULL,
		tenant_llm_id varchar(32) DEFAULT NULL,
		tenant_rerank_id varchar(35) DEFAULT NULL,
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
	// RunMigrations covers far more than these columns, so it also creates
	// tables this test never touches. Every other test in this file asserts that
	// its tables are absent before it starts, so the pass has to leave the
	// schema as empty as it found it.
	tablesBefore := tableNames(t, db)
	t.Cleanup(func() { dropTablesAddedSince(t, db, tablesBefore) })

	legacyID := strings.Repeat("a", tenantModelIDLength)
	groupRef := "gp-" + strings.Repeat("b", tenantModelIDLength)
	if len(groupRef) != int(common.GroupModelRefWidth) {
		t.Fatalf("group ref length = %d, want %d", len(groupRef), common.GroupModelRefWidth)
	}
	if err = db.WithContext(ctx).Exec(
		"INSERT INTO dialog (id, tenant_llm_id, tenant_rerank_id) VALUES ('d1', ?, ?)",
		legacyID, groupRef,
	).Error; err != nil {
		t.Fatalf("failed to seed dialog: %v", err)
	}

	// Run twice: the first pass widens the legacy column, the second proves a
	// steady-state startup neither re-types nor re-sweeps it.
	for pass := 1; pass <= 2; pass++ {
		if err = RunMigrations(ctx, db); err != nil {
			t.Fatalf("RunMigrations pass %d failed: %v", pass, err)
		}
		for _, column := range []string{"tenant_llm_id", "tenant_rerank_id"} {
			if got := columnLength(t, db, &entity.Chat{}, column); got != common.GroupModelRefWidth {
				t.Errorf("pass %d: %s width = %d, want %d", pass, column, got, common.GroupModelRefWidth)
			}
		}

		var storedLLM, storedRerank string
		if err = db.WithContext(ctx).Raw(
			"SELECT tenant_llm_id, tenant_rerank_id FROM dialog WHERE id = 'd1'",
		).Row().Scan(&storedLLM, &storedRerank); err != nil {
			t.Fatalf("pass %d: read dialog: %v", pass, err)
		}
		if storedLLM != legacyID {
			t.Errorf("pass %d: 32-character model id became %q", pass, storedLLM)
		}
		if storedRerank != groupRef {
			t.Errorf("pass %d: gp- reference became %q", pass, storedRerank)
		}
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
		for _, table := range []string{"knowledge_compile_docs", "knowledge_compile_docs_backlog", "knowledge_compile_docs_inflight", "system_settings"} {
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
	if err := db.AutoMigrate(&entity.KnowledgeCompileBacklog{}, &entity.KnowledgeCompileInflight{}, &entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	if err := setDatabaseMigrationVersion(ctx, db, tenantDefaultModelsTargetVersion); err != nil {
		t.Fatal(err)
	}
	if err = migrateKnowledgeCompileQueues(ctx, db); err != nil {
		t.Fatalf("migrateKnowledgeCompileQueues failed: %v", err)
	}
	if version, err := GetDatabaseMigrationVersion(ctx, db); err != nil || version != knowledgeCompileQueueTargetVersion {
		t.Fatalf("migration version=%q want=%q err=%v", version, knowledgeCompileQueueTargetVersion, err)
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

func openIngestionTaskMigrationDB(t *testing.T) (*gorm.DB, context.Context) {
	t.Helper()

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
	return db, context.Background()
}

func createDuplicateIngestionTaskSchema(t *testing.T, db *gorm.DB, ctx context.Context, withPipelineLog bool) {
	t.Helper()

	tables := []string{"ingestion_task"}
	if withPipelineLog {
		tables = append(tables, "pipeline_operation_log")
	}
	for _, table := range tables {
		if db.Migrator().HasTable(table) {
			t.Fatalf("%s already exists; this test requires an empty database", table)
		}
	}
	t.Cleanup(func() {
		for _, table := range tables {
			if err := db.WithContext(context.Background()).Migrator().DropTable(table); err != nil {
				t.Errorf("failed to drop %s test table: %v", table, err)
			}
		}
	})
	if err := db.WithContext(ctx).Exec(`CREATE TABLE ingestion_task (
		id varchar(32) NOT NULL PRIMARY KEY,
		document_id varchar(32) NOT NULL,
		status varchar(32) NOT NULL,
		create_time bigint NULL,
		pipeline_log_id varchar(32) NULL
	)`).Error; err != nil {
		t.Fatalf("create ingestion_task: %v", err)
	}
	if err := db.WithContext(ctx).Exec(
		"CREATE INDEX idx_ingestion_task_document_id ON ingestion_task(document_id)",
	).Error; err != nil {
		t.Fatalf("create legacy document index: %v", err)
	}
	if !withPipelineLog {
		return
	}
	if err := db.WithContext(ctx).Exec(`CREATE TABLE pipeline_operation_log (
		id varchar(32) NOT NULL PRIMARY KEY,
		document_id varchar(32) NOT NULL,
		operation_status varchar(32) NOT NULL
	)`).Error; err != nil {
		t.Fatalf("create pipeline_operation_log: %v", err)
	}
}

func TestMigrateIngestionTaskDocumentIDUnique_Integration(t *testing.T) {
	db, ctx := openIngestionTaskMigrationDB(t)

	t.Run("repairs terminal duplicates", func(t *testing.T) {
		createDuplicateIngestionTaskSchema(t, db, ctx, true)
		if err := db.WithContext(ctx).Exec(
			"INSERT INTO pipeline_operation_log (id, document_id, operation_status) VALUES (?, ?, ?)",
			"run-done", "doc-duplicate", string(entity.TaskStatusDone),
		).Error; err != nil {
			t.Fatalf("seed completed pipeline log: %v", err)
		}
		if err := db.WithContext(ctx).Exec(`INSERT INTO ingestion_task
			(id, document_id, status, create_time, pipeline_log_id)
			VALUES
			('task-old', 'doc-duplicate', 'COMPLETED', 100, 'run-done'),
			('task-new', 'doc-duplicate', 'FAILED', 200, NULL)`).Error; err != nil {
			t.Fatalf("seed duplicate ingestion tasks: %v", err)
		}

		if err := migrateIngestionTaskDocumentIDUnique(ctx, db); err != nil {
			t.Fatalf("migrateIngestionTaskDocumentIDUnique: %v", err)
		}
		var ids []string
		if err := db.WithContext(ctx).Table("ingestion_task").Pluck("id", &ids).Error; err != nil {
			t.Fatalf("list remaining ingestion tasks: %v", err)
		}
		if !reflect.DeepEqual(ids, []string{"task-new"}) {
			t.Fatalf("remaining task ids = %v, want [task-new]", ids)
		}
		hasUnique, err := hasUniqueIndex(ctx, db, "ingestion_task", []string{"document_id"})
		if err != nil {
			t.Fatalf("check unique document index: %v", err)
		}
		if !hasUnique {
			t.Fatal("ingestion_task.document_id is not unique after migration")
		}
		if err := migrateIngestionTaskDocumentIDUnique(ctx, db); err != nil {
			t.Fatalf("second migrateIngestionTaskDocumentIDUnique: %v", err)
		}
	})

	t.Run("rejects active duplicate", func(t *testing.T) {
		createDuplicateIngestionTaskSchema(t, db, ctx, false)
		if err := db.WithContext(ctx).Exec(`INSERT INTO ingestion_task
			(id, document_id, status, create_time, pipeline_log_id)
			VALUES
			('task-active', 'doc-duplicate', 'RUNNING', 100, NULL),
			('task-new', 'doc-duplicate', 'COMPLETED', 200, NULL)`).Error; err != nil {
			t.Fatalf("seed duplicate ingestion tasks: %v", err)
		}

		err := migrateIngestionTaskDocumentIDUnique(ctx, db)
		for _, want := range []string{"doc-duplicate", "task-active", common.RUNNING} {
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("migration error = %v, want it to contain %q", err, want)
			}
		}
		var count int64
		if err := db.WithContext(ctx).Table("ingestion_task").Count(&count).Error; err != nil {
			t.Fatalf("count ingestion tasks: %v", err)
		}
		if count != 2 {
			t.Fatalf("ingestion task count = %d, want 2 after failed migration", count)
		}
		hasUnique, err := hasUniqueIndex(ctx, db, "ingestion_task", []string{"document_id"})
		if err != nil {
			t.Fatalf("check unique document index: %v", err)
		}
		if hasUnique {
			t.Fatal("ingestion_task.document_id unexpectedly became unique")
		}
	})
}

// TestMigrateColumnWidths_Integration verifies that the legacy 32-character
// pipeline_title, 100-character tenant.name, and 32-character model-reference
// columns are widened in place and keep their existing values. It needs a
// throwaway database because it creates and drops the real table names.
func TestMigrateColumnWidths_Integration(t *testing.T) {
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

	if db.Migrator().HasTable("pipeline_operation_log") || db.Migrator().HasTable("tenant") || db.Migrator().HasTable("user_tenant") {
		t.Skip("test database already contains pipeline_operation_log, tenant, or user_tenant; use a throwaway database")
	}
	t.Cleanup(func() {
		db.Exec("DROP TABLE IF EXISTS pipeline_operation_log")
		db.Exec("DROP TABLE IF EXISTS tenant")
		db.Exec("DROP TABLE IF EXISTS user_tenant")
	})

	ctx := context.Background()
	if err := db.Exec(`CREATE TABLE pipeline_operation_log (
		id varchar(32) PRIMARY KEY,
		pipeline_title varchar(32) NULL,
		KEY idx_pipeline_operation_log_pipeline_title (pipeline_title)
	)`).Error; err != nil {
		t.Fatalf("create pipeline_operation_log: %v", err)
	}
	if err := db.Exec(`CREATE TABLE tenant (
		id varchar(32) PRIMARY KEY,
		name varchar(100) NULL
	)`).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := db.Exec(`CREATE TABLE user_tenant (
		id varchar(32) PRIMARY KEY,
		user_id varchar(32) NULL,
		tenant_id varchar(32) NULL,
		tenant_llm_id varchar(32) NULL
	)`).Error; err != nil {
		t.Fatalf("create user_tenant: %v", err)
	}

	legacyTitle := strings.Repeat("a", 32)
	legacyTenant := strings.Repeat("b", 100)
	if err := db.Exec(`INSERT INTO pipeline_operation_log (id, pipeline_title) VALUES (?, ?)`, "log-1", legacyTitle).Error; err != nil {
		t.Fatalf("seed pipeline_operation_log: %v", err)
	}
	if err := db.Exec(`INSERT INTO tenant (id, name) VALUES (?, ?)`, "tenant-1", legacyTenant).Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	legacyRef := strings.Repeat("c", 32)
	if err := db.Exec(`INSERT INTO user_tenant (id, user_id, tenant_id, tenant_llm_id) VALUES (?, ?, ?, ?)`, "ut-1", "user-1", "tenant-1", legacyRef).Error; err != nil {
		t.Fatalf("seed user_tenant: %v", err)
	}

	if err := migrateColumnWidths(ctx, db); err != nil {
		t.Fatalf("migrateColumnWidths failed: %v", err)
	}

	if got := columnLength(t, db, &entity.PipelineOperationLog{}, "pipeline_title"); got != common.PipelineTitleLimit {
		t.Fatalf("pipeline_title width = %d, want %d", got, common.PipelineTitleLimit)
	}
	if !db.Migrator().HasIndex(&entity.PipelineOperationLog{}, "idx_pipeline_operation_log_pipeline_title") {
		t.Fatal("pipeline_title index was dropped by the width migration")
	}
	if got := columnLength(t, db, &entity.Tenant{}, "name"); got != common.TenantNameLimit {
		t.Fatalf("tenant.name width = %d, want %d", got, common.TenantNameLimit)
	}
	if got := columnLength(t, db, &entity.UserTenant{}, "tenant_llm_id"); got != common.GroupModelRefWidth {
		t.Fatalf("user_tenant.tenant_llm_id width = %d, want %d", got, common.GroupModelRefWidth)
	}

	var title string
	if err := db.Raw(`SELECT pipeline_title FROM pipeline_operation_log WHERE id = ?`, "log-1").Scan(&title).Error; err != nil {
		t.Fatalf("read pipeline_title: %v", err)
	}
	if title != legacyTitle {
		t.Fatalf("pipeline_title = %q, want %q", title, legacyTitle)
	}
	var tenantName string
	if err := db.Raw(`SELECT name FROM tenant WHERE id = ?`, "tenant-1").Scan(&tenantName).Error; err != nil {
		t.Fatalf("read tenant.name: %v", err)
	}
	if tenantName != legacyTenant {
		t.Fatalf("tenant.name = %q, want %q", tenantName, legacyTenant)
	}

	// The widened columns must accept values that previously failed with 1406.
	longTitle := strings.Repeat("c", common.PipelineTitleLimit)
	if err := db.Exec(`UPDATE pipeline_operation_log SET pipeline_title = ? WHERE id = ?`, longTitle, "log-1").Error; err != nil {
		t.Fatalf("store widened pipeline_title: %v", err)
	}
}

func columnLength(t *testing.T, db *gorm.DB, model any, column string) int64 {
	t.Helper()
	types, err := db.Migrator().ColumnTypes(model)
	if err != nil {
		t.Fatalf("read column types: %v", err)
	}
	for _, ct := range types {
		if ct.Name() != column {
			continue
		}
		length, ok := ct.Length()
		if !ok {
			t.Fatalf("column %s does not report a length", column)
		}
		return length
	}
	t.Fatalf("column %s not found", column)
	return 0
}

// tableNames lists the tables of the connected database.
func tableNames(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var names []string
	if err := db.Raw(
		"SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE()",
	).Scan(&names).Error; err != nil {
		t.Fatalf("list tables: %v", err)
	}
	return names
}

// dropTablesAddedSince removes the tables a migration pass created, so a test
// that runs the whole pass leaves the shared throwaway database as empty as it
// found it. Table names come from the catalog, so they are quoted as identifiers
// rather than interpolated as values.
func dropTablesAddedSince(t *testing.T, db *gorm.DB, before []string) {
	t.Helper()
	existed := make(map[string]struct{}, len(before))
	for _, name := range before {
		existed[name] = struct{}{}
	}
	for _, name := range tableNames(t, db) {
		if _, ok := existed[name]; ok {
			continue
		}
		if err := db.Exec("DROP TABLE IF EXISTS `" + name + "`").Error; err != nil {
			t.Errorf("drop table %s: %v", name, err)
		}
	}
}
