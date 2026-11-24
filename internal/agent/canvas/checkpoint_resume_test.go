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
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cloudwego/eino/compose"
)

func TestLoopResumePreservesLargeVariable(t *testing.T) {
	const searchID = "11099848653006015581"
	for _, tc := range []struct {
		name, variable, operator string
		value                    any
	}{
		{"scalar", "saved", ">", float64(0)},
		{"list equality", "ids", "is", []any{float64(9007199254740992)}},
		{"list contains", "ids", "contains", float64(9007199254740992)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Canvas{
				Components: map[string]CanvasComponent{
					"loop": {Obj: CanvasComponentObj{ComponentName: "Loop", Params: map[string]any{
						"maximum_loop_count":         3,
						"loop_termination_condition": []any{map[string]any{"variable": tc.variable, "operator": tc.operator, "value": tc.value, "input_mode": "constant"}},
						"loop_variables":             []any{map[string]any{"variable": "saved", "input_mode": "variable", "value": "search_0@id", "type": "number"}, map[string]any{"variable": "ids", "input_mode": "variable", "value": "search_0@ids", "type": "array"}},
					}}},
					"input": {Obj: CanvasComponentObj{ComponentName: "UserFillUp", Params: map[string]any{
						"inputs": map[string]any{"value": map[string]any{"type": "line"}},
					}}, Upstream: []string{"loop"}, Downstream: []string{"consumer"}},
					"consumer": {Obj: CanvasComponentObj{ComponentName: "StringTransform", Params: map[string]any{
						"method": "merge", "script": "{{loop@saved}}:{{input@value}}", "delimiters": []any{"|"},
					}}, Upstream: []string{"input"}},
				},
				NodeParents: map[string]string{"input": "loop", "consumer": "loop"},
			}
			store, _ := newTestStore(t, time.Minute)
			compiled, err := Compile(t.Context(), c, WithCheckPointStore(store))
			if err != nil {
				t.Fatal(err)
			}
			newContext := func() context.Context { return WithState(t.Context(), NewCanvasState("run-loop", "session-loop")) }
			ctx := newContext()
			state, _ := GetStateFromContext(ctx)
			state.SetVar("search_0", "id", json.Number(searchID))
			state.SetVar("search_0", "ids", []any{json.Number("9007199254740992")})
			const checkpoint = "loop-large-id"
			_, err = compiled.Workflow.Invoke(ctx, map[string]any{}, compose.WithCheckPointID(checkpoint))
			if !IsInterruptError(err) {
				t.Fatalf("first run: want interrupt, got %v", err)
			}
			interruptID := FirstInterruptID(ExtractInterruptContexts(err))
			if interruptID == "" {
				t.Fatal("missing interrupt ID")
			}
			resumed := compose.ResumeWithData(newContext(), interruptID, "answer")
			out, err := compiled.Workflow.Invoke(resumed, map[string]any{}, compose.WithCheckPointID(checkpoint))
			if err != nil {
				t.Fatal(err)
			}
			// The consumer reads the loop snapshot after restore, not just the outer canvas state.
			body, _ := out["loop"].(map[string]any)
			if body["result"] != searchID+":answer" {
				t.Fatalf("loop result = %#v, want %s:answer", out, searchID)
			}
		})
	}
}
