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
	"strings"
	"testing"

	"ragflow/internal/agent/component/prompts"
)

// TestCitationPlusPrompt_EmptySources: no sources → render with empty
// sources block; IDs slice is empty.
func TestCitationPlusPrompt_EmptySources(t *testing.T) {
	rendered, ids := prompts.CitationPlusPrompt(nil)
	if len(ids) != 0 {
		t.Errorf("expected empty ids, got %v", ids)
	}
	if !strings.Contains(rendered, "<context>") {
		t.Error("expected <context> block in rendered prompt")
	}
	if !strings.Contains(rendered, "ID:") {
		t.Error("expected 'ID:' prefix in sources block (empty section)")
	}
}

// TestCitationPlusPrompt_WithSources: sources are rendered as
// `ID: <id>\n└── Content: <content>` blocks, ids are returned.
func TestCitationPlusPrompt_WithSources(t *testing.T) {
	sources := []prompts.CitationSource{
		{ID: "45", Content: "Smartphone market grew 7.8% in Q3 2024."},
		{ID: "46", Content: "5G adoption reached 1.5B users."},
	}
	rendered, ids := prompts.CitationPlusPrompt(sources)
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}
	if ids[0] != "45" || ids[1] != "46" {
		t.Errorf("ids=%v, want [45 46]", ids)
	}
	if !strings.Contains(rendered, "ID: 45") {
		t.Error("expected ID: 45 in rendered prompt")
	}
	if !strings.Contains(rendered, "Smartphone market grew 7.8%") {
		t.Error("expected source content in rendered prompt")
	}
}

// TestCitationPlusPrompt_SkipsEmptyFields: sources with empty ID or
// content are filtered out.
func TestCitationPlusPrompt_SkipsEmptyFields(t *testing.T) {
	sources := []prompts.CitationSource{
		{ID: "", Content: "no id"},
		{ID: "1", Content: ""},
		{ID: "2", Content: "valid"},
	}
	_, ids := prompts.CitationPlusPrompt(sources)
	if len(ids) != 1 || ids[0] != "2" {
		t.Errorf("expected only valid source; got ids=%v", ids)
	}
}
