package parser

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestPDFPositionRowsFromJSONShapes pins the wire shapes the chunker ordering
// and crop-region checks rely on: scalar and list page cells, a single flat
// 5-tuple, and a flat sequence of multiple 5-tuples grouped by five.
// Malformed input reports no rows instead of misaligned keys.
func TestPDFPositionRowsFromJSONShapes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want [][]float64
	}{
		{"scalar matrix", `[[1,10,100,20,80],[2,30,120,40,90]]`, [][]float64{{1, 10, 100, 20, 80}, {2, 30, 120, 40, 90}}},
		{"list page cell", `[[[1],10,100,10,100]]`, [][]float64{{1, 10, 100, 10, 100}}},
		{"flat single tuple", `[2,5,50,5,50]`, [][]float64{{2, 5, 50, 5, 50}}},
		{"flat multi tuple", `[1,10,100,20,80,2,30,120,40,90]`, [][]float64{{1, 10, 100, 20, 80}, {2, 30, 120, 40, 90}}},
		{"malformed flat", `[1,10,100,20,80,2,30]`, nil},
		{"non numeric row skipped", `[[1,"bad",100,10,100],[2,5,50,5,50]]`, [][]float64{{2, 5, 50, 5, 50}}},
		{"empty", ``, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PDFPositionRowsFromJSON(json.RawMessage(tc.raw))
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("PDFPositionRowsFromJSON(%s) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}
