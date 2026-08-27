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
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
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

func TestLoadFromIngestionTask_FallsBackToKnowledgebasePipelineID(t *testing.T) {
	db := setupTestDB(t)
	pushDB(t, db)

	if err := db.Create(&entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		Name:         ptr("doc.pdf"),
		Status:       ptr("1"),
	}).Error; err != nil {
		t.Fatalf("create document: %v", err)
	}
	kbPipelineID := "kb-flow-1"
	if err := db.Create(&entity.Knowledgebase{
		ID:           "kb-1",
		TenantID:     "tenant-1",
		EmbdID:       "embd-1",
		PipelineID:   &kbPipelineID,
		ParserConfig: entity.JSONMap{},
		Status:       ptr("1"),
	}).Error; err != nil {
		t.Fatalf("create knowledgebase: %v", err)
	}
	if err := db.Create(&entity.Tenant{
		ID:     "tenant-1",
		Status: ptr("1"),
	}).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	ctx, err := LoadFromIngestionTask(&entity.IngestionTask{
		ID:         "task-1",
		DocumentID: "doc-1",
		DatasetID:  "kb-1",
	})
	if err != nil {
		t.Fatalf("LoadFromIngestionTask: %v", err)
	}
	if ctx.PipelineID != kbPipelineID {
		t.Fatalf("PipelineID = %q, want %q", ctx.PipelineID, kbPipelineID)
	}
}

func TestLoadFromIngestionTask_PrefersDocumentPipelineID(t *testing.T) {
	db := setupTestDB(t)
	pushDB(t, db)

	docPipelineID := "doc-flow-1"
	if err := db.Create(&entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		PipelineID:   &docPipelineID,
		Name:         ptr("doc.pdf"),
		Status:       ptr("1"),
	}).Error; err != nil {
		t.Fatalf("create document: %v", err)
	}
	kbPipelineID := "kb-flow-1"
	if err := db.Create(&entity.Knowledgebase{
		ID:           "kb-1",
		TenantID:     "tenant-1",
		EmbdID:       "embd-1",
		PipelineID:   &kbPipelineID,
		ParserConfig: entity.JSONMap{},
		Status:       ptr("1"),
	}).Error; err != nil {
		t.Fatalf("create knowledgebase: %v", err)
	}
	if err := db.Create(&entity.Tenant{
		ID:     "tenant-1",
		Status: ptr("1"),
	}).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	ctx, err := LoadFromIngestionTask(&entity.IngestionTask{
		ID:         "task-1",
		DocumentID: "doc-1",
		DatasetID:  "kb-1",
	})
	if err != nil {
		t.Fatalf("LoadFromIngestionTask: %v", err)
	}
	if ctx.PipelineID != docPipelineID {
		t.Fatalf("PipelineID = %q, want %q", ctx.PipelineID, docPipelineID)
	}
}
