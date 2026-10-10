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

	// The agent templates preset an active sub-task; the ingestion_pipeline_*
	// templates ship every sub-task disabled and let the operator enable them
	// on the canvas, so only the former must be active out of the box.
	mustBeActive := map[string]bool{
		"advanced_ingestion_pipeline.json": true,
		"chunk_summary.json":               true,
		"title_chunker.json":               true,
	}

	// subTaskActive mirrors the runtime gate in internal/ingestion/component:
	// keywords/questions/tags run when top_n > 0 (extractor.go),
	// summary/metadata run when enabled is true.
	subTaskActive := func(name string, v any) bool {
		m, ok := v.(map[string]any)
		if !ok {
			return false
		}
		switch name {
		case "keywords", "questions", "tags":
			n, ok := m["top_n"].(float64)
			return ok && n > 0
		default:
			e, ok := m["enabled"].(bool)
			return ok && e
		}
	}

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
			base := filepath.Base(f)
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
				activeCount := 0
				for _, k := range modularKeys {
					v, ok := params[k]
					if !ok || v == nil {
						continue
					}
					hasModular = true
					if subTaskActive(k, v) {
						activeCount++
					}
				}
				if !hasModular {
					t.Errorf("%s: Extractor %s configures no modular sub-task (keywords/questions/tags/summary/metadata)", f, cpnID)
				}
				if mustBeActive[base] && activeCount == 0 {
					t.Errorf("%s: Extractor %s has every modular sub-task disabled; the template's purpose requires at least one active", f, cpnID)
				}

				// The params must also survive CleanComponentParams: no
				// dropped keys and any active sub-task left active with its
				// prompt intact.
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
				for _, k := range modularKeys {
					v, ok := params[k]
					if !ok || !subTaskActive(k, v) {
						continue
					}
					if !subTaskActive(k, got[k]) {
						t.Errorf("%s: Extractor %s sub-task %q no longer active after CleanComponentParams: %#v", f, cpnID, k, got[k])
						continue
					}
					if k == "summary" {
						cleanedSummary, _ := got[k].(map[string]any)
						prompt, _ := cleanedSummary["system_prompt"].(string)
						if prompt == "" {
							t.Errorf("%s: Extractor %s summary.system_prompt empty after CleanComponentParams", f, cpnID)
						}
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
