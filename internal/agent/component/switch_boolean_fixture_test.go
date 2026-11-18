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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ragflow/internal/agent/canvas"
)

// TestSwitch_BooleanDSLFixture verifies that legacy DSL conditions route
// actual boolean variables correctly against both string and JSON boolean
// comparison values, including the ELSE branch when no condition matches.
func TestSwitch_BooleanDSLFixture(t *testing.T) {
	path := filepath.Join("..", "dsl", "testdata", "switch_boolean.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Switch fixture: %v", err)
	}

	var fixture canvas.Canvas
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("decode Switch fixture: %v", err)
	}
	node, ok := fixture.Components["Switch:Boolean"]
	if !ok || node.Obj.ComponentName != "Switch" {
		t.Fatalf("fixture missing Switch:Boolean component: %+v", node.Obj)
	}
	switchNode, err := NewSwitchComponent(node.Obj.Params)
	if err != nil {
		t.Fatalf("NewSwitchComponent: %v", err)
	}
	beginNode, ok := fixture.Components["begin"]
	if !ok || beginNode.Obj.ComponentName != "Begin" {
		t.Fatalf("fixture missing Begin component: %+v", beginNode.Obj)
	}
	begin, err := NewBeginComponent(beginNode.Obj.Params)
	if err != nil {
		t.Fatalf("NewBeginComponent: %v", err)
	}

	cases := []struct {
		name   string
		key    string
		value  bool
		target string
	}{
		{name: "equals string true", key: "eq_string", value: true, target: "Message:EqualString"},
		{name: "equals boolean false", key: "eq_bool", value: false, target: "Message:EqualBoolean"},
		{name: "not equals string false", key: "ne_string", value: true, target: "Message:NotEqualString"},
		{name: "not equals boolean true", key: "ne_bool", value: false, target: "Message:NotEqualBoolean"},
		{name: "no match falls through", target: "Message:Fallback"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := canvas.NewCanvasState("switch-boolean-fixture", tc.name)
			ctx := withStateForTest(t.Context(), state)
			// These defaults make every condition false. Overriding one
			// input tests its matching branch without short-circuiting
			// any preceding condition.
			inputs := map[string]any{
				"eq_string": false,
				"eq_bool":   true,
				"ne_string": false,
				"ne_bool":   true,
			}
			if tc.key != "" {
				inputs[tc.key] = tc.value
			}
			beginOut, err := begin.Invoke(ctx, nil, inputs)
			if err != nil {
				t.Fatalf("Begin.Invoke: %v", err)
			}
			for key, value := range beginOut {
				state.SetVar("begin", key, value)
			}

			out, err := switchNode.Invoke(ctx, nil, nil)
			if err != nil {
				t.Fatalf("Switch.Invoke: %v", err)
			}
			targets := nextTargets(out)
			if len(targets) != 1 || targets[0] != tc.target {
				t.Fatalf("_next = %v, want [%q]", targets, tc.target)
			}
		})
	}
}
