package pipeline

import (
	"encoding/json"
	"testing"

	"ragflow/internal/entity"
)

// generalDSL returns a minimal DSL resembling the "general" template's component
// structure, with one Parser and one Chunker component.
func generalDSL(t *testing.T) []byte {
	t.Helper()
	dsl := map[string]any{
		"components": map[string]any{
			"Parser:HipSignsRhyme": map[string]any{
				"obj": map[string]any{
					"component_name": "Parser",
					"params": map[string]any{
						"outputs": map[string]any{},
						"pdf":     map[string]any{"parse_method": "DeepDOC", "lang": "en"},
						"docx":    map[string]any{"output_format": "json"},
					},
				},
			},
			"Chunker:LegalReadersDecide": map[string]any{
				"obj": map[string]any{
					"component_name": "Chunker",
					"params": map[string]any{
						"outputs":       map[string]any{},
						"chunk_size":    float64(512),
						"chunk_overlap": float64(128),
					},
				},
			},
		},
	}
	raw, err := json.Marshal(dsl)
	if err != nil {
		t.Fatalf("marshal dsl fixture: %v", err)
	}
	return raw
}
func TestCleanComponentParams_DropsLegacyFlatFields(t *testing.T) {
	dslJSON := generalDSL(t)
	raw := map[string]any{
		"chunk_token_num":    256,
		"image_context_size": 10,
	}
	result := CleanComponentParams(dslJSON, raw)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}

func TestCleanComponentParams_DropsUnknownCPNID(t *testing.T) {
	dslJSON := generalDSL(t)
	raw := map[string]any{
		"Parser:NoSuch": map[string]any{"chunk_size": float64(256)},
	}
	result := CleanComponentParams(dslJSON, raw)
	if _, ok := result["Parser:NoSuch"]; ok {
		t.Error("expected unknown cpnID to be dropped")
	}
}

func TestCleanComponentParams_DropsUnknownParamKey(t *testing.T) {
	dslJSON := generalDSL(t)
	raw := map[string]any{
		"Parser:HipSignsRhyme": map[string]any{
			"no_such_param": 1,
			"pdf":           map[string]any{"parse_method": "deepdoc"},
		},
	}
	result := CleanComponentParams(dslJSON, raw)
	params := result["Parser:HipSignsRhyme"].(map[string]any)
	if _, ok := params["no_such_param"]; ok {
		t.Error("expected unknown param key to be dropped")
	}
	if _, ok := params["pdf"]; !ok {
		t.Error("expected known param key 'pdf' to be kept")
	}
}

func TestCleanComponentParams_ReturnsInputOnDSLError(t *testing.T) {
	result := CleanComponentParams([]byte("not json"), map[string]any{"key": "val"})
	if result["key"] != "val" {
		t.Error("expected input returned as-is on DSL error")
	}
}

func TestCleanComponentParams_ValidCPNIDPassesThrough(t *testing.T) {
	dslJSON := generalDSL(t)
	raw := map[string]any{
		"Parser:HipSignsRhyme": map[string]any{
			"pdf": map[string]any{"parse_method": "deepdoc"},
		},
		"Chunker:LegalReadersDecide": map[string]any{
			"chunk_size": float64(256),
		},
	}
	result := CleanComponentParams(dslJSON, raw)
	if _, ok := result["Parser:HipSignsRhyme"]; !ok {
		t.Error("expected Parser:HipSignsRhyme to pass through")
	}
	if _, ok := result["Chunker:LegalReadersDecide"]; !ok {
		t.Error("expected Chunker:LegalReadersDecide to pass through")
	}
}

// --- BuildParserConfig ---

func TestBuildParserConfig_ShallowMerge_NestedParam(t *testing.T) {
	// A component with a nested-map param "chunk" that has sub-keys.
	dsl := map[string]any{
		"components": map[string]any{
			"Chunker:xyz": map[string]any{
				"obj": map[string]any{
					"component_name": "Chunker",
					"params": map[string]any{
						"outputs": map[string]any{},
						"chunk": map[string]any{
							"size":    float64(512),
							"overlap": float64(128),
						},
					},
				},
			},
		},
	}
	dslJSON, err := json.Marshal(dsl)
	if err != nil {
		t.Fatalf("marshal dsl: %v", err)
	}

	// User only overrides one sub-key of "chunk".
	overrides := map[string]any{
		"Chunker:xyz": map[string]any{
			"chunk": map[string]any{"size": float64(1024)},
		},
	}

	result := BuildParserConfig(dslJSON, overrides)
	chunker, ok := result["Chunker:xyz"].(map[string]any)
	if !ok {
		t.Fatal("expected Chunker:xyz in result")
	}
	chunk, ok := chunker["chunk"].(map[string]any)
	if !ok {
		t.Fatal("expected chunk key in result")
	}
	// After shallow merge: size is overridden, overlap is GONE.
	if chunk["size"] != float64(1024) {
		t.Errorf("expected size=1024 from override, got %v", chunk["size"])
	}
	if _, ok := chunk["overlap"]; ok {
		t.Error("shallow merge: overlap from defaults should NOT be preserved when chunk is fully replaced")
	}
}

func TestBuildParserConfig_ScalarOverridePreservesOtherDefaults(t *testing.T) {
	dslJSON := generalDSL(t)
	overrides := map[string]any{
		"Chunker:LegalReadersDecide": map[string]any{
			"chunk_size": float64(1024),
		},
	}
	result := BuildParserConfig(dslJSON, overrides)
	chunker, ok := result["Chunker:LegalReadersDecide"].(map[string]any)
	if !ok {
		t.Fatal("expected Chunker:LegalReadersDecide in result")
	}
	if chunker["chunk_size"] != float64(1024) {
		t.Errorf("expected chunk_size=1024, got %v", chunker["chunk_size"])
	}
	// chunk_overlap should be preserved from DSL defaults since it wasn't overridden.
	if chunker["chunk_overlap"] != float64(128) {
		t.Errorf("expected chunk_overlap=128 preserved from defaults, got %v", chunker["chunk_overlap"])
	}
}

func TestBuildParserConfig_UnknownCPNIDNotPresentInResult(t *testing.T) {
	dslJSON := generalDSL(t)
	overrides := map[string]any{
		"Parser:Unknown": map[string]any{"chunk_size": float64(256)},
	}
	result := BuildParserConfig(dslJSON, overrides)
	// Unknown cpnID should be dropped by CleanComponentParams; the result should
	// still contain the DSL-defined components with their defaults.
	if _, ok := result["Parser:Unknown"]; ok {
		t.Error("expected unknown cpnID to be absent from result")
	}
	if _, ok := result["Parser:HipSignsRhyme"]; !ok {
		t.Error("expected valid component from DSL to be present")
	}
}

func TestBuildParserConfig_FallbackOnDSLError(t *testing.T) {
	result := BuildParserConfig([]byte("not json"), map[string]any{"key": "val"})
	if result["key"] != "val" {
		t.Error("expected fallback to return raw config on DSL error")
	}
}

func TestBuildParserConfig_AllComponentsPresent(t *testing.T) {
	dslJSON := generalDSL(t)
	result := BuildParserConfig(dslJSON, nil)
	// Both components from the DSL fixture should be present.
	if _, ok := result["Parser:HipSignsRhyme"]; !ok {
		t.Error("expected Parser:HipSignsRhyme")
	}
	if _, ok := result["Chunker:LegalReadersDecide"]; !ok {
		t.Error("expected Chunker:LegalReadersDecide")
	}
}

// --- ResolveComponentParamsDefaults ---

func TestResolveComponentParamsDefaults_Basic(t *testing.T) {
	dslJSON := generalDSL(t)
	result, err := ResolveComponentParamsDefaults(dslJSON)
	if err != nil {
		t.Fatalf("ResolveComponentParamsDefaults: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected non-empty result")
	}
	// outputs should be stripped.
	parser := result["Parser:HipSignsRhyme"].(map[string]any)
	if _, ok := parser["outputs"]; ok {
		t.Error("expected outputs to be stripped")
	}
	if _, ok := parser["pdf"]; !ok {
		t.Error("expected pdf to be present")
	}
	chunker := result["Chunker:LegalReadersDecide"].(map[string]any)
	if chunker["chunk_size"] != float64(512) {
		t.Errorf("expected chunk_size=512, got %v", chunker["chunk_size"])
	}
}

func TestResolveComponentParamsDefaults_InvalidJSON(t *testing.T) {
	_, err := ResolveComponentParamsDefaults([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestResolveComponentParamsDefaults_ResultIsMutable(t *testing.T) {
	// Verify the returned map is a copy, not a reference to internal state.
	dslJSON := generalDSL(t)
	result, err := ResolveComponentParamsDefaults(dslJSON)
	if err != nil {
		t.Fatalf("ResolveComponentParamsDefaults: %v", err)
	}
	// Mutate the result.
	parser := result["Parser:HipSignsRhyme"].(map[string]any)
	delete(parser, "pdf")
	// Re-read: the second call should return a fresh copy unaffected by the mutation.
	result2, _ := ResolveComponentParamsDefaults(dslJSON)
	parser2 := result2["Parser:HipSignsRhyme"].(map[string]any)
	if _, ok := parser2["pdf"]; !ok {
		t.Error("expected result to be independent copy (pdf preserved)")
	}
}

// ensure entity.JSONMap is used (import used).
var _ entity.JSONMap
