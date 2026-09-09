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

package task

import (
	"testing"
)

// TestBuildDebugResultDSL locks the contract that the Go debug log's END-marker
// `dsl` must satisfy so the front-end "View result" page renders parsed chunks.
//
// The shape is the faithful Go analogue of Python's `Graph.__str__`
// (agent/canvas.py:119) + the END-marker `dsl` attachment (rag/flow/pipeline.py:98):
// for every component in the DSL we emit `dsl.components[<id>].obj` carrying
// `component_name` and `params.outputs[<format>].value`, plus `downstream` and
// `graph.nodes[].data.name`. The front-end reads exactly these keys
// (web/src/pages/dataflow-result/parser.tsx:41, hooks.ts:215).
func TestBuildDebugResultDSL(t *testing.T) {
	const (
		compA = "my.Parser"
		compB = "my.Tokenizer"
	)
	dsl := `{
		"dsl": {
			"components": {
				"begin": {"obj": {"component_name": "Begin", "params": {}}, "downstream": ["a"]},
				"a": {"obj": {"component_name": "` + compA + `", "params": {"setups": {"pdf": {"parse_method": "general"}}}}, "upstream": ["begin"], "downstream": ["b"]},
				"b": {"obj": {"component_name": "` + compB + `", "params": {"field_name": "content"}}, "upstream": ["a"]}
			},
			"graph": {"nodes": [
				{"id": "begin", "data": {"name": "开始"}},
				{"id": "a", "data": {"name": "解析"}},
				{"id": "b", "data": {"name": "分词"}}
			]},
			"path": ["begin", "a", "b"],
			"task_id": "task-42",
			"canvas_type": "dataflow"
		}
	}`

	// Run output: keyed by component id (state.go:284 `out[ref]=v`). `a` emits
	// chunks, `b` emits plain text, `begin` carries only the TrackElapsed
	// bookkeeping pair — the shape real no-payload components (e.g. File)
	// produce, since the canvas wraps every component body in TrackElapsed.
	output := map[string]any{
		"a": map[string]any{
			"chunks": []any{
				map[string]any{"text": "hello", "vector": []float64{0.1, 0.2}},
			},
			"_elapsed_time": 0.35,
			"_created_time": 100.0,
		},
		"b":     map[string]any{"text": "plain", "_elapsed_time": 0.02},
		"begin": map[string]any{"_elapsed_time": 0.01, "_created_time": 99.0},
	}

	result, err := BuildDebugResultDSL(dsl, output)
	if err != nil {
		t.Fatalf("BuildDebugResultDSL: %v", err)
	}

	components, ok := result["components"].(map[string]any)
	if !ok {
		t.Fatalf("result.components missing or wrong type: %#v", result["components"])
	}
	if len(components) != 3 {
		t.Fatalf("want 3 components (begin,a,b), got %d: %#v", len(components), components)
	}

	// Non-components top-level keys are carried verbatim (agent/canvas.py:126):
	// the persisted log DSL round-tripped by the rerun flow must not drop
	// path/task_id/canvas_type/graph.
	if p, _ := result["path"].([]any); len(p) != 3 || p[0] != "begin" || p[2] != "b" {
		t.Errorf("result.path=%#v want [begin a b]", result["path"])
	}
	if tid, _ := result["task_id"].(string); tid != "task-42" {
		t.Errorf("result.task_id=%v want task-42", result["task_id"])
	}
	if ct, _ := result["canvas_type"].(string); ct != "dataflow" {
		t.Errorf("result.canvas_type=%v want dataflow", result["canvas_type"])
	}
	if g := result["graph"]; g == nil {
		t.Error("result.graph must be carried, got nil")
	}

	// --- component "a": must carry chunks under params.outputs ---
	a, ok := components["a"].(map[string]any)
	if !ok {
		t.Fatalf("components.a wrong type: %#v", components["a"])
	}
	aObj, ok := a["obj"].(map[string]any)
	if !ok {
		t.Fatalf("components.a.obj wrong type: %#v", a["obj"])
	}
	if name, _ := aObj["component_name"].(string); name != compA {
		t.Errorf("components.a.obj.component_name=%v want %q", name, compA)
	}
	// downstream preserved
	if down, _ := a["downstream"].(string); down != "b" {
		// accept []any form too
		if dl, _ := a["downstream"].([]any); len(dl) != 1 || dl[0] != "b" {
			t.Errorf("components.a.downstream=%v want [b]", a["downstream"])
		}
	}
	aParams, ok := aObj["params"].(map[string]any)
	if !ok {
		t.Fatalf("components.a.obj.params wrong type: %#v", aObj["params"])
	}
	aOutputs, ok := aParams["outputs"].(map[string]any)
	if !ok {
		t.Fatalf("components.a.obj.params.outputs wrong type: %#v", aParams["outputs"])
	}
	// output_format selects "chunks"
	of, ok := aOutputs["output_format"].(map[string]any)
	if !ok || of["value"] != "chunks" {
		t.Errorf("components.a output_format=%#v want {value:\"chunks\"}", aOutputs["output_format"])
	}
	// The outputs wrapper records str(type(value)) type strings
	// (agent/component/base.py:467), not "".
	if of["type"] != "<class 'str'>" {
		t.Errorf("components.a output_format.type=%#v want <class 'str'>", of["type"])
	}
	// TrackElapsed bookkeeping rides along as plain {value, type} entries so
	// the front-end timeline renders per-node elapsed times.
	if et, _ := aOutputs["_elapsed_time"].(map[string]any); et["value"] != 0.35 {
		t.Errorf("components.a outputs._elapsed_time=%#v want {value:0.35}", aOutputs["_elapsed_time"])
	}
	if ct, _ := aOutputs["_created_time"].(map[string]any); ct["value"] != 100.0 {
		t.Errorf("components.a outputs._created_time=%#v want {value:100}", aOutputs["_created_time"])
	}
	if et, _ := aOutputs["_elapsed_time"].(map[string]any); et["type"] != "<class 'float'>" {
		t.Errorf("components.a outputs._elapsed_time.type=%#v want <class 'float'>", aOutputs["_elapsed_time"])
	}
	chunksOut, ok := aOutputs["chunks"].(map[string]any)
	if !ok {
		t.Fatalf("components.a outputs.chunks wrong type: %#v", aOutputs["chunks"])
	}
	chunksVal, _ := chunksOut["value"].([]any)
	if len(chunksVal) != 1 {
		t.Fatalf("components.a outputs.chunks.value len=%d want 1", len(chunksVal))
	}
	if chunksOut["type"] != "<class 'list'>" {
		t.Errorf("components.a outputs.chunks.type=%#v want <class 'list'>", chunksOut["type"])
	}
	chunk0, _ := chunksVal[0].(map[string]any)
	if chunk0["text"] != "hello" {
		t.Errorf("chunk text=%v want hello", chunk0["text"])
	}
	// VECTOR STRIPPING: Python's serialized obj excludes raw vectors; verify gone.
	if _, exists := chunk0["vector"]; exists {
		t.Errorf("vector key must be stripped from chunk payload, but it is present: %#v", chunk0)
	}
	// static params (setups) carried through, matching Python's cpn["params"] copy.
	if setups, _ := aParams["setups"].(map[string]any); setups == nil {
		t.Errorf("static params.setups must be carried from DSL, got %#v", aParams)
	}

	// --- component "b": plain text format ---
	b, _ := components["b"].(map[string]any)
	bObj, _ := b["obj"].(map[string]any)
	bParams, _ := bObj["params"].(map[string]any)
	bOutputs, _ := bParams["outputs"].(map[string]any)
	if bOutputs["output_format"].(map[string]any)["value"] != "text" {
		t.Errorf("components.b output_format=%#v want {value:\"text\"}", bOutputs["output_format"])
	}
	if tt := bOutputs["text"].(map[string]any)["type"]; tt != "<class 'str'>" {
		t.Errorf("components.b outputs.text.type=%#v want <class 'str'>", tt)
	}
	// static params.field_name carried (used by front-end chunk text key).
	if fn, _ := bParams["field_name"].(string); fn != "content" {
		t.Errorf("components.b params.field_name=%v want content", fn)
	}

	// --- component "begin": no recognized payload format, so no output_format
	// is invented — but the TrackElapsed bookkeeping keys must still surface
	// as outputs so the timeline shows its elapsed time. ---
	begin, _ := components["begin"].(map[string]any)
	beginObj, _ := begin["obj"].(map[string]any)
	beginParams, _ := beginObj["params"].(map[string]any)
	beginOutputs, ok := beginParams["outputs"].(map[string]any)
	if !ok {
		t.Fatalf("begin (bookkeeping-only output) must still carry params.outputs: %#v", beginParams)
	}
	if _, exists := beginOutputs["output_format"]; exists {
		t.Errorf("begin has no payload format, output_format must be absent: %#v", beginOutputs)
	}
	if et, _ := beginOutputs["_elapsed_time"].(map[string]any); et["value"] != 0.01 {
		t.Errorf("begin outputs._elapsed_time=%#v want {value:0.01}", beginOutputs["_elapsed_time"])
	}

	// --- graph.nodes copied through ---
	graph, ok := result["graph"].(map[string]any)
	if !ok {
		t.Fatalf("result.graph missing: %#v", result["graph"])
	}
	nodes, _ := graph["nodes"].([]any)
	if len(nodes) != 3 {
		t.Fatalf("graph.nodes len=%d want 3", len(nodes))
	}
}

// TestBuildDebugResultDSL_NestedState locks the REAL pipeline output shape.
//
// Unlike TestBuildDebugResultDSL (which uses a flat keyed-by-id map for
// readability), the production `pipe.Run` return value nests each component's
// outputs under output["state"][<componentID>] — see finalizeResult
// (pipeline.go:636, `merged["state"] = runState.Snapshot()`) and
// CanvasState.Snapshot() (state.go:296), which returns
// map[string]map[string]any keyed by cpn id. statePost (scheduler.go:229)
// writes each component's top-level output keys there via SetVar(cpnID, k, v).
//
// A prior bug read output[id] at the TOP level, which is always nil for the
// real run, so every component's params.outputs stayed empty and the front-end
// "View result" rendered blank tabs. This test pins the nested lookup so that
// regression cannot recur.
func TestBuildDebugResultDSL_NestedState(t *testing.T) {
	const (
		compA = "my.Parser"
		compB = "my.Tokenizer"
	)
	dsl := `{
		"dsl": {
			"components": {
				"begin": {"obj": {"component_name": "Begin", "params": {}}, "downstream": ["a"]},
				"a": {"obj": {"component_name": "` + compA + `", "params": {"setups": {"pdf": {"parse_method": "general"}}}}, "upstream": ["begin"], "downstream": ["b"]},
				"b": {"obj": {"component_name": "` + compB + `", "params": {"field_name": "content"}}, "upstream": ["a"]}
			},
			"graph": {"nodes": [
				{"id": "begin", "data": {"name": "开始"}},
				{"id": "a", "data": {"name": "解析"}},
				{"id": "b", "data": {"name": "分词"}}
			]}
		}
	}`

	// REAL shape: Snapshot() returns map[string]map[string]any (state.go:296),
	// so state is typed map[string]map[string]any at runtime — NOT
	// map[string]any. Using the real type here is what makes this test a true
	// regression guard: a single-type assertion in lookupComponentOutput would
	// pass a map[string]any-shaped test yet fail the production output.
	output := map[string]any{
		"state": map[string]map[string]any{
			"a": {
				"chunks": []any{
					map[string]any{"text": "hello", "vector": []float64{0.1, 0.2}},
				},
			},
			"b": {"text": "plain", "_elapsed_time": 0.02},
		},
	}

	result, err := BuildDebugResultDSL(dsl, output)
	if err != nil {
		t.Fatalf("BuildDebugResultDSL: %v", err)
	}
	components, ok := result["components"].(map[string]any)
	if !ok {
		t.Fatalf("result.components missing: %#v", result["components"])
	}

	// "a" must carry chunks under params.outputs even though the run output is
	// nested under output["state"]["a"].
	aObj := components["a"].(map[string]any)["obj"].(map[string]any)
	aParams := aObj["params"].(map[string]any)
	aOutputs, ok := aParams["outputs"].(map[string]any)
	if !ok {
		t.Fatalf("NESTED-SHAPE REGRESSION: components.a params.outputs missing; "+
			"BuildDebugResultDSL must read output[\"state\"][id], got params=%#v", aParams)
	}
	if of, _ := aOutputs["output_format"].(map[string]any); of["value"] != "chunks" {
		t.Errorf("components.a output_format=%#v want {value:\"chunks\"}", aOutputs["output_format"])
	}
	chunksVal := aOutputs["chunks"].(map[string]any)["value"].([]any)
	if len(chunksVal) != 1 {
		t.Fatalf("components.a outputs.chunks.value len=%d want 1", len(chunksVal))
	}
	chunk0 := chunksVal[0].(map[string]any)
	if chunk0["text"] != "hello" {
		t.Errorf("chunk text=%v want hello", chunk0["text"])
	}
	if _, exists := chunk0["vector"]; exists {
		t.Errorf("vector key must be stripped, but present: %#v", chunk0)
	}

	// "b": plain text nested under output["state"]["b"].
	bParams := components["b"].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
	bOutputs, ok := bParams["outputs"].(map[string]any)
	if !ok {
		t.Fatalf("NESTED-SHAPE REGRESSION: components.b params.outputs missing; "+
			"got params=%#v", bParams)
	}
	if of, _ := bOutputs["output_format"].(map[string]any); of["value"] != "text" {
		t.Errorf("components.b output_format=%#v want {value:\"text\"}", bOutputs["output_format"])
	}
	// Bookkeeping keys must resolve through the SAME nested-state lookup.
	if et, _ := bOutputs["_elapsed_time"].(map[string]any); et["value"] != 0.02 {
		t.Errorf("components.b outputs._elapsed_time=%#v want {value:0.02}", bOutputs["_elapsed_time"])
	}
}

// TestLookupComponentOutput locks the resolution rules of
// lookupComponentOutput: the production run output nests each component under
// output["state"][<id>] (finalizeResult + Snapshot), so that is the primary
// lookup; a flat keyed-by-id shape at the top level is the documented fallback
// for tests and any non-Snapshot caller. These branches are defensive and must
// keep behaving if the run-output envelope changes.
func TestLookupComponentOutput(t *testing.T) {
	const id = "a"

	// 1. nested present -> returns the nested value (primary path).
	out1 := map[string]any{"state": map[string]any{id: map[string]any{"chunks": "x"}}}
	if got := lookupComponentOutput(out1, id); got == nil {
		t.Errorf("case1: nested present, want non-nil, got nil")
	}

	// 2. nested present but NOT a map -> must fall back to top-level.
	out2 := map[string]any{"state": "not-a-map", id: map[string]any{"text": "y"}}
	if got := lookupComponentOutput(out2, id); got == nil {
		t.Errorf("case2: state wrong type, must fall back to top-level, got nil")
	} else if m, _ := got.(map[string]any); m["text"] != "y" {
		t.Errorf("case2: fallback got %#v want {text:\"y\"}", got)
	}

	// 3. nested present, id missing in state but present at top-level -> fallback.
	out3 := map[string]any{"state": map[string]any{"other": map[string]any{}}, id: map[string]any{"text": "z"}}
	if got := lookupComponentOutput(out3, id); got == nil {
		t.Errorf("case3: id missing in state, must fall back to top-level, got nil")
	} else if m, _ := got.(map[string]any); m["text"] != "z" {
		t.Errorf("case3: fallback got %#v want {text:\"z\"}", got)
	}

	// 4. nested present, id missing everywhere -> nil (safe empty).
	out4 := map[string]any{"state": map[string]any{"other": map[string]any{}}}
	if got := lookupComponentOutput(out4, id); got != nil {
		t.Errorf("case4: id missing everywhere, want nil, got %#v", got)
	}

	// 5. no "state" key at all -> flat top-level lookup.
	out5 := map[string]any{id: map[string]any{"json": "j"}}
	if got := lookupComponentOutput(out5, id); got == nil {
		t.Errorf("case5: flat-only, want non-nil, got nil")
	}
}

// TestDeepCopyStrip_StripsVectorsFromMapSlice locks the regression for review
// comment #5: real component chunk output is []map[string]any (as produced by
// ChunkDocsToMaps), which the type switch MUST recurse into AND strip vectorKeys
// from. Previously []map[string]any fell into the default branch — no recursion,
// no stripping — so embedding vectors leaked into the Redis debug log and the
// copy shared mutable state with the source.
func TestDeepCopyStrip_StripsVectorsFromMapSlice(t *testing.T) {
	src := []map[string]any{
		{
			"text":       "real chunk",
			"vector":     []float64{0.1, 0.2},
			"embedding":  []float64{0.3},
			"q_1024_vec": []float64{0.7},
			"nested":     map[string]any{"q_vec": []float64{0.9}, "q_4_vec": []float64{0.8}, "keep": "x"},
		},
		{"text": "second", "feature": []float64{0.4}},
	}

	got := deepCopy(src, true)

	// Returned slice must be a deep copy (new []any holding new maps), not the
	// original slice/map identity.
	cp, ok := got.([]any)
	if !ok {
		t.Fatalf("deepCopy(src, true) returned %T, want []any", got)
	}
	if len(cp) != 2 {
		t.Fatalf("len=%d want 2", len(cp))
	}

	// First chunk: vector & embedding dropped, text kept, nested vector dropped.
	c0, ok := cp[0].(map[string]any)
	if !ok {
		t.Fatalf("element 0 type %T, want map[string]any", cp[0])
	}
	if _, exists := c0["vector"]; exists {
		t.Errorf("vector must be stripped, but present: %#v", c0)
	}
	if _, exists := c0["embedding"]; exists {
		t.Errorf("embedding must be stripped, but present: %#v", c0)
	}
	if _, exists := c0["q_1024_vec"]; exists {
		t.Errorf("q_1024_vec (dimension-scoped vector key) must be stripped, but present: %#v", c0)
	}
	if c0["text"] != "real chunk" {
		t.Errorf("text=%v want 'real chunk'", c0["text"])
	}
	nested, _ := c0["nested"].(map[string]any)
	if _, exists := nested["q_vec"]; exists {
		t.Errorf("nested q_vec must be stripped, but present: %#v", nested)
	}
	if _, exists := nested["q_4_vec"]; exists {
		t.Errorf("nested q_4_vec (dimension-scoped vector key) must be stripped, but present: %#v", nested)
	}
	if nested["keep"] != "x" {
		t.Errorf("nested.keep=%v want x", nested["keep"])
	}

	// Second chunk: feature (a vectorKey) stripped, text kept.
	c1, ok := cp[1].(map[string]any)
	if !ok {
		t.Fatalf("element 1 type %T, want map[string]any", cp[1])
	}
	if _, exists := c1["feature"]; exists {
		t.Errorf("feature must be stripped, but present: %#v", c1)
	}
	if c1["text"] != "second" {
		t.Errorf("text=%v want 'second'", c1["text"])
	}

	// Mutation isolation: mutating the copy must not touch the source.
	c0["text"] = "mutated"
	if src[0]["text"] != "real chunk" {
		t.Errorf("copy mutation leaked into source: %#v", src[0])
	}
}

// TestDeepCopy_MapSliceIsDeep locks that deepCopy also recurses []map[string]any
// (structure preservation, no vector-strip concern for plain deepCopy).
func TestDeepCopy_MapSliceIsDeep(t *testing.T) {
	src := []map[string]any{{"text": "a", "n": map[string]any{"v": 1}}}
	got := deepCopy(src, false)
	cp, ok := got.([]any)
	if !ok {
		t.Fatalf("deepCopy returned %T, want []any", got)
	}
	m, ok := cp[0].(map[string]any)
	if !ok {
		t.Fatalf("element type %T, want map[string]any", cp[0])
	}
	if m["text"] != "a" {
		t.Errorf("text=%v want a", m["text"])
	}
	if n, _ := m["n"].(map[string]any); n["v"] != 1 {
		t.Errorf("nested not copied: %#v", m["n"])
	}
	// isolate
	m["text"] = "mut"
	if src[0]["text"] != "a" {
		t.Errorf("deepCopy shares mutable state: %#v", src[0])
	}
}

// TestDetectFormat_Priority locks the format selection order
// (chunks > json > text > html > Markdown) used to pick a component's payload
// key. A component that emits multiple recognized keys must surface the
// highest-priority one, matching NormalizeChunks and the front-end tab render.
func TestDetectFormat_Priority(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want string
	}{
		{"chunks beats text", map[string]any{"text": "t", "chunks": "c"}, "chunks"},
		{"json beats text", map[string]any{"text": "t", "json": "j"}, "json"},
		{"text beats html", map[string]any{"html": "h", "text": "t"}, "text"},
		{"html beats markdown", map[string]any{"markdown": "m", "html": "h"}, "html"},
		{"markdown alone", map[string]any{"markdown": "m"}, "markdown"},
		{"only unrecognized -> empty", map[string]any{"ok": true}, ""},
		{"empty map -> empty", map[string]any{}, ""},
		{"nil -> empty", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := detectFormat(c.in)
			if got != c.want {
				t.Errorf("detectFormat(%#v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestPythonTypeName locks the type-string mapping: scalar kinds map to their
// Python scalar classes, and every sequence/mapping reports list/dict
// regardless of the Go element type — typed containers like []int must not
// leak Go type syntax into the persisted log.
func TestPythonTypeName(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, "<class 'NoneType'>"},
		{"bool", true, "<class 'bool'>"},
		{"string", "s", "<class 'str'>"},
		{"int", 3, "<class 'int'>"},
		{"int64", int64(3), "<class 'int'>"},
		{"float64", 0.5, "<class 'float'>"},
		{"[]any", []any{1, "a"}, "<class 'list'>"},
		{"[]map[string]any", []map[string]any{{"k": 1}}, "<class 'list'>"},
		{"typed slice []int", []int{1, 2}, "<class 'list'>"},
		{"array", [2]int{1, 2}, "<class 'list'>"},
		{"map[string]any", map[string]any{"k": 1}, "<class 'dict'>"},
		{"typed map", map[string]string{"k": "v"}, "<class 'dict'>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pythonTypeName(c.in); got != c.want {
				t.Errorf("pythonTypeName(%#v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
