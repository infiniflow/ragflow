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
	insertCreateDatasetTenant(t, "user-1")
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
	insertCreateDatasetTenant(t, "owner-1")
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
			"metadata": map[string]any{
				"enabled":           false,
				"metadata":          []any{},
				"built_in_metadata": []any{},
			},
			"Extractor:AutoExtractDefault": map[string]any{
				"metadata": map[string]any{
					"enabled": true,
					"metadata": []any{
						map[string]any{"key": "stale", "type": "string"},
					},
					"built_in_metadata": []any{},
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
	if _, ok := extractor["enable_metadata"]; ok {
		t.Fatalf("extractor enable_metadata should not be present, got %#v", extractor["enable_metadata"])
	}
	extractorMeta, ok := extractor["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected modular metadata map in extractor, got %#v", extractor["metadata"])
	}
	if enabled, ok := extractorMeta["enabled"].(bool); !ok || enabled {
		t.Fatalf("extractor metadata.enabled = %#v, want false when dataset-level flag stays disabled", extractorMeta["enabled"])
	}

	// State 2: When enabled, updating fields updates component with enabled = true
	if err := dao.DB.Model(&entity.Knowledgebase{}).
		Where("id = ?", "kb-1").
		Update("parser_config", entity.JSONMap{
			"metadata": map[string]any{
				"enabled":           true,
				"metadata":          []any{},
				"built_in_metadata": []any{},
			},
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
	extractorMeta, ok = extractor["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected modular metadata map in extractor, got %#v", extractor["metadata"])
	}
	if enabled, ok := extractorMeta["enabled"].(bool); !ok || !enabled {
		t.Fatalf("extractor metadata.enabled = %#v, want true", extractorMeta["enabled"])
	}
	gotFields, ok := extractorMeta["metadata"].([]interface{})
	if !ok {
		t.Fatalf("extractor metadata.metadata = %#v, want []interface{}", extractorMeta["metadata"])
	}
	if len(gotFields) != 1 {
		t.Fatalf("extractor metadata.metadata len = %d, want 1 (%#v)", len(gotFields), gotFields)
	}
	gotBuiltIn, ok := extractorMeta["built_in_metadata"].([]interface{})
	if !ok {
		t.Fatalf("extractor metadata.built_in_metadata = %#v, want []interface{}", extractorMeta["built_in_metadata"])
	}
	if len(gotBuiltIn) != 1 {
		t.Fatalf("extractor metadata.built_in_metadata len = %d, want 1 (%#v)", len(gotBuiltIn), gotBuiltIn)
	}

	// State 3: Empty fields auto-disables enable_metadata
	_, code, err = (&DatasetService{
		kbDAO:     dao.NewKnowledgebaseDAO(),
		tenantDAO: dao.NewTenantDAO(),
	}).UpdateMetadataConfig(ctx, "kb-1", "tenant-1", &service.MetadataConfigRequest{
		Metadata:        []service.MetadataConfigField{},
		BuiltInMetadata: []service.MetadataConfigField{},
	})
	if err != nil {
		t.Fatalf("UpdateMetadataConfig with empty fields failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	persisted, err = dao.NewKnowledgebaseDAO().GetByID(ctx, db, "kb-1")
	if err != nil {
		t.Fatalf("failed to fetch persisted dataset: %v", err)
	}
	if _, ok := persisted.ParserConfig["enable_metadata"]; ok {
		t.Fatalf("enable_metadata should be absent, got %#v", persisted.ParserConfig["enable_metadata"])
	}
	metaObj, ok := persisted.ParserConfig["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected modular metadata map, got %#v", persisted.ParserConfig["metadata"])
	}
	if enabled, ok := metaObj["enabled"].(bool); !ok || enabled {
		t.Fatalf("expected modular metadata.enabled == false after emptying fields, got %#v", metaObj["enabled"])
	}
}

func TestDatasetServiceGetMetadataConfig(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")
	insertDatasetMetadataConfigKB(t, "kb-1", "tenant-1")

	ctx := t.Context()
	_, code, err := (&DatasetService{
		kbDAO:     dao.NewKnowledgebaseDAO(),
		tenantDAO: dao.NewTenantDAO(),
	}).UpdateMetadataConfig(ctx, "kb-1", "tenant-1", &service.MetadataConfigRequest{
		Metadata: []service.MetadataConfigField{
			{Key: "author", Type: "string"},
		},
		BuiltInMetadata: []service.MetadataConfigField{
			{Key: "file_name", Type: "string"},
		},
	})
	if err != nil {
		t.Fatalf("UpdateMetadataConfig failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}

	result, code, err := (&DatasetService{
		kbDAO:     dao.NewKnowledgebaseDAO(),
		tenantDAO: dao.NewTenantDAO(),
	}).GetMetadataConfig(ctx, "kb-1", "tenant-1")
	if err != nil {
		t.Fatalf("GetMetadataConfig failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	metadata, ok := result["metadata"].([]interface{})
	if !ok || len(metadata) != 1 {
		t.Fatalf("expected 1 metadata item, got %#v", result["metadata"])
	}
	field0, ok := metadata[0].(map[string]interface{})
	if !ok || field0["key"] != "author" {
		t.Fatalf("expected key author, got %#v", metadata[0])
	}
	builtIn, ok := result["built_in_metadata"].([]interface{})
	if !ok || len(builtIn) != 1 {
		t.Fatalf("expected 1 built_in_metadata item, got %#v", result["built_in_metadata"])
	}
	builtIn0, ok := builtIn[0].(map[string]interface{})
	if !ok || builtIn0["key"] != "file_name" {
		t.Fatalf("expected key file_name, got %#v", builtIn[0])
	}
}

func TestUpdateMetadataConfig_TenantLookupFailureDoesNotPersist(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	// kb-1 points at a tenant that does not exist, so tenantDAO.GetByID fails.
	insertDatasetMetadataConfigKB(t, "kb-1", "missing-tenant")

	ctx := t.Context()
	result, code, err := (&DatasetService{
		kbDAO:     dao.NewKnowledgebaseDAO(),
		tenantDAO: dao.NewTenantDAO(),
	}).UpdateMetadataConfig(ctx, "kb-1", "missing-tenant", &service.MetadataConfigRequest{
		Metadata: []service.MetadataConfigField{
			{Key: "author", Type: "string"},
		},
		BuiltInMetadata: []service.MetadataConfigField{},
	})
	if code != common.CodeServerError || err == nil {
		t.Fatalf("expected server error on tenant lookup failure, got code=%d err=%v result=%#v", code, err, result)
	}

	persisted, err := dao.NewKnowledgebaseDAO().GetByID(ctx, db, "kb-1")
	if err != nil {
		t.Fatalf("get kb: %v", err)
	}
	if _, ok := persisted.ParserConfig["metadata"]; ok {
		t.Fatalf("parser config must not be persisted on tenant lookup failure, got %#v", persisted.ParserConfig)
	}
}

func TestUpdateMetadataConfig_ExplicitEnabledPreservedWhenEmpty(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")
	insertDatasetMetadataConfigKB(t, "kb-1", "tenant-1")

	enabled := true
	ctx := t.Context()
	result, code, err := (&DatasetService{
		kbDAO:     dao.NewKnowledgebaseDAO(),
		tenantDAO: dao.NewTenantDAO(),
	}).UpdateMetadataConfig(ctx, "kb-1", "tenant-1", &service.MetadataConfigRequest{
		Enabled:         &enabled,
		Metadata:        []service.MetadataConfigField{},
		BuiltInMetadata: []service.MetadataConfigField{},
	})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateMetadataConfig failed: code=%d err=%v", code, err)
	}
	respEnabled, ok := result["enabled"].(bool)
	if !ok || !respEnabled {
		t.Fatalf("expected enabled=true in response, got %#v", result["enabled"])
	}

	persisted, err := dao.NewKnowledgebaseDAO().GetByID(ctx, db, "kb-1")
	if err != nil {
		t.Fatalf("get kb: %v", err)
	}
	metaObj, ok := persisted.ParserConfig["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected modular metadata, got %#v", persisted.ParserConfig["metadata"])
	}
	if persistedEnabled, _ := metaObj["enabled"].(bool); !persistedEnabled {
		t.Fatalf("expected metadata.enabled=true, got %#v", metaObj["enabled"])
	}
}

func TestUpdateDocumentMetadataConfig_KBAndTenantLookupFailure(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDatasetMetadataConfigKB(t, "kb-ok", "owner-1")
	insertDatasetMetadataConfigDoc(t, "doc-ok", "kb-ok", entity.JSONMap{})
	ctx := t.Context()

	// Missing KB -> database layer should surface as CodeServerError / not found
	_, code, err := testDatasetServiceForDocumentMetadataConfig(t).UpdateDocumentMetadataConfig(
		ctx, "owner-1", "kb-missing", "doc-ok",
		map[string]interface{}{"metadata": map[string]interface{}{"a": "b"}},
	)
	if code == common.CodeSuccess || err == nil {
		t.Fatalf("expected error for missing kb, got code=%v err=%v", code, err)
	}

	// KB exists but tenant lookup fails (kb.TenantID points to missing tenant)
	insertDatasetMetadataConfigKB(t, "kb-bad-tenant", "missing-tenant")
	insertDatasetMetadataConfigDoc(t, "doc2", "kb-bad-tenant", entity.JSONMap{})
	_, code, err = testDatasetServiceForDocumentMetadataConfig(t).UpdateDocumentMetadataConfig(
		ctx, "missing-tenant", "kb-bad-tenant", "doc2",
		map[string]interface{}{"metadata": map[string]interface{}{"a": "b"}},
	)
	if code != common.CodeServerError || err == nil {
		t.Fatalf("expected server error for tenant lookup failure, got code=%v err=%v", code, err)
	}
}
