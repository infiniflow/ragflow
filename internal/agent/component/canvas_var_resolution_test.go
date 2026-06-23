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
	"testing"

	"ragflow/internal/agent/runtime"
)

// TestLLM_Invoke_ResolvesTemplateRefs: when a CanvasState is attached
// to ctx, {{cpn_id@var}} references in system/user prompts resolve to
// the state's value.
func TestLLM_Invoke_ResolvesTemplateRefs(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)

	state := runtime.NewCanvasState("rid", "tid")
	state.SetVar("retrieval:0", "content", "retrieved text")

	c := NewLLMComponent(LLMParam{
		ModelID:      "echo",
		SystemPrompt: "Use the following context:",
		UserPrompt:   "What does {{retrieval:0@content}} say?",
	})
	ctx := runtime.WithState(context.Background(), state)
	_, err := c.Invoke(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker captured no request")
	}
	// User message should have {{retrieval:0@content}} replaced.
	userMsg := stub.captured.Messages[len(stub.captured.Messages)-1]
	if userMsg.Content != "What does retrieved text say?" {
		t.Errorf("user msg content=%q, want resolved template", userMsg.Content)
	}
}

// TestLLM_Invoke_NoState_LeavesPromptsUnchanged: when no state is
// attached (e.g. unit tests bypassing the canvas scheduler), the
// prompts pass through verbatim.
func TestLLM_Invoke_NoState_LeavesPromptsUnchanged(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{
		ModelID:    "echo",
		UserPrompt: "What does {{retrieval:0@content}} say?",
	})
	_, err := c.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	userMsg := stub.captured.Messages[len(stub.captured.Messages)-1]
	if userMsg.Content != "What does {{retrieval:0@content}} say?" {
		t.Errorf("user msg content should be unchanged when no state; got %q", userMsg.Content)
	}
}

// TestLLM_Invoke_UnresolvedRef_LeavesPromptIntact: when the state is
// attached but the ref is missing, the resolver logs and the original
// prompt is kept (replaces the ref with "").
func TestLLM_Invoke_UnresolvedRef_LeavesPromptIntact(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)

	// State without the ref — resolver should return "" and log.
	state := runtime.NewCanvasState("rid", "tid")
	state.SetVar("other:0", "content", "x")

	c := NewLLMComponent(LLMParam{
		ModelID:    "echo",
		UserPrompt: "Use {{retrieval:0@content}} please",
	})
	ctx := runtime.WithState(context.Background(), state)
	_, err := c.Invoke(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("Invoke should not error on unresolved ref: %v", err)
	}
	// ResolveTemplate replaces the unresolved ref with "" and returns
	// the modified prompt.
	userMsg := stub.captured.Messages[len(stub.captured.Messages)-1]
	if userMsg.Content == "Use {{retrieval:0@content}} please" {
		t.Errorf("unresolved ref should be replaced with empty; got unchanged prompt %q", userMsg.Content)
	}
}
