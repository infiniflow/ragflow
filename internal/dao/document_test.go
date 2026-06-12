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

func setupDocumentTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.Document{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func pushDocDB(t *testing.T, testDB *gorm.DB) {
	t.Helper()
	orig := DB
	DB = testDB
	t.Cleanup(func() { DB = orig })
}

func TestDocumentGetByIDs_Success(t *testing.T) {
	db := setupDocumentTestDB(t)
	pushDocDB(t, db)

	db.Create(&entity.Document{ID: "doc1", KbID: "kb1", Name: sp("Doc 1"), CreatedBy: "user1", ParserConfig: entity.JSONMap{}})
	db.Create(&entity.Document{ID: "doc2", KbID: "kb1", Name: sp("Doc 2"), CreatedBy: "user1", ParserConfig: entity.JSONMap{}})
	db.Create(&entity.Document{ID: "doc3", KbID: "kb2", Name: sp("Doc 3"), CreatedBy: "user2", ParserConfig: entity.JSONMap{}})

	dao := NewDocumentDAO()
	docs, err := dao.GetByIDs([]string{"doc1", "doc3"})
	if err != nil {
		t.Fatalf("GetByIDs failed: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(docs))
	}

	ids := make(map[string]bool)
	for _, d := range docs {
		ids[d.ID] = true
	}
	if !ids["doc1"] || !ids["doc3"] {
		t.Errorf("expected doc1 and doc3, got %v", ids)
	}
}

func TestDocumentGetByIDs_EmptyIDs(t *testing.T) {
	db := setupDocumentTestDB(t)
	pushDocDB(t, db)

	dao := NewDocumentDAO()
	docs, err := dao.GetByIDs([]string{})
	if err != nil {
		t.Fatalf("GetByIDs failed: %v", err)
	}
	if docs != nil {
		t.Errorf("expected nil for empty IDs, got %v", docs)
	}
}

func TestDocumentGetByIDs_NilIDs(t *testing.T) {
	db := setupDocumentTestDB(t)
	pushDocDB(t, db)

	dao := NewDocumentDAO()
	docs, err := dao.GetByIDs(nil)
	if err != nil {
		t.Fatalf("GetByIDs failed: %v", err)
	}
	if docs != nil {
		t.Errorf("expected nil for nil IDs, got %v", docs)
	}
}

func TestDocumentGetByIDs_NoMatch(t *testing.T) {
	db := setupDocumentTestDB(t)
	pushDocDB(t, db)

	db.Create(&entity.Document{ID: "doc1", KbID: "kb1", Name: sp("Doc 1"), CreatedBy: "user1", ParserConfig: entity.JSONMap{}})

	dao := NewDocumentDAO()
	docs, err := dao.GetByIDs([]string{"nonexistent"})
	if err != nil {
		t.Fatalf("GetByIDs failed: %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("expected 0 docs, got %d", len(docs))
	}
}

func TestDocumentGetByDocumentIDAndDatasetIDUsesKBID(t *testing.T) {
	db := setupDocumentTestDB(t)
	pushDocDB(t, db)

	db.Create(&entity.Document{ID: "doc1", KbID: "kb1", Name: sp("Doc 1"), CreatedBy: "user1", ParserConfig: entity.JSONMap{}})
	db.Create(&entity.Document{ID: "doc1-other", KbID: "kb2", Name: sp("Doc 2"), CreatedBy: "user1", ParserConfig: entity.JSONMap{}})

	doc, err := NewDocumentDAO().GetByDocumentIDAndDatasetID("doc1", "kb1")
	if err != nil {
		t.Fatalf("GetByDocumentIDAndDatasetID failed: %v", err)
	}
	if doc.ID != "doc1" || doc.KbID != "kb1" {
		t.Fatalf("unexpected document: id=%s kb_id=%s", doc.ID, doc.KbID)
	}

	if _, err := NewDocumentDAO().GetByDocumentIDAndDatasetID("doc1", "kb2"); err == nil {
		t.Fatal("expected no match when document does not belong to dataset")
	}
}

func sp(s string) *string { return &s }
