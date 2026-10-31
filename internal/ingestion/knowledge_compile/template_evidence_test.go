package knowledge_compile

import (
	"os"
	"path/filepath"
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
