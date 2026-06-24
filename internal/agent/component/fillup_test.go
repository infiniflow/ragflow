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

// TestFillup_PassesThroughInputs asserts the multi-field passthrough
// contract: every input key (with a non-file {value, type} payload)
// appears in the output with its inner `value` extracted. This is the
// primary contract downstream LLM/Retrieval nodes rely on.
func TestFillup_PassesThroughInputs(t *testing.T) {
	c, _ := New(componentNameFillup, nil)
	state := canvas.NewCanvasState("run-1", "task-1")
	ctx := withStateForTest(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{
		"inputs": map[string]any{
			"a": map[string]any{"value": 1},
			"b": map[string]any{"value": "x"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["a"].(int); got != 1 {
		t.Errorf("a: got %v, want 1", out["a"])
	}
	if got, _ := out["b"].(string); got != "x" {
		t.Errorf("b: got %q, want %q", got, "x")
	}
}

// TestFillup_NoTipsKey locks down the defining difference from
// UserFillUp: Fillup never emits a "tips" key, regardless of the
// params it was constructed with. Even if a misconfigured DSL passed
// `tips` or `enable_tips` (Fillup ignores them), the output stays
// tips-less.
func TestFillup_NoTipsKey(t *testing.T) {
	c, _ := New(componentNameFillup, map[string]any{
		"enable_tips": true, // ignored by Fillup
		"tips":        "should be ignored",
	})
	state := canvas.NewCanvasState("run-2", "task-2")
	ctx := withStateForTest(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{
		"inputs": map[string]any{
			"x": map[string]any{"value": "y"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if _, ok := out["tips"]; ok {
		t.Errorf("Fillup must not emit a tips key; got %v", out["tips"])
	}
	if got, _ := out["x"].(string); got != "y" {
		t.Errorf("x passthrough: got %q, want %q", got, "y")
	}
}

// TestFillup_NonDictInput covers the contract that a plain (non-dict)
// input value is passed through as-is. The {value, type} envelope is
// optional — when it is missing, the raw value lands in the output
// untouched. This mirrors fillup.py:78 (`v = v.get("value")` only runs
// when `v` is a dict).
func TestFillup_NonDictInput(t *testing.T) {
	c, _ := New(componentNameFillup, nil)
	state := canvas.NewCanvasState("run-3", "task-3")
	ctx := withStateForTest(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{
		"inputs": map[string]any{
			"plain_str":  "just a string",
			"plain_int":  42,
			"plain_list": []any{"a", "b", "c"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["plain_str"].(string); got != "just a string" {
		t.Errorf("plain_str: got %q, want %q", got, "just a string")
	}
	if got, _ := out["plain_int"].(int); got != 42 {
		t.Errorf("plain_int: got %v, want 42", out["plain_int"])
	}
	list, _ := out["plain_list"].([]any)
	if len(list) != 3 || list[0] != "a" || list[2] != "c" {
		t.Errorf("plain_list: got %v, want [a b c]", out["plain_list"])
	}
}

// TestFillup_FileInputStub pins the file-typed input stub:
// file-typed inputs are stubbed as "<file:key>" — same contract
// as UserFillUp, so downstream components see a consistent payload
// shape across the two form-filling components.
func TestFillup_FileInputStub(t *testing.T) {
	c, _ := New(componentNameFillup, nil)
	state := canvas.NewCanvasState("run-4", "task-4")
	ctx := withStateForTest(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{
		"inputs": map[string]any{
			"cv": map[string]any{
				"value": []any{"file-1"},
				"type":  "file",
			},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["cv"].(string); got != "<file:cv>" {
		t.Errorf("cv stub: got %q, want %q", got, "<file:cv>")
	}
}
