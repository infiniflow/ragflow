package service

import (
	"reflect"
	"testing"

	"ragflow/internal/entity"
)

func TestApplyComponentScopedParserConfig_ScopesDatasetMetadata(t *testing.T) {
	parserConfig := entity.JSONMap{
		"metadata": map[string]any{
			"enabled": true,
			"metadata": []any{
				map[string]any{"key": "author", "type": "string"},
			},
			"built_in_metadata": []any{
				map[string]any{"key": "document_name", "type": "string"},
			},
		},
		"Extractor:AutoExtractDefault": map[string]any{},
		"Compiler:KnownSwiftLions":     map[string]any{},
	}

	got := ApplyComponentScopedParserConfig(parserConfig, "llm-default")

	extractor := got["Extractor:AutoExtractDefault"].(map[string]any)
	if extractor["llm_id"] != "llm-default" {
		t.Fatalf("extractor llm_id = %#v, want llm-default", extractor["llm_id"])
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
}

func TestApplyComponentScopedParserConfig_StripsLegacyFlatFields(t *testing.T) {
	parserConfig := entity.JSONMap{
		"enable_metadata": true,
		"built_in_metadata": []any{
			map[string]any{"key": "document_name", "type": "string"},
		},
		"fields": []any{
			map[string]any{"key": "author", "type": "string"},
		},
		"Extractor:AutoExtractDefault": map[string]any{
			"enable_metadata":   1,
			"metadata_config":   map[string]any{"enabled": true},
			"built_in_metadata": []any{map[string]any{"key": "stale", "type": "string"}},
			"fields":            []any{map[string]any{"key": "stale", "type": "string"}},
		},
	}

	got := ApplyComponentScopedParserConfig(parserConfig, "tenant-llm")
	for _, key := range []string{"enable_metadata", "built_in_metadata", "fields", "metadata_config"} {
		if _, ok := got[key]; ok {
			t.Fatalf("top-level %s should be stripped, got %#v", key, got[key])
		}
	}
	extractor := got["Extractor:AutoExtractDefault"].(map[string]any)
	for _, key := range []string{"enable_metadata", "built_in_metadata", "fields", "metadata_config"} {
		if _, ok := extractor[key]; ok {
			t.Fatalf("node %s should be stripped, got %#v", key, extractor[key])
		}
	}
	wantMetadata := map[string]any{
		"enabled":           false,
		"metadata":          []any{},
		"built_in_metadata": []any{},
	}
	if !reflect.DeepEqual(extractor["metadata"], wantMetadata) {
		t.Fatalf("extractor metadata = %#v, want %#v", extractor["metadata"], wantMetadata)
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

func TestApplyComponentScopedParserConfig_NoDatasetMetadataDisablesNode(t *testing.T) {
	parserConfig := entity.JSONMap{
		"Extractor:AutoExtractDefault": map[string]any{},
	}

	got := ApplyComponentScopedParserConfig(parserConfig, "tenant-llm")
	extractor := got["Extractor:AutoExtractDefault"].(map[string]any)
	want := map[string]any{
		"enabled":           false,
		"metadata":          []any{},
		"built_in_metadata": []any{},
	}
	if !reflect.DeepEqual(extractor["metadata"], want) {
		t.Fatalf("extractor metadata = %#v, want %#v", extractor["metadata"], want)
	}
}

func TestApplyComponentScopedParserConfig_DatasetMetadataIsAuthoritative(t *testing.T) {
	parserConfig := entity.JSONMap{
		"metadata": map[string]any{
			"enabled": true,
			"metadata": []any{
				map[string]any{"key": "dataset_field", "type": "string"},
			},
			"built_in_metadata": []any{},
		},
		"Extractor:CanvasNode": map[string]any{
			"metadata": map[string]any{
				"enabled": false,
				"metadata": []any{
					map[string]any{"key": "node_field", "type": "string"},
				},
				"built_in_metadata": []any{},
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
		t.Fatalf("expected dataset metadata enabled=true, got false")
	}
	fields, ok := meta["metadata"].([]any)
	if !ok || len(fields) != 1 || fields[0].(map[string]any)["key"] != "dataset_field" {
		t.Fatalf("expected dataset_field to replace node_field, got %#v", meta["metadata"])
	}
}

func TestApplyComponentScopedParserConfig_PreservesNodeOnlyMetadata(t *testing.T) {
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
	if !ok || len(fields) != 1 || fields[0].(map[string]any)["key"] != "node_author" {
		t.Fatalf("expected node_author preserved, got %#v", meta["metadata"])
	}
	builtIn, ok := meta["built_in_metadata"].([]any)
	if !ok || len(builtIn) != 1 || builtIn[0].(map[string]any)["key"] != "node_doc_title" {
		t.Fatalf("expected node_doc_title preserved, got %#v", meta["built_in_metadata"])
	}
}
