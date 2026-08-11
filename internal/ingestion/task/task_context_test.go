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

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
)

func TestLoadFromIngestionTask_FallsBackToKnowledgebasePipelineID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	if err := db.Create(&entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		Name:         testutil.StrPtr("doc.pdf"),
		Status:       testutil.StrPtr("1"),
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
		Status:       testutil.StrPtr("1"),
	}).Error; err != nil {
		t.Fatalf("create knowledgebase: %v", err)
	}
	if err := db.Create(&entity.Tenant{
		ID:     "tenant-1",
		Status: testutil.StrPtr("1"),
	}).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	ctx := t.Context()
	taskCtx, err := LoadFromIngestionTask(ctx, &entity.IngestionTask{
		ID:         "task-1",
		DocumentID: "doc-1",
		DatasetID:  "kb-1",
	})
	if err != nil {
		t.Fatalf("LoadFromIngestionTask: %v", err)
	}
	if taskCtx.PipelineID != kbPipelineID {
		t.Fatalf("PipelineID = %q, want %q", taskCtx.PipelineID, kbPipelineID)
	}
}

func TestLoadFromIngestionTask_PrefersDocumentPipelineID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	docPipelineID := "doc-flow-1"
	if err := db.Create(&entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		PipelineID:   &docPipelineID,
		Name:         testutil.StrPtr("doc.pdf"),
		Status:       testutil.StrPtr("1"),
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
		Status:       testutil.StrPtr("1"),
	}).Error; err != nil {
		t.Fatalf("create knowledgebase: %v", err)
	}
	if err := db.Create(&entity.Tenant{
		ID:     "tenant-1",
		Status: testutil.StrPtr("1"),
	}).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	ctx := t.Context()
	taskCtx, err := LoadFromIngestionTask(ctx, &entity.IngestionTask{
		ID:         "task-1",
		DocumentID: "doc-1",
		DatasetID:  "kb-1",
	})
	if err != nil {
		t.Fatalf("LoadFromIngestionTask: %v", err)
	}
	if taskCtx.PipelineID != docPipelineID {
		t.Fatalf("PipelineID = %q, want %q", taskCtx.PipelineID, docPipelineID)
	}
}
