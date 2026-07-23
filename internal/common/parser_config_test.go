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
