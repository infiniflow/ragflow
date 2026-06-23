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
	"strings"
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

// TestListOperations_TopNLegacyAlias pins the legacy DSL alias used by
// all.json: operations=topN should behave like head with n items so old
// imported workflows keep compiling and running unchanged.
func TestListOperations_TopNLegacyAlias(t *testing.T) {
	c, err := NewListOperationsComponent(map[string]any{
		"query":      "cpn_0@xs",
		"operations": "topN",
		"n":          2,
	})
	if err != nil {
		t.Fatalf("NewListOperationsComponent: %v", err)
	}
	state := canvas.NewCanvasState("run-topn", "task-topn")
	state.Outputs["cpn_0"] = map[string]any{"xs": []any{1, 2, 3, 4}}
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	want := []any{1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("result: got %v, want %v", got, want)
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

// TestListOperations_SortByFieldList: with sort_by="score" the sort
// key is the value of the "score" map key, not the full hashable
// tuple. desc + score picks the row with the highest score first
// regardless of id ordering. Empty sort_by falls back to the legacy
// hashableKey (alphabetically first key) so existing DSLs keep
// working.
func TestListOperations_SortByFieldList(t *testing.T) {
	state := canvas.NewCanvasState("run-sort-by", "task-sort-by")
	state.Outputs["cpn_0"] = map[string]any{
		"rows": []any{
			map[string]any{"id": 1, "score": 0.91, "title": "Alpha"},
			map[string]any{"id": 2, "score": 0.88, "title": "Beta"},
			map[string]any{"id": 3, "score": 0.76, "title": "Gamma"},
		},
	}
	ctx := canvas.WithState(context.Background(), state)

	// sort_by="score", desc → Alpha(0.91), Beta(0.88), Gamma(0.76)
	cSortDesc, err := NewListOperationsComponent(map[string]any{
		"query":       "cpn_0@rows",
		"operations":  "sort",
		"sort_method": "desc",
		"sort_by":     "score",
	})
	if err != nil {
		t.Fatalf("NewListOperationsComponent: %v", err)
	}
	out, err := cSortDesc.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke sort_by=score desc: %v", err)
	}
	got, _ := out["result"].([]any)
	wantOrder := []any{
		map[string]any{"id": 1, "score": 0.91, "title": "Alpha"},
		map[string]any{"id": 2, "score": 0.88, "title": "Beta"},
		map[string]any{"id": 3, "score": 0.76, "title": "Gamma"},
	}
	if !reflect.DeepEqual(got, wantOrder) {
		t.Errorf("sort_by=score desc: got %v, want %v", got, wantOrder)
	}

	// sort_by="" — falls back to hashableKey (alphabetically first
	// field = id). desc → Gamma(id:3), Beta(id:2), Alpha(id:1).
	cLegacy, err := NewListOperationsComponent(map[string]any{
		"query":       "cpn_0@rows",
		"operations":  "sort",
		"sort_method": "desc",
		"sort_by":     "",
	})
	if err != nil {
		t.Fatalf("NewListOperationsComponent (legacy): %v", err)
	}
	out, err = cLegacy.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke sort_by=\"\" desc: %v", err)
	}
	got, _ = out["result"].([]any)
	wantLegacy := []any{
		map[string]any{"id": 3, "score": 0.76, "title": "Gamma"},
		map[string]any{"id": 2, "score": 0.88, "title": "Beta"},
		map[string]any{"id": 1, "score": 0.91, "title": "Alpha"},
	}
	if !reflect.DeepEqual(got, wantLegacy) {
		t.Errorf("sort_by=\"\" desc: got %v, want %v", got, wantLegacy)
	}

	// sort_by="score,title" — primary score, tiebreak title.
	cTiebreak, err := NewListOperationsComponent(map[string]any{
		"query":       "cpn_0@rows",
		"operations":  "sort",
		"sort_method": "asc",
		"sort_by":     "score,title",
	})
	if err != nil {
		t.Fatalf("NewListOperationsComponent (tiebreak): %v", err)
	}
	out, err = cTiebreak.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke sort_by=score,title asc: %v", err)
	}
	got, _ = out["result"].([]any)
	wantTiebreak := []any{
		map[string]any{"id": 3, "score": 0.76, "title": "Gamma"},
		map[string]any{"id": 2, "score": 0.88, "title": "Beta"},
		map[string]any{"id": 1, "score": 0.91, "title": "Alpha"},
	}
	if !reflect.DeepEqual(got, wantTiebreak) {
		t.Errorf("sort_by=score,title asc: got %v, want %v", got, wantTiebreak)
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

// TestListOperations_StrictMode_ReturnsError pins Change #2 (panic →
// recoverable error). Strict mode with n=0 must surface a *listOpPanic
// error from Invoke() (not a goroutine-level panic). The Python
// reference raises ValueError, which the canvas framework catches.
func TestListOperations_StrictMode_ReturnsError(t *testing.T) {
	c, err := NewListOperationsComponent(map[string]any{
		"query":      "cpn_0@xs",
		"operations": "nth",
		"n":          0,
		"strict":     true,
	})
	if err != nil {
		t.Fatalf("NewListOperationsComponent: %v", err)
	}
	state := canvas.NewCanvasState("run-strict", "task-strict")
	state.Outputs["cpn_0"] = map[string]any{"xs": []any{1, 2, 3}}
	ctx := canvas.WithState(context.Background(), state)

	_, err = c.Invoke(ctx, nil)
	if err == nil {
		t.Fatal("expected error from strict-mode nth with n=0, got nil")
	}
	if !strings.Contains(err.Error(), "strict mode") {
		t.Errorf("error %q should mention 'strict mode'", err)
	}
}

// TestListOperations_StrictStringCoercion pins Change #3: passing
// "strict" as a string ("true"/"1"/"yes"/"on") must be coerced to a
// true bool, matching Python's _is_strict accept-list.
func TestListOperations_StrictStringCoercion(t *testing.T) {
	for _, v := range []string{"true", "TRUE", "True", "1", "yes", "on"} {
		c, err := NewListOperationsComponent(map[string]any{
			"query":      "cpn_0@xs",
			"operations": "nth",
			"n":          0,
			"strict":     v,
		})
		if err != nil {
			t.Fatalf("[strict=%q] NewListOperationsComponent: %v", v, err)
		}
		state := canvas.NewCanvasState("run-str-"+v, "task-str-"+v)
		state.Outputs["cpn_0"] = map[string]any{"xs": []any{1, 2, 3}}
		ctx := canvas.WithState(context.Background(), state)

		_, err = c.Invoke(ctx, nil)
		if err == nil {
			t.Errorf("[strict=%q] expected error from strict-mode nth with n=0, got nil", v)
		}
	}
}

// TestListOperations_StrictFalseStringsIgnored pins the negative case
// for Change #3: strings other than the accept-list must coerce to
// false (no strict-mode error).
func TestListOperations_StrictFalseStringsIgnored(t *testing.T) {
	for _, v := range []string{"false", "FALSE", "0", "no", "off", "random"} {
		c, err := NewListOperationsComponent(map[string]any{
			"query":      "cpn_0@xs",
			"operations": "nth",
			"n":          99, // out-of-range, but non-strict → empty
			"strict":     v,
		})
		if err != nil {
			t.Fatalf("[strict=%q] NewListOperationsComponent: %v", v, err)
		}
		state := canvas.NewCanvasState("run-strf-"+v, "task-strf-"+v)
		state.Outputs["cpn_0"] = map[string]any{"xs": []any{1, 2, 3}}
		ctx := canvas.WithState(context.Background(), state)

		out, err := c.Invoke(ctx, nil)
		if err != nil {
			t.Errorf("[strict=%q] expected non-error (coerced to false), got %v", v, err)
		}
		got, _ := out["result"].([]any)
		if len(got) != 0 {
			t.Errorf("[strict=%q] expected empty result, got %v", v, got)
		}
	}
}

// TestListOperations_CoerceNBool pins Change #4: toInt must follow
// Python's int() semantics for booleans (true→1, false→0).
func TestListOperations_CoerceNBool(t *testing.T) {
	if got := toInt(true); got != 1 {
		t.Errorf("toInt(true) = %d, want 1", got)
	}
	if got := toInt(false); got != 0 {
		t.Errorf("toInt(false) = %d, want 0", got)
	}
}

// TestListOperations_FilterEqBool pins Change #5: normValue must render
// Go's bool as Python's str(bool) ("True"/"False") so filter `=` matches
// the Python DSL contract.
func TestListOperations_FilterEqBool(t *testing.T) {
	c, err := NewListOperationsComponent(map[string]any{
		"query":      "cpn_0@xs",
		"operations": "filter",
		"filter":     map[string]any{"operator": "=", "value": "True"},
	})
	if err != nil {
		t.Fatalf("NewListOperationsComponent: %v", err)
	}
	state := canvas.NewCanvasState("run-feq", "task-feq")
	state.Outputs["cpn_0"] = map[string]any{"xs": []any{true, false, true, "True"}}
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	// Both true values plus the "True" string should match.
	if len(got) != 3 {
		t.Errorf("expected 3 matches (true, true, 'True'), got %d: %v", len(got), got)
	}
}

// TestListOperations_UnknownOp_ReturnsError pins Change #1: the
// defensive default: branch in Invoke() must surface an explicit
// error for any operation name that bypasses the allowlist. We
// construct the struct directly (same package) to skip Check().
func TestListOperations_UnknownOp_ReturnsError(t *testing.T) {
	c := &ListOperationsComponent{
		name: "ListOperations",
		param: listOperationsParam{
			Query:      "cpn_0@xs",
			Operations: "bogus",
			N:          1,
		},
	}
	state := canvas.NewCanvasState("run-bogus", "task-bogus")
	state.Outputs["cpn_0"] = map[string]any{"xs": []any{1, 2, 3}}
	ctx := canvas.WithState(context.Background(), state)

	_, err := c.Invoke(ctx, nil)
	if err == nil {
		t.Fatal("expected error for unknown operation, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported operation") {
		t.Errorf("error %q should mention 'unsupported operation'", err)
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error %q should mention the bad op name 'bogus'", err)
	}
}

// TestListOperations_InputsDocMatchesAllowlist pins Change #6: the
// Inputs() docstring must not claim support for operations that are
// not in the Check() allowlist (slice/shuffle/take/reverse/deduplicate).
// A bug here misleads the editor dropdown and the agent developer.
func TestListOperations_InputsDocMatchesAllowlist(t *testing.T) {
	c, err := NewListOperationsComponent(map[string]any{
		"query": "cpn_0@xs",
	})
	if err != nil {
		t.Fatalf("NewListOperationsComponent: %v", err)
	}
	doc := c.Inputs()
	for _, banned := range []string{"slice", "shuffle", "reverse"} {
		if strings.Contains(doc["operations"], banned) {
			t.Errorf("Inputs()[operations] doc must not mention %q (not in allowlist): %q", banned, doc["operations"])
		}
	}
	// The doc must mention the six operations that are actually supported.
	for _, op := range []string{"nth", "head", "tail", "filter", "sort", "drop_duplicates"} {
		if !strings.Contains(doc["operations"], op) {
			t.Errorf("Inputs()[operations] doc must mention %q: %q", op, doc["operations"])
		}
	}
}
