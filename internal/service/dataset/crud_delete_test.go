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

package dataset

import (
	"context"
	"errors"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
)

func TestDeleteDatasetRemovesTemporaryFiles(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Dataset")
	if err := db.Exec("INSERT INTO document (id, kb_id, name, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", "doc-1", "kb-1", "Document", "general", "{}", "pdf", "tenant-1", "pdf", "tracked").Error; err != nil {
		t.Fatal(err)
	}
	store := storage.NewMemoryStorage()
	if err := store.Put(t.Context(), "kb-1", "tracked", []byte("content")); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(t.Context(), "kb-1", "temporary", []byte("content")); err != nil {
		t.Fatal(err)
	}
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(previous) })
	core, logs := observer.New(zapcore.InfoLevel)
	previousLogger := common.Logger
	common.Logger = zap.New(core)
	t.Cleanup(func() { common.Logger = previousLogger })

	kb, err := dao.NewKnowledgebaseDAO().GetByID(t.Context(), db, "kb-1")
	if err != nil {
		t.Fatal(err)
	}
	service := testDatasetUpdateService(t)
	plan, err := service.DeleteDatasetTx(t.Context(), db, kb)
	if err != nil {
		t.Fatalf("delete dataset with temporary file: %v", err)
	}
	service.CleanupDatasetExternal(t.Context(), plan)
	if store.ObjExist(t.Context(), "kb-1", "temporary") {
		t.Fatal("dataset deletion retained a temporary object")
	}
	if store.ObjExist(t.Context(), "kb-1", "tracked") {
		t.Fatal("dataset deletion retained a document object")
	}
	if store.BucketExists(t.Context(), "kb-1") {
		t.Fatal("dataset deletion retained the bucket")
	}
	var count int64
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("dataset retained after deletion: count=%d, err=%v", count, err)
	}
	if entries := logs.FilterMessage("Removed dataset document object").All(); len(entries) != 1 || entries[0].ContextMap()["location"] != "tracked" || entries[0].ContextMap()["dataset"] != "Dataset (kb-1)" {
		t.Errorf("document deletion logs = %v", entries)
	}
	if entries := logs.FilterMessage("Deleted dataset").All(); len(entries) != 1 || entries[0].ContextMap()["dataset"] != "Dataset (kb-1)" {
		t.Errorf("dataset deletion logs = %v", entries)
	}
}

func TestDeleteDatasetContinuesWhenStorageIsMissing(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Dataset")
	if err := db.Exec("INSERT INTO document (id, kb_id, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "doc-1", "kb-1", "general", "{}", "pdf", "tenant-1", "pdf", "missing").Error; err != nil {
		t.Fatal(err)
	}
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(storage.NewMemoryStorage())
	t.Cleanup(func() { factory.SetStorage(previous) })
	kb, err := dao.NewKnowledgebaseDAO().GetByID(t.Context(), db, "kb-1")
	if err != nil {
		t.Fatal(err)
	}
	svc := testDatasetUpdateService(t)
	plan, err := svc.DeleteDatasetTx(t.Context(), db, kb)
	if err != nil {
		t.Fatalf("delete dataset with missing storage: %v", err)
	}
	svc.CleanupDatasetExternal(t.Context(), plan)
	var count int64
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("dataset retained after missing storage: count=%d, err=%v", count, err)
	}
}

func TestDeleteDatasetRemovesIngestionTasks(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	ensureDatasetTestMembership(t, "tenant-1", "tenant-1", "owner")
	// DeleteDatasets normalizes ids via uuid.Parse then strips dashes.
	kbID := "11111111111111111111111111111111"
	insertDatasetUpdateKB(t, kbID, "tenant-1", "Dataset")
	insertDatasetUpdateKB(t, "kb-2", "tenant-1", "Other")
	for _, row := range [][2]string{{"doc-1", kbID}, {"doc-2", "kb-2"}, {"doc-3", kbID}} {
		if err := db.Exec("INSERT INTO document (id, kb_id, name, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", row[0], row[1], row[0], "general", "{}", "pdf", "tenant-1", "pdf", "").Error; err != nil {
			t.Fatal(err)
		}
	}
	tasks := []entity.IngestionTask{
		{ID: "task-1", UserID: "tenant-1", DocumentID: "doc-1", DatasetID: kbID, Status: common.SCHEDULED},
		{ID: "task-2", UserID: "tenant-1", DocumentID: "doc-2", DatasetID: "kb-2", Status: common.SCHEDULED},
		{ID: "task-3", UserID: "tenant-1", DocumentID: "doc-3", DatasetID: kbID, Status: common.RUNNING},
	}
	if err := db.Create(&tasks).Error; err != nil {
		t.Fatal(err)
	}
	logs := []entity.IngestionTaskLog{
		{TaskID: "task-1", Checkpoint: entity.JSONMap{}},
		{TaskID: "task-2", Checkpoint: entity.JSONMap{}},
		{TaskID: "task-3", Checkpoint: entity.JSONMap{}},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(storage.NewMemoryStorage())
	t.Cleanup(func() { factory.SetStorage(previous) })

	svc := testDatasetUpdateService(t)
	svc.taskStopWait = 600 * time.Millisecond
	svc.docEngine = &countingDeleteEngine{}
	start := time.Now()
	ctx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
	if _, code, err := svc.DeleteDatasets(ctx, []string{kbID}, false); err != nil || code != common.CodeSuccess {
		t.Fatalf("delete datasets: code=%v err=%v", code, err)
	}
	if elapsed := time.Since(start); elapsed < svc.taskStopWait {
		t.Fatalf("DeleteDatasets returned after %v, want it to wait %v for the active task", elapsed, svc.taskStopWait)
	}
	var remaining []string
	if err := db.Model(&entity.IngestionTask{}).Order("id").Pluck("id", &remaining).Error; err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0] != "task-2" {
		t.Fatalf("ingestion tasks after deleting the dataset = %v, want [task-2] (RUNNING task cancelled after the bounded wait)", remaining)
	}
	var logTasks []string
	if err := db.Model(&entity.IngestionTaskLog{}).Pluck("task_id", &logTasks).Error; err != nil {
		t.Fatal(err)
	}
	if len(logTasks) != 1 || logTasks[0] != "task-2" {
		t.Fatalf("ingestion task logs after deleting the dataset = %v, want the log of task-2 only", logTasks)
	}
}

// countingDeleteEngine records how many engine deletes a dataset deletion
// performs, so the rollback test can assert that external cleanup never runs
// before the caller's transaction commits.
type countingDeleteEngine struct {
	fakeChatDocEngine
	chunkCalls   int
	metadataCall int
}

func (e *countingDeleteEngine) DeleteChunks(_ context.Context, _ map[string]interface{}, _, _ string) (int64, error) {
	e.chunkCalls++
	return 0, nil
}
func (e *countingDeleteEngine) DeleteMetadata(_ context.Context, _ map[string]interface{}, _ string) (int64, error) {
	e.metadataCall++
	return 0, nil
}

// TestDeleteDatasetTxUsesCallerDB verifies that a dataset delete writes through
// the db handle supplied by the caller, so that a caller-owned transaction owns
// the deletion. If the implementation ignores the passed handle and reaches for
// the global dao.DB, the outer rollback will not undo the delete and the rows
// will be gone — this test fails in that case. It also pins the post-commit
// cleanup contract: on rollback no storage object or engine data may be removed.
func TestDeleteDatasetTxUsesCallerDB(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Dataset")
	if err := db.Exec("INSERT INTO document (id, kb_id, name, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", "doc-1", "kb-1", "Document", "general", "{}", "pdf", "tenant-1", "pdf", "tracked").Error; err != nil {
		t.Fatal(err)
	}
	fileID := "file-1"
	if err := db.Exec("INSERT INTO file (id, parent_id, tenant_id, created_by, name, type, source_type) VALUES (?, ?, ?, ?, ?, ?, ?)", fileID, "kb-1", "tenant-1", "tenant-1", "Dataset", "file", string(entity.FileSourceKnowledgebase)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO file2document (id, file_id, document_id) VALUES (?, ?, ?)", "f2d-1", fileID, "doc-1").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO task (id, doc_id, task_type) VALUES (?, ?, ?)", "task-1", "doc-1", "general").Error; err != nil {
		t.Fatal(err)
	}
	store := storage.NewMemoryStorage()
	if err := store.Put(t.Context(), "kb-1", "tracked", []byte("content")); err != nil {
		t.Fatal(err)
	}
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(previous) })
	kb, err := dao.NewKnowledgebaseDAO().GetByID(t.Context(), db, "kb-1")
	if err != nil {
		t.Fatal(err)
	}

	eng := &countingDeleteEngine{}
	svc := testDatasetUpdateService(t)
	svc.docEngine = eng
	err = db.Transaction(func(tx *gorm.DB) error {
		if _, e := svc.DeleteDatasetTx(t.Context(), tx, kb); e != nil {
			return e
		}
		// Simulate a caller that rolls back: the plan is discarded and no
		// external cleanup is invoked.
		return errForceRollback
	})
	if !errors.Is(err, errForceRollback) {
		t.Fatalf("expected forced rollback error, got %v", err)
	}

	// The outer transaction rolled back: every row the delete touched must
	// still be present. If the delete used dao.DB instead of tx, these are gone.
	var kbCount int64
	if e := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Count(&kbCount).Error; e != nil || kbCount != 1 {
		t.Fatalf("rollback did not restore knowledgebase: count=%d, err=%v", kbCount, e)
	}
	var docCount int64
	if e := db.Model(&entity.Document{}).Where("id = ?", "doc-1").Count(&docCount).Error; e != nil || docCount != 1 {
		t.Fatalf("rollback did not restore document: count=%d, err=%v", docCount, e)
	}
	var fileCount int64
	if e := db.Model(&entity.File{}).Where("id = ?", fileID).Count(&fileCount).Error; e != nil || fileCount != 1 {
		t.Fatalf("rollback did not restore file: count=%d, err=%v", fileCount, e)
	}
	var mappingCount int64
	if e := db.Model(&entity.File2Document{}).Where("id = ?", "f2d-1").Count(&mappingCount).Error; e != nil || mappingCount != 1 {
		t.Fatalf("rollback did not restore file2document mapping: count=%d, err=%v", mappingCount, e)
	}
	var taskCount int64
	if e := db.Model(&entity.Task{}).Where("id = ?", "task-1").Count(&taskCount).Error; e != nil || taskCount != 1 {
		t.Fatalf("rollback did not restore task: count=%d, err=%v", taskCount, e)
	}

	// Post-commit cleanup contract: a rolled-back caller must not lose any
	// external resource. Current implementation deletes storage and engine data
	// before the transaction, so these two assertions fail on the red run.
	if !store.ObjExist(t.Context(), "kb-1", "tracked") {
		t.Fatal("rollback removed a storage object that should survive")
	}
	if eng.chunkCalls != 0 || eng.metadataCall != 0 {
		t.Fatalf("rollback ran engine cleanup: chunkCalls=%d metadataCall=%d", eng.chunkCalls, eng.metadataCall)
	}
}

var errForceRollback = errors.New("force rollback")

// TestDeleteDatasetCleanupAfterCommit verifies the standalone entry point:
// DeleteDatasets deletes the DB rows through dao.DB (a real commit) and then
// removes external resources. The KB must be gone and the storage object gone.
func TestDeleteDatasetCleanupAfterCommit(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	ensureDatasetTestMembership(t, "tenant-1", "tenant-1", "owner")
	// DeleteDatasets normalizes ids via uuid.Parse then strips dashes, so seed
	// the KB with that exact normalized form.
	kbID := "11111111111111111111111111111111"
	insertDatasetUpdateKB(t, kbID, "tenant-1", "Dataset")
	if err := db.Exec("INSERT INTO document (id, kb_id, name, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", "doc-1", kbID, "Document", "general", "{}", "pdf", "tenant-1", "pdf", "tracked").Error; err != nil {
		t.Fatal(err)
	}
	store := storage.NewMemoryStorage()
	if err := store.Put(t.Context(), kbID, "tracked", []byte("content")); err != nil {
		t.Fatal(err)
	}
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(previous) })

	eng := &countingDeleteEngine{}
	svc := testDatasetUpdateService(t)
	svc.docEngine = eng
	ctx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
	result, code, err := svc.DeleteDatasets(ctx, []string{kbID}, false)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("delete datasets: code=%v err=%v", code, err)
	}
	if result["success_count"] != 1 {
		t.Fatalf("success_count = %v, want 1", result["success_count"])
	}
	var count int64
	if e := db.Model(&entity.Knowledgebase{}).Where("id = ?", kbID).Count(&count).Error; e != nil || count != 0 {
		t.Fatalf("dataset retained after deletion: count=%d, err=%v", count, e)
	}
	if store.ObjExist(t.Context(), kbID, "tracked") {
		t.Fatal("dataset deletion retained a document object")
	}
	if eng.chunkCalls == 0 || eng.metadataCall == 0 {
		t.Fatalf("post-commit cleanup did not run engine deletes: chunkCalls=%d metadataCall=%d", eng.chunkCalls, eng.metadataCall)
	}
}

// TestCleanupDatasetExternalIdempotent pins that replaying cleanup against the
// same plan is safe: already-removed objects and buckets are skipped, no panic.
func TestCleanupDatasetExternalIdempotent(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	store := storage.NewMemoryStorage()
	if err := store.Put(t.Context(), "kb-1", "tracked", []byte("content")); err != nil {
		t.Fatal(err)
	}
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(previous) })

	svc := testDatasetUpdateService(t)
	svc.docEngine = &countingDeleteEngine{}
	plan := DatasetCleanupPlan{
		KBID:         "kb-1",
		TenantID:     "tenant-1",
		DatasetName:  "Dataset (kb-1)",
		DocLocations: []string{"tracked"},
		DocIDs:       []string{"doc-1"},
	}
	svc.CleanupDatasetExternal(t.Context(), plan)
	if store.ObjExist(t.Context(), "kb-1", "tracked") {
		t.Fatal("first cleanup retained the object")
	}
	// Replay must not fail or panic even though the object and bucket are gone.
	svc.CleanupDatasetExternal(t.Context(), plan)
	if store.BucketExists(t.Context(), "kb-1") {
		t.Fatal("replayed cleanup retained the bucket")
	}
}

// ctxAwareEngine records engine deletes only when the received context is
// still live. It makes the test detect a cleanup that inherited a canceled
// request context: a canceled ctx produces zero counted calls.
type ctxAwareEngine struct {
	countingDeleteEngine
	liveChunkCalls int
	liveMetaCalls  int
}

func (e *ctxAwareEngine) DeleteChunks(ctx context.Context, cond map[string]interface{}, index, kbID string) (int64, error) {
	if ctx.Err() == nil {
		e.liveChunkCalls++
	}
	return e.countingDeleteEngine.DeleteChunks(ctx, cond, index, kbID)
}
func (e *ctxAwareEngine) DeleteMetadata(ctx context.Context, cond map[string]interface{}, tenantID string) (int64, error) {
	if ctx.Err() == nil {
		e.liveMetaCalls++
	}
	return e.countingDeleteEngine.DeleteMetadata(ctx, cond, tenantID)
}

// TestCleanupDatasetExternalDetachesFromRequestContext pins the post-commit
// cleanup contract from review #6069764981: once the DB rows are committed, a
// client disconnect or the API write deadline cancels the request context. The
// cleanup must run to completion on a context detached from that request, or
// the objects, chunks, and metadata are orphaned with no retry path.
func TestCleanupDatasetExternalDetachesFromRequestContext(t *testing.T) {
	store := storage.NewMemoryStorage()
	if err := store.Put(t.Context(), "kb-1", "tracked", []byte("content")); err != nil {
		t.Fatal(err)
	}
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(previous) })

	eng := &ctxAwareEngine{}
	svc := testDatasetUpdateService(t)
	svc.docEngine = eng
	plan := DatasetCleanupPlan{
		KBID:         "kb-1",
		TenantID:     "tenant-1",
		DatasetName:  "Dataset (kb-1)",
		DocLocations: []string{"tracked"},
		DocIDs:       []string{"doc-1"},
	}

	// Simulate a request whose context was canceled after the commit (client
	// disconnect or 180 s WriteTimeout). A cleanup that derives its context
	// from this one will skip every step and orphan the external data.
	reqCtx, cancel := context.WithCancel(t.Context())
	cancel()
	svc.CleanupDatasetExternal(reqCtx, plan)

	if store.ObjExist(t.Context(), "kb-1", "tracked") {
		t.Fatal("cleanup skipped object removal because it inherited a canceled request context")
	}
	if store.BucketExists(t.Context(), "kb-1") {
		t.Fatal("cleanup skipped bucket removal because it inherited a canceled request context")
	}
	if eng.liveChunkCalls == 0 || eng.liveMetaCalls == 0 {
		t.Fatalf("cleanup skipped engine deletes on a canceled context: liveChunkCalls=%d liveMetaCalls=%d", eng.liveChunkCalls, eng.liveMetaCalls)
	}
}
