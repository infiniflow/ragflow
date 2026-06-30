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

// Package component — Begin component (T3).
//
// Begin is the DSL entry node. It injects the request's `inputs` into
// the shared *CanvasState.Sys namespace and passes the input map
// through to its downstream unchanged. File-input handling
// (FileService.get_files) handles `query`, `user_id`, and file
// inputs alike.
package component

import (
	"context"
	"fmt"
	"maps"

	"ragflow/internal/agent/runtime"
)

// mapsCopy is a thin alias for the stdlib maps.Copy to keep the call
// sites uniform with the rest of the package (which uses the same name
// in switch.go and message.go).
func mapsCopy(dst, src map[string]any) {
	maps.Copy(dst, src)
}

const componentNameBegin = "Begin"

// BeginComponent is the canvas entry node. The exported fields are
// populated by the factory (registered via init) from the DSL params map.
// ParamBase surface is intentionally omitted for P0 — Begin is trivial
// and needs no validation beyond what the State writes perform.
type BeginComponent struct {
	name string
}

// NewBeginComponent constructs a Begin component. It accepts the DSL params
// map but does not retain it (Begin has no per-instance configuration).
func NewBeginComponent(_ map[string]any) (Component, error) {
	return &BeginComponent{name: componentNameBegin}, nil
}

// Name returns the registered component name. Used by the registry and
// the eino node-name injection in BuildWorkflow.
func (b *BeginComponent) Name() string { return b.name }

// Invoke writes inputs["query"] and (when present) inputs["user_id"] into
// the shared *CanvasState.Sys namespace, then returns the input map as
// outputs unchanged. The input map is shallow-copied to avoid aliasing
// surprises across concurrent goroutines that share an inputs map.
func (b *BeginComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil {
		return nil, fmt.Errorf("Begin: %w", err)
	}
	if state == nil {
		return nil, fmt.Errorf("Begin: nil canvas state")
	}

	// Query: required to drive downstream components.
	query, _ := inputs["query"].(string)
	state.Sys["query"] = query

	// Optional user_id — present in interactive chat flows, absent in
	// background jobs. Always a string when set; cast failure silently
	// drops the value (mirrors Python's getattr fallback).
	if uid, ok := inputs["user_id"].(string); ok && uid != "" {
		state.Sys["user_id"] = uid
	}

	// Webhook payload injection. The webhook HTTP handler sets
	// root["webhook_payload"] (see service/agent.go RunAgentWithWebhook)
	// which BuildWorkflow forwards into inputs. Surfacing it on
	// state.Sys lets downstream components read sys.webhook_payload the
	// same way they read sys.query / sys.user_id. The chat path never
	// sets this key, so existing tests stay green.
	if payload, ok := inputs["webhook_payload"].(map[string]any); ok && len(payload) > 0 {
		state.Sys["webhook_payload"] = payload
	}

	// Passthrough: a shallow copy keeps the caller's map un-aliased.
	out := make(map[string]any, len(inputs))
	mapsCopy(out, inputs)
	return out, nil
}

// Stream is a synchronous facade over Invoke for P0. SSE streaming of
// Begin output is not meaningful (Begin has no I/O), so the channel
// receives a single payload and closes — same shape as Invoke's return.
func (b *BeginComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := b.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns parameter metadata. Descriptions are short; the doc
// strings live on the struct / method above.
func (b *BeginComponent) Inputs() map[string]string {
	return map[string]string{
		"query":           "User query string (the chat input).",
		"user_id":         "Optional user/tenant identifier.",
		"webhook_payload": "Optional structured webhook request (set by the webhook HTTP handler; absent on chat flows).",
		"inputs":          "Optional free-form inputs map; passthrough only.",
	}
}

// Outputs returns the same keys as Inputs (Begin is a passthrough).
func (b *BeginComponent) Outputs() map[string]string {
	return map[string]string{
		"query":           "Query string (passthrough).",
		"user_id":         "User id, if provided (passthrough).",
		"webhook_payload": "Webhook request payload, if provided (passthrough; also written to state.Sys[webhook_payload]).",
		"inputs":          "Raw inputs map (passthrough).",
	}
}

func init() {
	Register(componentNameBegin, NewBeginComponent)
}
