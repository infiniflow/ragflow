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
	"errors"
	"reflect"
	"testing"

	"ragflow/internal/agent/canvas"
)

// TestStringTransform_SplitBasic: "a,b;c" with delimiters=[",", ";"] → ["a", "b", "c"].
func TestStringTransform_SplitBasic(t *testing.T) {
	c, err := NewStringTransformComponent(map[string]any{
		"method":     "split",
		"delimiters": []string{",", ";"},
	})
	if err != nil {
		t.Fatalf("NewStringTransformComponent: %v", err)
	}
	state := canvas.NewCanvasState("run-1", "task-1")
	ctx := canvas.WithState(t.Context(), state)

	out, err := c.Invoke(ctx, nil, map[string]any{"line": "a,b;c"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	want := []any{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("split: got %v, want %v", got, want)
	}
}

// TestStringTransform_SplitNoDelim: "abc" with delimiters=[","] → ["abc"].
func TestStringTransform_SplitNoDelim(t *testing.T) {
	c, _ := NewStringTransformComponent(map[string]any{
		"method":     "split",
		"delimiters": []string{","},
	})
	state := canvas.NewCanvasState("run-2", "task-2")
	ctx := canvas.WithState(t.Context(), state)

	out, err := c.Invoke(ctx, nil, map[string]any{"line": "abc"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	want := []any{"abc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("split: got %v, want %v", got, want)
	}
}

// TestStringTransform_Merge: script="{{x}} and {{y}}", inputs={x: "foo", y: "bar"} → "foo and bar".
func TestStringTransform_Merge(t *testing.T) {
	c, _ := NewStringTransformComponent(map[string]any{
		"method":     "merge",
		"delimiters": []string{","},
		"script":     "{{x}} and {{y}}",
	})
	state := canvas.NewCanvasState("run-3", "task-3")
	ctx := canvas.WithState(t.Context(), state)

	out, err := c.Invoke(ctx, nil, map[string]any{"x": "foo", "y": "bar"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["result"], "foo and bar"; got != want {
		t.Errorf("merge: got %v, want %v", got, want)
	}
}

func TestStringTransform_MergeIterationAliases(t *testing.T) {
	c, _ := NewStringTransformComponent(map[string]any{
		"method":     "merge",
		"delimiters": []string{","},
		"script":     "{index}: {item}",
	})
	state := canvas.NewCanvasState("run-iter", "task-iter")
	state.Globals["__item__"] = "beta"
	state.Globals["__index__"] = 1
	ctx := canvas.WithState(t.Context(), state)

	out, err := c.Invoke(ctx, nil, map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["result"], "1: beta"; got != want {
		t.Errorf("merge iteration aliases: got %v, want %v", got, want)
	}
}

// TestStringTransform_SplitFromStateRef: when "line" is absent, the
// component reads the value from state[split_ref].
func TestStringTransform_SplitFromStateRef(t *testing.T) {
	c, _ := NewStringTransformComponent(map[string]any{
		"method":     "split",
		"delimiters": []string{","},
		"split_ref":  "cpn_0@x",
	})
	state := canvas.NewCanvasState("run-4", "task-4")
	state.Outputs["cpn_0"] = map[string]any{"x": "alpha,beta,gamma"}
	ctx := canvas.WithState(t.Context(), state)

	out, err := c.Invoke(ctx, nil, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	want := []any{"alpha", "beta", "gamma"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("split from state: got %v, want %v", got, want)
	}
}

// TestStringTransform_MergeMissingPlaceholder: a placeholder not in
// inputs or state resolves to "" (matches Python's v is None branch).
func TestStringTransform_MergeMissingPlaceholder(t *testing.T) {
	c, _ := NewStringTransformComponent(map[string]any{
		"method":     "merge",
		"delimiters": []string{","},
		"script":     "hello {{name}}",
	})
	state := canvas.NewCanvasState("run-5", "task-5")
	ctx := canvas.WithState(t.Context(), state)

	out, err := c.Invoke(ctx, nil, map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["result"], "hello "; got != want {
		t.Errorf("merge missing: got %q, want %q", got, want)
	}
}

// TestStringTransform_ParamCheck: bad method rejected.
func TestStringTransform_ParamCheck(t *testing.T) {
	_, err := NewStringTransformComponent(map[string]any{
		"method":     "bogus",
		"delimiters": []string{","},
	})
	if err == nil {
		t.Fatal("expected error for bad method, got nil")
	}
}

// TestStringTransform_MergeStringDelimiter: the agent UI saves merge-mode
// delimiters as a single string (","), which must build and merge.
func TestStringTransform_MergeStringDelimiter(t *testing.T) {
	c, err := NewStringTransformComponent(map[string]any{
		"method":     "merge",
		"delimiters": ",",
		"script":     "{{x}} and {{y}}",
	})
	if err != nil {
		t.Fatalf("NewStringTransformComponent: %v", err)
	}
	state := canvas.NewCanvasState("run-6", "task-6")
	ctx := canvas.WithState(t.Context(), state)

	out, err := c.Invoke(ctx, nil, map[string]any{"x": "foo", "y": "bar"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["result"], "foo and bar"; got != want {
		t.Errorf("merge: got %v, want %v", got, want)
	}
}

// TestStringTransform_WhitespaceStringDelimiter: a whitespace string
// delimiter ("\n") is kept as is, not trimmed away.
func TestStringTransform_WhitespaceStringDelimiter(t *testing.T) {
	c, err := NewStringTransformComponent(map[string]any{
		"method":     "merge",
		"delimiters": "\n",
	})
	if err != nil {
		t.Fatalf("NewStringTransformComponent: %v", err)
	}
	got := c.(*StringTransformComponent).param.Delimiters
	want := []string{"\n"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("delimiters: got %q, want %q", got, want)
	}
}

// TestStringTransform_SplitStringDelimiter: "a;b" with delimiters=";" → ["a", "b"].
func TestStringTransform_SplitStringDelimiter(t *testing.T) {
	c, err := NewStringTransformComponent(map[string]any{
		"method":     "split",
		"delimiters": ";",
	})
	if err != nil {
		t.Fatalf("NewStringTransformComponent: %v", err)
	}
	state := canvas.NewCanvasState("run-7", "task-7")
	ctx := canvas.WithState(t.Context(), state)

	out, err := c.Invoke(ctx, nil, map[string]any{"line": "a;b"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	want := []any{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("split: got %v, want %v", got, want)
	}
}

// splitIntoState runs a split of line on "," and stores the outputs under
// cpnID, the way the canvas records a node's outputs for later nodes.
func splitIntoState(t *testing.T, state *canvas.CanvasState, cpnID, line string) {
	t.Helper()
	c, err := NewStringTransformComponent(map[string]any{
		"method":     "split",
		"delimiters": []string{","},
	})
	if err != nil {
		t.Fatalf("NewStringTransformComponent: %v", err)
	}
	out, err := c.Invoke(canvas.WithState(t.Context(), state), nil, map[string]any{"line": line})
	if err != nil {
		t.Fatalf("split Invoke: %v", err)
	}
	state.Outputs[cpnID] = out
}

// TestStringTransform_SplitFeedsListOperations: a split result is a list
// that ListOperations accepts in the same run.
func TestStringTransform_SplitFeedsListOperations(t *testing.T) {
	state := canvas.NewCanvasState("run-8", "task-8")
	splitIntoState(t, state, "st_0", "a,b,c")

	c, err := NewListOperationsComponent(map[string]any{
		"query":      "st_0@result",
		"operations": "head",
		"n":          2,
	})
	if err != nil {
		t.Fatalf("NewListOperationsComponent: %v", err)
	}
	out, err := c.Invoke(canvas.WithState(t.Context(), state), nil, nil)
	if err != nil {
		t.Fatalf("ListOperations Invoke: %v", err)
	}
	got, _ := out["result"].([]any)
	want := []any{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("head: got %v, want %v", got, want)
	}
}

// TestStringTransform_SplitFeedsVariableAssignerAppend: a split result is a
// list that the VariableAssigner append operator accepts in the same run.
func TestStringTransform_SplitFeedsVariableAssignerAppend(t *testing.T) {
	state := canvas.NewCanvasState("run-9", "task-9")
	splitIntoState(t, state, "st_0", "a,b,c")

	c, err := NewVariableAssignerComponent(map[string]any{"variables": []map[string]any{
		{"variable": "st_0@result", "operator": "append", "parameter": "d"},
	}})
	if err != nil {
		t.Fatalf("NewVariableAssignerComponent: %v", err)
	}
	if _, err := c.Invoke(canvas.WithState(t.Context(), state), nil, nil); err != nil {
		t.Fatalf("VariableAssigner Invoke: %v", err)
	}
	got := state.Outputs["st_0"]["result"]
	want := []any{"a", "b", "c", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("st_0@result: got %v, want %v", got, want)
	}
}

// runMerge runs a merge StringTransform against state and returns its result.
func runMerge(t *testing.T, state *canvas.CanvasState, delimiter, script string, inputs map[string]any) any {
	t.Helper()
	c, err := NewStringTransformComponent(map[string]any{
		"method":     "merge",
		"delimiters": delimiter,
		"script":     script,
	})
	if err != nil {
		t.Fatalf("NewStringTransformComponent: %v", err)
	}
	out, err := c.Invoke(canvas.WithState(t.Context(), state), nil, inputs)
	if err != nil {
		t.Fatalf("merge Invoke: %v", err)
	}
	return out["result"]
}

// TestStringTransform_MergeJoinsListRef: a list reference is joined with the
// first delimiter, as Python's get_kwargs did.
func TestStringTransform_MergeJoinsListRef(t *testing.T) {
	for _, tc := range []struct {
		name      string
		list      any
		delimiter string
		want      string
	}{
		{"comma", []any{"a", "b", "c"}, ",", "a,b,c"},
		{"line break", []any{"a", "b", "c"}, "\n", "a\nb\nc"},
		{"typed slice", []string{"a", "b"}, ",", "a,b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := canvas.NewCanvasState("run-10", "task-10")
			state.Outputs["st_0"] = map[string]any{"result": tc.list}
			if got := runMerge(t, state, tc.delimiter, "{st_0@result}", nil); got != tc.want {
				t.Errorf("merge: got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestStringTransform_MergeJoinsListPlaceholder: a {{name}} placeholder
// whose input is a list is joined the same way.
func TestStringTransform_MergeJoinsListPlaceholder(t *testing.T) {
	state := canvas.NewCanvasState("run-11", "task-11")
	got := runMerge(t, state, ",", "{{xs}}", map[string]any{"xs": []any{"a", "b"}})
	if got != "a,b" {
		t.Errorf("merge: got %q, want %q", got, "a,b")
	}
}

// TestStringTransform_MergeKeepsNonListRefs: references that are not lists
// resolve as before next to a joined list.
func TestStringTransform_MergeKeepsNonListRefs(t *testing.T) {
	state := canvas.NewCanvasState("run-12", "task-12")
	state.Sys["query"] = "q"
	state.Outputs["st_0"] = map[string]any{"result": []any{"a", "b", "c"}}
	got := runMerge(t, state, ",", "{sys.query}: {st_0@result}", nil)
	if got != "q: a,b,c" {
		t.Errorf("merge: got %q, want %q", got, "q: a,b,c")
	}
}

// TestStringTransform_MergeListItemsAreNotResolved: list items are inserted
// as text; ref-like text inside an item is neither resolved nor allowed to
// break the other refs.
func TestStringTransform_MergeListItemsAreNotResolved(t *testing.T) {
	state := canvas.NewCanvasState("run-14", "task-14")
	state.Sys["query"] = "q"
	state.Outputs["st_0"] = map[string]any{"result": []any{"a", "{foo@bar}"}}
	got := runMerge(t, state, ",", "{sys.query}: {st_0@result}", nil)
	if got != "q: a,{foo@bar}" {
		t.Errorf("merge: got %q, want %q", got, "q: a,{foo@bar}")
	}
}

// TestStringTransform_MergeListPlaceholderItemsAreNotResolved: a joined
// {{name}} list is inserted after the placeholder pass, so its items are not
// replaced by later placeholders.
func TestStringTransform_MergeListPlaceholderItemsAreNotResolved(t *testing.T) {
	state := canvas.NewCanvasState("run-15", "task-15")
	got := runMerge(t, state, ",", "{{a}}/{{b}}", map[string]any{"a": []any{"{{b}}"}, "b": "X"})
	if got != "{{b}}/X" {
		t.Errorf("merge: got %q, want %q", got, "{{b}}/X")
	}
}

// TestStringTransform_MergeInputsSurviveJinjaPath: a "|" sends the first pass
// through gonja, which only sees canvas state; {{name}} placeholders supplied
// in inputs must still be substituted.
func TestStringTransform_MergeInputsSurviveJinjaPath(t *testing.T) {
	for _, tc := range []struct{ name, script, want string }{
		{"double-brace list ref", "{{st_0@result}} | {{suffix}}", "a,b | X"},
		{"single-brace list ref", "{st_0@result} | {{suffix}}", "a,b | X"},
		{"spaces in placeholder", "{{ suffix }}", "X"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := canvas.NewCanvasState("run-16", "task-16")
			state.Outputs["st_0"] = map[string]any{"result": []any{"a", "b"}}
			got := runMerge(t, state, ",", tc.script, map[string]any{"suffix": "X"})
			if got != tc.want {
				t.Errorf("merge: got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestStringTransform_SplitThenMerge: a split result merged with another
// delimiter.
func TestStringTransform_SplitThenMerge(t *testing.T) {
	state := canvas.NewCanvasState("run-13", "task-13")
	splitIntoState(t, state, "st_0", "a,b,c")
	got := runMerge(t, state, ";", "{st_0@result}", nil)
	if got != "a;b;c" {
		t.Errorf("merge: got %q, want %q", got, "a;b;c")
	}
}

// TestStringTransform_EmptyStringDelimiter: delimiters="" is rejected.
func TestStringTransform_EmptyStringDelimiter(t *testing.T) {
	_, err := NewStringTransformComponent(map[string]any{
		"method":     "merge",
		"delimiters": "",
	})
	var pe *ParamError
	if !errors.As(err, &pe) {
		t.Fatalf("err=%v, want *ParamError", err)
	}
	if pe.Field != "delimiters" {
		t.Errorf("ParamError.Field=%q, want delimiters", pe.Field)
	}
}

// TestStringTransform_Registered: factory lookup.
func TestStringTransform_Registered(t *testing.T) {
	c, err := New("StringTransform", map[string]any{
		"method":     "split",
		"delimiters": []string{","},
	})
	if err != nil {
		t.Fatalf("registry lookup: %v", err)
	}
	if c.Name() != "StringTransform" {
		t.Errorf("Name()=%q, want StringTransform", c.Name())
	}
}

func TestStringTransform_GetInputFormSplit(t *testing.T) {
	c, err := NewStringTransformComponent(map[string]any{
		"method":     "split",
		"delimiters": []string{","},
	})
	if err != nil {
		t.Fatalf("NewStringTransformComponent: %v", err)
	}

	got := c.(*StringTransformComponent).GetInputForm()
	want := map[string]any{
		"line": map[string]any{
			"name": "String",
			"type": "line",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetInputForm split = %#v, want %#v", got, want)
	}
}

func TestStringTransform_GetInputFormMerge(t *testing.T) {
	c, err := NewStringTransformComponent(map[string]any{
		"method":     "merge",
		"delimiters": []string{","},
		"script":     "{{sys.query}} - {{CodeExec:Run@result}} - {{sys.query}}",
	})
	if err != nil {
		t.Fatalf("NewStringTransformComponent: %v", err)
	}

	got := c.(*StringTransformComponent).GetInputForm()
	want := map[string]any{
		"sys.query": map[string]any{
			"name": "sys.query",
			"type": "line",
		},
		"CodeExec:Run@result": map[string]any{
			"name": "CodeExec:Run@result",
			"type": "line",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetInputForm merge = %#v, want %#v", got, want)
	}
}

func TestGetInputElementsFromText(t *testing.T) {
	got := getInputElementsFromText("{{ sys.query }} {{env.foo}} {{Node_1@answer.text}} {{sys.query}}")
	want := map[string]map[string]any{
		"sys.query": {
			"name":       "sys.query",
			"value":      nil,
			"_retrieval": nil,
			"_cpn_id":    "",
		},
		"env.foo": {
			"name":       "env.foo",
			"value":      nil,
			"_retrieval": nil,
			"_cpn_id":    "",
		},
		"Node_1@answer.text": {
			"name":       "Node_1@answer.text",
			"value":      nil,
			"_retrieval": nil,
			"_cpn_id":    "Node_1",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("getInputElementsFromText = %#v, want %#v", got, want)
	}
}
