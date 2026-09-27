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

package chunker

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
)

func TestSplitTextParserSentencesKeepsSentenceUnits(t *testing.T) {
	item := schema.ChunkDoc{Text: "First!Second?第三。Fourth；", DocType: "text", CKType: "text"}
	parts := splitTextParserSentences(item)
	if got, want := len(parts), 4; got != want {
		t.Fatalf("sentence parts = %d, want %d: %#v", got, want, parts)
	}
	want := []string{"First!", "Second?", "第三。", "Fourth；"}
	for i, part := range parts {
		if part.Text != want[i] {
			t.Errorf("part[%d] = %q, want %q", i, part.Text, want[i])
		}
	}
}

func TestTokenChunkerTextParserSentenceFallbackEndToEnd(t *testing.T) {
	const source = "First!Second?第三。Fourth；"
	parsed := parser.NewTextParser().ParseWithResult(t.Context(), "sample.txt", []byte(source))
	if parsed.Err != nil {
		t.Fatalf("TextParser.ParseWithResult: %v", parsed.Err)
	}
	if len(parsed.JSON) != 1 {
		t.Fatalf("TextParser JSON items = %d, want one parser item", len(parsed.JSON))
	}

	chunker, err := NewTokenChunker(map[string]any{"chunk_token_size": 3})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := chunker.Invoke(context.Background(), nil, map[string]any{
		"name":          "sample.txt",
		"file_type":     "txt",
		"output_format": "json",
		"json":          parsed.JSON,
	})
	if err != nil {
		t.Fatalf("TokenChunker.Invoke: %v", err)
	}
	chunks := chunksFromOutput(t, out)
	want := []string{"First!", "Second?", "第三。", "Fourth；"}
	if len(chunks) != len(want) {
		t.Fatalf("chunks = %#v, want one chunk per sentence", chunks)
	}
	for i, chunk := range chunks {
		if got, wantText := chunk["text"], want[i]; got != wantText {
			t.Errorf("chunk[%d].text = %v, want %q", i, got, wantText)
		}
	}
}

func TestGeneralChunkerTextAppliesOverlapThroughInvoke(t *testing.T) {
	chunker, err := NewGeneralChunker(map[string]any{
		"chunk_token_size":   2,
		"overlapped_percent": 50,
	})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := chunker.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.txt",
		"file_type":     "txt",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "alpha beta", "doc_type_kwd": "text"},
			{"text": "gamma delta", "doc_type_kwd": "text"},
		},
	})
	if err != nil {
		t.Fatalf("GeneralChunker.Invoke: %v", err)
	}
	texts := outputTexts(t, out)
	if len(texts) != 2 {
		t.Fatalf("texts = %q, want two overlapped chunks", texts)
	}
	if texts[1] == "gamma delta" || !strings.HasSuffix(texts[1], "\ngamma delta") {
		t.Fatalf("text overlap = %q, want previous tail plus current text", texts[1])
	}
}

func TestGeneralChunkerEmptyJSONReturnsNoChunksForEachStrategy(t *testing.T) {
	for _, fileType := range []string{"txt", "md", "docx", "pdf", "xlsx"} {
		t.Run(fileType, func(t *testing.T) {
			chunker, err := NewGeneralChunker(nil)
			if err != nil {
				t.Fatalf("NewGeneralChunker: %v", err)
			}
			out, err := chunker.Invoke(t.Context(), nil, map[string]any{
				"name":          "empty." + fileType,
				"file_type":     fileType,
				"output_format": "json",
				"json":          []map[string]any{},
			})
			if err != nil {
				t.Fatalf("GeneralChunker.Invoke: %v", err)
			}
			if chunks := outputChunks(t, out); len(chunks) != 0 {
				t.Fatalf("chunks = %#v, want empty output", chunks)
			}
		})
	}
}

func TestGeneralChunkerMarkdownLongHeadingKeepsFollowingTableAtomic(t *testing.T) {
	chunker, err := NewGeneralChunker(map[string]any{"chunk_token_size": 1})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	longHeading := "# " + strings.Repeat("heading ", 60)
	out, err := chunker.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.md",
		"file_type":     "md",
		"output_format": "json",
		"json": []map[string]any{
			{"text": longHeading, "doc_type_kwd": "text", "ck_type": "heading"},
			{"text": "<table><tr><td>A</td></tr></table>", "doc_type_kwd": "table", "ck_type": "table"},
		},
	})
	if err != nil {
		t.Fatalf("GeneralChunker.Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %#v, want separate heading and table chunks", chunks)
	}
	if chunks[0]["ck_type"] != "heading" || chunks[0]["text"] != strings.TrimSpace(longHeading) {
		t.Errorf("heading chunk = %#v", chunks[0])
	}
	if chunks[1]["ck_type"] != "table" || chunks[1]["text"] != "<table><tr><td>A</td></tr></table>" {
		t.Errorf("table chunk = %#v", chunks[1])
	}
}

func TestGeneralChunkerKeepsHeaderOnlySheetsSeparate(t *testing.T) {
	chunker, err := NewGeneralChunker(nil)
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := chunker.Invoke(t.Context(), nil, map[string]any{
		"name":          "empty-sheets.xlsx",
		"file_type":     "xlsx",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "Name", "doc_type_kwd": "table", "ck_type": "table_header", "sheet_index": 1, "table_id": "sheet-1"},
			{"text": "Amount", "doc_type_kwd": "table", "ck_type": "table_header", "sheet_index": 2, "table_id": "sheet-2"},
		},
	})
	if err != nil {
		t.Fatalf("GeneralChunker.Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %#v, want one header-only chunk per sheet", chunks)
	}
	if chunks[0]["text"] != "Name" || chunks[1]["text"] != "Amount" {
		t.Fatalf("header-only chunks = %#v", chunks)
	}
}
