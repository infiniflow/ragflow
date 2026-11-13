package knowledge_compile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"ragflow/internal/ingestion/component/knowledge_compiler/structure"
)

// builtinTemplateConfig loads one built-in template's `config` block the same
// way the service does.
func builtinTemplateConfig(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("templates", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var doc struct {
		Config map[string]any `yaml:"config"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	if doc.Config == nil {
		t.Fatalf("%s has no config block", name)
	}
	return doc.Config
}

// outputFieldNamesForTest reads a section's declared output keys in either
// spelling, so the assertion below fails on the declaration the model actually
// sees rather than on one particular way of writing it.
func outputFieldNamesForTest(section map[string]any) []string {
	raw, _ := section["output_fields"].([]any)
	names := []string{}
	for _, entry := range raw {
		switch f := entry.(type) {
		case string:
			names = append(names, strings.TrimSpace(f))
		case map[string]any:
			if n, ok := f["name"].(string); ok {
				names = append(names, strings.TrimSpace(n))
			}
		}
	}
	return names
}

// The ontology template shipped without any evidence declaration for as long as
// it existed, and nothing noticed: the compilation ran, the rows were written,
// and every assertion was simply unaccountable. A row the reader cannot check
// against the source is the one failure this contract exists to prevent, so the
// declaration is asserted here rather than trusted to survive review.
func TestBuiltinOntologyTemplateAsksForEvidence(t *testing.T) {
	cfg := builtinTemplateConfig(t, "ontology.yaml")

	entity, _ := cfg["entity"].(map[string]any)
	if entity == nil {
		t.Fatal("ontology template has no entity section")
	}
	found := false
	for _, name := range outputFieldNamesForTest(entity) {
		if strings.EqualFold(name, "evidence") {
			found = true
		}
	}
	if !found {
		t.Fatal("entity.output_fields must declare evidence: an instance without a locatable quote cannot be audited")
	}

	// Relations are the stronger assertion, and the gate for them is opt-in —
	// so the declaration is what decides whether their quotes are ever checked.
	if !structure.RelationExpectsEvidence(cfg) {
		t.Fatal("relation.output_fields must declare evidence, otherwise the gate never validates relation quotes")
	}
}

var shapePlaceholderRE = regexp.MustCompile(`<[^<>]*>`)

// templateShapeText returns a declared `shape` in either spelling (plain string
// or per-language map), with the en text as the fallback the prompt uses.
func templateShapeText(raw any) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		if s, ok := v["en"].(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// A `shape` is pasted verbatim into the Response Format skeleton the model is
// told to fill, so a shape that is not itself valid JSON teaches the model to
// emit invalid JSON — and an elided member renders as an object key, which no
// repair pass can recover: the whole extraction is discarded as unparseable.
func TestBuiltinTemplateShapesAreValidJSON(t *testing.T) {
	entries, err := os.ReadDir("templates")
	if err != nil {
		t.Fatalf("read templates: %v", err)
	}
	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		template := entry.Name()
		cfg := builtinTemplateConfig(t, template)
		for _, section := range []string{"entity", "relation"} {
			fields, _ := cfg[section].(map[string]any)
			raw, _ := fields["output_fields"].([]any)
			for _, fieldEntry := range raw {
				field, ok := fieldEntry.(map[string]any)
				if !ok {
					continue
				}
				shape := templateShapeText(field["shape"])
				if shape == "" {
					continue
				}
				checked++
				name, _ := field["name"].(string)
				if !json.Valid([]byte(shapePlaceholderRE.ReplaceAllString(shape, "x"))) {
					t.Errorf("%s: %s.output_fields %q renders a skeleton that is not valid JSON: %s",
						template, section, name, shape)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no declared shapes found; the assertion would pass without checking anything")
	}
}
