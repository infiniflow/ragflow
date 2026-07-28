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

	"gorm.io/gorm"
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
	name   string
	params map[string]any
}

// NewBeginComponent constructs a Begin component. It retains the DSL
// params so Invoke can apply the single-input sys.query fallback
// (mirroring Python Begin._merge_runtime_inputs).
func NewBeginComponent(params map[string]any) (Component, error) {
	return &BeginComponent{name: componentNameBegin, params: params}, nil
}

// Name returns the registered component name. Used by the registry and
// the eino node-name injection in BuildWorkflow.
func (b *BeginComponent) Name() string { return b.name }

// Invoke writes inputs["query"] and (when present) inputs["user_id"] into
// the shared *CanvasState.Sys namespace, then returns the input map as
// outputs unchanged. The input map is shallow-copied to avoid aliasing
// surprises across concurrent goroutines that share an inputs map.
func (b *BeginComponent) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
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

	// Custom-input fallback (mirrors Python Begin._merge_runtime_inputs):
	// when the request carried no runtime inputs (nothing seeded into
	// Outputs["begin"] by the service layer) and the DSL declares
	// exactly one input field, sys.query becomes that field's value so
	// downstream {begin@<key>} references resolve.
	var fallbackKey, fallbackValue string
	if beginBucketEmpty(state) {
		if fieldKey, ok := b.singleDeclaredInputKey(); ok {
			if q, _ := state.Sys["query"].(string); q != "" {
				fallbackKey, fallbackValue = fieldKey, q
			}
		}
	}

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
	out := make(map[string]any, len(inputs)+1)
	mapsCopy(out, inputs)
	if fallbackKey != "" {
		out[fallbackKey] = fallbackValue
		// Persist into the reserved "begin" namespace the same way
		// request-supplied inputs are seeded, so {begin@<key>} refs
		// resolve even when the DSL node id is e.g. begin_0.
		state.SetVar("begin", fallbackKey, fallbackValue)
	}
	return out, nil
}

// beginBucketEmpty reports whether no begin outputs have been seeded
// for this run (i.e. the request did not carry runtime inputs).
func beginBucketEmpty(state *runtime.CanvasState) bool {
	if state == nil || state.Outputs == nil {
		return true
	}
	return len(state.Outputs["begin"]) == 0
}

// singleDeclaredInputKey returns the sole declared custom input key
// from the DSL params ("inputs" map), or false when the node declares
// zero or multiple fields.
func (b *BeginComponent) singleDeclaredInputKey() (string, bool) {
	declared, ok := b.params["inputs"].(map[string]any)
	if !ok || len(declared) != 1 {
		return "", false
	}
	for key := range declared {
		return key, true
	}
	return "", false
}

// Stream is a synchronous facade over Invoke for P0. SSE streaming of
// Begin output is not meaningful (Begin has no I/O), so the channel
// receives a single payload and closes — same shape as Invoke's return.
func (b *BeginComponent) Stream(ctx context.Context, db *gorm.DB, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := b.Invoke(ctx, db, inputs)
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
