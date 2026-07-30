package common

import "testing"

func TestInjectExtractorLLMID_SkipWhenUUID(t *testing.T) {
	uuid := "9e819c2442b14f9dab46062916e29195"
	pc := map[string]interface{}{
		"Extractor:A": map[string]interface{}{
			"llm_id": uuid,
		},
	}
	InjectExtractorLLMID(pc, "Qwen/Qwen3-8B@siliconflow")
	id := pc["Extractor:A"].(map[string]interface{})["llm_id"].(string)
	if id != uuid {
		t.Fatalf("expected UUID preserved, got %q", id)
	}
}

func TestInjectExtractorLLMID_SkipWhenComposite(t *testing.T) {
	composite := "Qwen/Qwen3-8B@siliconflow"
	pc := map[string]interface{}{
		"Extractor:B": map[string]interface{}{
			"llm_id": composite,
		},
	}
	InjectExtractorLLMID(pc, "DeepSeek@siliconflow")
	id := pc["Extractor:B"].(map[string]interface{})["llm_id"].(string)
	if id != composite {
		t.Fatalf("expected composite preserved, got %q", id)
	}
}

func TestInjectExtractorLLMID_InjectWhenEmpty(t *testing.T) {
	defaultLLM := "Qwen/Qwen3-8B@siliconflow"
	pc := map[string]interface{}{
		"Extractor:C": map[string]interface{}{},
	}
	InjectExtractorLLMID(pc, defaultLLM)
	id := pc["Extractor:C"].(map[string]interface{})["llm_id"].(string)
	if id != defaultLLM {
		t.Fatalf("expected %q injected, got %q", defaultLLM, id)
	}
}

func TestInjectExtractorLLMID_NoExtractor(t *testing.T) {
	pc := map[string]interface{}{
		"Parser:X": map[string]interface{}{"llm_id": ""},
	}
	InjectExtractorLLMID(pc, "default@provider")
	if _, ok := pc["Parser:X"]; !ok {
		t.Fatal("expected Parser:X still present")
	}
}

func TestInjectExtractorEnableMetadata_Disabled(t *testing.T) {
	pc := map[string]interface{}{
		"Extractor:A": map[string]interface{}{},
	}
	if InjectExtractorEnableMetadata(pc) {
		t.Fatal("expected no update when enable_metadata is off")
	}
	if _, ok := pc["Extractor:A"].(map[string]interface{})["enable_metadata"]; ok {
		t.Fatal("enable_metadata should not be set")
	}
}

func TestInjectExtractorEnableMetadata_InjectsAndMerges(t *testing.T) {
	pc := map[string]interface{}{
		"enable_metadata": true,
		"metadata": []interface{}{
			map[string]interface{}{"key": "author", "type": "string", "description": "doc author", "enum": []interface{}{"Alice", "Bob"}},
		},
		"built_in_metadata": []interface{}{
			map[string]interface{}{"key": "year", "type": "number"},
		},
		"Extractor:A": map[string]interface{}{},
		"Parser:X":    map[string]interface{}{},
	}
	if !InjectExtractorEnableMetadata(pc) {
		t.Fatal("expected update")
	}
	ext := pc["Extractor:A"].(map[string]interface{})
	if got, _ := ext["enable_metadata"].(int); got != 1 {
		t.Fatalf("expected enable_metadata=1, got %v", ext["enable_metadata"])
	}
	fields, ok := ext["metadata"].([]any)
	if !ok || len(fields) != 2 {
		t.Fatalf("expected 2 merged metadata, got %#v", ext["metadata"])
	}
	if _, ok := pc["Parser:X"].(map[string]interface{})["enable_metadata"]; ok {
		t.Fatal("Parser node must not be touched")
	}
}

func TestInjectExtractorEnableMetadata_RespectsExplicitEnabled(t *testing.T) {
	// A node the user already turned ON keeps its own config (no clobber).
	pc := map[string]interface{}{
		"enable_metadata": true,
		"metadata":        []interface{}{map[string]interface{}{"key": "author"}},
		"Extractor:A":     map[string]interface{}{"enable_metadata": 1},
	}
	if InjectExtractorEnableMetadata(pc) {
		t.Fatal("expected no update when user explicitly enabled enable_metadata")
	}
}

func TestInjectExtractorEnableMetadata_OverridesDefaultZero(t *testing.T) {
	// Shipped DSLs default enable_metadata to 0; the dataset flag must still
	// turn them on (otherwise auto-metadata could never activate).
	pc := map[string]interface{}{
		"enable_metadata": true,
		"metadata":        []interface{}{map[string]interface{}{"key": "author"}},
		"Extractor:A":     map[string]interface{}{"enable_metadata": 0},
	}
	if !InjectExtractorEnableMetadata(pc) {
		t.Fatal("expected update: dataset flag must override default enable_metadata=0")
	}
	ext := pc["Extractor:A"].(map[string]interface{})
	if got, _ := ext["enable_metadata"].(int); got != 1 {
		t.Fatalf("expected enable_metadata=1 after override, got %v", ext["enable_metadata"])
	}
	if _, ok := ext["metadata"].([]any); !ok {
		t.Fatalf("expected metadata field schema injected, got %#v", ext["metadata"])
	}
}

func TestInjectExtractorEnableMetadata_NoFields(t *testing.T) {
	pc := map[string]interface{}{
		"enable_metadata": true,
		"Extractor:A":     map[string]interface{}{},
	}
	if InjectExtractorEnableMetadata(pc) {
		t.Fatal("expected no update when no fields configured")
	}
}
