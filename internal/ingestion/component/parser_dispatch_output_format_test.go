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

	"ragflow/internal/parser/parser"
)

// TestBuildParserOutputs_FileTypeOTHER_NotProduced pins that a zero (not-produced)
// ParseResult — the shape dispatchParse returns for an unknown FileTypeOTHER —
// still emits the wire output_format="json" key but is routed to the raw-text
// fallback (never treated as a structured empty payload). This is the exact
// equivalence the Produced() gate must preserve for the FileTypeOTHER case.
func TestBuildParserOutputs_FileTypeOTHER_NotProduced(t *testing.T) {
	out := buildParserOutputs(context.Background(), parser.ParseResult{}, "f.bin", nil, "")
	if out["output_format"] != "json" {
		t.Errorf("output_format = %v, want json", out["output_format"])
	}
	items, ok := out["json"].([]parser.DocItem)
	if !ok {
		t.Fatalf("json has unexpected type %T", out["json"])
	}
	if len(items) == 0 {
		t.Error("expected raw-text fallback items for a zero result")
	}
}

// TestBuildParserOutputs_MarkdownProducerBecomesJSON pins that a producer that
// already returns normalized JSON (the post-refactor contract — markdown is
// converted at the producer boundary via NormalizeMarkdown, never carried in a
// ParseResult.Markdown field) is emitted verbatim by buildParserOutputs.
func TestBuildParserOutputs_MarkdownProducerBecomesJSON(t *testing.T) {
	dispatched := parser.ParseResult{
		JSON: parser.MapsToItems([]map[string]any{
			{"text": "x", "doc_type_kwd": "text"},
		}),
	}
	out := buildParserOutputs(context.Background(), dispatched, "f.md", nil, "")
	if out["output_format"] != "json" {
		t.Errorf("output_format = %v, want json", out["output_format"])
	}
	items, ok := out["json"].([]parser.DocItem)
	if !ok {
		t.Fatalf("json has unexpected type %T", out["json"])
	}
	if len(items) != 1 {
		t.Errorf("len(items) = %d, want 1", len(items))
	}
}
