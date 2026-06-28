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

package component

import (
	"context"
	"testing"

	"ragflow/internal/agent/canvas"
)

// TestVariableAggregator_FirstNonEmpty: 3 groups, 2 selectors each,
// first selector's ref is unset, second has a value → second wins.
func TestVariableAggregator_FirstNonEmpty(t *testing.T) {
	state := canvas.NewCanvasState("run-1", "task-1")
	state.Outputs["cpn_0"] = map[string]any{}
	state.Outputs["cpn_1"] = map[string]any{"y": "second-a"}
	state.Outputs["cpn_2"] = map[string]any{"y": "second-b"}
	state.Outputs["cpn_3"] = map[string]any{"y": "second-c"}
	ctx := canvas.WithState(context.Background(), state)

	groups := []map[string]any{
		{
			"group_name": "group_a",
			"variables": []any{
				map[string]any{"value": "cpn_0@missing"},
				map[string]any{"value": "cpn_1@y"},
			},
		},
		{
			"group_name": "group_b",
			"variables": []any{
				map[string]any{"value": "cpn_0@missing"},
				map[string]any{"value": "cpn_2@y"},
			},
		},
		{
			"group_name": "group_c",
			"variables": []any{
				map[string]any{"value": "cpn_0@missing"},
				map[string]any{"value": "cpn_3@y"},
			},
		},
	}
	c, err := NewVariableAggregatorComponent(map[string]any{"groups": groups})
	if err != nil {
		t.Fatalf("NewVariableAggregatorComponent: %v", err)
	}
	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["group_a"], "second-a"; got != want {
		t.Errorf("group_a: got %v, want %v", got, want)
	}
	if got, want := out["group_b"], "second-b"; got != want {
		t.Errorf("group_b: got %v, want %v", got, want)
	}
	if got, want := out["group_c"], "second-c"; got != want {
		t.Errorf("group_c: got %v, want %v", got, want)
	}
	if len(out) != 3 {
		t.Errorf("expected 3 output keys, got %d: %v", len(out), out)
	}
}

// TestVariableAggregator_SkipsEmptyString: empty string is falsy in
// Python's bool() — the Go port must treat "" the same way.
func TestVariableAggregator_SkipsEmptyString(t *testing.T) {
	state := canvas.NewCanvasState("run-2", "task-2")
	state.Outputs["cpn_0"] = map[string]any{"x": ""}
	state.Outputs["cpn_1"] = map[string]any{"y": "picked"}
	ctx := canvas.WithState(context.Background(), state)

	groups := []map[string]any{
		{
			"group_name": "out",
			"variables": []any{
				map[string]any{"value": "cpn_0@x"},
				map[string]any{"value": "cpn_1@y"},
			},
		},
	}
	c, err := NewVariableAggregatorComponent(map[string]any{"groups": groups})
	if err != nil {
		t.Fatalf("NewVariableAggregatorComponent: %v", err)
	}
	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["out"], "picked"; got != want {
		t.Errorf("out: got %v, want %v", got, want)
	}
}

// TestVariableAggregator_MultipleGroups: 3 groups, each picks its own
// first-non-empty from independent state namespaces.
func TestVariableAggregator_MultipleGroups(t *testing.T) {
	state := canvas.NewCanvasState("run-3", "task-3")
	state.Sys["a"] = "alpha"
	state.Sys["b"] = ""
	state.Env["c"] = "gamma"
	ctx := canvas.WithState(context.Background(), state)

	groups := []map[string]any{
		{
			"group_name": "g1",
			"variables": []any{
				map[string]any{"value": "sys.a"},
			},
		},
		{
			"group_name": "g2",
			"variables": []any{
				map[string]any{"value": "sys.b"},
				map[string]any{"value": "env.c"},
			},
		},
		{
			"group_name": "g3",
			"variables": []any{
				map[string]any{"value": "sys.d"}, // missing
				map[string]any{"value": "env.c"},
			},
		},
	}
	c, err := NewVariableAggregatorComponent(map[string]any{"groups": groups})
	if err != nil {
		t.Fatalf("NewVariableAggregatorComponent: %v", err)
	}
	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["g1"], "alpha"; got != want {
		t.Errorf("g1: got %v, want %v", got, want)
	}
	if got, want := out["g2"], "gamma"; got != want {
		t.Errorf("g2: got %v, want %v", got, want)
	}
	if got, want := out["g3"], "gamma"; got != want {
		t.Errorf("g3: got %v, want %v", got, want)
	}
}

// TestVariableAggregator_AllEmpty: no group picks a value → no output
// keys. The component must not error or panic.
func TestVariableAggregator_AllEmpty(t *testing.T) {
	state := canvas.NewCanvasState("run-4", "task-4")
	state.Outputs["cpn_0"] = map[string]any{}
	ctx := canvas.WithState(context.Background(), state)

	groups := []map[string]any{
		{
			"group_name": "g1",
			"variables": []any{
				map[string]any{"value": "cpn_0@missing"},
			},
		},
	}
	c, err := NewVariableAggregatorComponent(map[string]any{"groups": groups})
	if err != nil {
		t.Fatalf("NewVariableAggregatorComponent: %v", err)
	}
	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty outputs, got %v", out)
	}
}

// TestVariableAggregator_ParamCheck: empty groups list must be rejected
// at construction time.
func TestVariableAggregator_ParamCheck(t *testing.T) {
	_, err := NewVariableAggregatorComponent(map[string]any{"groups": []any{}})
	if err == nil {
		t.Fatal("expected error for empty groups, got nil")
	}
}

// TestVariableAggregator_Registered: factory lookup via the registry.
func TestVariableAggregator_Registered(t *testing.T) {
	c, err := New("VariableAggregator", map[string]any{
		"groups": []map[string]any{
			{
				"group_name": "g",
				"variables":  []any{map[string]any{"value": "sys.x"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("registry lookup: %v", err)
	}
	if c.Name() != "VariableAggregator" {
		t.Errorf("Name()=%q, want VariableAggregator", c.Name())
	}
}
