//go:build cgo && manual

package pdf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/deepdoc/parser/pdf/tool"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// TestTableRowSegmentationParity pins the table-row-segmentation divergence
// between Go and Python that the parity sweep reports as a FAIL (B-class):
// Go builds table rows from TSR structural "table row" lines (cross-product
// grid + empty/orphan cleanup), while Python counts rows from the per-char
// R/C labels TSR assigns to each text box. Both views come from the SAME TSR
// model output, so a faithful assembly should agree on the row count.
//
// Representative case (ocr_real): 说明书.pdf → Go assembles 81 grid rows,
// Python golden has 59. Go over-segments because it keeps TSR "table row"
// lines that Python's R/C view collapses.
//
// This is the minimal failing test for the row-segmentation fix: make Go's
// row count match Python's (by aligning Go's segmentation with the per-char
// R/C assignment, the authoritative TSR output Python uses).
func TestTableRowSegmentationParity(t *testing.T) {
	// Single-PDF focus. Override via BATCH_PARITY_ROWPDF to pin other cases.
	name := common.GetEnv("BATCH_PARITY_ROWPDF")
	if name == "" {
		name = "说明书.pdf"
	}

	dirs := tool.ParityDirsFor(common.GetEnv(common.EnvBatchParityVariant))
	charspyDir, pyTextDir := dirs.Charspy, dirs.Text
	dlaDir, tsrDir, ocrDir, tablesDir := dirs.DLA, dirs.TSRRaw, dirs.OCR, dirs.Tables

	// A documented go_intentional divergence (known_diffs.json) exempts a PDF
	// from the row-count assertion: its row/structure gap is deliberate (e.g.
	// qa.pdf's table-span-follow-tsr-geometry keeps Go's span-merged rows
	// instead of Python's flat grid), so the row-seg fix does not target it.
	for _, r := range loadPdfKnownDiffs(t) {
		if r.Tag == "go_intentional" && r.matchesExactly(name) {
			t.Skipf("%s: go_intentional (%s) — row divergence is deliberate, not a row-seg target", name, r.ID)
		}
	}

	// Replay Python's DLA + TSR intermediates through Go's assembly.
	RegisterReplayTableBuilder()

	jsonPath := filepath.Join(charspyDir, name+".json")
	engine, err := tool.LoadPythonChars(jsonPath)
	if err != nil {
		t.Skipf("charspy dump missing for %s: %v", name, err)
	}

	isEnglish := false
	if v := engine.IsEnglish(); v != nil {
		isEnglish = *v
	} else if pages, _ := engine.PageCount(); util.DetectEnglish(engine.PageChars(), pages, nil) {
		isEnglish = true
	}
	if isEnglish {
		engine.ClearChars()
	}

	cfg := pdf.DefaultParserConfig()
	cfg.SortByTop = true
	analyzer := NewPythonIntermediateDocAnalyzer(name, dlaDir, tsrDir, ocrDir, engine.PageDims())
	p := NewParser(cfg)
	result, err := p.ParseRaw(t.Context(), engine, analyzer)
	if err != nil {
		t.Fatalf("%s: parse: %v", name, err)
	}

	// Production path derives R/C itself; Go's grid row count is measured
	// directly against Python's golden row count.

	// Python golden row grid (authoritative row count).
	pyRows, pyHasTables := loadPythonTables(t, filepath.Join(tablesDir, name+".json"))
	goRows := goTableRows(result)

	if common.GetEnv("BATCH_PARITY_DEBUG") != "" {
		t.Logf("DEBUG per-table Go grids:")
		for ti, tbl := range result.Tables {
			t.Logf("  Go table[%d] page=%d rows=%d", ti, tbl.Page, len(tbl.Grid))
		}
	}

	pyPath := filepath.Join(pyTextDir, name+".txt")
	if _, err := os.Stat(pyPath); err != nil {
		t.Skipf("no Python text golden at %s", pyPath)
	}

	t.Logf("%s: Go assembled %d table rows, Python golden %d table rows (pyHasTables=%v)",
		name, len(goRows), len(pyRows), pyHasTables)

	if common.GetEnv("BATCH_PARITY_DEBUG") != "" {
		// Cell-level text diff between the R/C-rebuilt Go grid and Python's
		// golden, for diagnosing residual gridSim<100% text gaps.
		maxRows := len(goRows)
		if len(pyRows) > maxRows {
			maxRows = len(pyRows)
		}
		for ri := 0; ri < maxRows; ri++ {
			var g, p []string
			if ri < len(goRows) {
				g = goRows[ri]
			}
			if ri < len(pyRows) {
				p = pyRows[ri]
			}
			if strings.Join(g, "|") != strings.Join(p, "|") {
				t.Logf("  row %d: Go=%v Py=%v", ri, g, p)
			}
		}
	}

	if pyHasTables && len(goRows) != len(pyRows) {
		t.Errorf("ROW SEGMENTATION MISMATCH %s: Go=%d rows, Python=%d rows — "+
			"Go's TSR-line cross-product segmentation diverges from Python's per-char R/C row count",
			name, len(goRows), len(pyRows))
	}
}
