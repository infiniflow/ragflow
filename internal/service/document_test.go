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

package service

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// setupServiceTestDB initializes an in-memory SQLite database for service tests.
func setupServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	// Migrate tables used by deleteDocumentFull + DeleteDocuments
	if err := db.AutoMigrate(
		&entity.Document{},
		&entity.Knowledgebase{},
		&entity.Task{},
		&entity.File2Document{},
		&entity.File{},
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

// pushServiceDB swaps dao.DB for the test and restores after.
func pushServiceDB(t *testing.T, testDB *gorm.DB) {
	t.Helper()
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() {
		dao.DB = orig
	})
}

func testDocumentService(t *testing.T) *DocumentService {
	t.Helper()
	// Use nil engine since we test DB cleanup only; engine ops are nil-guarded.
	return &DocumentService{
		documentDAO:      dao.NewDocumentDAO(),
		kbDAO:            dao.NewKnowledgebaseDAO(),
		taskDAO:          dao.NewTaskDAO(),
		file2DocumentDAO: dao.NewFile2DocumentDAO(),
		docEngine:        nil,
		metadataSvc:      nil, // nil engine → metadata ops skipped
	}
}

// sptr returns a pointer to the given string.
func sptr(s string) *string { return &s }

func insertTestKB(t *testing.T, id, tenantID string, docNum, tokenNum, chunkNum int64) {
	t.Helper()
	kb := &entity.Knowledgebase{
		ID:       id,
		TenantID: tenantID,
		Name:     "test-kb",
		EmbdID:   "embd-1",
		CreatedBy: "user-1",
		Permission: string(entity.TenantPermissionTeam),
		DocNum:   docNum,
		TokenNum: tokenNum,
		ChunkNum: chunkNum,
		Status:   sptr(string(entity.StatusValid)),
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert test kb: %v", err)
	}
}

func insertTestDoc(t *testing.T, id, kbID string, tokenNum, chunkNum int64) {
	t.Helper()
	doc := &entity.Document{
		ID:           id,
		KbID:         kbID,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		TokenNum:     tokenNum,
		ChunkNum:     chunkNum,
		Suffix:       ".txt",
		Status:       sptr("1"),
	}
	if err := dao.DB.Create(doc).Error; err != nil {
		t.Fatalf("insert test doc: %v", err)
	}
}

func insertTestTask(t *testing.T, id, docID string) {
	t.Helper()
	task := &entity.Task{
		ID:    id,
		DocID: docID,
	}
	if err := dao.DB.Create(task).Error; err != nil {
		t.Fatalf("insert test task: %v", err)
	}
}

func insertTestFile2Document(t *testing.T, id, fileID, docID string) {
	t.Helper()
	f2d := &entity.File2Document{
		ID:         id,
		FileID:     &fileID,
		DocumentID: &docID,
	}
	if err := dao.DB.Create(f2d).Error; err != nil {
		t.Fatalf("insert test f2d: %v", err)
	}
}

func insertTestFile(t *testing.T, id, parentID, name string, location *string) {
	t.Helper()
	srcType := string(entity.FileSourceKnowledgebase)
	f := &entity.File{
		ID:         id,
		ParentID:   parentID,
		TenantID:   "tenant-1",
		CreatedBy:  "user-1",
		Name:       name,
		Location:   location,
		SourceType: srcType,
		Type:       "pdf",
	}
	if err := dao.DB.Create(f).Error; err != nil {
		t.Fatalf("insert test file: %v", err)
	}
}

func TestDeleteDocumentFull_Basic(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 3, 100, 50)
	insertTestDoc(t, "doc-1", "kb-1", 30, 10)
	insertTestTask(t, "task-1", "doc-1")

	svc := testDocumentService(t)

	err := svc.deleteDocumentFull("doc-1")
	if err != nil {
		t.Fatalf("deleteDocumentFull failed: %v", err)
	}

	// Verify document deleted
	_, err = dao.NewDocumentDAO().GetByID("doc-1")
	if err == nil {
		t.Fatal("document should be deleted but it still exists")
	}

	// Verify tasks deleted
	tasks, _ := dao.NewTaskDAO().GetAllTasks()
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}

	// Verify KB counters decremented
	kb, err := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if err != nil {
		t.Fatalf("kb not found: %v", err)
	}
	if kb.DocNum != 2 {
		t.Fatalf("doc_num: expected 2, got %d", kb.DocNum)
	}
	if kb.TokenNum != 70 {
		t.Fatalf("token_num: expected 70, got %d", kb.TokenNum)
	}
	if kb.ChunkNum != 40 {
		t.Fatalf("chunk_num: expected 40, got %d", kb.ChunkNum)
	}
}

func TestDeleteDocumentFull_NotFound(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	svc := testDocumentService(t)

	err := svc.deleteDocumentFull("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent document")
	}
}

func TestDeleteDocumentFull_CleansUpFile2Document(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDoc(t, "doc-1", "kb-1", 10, 5)
	loc := "path/to/blob"
	insertTestFile(t, "file-1", "kb-1", "test.pdf", &loc)
	insertTestFile2Document(t, "f2d-1", "file-1", "doc-1")

	svc := testDocumentService(t)

	err := svc.deleteDocumentFull("doc-1")
	if err != nil {
		t.Fatalf("deleteDocumentFull failed: %v", err)
	}

	// Verify f2d mapping deleted
	f2dDAO := dao.NewFile2DocumentDAO()
	mappings, _ := f2dDAO.GetByDocumentID("doc-1")
	if len(mappings) != 0 {
		t.Fatalf("expected 0 f2d mappings, got %d", len(mappings))
	}

	// Verify file record deleted (hard delete)
	files, _ := dao.NewFileDAO().GetByIDs([]string{"file-1"})
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

func TestDeleteDocumentFull_SharedFilePreserved(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 2, 20, 10)
	insertTestDoc(t, "doc-1", "kb-1", 10, 5)
	insertTestDoc(t, "doc-2", "kb-1", 10, 5)
	loc := "shared/blob"
	insertTestFile(t, "file-shared", "kb-1", "shared.pdf", &loc)

	// Same file linked to TWO documents
	insertTestFile2Document(t, "f2d-1", "file-shared", "doc-1")
	insertTestFile2Document(t, "f2d-2", "file-shared", "doc-2")

	svc := testDocumentService(t)

	// Delete doc-1; file-shared should survive because doc-2 still references it
	err := svc.deleteDocumentFull("doc-1")
	if err != nil {
		t.Fatalf("deleteDocumentFull failed: %v", err)
	}

	// f2d mapping for doc-1 should be gone
	f2dDAO := dao.NewFile2DocumentDAO()
	mappings, _ := f2dDAO.GetByDocumentID("doc-1")
	if len(mappings) != 0 {
		t.Fatalf("expected 0 f2d mappings for doc-1, got %d", len(mappings))
	}

	// file record should still exist (doc-2 still references it)
	files, _ := dao.NewFileDAO().GetByIDs([]string{"file-shared"})
	if len(files) != 1 {
		t.Fatalf("expected 1 file record to survive, got %d", len(files))
	}

	// f2d mapping for doc-2 should still exist
	mappings, _ = f2dDAO.GetByDocumentID("doc-2")
	if len(mappings) != 1 {
		t.Fatalf("expected 1 f2d mapping for doc-2, got %d", len(mappings))
	}
}

func insertUserTenantForAccessCheck(t *testing.T, userID, tenantID string) {
	t.Helper()
	// Insert user if not exists (email is NOT NULL, password is nullable pointer)
	var existingUser entity.User
	if err := dao.DB.Where("id = ?", userID).First(&existingUser).Error; err != nil {
		u := &entity.User{ID: userID, Nickname: "test-user", Email: userID + "@test.com", Password: sptr("x")}
		if err := dao.DB.Create(u).Error; err != nil {
			t.Fatalf("insert test user: %v", err)
		}
	}
	// Insert tenant if not exists (llm_id, embd_id, asr_id are NOT NULL)
	var existingTenant entity.Tenant
	if err := dao.DB.Where("id = ?", tenantID).First(&existingTenant).Error; err != nil {
		tn := &entity.Tenant{
			ID:     tenantID,
			LLMID:  "llm-default",
			EmbdID: "embd-default",
			ASRID:  "asr-default",
		}
		if err := dao.DB.Create(tn).Error; err != nil {
			t.Fatalf("insert test tenant: %v", err)
		}
	}
	// Insert user_tenant mapping if not exists
	var existingUT entity.UserTenant
	if err := dao.DB.Where("user_id = ? AND tenant_id = ?", userID, tenantID).First(&existingUT).Error; err != nil {
		ut := &entity.UserTenant{
			ID:       userID + "_" + tenantID,
			UserID:   userID,
			TenantID: tenantID,
			Role:     "admin",
		}
		if err := dao.DB.Create(ut).Error; err != nil {
			t.Fatalf("insert test user_tenant: %v", err)
		}
	}
}

func TestDeleteDocuments_DeleteAll(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")

	insertTestKB(t, "kb-1", "tenant-1", 3, 100, 50)
	insertTestDoc(t, "doc-1", "kb-1", 30, 10)
	insertTestDoc(t, "doc-2", "kb-1", 40, 20)
	insertTestDoc(t, "doc-3", "kb-1", 30, 20)

	svc := testDocumentService(t)

	deleted, err := svc.DeleteDocuments(nil, true, "kb-1", "user-1")
	if err != nil {
		t.Fatalf("DeleteDocuments failed: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("expected 3 deleted, got %d", deleted)
	}

	// KB counters: doc_num 3→0, token_num 100→0, chunk_num 50→0
	kb, _ := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if kb.DocNum != 0 {
		t.Fatalf("doc_num: expected 0, got %d", kb.DocNum)
	}
}

func TestDeleteDocuments_ByIDs(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")

	insertTestKB(t, "kb-1", "tenant-1", 3, 100, 50)
	insertTestDoc(t, "doc-1", "kb-1", 30, 10)
	insertTestDoc(t, "doc-2", "kb-1", 40, 20)
	insertTestDoc(t, "doc-3", "kb-1", 30, 20) // won't be deleted

	svc := testDocumentService(t)

	deleted, err := svc.DeleteDocuments([]string{"doc-1", "doc-2"}, false, "kb-1", "user-1")
	if err != nil {
		t.Fatalf("DeleteDocuments failed: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 deleted, got %d", deleted)
	}

	// doc-3 should still exist
	_, err = dao.NewDocumentDAO().GetByID("doc-3")
	if err != nil {
		t.Fatal("doc-3 should not have been deleted")
	}
}

func TestDeleteDocuments_WrongDataset(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")
	insertUserTenantForAccessCheck(t, "user-1", "tenant-2")

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestKB(t, "kb-2", "tenant-2", 1, 10, 5)
	insertTestDoc(t, "doc-1", "kb-2", 10, 5) // belongs to kb-2, not kb-1

	svc := testDocumentService(t)

	_, err := svc.DeleteDocuments([]string{"doc-1"}, false, "kb-1", "user-1")
	if err == nil {
		t.Fatal("expected error for doc not belonging to dataset")
	}
}

func TestDeleteDocuments_NotAccessible(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)

	svc := testDocumentService(t)

	// user-1 has no user_tenant entry → accessible returns false
	_, err := svc.DeleteDocuments([]string{"doc-1"}, false, "kb-1", "user-1")
	if err == nil {
		t.Fatal("expected error for inaccessible dataset")
	}
}

func TestDeleteDocuments_EmptyIDs(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")
	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0)

	svc := testDocumentService(t)

	// Empty ids, no deleteAll → returns 0, no error
	deleted, err := svc.DeleteDocuments([]string{}, false, "kb-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deleted, got %d", deleted)
	}
}

func TestDeleteDocuments_Deduplicate(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDoc(t, "doc-1", "kb-1", 10, 5)

	svc := testDocumentService(t)

	deleted, err := svc.DeleteDocuments([]string{"doc-1", "doc-1", "doc-1"}, false, "kb-1", "user-1")
	if err != nil {
		t.Fatalf("DeleteDocuments failed: %v", err)
	}
	// Dedup should result in only 1 delete
	if deleted != 1 {
		t.Fatalf("expected 1 deleted after dedup, got %d", deleted)
	}
}

// insertTestDocWithRun inserts a document with the given Run status for StopParseDocuments tests.
func insertTestDocWithRun(t *testing.T, id, kbID, run string, tokenNum, chunkNum int64) {
	t.Helper()
	doc := &entity.Document{
		ID:           id,
		KbID:         kbID,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		TokenNum:     tokenNum,
		ChunkNum:     chunkNum,
		Suffix:       ".txt",
		Status:       sptr("1"),
		Run:          &run,
	}
	if err := dao.DB.Create(doc).Error; err != nil {
		t.Fatalf("insert test doc: %v", err)
	}
}

// insertTestTaskWithProgress inserts a task with the given progress value.
func insertTestTaskWithProgress(t *testing.T, id, docID string, progress float64) {
	t.Helper()
	task := &entity.Task{
		ID:       id,
		DocID:    docID,
		Progress: progress,
	}
	if err := dao.DB.Create(task).Error; err != nil {
		t.Fatalf("insert test task: %v", err)
	}
}

func TestStopParseDocuments_Success(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDocWithRun(t, "doc-1", "kb-1", string(entity.TaskStatusRunning), 10, 5)
	insertTestTask(t, "task-1", "doc-1")

	svc := testDocumentService(t)

	result, err := svc.StopParseDocuments("kb-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("StopParseDocuments failed: %v", err)
	}

	sc, ok := result["success_count"].(int)
	if !ok {
		t.Fatalf("success_count not found or wrong type: %v", result)
	}
	if sc != 1 {
		t.Fatalf("expected success_count=1, got %d", sc)
	}

	// Verify document run status updated to CANCEL
	doc, _ := dao.NewDocumentDAO().GetByID("doc-1")
	if doc == nil || doc.Run == nil {
		t.Fatal("doc not found or run is nil")
	}
	if *doc.Run != string(entity.TaskStatusCancel) {
		t.Fatalf("expected run=%q, got %q", string(entity.TaskStatusCancel), *doc.Run)
	}
}

func TestStopParseDocuments_CancelStatus(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	// Doc is already in CANCEL state — should still be accepted
	insertTestDocWithRun(t, "doc-1", "kb-1", string(entity.TaskStatusCancel), 10, 5)
	insertTestTask(t, "task-1", "doc-1")

	svc := testDocumentService(t)

	result, err := svc.StopParseDocuments("kb-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("StopParseDocuments failed: %v", err)
	}

	sc := result["success_count"].(int)
	if sc != 1 {
		t.Fatalf("expected success_count=1, got %d", sc)
	}
}

func TestStopParseDocuments_NotRunningOrCancel(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	// Doc with Run="0" (UNSTART) and no unfinished tasks → cannot cancel
	insertTestDocWithRun(t, "doc-1", "kb-1", string(entity.TaskStatusUnstart), 10, 5)

	svc := testDocumentService(t)

	result, err := svc.StopParseDocuments("kb-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("StopParseDocuments failed: %v", err)
	}

	sc := result["success_count"].(int)
	if sc != 0 {
		t.Fatalf("expected success_count=0, got %d", sc)
	}
	errors, ok := result["errors"].([]string)
	if !ok || len(errors) == 0 {
		t.Fatal("expected errors in result")
	}
}

func TestStopParseDocuments_UnfinishedTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	// Doc with Run="0" but has an unfinished task (progress < 1) → can cancel
	insertTestDocWithRun(t, "doc-1", "kb-1", string(entity.TaskStatusUnstart), 10, 5)
	insertTestTaskWithProgress(t, "task-1", "doc-1", 0.0)

	svc := testDocumentService(t)

	result, err := svc.StopParseDocuments("kb-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("StopParseDocuments failed: %v", err)
	}

	sc := result["success_count"].(int)
	if sc != 1 {
		t.Fatalf("expected success_count=1 (has unfinished task), got %d", sc)
	}
}

func TestStopParseDocuments_WrongDataset(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestKB(t, "kb-2", "tenant-1", 1, 10, 5)
	insertTestDocWithRun(t, "doc-1", "kb-2", string(entity.TaskStatusRunning), 10, 5)

	svc := testDocumentService(t)

	_, err := svc.StopParseDocuments("kb-1", []string{"doc-1"})
	if err == nil {
		t.Fatal("expected error for doc not belonging to dataset")
	}
}

func TestStopParseDocuments_NotFound(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0)

	svc := testDocumentService(t)

	_, err := svc.StopParseDocuments("kb-1", []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent document IDs")
	}
}

func TestStopParseDocuments_EmptyIDs(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0)

	svc := testDocumentService(t)

	_, err := svc.StopParseDocuments("kb-1", []string{})
	if err == nil {
		t.Fatal("expected error for empty doc IDs")
	}
}

func TestStopParseDocuments_Deduplicate(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDocWithRun(t, "doc-1", "kb-1", string(entity.TaskStatusRunning), 10, 5)
	insertTestTask(t, "task-1", "doc-1")

	svc := testDocumentService(t)

	result, err := svc.StopParseDocuments("kb-1", []string{"doc-1", "doc-1", "doc-1"})
	if err != nil {
		t.Fatalf("StopParseDocuments failed: %v", err)
	}

	// Dedup should result in only 1 success
	sc := result["success_count"].(int)
	if sc != 1 {
		t.Fatalf("expected success_count=1 after dedup, got %d", sc)
	}
}

func TestDeleteDocument_DeligatesToFullCleanup(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 5, 2)
	insertTestDoc(t, "doc-1", "kb-1", 5, 2)

	svc := testDocumentService(t)

	// Public DeleteDocument should delegate to deleteDocumentFull
	err := svc.DeleteDocument("doc-1")
	if err != nil {
		t.Fatalf("DeleteDocument failed: %v", err)
	}

	_, err = dao.NewDocumentDAO().GetByID("doc-1")
	if err == nil {
		t.Fatal("document should be deleted")
	}
}

// --- Sub-method tests ---

func TestResolveDocAndKB_Success(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDoc(t, "doc-1", "kb-1", 10, 5)

	svc := testDocumentService(t)

	doc, kb, err := svc.resolveDocAndKB("doc-1")
	if err != nil {
		t.Fatalf("resolveDocAndKB: %v", err)
	}
	if doc.ID != "doc-1" {
		t.Fatalf("doc ID mismatch: %s", doc.ID)
	}
	if kb.ID != "kb-1" {
		t.Fatalf("kb ID mismatch: %s", kb.ID)
	}
	if kb.TenantID != "tenant-1" {
		t.Fatalf("tenant ID mismatch: %s", kb.TenantID)
	}
}

func TestResolveDocAndKB_DocNotFound(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	svc := testDocumentService(t)

	_, _, err := svc.resolveDocAndKB("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent doc")
	}
}

func TestResolveDocAndKB_KBNotFound(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	// Insert a doc with kb_id that has no KB row
	d := &entity.Document{
		ID: "orphan-doc", KbID: "no-such-kb", ParserID: "naive",
		ParserConfig: entity.JSONMap{}, Suffix: ".txt", Status: sptr("1"),
	}
	if err := dao.DB.Create(d).Error; err != nil {
		t.Fatalf("insert doc: %v", err)
	}

	svc := testDocumentService(t)

	_, _, err := svc.resolveDocAndKB("orphan-doc")
	if err == nil {
		t.Fatal("expected error for nonexistent KB")
	}
}

func TestDeleteDocRecordWithCounters_Success(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 3, 100, 50)
	insertTestDoc(t, "doc-1", "kb-1", 30, 10)

	doc, _ := dao.NewDocumentDAO().GetByID("doc-1")
	svc := testDocumentService(t)

	err := svc.deleteDocRecordWithCounters(doc, "kb-1")
	if err != nil {
		t.Fatalf("deleteDocRecordWithCounters: %v", err)
	}

	// Doc gone
	_, err = dao.NewDocumentDAO().GetByID("doc-1")
	if err == nil {
		t.Fatal("document should be deleted")
	}

	// Counters decremented
	kb, _ := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if kb.DocNum != 2 {
		t.Fatalf("doc_num: expected 2, got %d", kb.DocNum)
	}
	if kb.TokenNum != 70 {
		t.Fatalf("token_num: expected 70, got %d", kb.TokenNum)
	}
	if kb.ChunkNum != 40 {
		t.Fatalf("chunk_num: expected 40, got %d", kb.ChunkNum)
	}
}

func TestDeleteDocRecordWithCounters_DocAlreadyDeleted(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDoc(t, "doc-1", "kb-1", 10, 5)

	doc, _ := dao.NewDocumentDAO().GetByID("doc-1")
	svc := testDocumentService(t)

	// First delete: row removed, counters decremented
	if err := svc.deleteDocRecordWithCounters(doc, "kb-1"); err != nil {
		t.Fatalf("first delete: %v", err)
	}

	// Second delete: RowsAffected==0 → counters NOT decremented again
	if err := svc.deleteDocRecordWithCounters(doc, "kb-1"); err != nil {
		t.Fatalf("second delete should not error: %v", err)
	}

	// KB counters should be decremented exactly once: 1→0 for doc_num
	kb, _ := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if kb.DocNum != 0 {
		t.Fatalf("doc_num: expected 0 (decremented once), got %d", kb.DocNum)
	}
	if kb.TokenNum != 0 {
		t.Fatalf("token_num: expected 0, got %d", kb.TokenNum)
	}
	if kb.ChunkNum != 0 {
		t.Fatalf("chunk_num: expected 0, got %d", kb.ChunkNum)
	}
}

func TestCleanupFileReferences_NoMappings(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	svc := testDocumentService(t)
	// Should not panic with no f2d mappings
	svc.cleanupFileReferences("no-mappings")
}

func TestCleanupFileReferences_SingleFileDeleted(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	loc := "blob/path"
	insertTestFile(t, "file-1", "kb-1", "test.pdf", &loc)
	insertTestFile2Document(t, "f2d-1", "file-1", "doc-1")

	svc := testDocumentService(t)
	svc.cleanupFileReferences("doc-1")

	// f2d gone
	mappings, _ := dao.NewFile2DocumentDAO().GetByDocumentID("doc-1")
	if len(mappings) != 0 {
		t.Fatalf("expected 0 f2d after cleanup, got %d", len(mappings))
	}
	// file record gone
	files, _ := dao.NewFileDAO().GetByIDs([]string{"file-1"})
	if len(files) != 0 {
		t.Fatalf("expected 0 files after cleanup, got %d", len(files))
	}
}

func TestCleanupFileReferences_SharedFileSurvives(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	loc := "shared/blob"
	insertTestFile(t, "file-shared", "kb-1", "shared.pdf", &loc)
	insertTestFile2Document(t, "f2d-1", "file-shared", "doc-1")
	insertTestFile2Document(t, "f2d-2", "file-shared", "doc-2")

	svc := testDocumentService(t)
	svc.cleanupFileReferences("doc-1")

	// f2d for doc-1 gone
	mappings, _ := dao.NewFile2DocumentDAO().GetByDocumentID("doc-1")
	if len(mappings) != 0 {
		t.Fatalf("expected 0 f2d for doc-1, got %d", len(mappings))
	}
	// file record survives
	files, _ := dao.NewFileDAO().GetByIDs([]string{"file-shared"})
	if len(files) != 1 {
		t.Fatalf("expected 1 file record, got %d", len(files))
	}
	// f2d for doc-2 survives
	mappings, _ = dao.NewFile2DocumentDAO().GetByDocumentID("doc-2")
	if len(mappings) != 1 {
		t.Fatalf("expected 1 f2d for doc-2, got %d", len(mappings))
	}
}

func TestArtifactHelpers(t *testing.T) {
	// Test sanitizeArtifactFilename
	safe := sanitizeArtifactFilename("test@#file.txt")
	if safe != "test__file.txt" {
		t.Errorf("expected test__file.txt, got %s", safe)
	}

	// Test shouldForceArtifactAttachment
	if !shouldForceArtifactAttachment(".html", "text/html") {
		t.Error("expected true for .html")
	}
	if shouldForceArtifactAttachment(".txt", "text/plain") {
		t.Error("expected false for .txt")
	}
}

func TestGetDocumentArtifact_InvalidFilename(t *testing.T) {
	svc := testDocumentService(t)
	_, err := svc.GetDocumentArtifact("../test.txt")
	if err != ErrArtifactInvalidFilename {
		t.Errorf("expected ErrArtifactInvalidFilename, got %v", err)
	}
}

func TestGetDocumentArtifact_InvalidFileType(t *testing.T) {
	svc := testDocumentService(t)
	_, err := svc.GetDocumentArtifact("test.exe")
	if err != ErrArtifactInvalidFileType {
		t.Errorf("expected ErrArtifactInvalidFileType, got %v", err)
	}
}

func TestGetDocumentPreview_DocumentNotFound(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	svc := testDocumentService(t)

	_, err := svc.GetDocumentPreview("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent document")
	}
}

func TestDownloadDocument_MissingDocID(t *testing.T) {
	svc := testDocumentService(t)
	_, err := svc.DownloadDocument("ds-1", "")
	if err == nil {
		t.Error("expected error for missing docID")
	}
}

func TestDownloadDocument_WrongDataset(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 5, 2)
	insertTestDoc(t, "doc-1", "kb-1", 5, 2)
	svc := testDocumentService(t)

	_, err := svc.DownloadDocument("wrong-ds", "doc-1")
	if err == nil {
		t.Error("expected error for wrong dataset")
	}
}
