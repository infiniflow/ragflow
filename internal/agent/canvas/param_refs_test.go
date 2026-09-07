// Tests for the param-level reference extraction added in #19325.
// Mirrors the Python `ComponentBase.param_refs` + `get_dependency_ids`
// shape introduced in PR #19282.

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
