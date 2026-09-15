package service

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

// TestValidateTemplatePayload_AcceptsJSONMapConfig covers the create-from-UI
// regression: GroupTemplate.Config is an entity.JSONMap (a named map type), and
// ValidateTemplatePayload used a bare config.(map[string]interface{}) assertion
// that silently rejected it, surfacing as "102 invalid template config". Both
// the raw map and the JSONMap forms must validate identically.
func TestValidateTemplatePayload_AcceptsJSONMapConfig(t *testing.T) {
	cfg := map[string]interface{}{
		"kind": "graph",
		"entity": map[string]interface{}{
			"fields": []interface{}{
				map[string]interface{}{
					"type":        "organization",
					"description": "company",
				},
			},
		},
		"relation": map[string]interface{}{
			"fields": []interface{}{
				map[string]interface{}{
					"type":        "acquisition",
					"description": "took over",
				},
			},
		},
	}
	base := map[string]interface{}{
		"name": "tpl",
		"kind": "graph",
	}

	// 1) Raw map[string]interface{} payload (older path) must pass.
	raw := map[string]interface{}{}
	for k, v := range base {
		raw[k] = v
	}
	raw["config"] = cfg
	if err := ValidateTemplatePayload(raw, true); err != nil {
		t.Fatalf("raw map config rejected: %v", err)
	}

	// 2) entity.JSONMap payload (what the group create path builds from
	// GroupTemplate.Config) must also pass — this is the regression.
	jm := map[string]interface{}{}
	for k, v := range base {
		jm[k] = v
	}
	jm["config"] = entity.JSONMap(cfg)
	if err := ValidateTemplatePayload(jm, true); err != nil {
		t.Fatalf("entity.JSONMap config rejected: %v", err)
	}

	// 3) A non-map config is still an error.
	bad := map[string]interface{}{}
	for k, v := range base {
		bad[k] = v
	}
	bad["config"] = "not-a-map"
	if err := ValidateTemplatePayload(bad, true); err == nil {
		t.Fatal("non-map config should be rejected")
	}
}

func TestValidateTemplatePayloadCountsTextLimitsByRune(t *testing.T) {
	valid := map[string]interface{}{
		"name":        "模板",
		"description": strings.Repeat("描", 1024),
		"kind":        "graph",
		"config": map[string]interface{}{
			"global_rules": strings.Repeat("规", 4096),
			"entity": map[string]interface{}{
				"fields": []interface{}{map[string]interface{}{
					"type": "person", "description": strings.Repeat("人", 1024), "rule": strings.Repeat("则", 1024),
				}},
			},
			"relation": map[string]interface{}{"fields": []interface{}{}},
		},
	}
	if err := ValidateTemplatePayload(valid, true); err != nil {
		t.Fatalf("valid Unicode lengths rejected: %v", err)
	}

	invalid := map[string]interface{}{
		"name": "模板",
		"kind": "graph",
		"config": map[string]interface{}{
			"global_rules": strings.Repeat("规", 4097),
		},
	}
	if err := ValidateTemplatePayload(invalid, true); err == nil {
		t.Fatal("global_rules over 4096 Unicode characters should be rejected")
	}
}

// TestLoadWikiPresets_FrontendContract pins the Python API contract of
// /v1/compilation-templates/wiki-presets: every preset must expose the
// "example" JSON key filled from the yaml "example" key. The Go port once
// emitted "page_example" while reading a yaml key that does not exist, so
// the frontend received undefined for preset.example and the "Add template"
// page crashed with "TypeError: Cannot read properties of undefined
// (reading 'trim')" as soon as the Wiki kind was selected.
func TestLoadWikiPresets_FrontendContract(t *testing.T) {
	// LoadWikiPresets resolves the preset directory from the working
	// directory, so run from the repo root where the data files live.
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err = os.Chdir("../.."); err != nil {
		t.Fatalf("chdir to repo root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	svc := NewCompilationTemplateService()
	presets, err := svc.LoadWikiPresets()
	if err != nil {
		t.Fatalf("LoadWikiPresets: %v", err)
	}
	if len(presets) == 0 {
		t.Fatal("expected wiki presets to load from api/db/init_data")
	}
	for _, preset := range presets {
		if preset.Instruction == "" {
			t.Errorf("preset %q: empty instruction", preset.ID)
		}
		if preset.Example == "" {
			t.Errorf("preset %q: empty example (yaml key mismatch?)", preset.ID)
		}
		blob, merr := json.Marshal(preset)
		if merr != nil {
			t.Fatalf("marshal preset %q: %v", preset.ID, merr)
		}
		var decoded map[string]interface{}
		if uerr := json.Unmarshal(blob, &decoded); uerr != nil {
			t.Fatalf("unmarshal preset %q: %v", preset.ID, uerr)
		}
		if _, ok := decoded["example"]; !ok {
			t.Errorf("preset %q: JSON payload missing \"example\" key: %s", preset.ID, blob)
		}
		if _, ok := decoded["page_example"]; ok {
			t.Errorf("preset %q: stale \"page_example\" key in JSON payload: %s", preset.ID, blob)
		}
	}
}
