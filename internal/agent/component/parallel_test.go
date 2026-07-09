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

// Package component — Parallel unit tests.
//
// ParallelComponent is a no-op marker in the new architecture: real
// parallel execution is driven by workflowx.AddParallelNode, installed
// by canvas.BuildWorkflow when it sees a Parallel cpn in the DSL. The
// tests in this file exercise the contract ParallelComponent DOES
// expose — registry / factory / param parsing / Name / Inputs / Outputs /
// no-op Invoke / no-op Stream.
//
// Note: "Iteration" and "IterationItem" are still in the registry —
// they are the v1 fixture stubs (v1_stubs.go) that the e2e suite in
// internal/agent/canvas/dsl_examples_e2e_test.go needs to compile the
// v1 DSL examples (iteration.json, headhunter_zh.json). They are NOT
// production components; the real parallel / iteration engine lives
// in canvas/loop_subgraph.go and workflowx.AddLoopNode. The v1 stubs
// are deliberately registered under the v1 names so the e2e path
// resolves the factory to something non-panicking.
package component

import (
	"context"
	"slices"
	"testing"
)

// TestParallel_Registered confirms "Parallel" is in the registry.
//
// The v1 stub names "Iteration" and "IterationItem" are also in the
// registry by design — see the package comment above. We do NOT
// assert their absence here.
func TestParallel_Registered(t *testing.T) {
	names := RegisteredNames()
	if !slices.Contains(names, "parallel") {
		t.Errorf("Parallel not registered; RegisteredNames=%v", names)
	}
}

// TestParallel_FactoryReturnsComponent confirms the factory registered
// for "Parallel" produces a Component with the correct name.
func TestParallel_FactoryReturnsComponent(t *testing.T) {
	c, err := New("Parallel", map[string]any{
		"items_ref":       "sys.arr",
		"max_concurrency": 5,
	})
	if err != nil {
		t.Fatalf("New(Parallel): %v", err)
	}
	if c.Name() != "Parallel" {
		t.Errorf("Name: got %q, want \"Parallel\"", c.Name())
	}
}

// TestParallel_InvokeIsNoOp confirms ParallelComponent.Invoke returns
// an empty map and a nil error. State writes from this method are
// silently dropped by the eino graph because ParallelComponent is not
// registered as an eino node when the macro expansion fires.
func TestParallel_InvokeIsNoOp(t *testing.T) {
	c := NewParallelComponent(ParallelParam{
		ItemsRef:       "sys.arr",
		MaxConcurrency: 3,
	})
	out, err := c.Invoke(context.Background(), map[string]any{"in": 1})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("Invoke should return an empty map, got %v", out)
	}
}

// TestParallel_StreamMirrorsInvoke confirms Stream yields exactly one
// empty-map chunk and closes.
func TestParallel_StreamMirrorsInvoke(t *testing.T) {
	c := NewParallelComponent(ParallelParam{})
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

// TestParallel_ParamUpdate covers the ParallelParam.Update contract
// for the items_ref and max_concurrency fields. The canvas package's
// buildParallelExpansion reads these from the raw params map directly,
// but ParallelParam.Update is the canonical parser that the factory
// uses; it must round-trip both supported fields.
func TestParallel_ParamUpdate(t *testing.T) {
	var p ParallelParam
	if err := p.Update(map[string]any{
		"items_ref":       "sys.arr",
		"max_concurrency": 10,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if p.ItemsRef != "sys.arr" {
		t.Errorf("ItemsRef: got %q, want \"sys.arr\"", p.ItemsRef)
	}
	if p.MaxConcurrency != 10 {
		t.Errorf("MaxConcurrency: got %d, want 10", p.MaxConcurrency)
	}
}

// TestParallel_ParamUpdateNilConf confirms Update(nil) is a no-op.
func TestParallel_ParamUpdateNilConf(t *testing.T) {
	var p ParallelParam
	if err := p.Update(nil); err != nil {
		t.Fatalf("Update(nil): %v", err)
	}
	if p.ItemsRef != "" {
		t.Errorf("ItemsRef: got %q, want empty", p.ItemsRef)
	}
	if p.MaxConcurrency != 0 {
		t.Errorf("MaxConcurrency: got %d, want 0", p.MaxConcurrency)
	}
}

// TestParallel_ParamAsDict confirms AsDict round-trips the two
// supported fields when set, and omits them when zero.
func TestParallel_ParamAsDict(t *testing.T) {
	p := &ParallelParam{
		ItemsRef:       "sys.arr",
		MaxConcurrency: 0,
	}
	d := p.AsDict()
	if v, _ := d["items_ref"].(string); v != "sys.arr" {
		t.Errorf("AsDict items_ref: got %v, want \"sys.arr\"", v)
	}
	if _, ok := d["max_concurrency"]; ok {
		t.Errorf("AsDict: max_concurrency=0 should be omitted")
	}

	// Zero ParallelParam → empty AsDict.
	empty := (&ParallelParam{}).AsDict()
	if len(empty) != 0 {
		t.Errorf("AsDict zero: got %v, want empty", empty)
	}
}

// TestParallel_ParamCheckAlwaysTrue confirms Check is a no-op validator.
func TestParallel_ParamCheckAlwaysTrue(t *testing.T) {
	if err := (&ParallelParam{}).Check(); err != nil {
		t.Errorf("Check: got %v, want nil", err)
	}
}
