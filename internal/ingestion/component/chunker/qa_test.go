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
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
)

func TestQAChunker_Registered(t *testing.T) {
	factory, _, _, ok := runtime.DefaultRegistry.Lookup("QAChunker")
	if !ok {
		t.Fatal("QAChunker not found in registry")
	}
	comp, err := factory("QAChunker", nil)
	if err != nil {
		t.Fatalf("factory failed: %v", err)
	}
	if comp == nil {
		t.Fatal("component is nil")
	}
}

func TestQAChunker_DelimiterTab(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "What is Go?\tGo is a programming language.",
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	chunk := chunks[0]
	cww, _ := chunk["text"].(string)
	if cww != "Question: What is Go?\tAnswer: Go is a programming language." {
		t.Fatalf("unexpected content: %q", cww)
	}
}

func TestQAChunker_DelimiterComma(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.csv",
		"output_format": "text",
		"text":          "What is Rust?,Rust is a systems language.",
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	chunk := chunks[0]
	cww, _ := chunk["text"].(string)
	if cww != "Question: What is Rust?\tAnswer: Rust is a systems language." {
		t.Fatalf("unexpected content: %q", cww)
	}
}

func TestQAChunker_Markdown(t *testing.T) {
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.md",
		"output_format": "markdown",
		"markdown":      "# What is Go?\nGo is a programming language.\n\n# What is Rust?\nRust is a systems language.",
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
}

func TestQAChunker_HTMLTable(t *testing.T) {
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.xlsx",
		"output_format": "html",
		"html":          "<table><tr><td>Q1</td><td>A1</td></tr><tr><td>Q2</td><td>A2</td></tr></table>",
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
}

func TestQAChunker_CSVStrictPairRejectsThreeCells(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.CSV",
		"output_format": "html",
		"html":          "<table><tr><td>question</td><td></td><td>extra</td></tr></table>",
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks, got %d", len(chunks))
	}
}

func TestQAChunker_CSVStrictPairAcceptsTwoCells(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.csv",
		"output_format": "html",
		"html":          "<table><tr><td>question</td><td>answer</td></tr></table>",
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	cww, _ := chunks[0]["text"].(string)
	if cww != "Question: question\tAnswer: answer" {
		t.Fatalf("unexpected content: %q", cww)
	}
}

func TestQAChunker_JSONCSVNameUsesStrictRowShapeWithoutFileType(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "questions.csv",
		"output_format": "json",
		"json": []map[string]any{{
			"text":         "<table><tr><td>question</td><td></td><td>unexpected</td></tr></table>",
			"doc_type_kwd": "table",
			"ck_type":      "table",
		}},
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 0 {
		t.Fatalf("chunks = %#v, want malformed CSV row rejected", chunks)
	}
}

// Non-CSV names keep the old "first two non-empty cells" rule on the HTML
// table path. Since #18800 an .xlsx file reaches the chunker as "json", not
// "html", so this covers the shared HTML branch and not the XLSX pipeline.
func TestQAChunker_NonCSVHTMLThreeCellsKeepsFirstTwo(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.xls",
		"output_format": "html",
		"html":          "<table><tr><td>question</td><td></td><td>extra</td></tr></table>",
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	cww, _ := chunks[0]["text"].(string)
	if cww != "Question: question\tAnswer: extra" {
		t.Fatalf("unexpected content: %q", cww)
	}
}

func TestQAChunker_RmQAPrefix(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "Question: What is Go?\tAnswer: Go is a language.",
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	cww, _ := chunks[0]["text"].(string)
	if cww != "Question: What is Go?\tAnswer: Go is a language." {
		t.Fatalf("prefix not stripped: %q", cww)
	}
}

func TestQAChunker_Empty(t *testing.T) {
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "empty.txt",
		"output_format": "text",
		"text":          "",
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks, got %d", len(chunks))
	}
}

func TestQAChunker_CaseInsensitivePrefix(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "QUESTION: Hello\tANSWER: World",
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	cww, _ := chunks[0]["text"].(string)
	if cww != "Question: Hello\tAnswer: World" {
		t.Fatalf("case-insensitive prefix not stripped: %q", cww)
	}
}

func TestQAChunker_PrefixSpaceSeparatorStrips(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "A language model is useful\tQ How does it work",
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	cww, _ := chunks[0]["text"].(string)
	// Python qa.py:241 uses `[\t:： ]+`, so a space is a valid separator:
	// a leading "A"/"Q" followed by a space is stripped.
	if cww != "Question: language model is useful\tAnswer: How does it work" {
		t.Fatalf("space-separator prefix not stripped: %q", cww)
	}
}

func TestQAChunker_HeadingNoTrailingSpace(t *testing.T) {
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.md",
		"output_format": "markdown",
		"markdown":      "#Hello\nWorld\n",
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
}

func TestQAChunker_ChineseLang(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "Chinese"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "什么是Go？\tGo是一种编程语言。",
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	cww, _ := chunks[0]["text"].(string)
	if want := "问题：什么是Go？\t回答：Go是一种编程语言。"; cww != want {
		t.Fatalf("unexpected content: %q, want %q", cww, want)
	}
}

func TestQAChunker_MarkdownRendersHTML(t *testing.T) {
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.md",
		"output_format": "markdown",
		"markdown":      "# Title\nThis is **bold** text.\n",
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	cww, _ := chunks[0]["text"].(string)
	if !strings.Contains(cww, "<strong>bold</strong>") &&
		!strings.Contains(cww, "<b>bold</b>") {
		t.Fatalf("markdown not rendered to HTML: %q", cww)
	}
}

func TestQAChunker_XLSXJSONRegression(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "qa.xlsx",
		"output_format": "json",
		"json": []map[string]any{
			{
				"text":         "<table><caption>Sheet1</caption><tr><th>question</th><th>answer</th></tr><tr><td>What is RAGFlow?</td><td>A RAG engine.</td></tr><tr><td>Where are the docs?</td><td>On the website.</td></tr></table>",
				"doc_type_kwd": "table",
			},
		},
	}
	out, err := comp.Invoke(t.Context(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	expected := []string{
		"Question: question\tAnswer: answer",
		// rmQAPrefix strips a leading "A " answer marker from "A RAG engine."
		"Question: What is RAGFlow?\tAnswer: RAG engine.",
		"Question: Where are the docs?\tAnswer: On the website.",
	}
	for i, want := range expected {
		cww, _ := chunks[i]["text"].(string)
		if cww != want {
			t.Fatalf("chunk[%d] text = %q, want %q", i, cww, want)
		}
	}
}

func TestQAChunkerSpreadsheetWireTreatsFirstRowAsQAData(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := comp.Invoke(t.Context(), nil, map[string]any{
		"name":          "orders.xlsx",
		"file_type":     "xlsx",
		"output_format": "json",
		"json": []map[string]any{
			spreadsheetSegmentItem("Sheet1", []string{"ID", "Status"}, [][]string{{"A-100", "paid"}}, 1, 2),
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok || len(chunks) != 2 {
		t.Fatalf("chunks = %#v, want both QA rows", out["chunks"])
	}
	if got, _ := chunks[0]["text"].(string); got != "Question: ID\tAnswer: Status" {
		t.Fatalf("first-row QA = %q", got)
	}
	if got, _ := chunks[1]["text"].(string); got != "Question: A-100\tAnswer: paid" {
		t.Fatalf("second-row QA = %q", got)
	}
	// top_int keeps the legacy 0-based record index: rowStart - 1.
	for i, want := range []any{float64(0), float64(1)} {
		top, _ := chunks[i]["top_int"].([]any)
		if len(top) != 1 || top[0] != want {
			t.Errorf("chunk[%d] top_int = %v, want [%v]", i, chunks[i]["top_int"], want)
		}
	}
	// R1: each pair carries only its own row's tuple, not the segment matrix.
	for i, wantRow := range []float64{1, 2} {
		raw, _ := json.Marshal(chunks[i]["positions"])
		var matrix [][]float64
		if err := json.Unmarshal(raw, &matrix); err != nil {
			t.Fatalf("chunk[%d] positions = %v: %v", i, chunks[i]["positions"], err)
		}
		if len(matrix) != 1 {
			t.Fatalf("chunk[%d] positions = %v, want one tuple", i, chunks[i]["positions"])
		}
		if tuple := matrix[0]; len(tuple) != 5 || tuple[1] != wantRow {
			t.Errorf("chunk[%d] tuple = %v, want rowStart %v", i, tuple, wantRow)
		}
	}
}

// TestQAChunker_PlainTablePayloadNotSwallowed: a doc_type:"table" item whose
// payload is plain delimited text (the shape of a PDF table item) used to
// enter the HTML table extractor, find no <tr>, and lose every pair silently.
// Routing on what the row walker can read sends it to the text extractor
// instead.
func TestQAChunker_PlainTablePayloadNotSwallowed(t *testing.T) {
	items := []schema.ChunkDoc{
		{Text: "what is it\tit is a thing\nwho comes\tbob", DocType: "table"},
	}
	pairs := extractQAJSON(items, "")
	if len(pairs) != 2 {
		t.Fatalf("got %d pairs, want 2 (plain payload under a table label must still yield pairs)", len(pairs))
	}
	if pairs[0].Question != "what is it" || pairs[0].Answer != "it is a thing" {
		t.Errorf("pair0 = %q/%q", pairs[0].Question, pairs[0].Answer)
	}
}

// TestQAChunker_TextLabelledHTMLTableReadAsTable documents the accepted
// non-monotonic side of payload routing: a text-labelled block that
// genuinely holds table markup is now read as a table.
func TestQAChunker_TextLabelledHTMLTableReadAsTable(t *testing.T) {
	items := []schema.ChunkDoc{
		{Text: "<table><tr><td>q1</td><td>a1</td></tr><tr><td>q2</td><td>a2</td></tr></table>", DocType: "text"},
	}
	pairs := extractQAJSON(items, "")
	if len(pairs) != 2 {
		t.Fatalf("got %d pairs, want 2", len(pairs))
	}
	if pairs[1].Question != "q2" || pairs[1].Answer != "a2" {
		t.Errorf("pair1 = %q/%q", pairs[1].Question, pairs[1].Answer)
	}
}

// TestQAChunker_RowlessTableTextStaysProse: text that merely opens with
// "<table" — a table element without rows, or a "<tableau>" token — must
// stay on the prose path. The table extractor would find nothing to pair
// there, so routing it by prefix alone would silently turn readable text
// into zero pairs.
func TestQAChunker_RowlessTableTextStaysProse(t *testing.T) {
	items := []schema.ChunkDoc{
		{Text: "<table><caption>schema</caption></table>\nwhat is it\tit is a thing", DocType: "table"},
		{Text: "<tableau>\nwhat is it\tit is a thing", DocType: "table"},
	}
	pairs := extractQAJSON(items, "")
	if len(pairs) != 2 {
		t.Fatalf("got %d pairs, want one from each item via the prose path", len(pairs))
	}
	for i, pair := range pairs {
		if pair.Question != "what is it" || pair.Answer != "it is a thing" {
			t.Errorf("pair%d = %q/%q", i, pair.Question, pair.Answer)
		}
	}
}

// TestQAChunkerPositionsRequireSpreadsheetIdentityAndFiveFields: a positions
// matrix is read as per-row spreadsheet tuples only when the item carries
// spreadsheet identity and every tuple has the five wire fields. A short tuple
// must not panic the pair builder, and a matrix without identity must not move
// the row numbers (PDF items write layout boxes into the same field).
func TestQAChunkerPositionsRequireSpreadsheetIdentityAndFiveFields(t *testing.T) {
	table := "<table><tr><th>q</th><th>a</th></tr></table>"
	cases := []struct {
		name       string
		positions  string
		sheetIndex *int
		wantRowNum int
	}{
		{"aligned sheet matrix", `[[1,5,5,1,2]]`, intPtr(1), 4},
		{"short tuple", `[[1]]`, intPtr(1), 0},
		{"sheet matrix without identity", `[[1,5,5,1,2]]`, nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := schema.ChunkDoc{
				Text:       table,
				DocType:    "table",
				Positions:  json.RawMessage(tc.positions),
				SheetIndex: tc.sheetIndex,
			}
			pairs := extractQAJSON([]schema.ChunkDoc{item}, "")
			if len(pairs) != 1 {
				t.Fatalf("got %d pairs, want 1", len(pairs))
			}
			if pairs[0].RowNum != tc.wantRowNum {
				t.Errorf("RowNum = %d, want %d", pairs[0].RowNum, tc.wantRowNum)
			}
		})
	}
}
