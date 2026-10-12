package chunker

import (
	"encoding/json"
	"math"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

func decodeGeneralMatrix(t *testing.T, raw json.RawMessage) [][]float64 {
	t.Helper()
	var m [][]float64
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	return m
}

func TestSplitGeneralUnitsSlicesPositionsAbut(t *testing.T) {
	unit := schema.ChunkDoc{
		Text:         "第一句。第二句。",
		DocType:      "text",
		PDFPositions: json.RawMessage(`[[1,10,200,0,100]]`),
		Positions:    json.RawMessage(`[[1,10,200,0,100]]`),
	}
	pattern := compileDelimPattern([]string{"。"})
	got := splitGeneralUnits([]schema.ChunkDoc{unit}, pattern, false)
	if len(got) != 2 {
		t.Fatalf("split pieces = %d, want 2 (%q)", len(got), generalChunkTexts(got))
	}
	first := decodeGeneralMatrix(t, got[0].PDFPositions)
	second := decodeGeneralMatrix(t, got[1].PDFPositions)
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("single-row input must yield single-row slices: %v / %v", first, second)
	}
	if first[0][3] != 0 || second[0][4] != 100 {
		t.Errorf("outer bounds changed: %v / %v", first[0], second[0])
	}
	if math.Abs(first[0][4]-second[0][3]) > posSliceEpsilon {
		t.Errorf("adjacent General slices must abut: first bottom=%v second top=%v", first[0][4], second[0][3])
	}
	if math.Abs(first[0][4]-50) > 1e-6 {
		t.Errorf("equal 3+3 rune split must cut at mid height 50, got bottom=%v", first[0][4])
	}
}

func TestSplitGeneralChildrenSlicesPositionsAbut(t *testing.T) {
	chunk := schema.ChunkDoc{
		Text:         "aaa;bbb",
		DocType:      "text",
		CKType:       "text",
		PDFPositions: json.RawMessage(`[[1,10,20,0,100]]`),
		Positions:    json.RawMessage(`[[1,10,20,0,100]]`),
	}
	pattern := compileChildrenPattern([]string{";"})
	got := splitGeneralChildren([]schema.ChunkDoc{chunk}, pattern)
	if len(got) != 2 {
		t.Fatalf("children pieces = %d, want 2", len(got))
	}
	first := decodeGeneralMatrix(t, got[0].PDFPositions)
	second := decodeGeneralMatrix(t, got[1].PDFPositions)
	if math.Abs(first[0][4]-second[0][3]) > posSliceEpsilon {
		t.Errorf("adjacent children slices must abut: %v / %v", first[0], second[0])
	}
	if first[0][4] == 100 && second[0][3] == 0 {
		t.Errorf("children slices still carry the full box: %v / %v", first[0], second[0])
	}
}
