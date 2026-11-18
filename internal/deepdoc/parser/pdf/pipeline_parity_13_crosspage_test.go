//go:build cgo && manual

package pdf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/deepdoc/parser/pdf/tool"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// TestPipelineParity13CrosspageSeam is the focused, cell-level counterpart to
// the harness divergence for 13_crosspage_table.pdf (tracked as go_bug rule
// table-crosspage-merge-seam-duplication in
// table/testdata/parity/known_diffs.json, now resolved).
//
// 13 is a multi-page table: Python's golden is ONE 81x5 grid. The divergence is
// NOT the cross-page merge — each per-page grid is correct and MergeTablesAcrossPages
// (table/table_merge.go) stacks them correctly. The real cause was
// FillCellTextFromBoxesWithRows (table/table_cells.go) picking the row with the
// MAX 2D-overlap, which placed a box spanning two adjacent data rows in the
// LOWER row. Python's construct_table groups boxes into rows by each box's TSR
// row label b["R"] — the row the box STARTS in — so the same box lands in the
// UPPER row (pdf_parser.py construct_table:176-192).
//
// Concretely the cell-level diff showed, at the page seam, Go emitting
//
//	c0 = "2024-06 2024-07 2024-07"   (3 tokens) one row too low
//
// while Python emits a single cell
//
//	c0 = "2024-06 2024-07"           (2 tokens)
//
// The fix prefers the topmost row band whose Y range CONTAINS the box's TOP edge
// (top-containment), matching Python's R-grouping. The residual
// leading/trailing-whitespace diffs are normalized by the harness gridSim guard
// and are not the real bug; after the fix the dominant divergence (seam
// duplication) is gone and gridSim=100.0%.
//
// This test asserts the seam-duplication bug signature (a Go cell whose
// whitespace-split tokens contain a repeated token, e.g. "2024-07 2024-07") is
// absent AND gridSim==100. It is the green regression guard for rule
// table-crosspage-merge-seam-duplication; if FillCellTextFromBoxesWithRows
// regresses to max-overlap row selection, this test fails again.
func TestPipelineParity13CrosspageSeam(t *testing.T) {
	name := "13_crosspage_table.pdf"
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
	// replayed TSR intermediates (not a fresh analysis). Without this the
	// parser falls back to the production builder and result.Tables is empty.
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
	var seamDup []cellDiff
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
				seamDup = append(seamDup, d)
			} else {
				// goVal is already TrimSpace'd by goTableRows; a cell whose
				// Python value only differs by surrounding whitespace is not a
				// content gap — gridSim (CharSimilarity, whitespace/order
				// insensitive) below 100 is the content-parity guard below.
				otherCount++
			}
			t.Logf("ROW %d c%d: GO=%q PY=%q", i, c, gc, pyc)
		}
	}
	t.Logf("13_crosspage_table seam-duplication cells=%d other-diff cells=%d gridSim=%.1f%%",
		len(seamDup), otherCount, gridSim)

	// Regression guard: the seam-duplication signature must not exist, and the
	// grid must retain full content parity (gridSim==100). A partial fix that
	// drops or adds cell content drops gridSim below 100 even if no
	// repeated-token cell remains.
	if len(seamDup) > 0 || gridSim < 100 {
		ex := "no cells"
		if len(seamDup) > 0 {
			ex = fmt.Sprintf("e.g. %q vs Python %q", seamDup[0].goVal, seamDup[0].pyVal)
		}
		t.Errorf("REGRESSION table-crosspage-merge-seam-duplication (resolved): %d Go cells contain a duplicated seam token, gridSim=%.1f%% (%s). "+
			"FillCellTextFromBoxesWithRows must place a straddling box in the topmost row band containing its top edge (top-containment), matching Python's R-grouping, not max-overlap.",
			len(seamDup), gridSim, ex)
	}
}

// hasRepeatedToken reports whether s, split on whitespace, contains the same
// non-empty token twice consecutively — the signature of a box placed one row
// too low by FillCellTextFromBoxesWithRows, duplicating a value across the
// seam (e.g. "2024-07 2024-07").
func hasRepeatedToken(s string) bool {
	toks := strings.Fields(s)
	for i := 1; i < len(toks); i++ {
		if toks[i] != "" && toks[i] == toks[i-1] {
			return true
		}
	}
	return false
}

func rowOrEmpty(grid [][]string, i int) []string {
	if i < len(grid) {
		return grid[i]
	}
	return nil
}

func cellOrEmpty(row []string, c int) string {
	if c < len(row) {
		return row[c]
	}
	return ""
}
