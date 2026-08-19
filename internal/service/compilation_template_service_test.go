package service

import (
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
