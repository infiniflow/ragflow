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

// setupKBTestDB initializes an in-memory SQLite database for KB DAO tests.
func setupKBTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	// Migrate tables needed by KB access checks.
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func testKnowledgebase(t *testing.T, db *gorm.DB, id string, docNum, tokenNum, chunkNum int64) *entity.Knowledgebase {
	t.Helper()
	kb := &entity.Knowledgebase{
		ID:         id,
		TenantID:   "tenant-1",
		Name:       "test-kb-" + id,
		EmbdID:     "embd-1",
		CreatedBy:  "tenant-1",
		Permission: string(entity.TenantPermissionMe),
		DocNum:     docNum,
		TokenNum:   tokenNum,
		ChunkNum:   chunkNum,
		Status:     stringPtr(string(entity.StatusValid)),
	}
	if err := db.Create(kb).Error; err != nil {
		t.Fatalf("failed to create test kb: %v", err)
	}
	return kb
}

func testUserTenant(t *testing.T, db *gorm.DB, id, userID, tenantID string) {
	t.Helper()
	status := string(entity.StatusValid)
	userTenant := &entity.UserTenant{
		ID:        id,
		UserID:    userID,
		TenantID:  tenantID,
		Role:      "normal",
		InvitedBy: tenantID,
		Status:    &status,
	}
	if err := db.Create(userTenant).Error; err != nil {
		t.Fatalf("failed to create test user_tenant: %v", err)
	}
}

func stringPtr(s string) *string {
	return &s
}

func TestKnowledgebaseDAO_DecreaseDocumentNum(t *testing.T) {
	db := setupKBTestDB(t)
	pushDB(t, db)
	dao := NewKnowledgebaseDAO()

	testKnowledgebase(t, db, "kb-1", 5, 100, 50)

	err := dao.DecreaseDocumentNum("kb-1", 1, 20, 10)
	if err != nil {
		t.Fatalf("DecreaseDocumentNum failed: %v", err)
	}

	kb, err := dao.GetByID("kb-1")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if kb.DocNum != 4 {
		t.Fatalf("doc_num: expected 4, got %d", kb.DocNum)
	}
	if kb.TokenNum != 90 {
		t.Fatalf("token_num: expected 90, got %d", kb.TokenNum)
	}
	if kb.ChunkNum != 30 {
		t.Fatalf("chunk_num: expected 30, got %d", kb.ChunkNum)
	}
}

func TestKnowledgebaseDAO_DecreaseDocumentNum_ZeroDecrement(t *testing.T) {
	db := setupKBTestDB(t)
	pushDB(t, db)
	dao := NewKnowledgebaseDAO()

	testKnowledgebase(t, db, "kb-2", 3, 60, 15)

	err := dao.DecreaseDocumentNum("kb-2", 0, 0, 0)
	if err != nil {
		t.Fatalf("DecreaseDocumentNum failed: %v", err)
	}

	kb, err := dao.GetByID("kb-2")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if kb.DocNum != 3 {
		t.Fatalf("doc_num should be unchanged: expected 3, got %d", kb.DocNum)
	}
	if kb.TokenNum != 60 {
		t.Fatalf("token_num should be unchanged: expected 60, got %d", kb.TokenNum)
	}
	if kb.ChunkNum != 15 {
		t.Fatalf("chunk_num should be unchanged: expected 15, got %d", kb.ChunkNum)
	}
}

func TestKnowledgebaseDAO_AccessibleAllowsOwnerForPrivateKnowledgebase(t *testing.T) {
	db := setupKBTestDB(t)
	pushDB(t, db)
	dao := NewKnowledgebaseDAO()

	testKnowledgebase(t, db, "kb-private", 0, 0, 0)

	if !dao.Accessible("kb-private", "tenant-1") {
		t.Fatal("owner should access a private knowledgebase")
	}
}

func TestKnowledgebaseDAO_AccessibleRejectsTenantMemberForPrivateKnowledgebase(t *testing.T) {
	db := setupKBTestDB(t)
	pushDB(t, db)
	dao := NewKnowledgebaseDAO()

	testKnowledgebase(t, db, "kb-private", 0, 0, 0)
	testUserTenant(t, db, "ut-1", "member-1", "tenant-1")

	if dao.Accessible("kb-private", "member-1") {
		t.Fatal("tenant member should not access a private knowledgebase")
	}
}

func TestKnowledgebaseDAO_AccessibleRejectsTenantIDMatchWhenNotCreatorForPrivateKnowledgebase(t *testing.T) {
	db := setupKBTestDB(t)
	pushDB(t, db)
	dao := NewKnowledgebaseDAO()

	kb := testKnowledgebase(t, db, "kb-private-tenant-match", 0, 0, 0)
	kb.CreatedBy = "member-creator"
	kb.Permission = string(entity.TenantPermissionMe)
	if err := db.Save(kb).Error; err != nil {
		t.Fatalf("failed to update test kb creator: %v", err)
	}

	if dao.Accessible("kb-private-tenant-match", "tenant-1") {
		t.Fatal("non-creator should not access private knowledgebase even when tenant_id matches requester")
	}
}

func TestKnowledgebaseDAO_AccessibleAllowsTenantMemberForTeamKnowledgebase(t *testing.T) {
	db := setupKBTestDB(t)
	pushDB(t, db)
	dao := NewKnowledgebaseDAO()

	kb := testKnowledgebase(t, db, "kb-team", 0, 0, 0)
	kb.Permission = string(entity.TenantPermissionTeam)
	if err := db.Save(kb).Error; err != nil {
		t.Fatalf("failed to update test kb permission: %v", err)
	}
	testUserTenant(t, db, "ut-1", "member-1", "tenant-1")

	if !dao.Accessible("kb-team", "member-1") {
		t.Fatal("tenant member should access a team knowledgebase")
	}
}

func TestKnowledgebaseDAO_AccessibleRejectsRevokedTenantMemberForTeamKnowledgebase(t *testing.T) {
	db := setupKBTestDB(t)
	pushDB(t, db)
	dao := NewKnowledgebaseDAO()

	kb := testKnowledgebase(t, db, "kb-team", 0, 0, 0)
	kb.Permission = string(entity.TenantPermissionTeam)
	if err := db.Save(kb).Error; err != nil {
		t.Fatalf("failed to update test kb permission: %v", err)
	}

	invalidStatus := string(entity.StatusInvalid)
	revokedMembership := &entity.UserTenant{
		ID:        "ut-revoked",
		UserID:    "revoked-member",
		TenantID:  "tenant-1",
		Role:      "normal",
		InvitedBy: "tenant-1",
		Status:    &invalidStatus,
	}
	if err := db.Create(revokedMembership).Error; err != nil {
		t.Fatalf("failed to create revoked user_tenant: %v", err)
	}

	if dao.Accessible("kb-team", "revoked-member") {
		t.Fatal("revoked tenant member should not access a team knowledgebase")
	}
}

func TestKnowledgebaseDAO_AccessibleRejectsUnrelatedUserForTeamKnowledgebase(t *testing.T) {
	db := setupKBTestDB(t)
	pushDB(t, db)
	dao := NewKnowledgebaseDAO()

	kb := testKnowledgebase(t, db, "kb-team", 0, 0, 0)
	kb.Permission = string(entity.TenantPermissionTeam)
	if err := db.Save(kb).Error; err != nil {
		t.Fatalf("failed to update test kb permission: %v", err)
	}

	if dao.Accessible("kb-team", "stranger-1") {
		t.Fatal("unrelated user should not access a team knowledgebase")
	}
}
