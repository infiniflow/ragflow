package dao

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupKnowledgeCompileMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	if err := db.AutoMigrate(&entity.KnowledgeCompileDataset{}, &entity.KnowledgeCompileBacklog{}, &entity.KnowledgeCompileInflight{}, &entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"backlog_doc_ids", "inflight_doc_ids"} {
		if err := db.Exec("ALTER TABLE knowledge_compile_docs ADD COLUMN `" + column + "` LONGTEXT").Error; err != nil {
			t.Fatal(err)
		}
	}
	writeDatabaseVersion(t, db, permissionMigrationTargetVersion)
	return db
}

func TestMigrateKnowledgeCompileQueuesPreservesEventsAndLease(t *testing.T) {
	db := setupKnowledgeCompileMigrationDB(t)
	expires := time.Now().Add(time.Minute).UTC()
	row := entity.KnowledgeCompileDataset{
		DatasetID: "kb1", TenantID: "tenant1", ClaimOwner: "worker", ClaimToken: "token",
		ClaimExpiresAt: &expires, State: entity.DatasetStateRunning, Progress: 0.5,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	entries := make([]entity.KnowledgeCompileEntry, 205)
	for i := range entries {
		entries[i] = entity.KnowledgeCompileEntry{DocID: "same-doc", EventType: "doc_completed", Variants: []string{"structure"}, TaskTypes: []string{"Graph"}}
	}
	entries[204].EventType = "doc_deleted"
	payload, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&row).Updates(map[string]interface{}{
		"backlog_doc_ids": string(payload), "inflight_doc_ids": `[{"doc_id":"d2","event_type":"doc_disabled"}]`,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateKnowledgeCompileQueues(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if got := databaseVersion(t, db); got != knowledgeCompileQueueTargetVersion {
		t.Fatalf("migration version=%q, want %q", got, knowledgeCompileQueueTargetVersion)
	}
	for _, column := range []string{"backlog_doc_ids", "inflight_doc_ids"} {
		if db.Migrator().HasColumn(&entity.KnowledgeCompileDataset{}, column) {
			t.Fatalf("source column %s was not removed", column)
		}
	}
	var backlog []entity.KnowledgeCompileEntry
	if err := db.Table((entity.KnowledgeCompileBacklog{}).TableName()).Order("queue_position").Find(&backlog).Error; err != nil {
		t.Fatal(err)
	}
	if len(backlog) != len(entries) {
		t.Fatalf("backlog count = %d, want %d", len(backlog), len(entries))
	}
	for i, got := range backlog {
		want := entries[i]
		want.DatasetID = "kb1"
		want.QueuePosition = int64(i + 1)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("entry %d = %+v, want %+v", i, got, want)
		}
	}
	var inflight entity.KnowledgeCompileInflight
	if err := db.Take(&inflight).Error; err != nil {
		t.Fatal(err)
	}
	if inflight.DocID != "d2" || inflight.EventType != "doc_disabled" || inflight.Variants != nil || inflight.TaskTypes != nil {
		t.Fatalf("unexpected inflight entry: %+v", inflight)
	}
	var stored entity.KnowledgeCompileDataset
	if err := db.Where("dataset_id = ?", "kb1").Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ClaimToken != row.ClaimToken || stored.ClaimOwner != row.ClaimOwner || !stored.ClaimExpiresAt.Equal(expires) || stored.State != row.State || stored.Progress != row.Progress {
		t.Fatalf("migration changed the scheduling state: %+v", stored)
	}
	for _, tc := range []struct{ version, want string }{
		{permissionMigrationTargetVersion, knowledgeCompileQueueTargetVersion},
		{knowledgeCompileQueueTargetVersion, knowledgeCompileQueueTargetVersion},
		{"v1.0.0-rc3.dev6", "v1.0.0-rc3.dev6"},
	} {
		writeDatabaseVersion(t, db, tc.version)
		if err := migrateKnowledgeCompileQueues(t.Context(), db); err != nil {
			t.Fatalf("rerun: %v", err)
		}
		if got := databaseVersion(t, db); got != tc.want {
			t.Fatalf("rerun database version=%q, want %q", got, tc.want)
		}
		var count int64
		if err := db.Model(&entity.KnowledgeCompileBacklog{}).Count(&count).Error; err != nil || count != 205 {
			t.Fatalf("rerun duplicated entries: count=%d err=%v", count, err)
		}
	}
}

func TestMigrateKnowledgeCompileQueuesResumesAfterInvalidPayload(t *testing.T) {
	for _, queue := range []struct {
		column  string
		table   string
		version string
	}{
		{"backlog_doc_ids", (entity.KnowledgeCompileBacklog{}).TableName(), permissionMigrationTargetVersion},
		{"inflight_doc_ids", (entity.KnowledgeCompileInflight{}).TableName(), tenantDefaultModelsTargetVersion},
	} {
		t.Run(queue.column, func(t *testing.T) {
			db := setupKnowledgeCompileMigrationDB(t)
			writeDatabaseVersion(t, db, queue.version)
			for _, id := range []string{"", "a", "b"} {
				if err := db.Create(&entity.KnowledgeCompileDataset{DatasetID: id}).Error; err != nil {
					t.Fatal(err)
				}
			}
			valid := `[{"doc_id":"same","event_type":"doc_completed"},{"doc_id":"same","event_type":"doc_completed"}]`
			invalid := strings.TrimSuffix(valid, "]")
			if err := db.Exec("UPDATE knowledge_compile_docs SET backlog_doc_ids = ?, inflight_doc_ids = ?", valid, valid).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&entity.KnowledgeCompileDataset{}).Where("dataset_id = ?", "b").Update(queue.column, invalid).Error; err != nil {
				t.Fatal(err)
			}
			if err := migrateKnowledgeCompileQueues(t.Context(), db); err == nil {
				t.Fatal("invalid source must fail migration")
			}
			if got := databaseVersion(t, db); got != queue.version {
				t.Fatalf("failed migration advanced version=%q, want %q", got, queue.version)
			}
			if !db.Migrator().HasColumn(&entity.KnowledgeCompileDataset{}, queue.column) {
				t.Fatal("failed migration dropped source column")
			}
			var count int64
			if err := db.Table(queue.table).Where("dataset_id = ?", "b").Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("failed dataset was not rolled back: count=%d err=%v", count, err)
			}
			if err := db.Model(&entity.KnowledgeCompileDataset{}).Where("dataset_id = ?", "b").Update(queue.column, valid).Error; err != nil {
				t.Fatal(err)
			}
			if err := migrateKnowledgeCompileQueues(t.Context(), db); err != nil {
				t.Fatalf("resume: %v", err)
			}
			if got := databaseVersion(t, db); got != knowledgeCompileQueueTargetVersion {
				t.Fatalf("resumed migration version=%q, want %q", got, knowledgeCompileQueueTargetVersion)
			}
			for _, table := range []string{(entity.KnowledgeCompileBacklog{}).TableName(), (entity.KnowledgeCompileInflight{}).TableName()} {
				if err := db.Table(table).Count(&count).Error; err != nil || count != 6 {
					t.Fatalf("resumed migration lost or duplicated events in %s: count=%d err=%v", table, count, err)
				}
			}
		})
	}
}
