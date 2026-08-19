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
