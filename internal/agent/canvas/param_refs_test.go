// Tests for the param-level reference extraction added in #19325 and
// extended in #19325's follow-ups. Mirrors the Python
// `ComponentBase.param_refs` + `get_dependency_ids` shape introduced in
// PR #19282.

package canvas

import "testing"

func TestParamRefDependencies_VariableAggregatorDictShape(t *testing.T) {
	// Engine-decoded JSON shape: `groups` is []any, each entry is a
	// `map[string]any`, each `variables[j]` is `{"value": "cpn@field"}`.
	params := map[string]any{
		"groups": []any{
			map[string]any{
				"group_name": "out_a",
				"variables": []any{
					map[string]any{"value": "src_a@content"},
					map[string]any{"value": "src_b@content"},
				},
			},
			map[string]any{
				"group_name": "out_b",
				"variables": []any{
					map[string]any{"value": "src_c@content"},
				},
			},
		},
	}
	got := paramRefDependencies("VariableAggregator", params)
	want := []string{"src_a@content", "src_b@content", "src_c@content"}
	if !equalStringSlices(got, want) {
		t.Fatalf("paramRefDependencies: got %v, want %v", got, want)
	}
}

func TestParamRefDependencies_VariableAggregatorTypedShape(t *testing.T) {
	// Hand-built shape (test/direct construction): groups is
	// []map[string]any, variables is []map[string]any. The Go runtime
	// can also fall back to the typed form when callers skip JSON.
	params := map[string]any{
		"groups": []map[string]any{
			{
				"group_name": "out_a",
				"variables": []map[string]any{
					{"value": "src_a@content"},
				},
			},
		},
	}
	got := paramRefDependencies("VariableAggregator", params)
	want := []string{"src_a@content"}
	if !equalStringSlices(got, want) {
		t.Fatalf("paramRefDependencies: got %v, want %v", got, want)
	}
}

func TestParamRefDependencies_VariableAggregatorPlainStringShape(t *testing.T) {
	// The Python override accepts plain strings (not just `{"value": ...}`
	// dicts); the SDK sometimes drops the dict wrapper, so the
	// scheduler must too. Mirrors the Python `isinstance(selector, dict)
	// else selector` branch.
	params := map[string]any{
		"groups": []any{
			map[string]any{
				"group_name": "out_a",
				"variables": []any{
					"src_a@content",
				},
			},
		},
	}
	got := paramRefDependencies("VariableAggregator", params)
	want := []string{"src_a@content"}
	if !equalStringSlices(got, want) {
		t.Fatalf("paramRefDependencies: got %v, want %v", got, want)
	}
}

func TestParamRefDependencies_UnknownComponentReturnsEmpty(t *testing.T) {
	// Components without a param_refs equivalent (e.g. LLM, Agent)
	// return an empty list. The scheduler continues to rely on the
	// drawn-edge list for those.
	got := paramRefDependencies("LLM", map[string]any{"model_type": "chat"})
	if len(got) != 0 {
		t.Fatalf("expected empty for unknown component, got %v", got)
	}
}

func TestParamRefDependencies_NilParamsAndMissingGroups(t *testing.T) {
	// Defensive: nil params and a missing `groups` key both return
	// nil rather than panicking. The Python equivalent is
	// `self._param.groups` with a None default.
	if got := paramRefDependencies("VariableAggregator", nil); got != nil {
		t.Fatalf("nil params: got %v, want nil", got)
	}
	if got := paramRefDependencies("VariableAggregator", map[string]any{}); got != nil {
		t.Fatalf("missing groups: got %v, want nil", got)
	}
}

func TestParamRefDependencies_EmptyValueStringSkipped(t *testing.T) {
	// A selector that decodes to an empty value string is skipped
	// rather than becoming a "" edge. Defensive against malformed
	// DSL where the value field is missing.
	params := map[string]any{
		"groups": []any{
			map[string]any{
				"group_name": "out_a",
				"variables": []any{
					map[string]any{"value": ""},
					map[string]any{"value": "src_a@content"},
					map[string]any{"other_key": "ignored"},
				},
			},
		},
	}
	got := paramRefDependencies("VariableAggregator", params)
	want := []string{"src_a@content"}
	if !equalStringSlices(got, want) {
		t.Fatalf("paramRefDependencies: got %v, want %v", got, want)
	}
}

func TestParseParamRefComponent(t *testing.T) {
	// parseParamRefComponent returns the upstream id embedded in a
	// `cpnID@field` selector, or "" when the selector has no `@` (a
	// local template variable, not a cross-component dependency) or
	// when the `@` is at position 0 (which would mean a `@field` with
	// no component id; not a valid ref).
	cases := []struct {
		ref  string
		want string
	}{
		{"src_a@content", "src_a"},
		{"src_a@content@more", "src_a"}, // first @ wins
		{"@content", ""},                // @ at position 0 → no upstream
		{"just_a_field", ""},            // no @ at all → no upstream
		{"", ""},                        // empty → no upstream
		{"src_a@", "src_a"},             // @ at end still gives an upstream
	}
	for _, c := range cases {
		if got := parseParamRefComponent(c.ref); got != c.want {
			t.Errorf("parseParamRefComponent(%q): got %q, want %q", c.ref, got, c.want)
		}
	}
}

func TestIsVariableAggregator(t *testing.T) {
	// Case-insensitive match mirrors the registry's name normalisation.
	if !isVariableAggregator("VariableAggregator") {
		t.Fatalf("expected exact case match")
	}
	if !isVariableAggregator("variableaggregator") {
		t.Fatalf("expected lower case match")
	}
	if !isVariableAggregator("VARIABLEAGGREGATOR") {
		t.Fatalf("expected upper case match")
	}
	if isVariableAggregator("LLM") {
		t.Fatalf("LLM should not match")
	}
}

func TestParamRefDependencies_SwitchLeftAndRight(t *testing.T) {
	// Switch's `evaluateClause` resolves both `left` and `right`
	// through `runtime.ResolveTemplate`. A `{{other@content}}` on
	// either side is a real dependency the drawn-edge list does
	// not capture, so the scheduler must add it as an exec-only
	// edge.
	params := map[string]any{
		"conditions": []any{
			map[string]any{
				"op": "and",
				"to": "out_a",
				"clauses": []any{
					map[string]any{
						"left":  "src_a@content",
						"op":    "==",
						"right": "src_b@content",
					},
					map[string]any{
						"left":  "src_c@content",
						"op":    ">",
						"right": 42, // non-string right is a real comparison value, not a ref
					},
				},
			},
		},
	}
	got := paramRefDependencies("Switch", params)
	want := []string{"src_a@content", "src_b@content", "src_c@content"}
	if !equalStringSlices(got, want) {
		t.Fatalf("paramRefDependencies(Switch): got %v, want %v", got, want)
	}
}

func TestParamRefDependencies_SwitchMissingConditions(t *testing.T) {
	// Defensive: a Switch with no conditions key (or a nil params)
	// returns an empty list. The Python fix accepts the same shape.
	if got := paramRefDependencies("Switch", nil); got != nil {
		t.Fatalf("nil params: got %v, want nil", got)
	}
	if got := paramRefDependencies("Switch", map[string]any{}); got != nil {
		t.Fatalf("missing conditions: got %v, want nil", got)
	}
}

func TestParamRefDependencies_SwitchNoRefsReturnsEmpty(t *testing.T) {
	// A Switch with only literal comparisons (no `{{...}}` selectors)
	// has no param-refs and returns an empty list. The drawn-edge
	// list carries the dependency.
	params := map[string]any{
		"conditions": []any{
			map[string]any{
				"op": "and",
				"clauses": []any{
					map[string]any{"left": "raw text", "op": "==", "right": "other raw"},
					map[string]any{"left": "field", "op": "==", "right": "value"},
				},
			},
		},
	}
	got := paramRefDependencies("Switch", params)
	if len(got) != 0 {
		t.Fatalf("expected empty for literal-only Switch, got %v", got)
	}
}

func TestParamRefDependencies_CategorizeQueryAndItems(t *testing.T) {
	// Categorize's `resolveCategorizeQuery` resolves the query string
	// through `state.GetVar`; the Invoke path iterates items and
	// adds them to the prompt as context. A ref on either is a real
	// dependency.
	params := map[string]any{
		"query": "src_query@content",
		"items": []any{
			"src_item1@content",
			"src_item2@content",
		},
	}
	got := paramRefDependencies("Categorize", params)
	want := []string{"src_query@content", "src_item1@content", "src_item2@content"}
	if !equalStringSlices(got, want) {
		t.Fatalf("paramRefDependencies(Categorize): got %v, want %v", got, want)
	}
}

func TestParamRefDependencies_CategorizeStringItems(t *testing.T) {
	// Hand-built shape: items is []string (the typed-slice variant
	// Categorize's Update also accepts).
	params := map[string]any{
		"query": "src@content",
		"items": []string{"a@content", "b@content"},
	}
	got := paramRefDependencies("Categorize", params)
	want := []string{"src@content", "a@content", "b@content"}
	if !equalStringSlices(got, want) {
		t.Fatalf("paramRefDependencies(Categorize []string items): got %v, want %v", got, want)
	}
}

func TestParamRefDependencies_CategorizeQueryOnly(t *testing.T) {
	// Items absent: only the query contributes.
	params := map[string]any{
		"query": "src@content",
	}
	got := paramRefDependencies("Categorize", params)
	want := []string{"src@content"}
	if !equalStringSlices(got, want) {
		t.Fatalf("paramRefDependencies(Categorize query-only): got %v, want %v", got, want)
	}
}

func TestParamRefDependencies_CategorizeNoRefsReturnsEmpty(t *testing.T) {
	// Categorize with a literal query and literal items returns an
	// empty list — the `sys.query` default and literal item lists
	// are common in real DSLs and have no param-refs.
	params := map[string]any{
		"query": "what kind of document is this?",
		"items": []any{"a plain string", "another plain string"},
	}
	got := paramRefDependencies("Categorize", params)
	if len(got) != 0 {
		t.Fatalf("expected empty for literal-only Categorize, got %v", got)
	}
}

func TestIsSwitch(t *testing.T) {
	if !isSwitch("Switch") {
		t.Fatalf("expected exact case match")
	}
	if !isSwitch("switch") {
		t.Fatalf("expected lower case match")
	}
	if !isSwitch("SWITCH") {
		t.Fatalf("expected upper case match")
	}
	if isSwitch("Categorize") {
		t.Fatalf("Categorize should not match")
	}
}

func TestIsCategorize(t *testing.T) {
	if !isCategorize("Categorize") {
		t.Fatalf("expected exact case match")
	}
	if !isCategorize("categorize") {
		t.Fatalf("expected lower case match")
	}
	if !isCategorize("CATEGORIZE") {
		t.Fatalf("expected upper case match")
	}
	if isCategorize("Switch") {
		t.Fatalf("Switch should not match")
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
