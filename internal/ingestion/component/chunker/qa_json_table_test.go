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

package chunker

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"

	"ragflow/internal/parser/parser"
)

// newQASheet writes rows into the first sheet of a new workbook and returns
// the file bytes.
func newQASheet(t *testing.T, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	for i, row := range rows {
		for j, value := range row {
			cell, err := excelize.CoordinatesToCellName(j+1, i+1)
			if err != nil {
				t.Fatalf("CoordinatesToCellName: %v", err)
			}
			if err := f.SetCellValue(sheet, cell, value); err != nil {
				t.Fatalf("SetCellValue: %v", err)
			}
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return buf.Bytes()
}

// TestQAChunker_XLSXTablePairs runs the real XLSX parser into the QA chunker.
// The parser emits output_format "json" with a rendered HTML table in each
// item text, so the JSON path must read the table rows. Reported in #19174.
func TestQAChunker_XLSXTablePairs(t *testing.T) {
	data := newQASheet(t, [][]string{
		{"question", "answer"},
		{"What is RAGFlow?", "RAGFlow is a RAG engine."},
		{"Where are the docs?", "On the website."},
	})

	p, err := parser.NewXLSXParser("")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	parsed := p.ParseWithResult(t.Context(), "faq.xlsx", data)
	if parsed.Err != nil {
		t.Fatalf("ParseWithResult: %v", parsed.Err)
	}
	if parsed.OutputFormat != "json" {
		t.Fatalf("OutputFormat = %q, want %q", parsed.OutputFormat, "json")
	}

	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatalf("NewQAChunker: %v", err)
	}
	out, err := comp.Invoke(t.Context(), nil, map[string]any{
		"name":          "faq.xlsx",
		"output_format": parsed.OutputFormat,
		"json":          parsed.JSON,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if msg, ok := out["_ERROR"].(string); ok && msg != "" {
		t.Fatalf("QAChunker returned _ERROR: %s", msg)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks has type %T", out["chunks"])
	}

	want := []string{
		"Question: question\tAnswer: answer",
		"Question: What is RAGFlow?\tAnswer: RAGFlow is a RAG engine.",
		"Question: Where are the docs?\tAnswer: On the website.",
	}
	if len(chunks) != len(want) {
		t.Fatalf("chunk count = %d, want %d", len(chunks), len(want))
	}
	for i, expected := range want {
		got, _ := chunks[i]["content_with_weight"].(string)
		if got != expected {
			t.Errorf("chunk %d = %q, want %q", i, got, expected)
		}
		// Each pair keeps its source row, the same as the text path does.
		raw, ok := chunks[i]["top_int"].([]any)
		if !ok || len(raw) != 1 {
			t.Fatalf("chunk %d top_int has shape %#v", i, chunks[i]["top_int"])
		}
		if int(raw[0].(float64)) != i {
			t.Errorf("chunk %d top_int = %v, want %d", i, raw[0], i)
		}
	}
}

// TestQAChunker_JSONTabTextStaysOnTheTextPath keeps delimiter-separated item
// text on the text extractor. Only HTML markup goes to the table extractor.
func TestQAChunker_JSONTabTextStaysOnTheTextPath(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatalf("NewQAChunker: %v", err)
	}
	out, err := comp.Invoke(t.Context(), nil, map[string]any{
		"name":          "notes.pdf",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "What is Go?\tGo is a programming language.", "doc_type_kwd": "table"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks has type %T", out["chunks"])
	}
	if len(chunks) != 1 {
		t.Fatalf("chunk count = %d, want 1", len(chunks))
	}
	want := "Question: What is Go?\tAnswer: Go is a programming language."
	if got, _ := chunks[0]["content_with_weight"].(string); got != want {
		t.Errorf("chunk = %q, want %q", got, want)
	}
}
