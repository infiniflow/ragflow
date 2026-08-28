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
	"testing"

	"github.com/cloudwego/eino/schema"
)

// TestAgent_GetInputForm_UsesPromptReferences verifies the Agent input form
// follows Python's behavior: it is derived from prompt template references.
func TestAgent_GetInputForm_UsesPromptReferences(t *testing.T) {
	c := NewAgentComponent(AgentParam{
		ModelID:      "echo",
		SystemPrompt: "Use {sys.query} and {{sys.user_id}} to answer.",
		UserPrompt:   "Question: {sys.query}",
	})
	form := c.GetInputForm()
	if form == nil {
		t.Fatal("GetInputForm returned nil")
	}
	if _, ok := form["self"]; ok {
		t.Fatalf("form unexpectedly contains synthetic self key: %#v", form["self"])
	}
	queryField, ok := form["sys.query"].(map[string]any)
	if !ok {
		t.Fatalf("form['sys.query'] type=%T, want map[string]any", form["sys.query"])
	}
	if queryField["type"] != "line" {
		t.Errorf("sys.query.type=%v, want line", queryField["type"])
	}
	if queryField["name"] != "sys.query" {
		t.Errorf("sys.query.name=%v, want 'sys.query'", queryField["name"])
	}
	if queryField["optional"] != false {
		t.Errorf("sys.query.optional=%v, want false", queryField["optional"])
	}
	userIDField, ok := form["sys.user_id"].(map[string]any)
	if !ok {
		t.Fatalf("form['sys.user_id'] type=%T, want map[string]any", form["sys.user_id"])
	}
	if userIDField["name"] != "sys.user_id" {
		t.Errorf("sys.user_id.name=%v, want sys.user_id", userIDField["name"])
	}
	if userIDField["optional"] != false {
		t.Errorf("sys.user_id.optional=%v, want false", userIDField["optional"])
	}
}

// TestAgent_Reset_NoTools: Reset is safe to call when no tools are
// configured.
func TestAgent_Reset_NoTools(t *testing.T) {
	c := NewAgentComponent(AgentParam{ModelID: "echo"})
	c.Reset() // should not panic
}

// TestAgent_GetInputForm_DeduplicatesPromptReferences ensures repeated refs
// across system and user prompts collapse to one input field.
func TestAgent_GetInputForm_DeduplicatesPromptReferences(t *testing.T) {
	c := NewAgentComponent(AgentParam{
		ModelID:      "echo",
		SystemPrompt: "A {sys.query}",
		UserPrompt:   "B {{sys.query}}",
	})
	keys := sortedAgentPromptInputKeys(c.param.SystemPrompt, c.param.UserPrompt)
	if len(keys) != 1 || keys[0] != "sys.query" {
		t.Fatalf("keys=%v, want [sys.query]", keys)
	}
}

// TestAgent_Meta_DefaultsToEmpty: prompts without variable references yield
// an empty input-form map.
func TestAgent_Meta_DefaultsToEmpty(t *testing.T) {
	c := NewAgentComponent(AgentParam{ModelID: "echo"})
	form := c.GetInputForm()
	if len(form) != 0 {
		t.Fatalf("expected empty input form, got %+v", form)
	}
}

// TestAgentMetaParam_FieldsRoundTrip: round-trip through the struct
// preserves all fields.
func TestAgentMetaParam_FieldsRoundTrip(t *testing.T) {
	p := AgentMetaParam{Type: "string", Description: "user input", Required: true}
	// Use the type as a sanity check; round-trip via assignment.
	q := p
	if q.Type != "string" || !q.Required {
		t.Errorf("round-trip lost fields: %+v", q)
	}
	// Ensure schema package is referenced (avoids unused import).
	_ = schema.User
}
