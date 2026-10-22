// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// Licensed under the Apache License, Version 2.0.

package component

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

func llmStructuredSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{"name": map[string]any{"type": "string"}},
		"required":   []any{"name"}, "additionalProperties": false,
	}
}

type structuredLLMInvoker struct {
	responses []string
	requests  []ChatInvokeRequest
	err       error
	onCall    func()
}

func (s *structuredLLMInvoker) Invoke(_ context.Context, _ *gorm.DB, req ChatInvokeRequest) (*ChatInvokeResponse, error) {
	s.requests = append(s.requests, req)
	if s.onCall != nil {
		s.onCall()
	}
	if s.err != nil {
		return nil, s.err
	}
	i := len(s.requests) - 1
	if i >= len(s.responses) {
		return nil, errors.New("unexpected model call")
	}
	return &ChatInvokeResponse{Content: s.responses[i], Model: "echo", Tokens: i + 1, Stopped: true}, nil
}

func TestLLMStructuredOutput(t *testing.T) {
	for _, tc := range []struct {
		name      string
		responses []string
		retries   int
		want      string
		wantError bool
	}{
		{"valid", []string{`{"name":"Alice"}`}, 0, `{"name":"Alice"}`, false},
		{"markers inside JSON", []string{"{\"name\":\"</think>```json\"}"}, 0, "{\"name\":\"</think>```json\"}", false},
		{"fenced", []string{"<think>private</think>```json\n{\"name\":\"Alice\"}\n```"}, 0, `{"name":"Alice"}`, false},
		{"wrong type repaired", []string{`{"name":123}`, `{"name":"Bob"}`}, 1, `{"name":"Bob"}`, false},
		{"missing required", []string{`{}`}, 0, "", true},
		{"extra field", []string{`{"name":"Alice","secret":true}`}, 0, "", true},
		{"budget exhausted", []string{"not JSON", `{"name":12}`}, 1, "", true},
		{"two repairs", []string{"not JSON", `{}`, `{"name":"Bob"}`}, 2, `{"name":"Bob"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv := &structuredLLMInvoker{responses: tc.responses}
			withStubInvoker(t, inv)
			c := NewLLMComponent(LLMParam{ModelID: "echo", SystemPrompt: "keep instruction", UserPrompt: "who?", OutputStructure: llmStructuredSchema(), MaxRetries: tc.retries})
			out, err := c.Invoke(t.Context(), nil, nil)
			if tc.wantError {
				if err == nil || out != nil {
					t.Fatalf("invalid output published: out=%v err=%v", out, err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if strings.TrimSpace(out["content"].(string)) != tc.want {
					t.Fatalf("content=%v", out["content"])
				}
				if _, ok := out["structured"].(map[string]any); !ok {
					t.Fatalf("structured=%v", out["structured"])
				}
				if out["tokens"] != len(tc.responses) {
					t.Fatalf("stale response metadata: %v", out)
				}
			}
			if len(inv.requests) != len(tc.responses) {
				t.Fatalf("calls=%d want=%d", len(inv.requests), len(tc.responses))
			}
			for _, req := range inv.requests {
				if len(req.Tools) != 0 {
					t.Fatal("format correction bound tools")
				}
				if len(req.Messages) == 0 || !strings.Contains(req.Messages[0].Content, "keep instruction") || !strings.Contains(req.Messages[0].Content, "additionalProperties") {
					t.Fatalf("prompt lost: %+v", req.Messages)
				}
			}
			if len(inv.requests) > 1 {
				msgs := inv.requests[1].Messages
				if msgs[len(msgs)-2].Role != schema.Assistant || msgs[len(msgs)-2].Content != tc.responses[0] {
					t.Fatalf("failed response absent: %+v", msgs)
				}
			}
		})
	}
}

func TestLLMStructuredOutputConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name    string
		config  any
		enabled bool
		fail    bool
	}{
		{"schema", llmStructuredSchema(), true, false},
		{"empty", map[string]any{}, false, false},
		{"runtime only", map[string]any{"value": map[string]any{"old": true}}, false, false},
		{"array root", map[string]any{"type": "array"}, false, true},
		{"boolean schema", false, false, true},
		{"invalid schema", map[string]any{"properties": map[string]any{"name": map[string]any{"type": "wrong"}}}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv := &structuredLLMInvoker{responses: []string{`{"name":"Alice"}`}}
			withStubInvoker(t, inv)
			c, err := New("LLM", map[string]any{"llm_id": "echo", "user_prompt": "who?", "outputs": map[string]any{"structured": tc.config}, "max_retries": 0})
			if err != nil {
				t.Fatal(err)
			}
			out, err := c.Invoke(t.Context(), nil, nil)
			if tc.fail {
				if err == nil || len(inv.requests) != 0 || out != nil {
					t.Fatalf("invalid schema executed: %v %v calls=%d", out, err, len(inv.requests))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			_, ok := out["structured"]
			if ok != tc.enabled {
				t.Fatalf("structured enabled=%v want=%v", ok, tc.enabled)
			}
		})
	}
}

func TestLLMStructuredOutputCancellationAndProviderErrors(t *testing.T) {
	providerErr := errors.New("provider unavailable")
	for _, kind := range []string{"before", "after", "provider"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			inv := &structuredLLMInvoker{responses: []string{`{"name":"Alice"}`}}
			if kind == "before" {
				cancel()
			}
			if kind == "after" {
				inv.onCall = cancel
			}
			if kind == "provider" {
				inv.err = providerErr
			}
			withStubInvoker(t, inv)
			c := NewLLMComponent(LLMParam{ModelID: "echo", UserPrompt: "who?", OutputStructure: llmStructuredSchema()})
			out, err := c.Invoke(ctx, nil, nil)
			wantErr := context.Canceled
			if kind == "provider" {
				wantErr = providerErr
			}
			if !errors.Is(err, wantErr) || out != nil {
				t.Fatalf("out=%v err=%v", out, err)
			}
			wantCalls := 1
			if kind == "before" {
				wantCalls = 0
			}
			if len(inv.requests) != wantCalls {
				t.Fatalf("calls=%d", len(inv.requests))
			}
		})
	}
}

func TestLLMStructuredOutputStreamDoesNotPublishInvalidContent(t *testing.T) {
	inv := &structuredLLMInvoker{responses: []string{`{"name":123}`}}
	withStubInvoker(t, inv)
	c := NewLLMComponent(LLMParam{ModelID: "echo", UserPrompt: "who?", OutputStructure: llmStructuredSchema()})
	ch, err := c.Stream(t.Context(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	seenError := false
	for chunk := range ch {
		if _, ok := chunk["content"]; ok {
			t.Fatalf("invalid content emitted: %v", chunk)
		}
		if _, ok := chunk["done"]; ok {
			t.Fatalf("failed stream completed: %v", chunk)
		}
		if _, ok := chunk["error"]; ok {
			seenError = true
		}
	}
	if !seenError {
		t.Fatal("missing stream failure")
	}
}
