//go:build integration
// +build integration

package dao

import (
	"context"
	"os"
	"testing"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// openMigrationTestDB opens the throwaway MySQL database used by the DSL
// version migration tests. It reuses the same dialect helper as the other
// migration integration tests so DDL behaviour matches production MySQL.
// Like them it skips when RAGFLOW_TEST_MYSQL_DSN is unset (the CI
// integration job does not provision a throwaway MySQL), so the suite
// stays green there instead of failing on a hardcoded localhost connect.
func openMigrationTestDB(t *testing.T) *gorm.DB {
	dsn := os.Getenv("RAGFLOW_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("RAGFLOW_TEST_MYSQL_DSN unset: this test needs a throwaway MySQL database")
	}
	db, err := gorm.Open(newMigrationAwareDialector(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to connect to migration test database: %v", err)
	}
	return db
}

func strPtr(s string) *string { return &s }

// T1: migrateDSLVersionColumns adds pipeline_version_id to knowledgebase and
// document when the column is absent, and is a no-op (no error) on a second run.
func TestMigrateDSLVersionColumns_AddsAndIdempotent(t *testing.T) {
	db := openMigrationTestDB(t)
	ctx := context.Background()

	if err := db.WithContext(ctx).AutoMigrate(&entity.Knowledgebase{}, &entity.Document{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	// Drop the column so we exercise the add path (AutoMigrate would otherwise
	// have already created it because the entity now carries the field).
	_ = db.WithContext(ctx).Exec("ALTER TABLE knowledgebase DROP COLUMN pipeline_version_id").Error
	_ = db.WithContext(ctx).Exec("ALTER TABLE document DROP COLUMN pipeline_version_id").Error

	if err := MigrateDSLVersionColumns(ctx, db); err != nil {
		t.Fatalf("MigrateDSLVersionColumns: %v", err)
	}
	if !db.WithContext(ctx).Migrator().HasColumn(&entity.Knowledgebase{}, "pipeline_version_id") {
		t.Fatal("knowledgebase.pipeline_version_id was not added")
	}
	if !db.WithContext(ctx).Migrator().HasColumn(&entity.Document{}, "pipeline_version_id") {
		t.Fatal("document.pipeline_version_id was not added")
	}

	// Idempotent: column already present must not error (Error 1060 path).
	if err := MigrateDSLVersionColumns(ctx, db); err != nil {
		t.Fatalf("MigrateDSLVersionColumns second run: %v", err)
	}
}

// seedDSLVersionTables resets the four relevant tables and clears the migration
// version marker so backfill runs from scratch on every call.
func seedDSLVersionTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	// The shared integration database persists across runs, and another migration
	// test aborts if knowledgebase (a tenant-model-id table) already exists. Drop
	// every table we create so the suite stays isolated.
	t.Cleanup(func() {
		for _, m := range []interface{}{
			&entity.UserCanvasVersion{}, &entity.UserCanvas{}, &entity.Document{}, &entity.Knowledgebase{},
		} {
			_ = db.WithContext(context.Background()).Migrator().DropTable(m)
		}
	})
	for _, m := range []interface{}{
		&entity.Knowledgebase{}, &entity.Document{}, &entity.UserCanvas{}, &entity.UserCanvasVersion{},
	} {
		_ = db.WithContext(ctx).Migrator().DropTable(m)
	}
	if err := db.WithContext(ctx).AutoMigrate(&entity.Knowledgebase{}, &entity.Document{}, &entity.UserCanvas{}, &entity.UserCanvasVersion{}, &entity.SystemSettings{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	if err := db.WithContext(ctx).Exec("DELETE FROM system_settings WHERE name = ?", migrationDBVersionMarker).Error; err != nil {
		t.Fatalf("clear version marker: %v", err)
	}
}

// T2: backfill writes the latest version id for dataflow-canvas bindings and
// skips agent-canvas and built-in bindings.
func TestBackfillDSLVersionBindings_BindsDataflowSkipsAgentAndBuiltin(t *testing.T) {
	db := openMigrationTestDB(t)
	ctx := context.Background()
	seedDSLVersionTables(t, db)

	dfCanvas := "df-canvas-1"
	dfVersion := "df-ver-1"
	if err := db.WithContext(ctx).Create(&entity.UserCanvas{ID: dfCanvas, CanvasCategory: "dataflow_canvas", Title: strPtr("df")}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Create(&entity.UserCanvasVersion{ID: dfVersion, UserCanvasID: dfCanvas, DSL: entity.JSONMap{"k": "v"}}).Error; err != nil {
		t.Fatal(err)
	}

	agentCanvas := "agent-canvas-1"
	agentVersion := "agent-ver-1"
	db.WithContext(ctx).Create(&entity.UserCanvas{ID: agentCanvas, CanvasCategory: "agent_canvas", Title: strPtr("ag")})
	db.WithContext(ctx).Create(&entity.UserCanvasVersion{ID: agentVersion, UserCanvasID: agentCanvas, DSL: entity.JSONMap{"k": "v"}})

	builtin := "general"

	db.WithContext(ctx).Create(&entity.Knowledgebase{ID: "kb-df", PipelineID: strPtr(dfCanvas)})
	db.WithContext(ctx).Create(&entity.Knowledgebase{ID: "kb-agent", PipelineID: strPtr(agentCanvas)})
	db.WithContext(ctx).Create(&entity.Knowledgebase{ID: "kb-builtin", PipelineID: strPtr(builtin)})
	db.WithContext(ctx).Create(&entity.Document{ID: "doc-df", KbID: "kb-df", PipelineID: strPtr(dfCanvas), ParserConfig: entity.JSONMap{}})
	db.WithContext(ctx).Create(&entity.Document{ID: "doc-builtin", KbID: "kb-builtin2", PipelineID: strPtr(builtin), ParserConfig: entity.JSONMap{}})

	if err := BackfillDSLVersionBindings(ctx, db); err != nil {
		t.Fatalf("BackfillDSLVersionBindings: %v", err)
	}

	var gotDF, gotAgent, gotBuiltin entity.Knowledgebase
	db.WithContext(ctx).First(&gotDF, "id = ?", "kb-df")
	db.WithContext(ctx).First(&gotAgent, "id = ?", "kb-agent")
	db.WithContext(ctx).First(&gotBuiltin, "id = ?", "kb-builtin")
	if gotDF.PipelineVersionID == nil || *gotDF.PipelineVersionID != dfVersion {
		t.Fatalf("kb-df pipeline_version_id = %v, want %s", gotDF.PipelineVersionID, dfVersion)
	}
	if gotAgent.PipelineVersionID != nil {
		t.Fatalf("kb-agent must stay nil (agent canvas), got %v", *gotAgent.PipelineVersionID)
	}
	if gotBuiltin.PipelineVersionID != nil {
		t.Fatalf("kb-builtin must stay nil (built-in), got %v", *gotBuiltin.PipelineVersionID)
	}

	var gotDocDF, gotDocBuiltin entity.Document
	db.WithContext(ctx).First(&gotDocDF, "id = ?", "doc-df")
	db.WithContext(ctx).First(&gotDocBuiltin, "id = ?", "doc-builtin")
	if gotDocDF.PipelineVersionID == nil || *gotDocDF.PipelineVersionID != dfVersion {
		t.Fatalf("doc-df pipeline_version_id = %v, want %s", gotDocDF.PipelineVersionID, dfVersion)
	}
	if gotDocBuiltin.PipelineVersionID != nil {
		t.Fatalf("doc-builtin must stay nil (built-in), got %v", *gotDocBuiltin.PipelineVersionID)
	}

	// Idempotency: a second run must not change or error.
	if err := BackfillDSLVersionBindings(ctx, db); err != nil {
		t.Fatalf("BackfillDSLVersionBindings second run: %v", err)
	}
	var gotDFAgain entity.Knowledgebase
	db.WithContext(ctx).First(&gotDFAgain, "id = ?", "kb-df")
	if *gotDFAgain.PipelineVersionID != dfVersion {
		t.Fatalf("kb-df changed after second backfill: %v", *gotDFAgain.PipelineVersionID)
	}
}

// T17: a pre-existing (存量) dataflow binding is locked to the LATEST version by
// create_time, even when an older version exists.
func TestBackfillDSLVersionBindings_LocksLatestVersionByCreateTime(t *testing.T) {
	db := openMigrationTestDB(t)
	ctx := context.Background()
	seedDSLVersionTables(t, db)

	canvas := "df-canvas-order"
	db.WithContext(ctx).Create(&entity.UserCanvas{ID: canvas, CanvasCategory: "dataflow_canvas", Title: strPtr("df")})
	// Older version created first (earlier create_time), newer version created
	// second; GetLatest returns the newer. Explicit create_time makes the order
	// deterministic (two rows without it can share the same millisecond).
	oldCT, newCT := int64(100), int64(200)
	db.WithContext(ctx).Create(&entity.UserCanvasVersion{ID: "ver-old", UserCanvasID: canvas, DSL: entity.JSONMap{"v": 1}, CreateTime: &oldCT})
	db.WithContext(ctx).Create(&entity.UserCanvasVersion{ID: "ver-new", UserCanvasID: canvas, DSL: entity.JSONMap{"v": 2}, CreateTime: &newCT})

	db.WithContext(ctx).Create(&entity.Knowledgebase{ID: "kb-order", PipelineID: strPtr(canvas)})

	if err := BackfillDSLVersionBindings(ctx, db); err != nil {
		t.Fatalf("BackfillDSLVersionBindings: %v", err)
	}
	var got entity.Knowledgebase
	db.WithContext(ctx).First(&got, "id = ?", "kb-order")
	if got.PipelineVersionID == nil || *got.PipelineVersionID != "ver-new" {
		t.Fatalf("kb-order pipeline_version_id = %v, want ver-new", got.PipelineVersionID)
	}
}

// T18: a dataflow canvas bound via pipeline_id but carrying NO versions yet must
// stay unbound. Writing a dangling version id would break the not-dangling
// invariant, so the backfill skips it and leaves pipeline_version_id NULL.
func TestBackfillDSLVersionBindings_NoVersionsStaysNil(t *testing.T) {
	db := openMigrationTestDB(t)
	ctx := context.Background()
	seedDSLVersionTables(t, db)

	canvas := "df-canvas-nover"
	if err := db.WithContext(ctx).Create(&entity.UserCanvas{ID: canvas, CanvasCategory: "dataflow_canvas", Title: strPtr("df")}).Error; err != nil {
		t.Fatal(err)
	}
	// Intentionally no UserCanvasVersion rows for this canvas.
	db.WithContext(ctx).Create(&entity.Knowledgebase{ID: "kb-nover", PipelineID: strPtr(canvas)})

	if err := BackfillDSLVersionBindings(ctx, db); err != nil {
		t.Fatalf("BackfillDSLVersionBindings: %v", err)
	}
	var got entity.Knowledgebase
	db.WithContext(ctx).First(&got, "id = ?", "kb-nover")
	if got.PipelineVersionID != nil {
		t.Fatalf("kb-nover must stay nil when canvas has no versions, got %v", *got.PipelineVersionID)
	}
}

// T16: built-in pipelines are unaffected by any version logic. A built-in
// pipeline_id string with no backing user_canvas keeps pipeline_version_id NULL,
// and a knowledge base with no pipeline at all stays NULL too.
func TestBackfillDSLVersionBindings_BuiltinUnaffected(t *testing.T) {
	db := openMigrationTestDB(t)
	ctx := context.Background()
	seedDSLVersionTables(t, db)

	builtin := "naive"
	db.WithContext(ctx).Create(&entity.Knowledgebase{ID: "kb-builtin2", PipelineID: strPtr(builtin)})
	db.WithContext(ctx).Create(&entity.Knowledgebase{ID: "kb-none", PipelineID: nil})

	if err := BackfillDSLVersionBindings(ctx, db); err != nil {
		t.Fatalf("BackfillDSLVersionBindings: %v", err)
	}
	var gotBuiltin, gotNone entity.Knowledgebase
	db.WithContext(ctx).First(&gotBuiltin, "id = ?", "kb-builtin2")
	db.WithContext(ctx).First(&gotNone, "id = ?", "kb-none")
	if gotBuiltin.PipelineID == nil || *gotBuiltin.PipelineID != builtin {
		t.Fatalf("built-in pipeline_id must be preserved, got %v", gotBuiltin.PipelineID)
	}
	if gotBuiltin.PipelineVersionID != nil {
		t.Fatalf("built-in must not get a version id, got %v", *gotBuiltin.PipelineVersionID)
	}
	if gotNone.PipelineVersionID != nil {
		t.Fatalf("kb with no pipeline must stay nil, got %v", *gotNone.PipelineVersionID)
	}
}

// TestOpenMigrationTestDB_SkipsWhenDSNUnset locks the CI contract: when
// RAGFLOW_TEST_MYSQL_DSN is unset the migration helper must SKIP (never attempt
// a hardcoded localhost connection and fail). The ragflow_go_integration job does
// not provision a throwaway MySQL, so a non-skipping helper turns every DSL
// version migration test red there (matches migration_integration_test.go).
func TestOpenMigrationTestDB_SkipsWhenDSNUnset(t *testing.T) {
	if os.Getenv("RAGFLOW_TEST_MYSQL_DSN") != "" {
		t.Skip("this probe requires RAGFLOW_TEST_MYSQL_DSN to be unset")
	}
	returned := false
	ok := t.Run("probe", func(st *testing.T) {
		openMigrationTestDB(st)
		returned = true // reached only when the helper did NOT skip or fail
	})
	if returned {
		t.Fatalf("openMigrationTestDB returned a DB instead of skipping when RAGFLOW_TEST_MYSQL_DSN is unset")
	}
	if !ok {
		t.Fatalf("probe subtest failed instead of skipping when RAGFLOW_TEST_MYSQL_DSN is unset")
	}
}

// T1b: MigrateDSLVersionColumns also adds the user_canvas_version.alias column
// (design A2/F1) so a version save never fails with Error 1054 on a database
// whose table predates the column. Idempotent on a second run.
func TestMigrateDSLVersionColumns_AddsAliasColumnIdempotent(t *testing.T) {
	db := openMigrationTestDB(t)
	ctx := context.Background()

	if err := db.WithContext(ctx).AutoMigrate(&entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	// Drop the alias column so we exercise the add path (AutoMigrate would
	// otherwise have already created it because the entity carries the field).
	_ = db.WithContext(ctx).Exec("ALTER TABLE user_canvas_version DROP COLUMN alias").Error
	if db.WithContext(ctx).Migrator().HasColumn(&entity.UserCanvasVersion{}, "alias") {
		t.Fatal("precondition: alias column should be absent before migration")
	}

	if err := MigrateDSLVersionColumns(ctx, db); err != nil {
		t.Fatalf("MigrateDSLVersionColumns: %v", err)
	}
	if !db.WithContext(ctx).Migrator().HasColumn(&entity.UserCanvasVersion{}, "alias") {
		t.Fatal("user_canvas_version.alias was not added")
	}

	// Idempotent: column already present must not error.
	if err := MigrateDSLVersionColumns(ctx, db); err != nil {
		t.Fatalf("MigrateDSLVersionColumns second run: %v", err)
	}
}

// T19: many rows sharing the same dataflow canvas must all pin the SAME latest
// version. This exercises the per-canvas resolution cache: the canvas lookup and
// GetLatest must be resolved once and reused, so the result is consistent across
// every row (no row ends up pinned to a stale version due to cached-state reuse).
func TestBackfillDSLVersionBindings_CachesSharedCanvas(t *testing.T) {
	db := openMigrationTestDB(t)
	ctx := context.Background()
	seedDSLVersionTables(t, db)

	canvas := "df-canvas-shared"
	if err := db.WithContext(ctx).Create(&entity.UserCanvas{ID: canvas, CanvasCategory: "dataflow_canvas", Title: strPtr("df")}).Error; err != nil {
		t.Fatal(err)
	}
	oldCT, newCT := int64(100), int64(200)
	db.WithContext(ctx).Create(&entity.UserCanvasVersion{ID: "ver-old", UserCanvasID: canvas, DSL: entity.JSONMap{"v": 1}, CreateTime: &oldCT})
	db.WithContext(ctx).Create(&entity.UserCanvasVersion{ID: "ver-new", UserCanvasID: canvas, DSL: entity.JSONMap{"v": 2}, CreateTime: &newCT})

	// 5 knowledge bases bound to the same canvas, exercising the cache path.
	for i := 0; i < 5; i++ {
		if err := db.WithContext(ctx).Create(&entity.Knowledgebase{ID: "kb-shared-" + string(rune('0'+i)), PipelineID: strPtr(canvas)}).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := BackfillDSLVersionBindings(ctx, db); err != nil {
		t.Fatalf("BackfillDSLVersionBindings: %v", err)
	}
	var kbs []entity.Knowledgebase
	db.WithContext(ctx).Where("id LIKE ?", "kb-shared-%").Find(&kbs)
	if len(kbs) != 5 {
		t.Fatalf("got %d shared kbs, want 5", len(kbs))
	}
	for _, kb := range kbs {
		if kb.PipelineVersionID == nil || *kb.PipelineVersionID != "ver-new" {
			t.Fatalf("kb %s pipeline_version_id = %v, want ver-new", kb.ID, kb.PipelineVersionID)
		}
	}
}
