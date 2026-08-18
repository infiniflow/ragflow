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
		t.Fatalf("extractor enable_metadata should be deleted, got %#v", extractor["enable_metadata"])
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
		"enable_metadata":   false,
		"metadata":          []map[string]interface{}{},
		"built_in_metadata": []map[string]interface{}{},
		"Extractor:AutoExtractDefault": map[string]any{
			"metadata": map[string]any{
				"enabled": false,
				"metadata": []any{
					map[string]any{"key": "stale", "type": "string"},
				},
			},
		},
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

func TestApplyComponentScopedParserConfig_DoesNotTreatDocumentMetadataValuesAsSchema(t *testing.T) {
	parserConfig := entity.JSONMap{
		"metadata": map[string]interface{}{
			"author": "Alice",
		},
		"Extractor:AutoExtractDefault": map[string]any{
			"metadata": map[string]any{
				"enabled": true,
				"metadata": []any{
					map[string]any{"key": "author", "type": "string"},
				},
			},
		},
	}

	got := ApplyComponentScopedParserConfig(parserConfig, "tenant-llm")
	extractor := got["Extractor:AutoExtractDefault"].(map[string]any)

	want := map[string]any{
		"enabled": true,
		"metadata": []any{
			map[string]any{"key": "author", "type": "string"},
		},
		"built_in_metadata": []any{},
	}
	if !reflect.DeepEqual(extractor["metadata"], want) {
		t.Fatalf("extractor metadata = %#v, want %#v", extractor["metadata"], want)
	}
}

func TestApplyComponentScopedParserConfig_FiltersInvalidMetadataEntries(t *testing.T) {
	parserConfig := entity.JSONMap{
		"enable_metadata": true,
		"metadata": []any{
			map[string]any{"key": "valid_custom", "type": "string"},
			map[string]any{"key": "   ", "type": "string"}, // blank key should be filtered
			"invalid_string_entry",                         // non-map should be filtered
			123,                                            // non-map should be filtered
		},
		"built_in_metadata": []any{
			map[string]any{"key": "doc_name", "type": "string"},
			map[string]any{"invalid": "no_key"},
		},
		"Extractor:AutoExtractDefault": map[string]any{
			"metadata": map[string]any{
				"enabled": true,
			},
		},
	}

	got := ApplyComponentScopedParserConfig(parserConfig, "tenant-llm")
	extractor := got["Extractor:AutoExtractDefault"].(map[string]any)
	metaObj := extractor["metadata"].(map[string]any)

	if metaObj["enabled"] != true {
		t.Fatalf("expected enabled=true, got %v", metaObj["enabled"])
	}

	customList, ok := metaObj["metadata"].([]any)
	if !ok || len(customList) != 1 {
		t.Fatalf("expected 1 valid custom field, got %#v", metaObj["metadata"])
	}
	if customList[0].(map[string]any)["key"] != "valid_custom" {
		t.Fatalf("expected key=valid_custom, got %v", customList[0])
	}

	builtInList, ok := metaObj["built_in_metadata"].([]any)
	if !ok || len(builtInList) != 1 {
		t.Fatalf("expected 1 valid built-in field, got %#v", metaObj["built_in_metadata"])
	}
	if builtInList[0].(map[string]any)["key"] != "doc_name" {
		t.Fatalf("expected key=doc_name, got %v", builtInList[0])
	}
}

func TestApplyComponentScopedParserConfig_PreservesExplicitDisabledState(t *testing.T) {
	parserConfig := entity.JSONMap{
		"enable_metadata": true,
		"metadata": []any{
			map[string]any{"key": "author", "type": "string"},
		},
		"built_in_metadata": []any{
			map[string]any{"key": "doc_name", "type": "string"},
		},
		"Extractor:AutoExtractDefault": map[string]any{
			"metadata": map[string]any{
				"enabled": false,
			},
		},
	}

	got := ApplyComponentScopedParserConfig(parserConfig, "tenant-llm")
	extractor := got["Extractor:AutoExtractDefault"].(map[string]any)
	metaObj := extractor["metadata"].(map[string]any)

	if metaObj["enabled"] != false {
		t.Fatalf("expected enabled=false to be strictly preserved on node, got %v", metaObj["enabled"])
	}
	customList, ok := metaObj["metadata"].([]any)
	if !ok || len(customList) != 1 {
		t.Fatalf("expected 1 custom field synced, got %#v", metaObj["metadata"])
	}
}

func TestApplyComponentScopedParserConfig_PreservesNodeConfiguredFieldsWhenTopLevelEmpty(t *testing.T) {
	// Scenario: Frontend configures metadata on Extractor node only (no top-level metadata definition),
	// but preserveDatasetParserConfigMetadata derives top-level enable_metadata = true.
	parserConfig := entity.JSONMap{
		"enable_metadata": true,
		"Extractor:AutoExtractDefault": map[string]any{
			"metadata": map[string]any{
				"enabled": true,
				"metadata": []any{
					map[string]any{"key": "custom_category", "type": "string"},
				},
				"built_in_metadata": []any{
					map[string]any{"key": "doc_name", "type": "string"},
				},
			},
		},
	}

	got := ApplyComponentScopedParserConfig(parserConfig, "tenant-llm")
	extractor := got["Extractor:AutoExtractDefault"].(map[string]any)
	metaObj, ok := extractor["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata object, got %#v", extractor["metadata"])
	}

	if metaObj["enabled"] != true {
		t.Fatalf("expected enabled=true, got %v", metaObj["enabled"])
	}

	customList, ok := metaObj["metadata"].([]any)
	if !ok || len(customList) != 1 {
		t.Fatalf("expected 1 custom field preserved from node, got %#v", metaObj["metadata"])
	}
	if customList[0].(map[string]any)["key"] != "custom_category" {
		t.Fatalf("expected custom field key=custom_category, got %v", customList[0])
	}

	builtInList, ok := metaObj["built_in_metadata"].([]any)
	if !ok || len(builtInList) != 1 {
		t.Fatalf("expected 1 built-in field preserved from node, got %#v", metaObj["built_in_metadata"])
	}
	if builtInList[0].(map[string]any)["key"] != "doc_name" {
		t.Fatalf("expected built-in field key=doc_name, got %v", builtInList[0])
	}
}
