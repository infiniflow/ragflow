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

// TestBegin_InjectsWebhookPayload pins the contract added for the
// webhook HTTP handler: when inputs["webhook_payload"] is present, Begin
// must surface it on state.Sys["webhook_payload"] so downstream
// components (Retrieval, Agent, etc.) can read sys.webhook_payload the
// same way they read sys.query / sys.user_id.
//
// Mirrors python: agent/canvas.py (Begin component) reading
// `webhook_payload` from inputs and writing to state.Sys in the webhook
// branch.
func TestBegin_InjectsWebhookPayload(t *testing.T) {
	c, _ := NewBeginComponent(nil)
	state := canvas.NewCanvasState("run-3", "task-3")
	ctx := canvas.WithState(context.Background(), state)

	payload := map[string]any{
		"query":   map[string]any{"q": "hello"},
		"headers": map[string]any{"x-token": "abc"},
		"body":    map[string]any{"k": "v"},
	}
	inputs := map[string]any{
		"query":           "",
		"webhook_payload": payload,
	}
	out, err := c.Invoke(ctx, inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, ok := state.Sys["webhook_payload"].(map[string]any)
	if !ok {
		t.Fatalf("state.Sys[webhook_payload] missing or wrong type: %T", state.Sys["webhook_payload"])
	}
	if !reflect.DeepEqual(got, payload) {
		t.Errorf("state.Sys[webhook_payload] mismatch:\n got  %v\n want %v", got, payload)
	}
	// Passthrough preserved.
	if outPayload, _ := out["webhook_payload"].(map[string]any); !reflect.DeepEqual(outPayload, payload) {
		t.Errorf("outputs[webhook_payload] mismatch:\n got  %v\n want %v", outPayload, payload)
	}
}

// TestBegin_AbsentWebhookPayload confirms that the chat path (no
// webhook_payload key in inputs) leaves state.Sys["webhook_payload"]
// unset — adding the new branch must NOT pollute existing callers.
func TestBegin_AbsentWebhookPayload(t *testing.T) {
	c, _ := NewBeginComponent(nil)
	state := canvas.NewCanvasState("run-4", "task-4")
	ctx := canvas.WithState(context.Background(), state)

	if _, err := c.Invoke(ctx, map[string]any{"query": "plain chat"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if _, ok := state.Sys["webhook_payload"]; ok {
		t.Errorf("state.Sys[webhook_payload] should not be set when inputs lack it; got %v", state.Sys["webhook_payload"])
	}
}

// TestBegin_EmptyWebhookPayload confirms that an explicitly empty map
// is treated as "not present" — matching the python `if payload:` guard.
func TestBegin_EmptyWebhookPayload(t *testing.T) {
	c, _ := NewBeginComponent(nil)
	state := canvas.NewCanvasState("run-5", "task-5")
	ctx := canvas.WithState(context.Background(), state)

	if _, err := c.Invoke(ctx, map[string]any{
		"query":           "",
		"webhook_payload": map[string]any{},
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if _, ok := state.Sys["webhook_payload"]; ok {
		t.Errorf("state.Sys[webhook_payload] should not be set for empty payload; got %v", state.Sys["webhook_payload"])
	}
}
