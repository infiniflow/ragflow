//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// template_jinja_test.go — gonja-backed template tests.
//
// Three groups:
//
//   - ContainsJinjaSyntax: detection helpers (no gonja parse cost).
//   - ResolveTemplateJinja: gonja path directly. Verifies the
//     state-to-context flattening and basic Jinja2 syntax (filters,
//     ifs, comments).
//   - ResolveTemplateAuto: dispatcher. Verifies the fast path is
//     taken for the common case and gonja is taken when Jinja2
//     markers are present.

package runtime

import (
	"reflect"
	"strings"
	"testing"
)

// TestContainsJinjaSyntax_PlainReference: pure {{ref}} form does
// NOT trigger the gonja fallback — the regex fast path handles
// it.
func TestContainsJinjaSyntax_PlainReference(t *testing.T) {
	cases := []string{
		"hello {{name}}",
		"{{a.b}}",
		"{{ cpn_0@content }}",
		"no refs at all",
		"",
	}
	for _, c := range cases {
		if ContainsJinjaSyntax(c) {
			t.Errorf("ContainsJinjaSyntax(%q) = true, want false", c)
		}
	}
}

// TestContainsJinjaSyntax_StatementBlock: {% if %} triggers
// the fallback.
func TestContainsJinjaSyntax_StatementBlock(t *testing.T) {
	cases := []string{
		"{% if x %}yes{% endif %}",
		"before {% for i in list %}body{% endfor %} after",
	}
	for _, c := range cases {
		if !ContainsJinjaSyntax(c) {
			t.Errorf("ContainsJinjaSyntax(%q) = false, want true", c)
		}
	}
}

// TestContainsJinjaSyntax_Comment: {# comment #} triggers the
// fallback.
func TestContainsJinjaSyntax_Comment(t *testing.T) {
	if !ContainsJinjaSyntax("hello {# inline #} world") {
		t.Errorf("comment not detected as Jinja2 syntax")
	}
}

// TestContainsJinjaSyntax_FilterPipe: {{ x | upper }} triggers
// the fallback.
func TestContainsJinjaSyntax_FilterPipe(t *testing.T) {
	cases := []string{
		"{{ name | upper }}",
		"{{ x | filter('arg') }}",
	}
	for _, c := range cases {
		if !ContainsJinjaSyntax(c) {
			t.Errorf("filter pipe not detected in %q", c)
		}
	}
}

// TestResolveTemplateJinja_TopLevelVar: a top-level variable
// resolves from the state.
func TestResolveTemplateJinja_TopLevelVar(t *testing.T) {
	state := NewCanvasState("r1", "t1")
	state.SetVar("begin_0", "content", "hello from begin")
	out, err := ResolveTemplateJinja("{{ begin_0.content }}", state)
	if err != nil {
		t.Fatalf("ResolveTemplateJinja: %v", err)
	}
	if out != "hello from begin" {
		t.Errorf("output = %q, want \"hello from begin\"", out)
	}
}

// TestResolveTemplateJinja_NestedMap: nested maps are walked
// via gonja's standard dot syntax (no flatten needed).
func TestResolveTemplateJinja_NestedMap(t *testing.T) {
	state := NewCanvasState("r1", "t1")
	state.SetVar("agent_0", "user", map[string]any{
		"name": "alice",
		"role": "admin",
	})
	out, err := ResolveTemplateJinja("{{ agent_0.user.name }} / {{ agent_0.user.role }}", state)
	if err != nil {
		t.Fatalf("ResolveTemplateJinja: %v", err)
	}
	if out != "alice / admin" {
		t.Errorf("output = %q, want \"alice / admin\"", out)
	}
}

// TestResolveTemplateJinja_Filter: Jinja2 filters are honoured
// (verifies the gonja path is actually engaged).
func TestResolveTemplateJinja_Filter(t *testing.T) {
	state := NewCanvasState("r1", "t1")
	state.SetVar("begin_0", "content", "hello")
	out, err := ResolveTemplateJinja("{{ begin_0.content | upper }}", state)
	if err != nil {
		t.Fatalf("ResolveTemplateJinja: %v", err)
	}
	if out != "HELLO" {
		t.Errorf("output = %q, want \"HELLO\" (upper filter)", out)
	}
}

// TestResolveTemplateJinja_IfStatement: an if/else conditional
// works under gonja.
func TestResolveTemplateJinja_IfStatement(t *testing.T) {
	state := NewCanvasState("r1", "t1")
	state.SetVar("begin_0", "flag", true)
	out, err := ResolveTemplateJinja(`{% if begin_0.flag %}yes{% else %}no{% endif %}`, state)
	if err != nil {
		t.Fatalf("ResolveTemplateJinja: %v", err)
	}
	if out != "yes" {
		t.Errorf("output = %q, want \"yes\"", out)
	}
}

// TestResolveTemplateJinja_NilState: a nil state surfaces a
// clear error (gonja would panic on a nil map).
func TestResolveTemplateJinja_NilState(t *testing.T) {
	_, err := ResolveTemplateJinja("{{ x }}", nil)
	if err == nil {
		t.Fatal("expected error for nil state")
	}
	if !strings.Contains(err.Error(), "nil canvas state") {
		t.Errorf("error = %v, want 'nil canvas state'", err)
	}
}

// TestResolveTemplateAuto_PlainReferenceFallsToFastPath: a
// template without Jinja2 markers must resolve via the regex
// fast path. We assert this indirectly: the result matches what
// the legacy ResolveTemplate returns.
func TestResolveTemplateAuto_PlainReferenceFallsToFastPath(t *testing.T) {
	state := NewCanvasState("r1", "t1")
	state.SetVar("begin_0", "content", "fast path")
	got, err := ResolveTemplateAuto("{{begin_0@content}}", state)
	if err != nil {
		t.Fatalf("ResolveTemplateAuto: %v", err)
	}
	want, _ := ResolveTemplate("{{begin_0@content}}", state)
	if got != want {
		t.Errorf("ResolveTemplateAuto (plain) = %q, ResolveTemplate = %q", got, want)
	}
}

// TestResolveTemplateAuto_FilterFallsToGonja: a template with a
// filter pipe must resolve via gonja (regex fast path can't
// handle filters).
func TestResolveTemplateAuto_FilterFallsToGonja(t *testing.T) {
	state := NewCanvasState("r1", "t1")
	state.SetVar("begin_0", "content", "auto")
	got, err := ResolveTemplateAuto("{{ begin_0.content | upper }}", state)
	if err != nil {
		t.Fatalf("ResolveTemplateAuto: %v", err)
	}
	if got != "AUTO" {
		t.Errorf("output = %q, want \"AUTO\" (filter applied via gonja path)", got)
	}
}

// TestFlattenMap_ShallowNesting: nested maps flatten to dotted
// keys; scalars pass through. Slices are passed through as-is
// (compared with reflect.DeepEqual because []any is not
// comparable with ==).
func TestFlattenMap_ShallowNesting(t *testing.T) {
	in := map[string]any{
		"a": "x",
		"b": map[string]any{
			"c": "y",
			"d": map[string]any{
				"e": 1,
			},
		},
		"f": []any{1, 2, 3},
	}
	out := flattenMap(in)
	if got := out["a"]; got != "x" {
		t.Errorf("out[a] = %v, want \"x\"", got)
	}
	if got := out["b.c"]; got != "y" {
		t.Errorf("out[b.c] = %v, want \"y\"", got)
	}
	if got := out["b.d.e"]; got != 1 {
		t.Errorf("out[b.d.e] = %v, want 1", got)
	}
	// f: slice — use reflect.DeepEqual to avoid the
	// "comparing uncomparable type" panic.
	if !reflect.DeepEqual(out["f"], []any{1, 2, 3}) {
		t.Errorf("out[f] = %v, want [1 2 3]", out["f"])
	}
}
