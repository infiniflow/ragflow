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

package dataset

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func metadataFlagInt(t *testing.T, value interface{}) int {
	t.Helper()
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		t.Fatalf("unexpected metadata flag type %T (%#v)", value, value)
		return 0
	}
}

func testDatasetServiceForDocumentMetadataConfig(t *testing.T) *DatasetService {
	t.Helper()
	return &DatasetService{
		kbDAO:       dao.NewKnowledgebaseDAO(),
		documentDAO: dao.NewDocumentDAO(),
		tenantDAO:   dao.NewTenantDAO(),
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

func insertDatasetMetadataConfigTeamMember(t *testing.T, userID, tenantID string) {
	t.Helper()
	if err := dao.DB.Create(&entity.UserTenant{
		ID:        userID + "-" + tenantID,
		UserID:    userID,
		TenantID:  tenantID,
		Role:      "normal",
		InvitedBy: tenantID,
		Status:    sptr("1"),
	}).Error; err != nil {
		t.Fatalf("insert user tenant: %v", err)
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

	ctx := t.Context()
	metadata := map[string]interface{}{"author": "Alice", "year": float64(2026)}
	doc, code, err := testDatasetServiceForDocumentMetadataConfig(t).UpdateDocumentMetadataConfig(
		ctx,
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

	persisted, err := dao.NewDocumentDAO().GetByID(ctx, db, "doc-1")
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

	ctx := t.Context()
	_, code, err := testDatasetServiceForDocumentMetadataConfig(t).UpdateDocumentMetadataConfig(
		ctx,
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

	ctx := t.Context()
	_, code, err := testDatasetServiceForDocumentMetadataConfig(t).UpdateDocumentMetadataConfig(
		ctx,
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
	if err.Error() != "you don't own the dataset" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetServiceUpdateDocumentMetadataConfigAllowsTeamMember(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDatasetMetadataConfigKB(t, "kb-1", "owner-1")
	if err := dao.DB.Model(&entity.Knowledgebase{}).
		Where("id = ?", "kb-1").
		Update("permission", string(entity.TenantPermissionTeam)).Error; err != nil {
		t.Fatalf("update kb permission: %v", err)
	}
	insertDatasetMetadataConfigTeamMember(t, "user-1", "owner-1")
	insertDatasetMetadataConfigDoc(t, "doc-1", "kb-1", entity.JSONMap{})

	ctx := t.Context()
	doc, code, err := testDatasetServiceForDocumentMetadataConfig(t).UpdateDocumentMetadataConfig(
		ctx,
		"user-1",
		"kb-1",
		"doc-1",
		map[string]interface{}{"metadata": map[string]interface{}{"author": "Alice"}},
	)
	if err != nil {
		t.Fatalf("UpdateDocumentMetadataConfig failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if doc.ParserConfig["metadata"] == nil {
		t.Fatalf("metadata was not updated: %#v", doc.ParserConfig)
	}
}

func TestDatasetServiceUpdateMetadataConfigSyncsExtractorSchema(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")
	insertDatasetMetadataConfigKB(t, "kb-1", "tenant-1")
	if err := dao.DB.Model(&entity.Knowledgebase{}).
		Where("id = ?", "kb-1").
		Update("parser_config", entity.JSONMap{
			"enable_metadata": false,
			"Extractor:AutoExtractDefault": map[string]any{
				"enable_metadata": 1,
				"metadata": []any{
					map[string]any{"key": "stale", "type": "string"},
				},
			},
		}).Error; err != nil {
		t.Fatalf("seed parser_config: %v", err)
	}

	ctx := t.Context()
	result, code, err := (&DatasetService{
		kbDAO:     dao.NewKnowledgebaseDAO(),
		tenantDAO: dao.NewTenantDAO(),
	}).UpdateMetadataConfig(ctx, "kb-1", "tenant-1", &service.MetadataConfigRequest{
		Metadata: []service.MetadataConfigField{
			{Key: "author", Type: "string"},
		},
		BuiltInMetadata: []service.MetadataConfigField{
			{Key: "document_name", Type: "string"},
		},
	})
	if err != nil {
		t.Fatalf("UpdateMetadataConfig failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if result["metadata"] == nil {
		t.Fatalf("metadata response missing: %#v", result)
	}

	persisted, err := dao.NewKnowledgebaseDAO().GetByID(ctx, db, "kb-1")
	if err != nil {
		t.Fatalf("failed to fetch persisted dataset: %v", err)
	}
	extractor, ok := persisted.ParserConfig["Extractor:AutoExtractDefault"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected extractor component params, got %#v", persisted.ParserConfig["Extractor:AutoExtractDefault"])
	}
	if got := metadataFlagInt(t, extractor["enable_metadata"]); got != 0 {
		t.Fatalf("extractor enable_metadata = %#v, want 0 when top-level flag stays disabled", extractor["enable_metadata"])
	}

	if err := dao.DB.Model(&entity.Knowledgebase{}).
		Where("id = ?", "kb-1").
		Update("parser_config", entity.JSONMap{
			"enable_metadata":              true,
			"Extractor:AutoExtractDefault": map[string]any{},
		}).Error; err != nil {
		t.Fatalf("reset parser_config: %v", err)
	}

	_, code, err = (&DatasetService{
		kbDAO:     dao.NewKnowledgebaseDAO(),
		tenantDAO: dao.NewTenantDAO(),
	}).UpdateMetadataConfig(ctx, "kb-1", "tenant-1", &service.MetadataConfigRequest{
		Metadata: []service.MetadataConfigField{
			{Key: "author", Type: "string"},
		},
		BuiltInMetadata: []service.MetadataConfigField{
			{Key: "document_name", Type: "string"},
		},
	})
	if err != nil {
		t.Fatalf("UpdateMetadataConfig with enabled metadata failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}

	persisted, err = dao.NewKnowledgebaseDAO().GetByID(ctx, db, "kb-1")
	if err != nil {
		t.Fatalf("failed to fetch persisted dataset: %v", err)
	}
	extractor, ok = persisted.ParserConfig["Extractor:AutoExtractDefault"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected extractor component params, got %#v", persisted.ParserConfig["Extractor:AutoExtractDefault"])
	}
	if got := metadataFlagInt(t, extractor["enable_metadata"]); got != 1 {
		t.Fatalf("extractor enable_metadata = %#v, want 1", extractor["enable_metadata"])
	}
	gotFields, ok := extractor["metadata"].([]interface{})
	if !ok {
		t.Fatalf("extractor metadata = %#v, want []interface{}", extractor["metadata"])
	}
	wantFields := []map[string]interface{}{
		{"key": "author", "type": "string"},
		{"key": "document_name", "type": "string"},
	}
	if len(gotFields) != len(wantFields) {
		t.Fatalf("extractor metadata len = %d, want %d (%#v)", len(gotFields), len(wantFields), gotFields)
	}
	for i, want := range wantFields {
		field, ok := gotFields[i].(map[string]interface{})
		if !ok {
			t.Fatalf("extractor metadata[%d] = %#v, want map[string]interface{}", i, gotFields[i])
		}
		if field["key"] != want["key"] || field["type"] != want["type"] {
			t.Fatalf("extractor metadata[%d] = %#v, want key/type %#v", i, field, want)
		}
	}
}
