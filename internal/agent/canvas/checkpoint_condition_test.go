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

package canvas

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestRestoredLoopNumberConditions(t *testing.T) {
	state := NewCanvasState("run", "session")
	ctx := WithState(t.Context(), state)
	data := []byte(`{"id":11099848653006015581,"ids":[9007199254740992,9007199254740993,11099848653006015581],"count":2}`)
	if err := restoreLoopVariables(ctx, "loop", data); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, variable, op string
		rhs                any
		want               bool
	}{
		{"positive ID", "id", ">", float64(0), true},
		{"ID inequality", "id", "≠", float64(0), true},
		{"ordinary count", "count", "≥", float64(2), true},
		{"same represented value", "ids", "contains", float64(9007199254740992), true},
		{"different adjacent value", "ids", "not contains", json.Number("9007199254740994"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := evalOneLoopCondition(state, "loop", loopConditionSpec{Variable: tc.variable, Operator: tc.op, Value: tc.rhs, InputMode: "constant"})
			if err != nil || got != tc.want {
				t.Fatalf("condition=%v, err=%v, want %v", got, err, tc.want)
			}
		})
	}
	id, _ := state.GetVar("loop@id")
	if id != json.Number("11099848653006015581") {
		t.Fatalf("condition mutated ID: %v (%T)", id, id)
	}
}

func TestLoopListConditionsComparePreservedNumbersExactly(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b any
		want bool
	}{
		{"exact power of two", json.Number("9007199254740992"), float64(9007199254740992), true},
		{"adjacent exact ID", json.Number("9007199254740993"), float64(9007199254740992), false},
		{"same exact ID", json.Number("11099848653006015581"), json.Number("11099848653006015581"), true},
		{"different exact ID", json.Number("11099848653006015581"), json.Number("11099848653006015582"), false},
		{"same negative", json.Number("-9007199254740992"), float64(-9007199254740992), true},
		{"exact native integer", json.Number("9007199254740993"), int64(9007199254740993), true},
		{"number is not string", json.Number("9007199254740992"), "9007199254740992", false},
		{"ordinary types unchanged", int(2), float64(2), false},
		{"ordinary equality unchanged", float64(2), float64(2), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, pair := range [][2]any{{tc.a, tc.b}, {tc.b, tc.a}} {
				for _, op := range []string{"is", "is not", "contains", "not contains"} {
					rhs := pair[1]
					if op == "is" || op == "is not" {
						rhs = []any{rhs}
					}
					got, err := evaluateCondition([]any{pair[0]}, op, rhs)
					want := tc.want
					if op == "is not" || op == "not contains" {
						want = !want
					}
					if err != nil || got != want {
						t.Errorf("%s %s %s = %v (%v), want %v", fmt.Sprint(pair[0]), op, fmt.Sprint(pair[1]), got, err, want)
					}
				}
			}
		})
	}
}
