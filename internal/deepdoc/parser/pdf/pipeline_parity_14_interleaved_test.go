//go:build cgo && manual

package pdf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ragflow/internal/deepdoc/parser/pdf/tool"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// TestPipelineParity14TextTableInterleaved exposes the table-cell-fill
// pipeline-order divergence for 14_text_table_interleaved.pdf (tracked as
// go_bug rule table-cell-fill-pre-vs-post-merge-order in
// table/testdata/parity/known_diffs.json).
//
// Unlike 13 (a cross-PAGE seam duplication in MergeTablesAcrossPages), 14 is a
// SINGLE-PAGE divergence rooted in the ORDER of Go's table assembly:
//
//   - Go: processOneTable runs FillCellTextFromBoxes on the RAW OCR boxes
//     BEFORE buildLayout's NaiveVerticalMerge, so two vertically-overlapping
//     OCR boxes ('Software Hardware' y=270-300 and 'Hardware' y=287.7-300)
//     both match the same row band and their text is CONCATENATED into one
//     cell: 'Software Hardware Hardware'.
//   - Python: construct_table fills cells AFTER _naive_vertical_merge, so the
//     overlapping boxes are merged first and the cell becomes 'Software
//     Hardware'.
//
// The bug signature is a Go cell whose whitespace-split tokens contain a
// repeated token (e.g. 'Hardware Hardware'), which hasRepeatedToken detects.
// This test asserts that signature is absent; it currently FAILS (14 is a
// lockedGridPDF, gridSim=97.5%), documenting the open go_bug, and becomes a
// green regression guard once Go merges overlapping OCR boxes before cell
// fill (matching Python's pipeline order).
func TestPipelineParity14TextTableInterleaved(t *testing.T) {
	name := "14_text_table_interleaved.pdf"
	base := filepath.Join("testdata", "output", "py", "ocr")
	charspyDir := filepath.Join("testdata", "charspy")
	dlaDir := filepath.Join(base, "dla")
	tsrDir := filepath.Join(base, "tsr_raw")
	tablesDir := filepath.Join(base, "tables")
	ocrDir := filepath.Join(base, "ocr")

	engine, err := tool.LoadPythonChars(filepath.Join(charspyDir, name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	// Mirror the harness: English documents clear chars and fall through to
	// the OCR-replay path so both sides replay the same input.
	isEnglish := false
	if v := engine.IsEnglish(); v != nil {
		isEnglish = *v
	} else if pages, _ := engine.PageCount(); util.DetectEnglish(engine.PageChars(), pages, nil) {
		isEnglish = true
	}
	if isEnglish {
		engine.ClearChars()
	}

	// Register the replay TableBuilder so Go builds its grid from Python's
	// replayed TSR intermediates (not a fresh analysis).
	RegisterReplayTableBuilder()
	cfg := pdf.DefaultParserConfig()
	cfg.SortByTop = true
	analyzer := NewPythonIntermediateDocAnalyzer(name, dlaDir, tsrDir, ocrDir, engine.PageDims())
	p := NewParser(cfg)
	result, err := p.ParseRaw(t.Context(), engine, analyzer)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(tablesDir, name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var td pyTableDump
	if err := json.Unmarshal(raw, &td); err != nil {
		t.Fatal(err)
	}

	goRows := goTableRows(result)
	pyRows, pyHasTables := loadPythonTables(t, filepath.Join(tablesDir, name+".json"))
	if !pyHasTables {
		t.Fatal("Python golden has no tables")
	}

	// Log the full cell-level diff so the divergence is fully exposed.
	n := len(goRows)
	if len(pyRows) > n {
		n = len(pyRows)
	}
	type cellDiff struct {
		row, col     int
		goVal, pyVal string
	}
	var dup []cellDiff
	var other []cellDiff
	for i := 0; i < n; i++ {
		g := rowOrEmpty(goRows, i)
		py := rowOrEmpty(pyRows, i)
		cn := len(g)
		if len(py) > cn {
			cn = len(py)
		}
		for c := 0; c < cn; c++ {
			gc := cellOrEmpty(g, c)
			pyc := cellOrEmpty(py, c)
			if gc == pyc {
				continue
			}
			d := cellDiff{i, c, gc, pyc}
			if hasRepeatedToken(gc) {
				dup = append(dup, d)
			} else {
				other = append(other, d)
			}
			t.Logf("ROW %d c%d: GO=%q PY=%q", i, c, gc, pyc)
		}
	}
	t.Logf("14_text_table_interleaved dup-token cells=%d other-diff cells=%d gridSim=%.1f%%",
		len(dup), len(other), tool.CharSimilarity(joinGrid(goRows), joinGrid(pyRows)))

	// Regression guard: the overlapping-box concatenation signature must not
	// exist.
	if len(dup) > 0 {
		t.Errorf("OPEN go_bug table-cell-fill-pre-vs-post-merge-order: %d Go cells contain a duplicated token from overlapping-box concatenation "+
			"(e.g. %q vs Python %q). Go must merge overlapping OCR boxes before cell fill, matching Python's construct_table order.",
			len(dup), dup[0].goVal, dup[0].pyVal)
	}
}
