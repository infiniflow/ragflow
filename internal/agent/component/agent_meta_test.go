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

// TestAgent_GetInputForm_IncludesSelfMeta: GetInputForm returns the
// Agent's own meta-schema under the "self" key, even when no tools
// are configured.
func TestAgent_GetInputForm_IncludesSelfMeta(t *testing.T) {
	c := NewAgentComponent(AgentParam{
		ModelID: "echo",
		Meta: AgentMeta{
			Name:        "research_agent",
			Description: "Performs multi-step research",
			Parameters: map[string]AgentMetaParam{
				"user_prompt": {Type: "string", Description: "The question", Required: true},
			},
		},
	})
	form := c.GetInputForm()
	if form == nil {
		t.Fatal("GetInputForm returned nil")
	}
	self, ok := form["self"].(AgentMeta)
	if !ok {
		t.Fatalf("form['self'] type=%T, want AgentMeta", form["self"])
	}
	if self.Name != "research_agent" {
		t.Errorf("Name=%q, want research_agent", self.Name)
	}
	if self.Parameters["user_prompt"].Type != "string" {
		t.Errorf("user_prompt type=%q, want string", self.Parameters["user_prompt"].Type)
	}
}

// TestAgent_Reset_NoTools: Reset is safe to call when no tools are
// configured.
func TestAgent_Reset_NoTools(t *testing.T) {
	c := NewAgentComponent(AgentParam{ModelID: "echo"})
	c.Reset() // should not panic
}

// TestAgent_Meta_DefaultsToEmpty: zero-value AgentParam.Meta is the
// empty AgentMeta struct (not a nil map dereference).
func TestAgent_Meta_DefaultsToEmpty(t *testing.T) {
	c := NewAgentComponent(AgentParam{ModelID: "echo"})
	form := c.GetInputForm()
	self, ok := form["self"].(AgentMeta)
	if !ok {
		t.Fatalf("form['self'] type=%T, want AgentMeta", form["self"])
	}
	if self.Name != "" || self.Description != "" {
		t.Errorf("expected empty meta, got %+v", self)
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
