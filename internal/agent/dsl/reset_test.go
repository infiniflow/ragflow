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

package dsl

import (
	"reflect"
	"testing"
)

// TestResetForCanvas_ClearsPerRunState asserts the four top-level
// accumulators are reset to fresh empty slices, matching the Python
// `self.history = []` / `self.retrieval = []` / `self.memory = []`
// and the parent `Graph.reset()` `self.path = []` branches.
func TestResetForCanvas_ClearsPerRunState(t *testing.T) {
	in := map[string]any{
		"history":   []any{"m1", "m2"},
		"retrieval": []any{map[string]any{"doc": "x"}},
		"memory":    []any{"mem"},
		"path":      []any{"begin", "llm"},
	}
	got := ResetForCanvas(in)

	if v, ok := got["history"].([]any); !ok || len(v) != 0 {
		t.Errorf("history = %v (%T), want empty []any", got["history"], got["history"])
	}
	if v, ok := got["retrieval"].([]any); !ok || len(v) != 0 {
		t.Errorf("retrieval = %v (%T), want empty []any", got["retrieval"], got["retrieval"])
	}
	if v, ok := got["memory"].([]any); !ok || len(v) != 0 {
		t.Errorf("memory = %v (%T), want empty []any", got["memory"], got["memory"])
	}
	if v, ok := got["path"].([]any); !ok || len(v) != 0 {
		t.Errorf("path = %v (%T), want empty []any", got["path"], got["path"])
	}
}

// TestResetForCanvas_ZeroesSysGlobals walks every Python reset()
// branch for sys.* keys (string/int/float/list/dict/other).
func TestResetForCanvas_ZeroesSysGlobals(t *testing.T) {
	in := map[string]any{
		"globals": map[string]any{
			"sys.query":        "hello",
			"sys.conversation": 7,
			"sys.score":        0.85,
			"sys.history":      []any{"a", "b"},
			"sys.session_meta": map[string]any{"k": "v"},
			"sys.unknown_kind": struct{ X int }{X: 1},
			"user.preserve_me": "leave alone",
			// env.* is reset, not preserved: there is no matching
			// variables["preserve_me"] declaration, so the helper
			// falls into the "no declared default" branch and
			// zeroes the key to "" — matching the Python
			// `else: self.globals[k] = ""` line.
			"env.preserve_me": "leave alone",
		},
	}
	got := ResetForCanvas(in)
	g, ok := got["globals"].(map[string]any)
	if !ok {
		t.Fatalf("globals missing or wrong type: %T", got["globals"])
	}

	if g["sys.query"] != "" {
		t.Errorf("sys.query = %v, want \"\"", g["sys.query"])
	}
	// int / float → 0
	if v, ok := g["sys.conversation"].(int); !ok || v != 0 {
		t.Errorf("sys.conversation = %v (%T), want 0", g["sys.conversation"], g["sys.conversation"])
	}
	if v, ok := g["sys.score"].(float64); !ok || v != 0 {
		t.Errorf("sys.score = %v (%T), want 0", g["sys.score"], g["sys.score"])
	}
	if v, ok := g["sys.history"].([]any); !ok || len(v) != 0 {
		t.Errorf("sys.history = %v (%T), want []any{}", g["sys.history"], g["sys.history"])
	}
	if v, ok := g["sys.session_meta"].(map[string]any); !ok || len(v) != 0 {
		t.Errorf("sys.session_meta = %v (%T), want map[string]any{}", g["sys.session_meta"], g["sys.session_meta"])
	}
	if g["sys.unknown_kind"] != nil {
		t.Errorf("sys.unknown_kind = %v, want nil", g["sys.unknown_kind"])
	}
	// Non-sys./env. keys must NOT be touched.
	if g["user.preserve_me"] != "leave alone" {
		t.Errorf("user.preserve_me = %v, want \"leave alone\"", g["user.preserve_me"])
	}
	// env.preserve_me has no variables["preserve_me"] entry, so it
	// falls through to the Python `else: self.globals[k] = ""` line.
	if g["env.preserve_me"] != "" {
		t.Errorf("env.preserve_me = %v, want \"\"", g["env.preserve_me"])
	}
}

// TestResetForCanvas_RestoresEnvFromVariables covers the env.* branch:
//   - declared variable with value → globals restored to that value
//   - declared variable without value + numeric type → 0
//   - declared variable without value + boolean type → false
//   - declared variable without value + object type → {}
//   - declared variable without value + array type → []
//   - declared variable without value + string type → ""
//   - undeclared env.* key → ""
func TestResetForCanvas_RestoresEnvFromVariables(t *testing.T) {
	in := map[string]any{
		"variables": map[string]any{
			"with_value": map[string]any{
				"type":  "string",
				"value": "default-val",
			},
			"numeric": map[string]any{"type": "number"},
			"boolean": map[string]any{"type": "boolean"},
			"object":  map[string]any{"type": "object"},
			"arr":     map[string]any{"type": "array[string]"},
			"str":     map[string]any{"type": "string"},
		},
		"globals": map[string]any{
			"env.with_value": "stale",
			"env.numeric":    42,
			"env.boolean":    true,
			"env.object":     map[string]any{"k": "v"},
			"env.arr":        []any{"stale"},
			"env.str":        "stale",
			"env.undeclared": "stale",
		},
	}
	got := ResetForCanvas(in)
	g := got["globals"].(map[string]any)

	if g["env.with_value"] != "default-val" {
		t.Errorf("env.with_value = %v, want \"default-val\"", g["env.with_value"])
	}
	if v, ok := g["env.numeric"].(int); !ok || v != 0 {
		t.Errorf("env.numeric = %v (%T), want 0", g["env.numeric"], g["env.numeric"])
	}
	if v, ok := g["env.boolean"].(bool); !ok || v != false {
		t.Errorf("env.boolean = %v (%T), want false", g["env.boolean"], g["env.boolean"])
	}
	if v, ok := g["env.object"].(map[string]any); !ok || len(v) != 0 {
		t.Errorf("env.object = %v (%T), want empty map", g["env.object"], g["env.object"])
	}
	if v, ok := g["env.arr"].([]any); !ok || len(v) != 0 {
		t.Errorf("env.arr = %v (%T), want empty slice", g["env.arr"], g["env.arr"])
	}
	if v, _ := g["env.str"].(string); v != "" {
		t.Errorf("env.str = %v, want \"\"", g["env.str"])
	}
	if g["env.undeclared"] != "" {
		t.Errorf("env.undeclared = %v, want \"\"", g["env.undeclared"])
	}
}

// TestResetForCanvas_PreservesGraphAndComponents asserts the
// "anything else in the DSL is left untouched" contract: graph,
// components, and other top-level keys are passed through to the
// returned DSL so a reset is non-destructive on structure.
func TestResetForCanvas_PreservesGraphAndComponents(t *testing.T) {
	graph := map[string]any{
		"nodes": []any{map[string]any{"id": "begin"}},
		"edges": []any{},
	}
	comps := map[string]any{
		"begin": map[string]any{"obj": map[string]any{"component_name": "Begin"}},
	}
	in := map[string]any{
		"graph":      graph,
		"components": comps,
		"messages":   []any{"leave me alone"},
		"title":      "Untouched",
	}
	got := ResetForCanvas(in)

	if !reflect.DeepEqual(got["graph"], graph) {
		t.Errorf("graph mutated: got %v, want %v", got["graph"], graph)
	}
	if !reflect.DeepEqual(got["components"], comps) {
		t.Errorf("components mutated: got %v, want %v", got["components"], comps)
	}
	if !reflect.DeepEqual(got["messages"], []any{"leave me alone"}) {
		t.Errorf("messages mutated: got %v", got["messages"])
	}
	if got["title"] != "Untouched" {
		t.Errorf("title = %v, want \"Untouched\"", got["title"])
	}
}

// TestResetForCanvas_DefensiveCopy makes sure the input map is not
// mutated. The service layer feeds `row.DSL` straight from GORM into
// ResetForCanvas; mutating that in place would corrupt the entity in
// the calling goroutine and any in-flight readers of the same row.
func TestResetForCanvas_DefensiveCopy(t *testing.T) {
	in := map[string]any{
		"history": []any{"x"},
		"globals": map[string]any{
			"sys.query": "hello",
		},
	}
	_ = ResetForCanvas(in)

	if v, _ := in["history"].([]any); len(v) != 1 || v[0] != "x" {
		t.Errorf("input history mutated: %v", in["history"])
	}
	if g, _ := in["globals"].(map[string]any); g["sys.query"] != "hello" {
		t.Errorf("input globals mutated: %v", g["sys.query"])
	}
}

// TestResetForCanvas_NilAndEmptyDSL covers the safe-default branches:
// a nil input returns an empty map, and an input without a globals
// block is passed through without an injected nil/empty globals.
func TestResetForCanvas_NilAndEmptyDSL(t *testing.T) {
	if got := ResetForCanvas(nil); got == nil {
		t.Errorf("ResetForCanvas(nil) = nil, want non-nil empty map")
	}
	in := map[string]any{
		"graph":      map[string]any{"nodes": []any{}},
		"components": map[string]any{},
	}
	got := ResetForCanvas(in)
	if _, hasGlobals := got["globals"]; hasGlobals {
		t.Errorf("globals key injected: %v", got["globals"])
	}
}
