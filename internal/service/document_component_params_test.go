package service

import (
	"encoding/json"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// dslFixture returns a DSL JSON with Parser + Tokenizer components,
// matching the new DSL format (no setups wrapper; file-type keys at
// Parser params top level).
func validDSLFixture(t *testing.T) []byte {
	t.Helper()
	dsl := map[string]any{
		"components": map[string]any{
			"File": map[string]any{
				"obj": map[string]any{
					"component_name": "File",
					"params":         map[string]any{},
				},
			},
			"Parser:HipSignsRhyme": map[string]any{
				"obj": map[string]any{
					"component_name": "Parser",
					"params": map[string]any{
						"outputs": map[string]any{},
						"pdf":     map[string]any{"parse_method": "deepdoc"},
					},
				},
			},
			"Tokenizer:LegalReadersDecide": map[string]any{
				"obj": map[string]any{
					"component_name": "Tokenizer",
					"params": map[string]any{
						"fields":               "text",
						"filename_embd_weight": 0.1,
						"search_method":        []any{"embedding"},
						"outputs":              map[string]any{},
					},
				},
			},
		},
	}
	b, err := json.Marshal(dsl)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return b
}

func TestValidateComponentParams_Valid(t *testing.T) {
	dsl := validDSLFixture(t)
	params := map[string]map[string]any{
		"Parser:HipSignsRhyme": {
			"pdf": map[string]any{"parse_method": "plain_text"},
		},
	}
	if err := ValidateComponentParams(dsl, params); err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestValidateComponentParams_UnknownCpnID(t *testing.T) {
	dsl := validDSLFixture(t)
	params := map[string]map[string]any{
		"NonexistentCpn:BadID": {"pdf": map[string]any{}},
	}
	if err := ValidateComponentParams(dsl, params); err == nil {
		t.Error("expected error for unknown cpnID")
	}
}

func TestValidateComponentParams_UnknownParamKey(t *testing.T) {
	dsl := validDSLFixture(t)
	params := map[string]map[string]any{
		"Tokenizer:LegalReadersDecide": {
			"nonexistent_param": "value",
		},
	}
	if err := ValidateComponentParams(dsl, params); err == nil {
		t.Error("expected error for unknown param key")
	}
}

func TestValidateComponentParams_Empty(t *testing.T) {
	dsl := validDSLFixture(t)
	if err := ValidateComponentParams(dsl, nil); err != nil {
		t.Errorf("nil params should pass: %v", err)
	}
	if err := ValidateComponentParams(dsl, map[string]map[string]any{}); err != nil {
		t.Errorf("empty params should pass: %v", err)
	}
}

func TestValidateComponentParams_MultipleComponents(t *testing.T) {
	dsl := validDSLFixture(t)
	params := map[string]map[string]any{
		"Parser:HipSignsRhyme": {
			"pdf": map[string]any{"parse_method": "plain_text"},
		},
		"Tokenizer:LegalReadersDecide": {
			"fields":               "text",
			"filename_embd_weight": 0.5,
			"search_method":        []any{"embedding", "full_text"},
		},
	}
	if err := ValidateComponentParams(dsl, params); err != nil {
		t.Errorf("expected nil error for valid multi-component params, got: %v", err)
	}
}

func TestValidateComponentParams_InvalidDSL(t *testing.T) {
	params := map[string]map[string]any{
		"Parser:HipSignsRhyme": {"pdf": map[string]any{}},
	}
	if err := ValidateComponentParams([]byte("not json"), params); err == nil {
		t.Error("expected error for invalid DSL JSON")
	}
}

func TestValidateComponentParams_FileComponent_AnyParamRejected(t *testing.T) {
	dsl := validDSLFixture(t)
	// File has empty params, so any param key should be rejected.
	params := map[string]map[string]any{
		"File": {
			"some_key": "value",
		},
	}
	if err := ValidateComponentParams(dsl, params); err == nil {
		t.Error("expected error for File component with unknown param")
	}
}

// seedValidationDB spins up an in-memory sqlite DB, migrates the document and
// user_canvas tables, inserts doc (and canvas when non-nil), and points dao.DB
// at it. Returns the document's dataset id and document id.
func seedValidationDB(t *testing.T, doc *entity.Document, canvas *entity.UserCanvas) (datasetID, docID string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Document{}, &entity.UserCanvas{}); err != nil {
		t.Fatalf("migrate tables: %v", err)
	}
	previousDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = previousDB })
	if err := dao.DB.Create(doc).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}
	if canvas != nil {
		if err := dao.DB.Create(canvas).Error; err != nil {
			t.Fatalf("create canvas: %v", err)
		}
	}
	return doc.KbID, doc.ID
}

func validationDoc(parserID string, pipelineID *string) *entity.Document {
	return &entity.Document{ID: "doc-1", KbID: "kb-1", ParserID: parserID, PipelineID: pipelineID, ParserConfig: entity.JSONMap{}}
}

func canvasWithDSL(dsl map[string]any) *entity.UserCanvas {
	return &entity.UserCanvas{ID: "canvas-1", UserID: "u1", DSL: dsl}
}

func canvasStrPtr(s string) *string { return &s }

func seedCanvasOnly(t *testing.T, canvas *entity.UserCanvas) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Document{}, &entity.UserCanvas{}); err != nil {
		t.Fatalf("migrate tables: %v", err)
	}
	previousDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = previousDB })
	if err := dao.DB.Create(canvas).Error; err != nil {
		t.Fatalf("create canvas: %v", err)
	}
}

func TestValidateComponentParamsForUpdate_ValidBuiltin(t *testing.T) {
	datasetID, docID := seedValidationDB(t, validationDoc("general", nil), nil)
	req := &UpdateDatasetDocumentRequest{
		ParserConfig: map[string]any{
			"Parser:HipSignsRhyme": map[string]any{"pdf": map[string]any{}},
		},
	}
	present := map[string]bool{"parser_config": true}
	if err := (&DocumentService{}).validateComponentParamsForUpdate(datasetID, docID, "u1", req, present); err != nil {
		t.Errorf("expected nil for valid builtin component_params, got %v", err)
	}
}

func TestValidateComponentParamsForUpdate_UnknownCpnID(t *testing.T) {
	datasetID, docID := seedValidationDB(t, validationDoc("general", nil), nil)
	req := &UpdateDatasetDocumentRequest{
		ParserConfig: map[string]any{
			"Nonexistent:BadID": map[string]any{"pdf": map[string]any{}},
		},
	}
	present := map[string]bool{"parser_config": true}
	if err := (&DocumentService{}).validateComponentParamsForUpdate(datasetID, docID, "u1", req, present); err == nil {
		t.Error("expected error for unknown cpnID")
	}
}

func TestValidateComponentParamsForUpdate_UnknownParamKey(t *testing.T) {
	datasetID, docID := seedValidationDB(t, validationDoc("general", nil), nil)
	req := &UpdateDatasetDocumentRequest{
		ParserConfig: map[string]any{
			"Parser:HipSignsRhyme": map[string]any{"nonexistent_param": "x"},
		},
	}
	present := map[string]bool{"parser_config": true}
	if err := (&DocumentService{}).validateComponentParamsForUpdate(datasetID, docID, "u1", req, present); err == nil {
		t.Error("expected error for unknown param key")
	}
}

func TestValidateComponentParamsForUpdate_ParserIDSwitch(t *testing.T) {
	// Request switches the parser_id; the doc's stored parser is "general".
	datasetID, docID := seedValidationDB(t, validationDoc("general", nil), nil)
	newPID := "qa"
	req := &UpdateDatasetDocumentRequest{
		ParserID: &newPID,
		ParserConfig: map[string]any{
			"Parser:HipSignsRhyme": map[string]any{"pdf": map[string]any{}},
		},
	}
	present := map[string]bool{"parser_config": true, "parser_id": true}
	if err := (&DocumentService{}).validateComponentParamsForUpdate(datasetID, docID, "u1", req, present); err != nil {
		t.Errorf("expected nil for valid parser switch, got %v", err)
	}
}

// validateComponentParamsAgainstPipeline tests below use the lower-level helper
// to validate component_params against a DSL directly (without document lookup).

func TestValidateComponentParamsAgainstPipeline_BuiltinValid(t *testing.T) {
	params := map[string]map[string]any{
		"Parser:HipSignsRhyme": {"pdf": map[string]any{"parse_method": "plain_text"}},
	}
	if err := validateComponentParamsAgainstPipeline(false, "general", params); err != nil {
		t.Errorf("expected nil for valid builtin params, got %v", err)
	}
}

func TestValidateComponentParamsAgainstPipeline_BuiltinUnknownCpn(t *testing.T) {
	params := map[string]map[string]any{
		"Nonexistent:BadID": {"pdf": map[string]any{}},
	}
	if err := validateComponentParamsAgainstPipeline(false, "general", params); err == nil {
		t.Error("expected error for unknown cpnID against builtin")
	}
}

func TestValidateComponentParamsAgainstPipeline_CanvasValid(t *testing.T) {
	dslMap := map[string]any{
		"components": map[string]any{
			"Parser:CustomRhyme": map[string]any{
				"obj": map[string]any{
					"component_name": "Parser",
					"params": map[string]any{
						"outputs": map[string]any{},
						"pdf":     map[string]any{"parse_method": "deepdoc"},
					},
				},
			},
		},
	}
	seedCanvasOnly(t, canvasWithDSL(dslMap))
	params := map[string]map[string]any{
		"Parser:CustomRhyme": {"pdf": map[string]any{"parse_method": "plain_text"}},
	}
	if err := validateComponentParamsAgainstPipeline(true, "canvas-1", params); err != nil {
		t.Errorf("expected nil for valid canvas params, got %v", err)
	}
}

func TestValidateComponentParamsAgainstPipeline_CanvasUnknownCpn(t *testing.T) {
	dslMap := map[string]any{
		"components": map[string]any{
			"Parser:CustomRhyme": map[string]any{
				"obj": map[string]any{
					"component_name": "Parser",
					"params": map[string]any{
						"outputs": map[string]any{},
						"pdf":     map[string]any{"parse_method": "deepdoc"},
					},
				},
			},
		},
	}
	seedCanvasOnly(t, canvasWithDSL(dslMap))
	params := map[string]map[string]any{
		"Parser:WrongCpn": {"pdf": map[string]any{}},
	}
	if err := validateComponentParamsAgainstPipeline(true, "canvas-1", params); err == nil {
		t.Error("expected error for unknown cpnID in canvas")
	}
}
