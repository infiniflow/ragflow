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
		PDFPositions: json.RawMessage(`[[1,10,20,0,100]]`),
		Positions:    json.RawMessage(`[[1,10,20,0,100]]`),
	}
	pattern := compileDelimPattern([]string{";"})
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

func TestSplitGeneralUnitsKeepDelimiterSlicesPositions(t *testing.T) {
	unit := schema.ChunkDoc{
		Text:         "aaa。bbb。",
		DocType:      "text",
		PDFPositions: json.RawMessage(`[[1,10,200,0,100]]`),
		Positions:    json.RawMessage(`[[1,10,200,0,100]]`),
	}
	pattern := compileDelimPattern([]string{"。"})
	got := splitGeneralUnits([]schema.ChunkDoc{unit}, pattern, true)
	if len(got) != 2 {
		t.Fatalf("split pieces = %d, want 2 (%q)", len(got), generalChunkTexts(got))
	}
	if got[0].Text != "aaa。" || got[1].Text != "bbb。" {
		t.Errorf("keepDelimiter must retain delimiters, got %q", generalChunkTexts(got))
	}
	first := decodeGeneralMatrix(t, got[0].PDFPositions)
	second := decodeGeneralMatrix(t, got[1].PDFPositions)
	if math.Abs(first[0][4]-second[0][3]) > posSliceEpsilon {
		t.Errorf("keepDelimiter slices must abut: first bottom=%v second top=%v", first[0][4], second[0][3])
	}
}

func TestSplitGeneralUnitsMultiRowSplitAcrossPages(t *testing.T) {
	unit := schema.ChunkDoc{
		Text:         "上。下。",
		DocType:      "text",
		PDFPositions: json.RawMessage(`[[1,10,20,0,10],[2,10,20,10,20]]`),
		Positions:    json.RawMessage(`[[1,10,20,0,10],[2,10,20,10,20]]`),
	}
	pattern := compileDelimPattern([]string{"。"})
	got := splitGeneralUnits([]schema.ChunkDoc{unit}, pattern, false)
	if len(got) != 2 {
		t.Fatalf("split pieces = %d, want 2 (%q)", len(got), generalChunkTexts(got))
	}
	first := decodeGeneralMatrix(t, got[0].PDFPositions)
	second := decodeGeneralMatrix(t, got[1].PDFPositions)
	if len(first) != 1 || first[0][0] != 1 || first[0][3] != 0 || first[0][4] != 10 {
		t.Errorf("first piece must cover page-1 row exactly, got %v", first)
	}
	if len(second) != 1 || second[0][0] != 2 || second[0][3] != 10 || second[0][4] != 20 {
		t.Errorf("second piece must cover page-2 row exactly, got %v", second)
	}
}

func TestSplitGeneralUnitsMalformedMatrixKeepsFullBox(t *testing.T) {
	raw := json.RawMessage(`[[1,10,20,30,30]]`)
	unit := schema.ChunkDoc{
		Text:         "第一句。第二句。",
		DocType:      "text",
		PDFPositions: raw,
		Positions:    raw,
	}
	pattern := compileDelimPattern([]string{"。"})
	got := splitGeneralUnits([]schema.ChunkDoc{unit}, pattern, false)
	if len(got) != 2 {
		t.Fatalf("split pieces = %d, want 2", len(got))
	}
	for i, piece := range got {
		if string(piece.PDFPositions) != string(raw) || string(piece.Positions) != string(raw) {
			t.Errorf("piece %d must keep the source box on malformed input, got %s / %s",
				i, piece.PDFPositions, piece.Positions)
		}
	}
}

func TestSplitGeneralUnitsTagOnlyPieceKeepsFullBox(t *testing.T) {
	// Contract lock (see TestSlicePositionsByCounts_ZeroWeightPieceKeepsFullBox):
	// a tag-only segment between delimiters has no visible height yet keeps
	// the full source box so it retains its preview. Changing this to empty
	// positions requires checking ImgID-carrying media first.
	raw := json.RawMessage(`[[1,10,200,0,100]]`)
	unit := schema.ChunkDoc{
		Text:         "A。@@1\t10\t20\t30\t40##。B",
		DocType:      "text",
		PDFPositions: raw,
		Positions:    raw,
	}
	pattern := compileDelimPattern([]string{"。"})
	got := splitGeneralUnits([]schema.ChunkDoc{unit}, pattern, false)
	if len(got) != 3 {
		t.Fatalf("split pieces = %d, want 3 (%q)", len(got), generalChunkTexts(got))
	}
	if string(got[1].PDFPositions) != string(raw) {
		t.Errorf("tag-only piece must keep the full box, got %s", got[1].PDFPositions)
	}
	first := decodeGeneralMatrix(t, got[0].PDFPositions)
	third := decodeGeneralMatrix(t, got[2].PDFPositions)
	if math.Abs(first[0][4]-50) > 1e-6 || math.Abs(third[0][3]-50) > 1e-6 {
		t.Errorf("visible pieces must split around the tag-only segment, got %v / %v", first, third)
	}
}
