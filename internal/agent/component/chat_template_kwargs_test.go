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
)

// TestMergeLLMParam_ChatTemplateKwargsFromInputs: inputs["chat_template_kwargs"]
// flows into LLMParam.ChatTemplateKwargs.
func TestMergeLLMParam_ChatTemplateKwargsFromInputs(t *testing.T) {
	base := LLMParam{ModelID: "echo"}
	inputs := map[string]any{
		"chat_template_kwargs": map[string]any{
			"seed":            42,
			"response_format": "json_object",
		},
	}
	p := mergeLLMParam(base, inputs)
	if p.ChatTemplateKwargs == nil {
		t.Fatal("ChatTemplateKwargs not parsed from inputs")
	}
	if p.ChatTemplateKwargs["seed"].(int) != 42 {
		t.Errorf("seed=%v, want 42", p.ChatTemplateKwargs["seed"])
	}
	if p.ChatTemplateKwargs["response_format"].(string) != "json_object" {
		t.Errorf("response_format=%v, want json_object", p.ChatTemplateKwargs["response_format"])
	}
}

// TestLLMFactory_ChatTemplateKwargsFromParams: the registered factory
// (init()) parses chat_template_kwargs from the params map.
func TestLLMFactory_ChatTemplateKwargsFromParams(t *testing.T) {
	c, err := New("LLM", map[string]any{
		"model_id": "echo",
		"chat_template_kwargs": map[string]any{
			"seed": 7,
		},
	})
	if err != nil {
		t.Fatalf("New(LLM): %v", err)
	}
	comp := c.(*LLMComponent)
	if comp.param.ChatTemplateKwargs == nil {
		t.Fatal("ChatTemplateKwargs not parsed by factory")
	}
	if comp.param.ChatTemplateKwargs["seed"].(int) != 7 {
		t.Errorf("seed=%v, want 7", comp.param.ChatTemplateKwargs["seed"])
	}
}

// TestLLM_Invoke_ChatTemplateKwargsDoesNotBreak: when ChatTemplateKwargs
// is set, Invoke still produces a normal call. Driver-level
// pass-through is currently a field exposure; the eino chat
// model driver does not accept generic kwargs yet.
func TestLLM_Invoke_ChatTemplateKwargsDoesNotBreak(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{
		ModelID:            "echo",
		ChatTemplateKwargs: map[string]any{"seed": 1},
	})
	_, err := c.Invoke(context.Background(), map[string]any{"user_prompt": "hi"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.calls != 1 {
		t.Errorf("expected 1 call, got %d", stub.calls)
	}
}
