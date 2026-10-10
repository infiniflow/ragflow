package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestAgentTemplateExtractorsUseModularParams guards against #20716: the
// Extractor component only reads the modular sub-configs
// (keywords/questions/tags/summary/metadata) declared by schema.ExtractorParam.
// A template still baking the legacy flat fields (field_name/prompts/
// sys_prompt/tenant_llm_id) ships an Extractor whose prompt configuration is
// silently ignored at runtime.
func TestAgentTemplateExtractorsUseModularParams(t *testing.T) {
	dirs := []string{
		filepath.Join("..", "..", "agent", "templates"),
		filepath.Join("template"),
	}
	legacyKeys := []string{"field_name", "prompts", "sys_prompt", "tenant_llm_id"}
	modularKeys := []string{"keywords", "questions", "tags", "summary", "metadata"}

	seen := 0
	for _, dir := range dirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.json"))
		if err != nil {
			t.Fatalf("glob %s: %v", dir, err)
		}
		for _, f := range files {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			var doc map[string]any
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("unmarshal %s: %v", f, err)
			}
			if doc["canvas_type"] != "Ingestion Pipeline" {
				continue
			}
			components, _ := doc["dsl"].(map[string]any)["components"].(map[string]any)
			for cpnID, comp := range components {
				obj, _ := comp.(map[string]any)["obj"].(map[string]any)
				if obj["component_name"] != "Extractor" {
					continue
				}
				seen++
				params, _ := obj["params"].(map[string]any)
				for _, k := range legacyKeys {
					if _, found := params[k]; found {
						t.Errorf("%s: Extractor %s still uses legacy param %q; migrate it to the modular format", f, cpnID, k)
					}
				}
				hasModular := false
				for _, k := range modularKeys {
					if v, ok := params[k]; ok && v != nil {
						hasModular = true
						break
					}
				}
				if !hasModular {
					t.Errorf("%s: Extractor %s configures no modular sub-task (keywords/questions/tags/summary/metadata)", f, cpnID)
				}

				// The params must also survive CleanComponentParams: no
				// dropped keys and the modular config intact.
				dslRaw, _ := json.Marshal(doc["dsl"])
				cleaned := CleanComponentParams(dslRaw, map[string]any{cpnID: params})
				got, _ := cleaned[cpnID].(map[string]any)
				if got == nil {
					t.Errorf("%s: Extractor %s dropped by CleanComponentParams", f, cpnID)
					continue
				}
				for k := range params {
					if _, kept := got[k]; !kept {
						t.Errorf("%s: Extractor %s param %q dropped by CleanComponentParams", f, cpnID, k)
					}
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("no Ingestion Pipeline templates with an Extractor found; test paths stale?")
	}
	t.Logf("checked %d Extractor components across builtin ingestion templates", seen)
}
