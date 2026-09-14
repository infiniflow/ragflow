package dataset

import (
	"fmt"
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

func TestCreateDataset_SetsExplicitLanguage(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")
	ctx := t.Context()

	language := "  Chinese  "
	result, code, err := testDatasetCreateService(t).CreateDataset(ctx, &service.CreateDatasetRequest{
		Name:     "ds-language",
		Language: &language,
	}, "tenant-1")
	if err != nil {
		t.Fatalf("CreateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if result["language"] != "Chinese" {
		t.Fatalf("language = %#v, want %q", result["language"], "Chinese")
	}
}

func TestCreateDataset_OmittedLanguageKeepsDefault(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")
	ctx := t.Context()

	// An omitted language must stay unset on the insert so the column default
	// applies, mirroring the Python service dropping a None language.
	result, code, err := testDatasetCreateService(t).CreateDataset(ctx, &service.CreateDatasetRequest{Name: "ds-no-language"}, "tenant-1")
	if err != nil {
		t.Fatalf("CreateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}

	var stored entity.Knowledgebase
	if err := db.Where("id = ?", result["id"]).First(&stored).Error; err != nil {
		t.Fatalf("load created dataset: %v", err)
	}
	if stored.Language == nil {
		t.Fatal("expected the column default to be applied, got NULL language")
	}
	if *stored.Language != "English" {
		t.Fatalf("language = %q, want the %q column default", *stored.Language, "English")
	}
}

func TestCreateDataset_RejectsBlankLanguage(t *testing.T) {
	for _, language := range []string{"", "   ", "\t"} {
		t.Run(fmt.Sprintf("%q", language), func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			insertCreateDatasetTenant(t, "tenant-1")

			_, code, err := testDatasetCreateService(t).CreateDataset(t.Context(), &service.CreateDatasetRequest{
				Name:     "ds-blank-language",
				Language: &language,
			}, "tenant-1")
			if err == nil {
				t.Fatal("expected language validation error")
			}
			if code != common.CodeDataError {
				t.Fatalf("expected data error code, got %d", code)
			}
			if err.Error() != "String should have at least 1 character" {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCreateDataset_LanguageLimitCountsCharacters(t *testing.T) {
	// pydantic's max_length counts characters, so a 32-character non-ASCII
	// language name is accepted even though it is 96 bytes long.
	atLimit := strings.Repeat("中", datasetLanguageLimit)
	overLimit := strings.Repeat("中", datasetLanguageLimit+1)

	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")
	ctx := t.Context()

	result, code, err := testDatasetCreateService(t).CreateDataset(ctx, &service.CreateDatasetRequest{
		Name:     "ds-language-at-limit",
		Language: &atLimit,
	}, "tenant-1")
	if err != nil {
		t.Fatalf("CreateDataset failed for a %d-character language: %v", datasetLanguageLimit, err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if result["language"] != atLimit {
		t.Fatalf("language = %#v, want %q", result["language"], atLimit)
	}

	_, code, err = testDatasetCreateService(t).CreateDataset(ctx, &service.CreateDatasetRequest{
		Name:     "ds-language-over-limit",
		Language: &overLimit,
	}, "tenant-1")
	if err == nil {
		t.Fatal("expected language length validation error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if err.Error() != "String should have at most 32 characters" {
		t.Fatalf("unexpected error: %v", err)
	}
}
