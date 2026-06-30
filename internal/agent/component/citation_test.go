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

	"ragflow/internal/agent/component/prompts"
)

// TestCitationPrompt_NotEmpty: the prompt must be non-empty and contain
// the format instruction.
func TestCitationPrompt_NotEmpty(t *testing.T) {
	p := prompts.CitationPrompt()
	if p == "" {
		t.Fatal("CitationPrompt returned empty string")
	}
	if !strings.Contains(p, "[ID:") {
		t.Errorf("CitationPrompt does not contain [ID: format spec; got: %s", p[:100])
	}
	if !strings.Contains(p, "Maximum 4 citations") {
		t.Errorf("CitationPrompt missing 4-citation rule; got: %s", p[:200])
	}
}

// TestInjectCitationPrompt_EmptySystem: when system is empty, the
// prompt is returned as-is.
func TestInjectCitationPrompt_EmptySystem(t *testing.T) {
	got := injectCitationPrompt("")
	if got != prompts.CitationPrompt() {
		t.Errorf("expected prompt as-is when system empty")
	}
}

// TestInjectCitationPrompt_NonEmptySystem: prompt is appended with two
// newlines separating it from the user's system message.
func TestInjectCitationPrompt_NonEmptySystem(t *testing.T) {
	got := injectCitationPrompt("You are a helpful assistant.")
	if !strings.HasPrefix(got, "You are a helpful assistant.\n\n") {
		t.Errorf("expected user system first, then prompt separated by \\n\\n; got: %s", got[:80])
	}
	if !strings.Contains(got, "[ID:") {
		t.Errorf("citation prompt not appended")
	}
}

// TestBuildMessagesWithImages_CiteTrue: when cite=true, the system
// message includes the citation-instruction text.
func TestBuildMessagesWithImages_CiteTrue(t *testing.T) {
	msgs := buildMessagesWithImages("sys", "user", nil, true)
	if len(msgs) < 1 {
		t.Fatalf("expected at least 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Fatalf("expected system message first, got %v", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content, "[ID:") {
		t.Errorf("citation prompt not injected; system content: %s", msgs[0].Content[:200])
	}
}

// TestBuildMessagesWithImages_CiteFalse: when cite=false, the system
// message is the user's verbatim input.
func TestBuildMessagesWithImages_CiteFalse(t *testing.T) {
	msgs := buildMessagesWithImages("sys", "user", nil, false)
	if !strings.Contains(msgs[0].Content, "sys") || strings.Contains(msgs[0].Content, "[ID:") {
		t.Errorf("citation prompt should NOT be injected; got: %s", msgs[0].Content[:200])
	}
}

// TestBuildMessagesWithImages_CiteEmptySystem: when system is empty
// and cite=true, the citation prompt becomes the system message.
func TestBuildMessagesWithImages_CiteEmptySystem(t *testing.T) {
	msgs := buildMessagesWithImages("", "user", nil, true)
	if len(msgs) < 1 {
		t.Fatalf("expected at least 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "[ID:") {
		t.Errorf("citation prompt should be sole system content; got: %s", msgs[0].Content[:200])
	}
}

// TestLLMFactory_DefaultCiteIsTrue: the registered LLM factory
// (registered via init()) defaults Cite=true to match Python.
func TestLLMFactory_DefaultCiteIsTrue(t *testing.T) {
	c, err := New("LLM", map[string]any{"model_id": "echo"})
	if err != nil {
		t.Fatalf("New(LLM): %v", err)
	}
	comp := c.(*LLMComponent)
	if !comp.param.Cite {
		t.Errorf("factory default Cite=false; want true (matches Python)")
	}
}

// TestLLMFactory_ParsesCiteFalse: explicit cite=false propagates.
func TestLLMFactory_ParsesCiteFalse(t *testing.T) {
	c, err := New("LLM", map[string]any{
		"model_id": "echo",
		"cite":     false,
	})
	if err != nil {
		t.Fatalf("New(LLM): %v", err)
	}
	comp := c.(*LLMComponent)
	if comp.param.Cite {
		t.Errorf("Cite=true after inputs[cite]=false; want false")
	}
}

// TestLLM_Invoke_AppendsCitationPrompt: end-to-end — when Cite=true
// (factory default), the system message received by the invoker
// includes the citation instructions.
func TestLLM_Invoke_AppendsCitationPrompt(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{ModelID: "echo", SystemPrompt: "You are a bot.", Cite: true})
	if _, err := c.Invoke(context.Background(), map[string]any{
		"user_prompt": "hi",
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker captured no request")
	}
	if len(stub.captured.Messages) == 0 {
		t.Fatal("no messages captured")
	}
	sys := stub.captured.Messages[0]
	if !strings.Contains(sys.Content, "[ID:") {
		t.Errorf("system msg missing citation prompt; got: %s", sys.Content[:200])
	}
	if !strings.Contains(sys.Content, "You are a bot.") {
		t.Errorf("user system prompt not preserved at start; got: %s", sys.Content[:80])
	}
}

// TestLLM_Invoke_CiteFalseDisablesInjection: explicit inputs[cite]=false
// suppresses the citation injection even when factory default is true.
func TestLLM_Invoke_CiteFalseDisablesInjection(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{ModelID: "echo", SystemPrompt: "sys"})
	_, _ = c.Invoke(context.Background(), map[string]any{
		"user_prompt": "hi",
		"cite":        false,
	})
	if stub.captured == nil {
		t.Fatal("invoker captured no request")
	}
	if strings.Contains(stub.captured.Messages[0].Content, "[ID:") {
		t.Errorf("citation prompt should NOT be injected when cite=false")
	}
}
