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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const twoTemplateYAML = `templates:
  - id: smart-reasoning
    name: Progressive Agentic RAG
    description: full retrieval
    tools:
      - think
      - grep_chunks
      - search_chunks
      - list_chunks
    content: |
      FULL-MODE PROMPT
  - id: smart-grep
    name: Grep-Only Agentic RAG
    description: grep retrieval only
    tools:
      - think
      - grep_chunks
      - list_chunks
    content: |
      GREP-ONLY PROMPT
`

func setupTestConfig(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agentic_rag.yaml")
	if err := os.WriteFile(path, []byte(twoTemplateYAML), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	t.Setenv("AGENTIC_RAG_CONFIG", path)
	// Force a reload of the process-wide cache so each test sees its own file.
	configMu.Lock()
	cachedFile = nil
	configMu.Unlock()
	t.Cleanup(func() {
		configMu.Lock()
		cachedFile = nil
		configMu.Unlock()
	})
}

func TestResolveTemplateForExplicitIDPicksRequestedTemplate(t *testing.T) {
	setupTestConfig(t)

	tmpl, err := resolveTemplateFor("smart-grep")
	if err != nil {
		t.Fatalf("resolve smart-grep: %v", err)
	}
	if tmpl.ID != "smart-grep" {
		t.Fatalf("template id = %q, want smart-grep", tmpl.ID)
	}
	for _, tool := range tmpl.Tools {
		if tool == "search_chunks" {
			t.Fatalf("grep-only template must not include search_chunks: %v", tmpl.Tools)
		}
	}
	if tmpl.Content != "GREP-ONLY PROMPT\n" {
		t.Fatalf("content = %q, want GREP-ONLY PROMPT", tmpl.Content)
	}
}

func TestResolveTemplateForUnknownIDIsHardError(t *testing.T) {
	setupTestConfig(t)

	if _, err := resolveTemplateFor("no-such-template"); err == nil ||
		!strings.Contains(err.Error(), "not found") {
		t.Fatalf("unknown id must fail loudly, got err=%v", err)
	}
}

func TestResolveTemplateForEmptyIDIsHardError(t *testing.T) {
	setupTestConfig(t)

	if _, err := resolveTemplateFor(""); err == nil ||
		!strings.Contains(err.Error(), "empty agent_mode") {
		t.Fatalf("empty id must fail loudly, got err=%v", err)
	}
}

// TestTemplateTemperatureComesFromConfig pins the sampling knob to the
// template: a declared temperature wins, an undeclared one means the model's
// own default (nil - the field is never set).
func TestTemplateTemperatureComesFromConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentic_rag.yaml")
	yaml := `templates:
  - id: pinned
    name: Pinned
    description: declares its sampling
    temperature: 0.4
    tools:
      - list_chunks
    content: |
      PINNED PROMPT
  - id: unpinned
    name: Unpinned
    description: model default
    tools:
      - list_chunks
    content: |
      UNPINNED PROMPT
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	t.Setenv("AGENTIC_RAG_CONFIG", path)
	configMu.Lock()
	cachedFile = nil
	configMu.Unlock()
	t.Cleanup(func() {
		configMu.Lock()
		cachedFile = nil
		configMu.Unlock()
	})

	pinned, err := resolveTemplateFor("pinned")
	if err != nil {
		t.Fatalf("resolve pinned: %v", err)
	}
	if pinned.Temperature == nil || *pinned.Temperature != 0.4 {
		t.Fatalf("pinned temperature = %v, want 0.4", pinned.Temperature)
	}
	if got := TemplateTemperature("pinned"); got == nil || *got != 0.4 {
		t.Fatalf("TemplateTemperature(pinned) = %v, want 0.4", got)
	}

	unpinned, err := resolveTemplateFor("unpinned")
	if err != nil {
		t.Fatalf("resolve unpinned: %v", err)
	}
	if unpinned.Temperature != nil {
		t.Fatalf("unpinned temperature = %v, want nil (the model's own default)", unpinned.Temperature)
	}
	if got := TemplateTemperature("unpinned"); got != nil {
		t.Fatalf("TemplateTemperature(unpinned) = %v, want nil", got)
	}
}

// TestShippedConfigIsASingleDirectAnswerTemplate guards conf/agentic_rag.yaml
// end to end: exactly one template (smart-reasoning), the toolset the code and
// the prompt agree on, and a DIRECT-ANSWER prompt - no question-decomposition
// stage, no intermediate deliverable format, no auditor, and the mechanical
// provenance contract (`chunk_id: <id>`) the chat pipeline turns into
// citation markers.
func TestShippedConfigIsASingleDirectAnswerTemplate(t *testing.T) {
	t.Setenv("AGENTIC_RAG_CONFIG", filepath.Join("..", "..", "conf", "agentic_rag.yaml"))
	configMu.Lock()
	cachedFile = nil
	configMu.Unlock()
	t.Cleanup(func() {
		configMu.Lock()
		cachedFile = nil
		configMu.Unlock()
	})

	cfg := GetConfigFile()
	if cfg == nil {
		t.Fatal("shipped config failed to load")
	}
	if len(cfg.Templates) != 1 {
		t.Fatalf("shipped config carries %d templates, want exactly 1 (smart-reasoning)", len(cfg.Templates))
	}
	tmpl := cfg.Templates[0]
	if tmpl.ID != "smart-reasoning" {
		t.Fatalf("template id = %q, want smart-reasoning", tmpl.ID)
	}
	wantTools := []string{
		"think", "todo_write", "grep_chunks", "search_bm25_chunks",
		"search_semantic_chunks", "list_chunks", "run_javascript",
	}
	if strings.Join(tmpl.Tools, ",") != strings.Join(wantTools, ",") {
		t.Fatalf("tools = %v, want %v", tmpl.Tools, wantTools)
	}
	if tmpl.Temperature == nil || *tmpl.Temperature != 0.5 {
		t.Fatalf("temperature = %v, want 0.5", tmpl.Temperature)
	}

	content := tmpl.Content
	for _, want := range []string{
		"grep_chunks", "search_bm25_chunks", "search_semantic_chunks",
		"list_chunks", "chunk_id:", "Evidence-First",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("prompt must mention %q", want)
		}
	}
	for _, banned := range []string{
		"Candidate Matrix", "Reasoning Chain", "Sub-question",
		"question decomposition", "auditor", "audit_max_pass",
		"Final Answer", "Guessed Answer",
	} {
		if strings.Contains(content, banned) {
			t.Errorf("prompt must not contain %q (the direct-answer rewrite removed the intermediate deliverable)", banned)
		}
	}
}
