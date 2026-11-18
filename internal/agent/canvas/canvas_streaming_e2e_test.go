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
	"errors"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"
	"ragflow/internal/agent/chat"
	"ragflow/internal/agent/component"
	"ragflow/internal/agent/runtime"
)

type mockCanvasStreamingInvoker struct {
	mu      sync.Mutex
	calls   int
	deltas  []string
	postErr error
}

func (m *mockCanvasStreamingInvoker) Invoke(ctx context.Context, db *gorm.DB, req chat.Request) (*chat.Response, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	return &chat.Response{Content: "full"}, nil
}

func (m *mockCanvasStreamingInvoker) Stream(ctx context.Context, db *gorm.DB, req chat.Request, onDelta func(string, bool) error) (*chat.Response, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	for _, d := range m.deltas {
		if onDelta != nil {
			if err := onDelta(d, false); err != nil {
				return nil, err
			}
		}
	}
	if m.postErr != nil {
		return nil, m.postErr
	}
	return &chat.Response{Content: strings.Join(m.deltas, "")}, nil
}

func TestCanvas_DeferredLLMStreaming_E2E(t *testing.T) {
	makeStreamingCanvas := func(llmParams map[string]any) *Canvas {
		return &Canvas{
			Components: map[string]CanvasComponent{
				"begin_0": {
					Obj:        CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
					Downstream: []string{"llm_0"},
				},
				"llm_0": {
					Obj: CanvasComponentObj{
						ComponentName: "LLM",
						Params:        llmParams,
					},
					Upstream:   []string{"begin_0"},
					Downstream: []string{"message_0"},
				},
				"message_0": {
					Obj: CanvasComponentObj{
						ComponentName: "Message",
						Params: map[string]any{
							"text": "{llm_0@content}",
						},
					},
					Upstream:   []string{"llm_0"},
					Downstream: []string{},
				},
			},
			Path: []string{"begin_0", "llm_0", "message_0"},
		}
	}

	t.Run("successful streaming delivers multiple deltas and updates node_finished outputs", func(t *testing.T) {
		inv := &mockCanvasStreamingInvoker{deltas: []string{"Hello", " ", "streaming", " ", "world"}}
		prev := component.GetDefaultChatInvokerForTest()
		component.SetDefaultChatInvoker(inv)
		t.Cleanup(func() { component.SetDefaultChatInvoker(prev) })

		c := makeStreamingCanvas(map[string]any{
			"model_id":    "test-model@openai",
			"user_prompt": "hello",
		})
		cc, err := Compile(t.Context(), c)
		if err != nil {
			t.Fatalf("Compile() failed: %v", err)
		}

		runState := NewCanvasState("run-e2e-ok", "task-e2e-ok")
		ctx := withState(t.Context(), runState)
		ctx = runtime.WithDeferredNodeRegistry(ctx)

		var emittedDeltas []string
		ctx = runtime.WithCanvasMessageEventEmitter(ctx, func(content string, startToThink, endToThink bool) {
			if content != "" {
				emittedDeltas = append(emittedDeltas, content)
			}
		})

		eventsChan := make(chan RunEvent, 100)
		ctx = WithRunMeta(ctx, &RunMeta{
			Events:    eventsChan,
			MessageID: "msg-1",
			SessionID: "sess-1",
		})

		_, err = cc.Workflow.Invoke(ctx, map[string]any{"query": "go"})
		if err != nil {
			t.Fatalf("Workflow.Invoke() unexpected err = %v", err)
		}

		if inv.calls != 1 {
			t.Errorf("LLM invoker calls = %d, want 1", inv.calls)
		}
		if len(emittedDeltas) != len(inv.deltas) {
			t.Errorf("emitted deltas count = %d, want %d (%v)", len(emittedDeltas), len(inv.deltas), emittedDeltas)
		}
		if strings.Join(emittedDeltas, "") != "Hello streaming world" {
			t.Errorf("emitted text = %q, want %q", strings.Join(emittedDeltas, ""), "Hello streaming world")
		}

		// Verify node_finished event for llm_0
		var llmFinished *NodeFinishedData
		close(eventsChan)
		for ev := range eventsChan {
			if ev.Type == "node_finished" {
				var nf NodeFinishedData
				if err := json.Unmarshal([]byte(ev.Data), &nf); err == nil && nf.ComponentID == "llm_0" {
					llmFinished = &nf
				}
			}
		}

		if llmFinished == nil {
			t.Fatal("no node_finished event found for llm_0")
		}
		if llmFinished.Error != nil {
			t.Errorf("node_finished Error = %v, want nil", llmFinished.Error)
		}
		outputs, _ := llmFinished.Outputs.(map[string]any)
		if content, ok := outputs["content"].(string); !ok || content != "Hello streaming world" {
			t.Errorf("node_finished Outputs[content] = %v, want %q", outputs["content"], "Hello streaming world")
		}
	})

	t.Run("mid-stream failure surfaces DeferredStreamError and records error in node_finished", func(t *testing.T) {
		streamErr := errors.New("abrupt stream termination")
		inv := &mockCanvasStreamingInvoker{
			deltas:  []string{"chunk 1", "chunk 2"},
			postErr: streamErr,
		}
		prev := component.GetDefaultChatInvokerForTest()
		component.SetDefaultChatInvoker(inv)
		t.Cleanup(func() { component.SetDefaultChatInvoker(prev) })

		c := makeStreamingCanvas(map[string]any{
			"model_id":    "test-model@openai",
			"user_prompt": "hello",
		})
		cc, err := Compile(t.Context(), c)
		if err != nil {
			t.Fatalf("Compile() failed: %v", err)
		}

		runState := NewCanvasState("run-e2e-err", "task-e2e-err")
		ctx := withState(t.Context(), runState)
		ctx = runtime.WithDeferredNodeRegistry(ctx)

		eventsChan := make(chan RunEvent, 100)
		ctx = WithRunMeta(ctx, &RunMeta{
			Events:    eventsChan,
			MessageID: "msg-err",
			SessionID: "sess-err",
		})

		_, err = cc.Workflow.Invoke(ctx, map[string]any{"query": "go"})
		if err == nil {
			t.Fatal("expected Workflow.Invoke to return error on mid-stream failure, got nil")
		}

		var deferredErr *runtime.DeferredStreamError
		if !errors.As(err, &deferredErr) && !strings.Contains(err.Error(), "abrupt stream termination") {
			t.Errorf("err = %v, want DeferredStreamError or stream termination text", err)
		}

		if inv.calls != 1 {
			t.Errorf("LLM invoker calls = %d, want 1", inv.calls)
		}

		// Verify node_finished event for llm_0 reports error
		var llmFinished *NodeFinishedData
		close(eventsChan)
		for ev := range eventsChan {
			if ev.Type == "node_finished" {
				var nf NodeFinishedData
				if err := json.Unmarshal([]byte(ev.Data), &nf); err == nil && nf.ComponentID == "llm_0" {
					llmFinished = &nf
				}
			}
		}

		if llmFinished == nil {
			t.Fatal("no node_finished event found for llm_0")
		}
		if llmFinished.Error == nil || !strings.Contains(llmFinished.Error.(string), "abrupt stream termination") {
			t.Errorf("node_finished Error = %v, want abrupt stream termination", llmFinished.Error)
		}
	})

	t.Run("LLM with json_output or output_structure gates to eager execution", func(t *testing.T) {
		cases := []struct {
			name   string
			params map[string]any
		}{
			{"json_output=true", map[string]any{"model_id": "test-model@openai", "json_output": true}},
			{"output_structure set", map[string]any{"model_id": "test-model@openai", "output_structure": map[string]any{"reply": "str"}}},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				c := makeStreamingCanvas(tc.params)
				opts := resolveComponentExecutionOptions(c, "llm_0", "LLM")
				if opts.DeferLLMToMessage {
					t.Errorf("expected DeferLLMToMessage=false for %s, got true", tc.name)
				}
			})
		}
	})
}
