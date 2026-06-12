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

// TestSwitch_AndMatches: a single-condition AND group with a sys
// reference that matches the state must route to "matched_0" (the
// fallback name when no explicit "to" is provided).
func TestSwitch_AndMatches(t *testing.T) {
	s, _ := NewSwitchComponent(nil)
	state := canvas.NewCanvasState("run-1", "task-1")
	state.Sys["x"] = "yes"
	ctx := withStateForTest(context.Background(), state)

	inputs := map[string]any{
		"conditions": []any{
			map[string]any{
				"op": "and",
				"clauses": []any{
					map[string]any{"left": "{{sys.x}}", "op": "==", "right": "yes"},
				},
			},
		},
		"default": "fallback",
	}
	out, err := s.Invoke(ctx, inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["_next"].(string); got != "matched_0" {
		t.Errorf("_next: got %q, want %q", got, "matched_0")
	}
}

// TestSwitch_OrMatches: an OR group with one false clause and one
// true clause must match. Also covers explicit "to" routing and
// multi-clause groups.
func TestSwitch_OrMatches(t *testing.T) {
	s, _ := NewSwitchComponent(nil)
	state := canvas.NewCanvasState("run-2", "task-2")
	state.Sys["score"] = "85"
	state.Sys["flag"] = "no"
	ctx := withStateForTest(context.Background(), state)

	inputs := map[string]any{
		"conditions": []any{
			// First group: AND of two clauses, both must be true.
			// score != "85" is false → AND fails → no match.
			map[string]any{
				"op": "and",
				"clauses": []any{
					map[string]any{"left": "{{sys.score}}", "op": "==", "right": "100"},
					map[string]any{"left": "{{sys.flag}}", "op": "==", "right": "yes"},
				},
			},
			// Second group: OR; first clause is false but second
			// is true → match → route to "match_route".
			map[string]any{
				"op": "or",
				"to": "match_route",
				"clauses": []any{
					map[string]any{"left": "{{sys.flag}}", "op": "==", "right": "yes"},
					map[string]any{"left": "{{sys.score}}", "op": "==", "right": "85"},
				},
			},
		},
		"default": "fallback",
	}
	out, err := s.Invoke(ctx, inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["_next"].(string); got != "match_route" {
		t.Errorf("_next: got %q, want %q", got, "match_route")
	}
}

// TestSwitch_DefaultFallback: when no condition matches, the default
// cpn_id from inputs["default"] is returned.
func TestSwitch_DefaultFallback(t *testing.T) {
	s, _ := NewSwitchComponent(nil)
	state := canvas.NewCanvasState("run-3", "task-3")
	state.Sys["x"] = "no"
	ctx := withStateForTest(context.Background(), state)

	inputs := map[string]any{
		"conditions": []any{
			map[string]any{
				"op": "and",
				"clauses": []any{
					map[string]any{"left": "{{sys.x}}", "op": "==", "right": "yes"},
				},
			},
		},
		"default": "fallback_0",
	}
	out, err := s.Invoke(ctx, inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["_next"].(string); got != "fallback_0" {
		t.Errorf("_next: got %q, want %q", got, "fallback_0")
	}
}

// TestSwitch_ContainsAndEmpty covers two operators that are easy to
// regress: "contains" (substring match) and "empty" (nil/empty test).
func TestSwitch_ContainsAndEmpty(t *testing.T) {
	s, _ := NewSwitchComponent(nil)
	state := canvas.NewCanvasState("run-4", "task-4")
	state.Sys["body"] = "hello world"
	state.Sys["opt"] = ""
	ctx := withStateForTest(context.Background(), state)

	inputs := map[string]any{
		"conditions": []any{
			map[string]any{
				"op": "and",
				"clauses": []any{
					map[string]any{"left": "{{sys.body}}", "op": "contains", "right": "world"},
					map[string]any{"left": "{{sys.opt}}", "op": "empty"},
				},
			},
		},
		"default": "x",
	}
	out, err := s.Invoke(ctx, inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["_next"].(string); got != "matched_0" {
		t.Errorf("_next: got %q, want %q", got, "matched_0")
	}
}
