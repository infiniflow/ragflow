package dataset

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/service"
)

// parse_type is now a required field on dataset creation. These tests lock in
// that contract: a create request must explicitly select BuiltIn (1) or
// Pipeline (2) mode, and must supply the matching identifier.

func TestCreateDataset_RequiresParseType(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")

	// No parse_type, no parser_id, no pipeline_id. The previous code silently
	// defaulted to BuiltIn; now this must be rejected.
	_, code, err := testDatasetCreateService(t).CreateDataset(t.Context(), &service.CreateDatasetRequest{
		Name: "ds-no-parse-type",
	}, testDatasetSubject("tenant-1"))
	if err == nil {
		t.Fatal("expected parse_type required error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if err.Error() != "parse_type is required" {
		t.Fatalf("unexpected error: got %q, want %q", err.Error(), "parse_type is required")
	}
}

func TestCreateDataset_BuiltinRequiresParserID(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")

	parseType := 1
	_, code, err := testDatasetCreateService(t).CreateDataset(t.Context(), &service.CreateDatasetRequest{
		Name:      "ds-builtin-no-parser",
		ParseType: &parseType,
	}, testDatasetSubject("tenant-1"))
	if err == nil {
		t.Fatal("expected parser_id required error for BuiltIn mode")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if err.Error() != "parser_id is required when parse_type is BuiltIn" {
		t.Fatalf("unexpected error: got %q", err.Error())
	}
}

func TestCreateDataset_PipelineRequiresPipelineID(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")

	parseType := 2
	_, code, err := testDatasetCreateService(t).CreateDataset(t.Context(), &service.CreateDatasetRequest{
		Name:      "ds-pipeline-no-id",
		ParseType: &parseType,
	}, testDatasetSubject("tenant-1"))
	if err == nil {
		t.Fatal("expected pipeline_id required error for Pipeline mode")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if err.Error() != "pipeline_id is required when parse_type is Pipeline" {
		t.Fatalf("unexpected error: got %q", err.Error())
	}
}

func TestCreateDataset_RejectsInvalidParseType(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")

	parseType := 3
	_, code, err := testDatasetCreateService(t).CreateDataset(t.Context(), &service.CreateDatasetRequest{
		Name:      "ds-bad-parse-type",
		ParseType: &parseType,
	}, testDatasetSubject("tenant-1"))
	if err == nil {
		t.Fatal("expected invalid parse_type error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if err.Error() != "invalid parse_type: 3 (must be 1 or 2)" {
		t.Fatalf("unexpected error: got %q", err.Error())
	}
}

func TestCreateDataset_BuiltinSucceedsWithParseType(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")

	parseType := 1
	parserID := "general"
	result, code, err := testDatasetCreateService(t).CreateDataset(t.Context(), &service.CreateDatasetRequest{
		Name:      "ds-builtin-ok",
		ParseType: &parseType,
		ParserID:  &parserID,
	}, testDatasetSubject("tenant-1"))
	if err != nil {
		t.Fatalf("CreateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if result["parser_id"] != "general" {
		t.Fatalf("expected parser_id general, got %#v", result["parser_id"])
	}
}
