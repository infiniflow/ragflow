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

func setupFileTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&entity.File{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func testFile(t *testing.T, db *gorm.DB, id, parentID, tenantID, name, fileType string) {
	t.Helper()
	f := &entity.File{
		ID:        id,
		ParentID:  parentID,
		TenantID:  tenantID,
		CreatedBy: tenantID,
		Name:      name,
		Type:      fileType,
	}
	if err := db.Create(f).Error; err != nil {
		t.Fatalf("failed to create file %s: %v", id, err)
	}
}

// seedFileTree builds: root -> {dirA -> {subB -> [notes-deep.txt]}, top-report.pdf}, other -> [outside-report.txt]
func seedFileTree(t *testing.T, db *gorm.DB) {
	t.Helper()
	testFile(t, db, "root", "root", "t1", "/", "folder")
	testFile(t, db, "dirA", "root", "t1", "dirA", "folder")
	testFile(t, db, "subB", "dirA", "t1", "subB", "folder")
	testFile(t, db, "f-deep", "subB", "t1", "notes-deep.txt", "doc")
	testFile(t, db, "f-top", "root", "t1", "top-report.pdf", "doc")
	testFile(t, db, "other", "other", "t1", "other", "folder")
	testFile(t, db, "f-out", "other", "t1", "outside-report.txt", "doc")
	testFile(t, db, "f-t2", "root", "t2", "report-t2.txt", "doc")
}

func TestFileDAO_GetByPfID_KeywordsSearchesSubtree(t *testing.T) {
	db := setupFileTestDB(t)
	seedFileTree(t, db)
	d := NewFileDAO()
	ctx := t.Context()

	files, total, err := d.GetByPfID(ctx, db, "t1", "root", 1, 15, "create_time", true, "report", false)
	if err != nil {
		t.Fatalf("GetByPfID failed: %v", err)
	}
	if total != 1 || len(files) != 1 || files[0].ID != "f-top" {
		t.Fatalf("expected only f-top, got total=%d files=%v", total, files)
	}

	// Nested file two levels down must be found from the root folder.
	files, total, err = d.GetByPfID(ctx, db, "t1", "root", 1, 15, "create_time", true, "notes", false)
	if err != nil {
		t.Fatalf("GetByPfID failed: %v", err)
	}
	if total != 1 || len(files) != 1 || files[0].ID != "f-deep" {
		t.Fatalf("expected nested f-deep, got total=%d files=%v", total, files)
	}

	// Folders themselves are searchable by name.
	files, total, err = d.GetByPfID(ctx, db, "t1", "root", 1, 15, "create_time", true, "sub", false)
	if err != nil {
		t.Fatalf("GetByPfID failed: %v", err)
	}
	if total != 1 || len(files) != 1 || files[0].ID != "subB" {
		t.Fatalf("expected folder subB, got total=%d files=%v", total, files)
	}
}

func TestFileDAO_GetByPfID_KeywordsScopedToSubtree(t *testing.T) {
	db := setupFileTestDB(t)
	seedFileTree(t, db)
	d := NewFileDAO()
	ctx := t.Context()

	// Searching inside dirA must not match files outside that subtree.
	files, total, err := d.GetByPfID(ctx, db, "t1", "dirA", 1, 15, "create_time", true, "report", false)
	if err != nil {
		t.Fatalf("GetByPfID failed: %v", err)
	}
	if total != 0 || len(files) != 0 {
		t.Fatalf("expected no results outside subtree, got total=%d files=%v", total, files)
	}

	// Tenant isolation still applies.
	files, total, err = d.GetByPfID(ctx, db, "t2", "root", 1, 15, "create_time", true, "report", false)
	if err != nil {
		t.Fatalf("GetByPfID failed: %v", err)
	}
	if total != 1 || len(files) != 1 || files[0].ID != "f-t2" {
		t.Fatalf("expected only tenant t2 file, got total=%d files=%v", total, files)
	}
}

func TestFileDAO_GetByPfID_NoKeywordsListsDirectChildren(t *testing.T) {
	db := setupFileTestDB(t)
	seedFileTree(t, db)
	d := NewFileDAO()
	ctx := t.Context()

	files, total, err := d.GetByPfID(ctx, db, "t1", "root", 1, 15, "create_time", true, "", false)
	if err != nil {
		t.Fatalf("GetByPfID failed: %v", err)
	}
	if total != 2 || len(files) != 2 {
		t.Fatalf("expected 2 direct children, got total=%d files=%v", total, files)
	}
	for _, f := range files {
		if f.ParentID != "root" || f.ID == "root" {
			t.Fatalf("unexpected entry in direct listing: %+v", f)
		}
	}
}
