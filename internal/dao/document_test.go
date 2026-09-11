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
	"encoding/json"
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
		&entity.Knowledgebase{},
		&entity.Tenant{},
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

	db.Create(&entity.Document{ID: "doc1", KbID: "kb1", Name: sp("Doc 1"), CreatedBy: "user1", ParserConfig: entity.JSONMap{}})
	db.Create(&entity.Document{ID: "doc2", KbID: "kb1", Name: sp("Doc 2"), CreatedBy: "user1", ParserConfig: entity.JSONMap{}})
	db.Create(&entity.Document{ID: "doc3", KbID: "kb2", Name: sp("Doc 3"), CreatedBy: "user2", ParserConfig: entity.JSONMap{}})

	ctx := t.Context()
	dao := NewDocumentDAO()
	docs, err := dao.GetByIDs(ctx, db, []string{"doc1", "doc3"})
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
	ctx := t.Context()
	dao := NewDocumentDAO()
	docs, err := dao.GetByIDs(ctx, db, []string{})
	if err != nil {
		t.Fatalf("GetByIDs failed: %v", err)
	}
	if docs != nil {
		t.Errorf("expected nil for empty IDs, got %v", docs)
	}
}

func TestDocumentGetByIDs_NilIDs(t *testing.T) {
	db := setupDocumentTestDB(t)
	ctx := t.Context()
	dao := NewDocumentDAO()
	docs, err := dao.GetByIDs(ctx, db, nil)
	if err != nil {
		t.Fatalf("GetByIDs failed: %v", err)
	}
	if docs != nil {
		t.Errorf("expected nil for nil IDs, got %v", docs)
	}
}

func TestDocumentGetByIDs_NoMatch(t *testing.T) {
	db := setupDocumentTestDB(t)

	db.Create(&entity.Document{ID: "doc1", KbID: "kb1", Name: sp("Doc 1"), CreatedBy: "user1", ParserConfig: entity.JSONMap{}})
	ctx := t.Context()
	dao := NewDocumentDAO()
	docs, err := dao.GetByIDs(ctx, db, []string{"nonexistent"})
	if err != nil {
		t.Fatalf("GetByIDs failed: %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("expected 0 docs, got %d", len(docs))
	}
}

func TestDocumentGetByKBIDOrdersByCreateTime(t *testing.T) {
	db := setupDocumentTestDB(t)

	createTime10 := int64(10)
	createTime20 := int64(20)
	createTime30 := int64(30)
	db.Create(&entity.Document{ID: "doc-later", KbID: "kb1", Name: sp("Doc Later"), CreatedBy: "user1", ParserConfig: entity.JSONMap{}, BaseModel: entity.BaseModel{CreateTime: &createTime30}})
	db.Create(&entity.Document{ID: "doc-other", KbID: "kb2", Name: sp("Doc Other"), CreatedBy: "user1", ParserConfig: entity.JSONMap{}, BaseModel: entity.BaseModel{CreateTime: &createTime10}})
	db.Create(&entity.Document{ID: "doc-earlier", KbID: "kb1", Name: sp("Doc Earlier"), CreatedBy: "user1", ParserConfig: entity.JSONMap{}, BaseModel: entity.BaseModel{CreateTime: &createTime20}})
	ctx := t.Context()
	dao := NewDocumentDAO()
	docs, total, err := dao.GetByKBID(ctx, db, "kb1")
	if err != nil {
		t.Fatalf("fail to get document by dataset id: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total=2, got %d", total)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(docs))
	}
	if docs[0].ID != "doc-earlier" || docs[1].ID != "doc-later" {
		t.Fatalf("unexpected order: %s, %s", docs[0].ID, docs[1].ID)
	}
}

func TestDocumentListIncludesScheduledIngestionStatus(t *testing.T) {
	db := setupDocumentTestDB(t)
	if err := db.AutoMigrate(
		&entity.User{},
		&entity.UserCanvas{},
		&entity.File{},
		&entity.File2Document{},
		&entity.IngestionTask{},
	); err != nil {
		t.Fatalf("migrate document-list dependencies: %v", err)
	}
	if err := db.Create(&entity.Document{
		ID:           "doc-scheduled",
		KbID:         "kb-1",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		SourceType:   "local",
		Type:         "document",
		CreatedBy:    "user-1",
		Name:         sp("scheduled.pdf"),
		Suffix:       "pdf",
	}).Error; err != nil {
		t.Fatalf("create document: %v", err)
	}
	if err := db.Create(&entity.File{
		ID:        "file-scheduled",
		ParentID:  "parent-1",
		TenantID:  "tenant-1",
		CreatedBy: "user-1",
		Name:      "scheduled.pdf",
		Type:      "document",
	}).Error; err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := db.Create(&entity.File2Document{
		ID:         "link-scheduled",
		FileID:     sp("file-scheduled"),
		DocumentID: sp("doc-scheduled"),
	}).Error; err != nil {
		t.Fatalf("link file to document: %v", err)
	}
	if err := db.Create(&entity.IngestionTask{
		ID:         "task-scheduled",
		UserID:     "user-1",
		DocumentID: "doc-scheduled",
		DatasetID:  "kb-1",
		Status:     "SCHEDULED",
	}).Error; err != nil {
		t.Fatalf("create scheduled ingestion task: %v", err)
	}

	documents, total, err := NewDocumentDAO().ListByKBIDWithOptions(t.Context(), db, DocumentListOptions{
		KbID:    "kb-1",
		OrderBy: "create_time",
		Desc:    true,
		Offset:  0,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if total != 1 || len(documents) != 1 {
		t.Fatalf("listed %d documents (total %d), want 1", len(documents), total)
	}

	raw, err := json.Marshal(documents[0])
	if err != nil {
		t.Fatalf("marshal document list item: %v", err)
	}
	var listed map[string]interface{}
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatalf("unmarshal document list item: %v", err)
	}
	if got := listed["ingestion_status"]; got != "SCHEDULED" {
		t.Fatalf("ingestion_status = %v, want %q", got, "SCHEDULED")
	}
}

func TestDocumentListDeduplicatesHistoricalIngestionTasks(t *testing.T) {
	db := setupDocumentTestDB(t)
	if err := db.AutoMigrate(
		&entity.User{},
		&entity.UserCanvas{},
		&entity.File{},
		&entity.File2Document{},
		&entity.IngestionTask{},
	); err != nil {
		t.Fatalf("migrate document-list dependencies: %v", err)
	}
	if err := db.Exec("DROP INDEX idx_ingestion_task_document_id").Error; err != nil {
		t.Fatalf("drop ingestion task unique index: %v", err)
	}

	if err := db.Create(&entity.Document{
		ID:           "doc-duplicate-tasks",
		KbID:         "kb-1",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		SourceType:   "local",
		Type:         "document",
		CreatedBy:    "user-1",
		Name:         sp("duplicate-tasks.pdf"),
		Suffix:       "pdf",
	}).Error; err != nil {
		t.Fatalf("create document: %v", err)
	}
	if err := db.Create(&entity.File{
		ID:        "file-duplicate-tasks",
		ParentID:  "parent-1",
		TenantID:  "tenant-1",
		CreatedBy: "user-1",
		Name:      "duplicate-tasks.pdf",
		Type:      "document",
	}).Error; err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := db.Create(&entity.File2Document{
		ID:         "link-duplicate-tasks",
		FileID:     sp("file-duplicate-tasks"),
		DocumentID: sp("doc-duplicate-tasks"),
	}).Error; err != nil {
		t.Fatalf("link file to document: %v", err)
	}

	oldTaskTime := int64(100)
	newTaskTime := int64(200)
	for _, task := range []*entity.IngestionTask{
		{
			ID:         "task-old",
			UserID:     "user-1",
			DocumentID: "doc-duplicate-tasks",
			DatasetID:  "kb-1",
			Status:     "FAILED",
			BaseModel:  entity.BaseModel{CreateTime: &oldTaskTime},
		},
		{
			ID:         "task-new",
			UserID:     "user-1",
			DocumentID: "doc-duplicate-tasks",
			DatasetID:  "kb-1",
			Status:     "SCHEDULED",
			BaseModel:  entity.BaseModel{CreateTime: &newTaskTime},
		},
	} {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("create ingestion task %s: %v", task.ID, err)
		}
	}

	documents, total, err := NewDocumentDAO().ListByKBIDWithOptions(t.Context(), db, DocumentListOptions{
		KbID:    "kb-1",
		OrderBy: "create_time",
		Desc:    true,
		Offset:  0,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if total != 1 || len(documents) != 1 {
		t.Fatalf("listed %d documents (total %d), want 1", len(documents), total)
	}
	if documents[0].IngestionStatus == nil || *documents[0].IngestionStatus != "SCHEDULED" {
		t.Fatalf("ingestion status = %v, want %q", documents[0].IngestionStatus, "SCHEDULED")
	}
}

func TestDocumentGetByDocumentIDAndDatasetIDUsesKBID(t *testing.T) {
	db := setupDocumentTestDB(t)

	db.Create(&entity.Document{ID: "doc1", KbID: "kb1", Name: sp("Doc 1"), CreatedBy: "user1", ParserConfig: entity.JSONMap{}})
	db.Create(&entity.Document{ID: "doc1-other", KbID: "kb2", Name: sp("Doc 2"), CreatedBy: "user1", ParserConfig: entity.JSONMap{}})
	ctx := t.Context()
	dao := NewDocumentDAO()
	doc, err := dao.GetByDocumentIDAndDatasetID(ctx, db, "doc1", "kb1")
	if err != nil {
		t.Fatalf("GetByDocumentIDAndDatasetID failed: %v", err)
	}
	if doc.ID != "doc1" || doc.KbID != "kb1" {
		t.Fatalf("unexpected document: id=%s kb_id=%s", doc.ID, doc.KbID)
	}

	if _, err = dao.GetByDocumentIDAndDatasetID(ctx, db, "doc1", "kb2"); err == nil {
		t.Fatal("expected no match when document does not belong to dataset")
	}
}

func TestDocumentGetChunkingConfigScansParserConfig(t *testing.T) {
	db := setupDocumentTestDB(t)

	if err := db.Create(&entity.Tenant{
		ID:        "tenant1",
		LLMID:     "llm1",
		EmbdID:    "embd1",
		ASRID:     "asr1",
		Img2TxtID: "img2txt1",
		RerankID:  "rerank1",
		ParserIDs: "naive",
	}).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := db.Create(&entity.Knowledgebase{
		ID:           "kb1",
		TenantID:     "tenant1",
		Name:         "Dataset 1",
		Language:     sp("English"),
		EmbdID:       "kb-embd1",
		Permission:   "me",
		CreatedBy:    "user1",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
	}).Error; err != nil {
		t.Fatalf("create knowledgebase: %v", err)
	}
	if err := db.Create(&entity.Document{
		ID:           "doc1",
		KbID:         "kb1",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{"chunk_token_num": float64(128), "delimiter": "\\n"},
		SourceType:   "local",
		Type:         "doc",
		CreatedBy:    "user1",
		Size:         42,
		Suffix:       ".txt",
	}).Error; err != nil {
		t.Fatalf("create document: %v", err)
	}
	ctx := t.Context()
	dao := NewDocumentDAO()
	config, err := dao.GetChunkingConfig(ctx, db, "doc1")
	if err != nil {
		t.Fatalf("GetChunkingConfig failed: %v", err)
	}
	parserConfig, ok := config["parser_config"].(entity.JSONMap)
	if !ok {
		t.Fatalf("parser_config type = %T, want entity.JSONMap", config["parser_config"])
	}
	if parserConfig["chunk_token_num"] != float64(128) || parserConfig["delimiter"] != "\\n" {
		t.Fatalf("unexpected parser_config: %#v", parserConfig)
	}
	if config["tenant_id"] != "tenant1" || config["embd_id"] != "kb-embd1" {
		t.Fatalf("unexpected joined config: %#v", config)
	}
}

func sp(s string) *string { return &s }
