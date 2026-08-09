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

func lockedLabeledPages() []labeledPage {
	return []labeledPage{
		{"01_english_simple.pdf.json", 0, 1},
		{"02_chinese_simple.pdf.json", 0, 1},
		{"03_multipage.pdf.json", 0, 1},
		{"03_multipage.pdf.json", 1, 1},
		{"03_multipage.pdf.json", 2, 1},
		{"07_mixed_content.pdf.json", 0, 1},
		{"09_crosspage_paragraph.pdf.json", 0, 1},
		{"16_dense_cjk.pdf.json", 0, 1},
		{"1例3个月喉噗合并先天性心脏病患儿气管插管的麻醉护理.pdf.json", 0, 2},
		{"1例3个月喉噗合并先天性心脏病患儿气管插管的麻醉护理.pdf.json", 1, 2},
		{"2023-07-03.pdf.json", 30, 1},
		{"2023-07-03.pdf.json", 34, 1},
		{"2023-07-03.pdf.json", 40, 1},
		{"2024 - ZoomNeXt A Unified Collaborative Pyramid Network .pdf.json", 0, 2},
		{"2024 - ZoomNeXt A Unified Collaborative Pyramid Network .pdf.json", 1, 2},
		{"2024 - ZoomNeXt A Unified Collaborative Pyramid Network .pdf.json", 2, 2},
		{"2024 - ZoomNeXt A Unified Collaborative Pyramid Network .pdf.json", 3, 2},
		{"2024 - ZoomNeXt A Unified Collaborative Pyramid Network .pdf.json", 4, 2},
	}
}

// TestAssignColumnCombined_Labeled ports gap+KMeans to Go and scores it on the
// 18 human-labeled pages. Reference (Python) numbers: KMeans 11.1%, gap 61.1%,
// gap+KMeans combined 94.4%. The Go port must clear the gap baseline and land
// near the combined target.
func TestAssignColumnCombined_Labeled(t *testing.T) {
	charspyDir := "../testdata/charspy"
	if _, err := os.Stat(charspyDir); err != nil {
		t.Skipf("charspy corpus not found at %s (run from package dir): %v", charspyDir, err)
	}
	cases := lockedLabeledPages()
	var correct, fs, miss, sCorrect, sTotal, dCorrect, dTotal int
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
		match := k == c.truth
		if match {
			correct++
		} else if k > c.truth {
			fs++
		} else {
			miss++
		}
		if c.truth == 1 {
			sTotal++
			if match {
				sCorrect++
			}
		} else {
			dTotal++
			if match {
				dCorrect++
			}
		}
		tag := "OK"
		if !match {
			tag = "WRONG"
		}
		t.Logf("[%s] %s p%d  truth=%d got=%d  single=%d/%d double=%d/%d",
			tag, c.pdf, c.page, c.truth, k, sCorrect, sTotal, dCorrect, dTotal)
	}
	n := len(cases)
	acc := 100.0 * float64(correct) / float64(n)
	t.Logf("Go gap+KMeans combined: ACC=%.1f%% false-split=%d miss=%d  single=%d/%d double=%d/%d (n=%d)",
		acc, fs, miss, sCorrect, sTotal, dCorrect, dTotal, n)
	// Must beat the gap-only baseline (61.1%) and land near the combined target.
	if acc < 88.0 {
		t.Errorf("Go combined detector ACC=%.1f%% below expected ~94%% (gap baseline 61.1%%)", acc)
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
	if cents[0] > cents[1] {
		t.Errorf("bimodal: centroids not ordered: %v", cents)
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
