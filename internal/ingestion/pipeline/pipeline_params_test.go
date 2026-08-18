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

func TestBuildParserConfig_BuiltinExtractorKeepsTagFileID(t *testing.T) {
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	checked := 0
	for _, ref := range registry.Refs() {
		tpl, ok := registry.Get(ref)
		if !ok {
			t.Fatalf("registry.Get(%q) failed", ref)
		}
		dslJSON, err := json.Marshal(tpl.DSL)
		if err != nil {
			t.Fatalf("marshal DSL %q: %v", ref, err)
		}
		schemas, err := ExtractAllComponentParams(dslJSON)
		if err != nil {
			t.Fatalf("ExtractAllComponentParams %q: %v", ref, err)
		}
		for _, s := range schemas {
			if s.ComponentName != "Extractor" {
				continue
			}
			checked++
			overrides := map[string]any{
				s.CpnID: map[string]any{"tag_file_id": "file-123"},
			}
			result := BuildParserConfig(dslJSON, overrides)
			params, ok := result[s.CpnID].(map[string]any)
			if !ok {
				t.Fatalf("template %q: expected component %q in result", ref, s.CpnID)
			}
			tagFileID := params["tag_file_id"]
			if tags, ok := params["tags"].(map[string]any); ok && tags["tag_file_id"] != nil {
				tagFileID = tags["tag_file_id"]
			}
			if tagFileID != "file-123" {
				t.Errorf("template %q: expected tag_file_id to survive BuildParserConfig, got %v", ref, tagFileID)
			}
		}
	}
	if checked == 0 {
		t.Fatal("expected at least one builtin template with an Extractor component")
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

func TestBuildParserConfig_BuiltinExtractorKeepsBuiltInMetadata(t *testing.T) {
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	checked := 0
	for _, ref := range registry.Refs() {
		tpl, ok := registry.Get(ref)
		if !ok {
			t.Fatalf("registry.Get(%q) failed", ref)
		}
		dslJSON, err := json.Marshal(tpl.DSL)
		if err != nil {
			t.Fatalf("marshal DSL %q: %v", ref, err)
		}
		schemas, err := ExtractAllComponentParams(dslJSON)
		if err != nil {
			t.Fatalf("ExtractAllComponentParams %q: %v", ref, err)
		}
		for _, s := range schemas {
			if s.ComponentName != "Extractor" {
				continue
			}
			checked++
			overrides := map[string]any{
				s.CpnID: map[string]any{
					"metadata": map[string]any{
						"enabled": true,
						"built_in_metadata": []any{
							map[string]any{"key": "update_time", "type": "time"},
						},
					},
				},
			}
			result := BuildParserConfig(dslJSON, overrides)
			params, ok := result[s.CpnID].(map[string]any)
			if !ok {
				t.Fatalf("template %q: expected component %q in result", ref, s.CpnID)
			}
			if _, ok := params["metadata"]; !ok {
				t.Errorf("template %q: metadata dropped by CleanComponentParams", ref)
			}
		}
	}
	if checked == 0 {
		t.Fatal("expected at least one builtin template with an Extractor component")
	}
}

func TestCleanComponentParams_KeepsModularExtractorParams(t *testing.T) {
	// Builtin template with empty Extractor params: {}
	dsl := map[string]any{
		"components": map[string]any{
			"Extractor:AutoExtractDefault": map[string]any{
				"obj": map[string]any{
					"component_name": "Extractor",
					"params":         map[string]any{},
				},
			},
		},
	}
	dslJSON, err := json.Marshal(dsl)
	if err != nil {
		t.Fatalf("marshal dsl: %v", err)
	}

	overrides := map[string]any{
		"Extractor:AutoExtractDefault": map[string]any{
			"summary": map[string]any{
				"enabled":       true,
				"system_prompt": "summary prompt",
			},
			"metadata": map[string]any{
				"enabled": true,
				"metadata": []any{
					map[string]any{"key": "category", "type": "string"},
				},
			},
			"keywords": map[string]any{
				"top_n": 5,
			},
			"enable_summary": 1,
			"llm_id":         "gpt-4",
			"temperature":    0.7,
		},
	}

	result := CleanComponentParams(dslJSON, overrides)
	ext, ok := result["Extractor:AutoExtractDefault"].(map[string]any)
	if !ok {
		t.Fatalf("expected Extractor:AutoExtractDefault in result, got: %v", result)
	}

	if sum, ok := ext["summary"].(map[string]any); !ok || sum["enabled"] != true {
		t.Errorf("expected summary.enabled == true, got: %v", ext["summary"])
	}
	if meta, ok := ext["metadata"].(map[string]any); !ok || meta["enabled"] != true {
		t.Errorf("expected metadata.enabled == true, got: %v", ext["metadata"])
	}
	if kw, ok := ext["keywords"].(map[string]any); !ok || kw["top_n"] != 5 {
		t.Errorf("expected keywords.top_n == 5, got: %v", ext["keywords"])
	}
	if ext["llm_id"] != "gpt-4" {
		t.Errorf("expected llm_id == gpt-4, got: %v", ext["llm_id"])
	}
	if _, ok := ext["enable_summary"]; !ok {
		t.Errorf("expected legacy flat field enable_summary to be kept for backward compatibility, got: %v", ext["enable_summary"])
	}
}

func TestCleanComponentParams_KeepsLegacyExtractorParams(t *testing.T) {
	dsl := map[string]any{
		"components": map[string]any{
			"Extractor:AutoExtractDefault": map[string]any{
				"obj": map[string]any{
					"component_name": "Extractor",
					"params":         map[string]any{},
				},
			},
		},
	}
	dslJSON, err := json.Marshal(dsl)
	if err != nil {
		t.Fatalf("marshal dsl: %v", err)
	}

	legacyConfig := map[string]any{
		"Extractor:AutoExtractDefault": map[string]any{
			"auto_keywords":   4,
			"auto_questions":  2,
			"auto_tags":       3,
			"tag_file_id":     "tag-file-1",
			"enable_summary":  1,
			"enable_metadata": 1,
			"metadata_config": map[string]any{
				"enabled": true,
				"metadata": []any{
					map[string]any{"key": "cat", "type": "string"},
				},
			},
			"sys_prompt": "legacy summary sys prompt",
			"llm_id":     "gpt-4",
		},
	}

	result := CleanComponentParams(dslJSON, legacyConfig)
	ext, ok := result["Extractor:AutoExtractDefault"].(map[string]any)
	if !ok {
		t.Fatalf("expected Extractor:AutoExtractDefault in result, got: %v", result)
	}

	kw, ok := ext["keywords"].(map[string]any)
	if !ok || kw["top_n"] != 4 {
		t.Errorf("expected keywords.top_n == 4, got: %#v", ext["keywords"])
	}
	q, ok := ext["questions"].(map[string]any)
	if !ok || q["top_n"] != 2 {
		t.Errorf("expected questions.top_n == 2, got: %#v", ext["questions"])
	}
	tags, ok := ext["tags"].(map[string]any)
	if !ok || tags["top_n"] != 3 || tags["tag_file_id"] != "tag-file-1" {
		t.Errorf("expected tags.top_n == 3 / tag_file_id == tag-file-1, got: %#v", ext["tags"])
	}
	sum, ok := ext["summary"].(map[string]any)
	if !ok || sum["enabled"] != true || sum["system_prompt"] != "legacy summary sys prompt" {
		t.Errorf("expected summary.enabled == true / system_prompt == legacy summary sys prompt, got: %#v", ext["summary"])
	}
	meta, ok := ext["metadata"].(map[string]any)
	if !ok || meta["enabled"] != true {
		t.Errorf("expected metadata.enabled == true, got: %#v", ext["metadata"])
	}
	if ext["llm_id"] != "gpt-4" {
		t.Errorf("expected llm_id == gpt-4, got: %#v", ext["llm_id"])
	}
}
