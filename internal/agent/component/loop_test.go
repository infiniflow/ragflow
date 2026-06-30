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

// Package component — Loop unit tests.
//
// LoopComponent is a no-op marker in the current architecture:
// real loop execution is driven by workflowx.AddLoopNode,
// installed by canvas.BuildWorkflow when it sees a Loop cpn in
// the DSL. The tests in this file exercise the contract
// LoopComponent DOES expose — registry / factory / param parsing
// / Name / Inputs / Outputs / no-op Invoke / no-op Stream — and
// confirm the LoopItem / ExitLoop names are gone from the
// registry.
package component

import (
	"context"
	"testing"
)

// TestLoop_Registered confirms "Loop" is in the registry and
// "LoopItem" / "ExitLoop" are not. The canvas engine relies on this
// for DSL introspection (component.RegisteredNames).
func TestLoop_Registered(t *testing.T) {
	names := RegisteredNames()
	hasLoop, hasLoopItem, hasExitLoop := false, false, false
	for _, n := range names {
		switch n {
		case "loop":
			hasLoop = true
		case "loopitem":
			hasLoopItem = true
		case "exitloop":
			hasExitLoop = true
		}
	}
	if !hasLoop {
		t.Errorf("Loop not registered; RegisteredNames=%v", names)
	}
	if hasLoopItem {
		t.Errorf("LoopItem is still registered; expected gone. RegisteredNames=%v", names)
	}
	if hasExitLoop {
		t.Errorf("ExitLoop is still registered; expected gone. RegisteredNames=%v", names)
	}
}

// TestLoop_FactoryReturnsComponent confirms the factory registered
// for "Loop" produces a Component.
func TestLoop_FactoryReturnsComponent(t *testing.T) {
	c, err := New("Loop", map[string]any{
		"loop_variables": []any{
			map[string]any{"variable": "x", "input_mode": "constant", "value": 1, "type": "number"},
		},
	})
	if err != nil {
		t.Fatalf("New(Loop): %v", err)
	}
	if c.Name() != "Loop" {
		t.Errorf("Name: got %q, want \"Loop\"", c.Name())
	}
}

// TestLoop_InvokeIsNoOp confirms LoopComponent.Invoke returns an
// empty map and a nil error. State writes from this method are
// silently dropped by the eino graph because LoopComponent is not
// registered as an eino node when the macro expansion fires.
func TestLoop_InvokeIsNoOp(t *testing.T) {
	c := NewLoopComponent(loopParam{
		LoopVariables: []map[string]any{
			{"variable": "counter", "input_mode": "constant", "value": 7, "type": "number"},
		},
	})
	out, err := c.Invoke(context.Background(), map[string]any{"in": 1})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("Invoke should return an empty map, got %v", out)
	}
}

// TestLoop_StreamMirrorsInvoke confirms Stream yields exactly one
// empty-map chunk and closes.
func TestLoop_StreamMirrorsInvoke(t *testing.T) {
	c := NewLoopComponent(loopParam{})
	ch, err := c.Stream(context.Background(), nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	got, ok := <-ch
	if !ok {
		t.Fatal("Stream channel closed without emitting")
	}
	if len(got) != 0 {
		t.Errorf("Stream chunk: got %v, want empty map", got)
	}
	if _, open := <-ch; open {
		t.Errorf("Stream channel did not close after one chunk")
	}
}

// TestLoop_ParamUpdate covers the loopParam.Update contract for the
// loop_variables, loop_termination_condition, logical_operator and
// maximum_loop_count fields. The canvas package's buildLoopExpansion
// reads these from the raw params map directly, but loopParam.Update
// is the canonical parser that the factory uses; it must round-trip
// the four supported fields.
func TestLoop_ParamUpdate(t *testing.T) {
	var p loopParam
	if err := p.Update(map[string]any{
		"loop_variables": []any{
			map[string]any{"variable": "x", "input_mode": "constant", "value": 0, "type": "number"},
		},
		"loop_termination_condition": []any{
			map[string]any{"variable": "x", "operator": "≥", "value": 3, "input_mode": "constant"},
		},
		"logical_operator":   "or",
		"maximum_loop_count": 5,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := len(p.LoopVariables); got != 1 {
		t.Errorf("LoopVariables: got %d, want 1", got)
	}
	if got := len(p.LoopTerminationCondition); got != 1 {
		t.Errorf("LoopTerminationCondition: got %d, want 1", got)
	}
	if p.LogicalOperator != "or" {
		t.Errorf("LogicalOperator: got %q, want \"or\"", p.LogicalOperator)
	}
	if p.MaximumLoopCount != 5 {
		t.Errorf("MaximumLoopCount: got %d, want 5", p.MaximumLoopCount)
	}
}

// TestLoop_ParamAsDict confirms AsDict round-trips the four
// supported fields when set, and omits them when zero.
func TestLoop_ParamAsDict(t *testing.T) {
	p := &loopParam{
		LoopVariables: []map[string]any{
			{"variable": "x", "input_mode": "constant", "value": 0, "type": "number"},
		},
		LogicalOperator:  "and",
		MaximumLoopCount: 0,
	}
	d := p.AsDict()
	if _, ok := d["loop_variables"]; !ok {
		t.Errorf("AsDict: missing loop_variables")
	}
	if v, _ := d["logical_operator"].(string); v != "and" {
		t.Errorf("AsDict logical_operator: got %v, want \"and\"", v)
	}
	if _, ok := d["maximum_loop_count"]; ok {
		t.Errorf("AsDict: maximum_loop_count=0 should be omitted")
	}

	// Zero loopParam → empty AsDict.
	empty := (&loopParam{}).AsDict()
	if len(empty) != 0 {
		t.Errorf("AsDict zero: got %v, want empty", empty)
	}
}

// TestLoop_ParamCheckAlwaysTrue confirms Check is a no-op validator
// (mirrors Python's always-True check()).
func TestLoop_ParamCheckAlwaysTrue(t *testing.T) {
	if err := (&loopParam{}).Check(); err != nil {
		t.Errorf("Check: got %v, want nil", err)
	}
}
