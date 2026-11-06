package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/knowledge_compiler/common"
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

// A class `parent` may name several superclasses, `|`-separated. The
// unknown-parent check and the cyclic-chain check both have to walk every one of
// them: a cycle that closes through a NON-first parent must still be rejected,
// and a diamond (one ancestor reached through two parents) must not be.
func TestValidateOntologyTemplateMultiParent(t *testing.T) {
	config := func(employeeParent, organizationParent string) map[string]interface{} {
		fields := []interface{}{
			map[string]interface{}{"type": "agent"},
			map[string]interface{}{"type": "person", "parent": "agent"},
		}
		org := map[string]interface{}{"type": "organization"}
		if organizationParent != "" {
			org["parent"] = organizationParent
		}
		fields = append(fields, org, map[string]interface{}{"type": "employee", "parent": employeeParent})
		return map[string]interface{}{
			"kind":     "ontology",
			"entity":   map[string]interface{}{"fields": fields},
			"relation": map[string]interface{}{"fields": []interface{}{}},
		}
	}

	// A diamond: both parents descend from `agent`, which is not a cycle.
	if err := validateOntologyTemplate(config("person | organization", "agent")); err != nil {
		t.Fatalf("multi-parent template rejected: %v", err)
	}

	// The unknown parent is the second one, so stopping at the first would pass.
	err := validateOntologyTemplate(config("person | nope", "agent"))
	if err == nil || !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("second parent unchecked: err = %v", err)
	}

	// organization -> employee closes the cycle through employee's second
	// parent; the first parent (`person`) is acyclic.
	err = validateOntologyTemplate(config("person | organization", "employee"))
	if err == nil || !strings.Contains(err.Error(), "cyclic parent chain") {
		t.Fatalf("cycle through a non-first parent not detected: err = %v", err)
	}
}

// Aliases, a closed value set and a mandatory attribute are declaration-time
// choices, so every way they can contradict the rest of the template is refused
// when the template is saved rather than discovered in the model graph later.
//
// The alias rules exist because an alias is resolved back to its term during
// compilation: two terms claiming one string would put the same row in whichever
// class the map happened to yield, so the model graph would change between
// restarts.
func TestValidateOntologyTemplateAliasesAndAcceptanceRules(t *testing.T) {
	build := func(entity, relation []interface{}) map[string]interface{} {
		return map[string]interface{}{
			"kind":     "ontology",
			"entity":   map[string]interface{}{"fields": entity},
			"relation": map[string]interface{}{"fields": relation},
		}
	}
	classes := func(extra map[string]interface{}) []interface{} {
		organization := map[string]interface{}{"type": "organization"}
		for k, v := range extra {
			organization[k] = v
		}
		return []interface{}{map[string]interface{}{"type": "person"}, organization}
	}
	property := func(kind string, extra map[string]interface{}) []interface{} {
		field := map[string]interface{}{"type": "works_for", "kind": kind, "domain": "person", "range": "organization"}
		if kind == "datatype" {
			field["datatype"] = "string"
		}
		for k, v := range extra {
			field[k] = v
		}
		return []interface{}{field}
	}

	if err := validateOntologyTemplate(build(
		classes(map[string]interface{}{"aliases": []interface{}{"company", "firm"}}),
		property("object", map[string]interface{}{"aliases": []interface{}{"employed_by"}}),
	)); err != nil {
		t.Fatalf("declared aliases rejected: %v", err)
	}
	if err := validateOntologyTemplate(build(
		classes(nil),
		property("datatype", map[string]interface{}{"enum": "novel|poem", "required": true}),
	)); err != nil {
		t.Fatalf("declared enum/required rejected: %v", err)
	}

	// An alias that names another class would resolve that name to two terms.
	err := validateOntologyTemplate(build(
		classes(map[string]interface{}{"aliases": []interface{}{"person"}}),
		property("object", nil),
	))
	if err == nil || !strings.Contains(err.Error(), "already names class") {
		t.Fatalf("alias colliding with a class name not rejected: err = %v", err)
	}

	// A term aliasing itself reads as a pasted duplicate and maps to a no-op.
	err = validateOntologyTemplate(build(
		classes(map[string]interface{}{"aliases": []interface{}{"ORGANIZATION"}}),
		property("object", nil),
	))
	if err == nil || !strings.Contains(err.Error(), "lists itself as an alias") {
		t.Fatalf("self-alias not rejected: err = %v", err)
	}

	// Property aliases are checked in the property namespace.
	err = validateOntologyTemplate(build(
		classes(nil),
		append(property("object", map[string]interface{}{"aliases": []interface{}{"works_for"}}),
			map[string]interface{}{"type": "genre", "kind": "datatype", "domain": "person", "datatype": "string"}),
	))
	if err == nil || !strings.Contains(err.Error(), "lists itself as an alias") {
		t.Fatalf("property self-alias not rejected: err = %v", err)
	}

	// A closed value set and a mandatory value are both datatype-property ideas.
	// On an object property the compiler would ignore them silently, which is
	// worse than refusing them.
	err = validateOntologyTemplate(build(classes(nil), property("object", map[string]interface{}{"enum": []interface{}{"a"}})))
	if err == nil || !strings.Contains(err.Error(), "only applies to datatype") {
		t.Fatalf("enum on an object property not rejected: err = %v", err)
	}
	err = validateOntologyTemplate(build(classes(nil), property("object", map[string]interface{}{"required": true})))
	if err == nil || !strings.Contains(err.Error(), "only applies to datatype") {
		t.Fatalf("required on an object property not rejected: err = %v", err)
	}

	// An enum is rendered into every extraction prompt, so it is bounded.
	many := make([]interface{}, 0, maxOntologyEnumValues+1)
	for i := 0; i <= maxOntologyEnumValues; i++ {
		many = append(many, fmt.Sprintf("value-%d", i))
	}
	err = validateOntologyTemplate(build(classes(nil), property("datatype", map[string]interface{}{"enum": many})))
	if err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("oversized enum not rejected: err = %v", err)
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
		t.Fatal("expected wiki presets to load from internal/ingestion/knowledge_compile")
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

func TestLoadBuiltinTemplates_AllYAMLDefinitions(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err = os.Chdir("../.."); err != nil {
		t.Fatalf("chdir to repo root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	dir := filepath.Join("internal", "ingestion", "knowledge_compile", "templates")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read builtin template directory: %v", err)
	}
	wantIDs := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" && filepath.Ext(entry.Name()) != ".yml" {
			continue
		}
		wantIDs[strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))] = struct{}{}
	}

	templates, err := loadBuiltinTemplates()
	if err != nil {
		t.Fatalf("loadBuiltinTemplates: %v", err)
	}
	if len(templates) != len(wantIDs) {
		t.Fatalf("loaded %d builtin templates, want %d YAML definitions", len(templates), len(wantIDs))
	}

	for _, template := range templates {
		if _, ok := wantIDs[template.ID]; !ok {
			t.Errorf("loaded unexpected builtin template %q", template.ID)
		}
		if template.Config["kind"] == nil {
			t.Errorf("builtin template %q has no config.kind", template.ID)
		}
		if _, err = common.KindToVariant(template.Kind); err != nil {
			t.Errorf("builtin template %q kind %q is not supported: %v", template.ID, template.Kind, err)
		}
		// Tree intentionally carries empty entity/relation stubs for the shared
		// form shape; those fields are not part of the tree template contract.
		if template.Kind != "tree" {
			payload := map[string]interface{}{
				"name":   template.DisplayName,
				"kind":   template.Kind,
				"config": map[string]interface{}(template.Config),
			}
			if err = ValidateTemplatePayload(payload, true); err != nil {
				t.Errorf("builtin template %q failed validation: %v", template.ID, err)
			}
		}
		delete(wantIDs, template.ID)
	}
	for id := range wantIDs {
		t.Errorf("builtin YAML %q was not loaded", id)
	}
}
