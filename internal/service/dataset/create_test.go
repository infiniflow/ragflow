package dataset

import (
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
	ctx := t.Context()

	chunkMethod := "naive"
	parseType := 1
	result, code, err := testDatasetCreateService(t).CreateDataset(ctx, &service.CreateDatasetRequest{
		Name:      "ds-no-cp",
		ParserID:  &chunkMethod,
		ParseType: &parseType,
	}, "tenant-1")
	if err != nil {
		t.Fatalf("CreateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if result["parser_id"] != string(entity.ParserTypeGeneral) {
		t.Fatalf("expected canonical parser_id %q, got %#v", entity.ParserTypeGeneral, result["parser_id"])
	}
}

func TestCreateDataset_ComponentParamsPopulated(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")
	ctx := t.Context()

	chunkMethod := "general"
	parseType := 1
	result, code, err := testDatasetCreateService(t).CreateDataset(ctx, &service.CreateDatasetRequest{
		Name:      "ds-with-cp",
		ParserID:  &chunkMethod,
		ParseType: &parseType,
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
	extractor, ok := parserConfig["Extractor:AutoExtractDefault"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected extractor component params, got %#v", parserConfig["Extractor:AutoExtractDefault"])
	}
	if extractor["llm_id"] != "llm-default" {
		t.Fatalf("extractor llm_id = %#v, want llm-default", extractor["llm_id"])
	}
}

func TestCreateDataset_ParseTypeBuiltinClearsPipelineID(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")
	ctx := t.Context()

	pipelineID := "0123456789abcdef0123456789abcdef"
	parseTypeBuiltin := 1
	chunkMethod := "naive"
	result, code, err := testDatasetCreateService(t).CreateDataset(ctx, &service.CreateDatasetRequest{
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
	if result["parser_id"] != string(entity.ParserTypeGeneral) {
		t.Fatalf("expected canonical parser_id %q, got %#v", entity.ParserTypeGeneral, result["parser_id"])
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
	ctx := t.Context()

	pipelineID := "0123456789abcdef0123456789abcdef"
	parseTypePipeline := 2
	chunkMethod := "naive"
	result, code, err := testDatasetCreateService(t).CreateDataset(ctx, &service.CreateDatasetRequest{
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

func TestCreateDataset_ValidatesName(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")
	ctx := t.Context()

	_, code, err := testDatasetCreateService(t).CreateDataset(ctx, &service.CreateDatasetRequest{Name: "   "}, "tenant-1")
	if err == nil {
		t.Fatal("expected name validation error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if err.Error() != "dataset name can't be empty" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateDataset_DedupesDuplicateName(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")

	if err := db.Create(&entity.Knowledgebase{
		ID:        "kb-1",
		TenantID:  "tenant-1",
		Name:      "Existing",
		ParserID:  "naive",
		CreatedBy: "tenant-1",
		Status:    sptr(string(entity.StatusValid)),
	}).Error; err != nil {
		t.Fatalf("failed to create existing kb: %v", err)
	}

	ctx := t.Context()
	// Mirror Python's duplicate_name: the create appends (1) instead of failing.
	result, code, err := testDatasetCreateService(t).CreateDataset(ctx, &service.CreateDatasetRequest{Name: "Existing"}, "tenant-1")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if result["name"] != "Existing(1)" {
		t.Fatalf("unexpected name: %v", result["name"])
	}
}

func TestCreateDataset_RejectsInvalidEmbeddingModel(t *testing.T) {
	cases := []struct {
		name            string
		embeddingModel  string
		expectedMessage string
	}{
		{"empty", "", "embedding model identifier must follow <model_name>@<provider> format"},
		{"whitespace", " ", "embedding model identifier must follow <model_name>@<provider> format"},
		{"missing_at", "BAAI/bge-small-en-v1.5Builtin", "embedding model identifier must follow <model_name>@<provider> format"},
		{"empty_model_name", "@Builtin", "both model_name and provider must be non-empty strings"},
		{"empty_provider", "BAAI/bge-small-en-v1.5@", "both model_name and provider must be non-empty strings"},
		{"whitespace_model_name", " @Builtin", "both model_name and provider must be non-empty strings"},
		{"whitespace_provider", "BAAI/bge-small-en-v1.5@ ", "both model_name and provider must be non-empty strings"},
	}

	ctx := t.Context()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			insertCreateDatasetTenant(t, "tenant-1")

			_, code, err := testDatasetCreateService(t).CreateDataset(ctx, &service.CreateDatasetRequest{
				Name:           "ds-embd-" + tc.name,
				EmbeddingModel: &tc.embeddingModel,
			}, "tenant-1")
			if err == nil {
				t.Fatal("expected embedding model validation error")
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
