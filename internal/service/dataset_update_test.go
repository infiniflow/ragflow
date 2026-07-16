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
	"encoding/json"
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
	insertDatasetUpdateCanvas(t, "abcdef0123456789abcdef0123456789", "tenant-1")

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
		ParserID:       &chunkMethod,
		EmbeddingModel: &embeddingModel,
		PipelineID:     &pipelineID,
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
	// parser_config stores DSL runtime component params directly.
	if _, ok := persisted.ParserConfig["Parser:HipSignsRhyme"].(map[string]interface{}); !ok {
		t.Fatalf("expected Parser:HipSignsRhyme in parser_config, got %#v", persisted.ParserConfig)
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
	insertDatasetUpdateTenantModel(t, "model-1", "provider-1", "instance-1", "embedding-2", int(entity.ModelTypeEmbedding))

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

func TestDatasetServiceUpdateDatasetAcceptsEmbeddingModelID(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
	insertDatasetUpdateModelProvider(t, "provider-1", "tenant-1", "ZHIPU-AI")
	insertDatasetUpdateModelInstance(t, "instance-1", "provider-1", "test")
	insertDatasetUpdateTenantModel(t, "model-1", "provider-1", "instance-1", "embedding-2", int(entity.ModelTypeEmbedding))

	embeddingModelID := "model-1"
	result, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", UpdateDatasetRequest{
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

// --- KB-level component_params validation (mirrors document-level) ---

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

func TestValidateDatasetComponentParams_BuiltinValid(t *testing.T) {
	raw := map[string]any{
		"Parser:HipSignsRhyme": map[string]any{"pdf": map[string]any{"parse_method": "deepdoc"}},
	}
	if err := validateDatasetComponentParams("general", nil, raw, "u"); err != nil {
		t.Fatalf("expected valid builtin, got %v", err)
	}
}

func TestValidateDatasetComponentParams_BuiltinUnknownCpn(t *testing.T) {
	raw := map[string]any{"Parser:NoSuch": map[string]any{"pdf": map[string]any{}}}
	if err := validateDatasetComponentParams("general", nil, raw, "u"); err == nil {
		t.Fatal("expected error for unknown cpn in builtin")
	}
}

func TestValidateDatasetComponentParams_BuiltinUnknownParam(t *testing.T) {
	raw := map[string]any{"Parser:HipSignsRhyme": map[string]any{"no_such_param": 1}}
	if err := validateDatasetComponentParams("general", nil, raw, "u"); err == nil {
		t.Fatal("expected error for unknown param in builtin")
	}
}

func TestValidateDatasetComponentParams_CanvasValid(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	dsl := datasetUpdateCanvasDSL("Parser:CustomRhyme", "pdf")
	seedDatasetUpdateCanvas(t, "canvas-1", "u", dsl)
	raw := map[string]any{"Parser:CustomRhyme": map[string]any{"pdf": map[string]any{}}}
	pid := "canvas-1"
	if err := validateDatasetComponentParams("general", &pid, raw, "u"); err != nil {
		t.Fatalf("expected valid canvas, got %v", err)
	}
}

func TestValidateDatasetComponentParams_CanvasUnknownCpn(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	dsl := datasetUpdateCanvasDSL("Parser:CustomRhyme", "pdf")
	seedDatasetUpdateCanvas(t, "canvas-1", "u", dsl)
	raw := map[string]any{"Parser:NoSuch": map[string]any{"pdf": map[string]any{}}}
	pid := "canvas-1"
	if err := validateDatasetComponentParams("general", &pid, raw, "u"); err == nil {
		t.Fatal("expected error for unknown cpn in canvas")
	}
}

func TestValidateDatasetComponentParams_Malformed(t *testing.T) {
	// A non-object top level must be rejected by normalization.
	if err := validateDatasetComponentParams("general", nil, "not-an-object", "u"); err == nil {
		t.Fatal("expected error for malformed component_params")
	}
}

func TestValidateDatasetComponentParams_None(t *testing.T) {
	// No component_params at all is always valid.
	if err := validateDatasetComponentParams("general", nil, nil, "u"); err != nil {
		t.Fatalf("expected nil raw to be valid, got %v", err)
	}
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

func TestUpdateDataset_RejectsUnknownParam_Builtin(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	_, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", UpdateDatasetRequest{
		ParserConfig: map[string]interface{}{
			"Parser:HipSignsRhyme": map[string]interface{}{
				"no_such_param": 1,
			},
		},
	})
	if err == nil {
		t.Fatal("expected unknown param error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
}

func TestUpdateDataset_AcceptsValidComponentParams_Builtin(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	result, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", UpdateDatasetRequest{
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

func TestUpdateDataset_RejectsCanvasUnknownParam(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	dsl := datasetUpdateCanvasDSL("Parser:CustomRhyme", "pdf")
	seedDatasetUpdateCanvas(t, "canvas-1", "tenant-1", dsl)
	insertDatasetUpdateCanvasKB(t, "kb-1", "tenant-1", "Original", "canvas-1")

	_, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", UpdateDatasetRequest{
		ParserConfig: map[string]interface{}{
			"Parser:NoSuch": map[string]interface{}{
				"pdf": map[string]interface{}{},
			},
		},
	})
	if err == nil {
		t.Fatal("expected unknown cpn error for canvas")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
}

func TestUpdateDataset_AcceptsValidCanvasComponentParams(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	dsl := datasetUpdateCanvasDSL("Parser:CustomRhyme", "pdf")
	seedDatasetUpdateCanvas(t, "canvas-1", "tenant-1", dsl)
	insertDatasetUpdateCanvasKB(t, "kb-1", "tenant-1", "Original", "canvas-1")

	result, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", UpdateDatasetRequest{
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
	_, code, err := testDatasetUpdateService(t).UpdateDataset("kb-1", "tenant-1", UpdateDatasetRequest{
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
