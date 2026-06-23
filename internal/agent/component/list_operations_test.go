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
	"sort"
	"testing"

	"ragflow/internal/agent/canvas"
)

// TestListOperations_Head: [1,2,3,4,5] op=head n=3 → [1,2,3], first=1, last=3.
func TestListOperations_Head(t *testing.T) {
	c, err := NewListOperationsComponent(map[string]any{
		"query":      "cpn_0@xs",
		"operations": "head",
		"n":          3,
	})
	if err != nil {
		t.Fatalf("NewListOperationsComponent: %v", err)
	}
	state := canvas.NewCanvasState("run-1", "task-1")
	state.Outputs["cpn_0"] = map[string]any{"xs": []any{1, 2, 3, 4, 5}}
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	want := []any{1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("result: got %v, want %v", got, want)
	}
	if got, want := out["first"], 1; got != want {
		t.Errorf("first: got %v, want %v", got, want)
	}
	if got, want := out["last"], 3; got != want {
		t.Errorf("last: got %v, want %v", got, want)
	}
}

// TestListOperations_Filter: items ["foo", "bar", "foobar"], op=filter,
// operator="contains", value="bar" → ["bar", "foobar"].
func TestListOperations_Filter(t *testing.T) {
	c, _ := NewListOperationsComponent(map[string]any{
		"query":      "cpn_0@xs",
		"operations": "filter",
		"filter":     map[string]any{"operator": "contains", "value": "bar"},
	})
	state := canvas.NewCanvasState("run-2", "task-2")
	state.Outputs["cpn_0"] = map[string]any{"xs": []any{"foo", "bar", "foobar"}}
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	want := []any{"bar", "foobar"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filter: got %v, want %v", got, want)
	}
}

// TestListOperations_DropDuplicates: [{"k":1},{"k":1},{"k":2}] → [{"k":1},{"k":2}].
func TestListOperations_DropDuplicates(t *testing.T) {
	c, _ := NewListOperationsComponent(map[string]any{
		"query":      "cpn_0@xs",
		"operations": "drop_duplicates",
	})
	state := canvas.NewCanvasState("run-3", "task-3")
	state.Outputs["cpn_0"] = map[string]any{"xs": []any{
		map[string]any{"k": 1},
		map[string]any{"k": 1},
		map[string]any{"k": 2},
	}}
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	if len(got) != 2 {
		t.Fatalf("expected 2 elements, got %d: %v", len(got), got)
	}
	if !reflect.DeepEqual(got[0], map[string]any{"k": 1}) {
		t.Errorf("got[0]: %v, want {k:1}", got[0])
	}
	if !reflect.DeepEqual(got[1], map[string]any{"k": 2}) {
		t.Errorf("got[1]: %v, want {k:2}", got[1])
	}
}

// TestListOperations_Tail: [1,2,3,4,5] op=tail n=2 → [4,5].
func TestListOperations_Tail(t *testing.T) {
	c, _ := NewListOperationsComponent(map[string]any{
		"query":      "cpn_0@xs",
		"operations": "tail",
		"n":          2,
	})
	state := canvas.NewCanvasState("run-4", "task-4")
	state.Outputs["cpn_0"] = map[string]any{"xs": []any{1, 2, 3, 4, 5}}
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	want := []any{4, 5}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tail: got %v, want %v", got, want)
	}
}

// TestListOperations_NthPositive: [a,b,c,d] n=3 → [c].
func TestListOperations_NthPositive(t *testing.T) {
	c, _ := NewListOperationsComponent(map[string]any{
		"query":      "cpn_0@xs",
		"operations": "nth",
		"n":          3,
	})
	state := canvas.NewCanvasState("run-5", "task-5")
	state.Outputs["cpn_0"] = map[string]any{"xs": []any{"a", "b", "c", "d"}}
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	want := []any{"c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nth: got %v, want %v", got, want)
	}
}

// TestListOperations_SortDesc: numeric sort with sort_method=desc.
func TestListOperations_SortDesc(t *testing.T) {
	c, _ := NewListOperationsComponent(map[string]any{
		"query":       "cpn_0@xs",
		"operations":  "sort",
		"sort_method": "desc",
	})
	state := canvas.NewCanvasState("run-6", "task-6")
	state.Outputs["cpn_0"] = map[string]any{"xs": []any{3, 1, 4, 1, 5, 9, 2, 6}}
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	sortedAsc := append([]any{}, got...)
	sort.Slice(sortedAsc, func(i, j int) bool {
		af, _ := sortedAsc[i].(int)
		bf, _ := sortedAsc[j].(int)
		return af < bf
	})
	// Reverse for desc
	for i, j := 0, len(sortedAsc)-1; i < j; i, j = i+1, j-1 {
		sortedAsc[i], sortedAsc[j] = sortedAsc[j], sortedAsc[i]
	}
	if !reflect.DeepEqual(got, sortedAsc) {
		t.Errorf("sort desc: got %v, want %v", got, sortedAsc)
	}
}

// TestListOperations_NotAList: returns a clear error.
func TestListOperations_NotAList(t *testing.T) {
	c, _ := NewListOperationsComponent(map[string]any{
		"query":      "cpn_0@x",
		"operations": "head",
		"n":          1,
	})
	state := canvas.NewCanvasState("run-7", "task-7")
	state.Outputs["cpn_0"] = map[string]any{"x": "not-a-list"}
	ctx := canvas.WithState(context.Background(), state)

	_, err := c.Invoke(ctx, nil)
	if err == nil {
		t.Fatal("expected error for non-list input, got nil")
	}
}

// TestListOperations_ParamCheck: empty query rejected.
func TestListOperations_ParamCheck(t *testing.T) {
	_, err := NewListOperationsComponent(map[string]any{
		"query":      "",
		"operations": "head",
	})
	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}
}

// TestListOperations_Registered: factory lookup.
func TestListOperations_Registered(t *testing.T) {
	c, err := New("ListOperations", map[string]any{
		"query":      "sys.x",
		"operations": "head",
		"n":          1,
	})
	if err != nil {
		t.Fatalf("registry lookup: %v", err)
	}
	if c.Name() != "ListOperations" {
		t.Errorf("Name()=%q, want ListOperations", c.Name())
	}
}
