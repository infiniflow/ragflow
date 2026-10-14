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
	"testing"

	"ragflow/internal/agent/canvas"
)

// runOp is a small test helper that evaluates a single Switch
// clause and returns whether the group matched. It wraps the
// internal `evaluateClause` directly so the operator matrix is
// easy to assert without spinning up a full SwitchComponent +
// Invoke round-trip.
func runOp(t *testing.T, left string, op string, right any, sys map[string]any) bool {
	t.Helper()
	state := canvas.NewCanvasState("run-op", "task-op")
	for k, v := range sys {
		state.Sys[k] = v
	}
	clause := map[string]any{
		"left":  left,
		"op":    op,
		"right": right,
	}
	matched, err := evaluateClause(clause, state)
	if err != nil {
		t.Fatalf("evaluateClause(op=%q): %v", op, err)
	}
	return matched
}

// TestSwitch_Operators_NotContains covers the `not contains` operator
// (Python `switch.py` parity; OQ #13 follow-up).
func TestSwitch_Operators_NotContains(t *testing.T) {
	if matched := runOp(t, "hello world", "not contains", "foo", nil); !matched {
		t.Errorf("not contains(haystack,absent) should match")
	}
	if matched := runOp(t, "hello world", "not contains", "world", nil); matched {
		t.Errorf("not contains(haystack,present) should NOT match")
	}
}

// TestSwitch_Operators_StartWith covers `start with` (prefix match,
// case-insensitive).
func TestSwitch_Operators_StartWith(t *testing.T) {
	if matched := runOp(t, "Hello World", "start with", "hello", nil); !matched {
		t.Errorf("start with should be case-insensitive: 'Hello World' starts with 'hello' should match")
	}
	if matched := runOp(t, "Hello World", "start with", "world", nil); matched {
		t.Errorf("'Hello World' starts with 'world' should NOT match")
	}
	if matched := runOp(t, "/api/v1/canvas", "start with", "/api/", nil); !matched {
		t.Errorf("path prefix match failed")
	}
}

// TestSwitch_Operators_EndWith covers `end with` (suffix match,
// case-insensitive).
func TestSwitch_Operators_EndWith(t *testing.T) {
	if matched := runOp(t, "report.PDF", "end with", ".pdf", nil); !matched {
		t.Errorf("end with should be case-insensitive: 'report.PDF' ends with '.pdf' should match")
	}
	if matched := runOp(t, "image.png", "end with", ".jpg", nil); matched {
		t.Errorf("'image.png' ends with '.jpg' should NOT match")
	}
}

// TestSwitch_Operators_NotEmpty covers `not empty` (negation of
// `empty`).
func TestSwitch_Operators_NotEmpty(t *testing.T) {
	if matched := runOp(t, "{{sys.body}}", "not empty", nil, map[string]any{"body": "hello"}); !matched {
		t.Errorf("not empty on 'hello' should match")
	}
	if matched := runOp(t, "{{sys.body}}", "not empty", nil, map[string]any{"body": ""}); matched {
		t.Errorf("not empty on '' should NOT match")
	}
	// A missing sys value resolves to nil for unary emptiness checks.
	if matched := runOp(t, "{{sys.absent}}", "not empty", nil, map[string]any{}); matched {
		t.Errorf("not empty on missing var should not match")
	}
}

// TestSwitch_Operators_GE covers the ≥ (greater-or-equal) operator.
func TestSwitch_Operators_GE(t *testing.T) {
	if matched := runOp(t, "{{sys.x}}", ">=", 5, map[string]any{"x": 5}); !matched {
		t.Errorf("5 >= 5 should match")
	}
	if matched := runOp(t, "{{sys.x}}", ">=", 5, map[string]any{"x": 6}); !matched {
		t.Errorf("6 >= 5 should match")
	}
	if matched := runOp(t, "{{sys.x}}", ">=", 5, map[string]any{"x": 4}); matched {
		t.Errorf("4 >= 5 should NOT match")
	}
}

// TestSwitch_Operators_LE covers the ≤ (less-or-equal) operator.
func TestSwitch_Operators_LE(t *testing.T) {
	if matched := runOp(t, "{{sys.x}}", "<=", 5, map[string]any{"x": 5}); !matched {
		t.Errorf("5 <= 5 should match")
	}
	if matched := runOp(t, "{{sys.x}}", "<=", 5, map[string]any{"x": 4}); !matched {
		t.Errorf("4 <= 5 should match")
	}
	if matched := runOp(t, "{{sys.x}}", "<=", 5, map[string]any{"x": 6}); matched {
		t.Errorf("6 <= 5 should NOT match")
	}
}

// TestSwitch_Operators_EqualFolded confirms `==` is now case-
// insensitive (Python switch.py parity).
func TestSwitch_Operators_EqualFolded(t *testing.T) {
	if matched := runOp(t, "Hello", "==", "hello", nil); !matched {
		t.Errorf("== should be case-insensitive: 'Hello' == 'hello' should match")
	}
	if matched := runOp(t, "HELLO", "==", "hello", nil); !matched {
		t.Errorf("== should be case-insensitive: 'HELLO' == 'hello' should match")
	}
}

func TestSwitch_OperatorMatrix(t *testing.T) {
	tests := []struct {
		name      string
		left      any
		op        string
		right     any
		wantMatch bool
	}{
		{name: "empty", left: "", op: "empty", wantMatch: true},
		{name: "not empty", left: "hello", op: "not empty", wantMatch: true},
		{name: "equals", left: "hello", op: "==", right: "hello", wantMatch: true},
		{name: "not equals", left: "hello", op: "!=", right: "world", wantMatch: true},
		{name: "contains", left: "hello world", op: "contains", right: "world", wantMatch: true},
		{name: "not contains", left: "hello world", op: "not contains", right: "absent", wantMatch: true},
		{name: "starts with", left: "hello world", op: "start with", right: "hello", wantMatch: true},
		{name: "ends with", left: "hello world", op: "end with", right: "world", wantMatch: true},
		{name: "greater than", left: 6, op: ">", right: 5, wantMatch: true},
		{name: "greater or equal", left: 5, op: ">=", right: 5, wantMatch: true},
		{name: "less than", left: 4, op: "<", right: 5, wantMatch: true},
		{name: "less or equal", left: 5, op: "<=", right: 5, wantMatch: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := canvas.NewCanvasState("operator-matrix", tc.name)
			state.Sys["left"] = tc.left
			if matched := runOp(t, "{{sys.left}}", tc.op, tc.right, state.Sys); matched != tc.wantMatch {
				t.Fatalf("matching case = %v, want %v", matched, tc.wantMatch)
			}

			rejectLeft, rejectRight := tc.left, tc.right
			switch tc.op {
			case "empty":
				rejectLeft = "hello"
			case "not empty":
				rejectLeft = ""
			case "==":
				rejectRight = "world"
			case "!=":
				rejectRight = "hello"
			case "contains":
				rejectRight = "absent"
			case "not contains":
				rejectRight = "world"
			case "start with":
				rejectRight = "world"
			case "end with":
				rejectRight = "hello"
			case ">", ">=":
				rejectLeft = 4
			case "<", "<=":
				rejectLeft = 6
			}
			state.Sys["left"] = rejectLeft
			if matched := runOp(t, "{{sys.left}}", tc.op, rejectRight, state.Sys); matched {
				t.Fatalf("non-matching case = %v, want false", matched)
			}
		})
	}
}

func TestSwitch_EmptyValueTypes(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "nil", value: nil, want: true},
		{name: "empty string", value: "", want: true},
		{name: "whitespace", value: " ", want: false},
		{name: "zero", value: 0, want: false},
		{name: "false", value: false, want: false},
		{name: "empty array", value: []any{}, want: true},
		{name: "non-empty array", value: []any{"x"}, want: false},
		{name: "empty object", value: map[string]any{}, want: true},
		{name: "non-empty object", value: map[string]any{"x": 1}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isEmptyValue(tc.value); got != tc.want {
				t.Fatalf("isEmptyValue(%#v) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestSwitch_EmptyResolvedNil(t *testing.T) {
	state := canvas.NewCanvasState("empty-nil", "empty-nil")
	state.SetVar("begin", "a", nil)
	matched, err := evaluateClause(map[string]any{"left": "{{begin@a}}", "op": "empty"}, state)
	if err != nil || !matched {
		t.Fatalf("empty resolved nil = %v, %v; want true, nil", matched, err)
	}
}
