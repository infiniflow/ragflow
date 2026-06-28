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

package dao

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// setupTestDB initializes an in-memory SQLite database for DAO tests.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	// Migrate only the tables needed for file2document tests
	if err := db.AutoMigrate(&entity.File2Document{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

// saveDB saves the test DB and restores the original after the test.
func pushDB(t *testing.T, testDB *gorm.DB) {
	t.Helper()
	orig := DB
	DB = testDB
	t.Cleanup(func() {
		DB = orig
	})
}

func testFile2Document(t *testing.T, fileID, docID string) *entity.File2Document {
	t.Helper()
	f2d := &entity.File2Document{
		ID:         fileID + "_" + docID,
		FileID:     &fileID,
		DocumentID: &docID,
	}
	if err := DB.Create(f2d).Error; err != nil {
		t.Fatalf("failed to create test record: %v", err)
	}
	return f2d
}

func TestFile2DocumentDAO_GetByDocumentID(t *testing.T) {
	db := setupTestDB(t)
	pushDB(t, db)
	dao := NewFile2DocumentDAO()

	f2d := testFile2Document(t, "file-1", "doc-1")

	results, err := dao.GetByDocumentID("doc-1")
	if err != nil {
		t.Fatalf("GetByDocumentID failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if *results[0].FileID != *f2d.FileID {
		t.Fatalf("file_id mismatch: expected %q, got %q", *f2d.FileID, *results[0].FileID)
	}
	if *results[0].DocumentID != *f2d.DocumentID {
		t.Fatalf("document_id mismatch: expected %q, got %q", *f2d.DocumentID, *results[0].DocumentID)
	}
}

func TestFile2DocumentDAO_GetByDocumentID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	pushDB(t, db)
	dao := NewFile2DocumentDAO()

	results, err := dao.GetByDocumentID("nonexistent")
	if err != nil {
		t.Fatalf("GetByDocumentID failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestFile2DocumentDAO_GetByDocumentID_MultipleResults(t *testing.T) {
	db := setupTestDB(t)
	pushDB(t, db)
	dao := NewFile2DocumentDAO()

	testFile2Document(t, "file-1", "doc-shared")
	testFile2Document(t, "file-2", "doc-shared")

	results, err := dao.GetByDocumentID("doc-shared")
	if err != nil {
		t.Fatalf("GetByDocumentID failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestFile2DocumentDAO_DeleteByDocumentID(t *testing.T) {
	db := setupTestDB(t)
	pushDB(t, db)
	dao := NewFile2DocumentDAO()

	testFile2Document(t, "file-1", "doc-del")
	testFile2Document(t, "file-2", "doc-del")
	testFile2Document(t, "file-3", "doc-keep")

	err := dao.DeleteByDocumentID("doc-del")
	if err != nil {
		t.Fatalf("DeleteByDocumentID failed: %v", err)
	}

	// Verify deleted records are gone
	results, err := dao.GetByDocumentID("doc-del")
	if err != nil {
		t.Fatalf("GetByDocumentID failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results after delete, got %d", len(results))
	}

	// Verify other document's records are untouched
	results, err = dao.GetByDocumentID("doc-keep")
	if err != nil {
		t.Fatalf("GetByDocumentID failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 untouched result, got %d", len(results))
	}
}

func TestFile2DocumentDAO_DeleteByDocumentID_Noop(t *testing.T) {
	db := setupTestDB(t)
	pushDB(t, db)
	dao := NewFile2DocumentDAO()

	err := dao.DeleteByDocumentID("nonexistent")
	if err != nil {
		t.Fatalf("DeleteByDocumentID should not error on missing: %v", err)
	}
}
