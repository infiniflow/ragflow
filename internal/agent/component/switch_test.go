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

// nextTargets extracts the _next field as a list of target cpn_ids.
// Switch.Invoke now returns _next as []any (list of strings) to
// support Python's multi-target "to" field.
func nextTargets(out map[string]any) []string {
	raw, ok := out["_next"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []any:
		targets := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				targets = append(targets, s)
			}
		}
		return targets
	case string:
		if v != "" {
			return []string{v}
		}
		return nil
	}
	return nil
}

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
	targets := nextTargets(out)
	if len(targets) != 1 || targets[0] != "matched_0" {
		t.Errorf("_next: got %v, want [\"matched_0\"]", targets)
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
	targets := nextTargets(out)
	if len(targets) != 1 || targets[0] != "match_route" {
		t.Errorf("_next: got %v, want [\"match_route\"]", targets)
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
	targets := nextTargets(out)
	if len(targets) != 1 || targets[0] != "fallback_0" {
		t.Errorf("_next: got %v, want [\"fallback_0\"]", targets)
	}
}

func TestSwitch_LegacyEndCpnIDsFallback(t *testing.T) {
	s, _ := NewSwitchComponent(nil)
	state := canvas.NewCanvasState("run-end-cpn", "task-end-cpn")
	state.Sys["x"] = "no"
	ctx := withStateForTest(context.Background(), state)

	inputs := map[string]any{
		"conditions": []any{
			map[string]any{
				"logical_operator": "and",
				"items": []any{
					map[string]any{"cpn_id": "sys.x", "operator": "=", "value": "yes"},
				},
			},
		},
		"end_cpn_ids": []any{"legacy_fallback"},
	}
	out, err := s.Invoke(ctx, inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	targets := nextTargets(out)
	if len(targets) != 1 || targets[0] != "legacy_fallback" {
		t.Errorf("_next: got %v, want [\"legacy_fallback\"]", targets)
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
	targets := nextTargets(out)
	if len(targets) != 1 || targets[0] != "matched_0" {
		t.Errorf("_next: got %v, want [\"matched_0\"]", targets)
	}
}

func TestSwitch_LegacyConditionsAndArrayTo(t *testing.T) {
	s, _ := NewSwitchComponent(map[string]any{
		"conditions": []any{
			map[string]any{
				"logical_operator": "and",
				"items": []any{
					map[string]any{
						"cpn_id":   "UserFillUp:Menu@demo",
						"operator": "=",
						"value":    "loop",
					},
				},
				"to": []any{"Loop:InputUntil1"},
			},
		},
		"default": "Message:Help",
	})
	state := canvas.NewCanvasState("run-legacy", "task-legacy")
	state.SetVar("UserFillUp:Menu", "demo", "loop")
	ctx := withStateForTest(context.Background(), state)

	out, err := s.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	targets := nextTargets(out)
	if len(targets) != 1 || targets[0] != "Loop:InputUntil1" {
		t.Errorf("_next: got %v, want [\"Loop:InputUntil1\"]", targets)
	}
}

// TestSwitch_MultiTargetTo verifies that Switch returns all cpn_ids
// from a multi-element "to" field. This mirrors Python's behavior
// where a condition can route to multiple downstream nodes
// simultaneously (e.g. "data_ops" → ["DataOperations:UpdateSample",
// "ListOperations:Top2"]).
func TestSwitch_MultiTargetTo(t *testing.T) {
	s, _ := NewSwitchComponent(nil)
	state := canvas.NewCanvasState("run-multi", "task-multi")
	state.SetVar("UserFillUp:Menu", "demo", "data_ops")
	ctx := withStateForTest(context.Background(), state)

	inputs := map[string]any{
		"conditions": []any{
			map[string]any{
				"logical_operator": "and",
				"items": []any{
					map[string]any{
						"cpn_id":   "UserFillUp:Menu@demo",
						"operator": "=",
						"value":    "data_ops",
					},
				},
				"to": []any{"DataOperations:UpdateSample", "CodeExec:FunnyBronsShare"},
			},
		},
		"default": "Message:Help",
	}
	out, err := s.Invoke(ctx, inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	targets := nextTargets(out)
	if len(targets) != 2 || targets[0] != "DataOperations:UpdateSample" || targets[1] != "CodeExec:FunnyBronsShare" {
		t.Errorf("_next: got %v, want [\"DataOperations:UpdateSample\", \"CodeExec:FunnyBronsShare\"]", targets)
	}
}
