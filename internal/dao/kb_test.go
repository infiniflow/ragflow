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

	// Migrate knowledgebase table
	if err := db.AutoMigrate(&entity.Knowledgebase{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserTenant{}); err != nil {
		t.Fatalf("failed to migrate user_tenant: %v", err)
	}

	return db
}

func testKnowledgebase(t *testing.T, db *gorm.DB, id string, docNum, tokenNum, chunkNum int64) *entity.Knowledgebase {
	t.Helper()
	kb := &entity.Knowledgebase{
		ID:       id,
		TenantID: "tenant-1",
		Name:     "test-kb-" + id,
		EmbdID:   "embd-1",
		DocNum:   docNum,
		TokenNum: tokenNum,
		ChunkNum: chunkNum,
		Status:   stringPtr(string(entity.StatusValid)),
	}
	if err := db.Create(kb).Error; err != nil {
		t.Fatalf("failed to create test kb: %v", err)
	}
	return kb
}

func stringPtr(s string) *string {
	return &s
}

func TestKnowledgebaseDAO_DecreaseDocumentNum(t *testing.T) {
	db := setupKBTestDB(t)
	pushDB(t, db)
	dao := NewKnowledgebaseDAO()

	testKnowledgebase(t, db, "kb-1", 5, 100, 50)

	ctx := t.Context()
	err := dao.DecreaseDocumentNum(ctx, db, "kb-1", 1, 20, 10)
	if err != nil {
		t.Fatalf("DecreaseDocumentNum failed: %v", err)
	}

	kb, err := dao.GetByID(ctx, db, "kb-1")
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

	ctx := t.Context()

	testKnowledgebase(t, db, "kb-2", 3, 60, 15)

	err := dao.DecreaseDocumentNum(ctx, db, "kb-2", 0, 0, 0)
	if err != nil {
		t.Fatalf("DecreaseDocumentNum failed: %v", err)
	}

	kb, err := dao.GetByID(ctx, db, "kb-2")
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

// knowledgebaseQualifiedOrderExpressions are clause fragments rather than
// column names. ListByResourceIDs prefixes the value with "knowledgebase.", so
// these stand in for the `orderby` query parameter that reaches it from the
// dataset list request (issue #14268).
var knowledgebaseQualifiedOrderExpressions = []string{
	"name, (SELECT inner_kb.id FROM knowledgebase AS inner_kb WHERE inner_kb.id = knowledgebase.id)",
}

// seedOrderableKnowledgebases creates three datasets whose create_time order is
// the reverse of their name order, so a test can tell which column the ORDER BY
// clause actually used.
func seedOrderableKnowledgebases(t *testing.T, db *gorm.DB) {
	t.Helper()
	kbs := []entity.Knowledgebase{
		{ID: "kb-1", TenantID: "t1", Name: "zulu", EmbdID: "embd-1", CreatedBy: "t1", Permission: "me", Status: stringPtr(string(entity.StatusValid)), BaseModel: entity.BaseModel{CreateTime: int64Ptr(100)}},
		{ID: "kb-2", TenantID: "t1", Name: "mike", EmbdID: "embd-1", CreatedBy: "t1", Permission: "me", Status: stringPtr(string(entity.StatusValid)), BaseModel: entity.BaseModel{CreateTime: int64Ptr(200)}},
		{ID: "kb-3", TenantID: "t1", Name: "alpha", EmbdID: "embd-1", CreatedBy: "t1", Permission: "me", Status: stringPtr(string(entity.StatusValid)), BaseModel: entity.BaseModel{CreateTime: int64Ptr(300)}},
	}
	for i := range kbs {
		if err := db.Create(&kbs[i]).Error; err != nil {
			t.Fatalf("failed to create knowledgebase %s: %v", kbs[i].ID, err)
		}
	}
}

func assertKnowledgebaseIDOrder(t *testing.T, got, want []string, orderby string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("orderby %q returned %d rows, want %d", orderby, len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("orderby %q produced order %v, want %v: the expression reached the ORDER BY clause", orderby, got, want)
		}
	}
}

// TestKnowledgebaseDAOListByResourceIDsOrderByExpressionFallsBack checks that an
// `orderby` value that is not an allowed column name leaves the rows in the
// default create_time order.
func TestKnowledgebaseDAOListByResourceIDsOrderByExpressionFallsBack(t *testing.T) {
	db := setupKBTestDB(t)
	if err := db.AutoMigrate(&entity.User{}); err != nil {
		t.Fatalf("failed to migrate user: %v", err)
	}
	pushDB(t, db)
	seedOrderableKnowledgebases(t, db)
	ctx := t.Context()
	d := NewKnowledgebaseDAO()

	for _, orderby := range knowledgebaseQualifiedOrderExpressions {
		t.Run(orderby, func(t *testing.T) {
			rows, _, err := d.ListByResourceIDs(ctx, db, []string{"kb-1", "kb-2", "kb-3"}, nil, 1, 10, []OrderTerm{{Column: orderby}}, "", "", "")
			if err != nil {
				t.Fatalf("ListByResourceIDs with orderby %q: %v", orderby, err)
			}
			ids := make([]string, 0, len(rows))
			for _, row := range rows {
				ids = append(ids, row.ID)
			}
			assertKnowledgebaseIDOrder(t, ids, []string{"kb-1", "kb-2", "kb-3"}, orderby)
		})
	}
}

// TestKnowledgebaseDAOOrderByAllowedColumn checks that the list query honors a
// real column in both directions.
func TestKnowledgebaseDAOOrderByAllowedColumn(t *testing.T) {
	db := setupKBTestDB(t)
	if err := db.AutoMigrate(&entity.User{}); err != nil {
		t.Fatalf("failed to migrate user: %v", err)
	}
	pushDB(t, db)
	seedOrderableKnowledgebases(t, db)
	ctx := t.Context()
	d := NewKnowledgebaseDAO()

	rows, _, err := d.ListByResourceIDs(ctx, db, []string{"kb-1", "kb-2", "kb-3"}, nil, 1, 10, []OrderTerm{{Column: "name"}}, "", "", "")
	if err != nil {
		t.Fatalf("ListByResourceIDs ascending by name: %v", err)
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	assertKnowledgebaseIDOrder(t, ids, []string{"kb-3", "kb-2", "kb-1"}, "name")

}
