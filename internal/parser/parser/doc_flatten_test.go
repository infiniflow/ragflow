//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parser

import (
	"strings"
	"testing"
)

// sampleDocIR is a hand-built office_oxide IR JSON covering the element types
// a legacy .doc typically contains: a heading, paragraphs, a list, and a
// table. It exercises flattenDocIR without needing a real binary .doc fixture
// (the cgo integration test owns that). The schema mirrors docx_ir.go and
// office_oxide's format-independent IR.
const sampleDocIR = `{
  "sections": [
    {
      "title": "Quarterly Report",
      "elements": [
        {"type": "heading", "level": 1, "content": [{"type": "text", "text": "Summary"}]},
        {"type": "paragraph", "content": [{"type": "text", "text": "Revenue grew this quarter."}]},
        {"type": "paragraph", "content": [{"type": "text", "text": "Costs were controlled."}]},
        {"type": "list", "ordered": false, "items": [
          {"content": [{"type": "paragraph", "content": [{"type": "text", "text": "Hired two engineers"}]}]},
          {"content": [{"type": "paragraph", "content": [{"type": "text", "text": "Shipped the parser"}]}]}
        ]},
        {"type": "table", "rows": [
          {"cells": [{"content": [{"type": "paragraph", "content": [{"type": "text", "text": "Region"}]}]}, {"content": [{"type": "paragraph", "content": [{"type": "text", "text": "Sales"}]}]}]},
          {"cells": [{"content": [{"type": "paragraph", "content": [{"type": "text", "text": "East"}]}]}, {"content": [{"type": "paragraph", "content": [{"type": "text", "text": "120"}]}]}]},
          {"cells": [{"content": [{"type": "paragraph", "content": [{"type": "text", "text": "West"}]}]}, {"content": [{"type": "paragraph", "content": [{"type": "text", "text": "95"}]}]}]}
        ]}
      ]
    }
  ]
}`

func TestFlattenDocIR_PreservesStructure(t *testing.T) {
	got := flattenDocIR(sampleDocIR)
	if strings.TrimSpace(got) == "" {
		t.Fatalf("flattenDocIR returned empty text")
	}

	// Heading / section title and paragraph text must survive.
	for _, want := range []string{
		"Quarterly Report",
		"Summary",
		"Revenue grew this quarter.",
		"Costs were controlled.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("flattened text missing %q\n got=%q", want, got)
		}
	}

	// List items must survive (not dropped).
	for _, want := range []string{"Hired two engineers", "Shipped the parser"} {
		if !strings.Contains(got, want) {
			t.Errorf("flattened text missing list item %q\n got=%q", want, got)
		}
	}

	// Table cells must survive, separated by " | " within a row and by
	// newlines across rows — this is the fidelity gain over PlainText.
	for _, want := range []string{
		"Region | Sales",
		"East | 120",
		"West | 95",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("flattened text missing table cell group %q\n got=%q", want, got)
		}
	}
}

func TestFlattenDocIR_InvalidJSONReturnsEmpty(t *testing.T) {
	if got := flattenDocIR("not json"); got != "" {
		t.Errorf("flattenDocIR(invalid) = %q, want empty", got)
	}
}
