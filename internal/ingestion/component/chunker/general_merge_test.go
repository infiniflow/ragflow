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
	"reflect"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

func TestMergeGeneralUnitsAllowsOneBoundaryOverflow(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "alpha", DocType: "text", CKType: "text", TKNums: intPtr(2)},
		{Text: "beta", DocType: "text", CKType: "text", TKNums: intPtr(2)},
		{Text: "gamma", DocType: "text", CKType: "text", TKNums: intPtr(2)},
	}

	got := mergeGeneralUnits(units, 3, 0, "\n")
	if texts := generalChunkTexts(got); !reflect.DeepEqual(texts, []string{"alpha\nbeta", "gamma"}) {
		t.Fatalf("texts = %q, want [alpha\\nbeta gamma]", texts)
	}
	if count := intValue(got[0].TKNums); count != 4 {
		t.Errorf("first chunk tokens = %d, want running sum 4", count)
	}
}

func TestMergeSpreadsheetRowsUsesProjectedCapAndSheetBoundary(t *testing.T) {
	sheetOne := 1
	sheetTwo := 2
	rows := []schema.ChunkDoc{
		{Text: "row-1", DocType: "text", CKType: "table_row", TKNums: intPtr(2), SheetIndex: &sheetOne, RowStart: intPtr(2), RowEnd: intPtr(2), ColStart: intPtr(1), ColEnd: intPtr(2)},
		{Text: "row-2", DocType: "text", CKType: "table_row", TKNums: intPtr(1), SheetIndex: &sheetOne, RowStart: intPtr(3), RowEnd: intPtr(3), ColStart: intPtr(1), ColEnd: intPtr(2)},
		{Text: "row-3", DocType: "text", CKType: "table_row", TKNums: intPtr(1), SheetIndex: &sheetTwo, RowStart: intPtr(2), RowEnd: intPtr(2), ColStart: intPtr(1), ColEnd: intPtr(2)},
	}

	got := mergeSpreadsheetRows(rows, 3)
	if texts := generalChunkTexts(got); !reflect.DeepEqual(texts, []string{"row-1\nrow-2", "row-3"}) {
		t.Fatalf("texts = %q", texts)
	}
	if len(got) != 2 || got[0].TKNums == nil || *got[0].TKNums != 3 {
		t.Fatalf("merged row token count = %#v, want 3", got)
	}
	if got[0].RowStart == nil || *got[0].RowStart != 2 || got[0].RowEnd == nil || *got[0].RowEnd != 3 {
		t.Fatalf("merged row range = start:%v end:%v", got[0].RowStart, got[0].RowEnd)
	}
	if got[0].SheetIndex == nil || *got[0].SheetIndex != 1 || got[1].SheetIndex == nil || *got[1].SheetIndex != 2 {
		t.Fatalf("sheet boundaries not preserved: %#v", got)
	}
}

func TestMergeSpreadsheetRowsKeepsOversizedRowAndCapZeroAtomic(t *testing.T) {
	sheet := 1
	rows := []schema.ChunkDoc{
		{Text: "small", DocType: "text", CKType: "table_row", TKNums: intPtr(1), SheetIndex: &sheet, RowStart: intPtr(2), RowEnd: intPtr(2)},
		{Text: "oversized", DocType: "text", CKType: "table_row", TKNums: intPtr(8), SheetIndex: &sheet, RowStart: intPtr(3), RowEnd: intPtr(3)},
		{Text: "after", DocType: "text", CKType: "table_row", TKNums: intPtr(1), SheetIndex: &sheet, RowStart: intPtr(4), RowEnd: intPtr(4)},
	}
	if got := mergeSpreadsheetRows(rows, 3); !reflect.DeepEqual(generalChunkTexts(got), []string{"small", "oversized", "after"}) {
		t.Fatalf("oversized texts = %q", generalChunkTexts(got))
	}
	if got := mergeSpreadsheetRows(rows, 0); !reflect.DeepEqual(generalChunkTexts(got), []string{"small", "oversized", "after"}) {
		t.Fatalf("zero-cap texts = %q", generalChunkTexts(got))
	}
}

func TestGeneralChunkerSpreadsheetConsumesRowIR(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{"chunk_token_size": 3})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "orders.xlsx",
		"file_type":     "xlsx",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "ID; Status", "doc_type_kwd": "table", "ck_type": "table_header", "sheet_index": 1, "table_id": "sheet-1"},
			{"text": "ID：A-100; Status：paid", "doc_type_kwd": "text", "ck_type": "table_row", "tk_nums": 2, "sheet_index": 1, "table_id": "sheet-1", "row_start": 2, "row_end": 2},
			{"text": "ID：A-101; Status：pending", "doc_type_kwd": "text", "ck_type": "table_row", "tk_nums": 1, "sheet_index": 1, "table_id": "sheet-1", "row_start": 3, "row_end": 3},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if texts := outputTexts(t, out); !reflect.DeepEqual(texts, []string{"ID：A-100; Status：paid\nID：A-101; Status：pending"}) {
		t.Fatalf("spreadsheet chunks = %q", texts)
	}
}

func TestMergeGeneralUnitsMergesWhenCurrentEqualsCap(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "at cap", DocType: "text", CKType: "text", TKNums: intPtr(3)},
		{Text: "overflow", DocType: "text", CKType: "text", TKNums: intPtr(1)},
	}

	got := mergeGeneralUnits(units, 3, 0, "\n")
	if texts := generalChunkTexts(got); !reflect.DeepEqual(texts, []string{"at cap\noverflow"}) {
		t.Fatalf("texts = %q, want one overflow chunk", texts)
	}
}

func TestMergeGeneralUnitsKeepsOversizedAtomWhole(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "before", DocType: "text", CKType: "text", TKNums: intPtr(2)},
		{Text: "oversized atom stays whole", DocType: "text", CKType: "text", TKNums: intPtr(8)},
		{Text: "after", DocType: "text", CKType: "text", TKNums: intPtr(1)},
	}

	got := mergeGeneralUnits(units, 3, 0, "\n")
	if texts := generalChunkTexts(got); !reflect.DeepEqual(texts, []string{"before", "oversized atom stays whole", "after"}) {
		t.Fatalf("texts = %q", texts)
	}
}

func TestMergeGeneralUnitsAppliesUnconditionalCharacterOverlap(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "abcdefghij@@1\t0\t1\t2\t3##", DocType: "text", CKType: "text", TKNums: intPtr(2)},
		{Text: "klmnopqrst", DocType: "text", CKType: "text", TKNums: intPtr(2)},
		{Text: "uvwxyzABCD", DocType: "text", CKType: "text", TKNums: intPtr(2)},
	}

	got := mergeGeneralUnits(units, 2, 50, "\n")
	if texts := generalChunkTexts(got); !reflect.DeepEqual(texts, []string{
		"abcdefghij@@1\t0\t1\t2\t3##",
		"fghij\nklmnopqrst",
		"mnopqrst\nuvwxyzABCD",
	}) {
		t.Fatalf("texts = %q", texts)
	}
	if intValue(got[1].TKNums) <= 2 {
		t.Errorf("overlap was trimmed to cap: token count = %d", intValue(got[1].TKNums))
	}
}

func TestMergeSpreadsheetRowsUsesPositionSheetWhenIdentityFieldsAreMissing(t *testing.T) {
	rows := []schema.ChunkDoc{
		{Text: "sheet-1-row", DocType: "text", CKType: "table_row", TKNums: intPtr(1), Positions: json.RawMessage(`[[1,2,2,1,2]]`)},
		{Text: "sheet-2-row", DocType: "text", CKType: "table_row", TKNums: intPtr(1), Positions: json.RawMessage(`[[2,2,2,1,2]]`)},
	}

	got := mergeSpreadsheetRows(rows, 10)
	if texts := generalChunkTexts(got); !reflect.DeepEqual(texts, []string{"sheet-1-row", "sheet-2-row"}) {
		t.Fatalf("position-only sheet boundary was lost: texts = %q", texts)
	}
}

func TestMergeMarkdownUnitsCarriesCharacterOverlapIntoNextBudget(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "alpha beta", DocType: "text", CKType: "text"},
		{Text: "gamma delta", DocType: "text", CKType: "text"},
		{Text: "epsilon zeta", DocType: "text", CKType: "text"},
		{Text: "eta theta", DocType: "text", CKType: "text"},
	}
	for i := range units {
		units[i].TKNums = intPtr(tokenizeStr(units[i].Text))
	}
	target := intValue(units[0].TKNums) + intValue(units[1].TKNums)

	got := mergeMarkdownUnits(units, target, 50, "\n")
	want := []string{
		"alpha beta\ngamma delta",
		"gamma delta\nepsilon zeta",
		"epsilon zeta\neta theta",
	}
	if texts := generalChunkTexts(got); !reflect.DeepEqual(texts, want) {
		t.Fatalf("markdown overlap = %q, want %q", texts, want)
	}
}

func TestMergeGeneralUnitsKeepsMediaAtomicAndBreaksTextRun(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "before", DocType: "text", CKType: "text", TKNums: intPtr(1)},
		{Text: "A|B", DocType: "table", CKType: "table", TKNums: intPtr(2)},
		{Text: "after", DocType: "text", CKType: "text", TKNums: intPtr(1)},
	}

	got := mergeGeneralUnits(units, 10, 0, "\n")
	if texts := generalChunkTexts(got); !reflect.DeepEqual(texts, []string{"before", "A|B", "after"}) {
		t.Fatalf("texts = %q", texts)
	}
	if got[1].DocType != "table" || got[1].CKType != "table" {
		t.Errorf("table type changed: %#v", got[1])
	}
}

func TestMergeGeneralUnitsCombinesPositionsAndMetadata(t *testing.T) {
	first := schema.ChunkDoc{
		Text:         "alpha",
		DocType:      "text",
		CKType:       "text",
		TKNums:       intPtr(1),
		Image:        "image-a",
		Positions:    json.RawMessage(`[[0,0,10,0,10]]`),
		PDFPositions: json.RawMessage(`[[0,0,10,0,10]]`),
		Extra:        map[string]json.RawMessage{"source_order": json.RawMessage(`1`)},
		PageNumber:   intPtr(4),
	}
	second := schema.ChunkDoc{
		Text:         "beta",
		DocType:      "text",
		CKType:       "text",
		TKNums:       intPtr(1),
		Positions:    json.RawMessage(`[[0,0,10,20,30]]`),
		PDFPositions: json.RawMessage(`[[0,0,10,20,30]]`),
		Extra:        map[string]json.RawMessage{"heading_level": json.RawMessage(`2`)},
	}

	got := mergeGeneralUnits([]schema.ChunkDoc{first, second}, 1, 0, "\n")
	if len(got) != 1 {
		t.Fatalf("chunks = %d, want 1", len(got))
	}
	if got[0].Image != "image-a" {
		t.Errorf("image = %q, want image-a", got[0].Image)
	}
	if got[0].PageNumber == nil || *got[0].PageNumber != 4 {
		t.Errorf("page number = %v, want 4", got[0].PageNumber)
	}
	if string(got[0].Positions) != `[[0,0,10,0,10],[0,0,10,20,30]]` {
		t.Errorf("positions = %s", got[0].Positions)
	}
	if string(got[0].PDFPositions) != `[[0,0,10,0,10],[0,0,10,20,30]]` {
		t.Errorf("PDF positions = %s", got[0].PDFPositions)
	}
	if string(got[0].Extra["source_order"]) != "1" || string(got[0].Extra["heading_level"]) != "2" {
		t.Errorf("extra metadata = %#v", got[0].Extra)
	}
}

func TestGeneralChunkerPreservesImageWithoutText(t *testing.T) {
	component, err := NewGeneralChunker(nil)
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.pdf",
		"file_type":     "pdf",
		"output_format": "json",
		"json": []map[string]any{{
			"doc_type_kwd": "image",
			"image":        "image-payload",
		}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 1 || chunks[0]["image"] != "image-payload" {
		t.Fatalf("chunks = %#v, want image-only unit", chunks)
	}
}

func TestGeneralChunkerCustomDelimiterBypassesMerge(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{
		"chunk_token_size": 1,
		"delimiters":       []string{"`||`"},
	})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, generalTextInput("alpha beta||gamma delta"))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if texts := outputTexts(t, out); !reflect.DeepEqual(texts, []string{"alpha beta", "gamma delta"}) {
		t.Fatalf("texts = %q", texts)
	}
}

func TestGeneralChunkerBareDelimiterCreatesMergeAtoms(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{
		"chunk_token_size": 0,
		"delimiters":       []string{"|"},
	})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, generalTextInput("alpha|beta"))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if texts := outputTexts(t, out); !reflect.DeepEqual(texts, []string{"alpha", "beta"}) {
		t.Fatalf("texts = %q", texts)
	}
}

func TestGeneralChunkerSplitsChildrenAfterParentMerge(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{
		"chunk_token_size":    100,
		"delimiters":          []string{},
		"children_delimiters": []string{"|"},
	})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, generalTextInput("alpha|beta"))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if texts := outputTexts(t, out); !reflect.DeepEqual(texts, []string{"alpha|", "beta"}) {
		t.Fatalf("texts = %q", texts)
	}
	for i, chunk := range chunks {
		if chunk["mom"] != "alpha|beta" {
			t.Errorf("chunk[%d].mom = %q", i, chunk["mom"])
		}
	}
}

func generalChunkTexts(chunks []schema.ChunkDoc) []string {
	texts := make([]string, len(chunks))
	for i := range chunks {
		texts[i] = chunks[i].Text
	}
	return texts
}

func generalTextInput(text string) map[string]any {
	return map[string]any{
		"name":          "document.txt",
		"file_type":     "txt",
		"output_format": "json",
		"json":          []map[string]any{{"text": text, "doc_type_kwd": "text"}},
	}
}

func outputChunks(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks = %T, want []map[string]any", out["chunks"])
	}
	return chunks
}

func outputTexts(t *testing.T, out map[string]any) []string {
	t.Helper()
	chunks := outputChunks(t, out)
	texts := make([]string, len(chunks))
	for i := range chunks {
		texts[i], _ = chunks[i]["text"].(string)
	}
	return texts
}
