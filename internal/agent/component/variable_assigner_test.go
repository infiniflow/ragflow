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
	"reflect"
	"testing"

	"ragflow/internal/agent/canvas"
)

// TestVariableAssigner_Append: list append, verify state updated.
func TestVariableAssigner_Append(t *testing.T) {
	state := canvas.NewCanvasState("run-1", "task-1")
	state.Outputs["cpn_0"] = map[string]any{"xs": []any{1, 2}}
	ctx := canvas.WithState(context.Background(), state)

	vars := []map[string]any{
		{
			"variable":  "cpn_0@xs",
			"operator":  "append",
			"parameter": 3,
		},
	}
	c, err := NewVariableAssignerComponent(map[string]any{"variables": vars})
	if err != nil {
		t.Fatalf("NewVariableAssignerComponent: %v", err)
	}
	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got := state.Outputs["cpn_0"]["xs"]
	want := []any{1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("state.Outputs[cpn_0][xs]: got %v, want %v", got, want)
	}
	assigns, _ := out["assignments"].([]string)
	if len(assigns) != 1 || assigns[0] != "cpn_0@xs" {
		t.Errorf("assignments: got %v, want [cpn_0@xs]", assigns)
	}
}

// TestVariableAssigner_Overwrite: variable="cpn_0@x", operator="overwrite",
// parameter="cpn_1@y" → cpn_0@x = cpn_1@y value.
func TestVariableAssigner_Overwrite(t *testing.T) {
	state := canvas.NewCanvasState("run-2", "task-2")
	state.Outputs["cpn_0"] = map[string]any{"x": "old"}
	state.Outputs["cpn_1"] = map[string]any{"y": "fresh"}
	ctx := canvas.WithState(context.Background(), state)

	vars := []map[string]any{
		{
			"variable":  "cpn_0@x",
			"operator":  "overwrite",
			"parameter": "cpn_1@y",
		},
	}
	c, err := NewVariableAssignerComponent(map[string]any{"variables": vars})
	if err != nil {
		t.Fatalf("NewVariableAssignerComponent: %v", err)
	}
	if _, err := c.Invoke(ctx, nil); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := state.Outputs["cpn_0"]["x"], "fresh"; got != want {
		t.Errorf("state.Outputs[cpn_0][x]: got %v, want %v", got, want)
	}
}

// TestVariableAssigner_DivideByZero: assert "ERROR:DIVIDE_BY_ZERO"
// returned, state unchanged.
func TestVariableAssigner_DivideByZero(t *testing.T) {
	state := canvas.NewCanvasState("run-3", "task-3")
	state.Outputs["cpn_0"] = map[string]any{"n": 6.0}
	ctx := canvas.WithState(context.Background(), state)

	vars := []map[string]any{
		{
			"variable":  "cpn_0@n",
			"operator":  "/=",
			"parameter": 0,
		},
	}
	c, err := NewVariableAssignerComponent(map[string]any{"variables": vars})
	if err != nil {
		t.Fatalf("NewVariableAssignerComponent: %v", err)
	}
	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	errs, ok := out["errors"].([]string)
	if !ok || len(errs) == 0 {
		t.Fatalf("expected errors in outputs, got %v", out)
	}
	found := false
	for _, e := range errs {
		if containsString(e, "DIVIDE_BY_ZERO") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected DIVIDE_BY_ZERO error, got %v", errs)
	}
	// state must be unchanged
	if got, want := state.Outputs["cpn_0"]["n"], 6.0; got != want {
		t.Errorf("state.Outputs[cpn_0][n]: got %v, want %v (unchanged)", got, want)
	}
}

// TestVariableAssigner_Clear: list/str/dict/int → empty values.
func TestVariableAssigner_Clear(t *testing.T) {
	state := canvas.NewCanvasState("run-4", "task-4")
	state.Outputs["cpn_0"] = map[string]any{
		"a": []any{1, 2},
		"b": "hello",
		"c": map[string]any{"k": "v"},
		"d": 42,
	}
	ctx := canvas.WithState(context.Background(), state)

	vars := []map[string]any{
		{"variable": "cpn_0@a", "operator": "clear", "parameter": "x"},
		{"variable": "cpn_0@b", "operator": "clear", "parameter": "x"},
		{"variable": "cpn_0@c", "operator": "clear", "parameter": "x"},
		{"variable": "cpn_0@d", "operator": "clear", "parameter": "x"},
	}
	c, err := NewVariableAssignerComponent(map[string]any{"variables": vars})
	if err != nil {
		t.Fatalf("NewVariableAssignerComponent: %v", err)
	}
	if _, err := c.Invoke(ctx, nil); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := state.Outputs["cpn_0"]["a"]; reflect.DeepEqual(got, []any{1, 2}) {
		t.Errorf("cpn_0@a not cleared: %v", got)
	}
	if got, _ := state.Outputs["cpn_0"]["b"].(string); got != "" {
		t.Errorf("cpn_0@b: got %q, want \"\"", got)
	}
	if got, ok := state.Outputs["cpn_0"]["c"].(map[string]any); !ok || len(got) != 0 {
		t.Errorf("cpn_0@c: got %v, want empty map", state.Outputs["cpn_0"]["c"])
	}
	if got, _ := state.Outputs["cpn_0"]["d"].(int); got != 0 {
		t.Errorf("cpn_0@d: got %v, want 0", got)
	}
}

// TestVariableAssigner_Arithmetic: += -= *= /= on numeric values.
func TestVariableAssigner_Arithmetic(t *testing.T) {
	state := canvas.NewCanvasState("run-5", "task-5")
	state.Outputs["cpn_0"] = map[string]any{"n": 10.0}
	ctx := canvas.WithState(context.Background(), state)

	vars := []map[string]any{
		{"variable": "cpn_0@n", "operator": "+=", "parameter": 5},
		{"variable": "cpn_0@n", "operator": "-=", "parameter": 3},
		{"variable": "cpn_0@n", "operator": "*=", "parameter": 2},
		{"variable": "cpn_0@n", "operator": "/=", "parameter": 4},
	}
	c, err := NewVariableAssignerComponent(map[string]any{"variables": vars})
	if err != nil {
		t.Fatalf("NewVariableAssignerComponent: %v", err)
	}
	if _, err := c.Invoke(ctx, nil); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	// 10 + 5 = 15, 15 - 3 = 12, 12 * 2 = 24, 24 / 4 = 6
	if got, want := state.Outputs["cpn_0"]["n"], 6.0; got != want {
		t.Errorf("after +5 -3 *2 /4: got %v, want %v", got, want)
	}
}

// TestVariableAssigner_RemoveFirstLast: list slicing.
func TestVariableAssigner_RemoveFirstLast(t *testing.T) {
	state := canvas.NewCanvasState("run-6", "task-6")
	state.Outputs["cpn_0"] = map[string]any{"xs": []any{"a", "b", "c", "d"}}
	ctx := canvas.WithState(context.Background(), state)

	vars := []map[string]any{
		{"variable": "cpn_0@xs", "operator": "remove_first", "parameter": "x"},
	}
	c, _ := NewVariableAssignerComponent(map[string]any{"variables": vars})
	if _, err := c.Invoke(ctx, nil); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := state.Outputs["cpn_0"]["xs"], []any{"b", "c", "d"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after remove_first: got %v, want %v", got, want)
	}

	vars = []map[string]any{
		{"variable": "cpn_0@xs", "operator": "remove_last", "parameter": "x"},
	}
	c, _ = NewVariableAssignerComponent(map[string]any{"variables": vars})
	if _, err := c.Invoke(ctx, nil); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := state.Outputs["cpn_0"]["xs"], []any{"b", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after remove_last: got %v, want %v", got, want)
	}
}

// TestVariableAssigner_SysTarget: variable="sys.x" → state.Sys is written.
func TestVariableAssigner_SysTarget(t *testing.T) {
	state := canvas.NewCanvasState("run-7", "task-7")
	ctx := canvas.WithState(context.Background(), state)

	vars := []map[string]any{
		{"variable": "sys.x", "operator": "set", "parameter": "hello"},
	}
	c, _ := NewVariableAssignerComponent(map[string]any{"variables": vars})
	if _, err := c.Invoke(ctx, nil); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := state.Sys["x"], "hello"; got != want {
		t.Errorf("state.Sys[x]: got %v, want %v", got, want)
	}
}

// TestVariableAssigner_Registered: factory lookup.
func TestVariableAssigner_Registered(t *testing.T) {
	c, err := New("VariableAssigner", map[string]any{
		"variables": []map[string]any{},
	})
	if err != nil {
		t.Fatalf("registry lookup: %v", err)
	}
	if c.Name() != "VariableAssigner" {
		t.Errorf("Name()=%q, want VariableAssigner", c.Name())
	}
}
