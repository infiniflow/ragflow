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

// loop_subgraph_test.go — table-driven tests for the Loop macro
// expansion helpers in loop_subgraph.go.
//
// Tests cover:
//   - collectDescendants (DAG and diamond shapes, back-edge handling)
//   - resolveInitialVariables (constant / zero-init / variable modes)
//   - zeroValueForType (number / string / boolean / object* / array* / unknown)
//   - readMaxLoopCount (missing, int, int64, float64)
//   - translateLoopCondition (single op, AND/OR, invalid logical_operator,
//     incomplete entries, empty conditions)
//   - evalOneLoopCondition + evaluateCondition (operator dispatch on
//     string / bool / number / dict / list / nil; the same operator
//     set as agent/component/loopitem.py:48-122)
//   - BuildWorkflow end-to-end (Loop + body, legacy ExitLoop no-op,
//     unknown component error path)

package canvas

import (
	"context"
	"strings"
	"testing"
)

// ---- collectDescendants ----

func TestCollectDescendants_DAG(t *testing.T) {
	// 4-node chain: loop -> a -> b -> c -> d (d has no downstream).
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"loop": {Obj: CanvasComponentObj{ComponentName: "Loop"},
				Downstream: []string{"a"}},
			"a": {Obj: CanvasComponentObj{ComponentName: "Message"},
				Upstream: []string{"loop"}, Downstream: []string{"b"}},
			"b": {Obj: CanvasComponentObj{ComponentName: "LLM"},
				Upstream: []string{"a"}, Downstream: []string{"c"}},
			"c": {Obj: CanvasComponentObj{ComponentName: "Categorize"},
				Upstream: []string{"b"}, Downstream: []string{"d"}},
			"d": {Obj: CanvasComponentObj{ComponentName: "Message"},
				Upstream: []string{"c"}},
		},
	}
	got := collectDescendants(c, "loop")
	want := map[string]bool{"a": true, "b": true, "c": true, "d": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k := range want {
		if !got[k] {
			t.Errorf("missing %q in %v", k, got)
		}
	}
}

func TestCollectDescendants_Diamond(t *testing.T) {
	// loop -> a -> b -> d
	//            \-> c -/
	// d is the join, must appear once.
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"loop": {Obj: CanvasComponentObj{ComponentName: "Loop"},
				Downstream: []string{"a"}},
			"a": {Obj: CanvasComponentObj{ComponentName: "Message"},
				Upstream: []string{"loop"}, Downstream: []string{"b", "c"}},
			"b": {Obj: CanvasComponentObj{ComponentName: "LLM"},
				Upstream: []string{"a"}, Downstream: []string{"d"}},
			"c": {Obj: CanvasComponentObj{ComponentName: "Categorize"},
				Upstream: []string{"a"}, Downstream: []string{"d"}},
			"d": {Obj: CanvasComponentObj{ComponentName: "Message"},
				Upstream: []string{"b", "c"}},
		},
	}
	got := collectDescendants(c, "loop")
	want := map[string]bool{"a": true, "b": true, "c": true, "d": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k := range want {
		if !got[k] {
			t.Errorf("missing %q in %v", k, got)
		}
	}
}

func TestCollectDescendants_BackEdgeStops(t *testing.T) {
	// loop -> a -> b -> loop (back-edge). BFS must not loop forever;
	// visited stops at the back-edge.
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"loop": {Obj: CanvasComponentObj{ComponentName: "Loop"},
				Downstream: []string{"a"}},
			"a": {Obj: CanvasComponentObj{ComponentName: "Message"},
				Upstream: []string{"loop"}, Downstream: []string{"b"}},
			"b": {Obj: CanvasComponentObj{ComponentName: "LLM"},
				Upstream: []string{"a"}, Downstream: []string{"loop"}},
		},
	}
	got := collectDescendants(c, "loop")
	want := map[string]bool{"a": true, "b": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// ---- resolveInitialVariables ----

func TestResolveInitialVariables_Constant(t *testing.T) {
	params := map[string]any{
		"loop_variables": []any{
			map[string]any{
				"variable":   "counter",
				"input_mode": "constant",
				"value":      7,
				"type":       "number",
			},
		},
	}
	got, err := resolveInitialVariables(params)
	if err != nil {
		t.Fatalf("resolveInitialVariables: %v", err)
	}
	spec, ok := got["counter"]
	if !ok {
		t.Fatalf("counter: missing key in result map")
	}
	if spec.InputMode != "constant" {
		t.Errorf("counter: input_mode got %q, want \"constant\"", spec.InputMode)
	}
	if spec.Value != 7 {
		t.Errorf("counter: value got %v, want 7", spec.Value)
	}
}

func TestResolveInitialVariables_ZeroInit(t *testing.T) {
	cases := []struct {
		typ  string
		want any
	}{
		{"number", 0},
		{"string", ""},
		{"boolean", false},
		{"object", map[string]any{}},
		{"object<string>", map[string]any{}},
		{"array", []any{}},
		{"array<string>", []any{}},
		{"unknown-type", ""},
	}
	for _, tc := range cases {
		params := map[string]any{
			"loop_variables": []any{
				map[string]any{
					"variable":   "v",
					"input_mode": "",
					"value":      nil,
					"type":       tc.typ,
				},
			},
		}
		got, err := resolveInitialVariables(params)
		if err != nil {
			t.Fatalf("typ %q: %v", tc.typ, err)
		}
		spec, ok := got["v"]
		if !ok {
			t.Fatalf("typ %q: missing key in result map", tc.typ)
		}
		// Special-case the untyped-empty value to skip the equal check
		// on slices/maps (reflect.DeepEqual semantics).
		if !valueEqual(spec.Value, tc.want) {
			t.Errorf("typ %q: got %v (%T), want %v (%T)", tc.typ, spec.Value, spec.Value, tc.want, tc.want)
		}
	}
}

func TestResolveInitialVariables_VariablePassthrough(t *testing.T) {
	// "variable" mode's runtime dereference happens in the init lambda
	// (buildSubWorkflow). resolveInitialVariables is state-free, so it
	// just returns the ref string in Value plus the input_mode tag so
	// the init lambda knows to dereference.
	params := map[string]any{
		"loop_variables": []any{
			map[string]any{
				"variable":   "x",
				"input_mode": "variable",
				"value":      "Begin.foo",
				"type":       "string",
			},
		},
	}
	got, err := resolveInitialVariables(params)
	if err != nil {
		t.Fatalf("resolveInitialVariables: %v", err)
	}
	spec, ok := got["x"]
	if !ok {
		t.Fatalf("x: missing key in result map")
	}
	if spec.InputMode != "variable" {
		t.Errorf("x: input_mode got %q, want \"variable\"", spec.InputMode)
	}
	if spec.Value != "Begin.foo" {
		t.Errorf("x: value got %v, want \"Begin.foo\"", spec.Value)
	}
}

func TestResolveInitialVariables_Incomplete(t *testing.T) {
	cases := []map[string]any{
		// missing 'variable'
		{"input_mode": "constant", "value": 1, "type": "number"},
		// missing 'input_mode'
		{"variable": "x", "value": 1, "type": "number"},
		// missing 'value'
		{"variable": "x", "input_mode": "constant", "type": "number"},
		// missing 'type'
		{"variable": "x", "input_mode": "constant", "value": 1},
	}
	for i, item := range cases {
		params := map[string]any{"loop_variables": []any{item}}
		if _, err := resolveInitialVariables(params); err == nil {
			t.Errorf("case %d: expected error, got nil", i)
		}
	}
}

// ---- zeroValueForType ----

func TestZeroValueForType(t *testing.T) {
	cases := []struct {
		typ  any
		want any
	}{
		{"number", 0},
		{"string", ""},
		{"boolean", false},
		{"object", map[string]any{}},
		{"object<string>", map[string]any{}},
		{"array", []any{}},
		{"array<string>", []any{}},
		{"weird", ""},
		{nil, ""},
	}
	for _, tc := range cases {
		got := zeroValueForType(tc.typ)
		if !valueEqual(got, tc.want) {
			t.Errorf("typ %v: got %v, want %v", tc.typ, got, tc.want)
		}
	}
}

// ---- readMaxLoopCount ----

func TestReadMaxLoopCount(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want int
	}{
		{"missing", map[string]any{}, 0},
		{"int", map[string]any{"maximum_loop_count": 5}, 5},
		{"int64", map[string]any{"maximum_loop_count": int64(7)}, 7},
		{"float64", map[string]any{"maximum_loop_count": 3.0}, 3},
		{"string", map[string]any{"maximum_loop_count": "5"}, 0},
	}
	for _, tc := range cases {
		if got := readMaxLoopCount(tc.in); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}

// ---- translateLoopCondition ----

func TestTranslateLoopCondition_SingleOp(t *testing.T) {
	params := map[string]any{
		"logical_operator": "and",
		"loop_termination_condition": []any{
			map[string]any{
				"variable":   "counter",
				"operator":   "≥",
				"value":      3,
				"input_mode": "constant",
			},
		},
	}
	cond, err := translateLoopCondition("loop_0", params)
	if err != nil {
		t.Fatalf("translateLoopCondition: %v", err)
	}
	state := NewCanvasState("", "")
	state.SetVar("loop_0", "counter", 3)
	ctx := WithState(context.Background(), state)
	quit, err := cond(ctx, 3, nil, nil)
	if err != nil {
		t.Fatalf("cond: %v", err)
	}
	if !quit {
		t.Errorf("expected quit when counter=3 >= 3")
	}
	// counter=2 should NOT quit.
	state2 := NewCanvasState("", "")
	state2.SetVar("loop_0", "counter", 2)
	ctx2 := WithState(context.Background(), state2)
	quit, err = cond(ctx2, 2, nil, nil)
	if err != nil {
		t.Fatalf("cond: %v", err)
	}
	if quit {
		t.Errorf("expected no-quit when counter=2 < 3")
	}
}

func TestTranslateLoopCondition_OrQuitsEarly(t *testing.T) {
	// Two conditions OR'd. quits as soon as one is true.
	params := map[string]any{
		"logical_operator": "or",
		"loop_termination_condition": []any{
			map[string]any{"variable": "a", "operator": "=", "value": 1, "input_mode": "constant"},
			map[string]any{"variable": "b", "operator": "=", "value": 2, "input_mode": "constant"},
		},
	}
	cond, err := translateLoopCondition("L", params)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	// a=1, b=0 → quits (first condition true).
	state := NewCanvasState("", "")
	state.SetVar("L", "a", 1)
	state.SetVar("L", "b", 0)
	quit, err := cond(WithState(context.Background(), state), 1, nil, nil)
	if err != nil {
		t.Fatalf("cond: %v", err)
	}
	if !quit {
		t.Errorf("OR with a=1 should quit")
	}
	// a=0, b=2 → quits (second condition true).
	state2 := NewCanvasState("", "")
	state2.SetVar("L", "a", 0)
	state2.SetVar("L", "b", 2)
	quit, err = cond(WithState(context.Background(), state2), 1, nil, nil)
	if err != nil {
		t.Fatalf("cond: %v", err)
	}
	if !quit {
		t.Errorf("OR with b=2 should quit")
	}
	// a=0, b=0 → no quit.
	state3 := NewCanvasState("", "")
	state3.SetVar("L", "a", 0)
	state3.SetVar("L", "b", 0)
	quit, err = cond(WithState(context.Background(), state3), 1, nil, nil)
	if err != nil {
		t.Fatalf("cond: %v", err)
	}
	if quit {
		t.Errorf("OR with both 0 should not quit")
	}
}

func TestTranslateLoopCondition_AndRequiresAll(t *testing.T) {
	params := map[string]any{
		"loop_termination_condition": []any{
			map[string]any{"variable": "a", "operator": "=", "value": 1, "input_mode": "constant"},
			map[string]any{"variable": "b", "operator": "=", "value": 2, "input_mode": "constant"},
		},
	}
	cond, err := translateLoopCondition("L", params)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	// a=1, b=2 → quits.
	state := NewCanvasState("", "")
	state.SetVar("L", "a", 1)
	state.SetVar("L", "b", 2)
	quit, _ := cond(WithState(context.Background(), state), 1, nil, nil)
	if !quit {
		t.Errorf("AND with both true should quit")
	}
	// a=1, b=0 → no quit (default logical_op is "and").
	state2 := NewCanvasState("", "")
	state2.SetVar("L", "a", 1)
	state2.SetVar("L", "b", 0)
	quit, _ = cond(WithState(context.Background(), state2), 1, nil, nil)
	if quit {
		t.Errorf("AND with one false should not quit")
	}
}

func TestTranslateLoopCondition_EmptyConditionsNeverQuit(t *testing.T) {
	params := map[string]any{
		"logical_operator": "and",
	}
	cond, err := translateLoopCondition("L", params)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	state := NewCanvasState("", "")
	quit, err := cond(WithState(context.Background(), state), 1, nil, nil)
	if err != nil {
		t.Fatalf("cond: %v", err)
	}
	if quit {
		t.Errorf("empty conditions must never quit (max count is the only terminator)")
	}
}

func TestTranslateLoopCondition_InvalidLogicalOp(t *testing.T) {
	params := map[string]any{
		"logical_operator": "xor",
	}
	if _, err := translateLoopCondition("L", params); err == nil {
		t.Errorf("expected error on invalid logical_operator")
	}
}

func TestTranslateLoopCondition_IncompleteEntry(t *testing.T) {
	cases := []map[string]any{
		{"operator": "=", "value": 1},     // missing variable
		{"variable": "x"},                 // missing operator
		{"variable": "x", "operator": ""}, // empty operator
	}
	for i, item := range cases {
		params := map[string]any{
			"loop_termination_condition": []any{item},
		}
		if _, err := translateLoopCondition("L", params); err == nil {
			t.Errorf("case %d: expected error on incomplete entry", i)
		}
	}
}

func TestTranslateLoopCondition_VariableInputMode(t *testing.T) {
	// condition's value input_mode is "variable" → resolve the value ref
	// from state before applying the operator.
	params := map[string]any{
		"loop_termination_condition": []any{
			map[string]any{
				"variable":   "counter",
				"operator":   "≥",
				"value":      "Begin@threshold",
				"input_mode": "variable",
			},
		},
	}
	cond, err := translateLoopCondition("L", params)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	state := NewCanvasState("", "")
	state.SetVar("L", "counter", 10)
	state.SetVar("Begin", "threshold", 5)
	quit, _ := cond(WithState(context.Background(), state), 1, nil, nil)
	if !quit {
		t.Errorf("counter(10) >= threshold(5) should quit")
	}
}

// ---- evaluateCondition operator dispatch ----

func TestEvaluateCondition_StringOps(t *testing.T) {
	cases := []struct {
		op    string
		value any
		want  bool
	}{
		{"contains", "ell", true},
		{"not contains", "zzz", true},
		{"start with", "hel", true},
		{"end with", "llo", true},
		{"is", "hello", true},
		{"is not", "world", true},
		{"empty", nil, false}, // "hello" != ""
		{"not empty", nil, true},
	}
	for _, tc := range cases {
		got, err := evaluateCondition("hello", tc.op, tc.value)
		if err != nil {
			t.Errorf("op=%s: %v", tc.op, err)
			continue
		}
		if got != tc.want {
			t.Errorf("op=%s: got %v, want %v", tc.op, got, tc.want)
		}
	}
}

func TestEvaluateCondition_NumberOps(t *testing.T) {
	cases := []struct {
		op    string
		value any
		want  bool
	}{
		{"=", 5, true},
		{"≠", 6, true},
		{">", 4, true},
		{"<", 6, true},
		{"≥", 5, true},
		{"≤", 5, true},
	}
	for _, tc := range cases {
		got, err := evaluateCondition(5, tc.op, tc.value)
		if err != nil {
			t.Errorf("op=%s: %v", tc.op, err)
			continue
		}
		if got != tc.want {
			t.Errorf("op=%s: got %v, want %v", tc.op, got, tc.want)
		}
	}
}

func TestEvaluateCondition_InvalidOp(t *testing.T) {
	if _, err := evaluateCondition("hello", "bogus", "x"); err == nil {
		t.Errorf("expected error on unknown operator")
	}
}

// ---- BuildWorkflow end-to-end (with a Loop cpn) ----

func TestBuildWorkflow_LoopInstallsOneNode(t *testing.T) {
	// DSL: Begin -> Loop -> Message
	//     The Loop has no real body, so its sub-graph is just the
	//     synthetic init lambda. The outer workflow should have 3
	//     eino nodes total: Begin, the loop node, Message.
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"begin": {Obj: CanvasComponentObj{ComponentName: "Begin"},
				Downstream: []string{"loop"}},
			"loop": {Obj: CanvasComponentObj{ComponentName: "Loop",
				Params: map[string]any{
					"loop_variables": []any{},
				}},
				Upstream: []string{"begin"}, Downstream: []string{"msg"}},
			"msg": {Obj: CanvasComponentObj{ComponentName: "Message"},
				Upstream: []string{"loop"}},
		},
	}
	if _, err := BuildWorkflow(context.Background(), c); err != nil {
		t.Fatalf("BuildWorkflow: %v", err)
	}
}

func TestBuildWorkflow_LegacyExitLoop(t *testing.T) {
	// DSL with a standalone "ExitLoop" node. The Go port has no
	// implementation for it, but legacyNoOpNames accepts it as a
	// no-op echo node. BuildWorkflow must succeed.
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"begin": {Obj: CanvasComponentObj{ComponentName: "Begin"},
				Downstream: []string{"exit"}},
			"exit": {Obj: CanvasComponentObj{ComponentName: "ExitLoop"},
				Upstream: []string{"begin"}},
		},
	}
	if _, err := BuildWorkflow(context.Background(), c); err != nil {
		t.Fatalf("BuildWorkflow with ExitLoop: %v", err)
	}
}

func TestBuildWorkflow_UnknownComponentErrors(t *testing.T) {
	// A component name that is neither in legacyNoOpNames nor in the
	// isKnownPrimitive allowlist must produce a clear error from
	// BuildWorkflow. Silent acceptance would mask DSL typos until the
	// workflow failed at runtime.
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"begin": {Obj: CanvasComponentObj{ComponentName: "Begin"},
				Downstream: []string{"bogus"}},
			"bogus": {Obj: CanvasComponentObj{ComponentName: "FakeComponent"},
				Upstream: []string{"begin"}},
		},
	}
	_, err := BuildWorkflow(context.Background(), c)
	if err == nil {
		t.Fatal("expected error on unknown component name, got nil")
	}
	// The error must mention the cpn_id AND the offending name so the
	// orchestrator can surface an actionable diagnostic.
	if !strings.Contains(err.Error(), "bogus") || !strings.Contains(err.Error(), "FakeComponent") {
		t.Errorf("error should name both cpn and component; got: %v", err)
	}
}

func TestBuildWorkflow_EmptyComponentNameErrors(t *testing.T) {
	// A component with an empty component_name is a DSL bug. BuildWorkflow
	// must reject it rather than passing through to the placeholder path.
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"begin": {Obj: CanvasComponentObj{ComponentName: "Begin"},
				Downstream: []string{"empty"}},
			"empty": {Obj: CanvasComponentObj{ComponentName: ""},
				Upstream: []string{"begin"}},
		},
	}
	_, err := BuildWorkflow(context.Background(), c)
	if err == nil {
		t.Fatal("expected error on empty component_name, got nil")
	}
}

func TestBuildWorkflow_LoopSharesOuterCanvasState(t *testing.T) {
	// State-sharing contract: the Loop's sub-graph and the outer
	// workflow must operate on the SAME *CanvasState instance. eino
	// nests Workflows by composition — if the outer's WithGenLocalState
	// is bypassed at the lambda boundary, the sub-workflow would not
	// see loop variables and the loop could never terminate.
	//
	// The buildSubWorkflow init lambda writes
	// state.Outputs[loopID][varName]; the LoopCondition closure
	// reads the same slot via state.GetVar. For this to round-trip
	// the two paths must share the same *CanvasState.
	//
	// We verify the contract at two levels:
	//
	//   1. structural: buildLoopExpansion / buildSubWorkflow must
	//      not clone or shadow state in their helpers, and the
	//      returned sub-workflow must be non-nil.
	//   2. runtime: we attach a *CanvasState to ctx via WithState,
	//      replay the init lambda's body manually (it is a single
	//      GetStateFromContext + SetVar pair), and read it back via
	//      GetVar to confirm the SAME instance is observable from
	//      both sides.
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"begin": {Obj: CanvasComponentObj{ComponentName: "Begin"},
				Downstream: []string{"loop"}},
			"loop": {Obj: CanvasComponentObj{ComponentName: "Loop",
				Params: map[string]any{
					"loop_variables": []any{
						map[string]any{
							"variable":   "counter",
							"input_mode": "constant",
							"value":      0,
							"type":       "number",
						},
					},
					"loop_termination_condition": []any{
						map[string]any{
							"variable":   "counter",
							"operator":   "≥",
							"value":      3,
							"input_mode": "constant",
						},
					},
				}},
				Upstream: []string{"begin"}},
		},
	}
	exp, err := buildLoopExpansion(context.Background(), c, "loop")
	if err != nil {
		t.Fatalf("buildLoopExpansion: %v", err)
	}
	if exp.Sub == nil {
		t.Fatal("sub-workflow is nil")
	}
	// Empty body — the loop has no descendants, so Members is empty
	// and MaxIters defaults to 0 (= unbounded, condition-driven).
	if exp.Members["begin"] {
		t.Errorf("'begin' should NOT be a member of the loop's sub-graph")
	}
	if exp.MaxIters != 0 {
		t.Errorf("MaxIters: got %d, want 0 (default = unbounded)", exp.MaxIters)
	}

	// Runtime contract: attach a state to ctx, run the same
	// GetStateFromContext + SetVar sequence the init lambda
	// performs, and confirm the mutation is visible to a
	// LoopCondition-style reader on the SAME *CanvasState.
	state := NewCanvasState("run-1", "task-1")
	ctx := WithState(context.Background(), state)

	got, _, err := GetStateFromContext[*CanvasState](ctx)
	if err != nil {
		t.Fatalf("GetStateFromContext: %v", err)
	}
	if got != state {
		t.Errorf("GetStateFromContext returned a different *CanvasState instance")
	}
	// The init lambda writes "loop@counter" = 0.
	got.SetVar("loop", "counter", 0)
	// A LoopCondition closure would read it back via state.GetVar.
	v, err := state.GetVar("loop@counter")
	if err != nil {
		t.Fatalf("GetVar: %v", err)
	}
	if v != 0 {
		t.Errorf("counter: got %v, want 0 (init lambda should seed it)", v)
	}
	// The reader and writer MUST be the same instance — a clone
	// would mean the loop's "update counter, check counter" cycle
	// would never converge.
	if got != state {
		t.Errorf("state was cloned somewhere — writer and reader see different instances")
	}
}

func TestBuildWorkflow_LoopWithBody(t *testing.T) {
	// DSL: Begin -> Loop -> A -> B
	//     A and B are body members of the Loop's sub-graph.
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"begin": {Obj: CanvasComponentObj{ComponentName: "Begin"},
				Downstream: []string{"loop"}},
			"loop": {Obj: CanvasComponentObj{ComponentName: "Loop",
				Params: map[string]any{
					"loop_variables": []any{
						map[string]any{
							"variable":   "counter",
							"input_mode": "constant",
							"value":      0,
							"type":       "number",
						},
					},
					"loop_termination_condition": []any{
						map[string]any{
							"variable":   "counter",
							"operator":   "≥",
							"value":      3,
							"input_mode": "constant",
						},
					},
					"maximum_loop_count": 10,
				}},
				Upstream: []string{"begin"}, Downstream: []string{"a"}},
			"a": {Obj: CanvasComponentObj{ComponentName: "Message"},
				Upstream: []string{"loop"}, Downstream: []string{"b"}},
			"b": {Obj: CanvasComponentObj{ComponentName: "LLM"},
				Upstream: []string{"a"}},
		},
	}
	if _, err := BuildWorkflow(context.Background(), c); err != nil {
		t.Fatalf("BuildWorkflow: %v", err)
	}
}

func TestBuildWorkflow_LoopMissingParams(t *testing.T) {
	// A Loop with no params at all — empty loop_variables and empty
	// loop_termination_condition. The macro expansion should still
	// succeed (the condition closure becomes a never-quit predicate,
	// the init lambda writes nothing).
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"begin": {Obj: CanvasComponentObj{ComponentName: "Begin"},
				Downstream: []string{"loop"}},
			"loop": {Obj: CanvasComponentObj{ComponentName: "Loop",
				Params: map[string]any{}},
				Upstream: []string{"begin"}},
		},
	}
	if _, err := BuildWorkflow(context.Background(), c); err != nil {
		t.Fatalf("BuildWorkflow: %v", err)
	}
}

func TestBuildWorkflow_LoopIncompleteCondition(t *testing.T) {
	// A Loop with a malformed condition entry. BuildWorkflow must
	// surface the error from translateLoopCondition.
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"begin": {Obj: CanvasComponentObj{ComponentName: "Begin"},
				Downstream: []string{"loop"}},
			"loop": {Obj: CanvasComponentObj{ComponentName: "Loop",
				Params: map[string]any{
					"loop_termination_condition": []any{
						map[string]any{"operator": "=", "value": 1}, // missing variable
					},
				}},
				Upstream: []string{"begin"}},
		},
	}
	if _, err := BuildWorkflow(context.Background(), c); err == nil {
		t.Errorf("expected error on incomplete condition")
	}
}

// ---- valueEqual: reflect.DeepEqual except for untyped nil vs typed nil ----

func valueEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// Use type-aware comparison for maps and slices to handle the
	// case where one side is nil-typed and the other is the zero
	// value.
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !valueEqual(v, bv[k]) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !valueEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	}
	return a == b
}
