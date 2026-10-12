package parser

import "testing"

func TestSpreadsheetSelectionIsNotAPDFCropRegion(t *testing.T) {
	for _, item := range []map[string]any{
		{"sheet_index": 2, "positions": [][]any{{2, 42, 50, 1, 3}}},
		{"sheet": "Orders", "positions": []any{[]any{2, 42, 50, 1, 3}}},
	} {
		if matrix, ok := ExtractPDFPositions(item); ok || len(matrix) > 0 {
			t.Fatalf("sheet selection treated as PDF region: %v", matrix)
		}
	}
}
