// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parser

import (
	"encoding/json"
	"reflect"
	"testing"
)

// AbsoluteBox.Wire projects the PDF absolute bounding box [page, l, r, t, b]
// onto the historical wire tuple, keeping the page as an int (never float64).
func TestAbsoluteBoxWire(t *testing.T) {
	b := AbsoluteBox{PageNumber: 1, Left: 2, Right: 3, Top: 4, Bottom: 5}
	got := b.Wire()
	want := []any{1, 2.0, 3.0, 4.0, 5.0}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AbsoluteBox.Wire() = %#v, want %#v", got, want)
	}
}

// RowColumnBox.Wire projects the spreadsheet range [sheet, r0, r1, c0, c1]
// onto the historical wire tuple as all-int values.
func TestRowColumnBoxWire(t *testing.T) {
	b := RowColumnBox{Sheet: 3, R0: 1, R1: 9, C0: 2, C1: 7}
	got := b.Wire()
	want := []any{3, 1, 9, 2, 7}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RowColumnBox.Wire() = %#v, want %#v", got, want)
	}
}

// Kind() is a method on the concrete type (single source of truth), not a
// stored field, so it can never disagree with the real type.
func TestPositionKindMethod(t *testing.T) {
	if (AbsoluteBox{}).Kind() != KindAbsolute {
		t.Fatal("AbsoluteBox.Kind() != KindAbsolute")
	}
	if (RowColumnBox{}).Kind() != KindRowColumn {
		t.Fatal("RowColumnBox.Kind() != KindRowColumn")
	}
	var _ Position = AbsoluteBox{}
	var _ Position = RowColumnBox{}
}

// PDF path positions arrive as [][]any (int page + float64 coords) from
// normalizePDFPositions. wireToPositions must build AbsoluteBox and round-trip
// byte-exact, with the page preserved as int.
func TestWireToPositionsAbsoluteAny(t *testing.T) {
	raw := [][]any{{1, 2.0, 3.0, 4.0, 5.0}}
	got := wireToPositions(raw, false)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	ab, ok := got[0].(AbsoluteBox)
	if !ok {
		t.Fatalf("got[0] = %T, want AbsoluteBox", got[0])
	}
	if ab.PageNumber != 1 || ab.Left != 2 || ab.Top != 4 || ab.Bottom != 5 {
		t.Fatalf("AbsoluteBox = %+v", ab)
	}
	if !reflect.DeepEqual(positionsToWire(got), []any{[]any{1, 2.0, 3.0, 4.0, 5.0}}) {
		t.Fatalf("round-trip = %#v", positionsToWire(got))
	}
}

// Excel path positions arrive as [][]float64 of [sheet, r0, r1, c0, c1]. With
// the sheet_index signal present, wireToPositions must build RowColumnBox and
// the wire must be all-int []any, JSON-equal to the historical [][]float64.
func TestWireToPositionsRowColumn(t *testing.T) {
	raw := [][]float64{{3, 1, 9, 2, 7}}
	got := wireToPositions(raw, true)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	rb, ok := got[0].(RowColumnBox)
	if !ok {
		t.Fatalf("got[0] = %T, want RowColumnBox", got[0])
	}
	if rb.Sheet != 3 || rb.R0 != 1 || rb.R1 != 9 || rb.C0 != 2 || rb.C1 != 7 {
		t.Fatalf("RowColumnBox = %+v", rb)
	}
	out := positionsToWire(got)
	if !reflect.DeepEqual(out, [][]float64{{3, 1, 9, 2, 7}}) {
		t.Fatalf("positionsToWire = %#v, want [][]float64", out)
	}
	b, _ := json.Marshal(out)
	if string(b) != "[[3,1,9,2,7]]" {
		t.Fatalf("json = %s", b)
	}
}

// Malformed / non-5-tuple input must not panic and must return nil so the
// caller keeps the raw value in Extra verbatim (positions-dropping regression
// guard). Covers: short tuple, nil, and a completely wrong type.
func TestWireToPositionsMalformed(t *testing.T) {
	if got := wireToPositions([][]any{{1, 2, 3, 4}}, false); got != nil {
		t.Fatalf("4-tuple absolute = %#v, want nil", got)
	}
	if got := wireToPositions([][]float64{{1, 2}}, true); got != nil {
		t.Fatalf("2-tuple rowcolumn = %#v, want nil", got)
	}
	if got := wireToPositions(nil, false); got != nil {
		t.Fatalf("nil = %#v, want nil", got)
	}
	if got := wireToPositions("not a slice", false); got != nil {
		t.Fatalf("string = %#v, want nil", got)
	}
}

// SpreadsheetPositions is the producer-explicit helper (office_table_render):
// it builds RowColumnBoxes without relying on the legacy-map inference. A mixed
// Positions slice must render each element through its own Wire().
func TestSpreadsheetPositions(t *testing.T) {
	got := SpreadsheetPositions([][]float64{{2, 0, 5, 1, 3}, {2, 6, 9, 1, 3}})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	rb := got[0].(RowColumnBox)
	if rb.Sheet != 2 || rb.R0 != 0 || rb.R1 != 5 || rb.C0 != 1 || rb.C1 != 3 {
		t.Fatalf("RowColumnBox = %+v", rb)
	}
	mixed := Positions{
		AbsoluteBox{PageNumber: 1, Left: 1, Top: 1, Right: 2, Bottom: 2},
		RowColumnBox{Sheet: 2, R0: 0, R1: 1, C0: 0, C1: 1},
	}
	out := positionsToWire(mixed)
	want := []any{[]any{1, 1.0, 2.0, 1.0, 2.0}, []any{2, 0, 1, 0, 1}}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("mixed positionsToWire = %#v, want %#v", out, want)
	}
}

// hasSheetIndex picks the positions vocabulary for the in-bound conversion. It
// MUST mirror the downstream isSpreadsheetChunk (task/indexdoc/process.go)
// exactly — same criteria — so the parser's classification and the indexer's
// spreadsheet/PDF decision can never diverge: sheet_index present and non-nil,
// OR a non-empty "sheet" string. A present-but-nil sheet_index is NOT a
// spreadsheet signal, and the speculative capital "SheetIndex" key is not one
// either (isSpreadsheetChunk does not read it).
func TestHasSheetIndex_MirrorsIsSpreadsheetChunk(t *testing.T) {
	cases := []struct {
		name string
		m    map[string]any
		want bool
	}{
		{"sheet_index int", map[string]any{"sheet_index": 1}, true},
		{"sheet_index zero", map[string]any{"sheet_index": 0}, true},
		{"sheet_index nil", map[string]any{"sheet_index": nil}, false},
		{"sheet only", map[string]any{"sheet": "Sheet1"}, true},
		{"sheet empty", map[string]any{"sheet": ""}, false},
		{"both present", map[string]any{"sheet_index": 2, "sheet": "S"}, true},
		{"neither", map[string]any{}, false},
		{"capital SheetIndex ignored", map[string]any{"SheetIndex": 1}, false},
	}
	for _, tc := range cases {
		if got := hasSheetIndex(tc.m); got != tc.want {
			t.Errorf("%s: hasSheetIndex(%v) = %v, want %v", tc.name, tc.m, got, tc.want)
		}
	}
}
