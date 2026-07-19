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
)

// testDatasetCreateService builds a DatasetService for CreateDataset tests.
// CreateDataset resolves the tenant via d.tenantDAO, so it must be wired here
// (unlike the update path which does not touch it).
// insertCreateDatasetTenant seeds a tenant with status="1" (required by
// TenantDAO.GetByID, which CreateDataset calls) for the given tenant id.
func insertCreateDatasetTenant(t *testing.T, tenantID string) {
	t.Helper()
	var existing entity.Tenant
	if err := dao.DB.Where("id = ?", tenantID).First(&existing).Error; err != nil {
		tn := &entity.Tenant{
			ID:           tenantID,
			LLMID:        "llm-default",
			EmbdID:       "embd-default",
			TenantEmbdID: sptr("embd-1"),
			ASRID:        "asr-default",
			Status:       sptr("1"),
		}
		if err := dao.DB.Create(tn).Error; err != nil {
			t.Fatalf("insert test tenant: %v", err)
		}
	}
}

func testDatasetCreateService(t *testing.T) *DatasetService {
	t.Helper()

	return &DatasetService{
		kbDAO:        dao.NewKnowledgebaseDAO(),
		documentDAO:  dao.NewDocumentDAO(),
		connectorDAO: dao.NewConnectorDAO(),
		tenantDAO:    dao.NewTenantDAO(),
	}
}

func TestCreateDataset_NoComponentParams(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")

	chunkMethod := "naive"
	result, code, err := testDatasetCreateService(t).CreateDataset(&CreateDatasetRequest{
		Name:     "ds-no-cp",
		ParserID: &chunkMethod,
	}, "tenant-1")
	if err != nil {
		t.Fatalf("CreateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if result["parser_id"] != strings.TrimSpace(chunkMethod) {
		t.Fatalf("expected parser_id %q, got %#v", chunkMethod, result["parser_id"])
	}
}

func TestCreateDataset_ComponentParamsPopulated(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")

	chunkMethod := "naive"
	result, code, err := testDatasetCreateService(t).CreateDataset(&CreateDatasetRequest{
		Name:     "ds-with-cp-defaults",
		ParserID: &chunkMethod,
	}, "tenant-1")
	if err != nil {
		t.Fatalf("CreateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}

	parserConfig, ok := result["parser_config"].(entity.JSONMap)
	if !ok {
		t.Fatalf("parser_config is not entity.JSONMap, got %T", result["parser_config"])
	}
	if len(parserConfig) == 0 {
		t.Fatal("parser_config is empty, expected DSL component params defaults")
	}

	// Verify at least one component from the general/naive template has defaults.
	// The general template has TokenChunker, Tokenizer, Parser, and File components.
	found := false
	for cpnID := range parserConfig {
		if strings.Contains(cpnID, "TokenChunker") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("parser_config does not contain any TokenChunker: %v", parserConfig)
	}
}

func TestCreateDataset_ParseTypeBuiltinClearsPipelineID(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")
	// Seed a canvas so it exists, but parse_type=1 should ignore it.
	seedDatasetUpdateCanvas(t, "abcdef0123456789abcdef0123456789", "tenant-1",
		datasetUpdateCanvasDSL("Parser:HipSignsRhyme", "chunk_token_num"))

	chunkMethod := "naive"
	pipelineID := "ABCDEF0123456789ABCDEF0123456789"
	parseType := 1

	result, code, err := testDatasetCreateService(t).CreateDataset(&CreateDatasetRequest{
		Name:       "ds-builtin-clears-pipeline",
		ParserID:   &chunkMethod,
		PipelineID: &pipelineID,
		ParseType:  &parseType,
	}, "tenant-1")
	if err != nil {
		t.Fatalf("CreateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	// parse_type=1 clears pipeline_id → only parser_id should be persisted.
	if result["parser_id"] != chunkMethod {
		t.Fatalf("expected parser_id %q, got %#v", chunkMethod, result["parser_id"])
	}
	if pid, ok := result["pipeline_id"]; ok && pid != nil && pid != "" {
		t.Fatalf("expected pipeline_id to be cleared for BuiltIn mode, got %#v", pid)
	}
}

func TestCreateDataset_ParseTypePipelineIgnoresParserID(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")
	seedDatasetUpdateCanvas(t, "abcdef0123456789abcdef0123456789", "tenant-1",
		datasetUpdateCanvasDSL("Parser:CustomP", "chunk_token_num"))

	chunkMethod := "book"
	pipelineID := "ABCDEF0123456789ABCDEF0123456789"
	parseType := 2

	result, code, err := testDatasetCreateService(t).CreateDataset(&CreateDatasetRequest{
		Name:       "ds-pipeline-ignores-parser",
		ParserID:   &chunkMethod,
		PipelineID: &pipelineID,
		ParseType:  &parseType,
	}, "tenant-1")
	if err != nil {
		t.Fatalf("CreateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	// parse_type=2 ignores parser_id → pipeline_id should be persisted;
	// parser_id should fall back to the default ("naive") since it wasn't set.
	if result["pipeline_id"] != strings.ToLower(pipelineID) {
		t.Fatalf("expected pipeline_id %q, got %#v", strings.ToLower(pipelineID), result["pipeline_id"])
	}
}

func TestCreateDataset_RejectsBothWithoutParseType(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")

	chunkMethod := "naive"
	pipelineID := "abcdef0123456789abcdef0123456789"
	_, code, err := testDatasetCreateService(t).CreateDataset(&CreateDatasetRequest{
		Name:       "ds-both-no-parse-type",
		ParserID:   &chunkMethod,
		PipelineID: &pipelineID,
		// ParseType deliberately nil
	}, "tenant-1")
	if err == nil {
		t.Fatal("expected mutual-exclusivity error when both set without parse_type")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected error to mention 'mutually exclusive', got: %v", err)
	}
}
