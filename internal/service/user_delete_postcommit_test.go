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
	"errors"
	"slices"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

var errForceUserDeleteRollback = errors.New("force rollback")

func seedPostcommitUser(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec("INSERT INTO user (id, email, nickname, is_active, is_authenticated, is_anonymous) VALUES (?, ?, ?, ?, ?, ?)", "user-1", "user@example.com", "User", "0", "1", "0").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO knowledgebase (id, tenant_id, name, embd_id, permission, created_by, parser_id) VALUES (?, ?, ?, ?, ?, ?, ?)", "dataset", "team", "Dataset", "embd", "me", "user-1", "general").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO document (id, kb_id, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "doc", "dataset", "general", "{}", "pdf", "user-1", "pdf", "doc.pdf").Error; err != nil {
		t.Fatal(err)
	}
}

func TestDeleteUserTxDefersExternalCleanupUntilCommit(t *testing.T) {
	db := setupUserDeletionDB(t)
	seedPostcommitUser(t, db)
	store := &deletionStorage{}
	svc := UserDeletionService{engine: &deletionEngine{}, storage: store}
	user := &entity.User{ID: "user-1", Email: "user@example.com"}

	var snapshot *userDeletionData
	err := db.Transaction(func(tx *gorm.DB) error {
		// This row exists only inside the caller's transaction. If the read
		// phase used dao.DB instead of the caller's handle, it would be
		// invisible to the snapshot and the assertion below would fail.
		if e := tx.Exec("INSERT INTO document (id, kb_id, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "doc-tx", "dataset", "general", "{}", "pdf", "user-1", "pdf", "tx.pdf").Error; e != nil {
			return e
		}
		data, _, e := svc.DeleteTx(t.Context(), tx, user)
		if e != nil {
			return e
		}
		snapshot = data
		return errForceUserDeleteRollback
	})
	if !errors.Is(err, errForceUserDeleteRollback) {
		t.Fatalf("expected forced rollback, got %v", err)
	}
	if snapshot == nil {
		t.Fatal("DeleteTx must return a cleanup snapshot even when the caller rolls back")
	}
	// Rollback must restore every DB row the delete touched.
	for _, item := range [][2]string{{"user", "user-1"}, {"knowledgebase", "dataset"}, {"document", "doc"}} {
		var count int64
		if e := db.Table(item[0]).Where("id = ?", item[1]).Count(&count).Error; e != nil || count != 1 {
			t.Fatalf("rollback did not preserve %s/%s: count=%d, err=%v", item[0], item[1], count, e)
		}
	}
	// The row inserted inside the caller's transaction must be rolled back too.
	var txDocCount int64
	if e := db.Table("document").Where("id = ?", "doc-tx").Count(&txDocCount).Error; e != nil || txDocCount != 0 {
		t.Fatalf("uncommitted document survived rollback: count=%d, err=%v", txDocCount, e)
	}
	// The distinguishing assertion: no external cleanup happened inside DeleteTx.
	// If the implementation still called deleteExternalData eagerly, the snapshot
	// would already be emptied and the storage would show removals despite rollback.
	if len(store.files) != 0 || len(store.buckets) != 0 {
		t.Fatalf("external cleanup ran before commit: files=%v buckets=%v", store.files, store.buckets)
	}
	if len(snapshot.documentIDs) == 0 {
		t.Fatalf("snapshot must carry document IDs for later cleanup, got %+v", snapshot)
	}
	// The caller's uncommitted document must be in the snapshot. Reading through
	// dao.DB instead of the caller's handle would omit it and fail here.
	foundTxDoc := false
	for _, doc := range snapshot.documents {
		if doc.ID == "doc-tx" {
			foundTxDoc = true
			if doc.Location == nil || *doc.Location != "tx.pdf" {
				t.Fatalf("uncommitted document location missing from snapshot: %+v", doc)
			}
		}
	}
	if !foundTxDoc {
		t.Fatalf("snapshot missed the caller's uncommitted document: %+v", snapshot.documents)
	}
	hasLocation := false
	for _, doc := range snapshot.documents {
		if doc.Location != nil && *doc.Location != "" {
			hasLocation = true
			break
		}
	}
	if !hasLocation {
		t.Fatalf("snapshot must carry document locations for later cleanup, got %+v", snapshot)
	}
}

func TestDeleteUserTxThenCleanupUserExternal(t *testing.T) {
	db := setupUserDeletionDB(t)
	seedPostcommitUser(t, db)
	store := &deletionStorage{}
	svc := UserDeletionService{engine: &deletionEngine{}, storage: store}
	user := &entity.User{ID: "user-1", Email: "user@example.com"}

	snapshot, result, err := svc.DeleteTx(t.Context(), dao.DB, user)
	if err != nil {
		t.Fatalf("DeleteTx: %v", err)
	}
	if result == nil || result.UserCount != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	// dao.DB is standalone: DeleteTx committed on return. Now run external cleanup.
	warnings := svc.CleanupUserExternal(t.Context(), snapshot)
	if len(warnings) != 0 {
		t.Fatalf("unexpected cleanup warnings: %v", warnings)
	}
	if want := []string{"dataset/doc.pdf"}; !slices.Equal(store.files, want) {
		t.Fatalf("removed objects = %v, want %v", store.files, want)
	}
	if want := []string{"dataset"}; !slices.Equal(store.buckets, want) {
		t.Fatalf("removed buckets = %v, want %v", store.buckets, want)
	}
}

func TestCleanupUserExternalIsIdempotent(t *testing.T) {
	db := setupUserDeletionDB(t)
	seedPostcommitUser(t, db)
	store := &deletionStorage{}
	svc := UserDeletionService{engine: &deletionEngine{}, storage: store}
	user := &entity.User{ID: "user-1", Email: "user@example.com"}

	snapshot, _, err := svc.DeleteTx(t.Context(), dao.DB, user)
	if err != nil {
		t.Fatalf("DeleteTx: %v", err)
	}
	// Replaying cleanup against an already-cleaned snapshot must be safe: missing
	// objects are tolerated and no error/panic escapes.
	_ = svc.CleanupUserExternal(t.Context(), snapshot)
	second := svc.CleanupUserExternal(t.Context(), snapshot)
	if len(second) != 0 {
		t.Fatalf("replayed cleanup returned warnings: %v", second)
	}
}
