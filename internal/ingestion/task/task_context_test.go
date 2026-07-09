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

package task

import (
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.Task{},
		&entity.Document{},
		&entity.Knowledgebase{},
		&entity.Tenant{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func pushDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })
}

func ptr(s string) *string { return &s }

func TestLoadTaskContext(t *testing.T) {
	db := setupTestDB(t)
	pushDB(t, db)

	// Prepare test data in FK dependency order: tenant → kb → document → task
	tenant := &entity.Tenant{
		ID:        "tenant-1",
		LLMID:     "gpt-4",
		ASRID:     "whisper-1",
		Img2TxtID: "gpt-4-vision",
		Status:    ptr("1"),
	}
	kb := &entity.Knowledgebase{
		ID:       "kb-1",
		TenantID: "tenant-1",
		Language: ptr("Chinese"),
		EmbdID:   "embd-model-1",
		Pagerank: 0,
		Status:   ptr(string(entity.StatusValid)),
		ParserConfig: entity.JSONMap{
			"chunk_token_size": float64(512),
		},
	}
	doc := &entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{"pages": []any{[]any{float64(1), float64(10)}}},
		Name:         ptr("test.pdf"),
		Type:         "pdf",
		Location:     ptr("bucket/test.pdf"),
		Size:         1024,
	}
	task := &entity.Task{
		ID:       "task-1",
		DocID:    "doc-1",
		FromPage: 0,
		ToPage:   100000,
	}

	db.Create(tenant)
	db.Create(kb)
	db.Create(doc)
	db.Create(task)

	got, err := LoadTaskContext("task-1")
	if err != nil {
		t.Fatalf("LoadTaskContext: %v", err)
	}

	// === task table fields (no longer stored in TaskContext) ===
	// Task field removed from TaskContext, skipping tests

	// === document table fields ===
	if got.Doc.ID != "doc-1" {
		t.Errorf("Doc.ID = %v, want doc-1", got.Doc.ID)
	}
	if got.Doc.ParserID != "naive" {
		t.Errorf("Doc.ParserID = %v, want naive", got.Doc.ParserID)
	}
	if got.Doc.Name == nil || *got.Doc.Name != "test.pdf" {
		t.Errorf("Doc.Name = %v, want test.pdf", got.Doc.Name)
	}
	if got.Doc.Type != "pdf" {
		t.Errorf("Doc.Type = %v, want pdf", got.Doc.Type)
	}
	if got.Doc.Location == nil || *got.Doc.Location != "bucket/test.pdf" {
		t.Errorf("Doc.Location = %v, want bucket/test.pdf", got.Doc.Location)
	}
	if got.Doc.Size != 1024 {
		t.Errorf("Doc.Size = %v, want 1024", got.Doc.Size)
	}
	if got.Doc.ParserConfig == nil {
		t.Error("Doc.ParserConfig is nil")
	} else if got.Doc.ParserConfig["pages"] == nil {
		t.Error("Doc.ParserConfig.pages is nil")
	}

	// === knowledgebase table fields ===
	if got.KB.ID != "kb-1" {
		t.Errorf("KB.ID = %v, want kb-1", got.KB.ID)
	}
	if got.KB.TenantID != "tenant-1" {
		t.Errorf("KB.TenantID = %v, want tenant-1", got.KB.TenantID)
	}
	if got.KB.Language == nil || *got.KB.Language != "Chinese" {
		t.Errorf("KB.Language = %v, want Chinese", got.KB.Language)
	}
	if got.KB.EmbdID != "embd-model-1" {
		t.Errorf("KB.EmbdID = %v, want embd-model-1", got.KB.EmbdID)
	}
	if got.KB.Pagerank != 0 {
		t.Errorf("KB.Pagerank = %v, want 0", got.KB.Pagerank)
	}
	if got.KB.ParserConfig == nil {
		t.Error("KB.ParserConfig is nil")
	} else if got.KB.ParserConfig["chunk_token_size"] != float64(512) {
		t.Errorf("KB.ParserConfig.chunk_token_size = %v, want 512",
			got.KB.ParserConfig["chunk_token_size"])
	}

	// === tenant table fields ===
	if got.Tenant.LLMID != "gpt-4" {
		t.Errorf("Tenant.LLMID = %v, want gpt-4", got.Tenant.LLMID)
	}
}

func TestLoadTaskContext_TaskNotFound(t *testing.T) {
	db := setupTestDB(t)
	pushDB(t, db)

	_, err := LoadTaskContext("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestLoadTaskContext_DocNotFound(t *testing.T) {
	db := setupTestDB(t)
	pushDB(t, db)

	db.Create(&entity.Task{
		ID:       "task-1",
		DocID:    "nonexistent-doc",
		FromPage: 0,
		ToPage:   100000,
	})

	_, err := LoadTaskContext("task-1")
	if err == nil {
		t.Fatal("expected error when document not found")
	}
}
