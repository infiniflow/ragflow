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
	"strings"
	"testing"
)

// TestMatchOutputStructure_ValidJSON_AllKeysPresent: response has all
// expected keys → ok.
func TestMatchOutputStructure_ValidJSON_AllKeysPresent(t *testing.T) {
	resp := `{"name":"Alice","age":30}`
	parsed, ok := matchOutputStructure(resp, map[string]any{"name": "", "age": 0})
	if !ok {
		t.Fatalf("expected match")
	}
	if parsed["name"] != "Alice" {
		t.Errorf("name=%v, want Alice", parsed["name"])
	}
	if parsed["age"].(float64) != 30 {
		t.Errorf("age=%v, want 30", parsed["age"])
	}
}

// TestMatchOutputStructure_ValidJSON_MissingKey: parse OK but a key
// is missing → not ok.
func TestMatchOutputStructure_ValidJSON_MissingKey(t *testing.T) {
	resp := `{"name":"Alice"}`
	_, ok := matchOutputStructure(resp, map[string]any{"name": "", "age": 0})
	if ok {
		t.Fatalf("expected mismatch (age key missing)")
	}
}

// TestMatchOutputStructure_NotJSON: invalid JSON → not ok.
func TestMatchOutputStructure_NotJSON(t *testing.T) {
	resp := "this is not json"
	_, ok := matchOutputStructure(resp, map[string]any{"x": 0})
	if ok {
		t.Fatalf("expected mismatch (not valid JSON)")
	}
}

// TestMatchOutputStructure_EmptyExpected: no expected keys → any
// JSON object passes (vacuous truth).
func TestMatchOutputStructure_EmptyExpected(t *testing.T) {
	resp := `{"anything":1}`
	_, ok := matchOutputStructure(resp, map[string]any{})
	if !ok {
		t.Fatalf("expected match with empty expected set")
	}
}

// TestBuildStructuredRetryMessages_AppendsRetryTurn: the returned
// message list's last message is the retry user turn, and the system
// message still reflects the citation prompt (when cite=true).
func TestBuildStructuredRetryMessages_AppendsRetryTurn(t *testing.T) {
	msgs := buildStructuredRetryMessages("sys", "user", nil, true,
		map[string]any{"name": "", "age": 0},
		"first response was not JSON")
	if len(msgs) < 1 {
		t.Fatalf("expected at least 1 message, got %d", len(msgs))
	}
	last := msgs[len(msgs)-1]
	if last.Role != "user" {
		t.Fatalf("expected last role=user, got %v", last.Role)
	}
	if !strings.Contains(last.Content, "name") || !strings.Contains(last.Content, "age") {
		t.Errorf("retry prompt missing expected keys; got: %s", last.Content)
	}
	if !strings.Contains(last.Content, "first response was not JSON") {
		t.Errorf("retry prompt should reference the previous response; got: %s", last.Content[:200])
	}
	if msgs[0].Role != "system" || !strings.Contains(msgs[0].Content, "[ID:") {
		t.Errorf("system message lost; got role=%v content[:80]=%q", msgs[0].Role, msgs[0].Content[:80])
	}
}

// TestLLM_Invoke_OutputStructure_ValidFirstTry: stub returns valid
// JSON on the first call → no retry; outputs["structured"] populated.
func TestLLM_Invoke_OutputStructure_ValidFirstTry(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{
		Content: `{"name":"Alice","age":30}`,
		Model:   "echo",
	}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{
		ModelID:         "echo",
		OutputStructure: map[string]any{"name": "", "age": 0},
	})
	out, err := c.Invoke(context.Background(), map[string]any{"user_prompt": "who?"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.calls != 1 {
		t.Errorf("expected 1 call (no retry), got %d", stub.calls)
	}
	structured, ok := out["structured"].(map[string]any)
	if !ok {
		t.Fatalf("structured not populated; out=%+v", out)
	}
	if structured["name"] != "Alice" {
		t.Errorf("name=%v, want Alice", structured["name"])
	}
}

// TestLLM_Invoke_OutputStructure_RetryOnInvalid: stub returns
// non-JSON first, then valid JSON → retry happens; outputs["structured"]
// populated from second call.
func TestLLM_Invoke_OutputStructure_RetryOnInvalid(t *testing.T) {
	calls := 0
	inv := &callCountingInvoker{
		responses: []*ChatInvokeResponse{
			{Content: "not json at all", Model: "echo"},
			{Content: `{"name":"Bob"}`, Model: "echo"},
		},
		onCall: func() { calls++ },
	}
	withStubInvoker(t, inv)

	c := NewLLMComponent(LLMParam{
		ModelID:         "echo",
		OutputStructure: map[string]any{"name": ""},
	})
	out, err := c.Invoke(context.Background(), map[string]any{"user_prompt": "who?"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls (first + retry), got %d", calls)
	}
	structured, ok := out["structured"].(map[string]any)
	if !ok {
		t.Fatalf("structured not populated after retry; out=%+v", out)
	}
	if structured["name"] != "Bob" {
		t.Errorf("name=%v, want Bob", structured["name"])
	}
	if out["content"] != `{"name":"Bob"}` {
		t.Errorf("content not updated to validated response; got %q", out["content"])
	}
}

// TestLLM_Invoke_OutputStructure_RetryStillFails: stub returns
// non-JSON both times → no structured output, content kept as first
// response (caller can still see it), no error.
func TestLLM_Invoke_OutputStructure_RetryStillFails(t *testing.T) {
	calls := 0
	inv := &callCountingInvoker{
		responses: []*ChatInvokeResponse{
			{Content: "not json", Model: "echo"},
			{Content: "still not json", Model: "echo"},
		},
		onCall: func() { calls++ },
	}
	withStubInvoker(t, inv)

	c := NewLLMComponent(LLMParam{
		ModelID:         "echo",
		OutputStructure: map[string]any{"x": 0},
	})
	out, err := c.Invoke(context.Background(), map[string]any{"user_prompt": "go"})
	if err != nil {
		t.Fatalf("Invoke should not error on parse failure: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls (first + 1 retry), got %d", calls)
	}
	if _, hasStructured := out["structured"]; hasStructured {
		t.Errorf("structured should be absent after failed retry; out=%+v", out)
	}
	// content stays as the original (failed) first response
	if out["content"] != "not json" {
		t.Errorf("content should remain the first response; got %q", out["content"])
	}
}

// callCountingInvoker is a test-only ChatInvoker that returns
// pre-programmed responses in sequence and counts invocations.
type callCountingInvoker struct {
	responses []*ChatInvokeResponse
	onCall    func()
	calls     int
}

func (c *callCountingInvoker) Invoke(_ context.Context, _ ChatInvokeRequest) (*ChatInvokeResponse, error) {
	if c.onCall != nil {
		c.onCall()
	}
	c.calls++
	if c.calls-1 < len(c.responses) {
		return c.responses[c.calls-1], nil
	}
	return &ChatInvokeResponse{Content: "exhausted"}, nil
}

func (c *callCountingInvoker) Stream(_ context.Context, _ ChatInvokeRequest) (<-chan map[string]any, error) {
	return nil, nil
}

func (c *callCountingInvoker) Inputs() map[string]string  { return nil }
func (c *callCountingInvoker) Outputs() map[string]string { return nil }
