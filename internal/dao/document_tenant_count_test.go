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

package dao

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func documentCountTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal("open sqlite:", err)
	}
	if err := db.Exec("CREATE TABLE knowledgebase (id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL)").Error; err != nil {
		t.Fatal("create knowledgebase table:", err)
	}
	if err := db.Exec("CREATE TABLE document (id TEXT PRIMARY KEY, kb_id TEXT NOT NULL, created_by TEXT NOT NULL, user_id TEXT NOT NULL)").Error; err != nil {
		t.Fatal("create document table:", err)
	}
	if err := db.Exec("INSERT INTO knowledgebase (id, tenant_id) VALUES (?, ?), (?, ?)", "kb-1", "tenant-1", "kb-2", "tenant-2").Error; err != nil {
		t.Fatal("insert knowledgebases:", err)
	}
	return db
}

func TestDocumentDAOCountByTenantIDUsesKnowledgebaseTenant(t *testing.T) {
	db := documentCountTestDB(t)
	if err := db.Exec("INSERT INTO document (id, kb_id, created_by, user_id) VALUES (?, ?, ?, ?), (?, ?, ?, ?), (?, ?, ?, ?)",
		"doc-1", "kb-1", "user-1", "user-1",
		"doc-2", "kb-1", "user-2", "user-2",
		"doc-3", "kb-2", "user-3", "user-3",
	).Error; err != nil {
		t.Fatal("insert documents:", err)
	}

	count, err := NewDocumentDAO().CountByTenantID(t.Context(), db, "tenant-1")
	if err != nil {
		t.Fatalf("CountByTenantID: %v", err)
	}
	if count != 2 {
		t.Fatalf("CountByTenantID = %d, want 2 documents owned by tenant-1", count)
	}
}

func TestDocumentDAOCountByUserIDUsesUserIDWhenCreatedByIsTenant(t *testing.T) {
	db := documentCountTestDB(t)
	if err := db.Exec("INSERT INTO document (id, kb_id, created_by, user_id) VALUES (?, ?, ?, ?), (?, ?, ?, ?), (?, ?, ?, ?), (?, ?, ?, ?), (?, ?, ?, ?), (?, ?, ?, ?)",
		"doc-1", "kb-1", "tenant-1", "user-1",
		"doc-2", "kb-1", "user-2", "user-1",
		"doc-3", "kb-2", "user-1", "user-3",
		"doc-4", "kb-2", "user-4", "user-4",
		"doc-5", "kb-1", "tenant-1", "user-2",
		"doc-6", "kb-1", "tenant-1", "tenant-1",
	).Error; err != nil {
		t.Fatalf("insert documents: %v", err)
	}

	count, err := NewDocumentDAO().CountByUserID(t.Context(), db, "user-1")
	if err != nil {
		t.Fatalf("CountByUserID: %v", err)
	}
	if count != 2 {
		t.Fatalf("CountByUserID = %d, want 2 documents attributed to user-1", count)
	}

	tenantOwnerCount, err := NewDocumentDAO().CountByUserID(t.Context(), db, "tenant-1")
	if err != nil {
		t.Fatalf("CountByUserID for tenant owner: %v", err)
	}
	if tenantOwnerCount != 1 {
		t.Fatalf("CountByUserID for tenant owner = %d, want 1 owner document", tenantOwnerCount)
	}
}
