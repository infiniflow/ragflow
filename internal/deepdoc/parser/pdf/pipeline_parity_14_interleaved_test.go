//go:build cgo && manual

package pdf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"ragflow/internal/deepdoc/parser/pdf/tool"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// TestPipelineParity14TextTableInterleaved is the regression guard for the
// 14_text_table_interleaved.pdf go_bug table-cell-fill-pre-vs-post-merge-order
// (table/testdata/parity/known_diffs.json — RESOLVED by this PR).
//
// The bug: processOneTable pre-merged the table's OCR boxes with
// NaiveVerticalMerge before cell fill, and a strictly-NESTED re-detected box
// ('Hardware' fully inside 'Software Hardware') was CONCATENATED by
// mergeTwoBoxes into 'Software Hardware Hardware'; Python keeps the longer
// box only. Fixed by dedupNestedBoxes (table_extract.go), which drops a box
// strictly nested inside another box whose trimmed text contains it, before
// the merge.
//
// The bug signature is a Go cell whose whitespace-split tokens contain a
// repeated token (e.g. 'Hardware Hardware'), which hasRepeatedToken detects.
// This test asserts that signature is absent AND that no NON-whitespace cell
// difference remains (14 now reaches gridSim=100%, i.e. cell char-multiset
// parity; whitespace-only diffs are tolerated because Python's golden keeps
// surrounding spaces that the Go pipeline trims).
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
	otherCount := 0
	gridSim := tool.CharSimilarity(joinGrid(goRows), joinGrid(pyRows))
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
				// goVal is already TrimSpace'd by goTableRows; a cell whose
				// Python value only differs by surrounding whitespace (or a
				// row-index shift that preserves the char multiset) is not a
				// content gap — gridSim (CharSimilarity, whitespace/order
				// insensitive) below 100 is the content-parity guard below.
				otherCount++
			}
			t.Logf("ROW %d c%d: GO=%q PY=%q", i, c, gc, pyc)
		}
	}
	t.Logf("14_text_table_interleaved dup-token cells=%d other-diff cells=%d gridSim=%.1f%%",
		len(dup), otherCount, gridSim)

	// Regression guard: the overlapping-box concatenation signature must not
	// exist, and the grid must retain full content parity (gridSim==100). A
	// partial fix that drops or adds cell content drops gridSim below 100 even
	// if no repeated-token cell remains.
	if len(dup) > 0 || gridSim < 100 {
		ex := "no cells"
		if len(dup) > 0 {
			ex = fmt.Sprintf("e.g. %q vs Python %q", dup[0].goVal, dup[0].pyVal)
		}
		t.Errorf("table-cell-fill-pre-vs-post-merge-order regression: %d Go cells contain a duplicated token from overlapping-box concatenation, gridSim=%.1f%% (%s). "+
			"Go must merge overlapping OCR boxes before cell fill, matching Python's construct_table order.",
			len(dup), gridSim, ex)
	}
}
