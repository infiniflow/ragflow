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

// TestSwitch_NilUpstreamContainsEmptyNeedleMatches ports the
// regression covered by python PR #16320: when an upstream
// component yields nil and the configured value is the empty
// string, the "contains" operator must match (Python semantics
// after the fix: "" in "anything"). Pre-fix Python crashed with
// AttributeError; pre-port Go returned false because fmt rendered
// nil as "<nil>" instead of "". The fix coerces nil → "" before
// formatting, restoring parity with the Python workflow.
func TestSwitch_NilUpstreamContainsEmptyNeedleMatches(t *testing.T) {
	s, _ := NewSwitchComponent(nil)
	state := canvas.NewCanvasState("run-nil-contains", "task-nil-contains")
	state.Sys["answer"] = nil
	ctx := withStateForTest(context.Background(), state)

	inputs := map[string]any{
		"conditions": []any{
			map[string]any{
				"op": "and",
				"to": []any{"case_target"},
				"clauses": []any{
					map[string]any{"left": "{{sys.answer}}", "op": "contains", "right": ""},
				},
			},
		},
		"default": "else_target",
	}
	out, err := s.Invoke(ctx, inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	targets := nextTargets(out)
	if len(targets) != 1 || targets[0] != "case_target" {
		t.Errorf("_next: got %v, want [\"case_target\"] (nil coerced to \"\" should match empty needle)", targets)
	}
}

// TestSwitch_NilUpstreamContainsNonEmptyDoesNotMatch verifies
// the inverse: nil coerced to "" still must NOT match a
// non-empty needle (we only coerce, we don't synthesize a match).
func TestSwitch_NilUpstreamContainsNonEmptyDoesNotMatch(t *testing.T) {
	s, _ := NewSwitchComponent(nil)
	state := canvas.NewCanvasState("run-nil-needle", "task-nil-needle")
	state.Sys["answer"] = nil
	ctx := withStateForTest(context.Background(), state)

	inputs := map[string]any{
		"conditions": []any{
			map[string]any{
				"op": "and",
				"to": []any{"case_target"},
				"clauses": []any{
					map[string]any{"left": "{{sys.answer}}", "op": "contains", "right": "foo"},
				},
			},
		},
		"default": "else_target",
	}
	out, err := s.Invoke(ctx, inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	targets := nextTargets(out)
	if len(targets) != 1 || targets[0] != "else_target" {
		t.Errorf("_next: got %v, want [\"else_target\"] (\"\" does not contain \"foo\")", targets)
	}
}

// TestSwitch_NilValueContainsDoesNotRaise mirrors python test
// "test_switch_none_value_contains_does_not_raise": the configured
// value can also be nil and the operator must not crash. With
// nil coerced to "" on both sides, "foobar" contains "" matches.
func TestSwitch_NilValueContainsDoesNotRaise(t *testing.T) {
	s, _ := NewSwitchComponent(nil)
	state := canvas.NewCanvasState("run-nil-value", "task-nil-value")
	state.Sys["answer"] = "foobar"
	ctx := withStateForTest(context.Background(), state)

	inputs := map[string]any{
		"conditions": []any{
			map[string]any{
				"op": "and",
				"to": []any{"case_target"},
				"clauses": []any{
					map[string]any{"left": "{{sys.answer}}", "op": "contains", "right": nil},
				},
			},
		},
		"default": "else_target",
	}
	out, err := s.Invoke(ctx, inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	targets := nextTargets(out)
	if len(targets) != 1 || targets[0] != "case_target" {
		t.Errorf("_next: got %v, want [\"case_target\"] (nil value coerced to \"\" matches any string)", targets)
	}
}

// TestSwitch_NilUpstreamStartWithEndWithDoNotCrash guards the
// remaining two string operators covered by PR #16320. They were
// crash-prone in Python for the same reason; in Go they don't
// crash but the nil → "" coercion still applies, so a nil
// upstream with an empty prefix/suffix must match (rather than
// being rendered as "<nil>" and silently missing).
func TestSwitch_NilUpstreamStartWithEndWithDoNotCrash(t *testing.T) {
	s, _ := NewSwitchComponent(nil)
	state := canvas.NewCanvasState("run-nil-start-end", "task-nil-start-end")
	state.Sys["answer"] = nil
	ctx := withStateForTest(context.Background(), state)

	for _, tc := range []struct {
		name string
		op   string
	}{
		{name: "start with", op: "start with"},
		{name: "end with", op: "end with"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inputs := map[string]any{
				"conditions": []any{
					map[string]any{
						"op": "and",
						"to": []any{"case_target"},
						"clauses": []any{
							map[string]any{"left": "{{sys.answer}}", "op": tc.op, "right": ""},
						},
					},
				},
				"default": "else_target",
			}
			out, err := s.Invoke(ctx, inputs)
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			targets := nextTargets(out)
			if len(targets) != 1 || targets[0] != "case_target" {
				t.Errorf("_next: got %v, want [\"case_target\"] (nil coerced to \"\" %s \"\")", targets, tc.op)
			}
		})
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

// TestSwitch_EmptyAndConditionFallsThrough guards PR #15644 port:
// an "and" condition with no clauses (or all clauses filtered out
// by the legacy normaliser) must NOT match. Previously the empty
// `clauses` short-circuit returned `true` (vacuously), routing the
// Switch to the empty group's `to` target before reaching the
// default / end_cpn_ids branch.
func TestSwitch_EmptyAndConditionFallsThrough(t *testing.T) {
	s, _ := NewSwitchComponent(nil)
	state := canvas.NewCanvasState("run-empty-and", "task-1")
	ctx := withStateForTest(context.Background(), state)

	// Empty clauses: must not match. Should fall through to default.
	inputs := map[string]any{
		"conditions": []any{
			map[string]any{
				"op":      "and",
				"clauses": []any{},
				"to":      "WRONG_TARGET",
			},
		},
		"default": "DEFAULT",
	}
	out, err := s.Invoke(ctx, inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	targets := nextTargets(out)
	for _, tgt := range targets {
		if tgt == "WRONG_TARGET" {
			t.Errorf("empty and-condition must not match, but Switch routed to %q", tgt)
		}
	}
}

// TestSwitch_LegacyEmptyItemsFallsThrough covers the legacy
// "logical_operator" / "items" form, where every item has an
// empty `cpn_id` (the bug scenario from the Python PR: items
// list non-empty but every item skipped, so clauses is empty).
func TestSwitch_LegacyEmptyItemsFallsThrough(t *testing.T) {
	s, _ := NewSwitchComponent(nil)
	state := canvas.NewCanvasState("run-legacy-empty", "task-1")
	ctx := withStateForTest(context.Background(), state)

	inputs := map[string]any{
		"conditions": []any{
			map[string]any{
				"logical_operator": "and",
				"items": []any{
					map[string]any{"cpn_id": "", "operator": "contains", "value": "x"},
				},
				"to": "WRONG_TARGET",
			},
		},
		"default": "DEFAULT",
	}
	out, err := s.Invoke(ctx, inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	targets := nextTargets(out)
	for _, tgt := range targets {
		if tgt == "WRONG_TARGET" {
			t.Errorf("all-skipped and-condition must not match, but Switch routed to %q", tgt)
		}
	}
}

// TestSwitch_SatisfiedAndConditionStillRoutes is the negative
// control: a genuinely satisfied "and" condition MUST still match
// after the fix. Without this guard, a refactor that breaks the
// real all() path would slip through.
func TestSwitch_SatisfiedAndConditionStillRoutes(t *testing.T) {
	s, _ := NewSwitchComponent(nil)
	state := canvas.NewCanvasState("run-and-ok", "task-1")
	state.Sys["greeting"] = "hello world"
	ctx := withStateForTest(context.Background(), state)

	inputs := map[string]any{
		"conditions": []any{
			map[string]any{
				"logical_operator": "and",
				"items": []any{
					map[string]any{"cpn_id": "sys.greeting", "operator": "contains", "value": "hello"},
				},
				"to": "CORRECT_TARGET",
			},
		},
		"default": "DEFAULT",
	}
	out, err := s.Invoke(ctx, inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	targets := nextTargets(out)
	if len(targets) == 0 {
		t.Fatalf("expected non-empty _next, got nothing (out=%v)", out)
	}
	found := false
	for _, tgt := range targets {
		if tgt == "CORRECT_TARGET" {
			found = true
		}
	}
	if !found {
		t.Errorf("satisfied and-condition must route to CORRECT_TARGET, got %v", targets)
	}
}
