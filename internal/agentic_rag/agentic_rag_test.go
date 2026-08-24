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

package agentic_rag

import (
	"context"
	"strings"
	"testing"
)

// TestRun_NilModel: Run must reject a nil model up front.
func TestRun_NilModel(t *testing.T) {
	_, err := Run(context.Background(), Input{Model: nil})
	if err == nil {
		t.Fatal("expected error for nil model")
	}
}

// TestDefaultTools: the default tool set contains the ported tools, the
// list_chunks deep-read tool, and the run_javascript sandbox.
func TestDefaultTools(t *testing.T) {
	tools := DefaultTools("", nil)
	if len(tools) != 6 {
		t.Fatalf("len=%d, want 6", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatalf("Info: %v", err)
		}
		names[info.Name] = true
	}
	for _, want := range []string{"think", "todo_write", "grep_chunks", "search_chunks"} {
		if !names[want] {
			t.Errorf("missing tool %q; got %v", want, names)
		}
	}
}

// TestPrompt: the prompt must declare the six tools and contain no removed
// tool references (get_document_info / web_search / query_knowledge_graph) and
// no leftover placeholders.
func TestPrompt(t *testing.T) {
	p := Prompt()
	for _, want := range []string{
		"grep_chunks", "search_chunks", "list_chunks",
		"todo_write", "think", "run_javascript",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt must mention %q", want)
		}
	}
	for _, banned := range []string{
		"get_document_info", "query_knowledge_graph", "web_search", "web_fetch",
		"{{", "}}",
	} {
		if strings.Contains(p, banned) {
			t.Errorf("prompt must not contain %q", banned)
		}
	}
}
