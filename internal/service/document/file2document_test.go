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

package document

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

// setupFile2DocumentTestEnv initializes an isolated SQLite test environment with all required tables and records.
func setupFile2DocumentTestEnv(t *testing.T) (*File2DocumentService, string, string, string) {
	t.Helper()
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	// Ensure all required tables exist
	if err := db.AutoMigrate(
		&entity.Document{},
		&entity.File{},
		&entity.File2Document{},
		&entity.Knowledgebase{},
		&entity.UserTenant{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	userID := "user-test-1"
	tenantID := userID
	validStatus := string(entity.StatusValid)

	membership := &entity.UserTenant{
		ID:       "membership-1",
		UserID:   userID,
		TenantID: tenantID,
		Role:     string(permission.RoleOwner),
		Status:   &validStatus,
	}
	if err := db.Create(membership).Error; err != nil {
		t.Fatalf("create user tenant failed: %v", err)
	}

	kb := &entity.Knowledgebase{
		ID:         "kb-test-1",
		TenantID:   tenantID,
		CreatedBy:  userID,
		Name:       "test-dataset",
		Permission: string(entity.TenantPermissionMe),
		Status:     &validStatus,
	}
	if err := db.Create(kb).Error; err != nil {
		t.Fatalf("create kb failed: %v", err)
	}

	file := &entity.File{
		ID:        "file-test-1",
		TenantID:  tenantID,
		CreatedBy: userID,
		Name:      "test.pdf",
		Type:      "pdf",
		Size:      1024,
	}
	if err := db.Create(file).Error; err != nil {
		t.Fatalf("create file failed: %v", err)
	}

	return NewFile2DocumentService(), userID, file.ID, kb.ID
}

// createTestKB inserts an extra valid dataset owned by userID.
func createTestKB(t *testing.T, userID, id string) *entity.Knowledgebase {
	t.Helper()
	validStatus := string(entity.StatusValid)
	kb := &entity.Knowledgebase{
		ID:         id,
		TenantID:   userID,
		CreatedBy:  userID,
		Name:       "dataset-" + id,
		Permission: string(entity.TenantPermissionMe),
		Status:     &validStatus,
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("create kb %s failed: %v", id, err)
	}
	return kb
}

// TestLinkToDatasets_SliceIsolation verifies that mutating caller request slice headers after LinkToDatasets returns does not affect the background conversion goroutine.
func TestLinkToDatasets_SliceIsolation(t *testing.T) {
	svc, userID, fileID, kbID := setupFile2DocumentTestEnv(t)
	ctx := t.Context()

	var wg sync.WaitGroup
	wg.Add(1)

	var receivedKBIDs []string
	svc.convertFilesFn = func(ctx context.Context, fileIDs, kbIDs []string, userID, mode string) error {
		defer wg.Done()
		receivedKBIDs = append([]string(nil), kbIDs...)
		return nil
	}

	req := &LinkToDatasetsRequest{
		FileIDs: []string{fileID},
		KbIDs:   []string{kbID},
	}
	err := svc.LinkToDatasets(ctx, userID, req, "add")
	if err != nil {
		t.Fatalf("LinkToDatasets failed: %v", err)
	}

	// Mutate caller slice immediately after LinkToDatasets returns
	req.KbIDs[0] = "mutated-kb-id"

	wg.Wait()

	if len(receivedKBIDs) != 1 || receivedKBIDs[0] != kbID {
		t.Fatalf("expected background goroutine to receive %s, got: %v", kbID, receivedKBIDs)
	}
}

// TestRunConversion_PanicRecovery verifies that a panic inside convertFiles is recovered instead of crashing the process.
func TestRunConversion_PanicRecovery(t *testing.T) {
	svc, userID, fileID, kbID := setupFile2DocumentTestEnv(t)
	svc.convertFilesFn = func(context.Context, []string, []string, string, string) error {
		panic("simulated panic inside background conversion")
	}

	svc.runConversion(t.Context(), []string{fileID}, []string{kbID}, userID, "add")
}

// TestRunConversion_ContextDetachedWithTimeout verifies that the background context survives cancellation of the request context and is bounded by convertTimeout.
func TestRunConversion_ContextDetachedWithTimeout(t *testing.T) {
	svc, userID, fileID, kbID := setupFile2DocumentTestEnv(t)

	reqCtx, cancelReq := context.WithCancel(context.Background())
	cancelReq()

	var ctxErr error
	var hasDeadline bool
	var remaining time.Duration
	svc.convertFilesFn = func(ctx context.Context, _, _ []string, _, _ string) error {
		ctxErr = ctx.Err()
		if deadline, ok := ctx.Deadline(); ok {
			hasDeadline = true
			remaining = time.Until(deadline)
		}
		return nil
	}

	svc.runConversion(reqCtx, []string{fileID}, []string{kbID}, userID, "add")

	if ctxErr != nil {
		t.Fatalf("expected detached context to survive request cancellation, got: %v", ctxErr)
	}
	if !hasDeadline {
		t.Fatal("expected background context to have a deadline")
	}
	if remaining <= convertTimeout-5*time.Second || remaining > convertTimeout {
		t.Fatalf("expected deadline around %v, remaining: %v", convertTimeout, remaining)
	}
}

// TestFile2Document_ConvertFiles_SuccessAndIdempotency tests successful document insertion, mapping creation, counter increment, and idempotency on duplicate conversion.
func TestFile2Document_ConvertFiles_SuccessAndIdempotency(t *testing.T) {
	svc, userID, fileID, kbID := setupFile2DocumentTestEnv(t)
	ctx := t.Context()

	// Initial conversion
	err := svc.convertFiles(ctx, []string{fileID}, []string{kbID}, userID, "add")
	if err != nil {
		t.Fatalf("convertFiles failed: %v", err)
	}

	// Verify document created
	var doc entity.Document
	if err := dao.DB.Where("kb_id = ?", kbID).First(&doc).Error; err != nil {
		t.Fatalf("find document failed: %v", err)
	}
	if doc.Name == nil || *doc.Name != "test.pdf" {
		t.Fatalf("expected doc name test.pdf, got: %v", doc.Name)
	}
	if doc.Type != "pdf" || doc.Suffix != "pdf" {
		t.Fatalf("expected type/suffix pdf, got: %s/%s", doc.Type, doc.Suffix)
	}

	// Verify file2document mapping created
	var mapping entity.File2Document
	if err := dao.DB.Where("file_id = ? AND document_id = ?", fileID, doc.ID).First(&mapping).Error; err != nil {
		t.Fatalf("find mapping failed: %v", err)
	}

	// Verify kb.DocNum is 1
	var kb entity.Knowledgebase
	if err := dao.DB.Where("id = ?", kbID).First(&kb).Error; err != nil {
		t.Fatalf("find kb failed: %v", err)
	}
	if kb.DocNum != 1 {
		t.Fatalf("expected kb.DocNum == 1, got: %d", kb.DocNum)
	}

	// Run again (idempotent duplicate call)
	err = svc.convertFiles(ctx, []string{fileID}, []string{kbID}, userID, "add")
	if err != nil {
		t.Fatalf("second convertFiles failed: %v", err)
	}

	var count int64
	dao.DB.Model(&entity.Document{}).Where("kb_id = ?", kbID).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 document after second convert, got %d", count)
	}
}

// TestFile2Document_ConvertFiles_ReplaceMode tests replace mode behavior, unlinking outdated datasets and establishing links to newly requested datasets.
func TestFile2Document_ConvertFiles_ReplaceMode(t *testing.T) {
	svc, userID, fileID, kb1ID := setupFile2DocumentTestEnv(t)
	ctx := t.Context()

	kb2 := createTestKB(t, userID, "kb-test-2")

	// Link to kb1 first
	if err := svc.convertFiles(ctx, []string{fileID}, []string{kb1ID}, userID, "add"); err != nil {
		t.Fatalf("convertFiles add failed: %v", err)
	}

	// Replace with kb2
	if err := svc.convertFiles(ctx, []string{fileID}, []string{kb2.ID}, userID, "replace"); err != nil {
		t.Fatalf("convertFiles replace failed: %v", err)
	}

	// Verify kb1 doc is removed
	var count1 int64
	dao.DB.Model(&entity.Document{}).Where("kb_id = ?", kb1ID).Count(&count1)
	if count1 != 0 {
		t.Fatalf("expected 0 docs in kb1, got %d", count1)
	}

	// Verify kb2 doc exists
	var doc2 entity.Document
	if err := dao.DB.Where("kb_id = ?", kb2.ID).First(&doc2).Error; err != nil {
		t.Fatalf("find doc2 failed: %v", err)
	}

	// Verify mapping points to doc2
	var mapping entity.File2Document
	if err := dao.DB.Where("file_id = ?", fileID).First(&mapping).Error; err != nil {
		t.Fatalf("find mapping failed: %v", err)
	}
	if mapping.DocumentID == nil || *mapping.DocumentID != doc2.ID {
		t.Fatalf("expected mapping to doc2, got %v", mapping.DocumentID)
	}
}

// TestFile2Document_ConvertFiles_ContextCancelled verifies that convertFiles immediately exits and preserves accumulated errors when context is cancelled.
func TestFile2Document_ConvertFiles_ContextCancelled(t *testing.T) {
	svc, userID, fileID, kbID := setupFile2DocumentTestEnv(t)

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := svc.convertFiles(cancelCtx, []string{fileID}, []string{kbID}, userID, "add")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}

	var count int64
	dao.DB.Model(&entity.Document{}).Where("kb_id = ?", kbID).Count(&count)
	if count != 0 {
		t.Fatalf("expected 0 docs when context cancelled, got %d", count)
	}
}

// TestFile2Document_ConvertFiles_RollbackOnMappingCreateFailure tests rollback compensation, ensuring document row and KB doc_num counter are reverted if mapping creation fails.
func TestFile2Document_ConvertFiles_RollbackOnMappingCreateFailure(t *testing.T) {
	svc, userID, fileID, kbID := setupFile2DocumentTestEnv(t)
	ctx := t.Context()

	// Register a GORM create callback that fails specifically on file2document table
	callbackName := "test:fail_file2document_create"
	if err := dao.DB.Callback().Create().Before("gorm:create").Register(callbackName, func(db *gorm.DB) {
		if db.Statement.Table == "file2document" {
			_ = db.AddError(errors.New("simulated file2document create error"))
		}
	}); err != nil {
		t.Fatalf("register callback failed: %v", err)
	}
	t.Cleanup(func() { _ = dao.DB.Callback().Create().Remove(callbackName) })

	err := svc.convertFiles(ctx, []string{fileID}, []string{kbID}, userID, "add")
	if err == nil || !strings.Contains(err.Error(), "create mapping") {
		t.Fatalf("expected mapping create error, got: %v", err)
	}

	// Verify document was rolled back / removed, and kb.DocNum is 0
	var count int64
	dao.DB.Model(&entity.Document{}).Where("kb_id = ?", kbID).Count(&count)
	if count != 0 {
		t.Fatalf("expected doc to be rolled back, but found %d docs", count)
	}

	var kb entity.Knowledgebase
	if err := dao.DB.Where("id = ?", kbID).First(&kb).Error; err != nil {
		t.Fatalf("find kb failed: %v", err)
	}
	if kb.DocNum != 0 {
		t.Fatalf("expected kb.DocNum == 0 after rollback, got %d", kb.DocNum)
	}
}

// TestFile2Document_ConvertFiles_MappingLookupFailureSkipsFile verifies that an unexpected DB failure
// when looking up an existing document mapping causes convertFiles to record the error and skip the file,
// avoiding phantom duplicate document creation.
func TestFile2Document_ConvertFiles_MappingLookupFailureSkipsFile(t *testing.T) {
	svc, userID, fileID, kbID := setupFile2DocumentTestEnv(t)
	ctx := t.Context()

	// Create an initial document and file2document mapping
	if err := svc.convertFiles(ctx, []string{fileID}, []string{kbID}, userID, "add"); err != nil {
		t.Fatalf("initial convertFiles failed: %v", err)
	}

	// Now register a GORM query callback that fails on document queries
	callbackName := "test:fail_document_query"
	if err := dao.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(db *gorm.DB) {
		if db.Statement.Table == "document" {
			_ = db.AddError(errors.New("simulated document query error"))
		}
	}); err != nil {
		t.Fatalf("register callback failed: %v", err)
	}

	// Run convertFiles again: should fail-safe skip without creating duplicate documents
	err := svc.convertFiles(ctx, []string{fileID}, []string{kbID}, userID, "add")
	if err == nil || !strings.Contains(err.Error(), "get doc") {
		t.Fatalf("expected get doc error, got: %v", err)
	}

	// Remove callback so subsequent verification query can succeed
	_ = dao.DB.Callback().Query().Remove(callbackName)

	var count int64
	if err := dao.DB.Model(&entity.Document{}).Where("kb_id = ?", kbID).Count(&count).Error; err != nil {
		t.Fatalf("count documents failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 document, got: %d", count)
	}
}

// TestFile2Document_ConvertFiles_DanglingMappingCleanedUp verifies that dangling file2document mappings
// pointing to already-deleted documents are self-healed and physically removed during convertFiles.
func TestFile2Document_ConvertFiles_DanglingMappingCleanedUp(t *testing.T) {
	svc, userID, fileID, kbID := setupFile2DocumentTestEnv(t)
	ctx := t.Context()

	// Insert a dangling mapping pointing to a non-existent document
	danglingDocID := "dangling-doc-id"
	danglingMapping := &entity.File2Document{
		ID:         "dangling-map-1",
		FileID:     &fileID,
		DocumentID: &danglingDocID,
	}
	if err := dao.DB.Create(danglingMapping).Error; err != nil {
		t.Fatalf("create dangling mapping failed: %v", err)
	}

	// Run convertFiles
	err := svc.convertFiles(ctx, []string{fileID}, []string{kbID}, userID, "add")
	if err != nil {
		t.Fatalf("convertFiles failed: %v", err)
	}

	// Verify that the dangling mapping was deleted from file2document table
	var dangling entity.File2Document
	err = dao.DB.Where("id = ?", danglingMapping.ID).First(&dangling).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected dangling mapping to be deleted, got err: %v", err)
	}

	// Verify that the real document and mapping were created
	var count int64
	if err := dao.DB.Model(&entity.Document{}).Where("kb_id = ?", kbID).Count(&count).Error; err != nil {
		t.Fatalf("count documents failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 new document, got: %d", count)
	}
}

// TestFile2Document_ConvertFiles_ReplaceModeFailures verifies the failure policy when replacing links:
// a failed document removal aborts the file, while a failed mapping delete (document already gone)
// is reported but the file is still linked to the new dataset.
func TestFile2Document_ConvertFiles_ReplaceModeFailures(t *testing.T) {
	tests := []struct {
		name        string
		failTable   string
		wantErr     string
		wantOldDocs int64
		wantNewDocs int64
	}{
		{name: "document removal fails", failTable: "document", wantErr: "remove doc", wantOldDocs: 1, wantNewDocs: 0},
		{name: "mapping delete fails", failTable: "file2document", wantErr: "delete mapping", wantOldDocs: 0, wantNewDocs: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userID, fileID, kb1ID := setupFile2DocumentTestEnv(t)
			ctx := t.Context()
			kb2 := createTestKB(t, userID, "kb-test-2")

			if err := svc.convertFiles(ctx, []string{fileID}, []string{kb1ID}, userID, "add"); err != nil {
				t.Fatalf("convertFiles add failed: %v", err)
			}

			callbackName := "test:fail_delete"
			if err := dao.DB.Callback().Delete().Before("gorm:delete").Register(callbackName, func(db *gorm.DB) {
				if db.Statement.Table == tt.failTable {
					_ = db.AddError(errors.New("simulated delete error"))
				}
			}); err != nil {
				t.Fatalf("register callback failed: %v", err)
			}

			err := svc.convertFiles(ctx, []string{fileID}, []string{kb2.ID}, userID, "replace")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected %q error, got: %v", tt.wantErr, err)
			}

			// Remove the callback so the verification queries can run.
			_ = dao.DB.Callback().Delete().Remove(callbackName)

			var oldDocs, newDocs int64
			dao.DB.Model(&entity.Document{}).Where("kb_id = ?", kb1ID).Count(&oldDocs)
			dao.DB.Model(&entity.Document{}).Where("kb_id = ?", kb2.ID).Count(&newDocs)
			if oldDocs != tt.wantOldDocs || newDocs != tt.wantNewDocs {
				t.Fatalf("expected %d old and %d new docs, got %d and %d", tt.wantOldDocs, tt.wantNewDocs, oldDocs, newDocs)
			}
		})
	}
}
