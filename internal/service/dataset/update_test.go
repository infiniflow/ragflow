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
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"

	"gorm.io/gorm"
)

func TestDatasetServiceUpdateDatasetUpdatesFields(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
	insertDatasetUpdateCanvas(t, "abcdef0123456789abcdef0123456789", "tenant-1")

	name := "  Renamed Dataset  "
	description := "updated description"
	language := "English"
	permission := string(entity.TenantPermissionTeam)
	chunkMethod := string(entity.ParserTypeBook)
	embeddingModel := "BAAI/bge-large-zh-v1.5@Builtin"

	result, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{
		Name:           &name,
		Description:    &description,
		Language:       &language,
		Permission:     &permission,
		ParserID:       &chunkMethod,
		EmbeddingModel: &embeddingModel,
	})
	if err != nil {
		t.Fatalf("UpdateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if result["name"] != "Renamed Dataset" {
		t.Fatalf("expected trimmed name in response, got %#v", result["name"])
	}
	if result["parser_id"] != chunkMethod {
		t.Fatalf("expected parser_id %q, got %#v", chunkMethod, result["parser_id"])
	}
	if result["embedding_model"] != embeddingModel {
		t.Fatalf("expected embedding model %q, got %#v", embeddingModel, result["embedding_model"])
	}
	if connectors, ok := result["connectors"].([]*dao.ConnectorDatasetListItem); !ok || len(connectors) != 0 {
		t.Fatalf("expected empty connector list, got %#v", result["connectors"])
	}

	persisted, err := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if err != nil {
		t.Fatalf("get updated kb: %v", err)
	}
	if persisted.Name != "Renamed Dataset" {
		t.Fatalf("expected persisted trimmed name, got %q", persisted.Name)
	}
	if persisted.Description == nil || *persisted.Description != description {
		t.Fatalf("expected persisted description, got %#v", persisted.Description)
	}
	if persisted.Permission != permission {
		t.Fatalf("expected persisted permission %q, got %q", permission, persisted.Permission)
	}
	if persisted.ParserID != chunkMethod {
		t.Fatalf("expected parser id %q, got %q", chunkMethod, persisted.ParserID)
	}
	if persisted.EmbdID != embeddingModel {
		t.Fatalf("expected embd id %q, got %q", embeddingModel, persisted.EmbdID)
	}
	// parser_config stores DSL runtime component params directly.
	if _, ok := persisted.ParserConfig["Parser:HipSignsRhyme"].(map[string]interface{}); !ok {
		t.Fatalf("expected Parser:HipSignsRhyme in parser_config, got %#v", persisted.ParserConfig)
	}
}

func TestUpdateDataset_RejectsSimultaneousParserIDAndPipelineID(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	chunkMethod := "book"
	pipelineID := "abcdef0123456789abcdef0123456789"

	_, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{
		ParserID:   &chunkMethod,
		PipelineID: &pipelineID,
	})
	if err == nil {
		t.Fatal("expected mutual-exclusivity error when both parser_id and pipeline_id are set")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected error to mention 'mutually exclusive', got: %v", err)
	}
}

func TestUpdateDataset_ParseTypeBuiltinClearsPipelineID(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
	seedDatasetUpdateCanvas(t, "abcdef0123456789abcdef0123456789", "tenant-1",
		datasetUpdateCanvasDSL("Parser:HipSignsRhyme", "chunk_token_num"))

	chunkMethod := "book"
	pipelineID := "ABCDEF0123456789ABCDEF0123456789"
	parseType := 1

	result, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{
		ParserID:   &chunkMethod,
		PipelineID: &pipelineID,
		ParseType:  &parseType,
	})
	if err != nil {
		t.Fatalf("UpdateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	// parse_type=1 clears pipeline_id → only parser_id should be set.
	if result["parser_id"] != chunkMethod {
		t.Fatalf("expected parser_id %q, got %#v", chunkMethod, result["parser_id"])
	}
	if pid, ok := result["pipeline_id"]; ok && pid != nil && pid != "" {
		t.Fatalf("expected pipeline_id to be cleared for BuiltIn mode, got %#v", pid)
	}
}

func TestUpdateDataset_ParseTypePipelineIgnoresParserID(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
	seedDatasetUpdateCanvas(t, "abcdef0123456789abcdef0123456789", "tenant-1",
		datasetUpdateCanvasDSL("Parser:CustomP", "chunk_token_num"))

	chunkMethod := "book"
	pipelineID := "ABCDEF0123456789ABCDEF0123456789"
	parseType := 2

	result, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{
		ParserID:   &chunkMethod,
		PipelineID: &pipelineID,
		ParseType:  &parseType,
	})
	if err != nil {
		t.Fatalf("UpdateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	// parse_type=2 ignores parser_id → pipeline_id should be set;
	// parser_id should keep the original value.
	if result["pipeline_id"] != strings.ToLower(pipelineID) {
		t.Fatalf("expected pipeline_id %q, got %#v", strings.ToLower(pipelineID), result["pipeline_id"])
	}
}

func TestDatasetServiceGetDatasetReturnsEmptyConnectorList(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	datasetID := "11111111111141118111111111111111"
	insertDatasetUpdateKB(t, datasetID, "tenant-1", "Original")

	result, code, err := testDatasetUpdateService(t).GetDataset("11111111-1111-4111-8111-111111111111", "tenant-1")
	if err != nil {
		t.Fatalf("GetDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	connectors, ok := result["connectors"].([]*dao.ConnectorDatasetListItem)
	if !ok {
		t.Fatalf("expected connector list, got %#v", result["connectors"])
	}
	if connectors == nil {
		t.Fatal("expected empty connector list, got nil")
	}
	if len(connectors) != 0 {
		t.Fatalf("expected empty connector list, got %#v", connectors)
	}
}

func TestDatasetServiceUpdateDatasetRejectsMissingDataset(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)

	name := "Renamed"
	_, code, err := testDatasetUpdateService(t).UpdateDataset("missing-kb", "tenant-1", service.UpdateDatasetRequest{Name: &name})
	if err == nil {
		t.Fatal("expected missing dataset error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if err.Error() != "Dataset not found" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetServiceUpdateDatasetRejectsNonOwner(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	name := "Renamed"
	_, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-2", service.UpdateDatasetRequest{Name: &name})
	if err == nil {
		t.Fatal("expected permission error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if !strings.Contains(err.Error(), "lacks permission") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetServiceUpdateDatasetValidatesName(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	name := "   "
	_, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{Name: &name})
	if err == nil {
		t.Fatal("expected name validation error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if err.Error() != "`name` is required." {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetServiceUpdateDatasetRejectsDuplicateName(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
	insertDatasetUpdateKB(t, "kb-2", "tenant-1", "Existing")

	name := "Existing"
	_, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{Name: &name})
	if err == nil {
		t.Fatal("expected duplicate name error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetServiceUpdateDatasetRejectsNoPropertiesModified(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	_, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{})
	if err == nil {
		t.Fatal("expected no-op update error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if err.Error() != "No properties were modified" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetServiceUpdateDatasetLinksConnectors(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
	insertDatasetUpdateConnector(t, "connector-1", "tenant-1")

	autoParse := "0"
	result, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{
		Connectors: &[]service.DatasetConnectorRequest{{ID: "connector-1", AutoParse: autoParse}},
	})
	if err != nil {
		t.Fatalf("UpdateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	connectors, ok := result["connectors"].([]*dao.ConnectorDatasetListItem)
	if !ok {
		t.Fatalf("expected connector list, got %#v", result["connectors"])
	}
	if len(connectors) != 1 {
		t.Fatalf("expected one connector, got %d", len(connectors))
	}
	if connectors[0].ID != "connector-1" || connectors[0].AutoParse != autoParse {
		t.Fatalf("unexpected connector: %#v", connectors[0])
	}

	var link entity.Connector2Kb
	if err := dao.DB.Where("kb_id = ? AND connector_id = ?", "kb-1", "connector-1").First(&link).Error; err != nil {
		t.Fatalf("expected connector link: %v", err)
	}
	if link.AutoParse != autoParse {
		t.Fatalf("expected auto_parse %q, got %q", autoParse, link.AutoParse)
	}
}

func TestDatasetServiceUpdateDatasetAcceptsProviderInstanceEmbedding(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
	insertDatasetUpdateModelProvider(t, "provider-1", "tenant-1", "ZHIPU-AI")
	insertDatasetUpdateModelInstance(t, "instance-1", "provider-1", "test")
	insertDatasetUpdateTenantModel(t, "model-1", "provider-1", "instance-1", "embedding-2", int(entity.ModelTypeEmbedding))

	embeddingModel := "embedding-2@test@ZHIPU-AI"
	result, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{
		EmbeddingModel: &embeddingModel,
	})
	if err != nil {
		t.Fatalf("UpdateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if result["embedding_model"] != embeddingModel {
		t.Fatalf("expected embedding model %q, got %#v", embeddingModel, result["embedding_model"])
	}

	persisted, err := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if err != nil {
		t.Fatalf("get updated kb: %v", err)
	}
	if persisted.EmbdID != embeddingModel {
		t.Fatalf("expected persisted embedding model %q, got %q", embeddingModel, persisted.EmbdID)
	}
}

func TestDatasetServiceUpdateDatasetAcceptsEmbeddingModelID(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
	insertDatasetUpdateModelProvider(t, "provider-1", "tenant-1", "ZHIPU-AI")
	insertDatasetUpdateModelInstance(t, "instance-1", "provider-1", "test")
	insertDatasetUpdateTenantModel(t, "aabbccdd11223344aabbccdd11223344", "provider-1", "instance-1", "embedding-2", int(entity.ModelTypeEmbedding))

	embeddingModelID := "aabbccdd11223344aabbccdd11223344"
	result, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{
		EmbeddingModel: &embeddingModelID,
	})
	if err != nil {
		t.Fatalf("UpdateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if result["embedding_model"] != embeddingModelID {
		t.Fatalf("expected embedding model %q, got %#v", embeddingModelID, result["embedding_model"])
	}

	persisted, err := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if err != nil {
		t.Fatalf("get updated kb: %v", err)
	}
	if persisted.EmbdID != embeddingModelID {
		t.Fatalf("expected persisted embedding model %q, got %q", embeddingModelID, persisted.EmbdID)
	}
	if persisted.TenantEmbdID == nil || *persisted.TenantEmbdID != embeddingModelID {
		t.Fatalf("expected persisted tenant_embd_id %q, got %#v", embeddingModelID, persisted.TenantEmbdID)
	}
}

func TestDatasetServiceUpdateDatasetRejectsEmptyConnectorID(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	connectors := []service.DatasetConnectorRequest{{ID: "  "}}
	_, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{
		Connectors: &connectors,
	})
	if err == nil {
		t.Fatal("expected connector validation error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if err.Error() != "connector id is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetServiceUpdateDatasetRejectsInvalidEmbeddingModelFormat(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	cases := []struct {
		name            string
		embeddingModel  string
		expectedMessage string
	}{
		{"empty", "", "Embedding model identifier must follow <model_name>@<provider> format"},
		{"whitespace", " ", "Embedding model identifier must follow <model_name>@<provider> format"},
		{"missing_at", "BAAI/bge-small-en-v1.5Builtin", "Embedding model identifier must follow <model_name>@<provider> format"},
		{"empty_model_name", "@Builtin", "Both model_name and provider must be non-empty strings"},
		{"empty_provider", "BAAI/bge-small-en-v1.5@", "Both model_name and provider must be non-empty strings"},
	}

	svc := testDatasetUpdateService(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			embdModel := tc.embeddingModel
			_, code, err := svc.UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{
				EmbeddingModel: &embdModel,
			})
			if err == nil {
				t.Fatal("expected embedding model format error")
			}
			if code != common.CodeDataError {
				t.Fatalf("expected data error code, got %d", code)
			}
			if err.Error() != tc.expectedMessage {
				t.Fatalf("unexpected error: got %q, want %q", err.Error(), tc.expectedMessage)
			}
		})
	}
}

func TestDatasetServiceUpdateDatasetRejectsDuplicateNameCaseInsensitive(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
	insertDatasetUpdateKB(t, "kb-2", "tenant-1", "Existing")

	uppercaseName := "EXISTING"
	_, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{
		Name: &uppercaseName,
	})
	if err == nil {
		t.Fatal("expected case-insensitive duplicate name error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetServiceUpdateDatasetPreservesUnmodifiedFields(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)

	description := "original description"
	language := "English"
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
	dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Updates(map[string]interface{}{
		"description": description,
		"language":    language,
	})

	newName := "Renamed Only"
	result, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("UpdateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if result["name"] != newName {
		t.Fatalf("expected updated name %q, got %#v", newName, result["name"])
	}
	if result["description"] != description {
		t.Fatalf("expected description preserved, got %#v", result["description"])
	}
	if result["language"] != language {
		t.Fatalf("expected language preserved, got %#v", result["language"])
	}
	if result["embedding_model"] != "BAAI/bge-large-zh-v1.5@Builtin" {
		t.Fatalf("expected embedding_model preserved, got %#v", result["embedding_model"])
	}

	persisted, err := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if err != nil {
		t.Fatalf("get updated kb: %v", err)
	}
	if persisted.Name != newName {
		t.Fatalf("expected persisted name %q, got %q", newName, persisted.Name)
	}
	if persisted.Description == nil || *persisted.Description != description {
		t.Fatalf("expected persisted description %q, got %#v", description, persisted.Description)
	}
}

func TestDatasetServiceUpdateDatasetPreservesParserConfigOnEmptyUpdate(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
	dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Update("parser_config", entity.JSONMap{
		"chunk_token_num": float64(512),
		"delimiter":       "\n",
	})

	name := "Updated Name"
	_, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{
		Name: &name,
	})
	if err != nil {
		t.Fatalf("UpdateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}

	persisted, err := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if err != nil {
		t.Fatalf("get updated kb: %v", err)
	}
	if persisted.ParserConfig["chunk_token_num"] != float64(512) {
		t.Fatalf("expected chunk_token_num preserved, got %#v", persisted.ParserConfig["chunk_token_num"])
	}
	if persisted.ParserConfig["delimiter"] != "\n" {
		t.Fatalf("expected delimiter preserved, got %#v", persisted.ParserConfig["delimiter"])
	}
}

func TestDatasetServiceDeleteDatasetsRejectsUnauthorizedID(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "11111111111141118111111111111111", "tenant-1", "Test")

	svc := NewDatasetService()
	normalizedID := "11111111111141118111111111111111"
	_, code, err := svc.DeleteDatasets([]string{normalizedID}, false, "tenant-2")
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if !strings.Contains(err.Error(), "lacks permission") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetServiceDeleteDatasetsRejectsAllUnauthorized(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)

	svc := NewDatasetService()
	_, code, err := svc.DeleteDatasets([]string{"d94a8dc02c9711f0930f7fbc369eab6d"}, false, "tenant-1")
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if !strings.Contains(err.Error(), "lacks permission") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func setupDatasetUpdateTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(
		&entity.Connector{},
		&entity.Connector2Kb{},
		&entity.SyncLogs{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
		&entity.UserCanvas{},
	); err != nil {
		t.Fatalf("failed to migrate dataset update tables: %v", err)
	}
	return db
}

func testDatasetUpdateService(t *testing.T) *DatasetService {
	t.Helper()

	return &DatasetService{
		kbDAO:        dao.NewKnowledgebaseDAO(),
		documentDAO:  dao.NewDocumentDAO(),
		connectorDAO: dao.NewConnectorDAO(),
	}
}

func insertDatasetUpdateKB(t *testing.T, id, tenantID, name string) {
	t.Helper()

	kb := &entity.Knowledgebase{
		ID:           id,
		TenantID:     tenantID,
		Name:         name,
		EmbdID:       "BAAI/bge-large-zh-v1.5@Builtin",
		CreatedBy:    tenantID,
		Permission:   string(entity.TenantPermissionMe),
		ParserID:     string(entity.ParserTypeNaive),
		ParserConfig: entity.JSONMap{"chunk_token_num": float64(128)},
		Status:       sptr(string(entity.StatusValid)),
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert test kb: %v", err)
	}
}

func insertDatasetUpdateCanvas(t *testing.T, id, userID string) {
	t.Helper()
	canvas := &entity.UserCanvas{
		ID:     id,
		UserID: userID,
		DSL:    entity.JSONMap{},
	}
	if err := dao.DB.Create(canvas).Error; err != nil {
		t.Fatalf("insert test canvas: %v", err)
	}
}

func insertDatasetUpdateConnector(t *testing.T, id, tenantID string) {
	t.Helper()

	connector := &entity.Connector{
		ID:        id,
		TenantID:  tenantID,
		Name:      "Test Connector",
		Source:    "google_drive",
		InputType: "oauth",
		Config:    entity.JSONMap{"sync_deleted_files": false},
		Status:    string(entity.TaskStatusDone),
	}
	if err := dao.DB.Create(connector).Error; err != nil {
		t.Fatalf("insert test connector: %v", err)
	}
}

func insertDatasetUpdateModelProvider(t *testing.T, id, tenantID, providerName string) {
	t.Helper()

	provider := &entity.TenantModelProvider{
		ID:           id,
		TenantID:     tenantID,
		ProviderName: providerName,
	}
	if err := dao.DB.Create(provider).Error; err != nil {
		t.Fatalf("insert test model provider: %v", err)
	}
}

func insertDatasetUpdateModelInstance(t *testing.T, id, providerID, instanceName string) {
	t.Helper()

	instance := &entity.TenantModelInstance{
		ID:           id,
		ProviderID:   providerID,
		InstanceName: instanceName,
		APIKey:       "test-api-key",
		Status:       "active",
		Extra:        "{}",
	}
	if err := dao.DB.Create(instance).Error; err != nil {
		t.Fatalf("insert test model instance: %v", err)
	}
}

func insertDatasetUpdateTenantModel(t *testing.T, id, providerID, instanceID, modelName string, modelType int) {
	t.Helper()

	model := &entity.TenantModel{
		ID:         id,
		ProviderID: providerID,
		InstanceID: instanceID,
		ModelName:  modelName,
		ModelType:  modelType,
		Status:     "active",
		Extra:      "{}",
	}
	if err := dao.DB.Create(model).Error; err != nil {
		t.Fatalf("insert test tenant model: %v", err)
	}
}

// seedDatasetUpdateCanvas migrates user_canvas on the active test DB and
// inserts a canvas row with the given DSL.
func seedDatasetUpdateCanvas(t *testing.T, id, userID string, dslJSON []byte) {
	t.Helper()
	if err := dao.DB.AutoMigrate(&entity.UserCanvas{}); err != nil {
		t.Fatalf("migrate user_canvas: %v", err)
	}
	var dslMap map[string]any
	if err := json.Unmarshal(dslJSON, &dslMap); err != nil {
		t.Fatalf("unmarshal seed dsl: %v", err)
	}
	canvas := &entity.UserCanvas{
		ID:             id,
		UserID:         userID,
		Tags:           "",
		Permission:     "me",
		CanvasCategory: "agent_canvas",
		DSL:            entity.JSONMap(dslMap),
	}
	if err := dao.DB.Create(canvas).Error; err != nil {
		t.Fatalf("seed canvas: %v", err)
	}
}

// datasetUpdateCanvasDSL builds a minimal canvas DSL declaring a Parser
// component with the given cpnID and param keys.
func datasetUpdateCanvasDSL(cpnID string, paramKeys ...string) []byte {
	params := map[string]any{"outputs": map[string]any{}}
	for _, k := range paramKeys {
		params[k] = map[string]any{}
	}
	dsl := map[string]any{
		"components": map[string]any{
			cpnID: map[string]any{
				"obj": map[string]any{"component_name": "Parser", "params": params},
			},
		},
	}
	raw, _ := json.Marshal(dsl)
	return raw
}

// --- Step 3: component_params validation wired into UpdateDataset ---

// insertDatasetUpdateCanvasKB seeds a KB bound to a custom canvas pipeline
// (PipelineID set, ParserID empty) for the canvas validation tests.
func insertDatasetUpdateCanvasKB(t *testing.T, id, tenantID, name, pipelineID string) {
	t.Helper()

	pid := pipelineID
	kb := &entity.Knowledgebase{
		ID:           id,
		TenantID:     tenantID,
		Name:         name,
		EmbdID:       "BAAI/bge-large-zh-v1.5@Builtin",
		CreatedBy:    tenantID,
		Permission:   string(entity.TenantPermissionMe),
		ParserID:     "",
		PipelineID:   &pid,
		ParserConfig: entity.JSONMap{"chunk_token_num": float64(128)},
		Status:       sptr(string(entity.StatusValid)),
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert test canvas kb: %v", err)
	}
}

func TestUpdateDataset_StripsUnknownParam_Builtin(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	result, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{
		ParserConfig: map[string]interface{}{
			"Parser:HipSignsRhyme": map[string]interface{}{
				"no_such_param": 1,
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateDataset should succeed (unknown params are stripped): %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}

	persisted, err := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if err != nil {
		t.Fatalf("get updated kb: %v", err)
	}
	cp := map[string]interface{}(persisted.ParserConfig)
	rhyme, ok := cp["Parser:HipSignsRhyme"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected Parser:HipSignsRhyme persisted, got %#v", cp)
	}
	if _, exists := rhyme["no_such_param"]; exists {
		t.Fatal("expected no_such_param to be stripped")
	}
	if result["parser_id"] != string(entity.ParserTypeNaive) {
		t.Fatalf("expected parser_id preserved, got %#v", result["parser_id"])
	}
}

func TestUpdateDataset_AcceptsValidComponentParams_Builtin(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	result, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{
		ParserConfig: map[string]interface{}{
			"Parser:HipSignsRhyme": map[string]interface{}{
				"pdf": map[string]interface{}{"parse_method": "deepdoc"},
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if result["parser_id"] != string(entity.ParserTypeNaive) {
		t.Fatalf("expected parser_id preserved, got %#v", result["parser_id"])
	}

	persisted, err := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if err != nil {
		t.Fatalf("get updated kb: %v", err)
	}
	cp := map[string]interface{}(persisted.ParserConfig)
	if len(cp) == 0 {
		t.Fatalf("expected component_params persisted, got %#v", persisted.ParserConfig)
	}
	rhyme, ok := cp["Parser:HipSignsRhyme"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected Parser:HipSignsRhyme persisted, got %#v", cp)
	}
	pdf, ok := rhyme["pdf"].(map[string]interface{})
	if !ok || pdf["parse_method"] != "deepdoc" {
		t.Fatalf("expected pdf setup persisted, got %#v", rhyme)
	}
}

func TestUpdateDataset_StripsCanvasUnknownParam(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	dsl := datasetUpdateCanvasDSL("Parser:CustomRhyme", "pdf")
	seedDatasetUpdateCanvas(t, "canvas-1", "tenant-1", dsl)
	insertDatasetUpdateCanvasKB(t, "kb-1", "tenant-1", "Original", "canvas-1")

	result, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{
		ParserConfig: map[string]interface{}{
			"Parser:NoSuch": map[string]interface{}{
				"pdf": map[string]interface{}{},
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateDataset should succeed (unknown cpnID is stripped): %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}

	persisted, err := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if err != nil {
		t.Fatalf("get updated kb: %v", err)
	}
	cp := map[string]interface{}(persisted.ParserConfig)
	if _, exists := cp["Parser:NoSuch"]; exists {
		t.Fatal("expected unknown cpnID Parser:NoSuch to be stripped")
	}
	// The valid cpnID from the canvas DSL should still be present.
	if _, exists := cp["Parser:CustomRhyme"]; !exists {
		t.Fatal("expected Parser:CustomRhyme (from canvas DSL defaults) to be present")
	}
	if result["pipeline_id"] != "canvas-1" {
		t.Fatalf("expected pipeline_id preserved, got %#v", result["pipeline_id"])
	}
}

func TestUpdateDataset_AcceptsValidCanvasComponentParams(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	dsl := datasetUpdateCanvasDSL("Parser:CustomRhyme", "pdf")
	seedDatasetUpdateCanvas(t, "canvas-1", "tenant-1", dsl)
	insertDatasetUpdateCanvasKB(t, "kb-1", "tenant-1", "Original", "canvas-1")

	result, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{
		ParserConfig: map[string]interface{}{
			"Parser:CustomRhyme": map[string]interface{}{
				"pdf": map[string]interface{}{},
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if result["pipeline_id"] != "canvas-1" {
		t.Fatalf("expected pipeline_id preserved, got %#v", result["pipeline_id"])
	}
}

// TestUpdateDataset_SwitchCanvasToBuiltinValidatesAgainstBuiltin covers the
// effective-ref edge where a request parser_id clears the existing canvas
// pipeline: the override must be validated against the new builtin template,
// not the stale canvas.
func TestUpdateDataset_SwitchCanvasToBuiltinValidatesAgainstBuiltin(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	dsl := datasetUpdateCanvasDSL("Parser:CustomRhyme", "pdf")
	seedDatasetUpdateCanvas(t, "canvas-1", "tenant-1", dsl)
	insertDatasetUpdateCanvasKB(t, "kb-1", "tenant-1", "Original", "canvas-1")

	chunkMethod := "naive"
	_, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", service.UpdateDatasetRequest{
		ParserID: &chunkMethod,
		ParserConfig: map[string]interface{}{
			// Valid for the "general" builtin template, not the canvas.
			"Parser:HipSignsRhyme": map[string]interface{}{
				"pdf": map[string]interface{}{"parse_method": "deepdoc"},
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
}
