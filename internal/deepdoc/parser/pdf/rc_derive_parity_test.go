//go:build cgo && manual

package pdf

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/deepdoc/parser/pdf/tool"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// rcLine is one TSR structural line ("table row" / "table column") in
// page-cumulative PDF points, mirroring Python's self.tb_cpns entries.
type rcLine struct {
	page           int
	layoutno       string
	score          float64
	x0, y0, x1, y1 float64
}

// findHorizontallyTightestFitPy mirrors Python
// Recognizer.find_horizontally_tightest_fit(box, clmns): it returns the index
// of the column with the minimal horizontal edge distance, restricted to
// columns that (1) share the box's layoutno (the per-page table key — a box
// only matches ITS table's columns, never another table's with a similar x
// range) and (2) share vertical extent with the box (page-cumulative Y
// rejects same-layoutno columns on other pages). Distance is
// min(|x0-cx0|, |x1-cx1|, |(x0+x1)-(cx0+cx1)|/2), as in Python.
func findHorizontallyTightestFitPy(box pdf.TextBox, clmns []rcLine) int {
	best, bestDis := -1, 1000000.0
	for i, c := range clmns {
		if c.layoutno != box.LayoutNo {
			continue
		}
		if minf(box.Bottom, c.y1) <= maxf(box.Top, c.y0) {
			continue
		}
		dis := minf(minf(math.Abs(box.X0-c.x0), math.Abs(box.X1-c.x1)), math.Abs(box.X0+box.X1-c.x1-c.x0)/2)
		if dis < bestDis {
			best, bestDis = i, dis
		}
	}
	return best
}

// overlappedAreaPy mirrors Python Recognizer.overlapped_area(a, b, ratio):
// the intersection area of a and b, divided by a's area when ratio is true
// (0 when disjoint).
func overlappedAreaPy(ax0, atop, ax1, abtm, bx0, btop, bx1, bbtm float64, ratio bool) float64 {
	if bx0 > ax1 || bx1 < ax0 {
		return 0
	}
	if bbtm < atop || btop > abtm {
		return 0
	}
	ix0, ix1 := maxf(bx0, ax0), minf(bx1, ax1)
	itop, ibtm := maxf(btop, atop), minf(bbtm, abtm)
	ov := (ibtm - itop) * (ix1 - ix0)
	if ov <= 0 {
		return 0
	}
	if ratio {
		if aArea := (ax1 - ax0) * (abtm - atop); aArea > 0 {
			ov /= aArea
		}
	}
	return ov
}

// rcNotOverlapped reports whether two structure lines are disjoint (Python's
// layouts_cleanup.not_overlapped).
func rcNotOverlapped(a, b rcLine) bool {
	return a.x1 < b.x0 || a.x0 > b.x1 || a.y1 < b.y0 || a.y0 > b.y1
}

// layoutsCleanupPy mirrors Python Recognizer.layouts_cleanup(boxes, layouts,
// far, thr): it collapses a layout line whose neighbor (within far) is
// overlapping and same-typed. When both carry a detection score the
// higher-score line is kept (recognizer.py:141); otherwise the one with the
// larger total overlap against the full page box set survives (area branch).
// This is what turns the raw "table column" lines into the effective column
// set Python's find_horizontally_tightest_fit sees.
func layoutsCleanupPy(lines []rcLine, boxes []pdf.TextBox, far int, thr float64) []rcLine {
	i := 0
	for i+1 < len(lines) {
		j := i + 1
		for j < min(i+far, len(lines)) && rcNotOverlapped(lines[i], lines[j]) {
			j++
		}
		if j >= min(i+far, len(lines)) {
			i++
			continue
		}
		if overlappedAreaPy(lines[i].x0, lines[i].y0, lines[i].x1, lines[i].y1, lines[j].x0, lines[j].y0, lines[j].x1, lines[j].y1, true) < thr &&
			overlappedAreaPy(lines[j].x0, lines[j].y0, lines[j].x1, lines[j].y1, lines[i].x0, lines[i].y0, lines[i].x1, lines[i].y1, true) < thr {
			i++
			continue
		}
		if lines[i].score > 0 && lines[j].score > 0 {
			// Python: higher score survives (score tie keeps lines[i]).
			if lines[i].score > lines[j].score {
				lines = append(lines[:j], lines[j+1:]...)
			} else {
				lines = append(lines[:i], lines[i+1:]...)
			}
			continue
		}
		areaI, areaJ := 0.0, 0.0
		for _, b := range boxes {
			if !rcBoxNotOverlapped(b, lines[i]) {
				areaI += overlappedAreaPy(b.X0, b.Top, b.X1, b.Bottom, lines[i].x0, lines[i].y0, lines[i].x1, lines[i].y1, false)
			}
			if !rcBoxNotOverlapped(b, lines[j]) {
				areaJ += overlappedAreaPy(b.X0, b.Top, b.X1, b.Bottom, lines[j].x0, lines[j].y0, lines[j].x1, lines[j].y1, false)
			}
		}
		if areaI > areaJ {
			lines = append(lines[:j], lines[j+1:]...)
		} else {
			lines = append(lines[:i], lines[i+1:]...)
		}
	}
	return lines
}

func rcBoxNotOverlapped(b pdf.TextBox, l rcLine) bool {
	return b.X1 < l.x0 || b.X0 > l.x1 || b.Bottom < l.y0 || b.Top > l.y1
}

// findOverlappedWithThresholdPy mirrors Python
// Recognizer.find_overlapped_with_threshold(box, lines, thr=0.3): it returns
// the index of the line with the largest (boxRatio, lineRatio) tuple where
// boxRatio >= thr, or -1 when none qualifies. boxRatio is the fraction of the
// BOX covered by the line; lineRatio the fraction of the LINE covered by the
// box. Python gates on boxRatio only and tie-breaks by lineRatio.
func findOverlappedWithThresholdPy(box pdf.TextBox, lines []rcLine, thr float64) int {
	best := -1
	bestOv, bestOv2 := thr, 0.0
	for i, ln := range lines {
		ov := overlappedAreaPy(box.X0, box.Top, box.X1, box.Bottom, ln.x0, ln.y0, ln.x1, ln.y1, true)
		ov2 := overlappedAreaPy(ln.x0, ln.y0, ln.x1, ln.y1, box.X0, box.Top, box.X1, box.Bottom, true)
		if ov < bestOv || (ov == bestOv && ov2 < bestOv2) {
			continue
		}
		best, bestOv, bestOv2 = i, ov, ov2
	}
	return best
}

// TestRCDeriveParity is stage-2a of the table row-segmentation work: it
// verifies that deriving each box's R/C by matching it against the GLOBAL
// TSR "table row"/"table column" structure lines — exactly Python's
// _table_transformer_job (pdf_parser.py:610/624), which feeds
// find_overlapped_with_threshold(box, rows, 0.3) for R and
// find_horizontally_tightest_fit(box, clmns) for C — reproduces the
// authoritative R/C captured in the table_boxes/ dump, 1:1 for every box.
//
// To reproduce the exact derivation the dump must carry two extras beyond the
// per-table boxes: (1) the detection SCORE of every TSR line, because
// layouts_cleanup (recognizer.py:141) keeps the higher-score line when two
// overlap — without it the area branch deletes a different line and the
// global row/column index drifts; and (2) the FULL page box snapshot
// (table_boxes/{name}.all_boxes.json), because the area branch sums overlap
// against self.boxes (every page box, table and non-table).
//
// This is the prerequisite for making Go's PRODUCTION path derive per-char
// R/C the same way Python does (today Go's AnnotateTableBoxes matches boxes
// against the line cross-product grid with max(boxRatio, cellRatio) and no
// layout cleanup, which diverges).
func TestRCDeriveParity(t *testing.T) {
	dirs := tool.ParityDirsFor(common.GetEnv(common.EnvBatchParityVariant))
	entries, err := os.ReadDir(dirs.TableBoxes)
	if err != nil {
		t.Skipf("no table_boxes/ dump: %v", err)
	}

	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".all_boxes.json") {
			continue // snapshot for layouts_cleanup area, not a per-table dump
		}
		name := strings.TrimSuffix(e.Name(), ".json")

		boxes, err := tool.LoadPythonTableBoxes(filepath.Join(dirs.TableBoxes, e.Name()))
		if err != nil {
			t.Errorf("%s: load table_boxes: %v", name, err)
			continue
		}
		// Every page box (table and non-table) at R/C-annotation time — the
		// set Python's layouts_cleanup area branch sums overlap against. When
		// the snapshot is absent the test falls back to the table boxes only,
		// which drifts the cleanup on PDFs with overlapping lines.
		allBoxes := boxes
		if ab, err := tool.LoadPythonAllBoxes(filepath.Join(dirs.TableBoxes, name+".all_boxes.json")); err == nil {
			allBoxes = ab
		}
		tsr, err := tool.LoadPythonTSR(filepath.Join(dirs.TSRRaw, e.Name()))
		if err != nil {
			t.Errorf("%s: load tsr_raw: %v", name, err)
			continue
		}

		// Global structure lines, sorted exactly as Python sorts self.tb_cpns
		// in _table_transformer_job: rows by (page, y0) via gather's
		// sort_Y_firstly; columns by (page, layoutno, x0). layoutno is the
		// per-page table ordinal; the tsr_raw dump carries the document-global
		// table_index, so the per-page rank is derived (identical within a page).
		// layoutno is the PER-PAGE table ordinal (pdf_parser.py:509
		// f"table-{page_table_index}"), so the per-page rank of each
		// document-global table_index must be derived from the tsr_raw dump.
		perPageTables := make(map[int][]int)
		for _, c := range tsr {
			dup := false
			for _, ti := range perPageTables[c.Page] {
				if ti == c.TableIndex {
					dup = true
					break
				}
			}
			if !dup {
				perPageTables[c.Page] = append(perPageTables[c.Page], c.TableIndex)
			}
		}
		rankInPage := make(map[int]int) // table_index → per-page ordinal
		for _, tis := range perPageTables {
			sort.Ints(tis)
			for i, ti := range tis {
				rankInPage[ti] = i
			}
		}
		layoutnoOf := func(tableIndex int) string {
			return fmt.Sprintf("table-%d", rankInPage[tableIndex])
		}

		var rows, clmns []rcLine
		for _, c := range tsr {
			ln := rcLine{page: c.Page, layoutno: layoutnoOf(c.TableIndex), score: c.Score, x0: c.X0, y0: c.Y0, x1: c.X1, y1: c.Y1}
			switch c.Label {
			case "table row":
				rows = append(rows, ln)
			case "table column":
				clmns = append(clmns, ln)
			}
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].page != rows[j].page {
				return rows[i].page < rows[j].page
			}
			return rows[i].y0 < rows[j].y0
		})
		sort.Slice(clmns, func(i, j int) bool {
			if clmns[i].page != clmns[j].page {
				return clmns[i].page < clmns[j].page
			}
			if clmns[i].layoutno != clmns[j].layoutno {
				return clmns[i].layoutno < clmns[j].layoutno
			}
			return clmns[i].x0 < clmns[j].x0
		})
		// Python's _table_transformer_job de-duplicates the structure lines
		// against the full page box set before matching: rows via gather
		// (layouts_cleanup far=5 thr=0.6), columns via layouts_cleanup far=5
		// thr=0.5. The deduped line set is what find_overlapped_with_threshold
		// / find_horizontally_tightest_fit index into.
		rows = layoutsCleanupPy(rows, allBoxes, 5, 0.6)
		clmns = layoutsCleanupPy(clmns, allBoxes, 5, 0.5)

		matchR, matchC, total := 0, 0, 0
		var rMiss, cMiss []string
		for _, b := range boxes {
			gotR := findOverlappedWithThresholdPy(b, rows, 0.3)
			gotC := findHorizontallyTightestFitPy(b, clmns)
			total++
			if gotR == b.R {
				matchR++
			} else if len(rMiss) < 5 {
				rMiss = append(rMiss, trimBoxText(b.Text))
			}
			if gotC == b.C {
				matchC++
			} else if len(cMiss) < 5 {
				cMiss = append(cMiss, trimBoxText(b.Text))
			}
		}
		t.Logf("%s: R derived==dump %d/%d, C derived==dump %d/%d (rows=%d cols=%d)",
			name, matchR, total, matchC, total, len(rows), len(clmns))
		// With the tsr_raw detection score and the full page box snapshot in
		// the dump, both derivations reproduce Python's R/C 1:1.
		if matchR < total {
			t.Errorf("%s: R derivation diverges from dump — miss on %v", name, rMiss)
		}
		if matchC < total {
			t.Errorf("%s: C derivation diverges from dump — miss on %v", name, cMiss)
		}
	}
}

func trimBoxText(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 20 {
		return s[:20] + "…"
	}
	return s
}
