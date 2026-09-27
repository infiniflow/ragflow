//go:build cgo && manual

package pdf

import (
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/deepdoc/parser/pdf/tool"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestCrossPageMerge_HeaderRowKeepsColumns asserts that a cross-page merged
// table's header row keeps every column distinct: a cell that contains BOTH
// "类型" and "必选" means the two adjacent header columns were merged into
// one (spurious span folding or orphan-column merge inside ConstructTable),
// which inflates nothing but narrows the row and breaks grid structure
// against Python.
//
// RED for icbccs: Go renders the page5+page6 merged table's header as
// "类型 必选" in a single cell while Python keeps "类型" and "必选" apart.
func TestCrossPageMerge_HeaderRowKeepsColumns(t *testing.T) {
	name := "icbccs deployment.pdf"
	dirs := tool.ParityDirsFor("ocr_real")
	engine, err := tool.LoadPythonChars(filepath.Join(dirs.Charspy, name+".json"))
	if err != nil {
		t.Skipf("charspy dump missing: %v", err)
	}
	RegisterReplayTableBuilder()
	cfg := pdf.DefaultParserConfig()
	cfg.SortByTop = true
	analyzer := NewPythonIntermediateDocAnalyzer(name, dirs.DLA, dirs.TSRRaw, dirs.OCR, engine.PageDims())
	p := NewParser(cfg)
	result, err := p.ParseRaw(t.Context(), engine, analyzer)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	merged := 0
	for ti, tab := range result.Tables {
		pages := map[int]bool{}
		for _, pos := range tab.Positions {
			for _, pn := range pos.PageNumbers {
				pages[pn] = true
			}
		}
		if len(pages) < 2 {
			continue
		}
		merged++
		for ri, row := range tab.Grid {
			for ci, cell := range row {
				if strings.Contains(cell.Text, "类型") && strings.Contains(cell.Text, "必选") {
					t.Errorf("merged table[%d] row %d col %d: cell %q merges 类型+必选 — adjacent header columns must stay distinct", ti, ri, ci, cell.Text)
				}
			}
		}
	}
	if merged == 0 {
		t.Skipf("%s: no cross-page merged table found", name)
	}
}
