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

// TestPrompt: the fallback prompt is a minimal, tool-agnostic safety net used
// only when a template declares no content of its own; the shipped toolset and
// its per-tool guidance live in conf/agentic_rag.yaml (guarded by the config
// test below). This asserts it is non-empty, keeps the evidence-first contract,
// and carries no removed references, placeholders or intermediate deliverable.
func TestPrompt(t *testing.T) {
	p := Prompt()
	if strings.TrimSpace(p) == "" {
		t.Fatal("fallback prompt must not be empty")
	}
	for _, want := range []string{"RAGFlow", "evidence-first"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt must mention %q", want)
		}
	}
	for _, banned := range []string{
		"get_document_info", "query_knowledge_graph", "web_fetch",
		"Candidate Matrix", "Reasoning Chain", "Sub-question",
		"question decomposition", "auditor", "Final Answer", "Guessed Answer",
		"{{", "}}",
	} {
		if strings.Contains(p, banned) {
			t.Errorf("prompt must not contain %q", banned)
		}
	}
}
