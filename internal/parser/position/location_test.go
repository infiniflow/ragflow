package position

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParserLocationsKeepCoordinateVocabulary(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input map[string]any
		sheet bool
		want  [][]float64
	}{
		{"sheet", map[string]any{"sheet": "Data", "sheet_index": 2, "positions": [][]float64{{2, 42, 42, 1, 3}}}, true, [][]float64{{2, 42, 42, 1, 3}}},
		{"sheet name only", map[string]any{"sheet": "Data", "positions": [][]float64{{2, 42, 42, 1, 3}}}, true, [][]float64{{2, 42, 42, 1, 3}}},
		{"page only", map[string]any{"positions": []float64{3, 0, 0, 0, 0}}, false, [][]float64{{3, 0, 0, 0, 0}}},
		{"bad tuple", map[string]any{"positions": [][]float64{{3, 1, 2}}}, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := FromParserMap(tc.input)
			if l.IsSpreadsheet() != tc.sheet {
				t.Fatalf("spreadsheet=%v", l.IsSpreadsheet())
			}
			if !reflect.DeepEqual(l.IndexMatrix(), tc.want) {
				t.Fatalf("index=%v, want %v", l.IndexMatrix(), tc.want)
			}
		})
	}
}

func TestPageLocationKeepsDistinctIndexAndPreview(t *testing.T) {
	l := FromParserMap(map[string]any{"positions": [][]float64{{2, 10, 20, 30, 40}}, "_pdf_positions": [][]float64{{3, 50, 60, 70, 80}}})
	if !reflect.DeepEqual(l.IndexMatrix(), [][]float64{{2, 10, 20, 30, 40}}) || !reflect.DeepEqual(l.Preview(), []Box{{3, 50, 60, 70, 80}}) {
		t.Fatalf("location=%+v", l.Page)
	}
	previewOnly := FromParserMap(map[string]any{"_pdf_positions": [][]float64{{3, 50, 60, 70, 80}}})
	if len(previewOnly.IndexMatrix()) != 0 || len(previewOnly.Preview()) != 1 {
		t.Fatal("preview geometry changed index semantics")
	}
}

func TestLocationSliceKeepsSheetRangesDiscrete(t *testing.T) {
	l := FromParserMap(map[string]any{"sheet_index": 1, "positions": [][]float64{{1, 42, 42, 1, 3}}})
	if got := l.SlicePage(0, .5).IndexMatrix(); !reflect.DeepEqual(got, [][]float64{{1, 42, 42, 1, 3}}) {
		t.Fatalf("sheet slice=%v", got)
	}
	page := FromParserMap(map[string]any{"positions": [][]float64{{1, 10, 20, 0, 100}, {2, 10, 20, 0, 100}}})
	if got := page.SlicePage(.25, .75).IndexMatrix(); !reflect.DeepEqual(got, [][]float64{{1, 10, 20, 50, 100}, {2, 10, 20, 0, 50}}) {
		t.Fatalf("page slice=%v", got)
	}
}

func TestLocationCloneAndRowSelectionDoNotAliasSource(t *testing.T) {
	l := FromParserMap(map[string]any{"sheet": "Data", "sheet_index": 1, "positions": [][]float64{{1, 1, 1, 1, 2}, {1, 42, 42, 1, 2}}})
	row := l.SelectRows([]int{1})
	row.Spreadsheet.Ranges[0].RowStart = 7
	*row.Spreadsheet.Index = 2
	if *l.Spreadsheet.Index != 1 || l.Spreadsheet.Ranges[1].RowStart != 42 {
		t.Fatal("row selection mutated source")
	}
	identity := l.SelectRows(nil)
	if !identity.IsSpreadsheet() || identity.HasCoordinates() {
		t.Fatal("dropping ranges lost identity")
	}
}

func TestLocationRoundTripAndConflict(t *testing.T) {
	l := FromParserMap(map[string]any{"sheet": "Data", "sheet_index": 1, "positions": [][]float64{{1, 42, 42, 1, 2}}})
	b, err := json.Marshal(map[string]any{"source_location": l})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err = json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	back, err := FromMap(m)
	if err != nil || !reflect.DeepEqual(back, l) {
		t.Fatalf("round trip=%+v err=%v", back, err)
	}
	if _, err = FromMap(map[string]any{"source_location": map[string]any{"page": map[string]any{}, "spreadsheet": map[string]any{}}}); err == nil {
		t.Fatal("mixed vocabulary accepted")
	}
}

func TestPageLocationAppendAndMergePreserveStrategy(t *testing.T) {
	a := FromParserMap(map[string]any{"positions": [][]float64{{2, 10, 20, 30, 40}}})
	b := FromParserMap(map[string]any{"positions": [][]float64{{1, 10, 20, 30, 40}, {2, 10, 20, 30, 40}}})
	if got := a.Append(b).IndexMatrix(); !reflect.DeepEqual(got, [][]float64{{2, 10, 20, 30, 40}, {1, 10, 20, 30, 40}, {2, 10, 20, 30, 40}}) {
		t.Fatalf("append=%v", got)
	}
	if got := a.MergePage(b).IndexMatrix(); !reflect.DeepEqual(got, [][]float64{{1, 10, 20, 30, 40}, {2, 10, 20, 30, 40}}) {
		t.Fatalf("merge=%v", got)
	}
}

func TestAppendDoesNotModifyOrAliasSources(t *testing.T) {
	a := FromParserMap(map[string]any{"positions": [][]float64{{1, 0, 10, 0, 10}}})
	b := FromParserMap(map[string]any{"positions": [][]float64{{2, 0, 10, 0, 10}}})
	var dst *Location
	dst = dst.Append(a).Append(b)
	dst.Page.IndexBoxes[0].Top = 7
	dst.Page.IndexBoxes[1].Top = 8
	if a.Page.IndexBoxes[0].Top != 0 || b.Page.IndexBoxes[0].Top != 0 {
		t.Fatal("append modified source geometry")
	}
}

func TestIncompatibleMergeCannotAcquirePartialLocation(t *testing.T) {
	a := FromParserMap(map[string]any{"sheet_index": 1, "positions": [][]float64{{1, 42, 42, 1, 3}}})
	b := FromParserMap(map[string]any{"sheet_index": 2, "positions": [][]float64{{2, 7, 7, 1, 3}}})
	dst := a.Clone().Append(b).Append(a)
	if dst.HasCoordinates() {
		t.Fatal("incompatible merge recovered a partial selection")
	}
}

func TestCanonicalSpreadsheetLocationRejectsConflictingSheetIdentity(t *testing.T) {
	for _, raw := range []string{
		`{"spreadsheet":{"index":1,"ranges":[{"sheet_index":2,"row_start":42,"row_end":42,"col_start":1,"col_end":3}]}}`,
		`{"spreadsheet":{"sheet":"Orders","ranges":[{"sheet_index":1,"row_start":1,"row_end":1,"col_start":1,"col_end":3},{"sheet_index":2,"row_start":7,"row_end":7,"col_start":1,"col_end":3}]}}`,
	} {
		if _, err := FromMap(map[string]any{"source_location": json.RawMessage(raw)}); err == nil {
			t.Fatal("mixed sheet identity accepted")
		}
	}
}
