package service

import (
	"reflect"
	"testing"

	"ragflow/internal/entity"
)

func TestApplyComponentScopedParserConfig_SyncsExtractorAndCompiler(t *testing.T) {
	parserConfig := entity.JSONMap{
		"enable_metadata": true,
		"metadata": []any{
			map[string]any{"key": "author", "type": "string"},
		},
		"built_in_metadata": []any{
			map[string]any{"key": "document_name", "type": "string"},
		},
		"Extractor:AutoExtractDefault": map[string]any{},
		"Compiler:KnownSwiftLions":     map[string]any{},
	}

	got := ApplyComponentScopedParserConfig(
		parserConfig,
		"llm-default",
	)

	extractor := got["Extractor:AutoExtractDefault"].(map[string]any)
	if extractor["llm_id"] != "llm-default" {
		t.Fatalf("extractor llm_id = %#v, want llm-default", extractor["llm_id"])
	}
	if _, ok := extractor["enable_metadata"]; ok {
		t.Fatalf("extractor enable_metadata should not be present, got %#v", extractor["enable_metadata"])
	}
	wantMetadata := map[string]any{
		"enabled": true,
		"metadata": []any{
			map[string]any{"key": "author", "type": "string"},
		},
		"built_in_metadata": []any{
			map[string]any{"key": "document_name", "type": "string"},
		},
	}
	if !reflect.DeepEqual(extractor["metadata"], wantMetadata) {
		t.Fatalf("extractor metadata = %#v, want %#v", extractor["metadata"], wantMetadata)
	}

	compiler := got["Compiler:KnownSwiftLions"].(map[string]any)
	if compiler["llm_id"] != "llm-default" {
		t.Fatalf("compiler llm_id = %#v, want llm-default", compiler["llm_id"])
	}
	if _, ok := compiler["embedding_model"]; ok {
		t.Fatalf("compiler embedding_model = %#v, want absent", compiler["embedding_model"])
	}
	if _, ok := compiler["tenant_id"]; ok {
		t.Fatalf("compiler tenant_id = %#v, want absent", compiler["tenant_id"])
	}
	if _, ok := compiler["dataset_id"]; ok {
		t.Fatalf("compiler dataset_id = %#v, want absent", compiler["dataset_id"])
	}
}

func TestApplyComponentScopedParserConfig_PreservesExplicitExtractorLLMID(t *testing.T) {
	parserConfig := entity.JSONMap{
		"Extractor:Custom": map[string]any{
			"llm_id": "custom-llm",
		},
	}

	got := ApplyComponentScopedParserConfig(parserConfig, "tenant-llm")
	extractor := got["Extractor:Custom"].(map[string]any)
	if extractor["llm_id"] != "custom-llm" {
		t.Fatalf("extractor llm_id = %#v, want custom-llm", extractor["llm_id"])
	}
}

func TestApplyComponentScopedParserConfig_AcceptsTypedMetadataSlices(t *testing.T) {
	parserConfig := entity.JSONMap{
		"enable_metadata": true,
		"metadata": []map[string]interface{}{
			{"key": "author", "type": "string"},
		},
		"built_in_metadata": []map[string]interface{}{
			{"key": "document_name", "type": "string"},
		},
		"Extractor:AutoExtractDefault": map[string]any{},
	}

	got := ApplyComponentScopedParserConfig(parserConfig, "tenant-llm")
	extractor := got["Extractor:AutoExtractDefault"].(map[string]any)

	wantMetadata := map[string]any{
		"enabled": true,
		"metadata": []any{
			map[string]interface{}{"key": "author", "type": "string"},
		},
		"built_in_metadata": []any{
			map[string]interface{}{"key": "document_name", "type": "string"},
		},
	}
	if !reflect.DeepEqual(extractor["metadata"], wantMetadata) {
		t.Fatalf("extractor metadata = %#v, want %#v", extractor["metadata"], wantMetadata)
	}
}

func TestApplyComponentScopedParserConfig_ClearsExtractorMetadataWhenDisabled(t *testing.T) {
	parserConfig := entity.JSONMap{
		"enable_metadata": false,
		"metadata":        []map[string]interface{}{},
		"built_in_metadata": []map[string]interface{}{
			{"key": "document_name", "type": "string"},
		},
		"Extractor:AutoExtractDefault": map[string]any{
			"enable_metadata": 1,
			"metadata": map[string]any{
				"enabled": true,
				"metadata": []any{
					map[string]any{"key": "stale", "type": "string"},
				},
			},
		},
	}

	got := ApplyComponentScopedParserConfig(parserConfig, "tenant-llm")
	extractor := got["Extractor:AutoExtractDefault"].(map[string]any)

	if _, ok := extractor["enable_metadata"]; ok {
		t.Fatalf("extractor enable_metadata should not be present, got %#v", extractor["enable_metadata"])
	}
	wantMetadata := map[string]any{
		"enabled":  false,
		"metadata": []any{},
		"built_in_metadata": []any{
			map[string]interface{}{"key": "document_name", "type": "string"},
		},
	}
	if !reflect.DeepEqual(extractor["metadata"], wantMetadata) {
		t.Fatalf("extractor metadata = %#v, want %#v", extractor["metadata"], wantMetadata)
	}
}

func TestApplyComponentScopedParserConfig_DoesNotTreatDocumentMetadataValuesAsSchema(t *testing.T) {
	parserConfig := entity.JSONMap{
		"metadata": map[string]interface{}{
			"author": "Alice",
		},
		"Extractor:AutoExtractDefault": map[string]any{},
	}

	got := ApplyComponentScopedParserConfig(parserConfig, "tenant-llm")
	extractor := got["Extractor:AutoExtractDefault"].(map[string]any)

	wantMetadata := map[string]any{
		"enabled":           false,
		"metadata":          []any{},
		"built_in_metadata": []any{},
	}
	if !reflect.DeepEqual(extractor["metadata"], wantMetadata) {
		t.Fatalf("extractor metadata = %#v, want %#v", extractor["metadata"], wantMetadata)
	}
}

func TestMergeMetadataFields_MergesFieldsAndBuiltIn(t *testing.T) {
	parserConfig := entity.JSONMap{
		"metadata": []any{
			map[string]any{"key": "author", "type": "string"},
		},
		"built_in_metadata": []any{
			map[string]any{"key": "document_name", "type": "string"},
		},
	}

	got := mergeMetadataFields(parserConfig)
	want := []any{
		map[string]any{"key": "author", "type": "string"},
		map[string]any{"key": "document_name", "type": "string"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeMetadataFields = %#v, want %#v", got, want)
	}
}

func TestApplyComponentScopedParserConfig_PreservesNodeOnlyMetadataOnDatasetSave(t *testing.T) {
	parserConfig := entity.JSONMap{
		"Extractor:CanvasNode": map[string]any{
			"llm_id": "model-1",
			"metadata": map[string]any{
				"enabled": true,
				"metadata": []any{
					map[string]any{"key": "node_author", "type": "string"},
				},
				"built_in_metadata": []any{
					map[string]any{"key": "node_doc_title", "type": "string"},
				},
			},
		},
	}

	got := ApplyComponentScopedParserConfig(parserConfig, "tenant-llm")
	node := got["Extractor:CanvasNode"].(map[string]any)

	meta, ok := node["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected node metadata map, got %T: %#v", node["metadata"], node["metadata"])
	}
	if enabled, _ := meta["enabled"].(bool); !enabled {
		t.Fatalf("expected node metadata.enabled = true, got false")
	}

	fields, ok := meta["metadata"].([]any)
	if !ok || len(fields) != 1 {
		t.Fatalf("expected 1 custom field preserved, got %#v", meta["metadata"])
	}
	if fields[0].(map[string]any)["key"] != "node_author" {
		t.Fatalf("expected node_author preserved, got %#v", fields[0])
	}
	builtIn, ok := meta["built_in_metadata"].([]any)
	if !ok || len(builtIn) != 1 {
		t.Fatalf("expected 1 built-in field preserved, got %#v", meta["built_in_metadata"])
	}
	if builtIn[0].(map[string]any)["key"] != "node_doc_title" {
		t.Fatalf("expected node_doc_title preserved, got %#v", builtIn[0])
	}
}

func TestApplyComponentScopedParserConfig_PreservesNodeOnlyMetadataConfigShape(t *testing.T) {
	parserConfig := entity.JSONMap{
		"enable_metadata": false,
		"Extractor:CanvasNode": map[string]any{
			"llm_id": "model-1",
			"metadata_config": map[string]any{
				"enabled": true,
				"metadata": []any{
					map[string]any{"key": "custom_field", "type": "string"},
				},
			},
		},
	}

	got := ApplyComponentScopedParserConfig(parserConfig, "tenant-llm")
	node := got["Extractor:CanvasNode"].(map[string]any)

	if _, ok := node["metadata_config"]; ok {
		t.Fatalf("metadata_config should be deleted from node, got %#v", node["metadata_config"])
	}
	meta, ok := node["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected node metadata map, got %T: %#v", node["metadata"], node["metadata"])
	}
	if enabled, _ := meta["enabled"].(bool); !enabled {
		t.Fatalf("expected node metadata.enabled = true, got false")
	}
	fields, ok := meta["metadata"].([]any)
	if !ok || len(fields) != 1 {
		t.Fatalf("expected 1 custom field preserved, got %#v", meta["metadata"])
	}
	if fields[0].(map[string]any)["key"] != "custom_field" {
		t.Fatalf("expected custom_field preserved, got %#v", fields[0])
	}
}
