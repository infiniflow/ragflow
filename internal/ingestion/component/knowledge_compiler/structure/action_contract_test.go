package structure

import (
	"strings"
	"testing"

	"ragflow/internal/ontology/contract"
)

// actionClassFields builds the entity field list of a template that declares an
// action class, with the contract narrowed to `declared` (nil = no narrowing).
func actionClassFields(declared any) []any {
	action := map[string]any{"type": contract.ActionClassType}
	if declared != nil {
		action["contract"] = declared
	}
	return []any{
		map[string]any{"type": "title", "description": "a work"},
		action,
	}
}

func TestDeclaredContractFieldsNarrowing(t *testing.T) {
	// A template written by hand may use either spelling: the "|"-separated
	// string (same shape as `aliases`) or a list.
	for _, tc := range []struct {
		name     string
		raw      any
		expected []string
	}{
		{"pipe string", "authorization|audit_required", []string{"authorization", "audit_required"}},
		{"list", []any{"authorization", "audit_required"}, []string{"authorization", "audit_required"}},
		{"trimmed", " authorization | follow_ups ", []string{"authorization", "follow_ups"}},
		{"repeated name is one declaration", "authorization|authorization", []string{"authorization"}},
		{"unknown names dropped", "authorization|parameters|definition", []string{"authorization"}},
		{"nothing declared", nil, contract.ActionContractFields()},
		{"nothing recognisable", "parameters|definition", contract.ActionContractFields()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := declaredContractFields(tc.raw)
			if strings.Join(got, ",") != strings.Join(tc.expected, ",") {
				t.Fatalf("declared fields = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestActionClassContractFieldsOnlyForActionClass(t *testing.T) {
	declared, isAction := actionClassContractFields(actionClassFields("authorization"))
	if !isAction {
		t.Fatal("a declared action class must be recognised")
	}
	if len(declared) != 1 || declared[0] != "authorization" {
		t.Fatalf("declared = %v, want [authorization]", declared)
	}

	// Case and surrounding space on `type` are not part of the declaration.
	_, isAction = actionClassContractFields([]any{map[string]any{"type": " Action "}})
	if !isAction {
		t.Fatal("type matching must ignore case and padding")
	}

	if _, isAction := actionClassContractFields([]any{map[string]any{"type": "title"}}); isAction {
		t.Fatal("a non-action template must not be treated as declaring an action")
	}
	if _, isAction := actionClassContractFields(nil); isAction {
		t.Fatal("no class fields must not be treated as declaring an action")
	}
}

func TestContractShapeRendersOnlyDeclared(t *testing.T) {
	// The skeleton is the mechanism: a key absent from it is a key the model does
	// not emit. Rendering all six here is what made a narrow declaration useless.
	shape := contractShape([]string{"authorization", "audit_required"})
	if shape != `{"authorization": "<string>", "audit_required": "<true|false>"}` {
		t.Fatalf("shape = %s", shape)
	}
	for _, name := range []string{"preconditions", "atomic_effects", "follow_ups", "failure_policy"} {
		if strings.Contains(shape, name) {
			t.Fatalf("undeclared field %s leaked into the skeleton: %s", name, shape)
		}
	}
}

func TestContractShapePlaceholdersFollowFieldType(t *testing.T) {
	shape := contractShape(contract.ActionContractFields())
	for _, want := range []string{
		`"atomic_effects": ["<string>"]`,
		`"follow_ups": ["<string>"]`,
		`"audit_required": "<true|false>"`,
		`"authorization": "<string>"`,
		`"preconditions": "<string>"`,
		`"failure_policy": "<string>"`,
	} {
		if !strings.Contains(shape, want) {
			t.Fatalf("expected %s in %s", want, shape)
		}
	}
}

func TestQuoteShapeRendersOnlyDeclared(t *testing.T) {
	shape := quoteShape([]string{"authorization"})
	if shape != `{"authorization": "<verbatim sentence>"}` {
		t.Fatalf("quote shape = %s", shape)
	}
}

func TestWithActionContractFieldsSkipsNonActionTemplate(t *testing.T) {
	extra := configOutputFields(map[string]any{
		"output_fields": []any{map[string]any{"name": "evidence", "shape": `[{"quote": "<q>"}]`}},
	})
	got := withActionContractFields(extra, []any{map[string]any{"type": "title"}})
	if len(got) != len(extra) {
		t.Fatalf("non-action template gained fields: %v", got)
	}
}

func TestWithActionContractFieldsCarriesTheDeclaration(t *testing.T) {
	got := withActionContractFields(nil, actionClassFields("authorization|follow_ups"))
	if len(got) != 2 {
		t.Fatalf("expected contract and contract_quotes, got %v", got)
	}
	value, _ := got[0].(map[string]any)
	quotes, _ := got[1].(map[string]any)
	if value["name"] != "contract" || quotes["name"] != "contract_quotes" {
		t.Fatalf("unexpected field order: %v", got)
	}
	for _, field := range []map[string]any{value, quotes} {
		shape, _ := field["shape"].(string)
		if !strings.Contains(shape, `"authorization"`) || !strings.Contains(shape, `"follow_ups"`) {
			t.Fatalf("%v skeleton lost declared fields: %s", field["name"], shape)
		}
		if strings.Contains(shape, "failure_policy") {
			t.Fatalf("%v skeleton carries an undeclared field: %s", field["name"], shape)
		}
	}
}

func TestEntityPromptCarriesTheNarrowContractSkeleton(t *testing.T) {
	parserConfig := map[string]any{
		"kind": "ontology",
		"entity": map[string]any{
			"fields": actionClassFields("authorization"),
			"output_fields": []any{
				map[string]any{"name": "attributes", "shape": `{"<attribute>": <value>}`},
			},
		},
		"relation": map[string]any{"fields": []any{map[string]any{"type": "include"}}},
	}

	nodePrompt, _ := HypergraphPrompts(parserConfig, "English")

	if !strings.Contains(nodePrompt, `"contract"`) || !strings.Contains(nodePrompt, `"contract_quotes"`) {
		t.Fatalf("action class did not reach the response skeleton:\n%s", nodePrompt)
	}
	if !strings.Contains(nodePrompt, `<verbatim sentence>`) {
		t.Fatalf("contract_quotes shape missing from the skeleton:\n%s", nodePrompt)
	}
	// The narrowing must survive prompt rendering, not just field construction.
	if strings.Contains(nodePrompt, "failure_policy") {
		t.Fatalf("undeclared contract field reached the prompt:\n%s", nodePrompt)
	}
}
