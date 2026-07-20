package dataset

import (
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

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
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")

	chunkMethod := "naive"
	result, code, err := testDatasetCreateService(t).CreateDataset(&service.CreateDatasetRequest{
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
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")

	chunkMethod := "general"
	result, code, err := testDatasetCreateService(t).CreateDataset(&service.CreateDatasetRequest{
		Name:     "ds-with-cp",
		ParserID: &chunkMethod,
	}, "tenant-1")
	if err != nil {
		t.Fatalf("CreateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	parserConfig, ok := result["parser_config"].(entity.JSONMap)
	if !ok || len(parserConfig) == 0 {
		t.Fatal("expected non-empty parser_config for general pipeline")
	}
}

func TestCreateDataset_ParseTypeBuiltinClearsPipelineID(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")

	pipelineID := "0123456789abcdef0123456789abcdef"
	parseTypeBuiltin := 1
	chunkMethod := "naive"
	result, code, err := testDatasetCreateService(t).CreateDataset(&service.CreateDatasetRequest{
		Name:       "ds-parse-builtin",
		ParserID:   &chunkMethod,
		PipelineID: &pipelineID,
		ParseType:  &parseTypeBuiltin,
	}, "tenant-1")
	if err != nil {
		t.Fatalf("CreateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if result["parser_id"] != chunkMethod {
		t.Fatalf("expected parser_id %q, got %#v", chunkMethod, result["parser_id"])
	}
	if v, ok := result["pipeline_id"]; ok && v != nil {
		t.Fatalf("expected pipeline_id to be nil for BuiltIn mode, got %#v", v)
	}
}

func TestCreateDataset_ParseTypePipelineIgnoresParserID(t *testing.T) {
	t.Skip("requires canvas seed data in test DB")
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")

	pipelineID := "0123456789abcdef0123456789abcdef"
	parseTypePipeline := 2
	chunkMethod := "naive"
	result, code, err := testDatasetCreateService(t).CreateDataset(&service.CreateDatasetRequest{
		Name:       "ds-parse-pipeline",
		ParserID:   &chunkMethod,
		PipelineID: &pipelineID,
		ParseType:  &parseTypePipeline,
	}, "tenant-1")
	if err != nil {
		t.Fatalf("CreateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if v, ok := result["parser_id"]; !ok || v == nil {
	} else {
		t.Fatalf("expected parser_id to be empty for Pipeline mode, got %#v", v)
	}
}

func TestCreateDataset_RejectsBothWithoutParseType(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")

	pipelineID := "0123456789abcdef0123456789abcdef"
	chunkMethod := "naive"
	_, code, err := testDatasetCreateService(t).CreateDataset(&service.CreateDatasetRequest{
		Name:       "ds-both",
		ParserID:   &chunkMethod,
		PipelineID: &pipelineID,
	}, "tenant-1")
	if err == nil {
		t.Fatal("expected error when both parser_id and pipeline_id are provided without parse_type")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected CodeDataError, got %d", code)
	}
}
