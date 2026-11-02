package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestAgentTemplateExtractorsUseModularParams guards against #20716: the
// Extractor component only reads the modular sub-configs
// (keywords/questions/tags/summary/metadata) declared by schema.ExtractorParam.
// A template still baking the legacy flat fields (field_name/prompts/
// sys_prompt/tenant_llm_id) ships an Extractor whose prompt configuration is
// silently ignored at runtime.
func TestAgentTemplateExtractorsUseModularParams(t *testing.T) {
	// The agent templates carry a canvas form (data.form) that mirrors the DSL
	// params; the ingestion_pipeline_* templates keep their params only in the
	// DSL component, so only the former must have a matching form.
	dirs := []struct {
		path        string
		requireForm bool
	}{
		{filepath.Join("..", "..", "agent", "templates"), true},
		{filepath.Join("template"), false},
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

	// subTaskActive mirrors the runtime gates in internal/ingestion/component:
	// summary/metadata run when enabled is true; keywords/questions/tags follow
	// resolveExtractToggle — an explicit "enabled" key is authoritative, and only
	// a legacy group without it derives its state from top_n > 0.
	subTaskActive := func(name string, v any) bool {
		m, ok := v.(map[string]any)
		if !ok {
			return false
		}
		switch name {
		case "keywords", "questions", "tags":
			if enabled, ok := m["enabled"]; ok {
				if b, ok := enabled.(bool); ok {
					return b
				}
				n, ok := enabled.(float64)
				return ok && n == 1
			}
			n, ok := m["top_n"].(float64)
			return ok && n > 0
		default:
			e, ok := m["enabled"].(bool)
			return ok && e
		}
	}

	seen := 0
	for _, dir := range dirs {
		files, err := filepath.Glob(filepath.Join(dir.path, "*.json"))
		if err != nil {
			t.Fatalf("glob %s: %v", dir.path, err)
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

			// Canvas nodes mirror the component params in data.form for the
			// agent templates; collect them by id so the two copies can be
			// compared.
			nodeForms := map[string]map[string]any{}
			if nodes, ok := doc["dsl"].(map[string]any)["graph"].(map[string]any)["nodes"].([]any); ok {
				for _, n := range nodes {
					node, _ := n.(map[string]any)
					id, _ := node["id"].(string)
					form, _ := node["data"].(map[string]any)["form"].(map[string]any)
					if id != "" && form != nil {
						nodeForms[id] = form
					}
				}
			}
			for cpnID, comp := range components {
				obj, _ := comp.(map[string]any)["obj"].(map[string]any)
				if obj["component_name"] != "Extractor" {
					continue
				}
				seen++
				params, _ := obj["params"].(map[string]any)

				if form, ok := nodeForms[cpnID]; ok {
					if !reflect.DeepEqual(form, params) {
						t.Errorf("%s: Extractor %s params and canvas data.form diverge; both must be updated together", f, cpnID)
					}
				} else if dir.requireForm {
					t.Errorf("%s: Extractor %s has no matching canvas data.form node to compare against params", f, cpnID)
				}
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

				// The params must also survive BuildParserConfig: no
				// dropped keys and any active sub-task left active with its
				// prompt intact.
				dslRaw, _ := json.Marshal(doc["dsl"])
				cleaned := buildTestParserConfig(t, dslRaw, map[string]any{cpnID: params})
				got, _ := cleaned[cpnID].(map[string]any)
				if got == nil {
					t.Errorf("%s: Extractor %s dropped by BuildParserConfig", f, cpnID)
					continue
				}
				for k := range params {
					if _, kept := got[k]; !kept {
						t.Errorf("%s: Extractor %s param %q dropped by BuildParserConfig", f, cpnID, k)
					}
				}
				for _, k := range modularKeys {
					v, ok := params[k]
					if !ok || !subTaskActive(k, v) {
						continue
					}
					if !subTaskActive(k, got[k]) {
						t.Errorf("%s: Extractor %s sub-task %q no longer active after BuildParserConfig: %#v", f, cpnID, k, got[k])
						continue
					}
					if k == "summary" {
						cleanedSummary, _ := got[k].(map[string]any)
						prompt, _ := cleanedSummary["system_prompt"].(string)
						if prompt == "" {
							t.Errorf("%s: Extractor %s summary.system_prompt empty after BuildParserConfig", f, cpnID)
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
