package layout

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// ---- charspy JSON binding + line reconstruction (mirrors tool-py
// extract_column_divergence._reconstruct_lines so the Go detector is scored on
// the SAME line boxes the Python reference used) ----

type charBox struct {
	Text   string  `json:"text"`
	X0     float64 `json:"x0"`
	X1     float64 `json:"x1"`
	Top    float64 `json:"top"`
	Bottom float64 `json:"bottom"`
	Size   float64 `json:"size"`
}

type charPage struct {
	Pages [][]charBox `json:"pages"`
}

func reconstructLines(chars []charBox) []pdf.TextBox {
	if len(chars) == 0 {
		return nil
	}
	sizes := make([]float64, len(chars))
	for i, c := range chars {
		sizes[i] = c.Size
	}
	sort.Float64s(sizes)
	medSize := sizes[len(sizes)/2]
	vTol := math.Max(medSize*0.8, 4.0)
	hGap := math.Max(medSize*3.0, 15.0)

	rows := map[int][]charBox{}
	for _, c := range chars {
		key := int(math.Round(c.Top / vTol))
		rows[key] = append(rows[key], c)
	}
	keys := make([]int, 0, len(rows))
	for k := range rows {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	var lines []pdf.TextBox
	for _, key := range keys {
		row := rows[key]
		sort.Slice(row, func(a, b int) bool { return row[a].X0 < row[b].X0 })
		var cur *pdf.TextBox
		for _, c := range row {
			if cur == nil {
				cur = &pdf.TextBox{X0: c.X0, X1: c.X1, Top: c.Top, Bottom: c.Bottom, Text: c.Text}
			} else if c.X0-cur.X1 > hGap {
				lines = append(lines, *cur)
				cur = &pdf.TextBox{X0: c.X0, X1: c.X1, Top: c.Top, Bottom: c.Bottom, Text: c.Text}
			} else if c.X1 > cur.X1 {
				cur.X1 = c.X1
			}
		}
		if cur != nil {
			lines = append(lines, *cur)
		}
	}
	return lines
}

// labeledPage mirrors the first 18 (locked) entries of
// tool-py/column_labeling_sheet.json — human-confirmed column truth.
type labeledPage struct {
	pdf   string
	page  int
	truth int
}

// sheetEntry mirrors the JSON shape of tool-py/column_labeling_sheet.json.
type sheetEntry struct {
	PDF    string `json:"pdf"`
	Page   int    `json:"page"`
	TruthK *int   `json:"truth_k"`
}

// loadLabeledPages reads every page with a non-null truth_k from the shared
// label sheet (single source of truth, also used by the Python reference).
// Pages still marked null (need the rendered PDF to decide) are skipped.
func loadLabeledPages(t *testing.T) []labeledPage {
	sheetPath := "../tool-py/column_labeling_sheet.json"
	raw, err := os.ReadFile(sheetPath)
	if err != nil {
		t.Skipf("label sheet not found at %s: %v", sheetPath, err)
	}
	var entries []sheetEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("unmarshal sheet: %v", err)
	}
	var out []labeledPage
	for _, e := range entries {
		if e.TruthK == nil {
			continue
		}
		out = append(out, labeledPage{pdf: e.PDF, page: e.Page, truth: *e.TruthK})
	}
	if len(out) == 0 {
		t.Skip("no labeled pages in sheet")
	}
	return out
}

// TestAssignColumnCombined_Labeled scores the gap+KMeans hybrid against the
// LIVE gap-only baseline on every labeled page in the shared sheet (the sheet
// is the single source of truth, also used by the Python reference). Hard
// pages were labeled from the page coverage profile + strip map in
// tool-py/analyze_hardcases.py, tagged H (high) / L (low) confidence in
// truth_note. The 18-page labeled sample that motivated this detector reported
// KMeans 11.1% / gap 61.1% accuracy, but that sample was hand-picked and must
// not be used as a target. The per-page / single / double / ACC lines below
// are the real signal, and the only asserted contract is "combined must not
// regress gap by >5pt".
func TestAssignColumnCombined_Labeled(t *testing.T) {
	charspyDir := "../testdata/charspy"
	if _, err := os.Stat(charspyDir); err != nil {
		t.Skipf("charspy corpus not found at %s (run from package dir): %v", charspyDir, err)
	}
	cases := loadLabeledPages(t)
	var correct, fs, miss, sCorrect, sTotal, dCorrect, dTotal int
	var gCorrect, gFs, gMiss, gsCorrect, gsTotal, gdCorrect, gdTotal int
	for _, c := range cases {
		raw, err := os.ReadFile(filepath.Join(charspyDir, c.pdf))
		if err != nil {
			t.Fatalf("read %s: %v", c.pdf, err)
		}
		var cp charPage
		if err := json.Unmarshal(raw, &cp); err != nil {
			t.Fatalf("unmarshal %s: %v", c.pdf, err)
		}
		lines := reconstructLines(cp.Pages[c.page])
		boxes := make([]pdf.TextBox, len(lines))
		for i, l := range lines {
			boxes[i] = l
			boxes[i].PageNumber = 0
		}
		res := AssignColumn(boxes)
		k := 1
		for _, b := range res {
			if b.ColID+1 > k {
				k = b.ColID + 1
			}
		}
		// Live gap-only baseline on the SAME lines (combined wraps gap, so
		// this is the honest reference — not the old 18-page Python 61.1%).
		g := gapColumnCount(lines, 0.04, 0.15, 2.0)
		record := func(got, truth int, corr, fsC, missC, sCorr, sTot, dCorr, dTot *int) {
			if got == truth {
				*corr++
			} else if got > truth {
				*fsC++
			} else {
				*missC++
			}
			if truth == 1 {
				*sTot++
				if got == truth {
					*sCorr++
				}
			} else {
				*dTot++
				if got == truth {
					*dCorr++
				}
			}
		}
		record(k, c.truth, &correct, &fs, &miss, &sCorrect, &sTotal, &dCorrect, &dTotal)
		record(g, c.truth, &gCorrect, &gFs, &gMiss, &gsCorrect, &gsTotal, &gdCorrect, &gdTotal)
		tag := "OK"
		if k != c.truth {
			tag = "WRONG"
		}
		t.Logf("[%s] %s p%d  truth=%d got=%d (gap=%d)  comb single=%d/%d double=%d/%d  gap single=%d/%d double=%d/%d",
			tag, c.pdf, c.page, c.truth, k, g, sCorrect, sTotal, dCorrect, dTotal, gsCorrect, gsTotal, gdCorrect, gdTotal)
	}
	n := len(cases)
	acc := 100.0 * float64(correct) / float64(n)
	gapAcc := 100.0 * float64(gCorrect) / float64(n)
	t.Logf("Go gap+KMeans combined: ACC=%.1f%% false-split=%d miss=%d  single=%d/%d double=%d/%d (n=%d)",
		acc, fs, miss, sCorrect, sTotal, dCorrect, dTotal, n)
	t.Logf("Go gap-only  baseline: ACC=%.1f%% false-split=%d miss=%d  single=%d/%d double=%d/%d (n=%d)",
		gapAcc, gFs, gMiss, gsCorrect, gsTotal, gdCorrect, gdTotal, n)
	// The hybrid's contract: gap is the safe base, KMeans only ADDS recoveries
	// (double columns gap misses). So combined must not regress gap by more
	// than a small tolerance. The 75% / 88.9% figures were tuned targets on
	// the comfortable 18 pages and must not be asserted (circular). Watch the
	// two ACC lines above as harder pages are labeled and added.
	if acc < gapAcc-5.0 {
		t.Errorf("combined ACC=%.1f%% regresses gap-only baseline %.1f%% by >5pt (KMeans addition is harmful)", acc, gapAcc)
	}
}

// TestKmeansK2PlusPlus_Smoke sanity-checks the density-aware k=2 gate on a
// clean bimodal vs single-mode input.
func TestKmeansK2PlusPlus_Smoke(t *testing.T) {
	// Two well-separated modes, balanced -> two real clusters.
	bimodal := []float64{10, 11, 12, 13, 90, 91, 92, 93}
	labels, cents := kmeansK2PlusPlus(bimodal, 42)
	if len(uniqueInts(labels)) != 2 {
		t.Errorf("bimodal: expected 2 clusters, got %d", len(uniqueInts(labels)))
	}
	// kmeansK2PlusPlus does not guarantee centroid ordering, so assert the
	// property it DOES guarantee: a bimodal split yields two well-separated
	// centroids. (assignColIDs re-sorts them for ColID assignment.)
	if math.Abs(cents[0]-cents[1]) < 50 {
		t.Errorf("bimodal: centroids should be well separated, got %v", cents)
	}
	// Single tight mode -> still 2 labels after Lloyd, but minority tiny.
	tight := []float64{10, 10, 10, 10, 11, 10, 10, 10}
	labels2, _ := kmeansK2PlusPlus(tight, 42)
	counts := map[int]int{}
	for _, l := range labels2 {
		counts[l]++
	}
	minC := math.MaxInt32
	for _, c := range counts {
		if c < minC {
			minC = c
		}
	}
	if float64(minC) >= 0.30*float64(len(tight)) {
		t.Errorf("tight single-mode: minority %.0f%% should be <30%%", 100*float64(minC)/float64(len(tight)))
	}
}

// TestAssignColumn_FullWidthBridgeKeepsBalancedTwoColumn is a regression test
// for the balance-gate / prune asymmetry: the balance gate clusters the BODY
// (full-width front matter excluded), but pruneColumns must count the SAME
// body — not all lines. A real but minority-right two-column block dominated
// by full-width title/abstract lines must stay two columns; otherwise the
// full-width lines inflate the left column and prune wrongly collapses the
// page to one.
func TestAssignColumn_FullWidthBridgeKeepsBalancedTwoColumn(t *testing.T) {
	const (
		minX0 = 100.0
		maxX1 = 700.0
	)
	var boxes []pdf.TextBox
	add := func(x0, x1 float64) {
		top := float64(len(boxes)) * 12
		boxes = append(boxes, pdf.TextBox{
			PageNumber: 0, X0: x0, X1: x1, Top: top, Bottom: top + 10, Text: "b",
		})
	}
	// Body: a genuine two-column block with a real gutter. The right column is
	// the minority (6 of 20 body lines, i.e. 30% — exactly the gate floor).
	for i := 0; i < 14; i++ {
		add(minX0, 350) // left column
	}
	for i := 0; i < 6; i++ {
		add(450, maxX1) // right column
	}
	// 60 full-width front-matter lines (title + abstract) bridge the gutter so
	// gap reports a single column. They must not be re-counted by prune.
	for i := 0; i < 60; i++ {
		add(minX0, maxX1) // full width
	}

	res := AssignColumn(boxes)
	k := 1
	for _, b := range res {
		if b.ColID+1 > k {
			k = b.ColID + 1
		}
	}
	if k != 2 {
		t.Fatalf("balanced two-column page with full-width front matter collapsed to %d column(s); want 2", k)
	}
}
