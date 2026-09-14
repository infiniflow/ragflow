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

// fileOrderExpressions are SQL expressions rather than column names. They stand
// in for the `orderby` query parameter of GET /api/v1/files, which reaches
// FileDAO straight from the request (issue #14268).
var fileOrderExpressions = []string{
	"(SELECT inner_file.name FROM file AS inner_file WHERE inner_file.id = file.id)",
}

// seedOrderableFiles creates a root folder and three children whose create_time
// order is the reverse of their name order, so a test can tell which column the
// ORDER BY clause actually used.
func seedOrderableFiles(t *testing.T, db *gorm.DB) {
	t.Helper()
	testFile(t, db, "root", "root", "t1", "/", "folder")
	files := []entity.File{
		{ID: "f-1", ParentID: "root", TenantID: "t1", CreatedBy: "t1", Name: "zulu.txt", Type: "doc", BaseModel: entity.BaseModel{CreateTime: int64Ptr(100)}},
		{ID: "f-2", ParentID: "root", TenantID: "t1", CreatedBy: "t1", Name: "mike.txt", Type: "doc", BaseModel: entity.BaseModel{CreateTime: int64Ptr(200)}},
		{ID: "f-3", ParentID: "root", TenantID: "t1", CreatedBy: "t1", Name: "alpha.txt", Type: "doc", BaseModel: entity.BaseModel{CreateTime: int64Ptr(300)}},
	}
	for i := range files {
		if err := db.Create(&files[i]).Error; err != nil {
			t.Fatalf("failed to create file %s: %v", files[i].ID, err)
		}
	}
}

func assertFileOrder(t *testing.T, rows []*entity.File, want []string, orderBy string) {
	t.Helper()
	if len(rows) != len(want) {
		t.Fatalf("orderby %q returned %d rows, want %d", orderBy, len(rows), len(want))
	}
	for i, row := range rows {
		if row.ID != want[i] {
			got := make([]string, 0, len(rows))
			for _, r := range rows {
				got = append(got, r.ID)
			}
			t.Fatalf("orderby %q produced order %v, want %v: the expression reached the ORDER BY clause", orderBy, got, want)
		}
	}
}

// TestFileDAO_GetByPfID_OrderByExpressionFallsBack checks that an `orderby`
// value that is an expression rather than an allowed column name leaves the
// rows in the default create_time order.
func TestFileDAO_GetByPfID_OrderByExpressionFallsBack(t *testing.T) {
	db := setupFileTestDB(t)
	seedOrderableFiles(t, db)
	ctx := t.Context()
	d := NewFileDAO()

	for _, orderBy := range fileOrderExpressions {
		t.Run(orderBy, func(t *testing.T) {
			rows, _, err := d.GetByPfID(ctx, db, "t1", "root", 1, 15, orderBy, false, "", false)
			if err != nil {
				t.Fatalf("GetByPfID with orderby %q: %v", orderBy, err)
			}
			assertFileOrder(t, rows, []string{"f-1", "f-2", "f-3"}, orderBy)
		})
	}
}

// TestFileDAO_GetByPfID_OrderByAllowedColumn checks that the allowlist still
// honors a real column in both directions. The skill folder listing already
// calls GetByPfID with "name".
func TestFileDAO_GetByPfID_OrderByAllowedColumn(t *testing.T) {
	db := setupFileTestDB(t)
	seedOrderableFiles(t, db)
	ctx := t.Context()
	d := NewFileDAO()

	rows, _, err := d.GetByPfID(ctx, db, "t1", "root", 1, 15, "name", false, "", false)
	if err != nil {
		t.Fatalf("GetByPfID ascending by name: %v", err)
	}
	assertFileOrder(t, rows, []string{"f-3", "f-2", "f-1"}, "name")

	rows, _, err = d.GetByPfID(ctx, db, "t1", "root", 1, 15, "name", true, "", false)
	if err != nil {
		t.Fatalf("GetByPfID descending by name: %v", err)
	}
	assertFileOrder(t, rows, []string{"f-1", "f-2", "f-3"}, "name")
}
