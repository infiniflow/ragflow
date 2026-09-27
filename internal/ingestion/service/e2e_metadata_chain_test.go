package service

import (
	"context"
	"testing"

	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/service"
)

// TestE2E_EnabledFalseDoesNotWriteBuiltIn verifies the full chain's gate:
// dataset metadata enabled=false -> pipeline AutoMetadataEnabled=false -> doc_state does not write built_in.
func TestE2E_EnabledFalseDoesNotWriteBuiltIn(t *testing.T) {
	// Directly exercise the doc_state gate which is the last step of the chain.
	// The earlier steps (dataset UpdateMetadataConfig -> ApplyComponentScoped -> pipeline builtInMetadataFromParserConfig)
	// are covered by unit tests; this E2E asserts the gate is not bypassed.
	svc := &stubDocStateSvc{metaData: map[string]any{"existing": "keep"}}
	u := &docStateUpdater{docSvc: svc}
	ctx := t.Context()
	u.apply(ctx, &taskpkg.PipelineResult{
		DocID:                 "doc-e2e",
		DocName:               "a.pdf",
		AutoMetadataEnabled:   false,
		BuiltInMetadataConfig: []any{map[string]any{"key": "file_name", "type": "string"}, map[string]any{"key": "update_time", "type": "time"}},
		Metadata:              map[string]any{"author": "Bob"},
		ChunkCount:            1,
	})
	if _, ok := svc.metaData["file_name"]; ok {
		t.Fatalf("must not write file_name when enabled=false, got %v", svc.metaData)
	}
	if svc.metaData["author"] != "Bob" {
		t.Fatalf("custom metadata should still be written, got %v", svc.metaData)
	}
	if svc.metaData["existing"] != "keep" {
		t.Fatalf("existing must be kept, got %v", svc.metaData)
	}
}

func TestE2E_ApplyComponentScopedThenPipelineThenDocState(t *testing.T) {
	// ParserConfig as it would be after DatasetService.UpdateMetadataConfig
	parserConfig := entity.JSONMap{
		"metadata": map[string]any{
			"enabled":           true,
			"metadata":          []any{map[string]any{"key": "author", "type": "string"}},
			"built_in_metadata": []any{map[string]any{"key": "file_name", "type": "string"}, map[string]any{"key": "update_time", "type": "time"}},
		},
		"Extractor:AutoExtractDefault": map[string]any{},
	}
	// Scope to node
	got := service.ApplyComponentScopedParserConfig(parserConfig, "llm-1")
	node, _ := got["Extractor:AutoExtractDefault"].(map[string]any)
	meta, _ := node["metadata"].(map[string]any)
	if enabled, _ := meta["enabled"].(bool); !enabled {
		t.Fatalf("scoped enabled must be true, got %v", meta["enabled"])
	}
	if _, ok := meta["built_in_metadata"]; !ok {
		t.Fatalf("scoped built_in missing, got %v", meta)
	}
	// Pipeline would read built_in -> doc_state would write it
	svc := &stubDocStateSvc{metaData: map[string]any{}}
	u := &docStateUpdater{docSvc: svc}
	u.apply(context.Background(), &taskpkg.PipelineResult{
		DocID: "doc-1", KbID: "kb-1", DocName: "report.pdf",
		AutoMetadataEnabled:   true,
		BuiltInMetadataConfig: []any{map[string]any{"key": "file_name", "type": "string"}},
		Metadata:              map[string]any{"author": "Alice"},
		ChunkCount:            1,
	})
	if svc.metaData["file_name"] != "report.pdf" {
		t.Fatalf("file_name not written, got %v", svc.metaData)
	}
}
