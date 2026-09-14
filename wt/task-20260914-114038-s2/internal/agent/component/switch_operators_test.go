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
	// When a var ref fails to resolve, leftValue returns the raw
	// template literal (e.g. "{{sys.absent}}") so that == / != can
	// still operate and a misconfigured ref doesn't crash the run.
	// The raw literal is non-empty, so `not empty` evaluates to
	// true. This is documented in leftValue's comment.
	if matched := runOp(t, "{{sys.absent}}", "not empty", nil, map[string]any{}); !matched {
		t.Errorf("not empty on unresolved var ref (raw literal) should match (raw is non-empty)")
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
