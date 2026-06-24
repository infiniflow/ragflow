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
	"reflect"
	"testing"

	"ragflow/internal/agent/canvas"
)

// TestBegin_InjectsSys verifies the canonical happy path: a query flows
// through Invoke and lands in state.Sys["query"]. user_id is optional
// and absent in this test (omitted from inputs entirely).
func TestBegin_InjectsSys(t *testing.T) {
	c, err := NewBeginComponent(nil)
	if err != nil {
		t.Fatalf("NewBeginComponent: %v", err)
	}
	state := canvas.NewCanvasState("run-1", "task-1")
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{"query": "hello"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := state.Sys["query"].(string); got != "hello" {
		t.Errorf("state.Sys[query]: got %q, want %q", got, "hello")
	}
	// user_id absent in inputs → must not be present in state.Sys
	if _, ok := state.Sys["user_id"]; ok {
		t.Errorf("state.Sys[user_id] should not be set when inputs lack it; got %v", state.Sys["user_id"])
	}
	// Output passthrough
	if out["query"] != "hello" {
		t.Errorf("outputs[query]: got %v, want %q", out["query"], "hello")
	}
}

// TestBegin_PassesThroughInputs asserts the full inputs map — including
// arbitrary keys beyond query / user_id — is returned unchanged as
// outputs. This is the contract downstream components rely on to access
// DSL-level inputs the engine has not explicitly modeled.
func TestBegin_PassesThroughInputs(t *testing.T) {
	c, _ := NewBeginComponent(nil)
	state := canvas.NewCanvasState("run-2", "task-2")
	ctx := canvas.WithState(context.Background(), state)

	inputs := map[string]any{
		"query":   "what is ragflow",
		"user_id": "tenant-7",
		"inputs":  map[string]any{"k": "v"},
		"extra":   42,
	}
	out, err := c.Invoke(ctx, inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !reflect.DeepEqual(out, inputs) {
		t.Errorf("output passthrough failed:\n got  %v\n want %v", out, inputs)
	}
	if got, _ := state.Sys["user_id"].(string); got != "tenant-7" {
		t.Errorf("state.Sys[user_id]: got %q, want %q", got, "tenant-7")
	}
}

// withStateForTest is a thin alias for canvas.WithState kept for
// readability at the test call sites. Declared once in this file; the
// other test files in this package (message_test.go, switch_test.go)
// reference the same symbol because Go test files share a package.
func withStateForTest(ctx context.Context, s *canvas.CanvasState) context.Context {
	return canvas.WithState(ctx, s)
}
