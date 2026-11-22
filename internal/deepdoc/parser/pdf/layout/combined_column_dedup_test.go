package layout

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// assignColumnReference is the pre-optimization oracle kept as a test-only
// copy of AssignColumn. It calls detectColumnCount twice per page: once for
// the document-wide majority vote (cents discarded), once for centroid
// assignment. The optimized AssignColumn must produce byte-identical ColIDs.
func assignColumnReference(boxes []pdf.TextBox) []pdf.TextBox {
	if len(boxes) == 0 {
		return boxes
	}
	pageGroups, sortedPages := groupBoxesByPage(boxes)
	result := make([]pdf.TextBox, len(boxes))
	copy(result, boxes)

	colCount := map[int]int{}
	var firstK []int
	for _, pg := range sortedPages {
		k, _ := detectColumnCount(boxes, pageGroups[pg])
		if colCount[k] == 0 {
			firstK = append(firstK, k)
		}
		colCount[k]++
	}
	modeK, modeN := firstK[0], colCount[firstK[0]]
	for _, k := range firstK {
		if colCount[k] > modeN {
			modeK, modeN = k, colCount[k]
		}
	}
	if modeK == 1 {
		for i := range result {
			result[i].ColID = 0
		}
		return result
	}
	for _, pg := range sortedPages {
		indices := pageGroups[pg]
		k, cents := detectColumnCount(boxes, indices)
		assignColIDs(boxes, result, indices, k, cents)
	}
	return result
}

func setPageNumber(boxes []pdf.TextBox, page int) []pdf.TextBox {
	for i := range boxes {
		boxes[i].PageNumber = page
	}
	return boxes
}

// mixedPageFixture: page0 is a balanced double, page1 a single column.
// The tie on the majority vote resolves to the first page's k=2, so the
// cached-(k,cents) reuse path is actually exercised (not the modeK==1 early exit).
func mixedPageFixture() []pdf.TextBox {
	p0 := append(stackedColumn(50, 240, 24, 10, 10), stackedColumn(270, 460, 24, 10, 10)...)
	setPageNumber(p0, 0)
	p1 := stackedColumn(50, 240, 16, 10, 10)
	setPageNumber(p1, 1)
	return append(p0, p1...)
}

// allSingleFixture: every page single-column -> modeK==1 early-exit path.
func allSingleFixture() []pdf.TextBox {
	p0 := setPageNumber(stackedColumn(50, 240, 20, 10, 10), 0)
	p1 := setPageNumber(stackedColumn(60, 250, 18, 10, 10), 1)
	return append(p0, p1...)
}

func TestAssignColumnSinglePassEquivalence(t *testing.T) {
	cases := map[string][]pdf.TextBox{
		"mixed-majority-reuse":  mixedPageFixture(),
		"all-single-early-exit": allSingleFixture(),
	}
	for name, boxes := range cases {
		t.Run(name, func(t *testing.T) {
			want := assignColumnReference(boxes)
			got := AssignColumn(boxes)
			if len(got) != len(want) {
				t.Fatalf("length: got %d want %d", len(got), len(want))
			}
			for i := range got {
				if got[i].ColID != want[i].ColID {
					t.Fatalf("box %d ColID: got %d want %d", i, got[i].ColID, want[i].ColID)
				}
			}
		})
	}
}

func BenchmarkAssignColumnSinglePass(b *testing.B) {
	boxes := mixedPageFixture()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		AssignColumn(boxes)
	}
}
