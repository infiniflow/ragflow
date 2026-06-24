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

// loop_semantics_test.go — end-to-end Loop semantics tests.
//
// Unlike loop_subgraph_test.go (which unit-tests helpers in isolation
// with no factory registered), this file imports
// internal/agent/component as a side-effect to install the real
// component factory via runtime.SetDefaultFactory. The tests then
// compile and run a full Begin → Loop → ... DSL and assert that the
// loop body actually mutates CanvasState across iterations, that
// termination conditions fire on the real state values, and that
// factory errors surface with cpn-scoped diagnostics.
//
// The blank import below is what wires component.New into the
// canvas builder's runtime.DefaultFactory() lookup; without it,
// BuildWorkflow would fall back to its placeholder echo body and the
// loop would never observe the counter increment.
package canvas

import (
	"context"
	"errors"
	"strings"
	"testing"

	// Blank-import to trigger component package init(), which calls
	// runtime.SetDefaultFactory(component.New). Without this, the
	// canvas builder uses its placeholder body and these tests cannot
	// exercise real component invocation.
	_ "ragflow/internal/agent/component"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/agent/workflowx"
)

// runLoopCanvas is the common harness for the e2e loop tests. It
// compiles dsl, attaches state to a fresh ctx, invokes the workflow,
// and returns the run error. Callers inspect state after the run to
// assert per-iteration writes landed.
func runLoopCanvas(t *testing.T, dsl *Canvas) (*CanvasState, error) {
	t.Helper()
	cc, err := Compile(context.Background(), dsl)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	state := NewCanvasState("run-loop", "task-loop")
	ctx := withState(context.Background(), state)
	_, runErr := cc.Workflow.Invoke(ctx, map[string]any{"query": "go"})
	return state, runErr
}

// counterLoopDSL builds a Begin → Loop DSL with one VariableAssigner
// body node that adds the supplied step to a counter loop variable
// each iteration. The loop terminates when counter >= threshold.
func counterLoopDSL(step int, threshold int, maxCount int) *Canvas {
	return &Canvas{
		Components: map[string]CanvasComponent{
			"begin": {
				Obj:        CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
				Downstream: []string{"loop"},
			},
			"loop": {
				Obj: CanvasComponentObj{
					ComponentName: "Loop",
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
								"value":      threshold,
								"input_mode": "constant",
							},
						},
						"logical_operator":   "and",
						"maximum_loop_count": maxCount,
					},
				},
				Upstream:   []string{"begin"},
				Downstream: []string{"bump"},
			},
			"bump": {
				Obj: CanvasComponentObj{
					ComponentName: "VariableAssigner",
					Params: map[string]any{
						"variables": []any{
							map[string]any{
								"variable":  "loop@counter",
								"operator":  "+=",
								"parameter": step,
							},
						},
					},
				},
				Upstream: []string{"loop"},
			},
		},
		Path: []string{"begin", "loop"},
	}
}

// TestLoop_DoWhileCounter is the keystone test: it proves that the
// real VariableAssigner component runs inside the loop body, mutates
// the shared CanvasState, and that the termination condition fires
// on the mutated value. If the loop body were still a placeholder
// echo lambda the counter would stay at 0 and the loop would run to
// maximum_loop_count or hit defaultMaxIterations.
func TestLoop_DoWhileCounter(t *testing.T) {
	state, err := runLoopCanvas(t, counterLoopDSL(1, 3, 50))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	v, err := state.GetVar("loop@counter")
	if err != nil {
		t.Fatalf("GetVar: %v", err)
	}
	got, ok := v.(float64)
	if !ok {
		t.Fatalf("counter: want float64 (VariableAssigner += produces float64), got %T: %v", v, v)
	}
	// The loop performs do-while semantics: it runs the body, THEN
	// checks the condition. Starting at counter=0, the body
	// increments to 1, 2, 3 — the condition (counter >= 3) becomes
	// true after the third iteration, so the final value is 3.
	if got != 3 {
		t.Errorf("counter: got %v, want 3", got)
	}
}

// TestLoop_MaxCount proves that maximum_loop_count caps iterations
// when the termination condition never fires. The condition asks for
// counter >= 100 but maximum_loop_count is 5; the loop must stop at
// counter=5 (5 successful body runs).
func TestLoop_MaxCount(t *testing.T) {
	state, err := runLoopCanvas(t, counterLoopDSL(1, 100, 5))
	// workflowx surfaces a MaxIterationsExceeded error when the cap
	// is hit. Both the error path AND the partial state must be
	// observable to the caller — the state writes that succeeded
	// before the cap should still be present.
	if err == nil {
		t.Fatalf("expected ErrLoopMaxIterationsExceeded, got nil")
	}
	if !errors.Is(err, workflowx.ErrLoopMaxIterationsExceeded) {
		t.Fatalf("want ErrLoopMaxIterationsExceeded, got: %v", err)
	}
	v, err := state.GetVar("loop@counter")
	if err != nil {
		t.Fatalf("GetVar: %v", err)
	}
	got, ok := v.(float64)
	if !ok {
		t.Fatalf("counter: want float64, got %T: %v", v, v)
	}
	if got != 5 {
		t.Errorf("counter at cap: got %v, want 5 (maximum_loop_count)", got)
	}
}

// TestLoop_FactoryErrorSurfaces proves that a factory rejection of a
// loop body member produces a cpn-scoped error from BuildWorkflow
// (not a silent placeholder fallback or an opaque error from the
// workflowx layer).
//
// VariableAssigner's factory rejects a non-list `variables` param
// (see variable_assigner.go's Update). We trigger that by supplying
// a string instead of a list.
func TestLoop_FactoryErrorSurfaces(t *testing.T) {
	dsl := &Canvas{
		Components: map[string]CanvasComponent{
			"begin": {
				Obj:        CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
				Downstream: []string{"loop"},
			},
			"loop": {
				Obj: CanvasComponentObj{
					ComponentName: "Loop",
					Params: map[string]any{
						"loop_variables":             []any{},
						"loop_termination_condition": []any{},
					},
				},
				Upstream:   []string{"begin"},
				Downstream: []string{"bad"},
			},
			"bad": {
				Obj: CanvasComponentObj{
					ComponentName: "VariableAssigner",
					Params: map[string]any{
						"variables": "not-a-list", // factory rejects this
					},
				},
				Upstream: []string{"loop"},
			},
		},
	}
	_, err := Compile(context.Background(), dsl)
	if err == nil {
		t.Fatal("expected factory error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "bad") {
		t.Errorf("error should name the cpn_id 'bad'; got: %v", err)
	}
	if !strings.Contains(msg, "VariableAssigner") {
		t.Errorf("error should name the component type 'VariableAssigner'; got: %v", err)
	}
}

// TestLoop_LegacyExitLoopStaysNoOp confirms that the DSL v1 sentinel
// "ExitLoop" continues to compile as a no-op even when a factory is
// registered (the legacy-no-op path takes precedence over factory
// lookup). This is the protection against a future "ExitLoop" being
// accidentally registered as a real component and changing behaviour
// for v1 DSLs.
func TestLoop_LegacyExitLoopStaysNoOp(t *testing.T) {
	dsl := &Canvas{
		Components: map[string]CanvasComponent{
			"begin": {
				Obj:        CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
				Downstream: []string{"exit"},
			},
			"exit": {
				Obj:      CanvasComponentObj{ComponentName: "ExitLoop"},
				Upstream: []string{"begin"},
			},
		},
	}
	if _, err := Compile(context.Background(), dsl); err != nil {
		t.Fatalf("Compile with legacy ExitLoop (factory registered): %v", err)
	}
	// Also verify the factory IS registered — otherwise this test
	// would be no different from the canvas-only TestBuildWorkflow_LegacyExitLoop.
	if runtime.DefaultFactory() == nil {
		t.Fatal("factory must be registered for this test to be meaningful")
	}
}

// TestLoop_FactoryRegisteredInThisBinary is a sanity guard: if a
// future refactor breaks the blank import in this file, the other
// e2e tests would silently fall back to placeholder bodies and
// pass for the wrong reason. This test fails loudly if the factory
// is not installed.
func TestLoop_FactoryRegisteredInThisBinary(t *testing.T) {
	if runtime.DefaultFactory() == nil {
		t.Fatal("runtime.DefaultFactory() is nil; the blank import of internal/agent/component is missing or broken")
	}
}

// variableModeLoopDSL builds a Begin → VariableAssigner(seed) → Loop →
// VariableAssigner(bump) DSL where the loop's counter is seeded from
// the seed component's output via input_mode="variable". The loop
// terminates when counter >= threshold; the bump node increments
// counter by step each iteration.
//
// This is the regression test for the "input_mode=variable" loop
// variable init bug: the init lambda must dereference the value
// against the live CanvasState (state.GetVar) at init time, not
// store the raw ref string. If the dereference is missing, counter
// is seeded with the literal string "seed@initial" and the body's
// `+=` operator fails with PARAMETER_NOT_NUMBER on the first
// iteration — the loop terminates after a single body run with
// counter=0 (or errors out).
//
// The seed uses VariableAssigner's `set` operator with an int
// parameter (not `overwrite` with a {{literal}} — `overwrite` looks
// the parameter up as a state ref, so a bare number would error with
// PARAMETER_UNRESOLVED). `set` falls through to return the raw param
// for non-string types, which is what we want here.
func variableModeLoopDSL(threshold, step int) *Canvas {
	return &Canvas{
		Components: map[string]CanvasComponent{
			"begin": {
				Obj:        CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
				Downstream: []string{"seed"},
			},
			"seed": {
				Obj: CanvasComponentObj{
					ComponentName: "VariableAssigner",
					Params: map[string]any{
						"variables": []any{
							map[string]any{
								"variable":  "seed@initial",
								"operator":  "set",
								"parameter": 5,
							},
						},
					},
				},
				Upstream:   []string{"begin"},
				Downstream: []string{"loop"},
			},
			"loop": {
				Obj: CanvasComponentObj{
					ComponentName: "Loop",
					Params: map[string]any{
						"loop_variables": []any{
							map[string]any{
								"variable":   "counter",
								"input_mode": "variable", // dereference against state
								"value":      "seed@initial",
								"type":       "number",
							},
						},
						"loop_termination_condition": []any{
							map[string]any{
								"variable":   "counter",
								"operator":   "≥",
								"value":      threshold,
								"input_mode": "constant",
							},
						},
						"logical_operator":   "and",
						"maximum_loop_count": 50,
					},
				},
				Upstream:   []string{"seed"},
				Downstream: []string{"bump"},
			},
			"bump": {
				Obj: CanvasComponentObj{
					ComponentName: "VariableAssigner",
					Params: map[string]any{
						"variables": []any{
							map[string]any{
								"variable":  "loop@counter",
								"operator":  "+=",
								"parameter": step,
							},
						},
					},
				},
				Upstream: []string{"loop"},
			},
		},
		Path: []string{"begin", "loop"},
	}
}

// TestLoop_VariableModeInitDereferencesRef proves that the loop init
// lambda actually dereferences input_mode="variable" refs against the
// live CanvasState. Seed writes 5 to Outputs["seed"]["initial"]; the
// loop's counter is initialised from "seed@initial" (a ref), so the
// expected starting counter is 5. The bump node increments by 1 and
// the loop terminates when counter >= 8. With correct resolution,
// counter walks 5 → 6 → 7 → 8 (3 successful body runs) and stops.
//
// If the init lambda fails to dereference, counter is seeded with the
// literal string "seed@initial" and `+= 1` fails on the first
// iteration; the test would observe a counter of 0 (or a
// PARAMETER_NOT_NUMBER error surfacing from bump).
func TestLoop_VariableModeInitDereferencesRef(t *testing.T) {
	state, err := runLoopCanvas(t, variableModeLoopDSL(8, 1))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	v, err := state.GetVar("loop@counter")
	if err != nil {
		t.Fatalf("GetVar: %v", err)
	}
	got, ok := v.(float64)
	if !ok {
		t.Fatalf("counter: want float64 (VariableAssigner += produces float64), got %T: %v — input_mode=variable init did not dereference the ref; the seed was written as the literal string %q instead of the resolved value", v, v, "seed@initial")
	}
	// 5 (resolved from seed@initial) + 1 + 1 + 1 = 8 (do-while: body
	// runs, THEN condition is checked). Threshold is 8, so the
	// condition fires after the 3rd body run, leaving counter=8.
	if got != 8 {
		t.Errorf("counter: got %v, want 8 (input_mode=variable should seed from seed@initial=5, then 3 increments to reach threshold)", got)
	}
}
