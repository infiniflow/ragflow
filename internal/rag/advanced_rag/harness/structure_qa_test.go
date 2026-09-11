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

package harness

import (
	"context"
	"strings"
	"testing"
)

// entity map helper (mirrors Python structure_qa.py rendering inputs).
func saEntity(name, typ, desc string) map[string]any {
	return map[string]any{"name": name, "type": typ, "description": desc}
}

func saRelation(from, to, typ string) map[string]any {
	return map[string]any{"from": from, "to": to, "type": typ}
}

// RenderStructure feeds the LLM prompt in graph_explore (via AskStructure); it
// must render exactly "Name (Type): Description" for entities (Type defaulting
// to "other") and "From -[Type]-> To" for relations (Type defaulting to
// "related"), mirroring Python _render_structure.
func TestRenderStructure(t *testing.T) {
	entities := []map[string]any{
		saEntity("OmiyaSoft", "company", "game   dev"),
		saEntity("NoType", "", "   "),
		{"name": "  "}, // blank name dropped
	}
	relations := []map[string]any{
		saRelation("OmiyaSoft", "Culdcept", "founded"),
		saRelation("A", "B", ""), // missing type -> related
	}
	out := RenderStructure(entities, relations)

	if !strings.Contains(out, "Entities:") || !strings.Contains(out, "Relations:") {
		t.Fatalf("missing section headers: %q", out)
	}
	// Whitespace in descriptions is collapsed.
	if !strings.Contains(out, "- OmiyaSoft (company): game dev") {
		t.Errorf("entity with description not rendered: %q", out)
	}
	// No type -> "other"; blank description -> no trailing colon.
	if !strings.Contains(out, "- NoType (other)") {
		t.Errorf("entity default type missing: %q", out)
	}
	if strings.Contains(out, "- NoType (other):") {
		t.Errorf("blank description must not render a colon: %q", out)
	}
	if strings.Contains(out, "-  (") {
		t.Errorf("blank-name entity must be dropped: %q", out)
	}
	if !strings.Contains(out, "- OmiyaSoft -[founded]-> Culdcept") {
		t.Errorf("relation not rendered: %q", out)
	}
	// Missing relation type -> "related".
	if !strings.Contains(out, "- A -[related]-> B") {
		t.Errorf("relation default type missing: %q", out)
	}
}

func TestRenderStructureCapsAtLimits(t *testing.T) {
	entities := make([]map[string]any, maxStructureEntities+5)
	for i := range entities {
		entities[i] = saEntity("E", "t", "")
	}
	relations := make([]map[string]any, maxStructureRelations+5)
	for i := range relations {
		relations[i] = saRelation("A", "B", "r")
	}
	out := RenderStructure(entities, relations)
	// Each entity bullet is "\n- E (t)"; caps at maxStructureEntities.
	if n := strings.Count(out, "\n- E (t)"); n != maxStructureEntities {
		t.Errorf("entity bullets = %d, want cap %d", n, maxStructureEntities)
	}
	if n := strings.Count(out, "\n- A -[r]-> B"); n != maxStructureRelations {
		t.Errorf("relation bullets = %d, want cap %d", n, maxStructureRelations)
	}
}

// Python _render_structure joins an empty list, so both-empty input renders "".
func TestRenderStructureEmpty(t *testing.T) {
	if out := RenderStructure(nil, nil); out != "" {
		t.Errorf("empty structure must render empty string, got %q", out)
	}
}

// capitalizeWord must mirror Python str.capitalize(): uppercase the first rune
// AND lowercase every remaining rune. The previous Go version left the tail
// untouched, so "hELLo" produced "HELLO" instead of Python's "Hello".
func TestCapitalizeWord(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"knowledge graph", "Knowledge graph"},
		{"hELLo", "Hello"},
		{"WORLD", "World"},
		{"already", "Already"},
		{"123abc", "123abc"}, // first char non-letter: untouched, rest lowercased
		{"ÜBER", "Über"},     // unicode: first upper, rest lower
	}
	for _, c := range cases {
		if got := capitalizeWord(c.in); got != c.want {
			t.Errorf("capitalizeWord(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// AskStructure: request-scoped model + temperature 0.2 (Python _ask_structure).
// ---------------------------------------------------------------------------

// TestAskStructureNilModelSkips verifies a nil model short-circuits to an empty
// verdict instead of reaching the global invoker (mirrors Python's no-op when
// tools.chat_mdl is absent).
func TestAskStructureNilModelSkips(t *testing.T) {
	answer, relevant := AskStructure(context.Background(), nil, "Q", "knowledge graph", "Graph exploration", nil, nil)
	if answer != "" || len(relevant) != 0 {
		t.Errorf("nil model -> (%q, %v), want empty", answer, relevant)
	}
}

// TestAskStructureUsesTemperatureTwoTenths verifies the structure verdict is a
// mechanical rewrite drawn at 0.2, matching Python {"temperature": 0.2}.
func TestAskStructureUsesTemperatureTwoTenths(t *testing.T) {
	mdl := &tempRecordingModel{replies: []*ModelReply{{
		Content: `{"is_sufficient": true, "answer": "founded in 1984", "relevant_entities": ["OmiyaSoft"]}`,
	}}}
	answer, relevant := AskStructure(context.Background(), mdl, "who founded Culdcept?",
		"knowledge graph", "Graph exploration",
		[]map[string]any{saEntity("OmiyaSoft", "company", "")}, nil)
	if mdl.temperature == nil {
		t.Fatal("CompleteWithTemperature was not called")
	}
	if *mdl.temperature != 0.2 {
		t.Errorf("temperature = %v, want 0.2", *mdl.temperature)
	}
	if answer != "founded in 1984" {
		t.Errorf("answer = %q, want the sufficient verdict answer", answer)
	}
	if len(relevant) != 1 || relevant[0] != "OmiyaSoft" {
		t.Errorf("relevant = %v, want [OmiyaSoft]", relevant)
	}
}

// TestAskStructureInsufficientKeepsRelevant verifies an insufficient verdict
// still returns the relevant entities (evidence pulled from the source chunks).
func TestAskStructureInsufficientKeepsRelevant(t *testing.T) {
	mdl := &tempRecordingModel{replies: []*ModelReply{{
		Content: `{"is_sufficient": false, "answer": "", "relevant_entities": ["Culdcept"]}`,
	}}}
	answer, relevant := AskStructure(context.Background(), mdl, "Q", "mindmap", "Mindmap exploration",
		[]map[string]any{saEntity("Culdcept", "game", "")}, nil)
	if answer != "" {
		t.Errorf("insufficient answer = %q, want empty", answer)
	}
	if len(relevant) != 1 || relevant[0] != "Culdcept" {
		t.Errorf("relevant = %v, want [Culdcept]", relevant)
	}
}
