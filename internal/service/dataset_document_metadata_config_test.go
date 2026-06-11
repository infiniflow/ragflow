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
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func testDatasetServiceForDocumentMetadataConfig(t *testing.T) *DatasetService {
	t.Helper()
	return &DatasetService{
		kbDAO:       dao.NewKnowledgebaseDAO(),
		documentDAO: dao.NewDocumentDAO(),
	}
}

func insertDatasetMetadataConfigKB(t *testing.T, datasetID, tenantID string) {
	t.Helper()
	kb := &entity.Knowledgebase{
		ID:           datasetID,
		TenantID:     tenantID,
		Name:         "test-kb",
		EmbdID:       "embedding@OpenAI",
		CreatedBy:    tenantID,
		Permission:   string(entity.TenantPermissionMe),
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		Status:       sptr("1"),
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert test kb: %v", err)
	}
}

func insertDatasetMetadataConfigDoc(t *testing.T, docID, datasetID string, parserConfig entity.JSONMap) {
	t.Helper()
	doc := &entity.Document{
		ID:           docID,
		KbID:         datasetID,
		ParserID:     "naive",
		ParserConfig: parserConfig,
		SourceType:   "local",
		Type:         "pdf",
		CreatedBy:    "user-1",
		Suffix:       ".pdf",
		Status:       sptr("1"),
	}
	if err := dao.DB.Create(doc).Error; err != nil {
		t.Fatalf("insert test doc: %v", err)
	}
}

func TestDatasetServiceUpdateDocumentMetadataConfig(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDatasetMetadataConfigKB(t, "kb-1", "user-1")
	insertDatasetMetadataConfigDoc(t, "doc-1", "kb-1", entity.JSONMap{"pages": []interface{}{1, 2}})

	metadata := map[string]interface{}{"author": "Alice", "year": float64(2026)}
	doc, code, err := testDatasetServiceForDocumentMetadataConfig(t).UpdateDocumentMetadataConfig(
		"user-1",
		"kb-1",
		"doc-1",
		map[string]interface{}{"metadata": metadata},
	)
	if err != nil {
		t.Fatalf("UpdateDocumentMetadataConfig failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if doc == nil {
		t.Fatal("expected updated document")
	}
	if doc.ParserConfig["pages"] == nil {
		t.Fatalf("existing parser_config fields should be preserved: %#v", doc.ParserConfig)
	}

	updatedMetadata, ok := doc.ParserConfig["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected metadata map, got %#v", doc.ParserConfig["metadata"])
	}
	if updatedMetadata["author"] != "Alice" || updatedMetadata["year"] != float64(2026) {
		t.Fatalf("unexpected metadata: %#v", updatedMetadata)
	}

	persisted, err := dao.NewDocumentDAO().GetByID("doc-1")
	if err != nil {
		t.Fatalf("failed to fetch persisted document: %v", err)
	}
	if persisted.ParserConfig["metadata"] == nil {
		t.Fatalf("metadata was not persisted: %#v", persisted.ParserConfig)
	}
}

func TestDatasetServiceUpdateDocumentMetadataConfigRequiresMetadata(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDatasetMetadataConfigKB(t, "kb-1", "user-1")
	insertDatasetMetadataConfigDoc(t, "doc-1", "kb-1", entity.JSONMap{})

	_, code, err := testDatasetServiceForDocumentMetadataConfig(t).UpdateDocumentMetadataConfig(
		"user-1",
		"kb-1",
		"doc-1",
		map[string]interface{}{},
	)
	if err == nil {
		t.Fatal("expected metadata required error")
	}
	if code != common.CodeArgumentError {
		t.Fatalf("expected argument error code, got %d", code)
	}
	if err.Error() != "metadata is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetServiceUpdateDocumentMetadataConfigRejectsNonOwner(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDatasetMetadataConfigKB(t, "kb-1", "owner-1")
	insertDatasetMetadataConfigDoc(t, "doc-1", "kb-1", entity.JSONMap{})

	_, code, err := testDatasetServiceForDocumentMetadataConfig(t).UpdateDocumentMetadataConfig(
		"user-1",
		"kb-1",
		"doc-1",
		map[string]interface{}{"metadata": map[string]interface{}{"author": "Alice"}},
	)
	if err == nil {
		t.Fatal("expected ownership error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if err.Error() != "You don't own the dataset." {
		t.Fatalf("unexpected error: %v", err)
	}
}
