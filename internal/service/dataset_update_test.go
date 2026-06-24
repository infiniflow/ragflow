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
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

func TestDatasetServiceUpdateDatasetUpdatesFields(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	name := "  Renamed Dataset  "
	description := "updated description"
	language := "English"
	permission := string(entity.TenantPermissionTeam)
	chunkMethod := string(entity.ParserTypeBook)
	embeddingModel := "BAAI/bge-large-zh-v1.5@Builtin"
	pipelineID := "ABCDEF0123456789ABCDEF0123456789"

	result, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", UpdateDatasetRequest{
		Name:           &name,
		Description:    &description,
		Language:       &language,
		Permission:     &permission,
		ChunkMethod:    &chunkMethod,
		EmbeddingModel: &embeddingModel,
		PipelineID:     &pipelineID,
		ParserConfig: map[string]interface{}{
			"parent_child": map[string]interface{}{
				"use_parent_child": true,
			},
			"ext": map[string]interface{}{
				"delimiter": "\n\n",
			},
		},
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
	if result["chunk_method"] != chunkMethod {
		t.Fatalf("expected chunk method %q, got %#v", chunkMethod, result["chunk_method"])
	}
	if result["embedding_model"] != embeddingModel {
		t.Fatalf("expected embedding model %q, got %#v", embeddingModel, result["embedding_model"])
	}
	if result["pipeline_id"] != strings.ToLower(pipelineID) {
		t.Fatalf("expected normalized pipeline id, got %#v", result["pipeline_id"])
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
	if persisted.PipelineID == nil || *persisted.PipelineID != strings.ToLower(pipelineID) {
		t.Fatalf("expected normalized pipeline id persisted, got %#v", persisted.PipelineID)
	}
	if persisted.ParserConfig["chunk_token_num"] != float64(128) {
		t.Fatalf("expected existing parser_config value preserved, got %#v", persisted.ParserConfig)
	}
	if persisted.ParserConfig["enable_children"] != true {
		t.Fatalf("expected enable_children normalized, got %#v", persisted.ParserConfig)
	}
	if persisted.ParserConfig["children_delimiter"] != "\n" {
		t.Fatalf("expected default children delimiter, got %#v", persisted.ParserConfig["children_delimiter"])
	}
	if persisted.ParserConfig["delimiter"] != "\n\n" {
		t.Fatalf("expected ext parser config fields flattened, got %#v", persisted.ParserConfig)
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
	_, code, err := testDatasetUpdateService(t).UpdateDataset("missing-kb", "tenant-1", UpdateDatasetRequest{Name: &name})
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
	_, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-2", UpdateDatasetRequest{Name: &name})
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
	_, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", UpdateDatasetRequest{Name: &name})
	if err == nil {
		t.Fatal("expected name validation error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if err.Error() != "String should have at least 1 character" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetServiceUpdateDatasetRejectsDuplicateName(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
	insertDatasetUpdateKB(t, "kb-2", "tenant-1", "Existing")

	name := "Existing"
	_, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", UpdateDatasetRequest{Name: &name})
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

	_, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", UpdateDatasetRequest{})
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
	result, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", UpdateDatasetRequest{
		Connectors: &[]DatasetConnectorRequest{{ID: "connector-1", AutoParse: autoParse}},
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
	insertDatasetUpdateTenantModel(t, "model-1", "provider-1", "instance-1", "embedding-2", string(entity.ModelTypeEmbedding))

	embeddingModel := "embedding-2@test@ZHIPU-AI"
	result, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", UpdateDatasetRequest{
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

func TestDatasetServiceUpdateDatasetRejectsEmptyConnectorID(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	connectors := []DatasetConnectorRequest{{ID: "  "}}
	_, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", UpdateDatasetRequest{
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

func insertDatasetUpdateTenantModel(t *testing.T, id, providerID, instanceID, modelName, modelType string) {
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
