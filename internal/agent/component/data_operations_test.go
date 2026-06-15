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

// TestDataOperations_SelectKeys: keep only specified keys.
func TestDataOperations_SelectKeys(t *testing.T) {
	c, err := NewDataOperationsComponent(map[string]any{
		"query":       []string{"cpn_0@items"},
		"operations":  "select_keys",
		"select_keys": []string{"a", "c"},
	})
	if err != nil {
		t.Fatalf("NewDataOperationsComponent: %v", err)
	}
	state := canvas.NewCanvasState("run-1", "task-1")
	state.Outputs["cpn_0"] = map[string]any{"items": []any{
		map[string]any{"a": 1, "b": 2, "c": 3},
	}}
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	if len(got) != 1 {
		t.Fatalf("expected 1 element, got %d", len(got))
	}
	item, _ := got[0].(map[string]any)
	if _, ok := item["b"]; ok {
		t.Errorf("b should have been removed; got %v", item)
	}
	if got, want := item["a"], 1; got != want {
		t.Errorf("a: got %v, want %v", got, want)
	}
	if got, want := item["c"], 3; got != want {
		t.Errorf("c: got %v, want %v", got, want)
	}
}

// TestDataOperations_Combine: merge 2 dicts; key conflict on "k":
// first=[1], second=[2,3] → result has "k"=[1,2,3].
func TestDataOperations_Combine(t *testing.T) {
	c, _ := NewDataOperationsComponent(map[string]any{
		"query":      []string{"cpn_0@d1", "cpn_1@d2"},
		"operations": "combine",
	})
	state := canvas.NewCanvasState("run-2", "task-2")
	state.Outputs["cpn_0"] = map[string]any{"d1": map[string]any{"k": []any{1}}}
	state.Outputs["cpn_1"] = map[string]any{"d2": map[string]any{"k": []any{2, 3}}}
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	merged, _ := out["result"].(map[string]any)
	if merged == nil {
		t.Fatalf("expected map result, got %T", out["result"])
	}
	if got, want := merged["k"], []any{1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Errorf("k: got %v, want %v", got, want)
	}
}

// TestDataOperations_RemoveKeys: copy and remove specified keys.
func TestDataOperations_RemoveKeys(t *testing.T) {
	c, _ := NewDataOperationsComponent(map[string]any{
		"query":       []string{"cpn_0@items"},
		"operations":  "remove_keys",
		"remove_keys": []string{"secret", "internal"},
	})
	state := canvas.NewCanvasState("run-3", "task-3")
	state.Outputs["cpn_0"] = map[string]any{"items": []any{
		map[string]any{
			"name":   "alpha",
			"secret": "shh",
			"value":  42,
		},
	}}
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	if len(got) != 1 {
		t.Fatalf("expected 1 element, got %d", len(got))
	}
	item, _ := got[0].(map[string]any)
	if _, ok := item["secret"]; ok {
		t.Errorf("secret should have been removed; got %v", item)
	}
	if _, ok := item["internal"]; ok {
		t.Errorf("internal should have been removed; got %v", item)
	}
	if got, want := item["name"], "alpha"; got != want {
		t.Errorf("name: got %v, want %v", got, want)
	}
	if got, want := item["value"], 42; got != want {
		t.Errorf("value: got %v, want %v", got, want)
	}
}

// TestDataOperations_LiteralEval: a string leaf that's a JSON literal
// gets parsed.
func TestDataOperations_LiteralEval(t *testing.T) {
	c, _ := NewDataOperationsComponent(map[string]any{
		"query":      []string{"cpn_0@items"},
		"operations": "literal_eval",
	})
	state := canvas.NewCanvasState("run-4", "task-4")
	state.Outputs["cpn_0"] = map[string]any{"items": []any{
		map[string]any{
			"plain":  "hello",
			"json":   `{"k": 1, "nested": [2, 3]}`,
			"number": "42",
			"bool":   "true",
		},
	}}
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	if len(got) != 1 {
		t.Fatalf("expected 1 element, got %d", len(got))
	}
	item, _ := got[0].(map[string]any)
	if got, want := item["plain"], "hello"; got != want {
		t.Errorf("plain: got %v, want %v", got, want)
	}
	// json should be decoded into a map
	if jm, ok := item["json"].(map[string]any); !ok {
		t.Errorf("json: not a map, got %T (%v)", item["json"], item["json"])
	} else if got, want := jm["k"], 1.0; got != want {
		t.Errorf("json.k: got %v, want %v", got, want)
	}
	if got, want := item["number"], 42.0; got != want {
		t.Errorf("number: got %v, want %v", got, want)
	}
	if got, want := item["bool"], true; got != want {
		t.Errorf("bool: got %v, want %v", got, want)
	}
}

// TestDataOperations_FilterValues: keep dicts that match the rule.
func TestDataOperations_FilterValues(t *testing.T) {
	c, _ := NewDataOperationsComponent(map[string]any{
		"query":         []string{"cpn_0@items"},
		"operations":    "filter_values",
		"filter_values": []map[string]any{{"key": "k", "operator": "contains", "value": "1"}},
	})
	state := canvas.NewCanvasState("run-5", "task-5")
	state.Outputs["cpn_0"] = map[string]any{"items": []any{
		map[string]any{"k": "1-abc"},
		map[string]any{"k": "2-abc"},
		map[string]any{"k": "3-1abc"},
	}}
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	if len(got) != 2 {
		t.Fatalf("expected 2 kept dicts, got %d: %v", len(got), got)
	}
}

// TestDataOperations_AppendOrUpdate: applies updates and resolves
// {{ref}} placeholders against state.
func TestDataOperations_AppendOrUpdate(t *testing.T) {
	c, _ := NewDataOperationsComponent(map[string]any{
		"query":      []string{"cpn_0@items"},
		"operations": "append_or_update",
		"updates":    []map[string]any{{"key": "owner", "value": "sys.user_id"}},
	})
	state := canvas.NewCanvasState("run-6", "task-6")
	state.Sys["user_id"] = "tenant-7"
	state.Outputs["cpn_0"] = map[string]any{"items": []any{
		map[string]any{"name": "x"},
	}}
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	if len(got) != 1 {
		t.Fatalf("expected 1 element, got %d", len(got))
	}
	item, _ := got[0].(map[string]any)
	if got, want := item["owner"], "tenant-7"; got != want {
		t.Errorf("owner: got %v, want %v", got, want)
	}
}

// TestDataOperations_RenameKeys: rename per the configured pairs.
func TestDataOperations_RenameKeys(t *testing.T) {
	c, _ := NewDataOperationsComponent(map[string]any{
		"query":       []string{"cpn_0@items"},
		"operations":  "rename_keys",
		"rename_keys": []map[string]any{{"old_key": "k", "new_key": "key"}},
	})
	state := canvas.NewCanvasState("run-7", "task-7")
	state.Outputs["cpn_0"] = map[string]any{"items": []any{
		map[string]any{"k": 1, "other": "x"},
	}}
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	if len(got) != 1 {
		t.Fatalf("expected 1 element, got %d", len(got))
	}
	item, _ := got[0].(map[string]any)
	if _, ok := item["k"]; ok {
		t.Errorf("k should have been renamed away; got %v", item)
	}
	if got, want := item["key"], 1; got != want {
		t.Errorf("key: got %v, want %v", got, want)
	}
	if got, want := item["other"], "x"; got != want {
		t.Errorf("other: got %v, want %v", got, want)
	}
}

// TestDataOperations_ParamCheck: bad operation rejected.
func TestDataOperations_ParamCheck(t *testing.T) {
	_, err := NewDataOperationsComponent(map[string]any{
		"query":      []string{"cpn_0@x"},
		"operations": "bogus",
	})
	if err == nil {
		t.Fatal("expected error for bad operations, got nil")
	}
}

// TestDataOperations_Registered: factory lookup.
func TestDataOperations_Registered(t *testing.T) {
	c, err := New("DataOperations", map[string]any{
		"query":      []string{"sys.x"},
		"operations": "select_keys",
	})
	if err != nil {
		t.Fatalf("registry lookup: %v", err)
	}
	if c.Name() != "DataOperations" {
		t.Errorf("Name()=%q, want DataOperations", c.Name())
	}
}
