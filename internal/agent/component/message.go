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

// Package component — Message component (T3, plan §2.11.3 row 4).
//
// Message is the canvas terminal output node. It resolves a Jinja2-style
// {{...}} template against the current *CanvasState and (optionally) emits
// the result as a single SSE chunk. Memory persistence and chunked
// streaming are deferred (plan §2.11.6 P0 scope): the P0 implementation
// resolves the template once, returns it as outputs["content"], and
// exposes Stream() that yields one chunk + closes.
package component

import (
	"context"
	"fmt"
	"log"
	"maps"

	"ragflow/internal/agent/runtime"
)

const componentNameMessage = "Message"

// MessageComponent is the canvas terminal output node. It owns the
// resolved text template as a per-instance field — the factory sets
// it from the DSL params at build time, and Invoke falls back to it
// when the input map does not carry a fresh "text" override.
type MessageComponent struct {
	name string
	text string
}

// NewMessageComponent constructs a Message component. The params map
// may carry:
//
//   - "text"    (string) — the canonical v2 name
//   - "content" (string | []string | []any) — the v1 name; when a
//     list, the first element is taken as the template (matches the
//     Python v1 message surface where content is a list of paragraphs)
//
// At least one must produce a non-empty string; otherwise the node
// emits an empty content (it is the canvas terminal, so a runtime
// error would be louder than a missing template).
func NewMessageComponent(params map[string]any) (Component, error) {
	tpl := extractMessageText(params)
	return &MessageComponent{name: componentNameMessage, text: tpl}, nil
}

// extractMessageText reads text / content from params in the v1 / v2
// order documented on NewMessageComponent. Returns the empty string
// when neither key is present or the value is not a string-shaped
// scalar.
func extractMessageText(params map[string]any) string {
	if v, ok := params["text"].(string); ok {
		return v
	}
	if v, ok := params["content"]; ok {
		switch x := v.(type) {
		case string:
			return x
		case []string:
			if len(x) > 0 {
				return x[0]
			}
		case []any:
			if len(x) > 0 {
				if s, ok := x[0].(string); ok {
					return s
				}
			}
		}
	}
	return ""
}

// Name returns the registered component name.
func (m *MessageComponent) Name() string { return m.name }

// Invoke resolves inputs["text"] (or the per-instance text seeded
// from params at build time) as a template against the current
// *CanvasState, returns the resolved string at outputs["content"], and
// (if inputs["stream"] == true) records the number of chunks in
// outputs["streamed_chunks"]. Memory persistence (memory_save) is
// logged as deferred to a later phase per the P0 plan.
//
// inputs["text"] takes precedence over the per-instance text so the
// same node can be reused with different templates at run time when
// the orchestrator wants to override the DSL-declared value.
func (m *MessageComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil {
		return nil, fmt.Errorf("Message: %w", err)
	}
	if state == nil {
		return nil, fmt.Errorf("Message: nil canvas state")
	}

	text, _ := inputs["text"].(string)
	if text == "" {
		text = m.text
	}
	resolved, err := runtime.ResolveTemplate(text, state)
	if err != nil {
		// ResolveTemplate surfaces unresolved references as errors, but
		// the partial output (with empty-string substitutions) is still
		// returned so the SSE consumer can choose to log it. Match
		// the existing canvas package's contract here.
		return nil, fmt.Errorf("Message: template resolve: %w", err)
	}

	if memSave, _ := inputs["memory_save"].(bool); memSave {
		log.Printf("Message: memory_save=true (memory persistence deferred to Phase 2.5)")
	}

	out := map[string]any{"content": resolved}
	if streamOn, _ := inputs["stream"].(bool); streamOn {
		// P0: one chunk for the whole resolved content. A later phase
		// can split on token / sentence boundaries.
		out["streamed_chunks"] = 1
	}
	return out, nil
}

// Stream is the SSE variant. The P0 implementation produces a single
// chunk containing the resolved content (key "content") and closes the
// channel. A future phase can split the resolved string into multiple
// chunks; for now the contract is "one chunk, then close".
func (m *MessageComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := m.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns the public parameter surface. Field types match the
// Python DSL contract (text template, stream toggle, memory_save toggle).
func (m *MessageComponent) Inputs() map[string]string {
	return map[string]string{
		"text":        "Template string with {{...}} references; resolved against the canvas state.",
		"stream":      "When true, the resolved content is delivered as an SSE stream (P0: one chunk).",
		"memory_save": "When true, persist the message to API4Conversation (deferred to Phase 2.5).",
	}
}

// Outputs returns the resolved template plus the streamed-chunk counter.
func (m *MessageComponent) Outputs() map[string]string {
	return map[string]string{
		"content":         "Resolved template string (the message text).",
		"streamed_chunks": "Number of SSE chunks emitted (present when stream=true).",
	}
}

// mapCopy shallow-copies src into a fresh map. Used to keep Message's
// passthrough outputs un-aliased from the caller's inputs map.
func mapCopy(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	maps.Copy(out, src)
	return out
}

func init() {
	Register(componentNameMessage, NewMessageComponent)
}
