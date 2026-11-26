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

package file

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/permission"

	"gorm.io/gorm"
)

var errForceFileCommitRollback = errors.New("force rollback")

// seedCommitWithTree seeds a parent commit whose tree contains f1 so a
// delete-only change can be recorded without a live file row.
func seedCommitWithTree(t *testing.T, db *gorm.DB, id, folderID string) {
	t.Helper()
	tree := `{"f1":{"hash":"h1","location":".objects/h1","name":"a.txt","size":3,"status":"1","parent_id":"` + folderID + `"}}`
	parent := &entity.FileCommit{ID: id, FolderID: folderID, Message: "init", AuthorID: "u1", FileCount: 1, TreeState: &tree}
	if err := db.Create(parent).Error; err != nil {
		t.Fatalf("seed parent commit: %v", err)
	}
}

func TestCreateCommitUsesCallerDB(t *testing.T) {
	db := newCommitDeleteTestDB(t)
	seedFolder(t, db, "folder-1", "")
	seedCommitWithTree(t, db, "c1", "folder-1")
	svc := NewFileCommitService()

	err := db.Transaction(func(tx *gorm.DB) error {
		_, e := svc.CreateCommit(permission.WithIdentity(t.Context(), "u1", "t1"), tx, "folder-1", "u1", "remove a.txt", []entity.FileChange{
			{FileID: "f1", FileName: "a.txt", Operation: "delete"},
		})
		if e != nil {
			return e
		}
		return errForceFileCommitRollback
	})
	if !errors.Is(err, errForceFileCommitRollback) {
		t.Fatalf("expected forced rollback, got %v", err)
	}
	var count int64
	if e := db.Model(&entity.FileCommit{}).Where("folder_id = ?", "folder-1").Count(&count).Error; e != nil || count != 1 {
		t.Fatalf("commit created outside caller transaction: count=%d (want only seeded parent), err=%v", count, e)
	}
}

func TestCreateCommitReadsParentThroughCallerDB(t *testing.T) {
	db := newCommitDeleteTestDB(t)
	seedFolder(t, db, "folder-1", "")
	svc := NewFileCommitService()

	var newCommit *entity.FileCommit
	err := db.Transaction(func(tx *gorm.DB) error {
		// Seed the parent inside the caller's transaction but do not commit it.
		seedCommitWithTree(t, tx, "uncommitted-parent", "folder-1")
		created, e := svc.CreateCommit(permission.WithIdentity(t.Context(), "u1", "t1"), tx, "folder-1", "u1", "remove a.txt", []entity.FileChange{
			{FileID: "f1", FileName: "a.txt", Operation: "delete"},
		})
		if e != nil {
			return e
		}
		newCommit = created
		return nil
	})
	if err != nil {
		t.Fatalf("transaction: %v", err)
	}
	if newCommit.ParentID == nil || *newCommit.ParentID != "uncommitted-parent" {
		t.Fatalf("parent resolved outside caller transaction: got %v, want uncommitted-parent", newCommit.ParentID)
	}
}

func TestRecordPageEditUsesCallerDB(t *testing.T) {
	db := newPageCommitTestDB(t)
	svc := NewFileCommitService()

	err := WithPageCommitLock(t.Context(), "dataset-1", "doc", "page-1", func(locked context.Context) error {
		return db.Transaction(func(tx *gorm.DB) error {
			_, e := svc.RecordPageEdit(locked, tx, PageEditCommitInput{
				DatasetID:  "dataset-1",
				Slug:       "page-1",
				PageType:   "doc",
				Title:      "Page",
				AuthorID:   "u1",
				OldContent: "old",
				NewContent: "new",
			})
			if e != nil {
				return e
			}
			return errForceFileCommitRollback
		})
	})
	if !errors.Is(err, errForceFileCommitRollback) {
		t.Fatalf("expected forced rollback, got %v", err)
	}
	var count int64
	if e := db.Model(&entity.FileCommit{}).Where("folder_id = ?", "dataset-1").Count(&count).Error; e != nil || count != 0 {
		t.Fatalf("page commit created outside caller transaction: count=%d, err=%v", count, e)
	}
}

// TestRecordPageEditReadsParentThroughCallerDB pins that the parent lookup
// follows the caller's handle: a parent created inside the caller's transaction
// but not yet committed must be visible to the next edit on the same page.
func TestRecordPageEditReadsParentThroughCallerDB(t *testing.T) {
	db := newPageCommitTestDB(t)
	svc := NewFileCommitService()

	var second *entity.FileCommit
	err := WithPageCommitLock(t.Context(), "dataset-1", "doc", "page-1", func(locked context.Context) error {
		return db.Transaction(func(tx *gorm.DB) error {
			_, e := svc.RecordPageEdit(locked, tx, PageEditCommitInput{
				DatasetID: "dataset-1", Slug: "page-1", PageType: "doc", Title: "First", AuthorID: "u1",
				OldContent: "", NewContent: "v1",
			})
			if e != nil {
				return e
			}
			second, e = svc.RecordPageEdit(locked, tx, PageEditCommitInput{
				DatasetID: "dataset-1", Slug: "page-1", PageType: "doc", Title: "Second", AuthorID: "u1",
				OldContent: "v1", NewContent: "v2",
			})
			return e
		})
	})
	if err != nil {
		t.Fatalf("transaction: %v", err)
	}
	if second.ParentID == nil || *second.ParentID == "" {
		t.Fatal("second page edit must chain to the uncommitted first edit via the caller's db")
	}
	var firstID string
	if e := db.Model(&entity.FileCommit{}).Where("folder_id = ? AND title = ?", "dataset-1", "First").Pluck("id", &firstID).Error; e != nil {
		t.Fatal(e)
	}
	if firstID == "" || *second.ParentID != firstID {
		t.Fatalf("parent = %v, want first commit %s", second.ParentID, firstID)
	}
}

// TestWithPageCommitLockHoldsLockThroughFn pins the core P1 fix: while the
// wrapper's fn runs (which is where the caller commits its transaction), the
// per-page mutex must still be held. TryLock reports whether the lock is free;
// it must fail inside fn, proving a concurrent edit cannot resolve the parent
// from committed data before this transaction's parent write is committed.
func TestWithPageCommitLockHoldsLockThroughFn(t *testing.T) {
	fileID := wikiFileID("dataset-1", "doc", "page-1")
	mu := pageCommitLock(fileID)
	lockFreeInsideFn := true
	err := WithPageCommitLock(t.Context(), "dataset-1", "doc", "page-1", func(context.Context) error {
		lockFreeInsideFn = mu.TryLock() // false => wrapper holds it (correct)
		if lockFreeInsideFn {
			mu.Unlock() // avoid leaving a stray lock if the invariant breaks
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithPageCommitLock: %v", err)
	}
	if lockFreeInsideFn {
		t.Fatal("page lock was free inside WithPageCommitLock fn — a caller tx could commit after the lock was released")
	}
}

// TestRecordPageEditRejectsUnwrappedTx pins the fail-fast contract: passing a
// caller-owned transaction handle without holding the page lock across the
// transaction is rejected, because the lock would otherwise be released before
// the caller commits and a concurrent edit could fork the parent chain.
func TestRecordPageEditRejectsUnwrappedTx(t *testing.T) {
	db := newPageCommitTestDB(t)
	svc := NewFileCommitService()

	err := db.Transaction(func(tx *gorm.DB) error {
		_, e := svc.RecordPageEdit(t.Context(), tx, PageEditCommitInput{
			DatasetID: "dataset-1", Slug: "page-1", PageType: "doc", Title: "Page", AuthorID: "u1",
			OldContent: "old", NewContent: "new",
		})
		return e
	})
	if err == nil || !strings.Contains(err.Error(), "WithPageCommitLock") {
		t.Fatalf("expected unwrapped tx to be rejected with WithPageCommitLock guidance, got %v", err)
	}
}

// TestFileCommitMethodsAcceptNilDB pins the fallback: a nil db uses dao.DB so
// existing callers that have no handle can still call the methods.
func TestFileCommitMethodsAcceptNilDB(t *testing.T) {
	db := newCommitDeleteTestDB(t)
	seedFolder(t, db, "folder-1", "")
	seedCommitWithTree(t, db, "c1", "folder-1")
	svc := NewFileCommitService()

	commit, err := svc.CreateCommit(permission.WithIdentity(t.Context(), "u1", "t1"), nil, "folder-1", "u1", "remove a.txt", []entity.FileChange{
		{FileID: "f1", FileName: "a.txt", Operation: "delete"},
	})
	if err != nil {
		t.Fatalf("CreateCommit with nil db: %v", err)
	}
	if commit == nil || commit.ID == "" {
		t.Fatal("CreateCommit with nil db returned no commit")
	}
}
