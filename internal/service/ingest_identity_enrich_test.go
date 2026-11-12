package service

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// TestEmitLifecycleEventResolvesUserAndTenantName verifies that emitLifecycleEvent
// resolves the owning user's display name (from task.UserID) and the tenant display
// name (from doc.KbID -> tenant) and attaches them to the emit context, so the
// downstream sink (e.g. the EE ClickHouse ingestion_events_fact) records who and
// which tenant triggered the ingestion. This is the OSS-side resolution that the
// sink consumes via observability.BuildIngestLog.
func TestEmitLifecycleEventResolvesUserAndTenantName(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.User{}, &entity.Tenant{}, &entity.Knowledgebase{}, &entity.Document{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	if err := db.Create(&entity.User{ID: "u1", Nickname: "Alice"}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := db.Create(&entity.Tenant{ID: "t1", Name: strptr("Acme"), Status: strptr("1")}).Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if err := db.Create(&entity.Knowledgebase{ID: "kb1", TenantID: "t1"}).Error; err != nil {
		t.Fatalf("seed knowledgebase: %v", err)
	}
	if err := db.Create(&entity.Document{ID: "doc1", KbID: "kb1", ParserConfig: entity.JSONMap{}}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	prevDB := dao.DB
	dao.DB = db
	defer func() { dao.DB = prevDB }()

	runX := "run-x"
	rec := &recordingEmitter{}
	svc := &IngestionTaskService{documentDAO: &dao.DocumentDAO{}, eventEmitter: rec}
	task := &entity.IngestionTask{ID: "task1", DocumentID: "doc1", UserID: "u1", PipelineLogID: &runX}

	svc.emitLifecycleEvent(context.Background(), task, "completed", "COMPLETED", "")

	if !waitForEmitCount(rec.count, 1) {
		t.Fatalf("event not emitted (async) within timeout")
	}
	if rec.count() != 1 {
		t.Fatalf("expected 1 emitted event, got %d", rec.count())
	}
	got := rec.last()
	if got.UserName != "Alice" {
		t.Errorf("UserName = %q, want Alice", got.UserName)
	}
	if got.TenantName != "Acme" {
		t.Errorf("TenantName = %q, want Acme", got.TenantName)
	}
}

// TestEmitLifecycleEventMissingTenantStillResolvesUser is an adversarial case for the
// coupled user/tenant fields: the document points to a knowledgebase whose tenant row
// is absent. Resolution must be best-effort and independent per field — the user name
// still resolves, the tenant name stays empty, and the emit must not panic or partially
// populate.
func TestEmitLifecycleEventMissingTenantStillResolvesUser(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.User{}, &entity.Tenant{}, &entity.Knowledgebase{}, &entity.Document{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	if err := db.Create(&entity.User{ID: "u1", Nickname: "Alice"}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// kb1 -> t1, but no Tenant row for t1: tenant resolution must fail gracefully.
	if err := db.Create(&entity.Knowledgebase{ID: "kb1", TenantID: "t1"}).Error; err != nil {
		t.Fatalf("seed knowledgebase: %v", err)
	}
	if err := db.Create(&entity.Document{ID: "doc1", KbID: "kb1", ParserConfig: entity.JSONMap{}}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	prevDB := dao.DB
	dao.DB = db
	defer func() { dao.DB = prevDB }()

	runX := "run-x"
	rec := &recordingEmitter{}
	svc := &IngestionTaskService{documentDAO: &dao.DocumentDAO{}, eventEmitter: rec}
	task := &entity.IngestionTask{ID: "task1", DocumentID: "doc1", UserID: "u1", PipelineLogID: &runX}

	svc.emitLifecycleEvent(context.Background(), task, "completed", "COMPLETED", "")

	if !waitForEmitCount(rec.count, 1) {
		t.Fatalf("event not emitted (async) within timeout")
	}
	if rec.count() != 1 {
		t.Fatalf("expected 1 emitted event, got %d", rec.count())
	}
	got := rec.last()
	if got.UserName != "Alice" {
		t.Errorf("UserName = %q, want Alice (user must still resolve when tenant is missing)", got.UserName)
	}
	if got.TenantName != "" {
		t.Errorf("TenantName = %q, want empty (tenant row absent)", got.TenantName)
	}
}

// TestEmitLifecycleEventMissingIdentityIsNoop guards the best-effort contract: when the
// task carries no user id and no document (so neither user nor tenant can be
// resolved), the emit still succeeds, names stay empty, and the pipeline is not
// disrupted.
func TestEmitLifecycleEventMissingIdentityIsNoop(t *testing.T) {
	rec := &recordingEmitter{}
	svc := &IngestionTaskService{eventEmitter: rec}
	task := &entity.IngestionTask{ID: "task1"}

	svc.emitLifecycleEvent(context.Background(), task, "completed", "COMPLETED", "")

	if !waitForEmitCount(rec.count, 1) {
		t.Fatalf("event not emitted (async) within timeout")
	}
	if rec.count() != 1 {
		t.Fatalf("expected 1 emitted event, got %d", rec.count())
	}
	got := rec.last()
	if got.UserName != "" || got.TenantName != "" {
		t.Errorf("with no identity inputs names = %q/%q, want both empty", got.UserName, got.TenantName)
	}
}
