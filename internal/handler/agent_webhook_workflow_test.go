// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package handler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"ragflow/internal/agent/canvas"
)

// webhookWorkflowLoader runs real canvas nodes while replacing database-backed canvas loading.
type webhookWorkflowLoader struct{ *fakeCanvasLoader }

// RunAgentWithWebhook forwards the cleaned HTTP payload into a compiled Begin-to-Message workflow.
func (f *webhookWorkflowLoader) RunAgentWithWebhook(ctx context.Context, _, _ string, payload map[string]any) (<-chan canvas.RunEvent, error) {
	raw, err := json.Marshal(f.canvas.DSL)
	if err != nil {
		return nil, err
	}
	var workflow canvas.Canvas
	if err := json.Unmarshal(raw, &workflow); err != nil {
		return nil, err
	}
	compiled, err := canvas.Compile(ctx, &workflow)
	if err != nil {
		return nil, err
	}
	state := canvas.NewCanvasState("header-run", "header-session")
	ctx = canvas.WithState(ctx, state)
	if _, err := compiled.Workflow.Invoke(ctx, map[string]any{"query": "", "webhook_payload": payload}); err != nil {
		return nil, err
	}
	content, err := state.GetVar("message@content")
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(map[string]any{"content": content})
	if err != nil {
		return nil, err
	}
	events := make(chan canvas.RunEvent, 1)
	events <- canvas.RunEvent{Type: "message", Data: string(data)}
	close(events)
	return events, nil
}

// TestWebhookHeaderReachesDownstreamMessage covers HTTP parsing, schema extraction, Begin outputs and Message resolution.
func TestWebhookHeaderReachesDownstreamMessage(t *testing.T) {
	for _, field := range []string{"x-request-id", "X-Request-ID", "request_id"} {
		t.Run(field, func(t *testing.T) {
			cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
				"execution_mode": "Streaming",
				"schema": map[string]any{"headers": map[string]any{
					"properties": map[string]any{field: map[string]any{"type": "string"}},
					"required":   []any{field},
				}},
			})
			components := cv.DSL["components"].(map[string]any)
			components["begin"].(map[string]any)["downstream"] = []string{"message"}
			components["message"] = map[string]any{
				"obj":      map[string]any{"component_name": "Message", "params": map[string]any{"content": "{{begin@headers." + field + "}}"}},
				"upstream": []string{"begin"},
			}
			h := &AgentHandler{loader: &webhookWorkflowLoader{&fakeCanvasLoader{canvas: cv}}}
			h.webhookTraceAppender = func(context.Context, string, time.Time, canvas.RunEvent) {}
			c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook/test", `{}`, "application/json")
			c.Request.Header.Set(field, "request-123")
			h.Webhook(c)
			var result struct {
				Message string `json:"message"`
				Success bool   `json:"success"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if !result.Success || result.Message != "request-123" {
				t.Fatalf("downstream response = %s, want original header value", w.Body.String())
			}
		})
	}
}
