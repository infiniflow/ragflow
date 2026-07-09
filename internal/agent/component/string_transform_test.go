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
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{"line": "a,b;c"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]string)
	want := []string{"a", "b", "c"}
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
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{"line": "abc"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]string)
	want := []string{"abc"}
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
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{"x": "foo", "y": "bar"})
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
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{})
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
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["result"].([]string)
	want := []string{"alpha", "beta", "gamma"}
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
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{})
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
