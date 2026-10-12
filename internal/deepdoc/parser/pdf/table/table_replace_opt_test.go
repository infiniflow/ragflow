package table

import (
	"maps"
	"math/rand"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// repKey identifies a replacement by (tableIdx, boxIdx) for set comparison,
// since the per-page index may emit the same pairs in a different order than
// the brute-force reference.
type repKey struct{ ti, bi int }

func repSet(reps []replacement) map[repKey]bool {
	m := make(map[repKey]bool, len(reps))
	for _, r := range reps {
		m[repKey{r.tableIdx, r.boxIdx}] = true
	}
	return m
}

// referenceBuildReplacementsAfterMerge is the unoptimized O(tables*boxes*positions)
// implementation, used as an oracle to prove the per-page index is equivalent.
func referenceBuildReplacementsAfterMerge(boxes []pdf.TextBox, tables []pdf.TableItem, removeSet map[int]bool) []replacement {
	var reps []replacement
	for ti := range tables {
		for i := range boxes {
			if boxes[i].LayoutType != pdf.LayoutTypeTable || removeSet[i] {
				continue
			}
			for _, tp := range tables[ti].Positions {
				if boxOverlapsPositionPage(boxes[i], tp) {
					reps = append(reps, replacement{tableIdx: ti, boxIdx: i})
					break
				}
			}
		}
	}
	return reps
}

// referenceMarkNoMergeTables mirrors MarkNoMergeTables with the full cross
// product, so we can assert the indexed version mutates NoMerge identically. It
// faithfully reproduces the original pre-index semantics: tables are scanned in
// ascending index order and lastTableTI keeps the HIGHEST-indexed matching
// table (there is no early break on the first match).
func referenceMarkNoMergeTables(boxes []pdf.TextBox, tables []pdf.TableItem) {
	var lastTableTI int = -1
	for i := range boxes {
		lt := boxes[i].LayoutType
		if lt == pdf.LayoutTypeTable {
			matched := false
			for ti := range tables {
				for _, tp := range tables[ti].Positions {
					if boxOverlapsPositionPage(boxes[i], tp) {
						lastTableTI = ti
						matched = true
						break
					}
				}
			}
			if !matched {
				lastTableTI = -1
			}
			continue
		}
		if lastTableTI >= 0 && (lt == pdf.LayoutTypeTitle || lt == pdf.DLALabelTableCaption || lt == pdf.DLALabelFigureCaption || lt == pdf.LayoutTypeReference || IsCaptionBox(boxes[i].Text, lt)) {
			tables[lastTableTI].NoMerge = true
		}
	}
}

// cloneTables deep-copies the NoMerge-relevant part of a table slice so the
// two implementations can run against independent copies and be compared.
func cloneTables(in []pdf.TableItem) []pdf.TableItem {
	out := make([]pdf.TableItem, len(in))
	for i := range in {
		out[i] = pdf.TableItem{
			Positions: in[i].Positions,
			NoMerge:   in[i].NoMerge,
		}
	}
	return out
}

// TestBuildReplacementsAfterMergeCrossPageEquivalence checks the per-page index
// against a brute-force oracle over randomly generated multi-page documents,
// including tables that span several pages. This is the guard that the
// optimization does not drop valid (cross-page) matches and does not invent
// invalid ones.
func TestBuildReplacementsAfterMergeCrossPageEquivalence(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const (
		pages      = 12
		nBoxes     = 300
		nTables    = 80
		posPerTab  = 3
		coordRange = 1000.0
	)
	boxes := make([]pdf.TextBox, nBoxes)
	for i := 0; i < nBoxes; i++ {
		p := rng.Intn(pages) + 1
		x0 := rng.Float64() * coordRange
		y0 := rng.Float64() * coordRange
		boxes[i] = pdf.TextBox{
			PageNumber:    p,
			HasPageNumber: true,
			X0:            x0,
			X1:            x0 + rng.Float64()*50,
			Top:           y0,
			Bottom:        y0 + rng.Float64()*50,
			LayoutType:    pdf.LayoutTypeTable,
		}
	}
	tables := make([]pdf.TableItem, nTables)
	for ti := 0; ti < nTables; ti++ {
		// A table spans 1..posPerTab consecutive pages (a real cross-page table).
		start := rng.Intn(pages) + 1
		poss := make([]pdf.Position, 0, posPerTab)
		for k := 0; k < posPerTab; k++ {
			p := ((start + k - 1) % pages) + 1
			x0 := rng.Float64() * coordRange
			y0 := rng.Float64() * coordRange
			poss = append(poss, pdf.Position{
				PageNumbers: []int{p},
				Left:        x0,
				Right:       x0 + rng.Float64()*50,
				Top:         y0,
				Bottom:      y0 + rng.Float64()*50,
			})
		}
		tables[ti] = pdf.TableItem{Positions: poss}
	}
	// Add a table whose positions carry NO page metadata: it is page-agnostic
	// and must still match any box via the X/Y fallback (the noPage bucket).
	noPageTable := pdf.TableItem{Positions: []pdf.Position{
		{Left: 0, Right: coordRange, Top: 0, Bottom: coordRange},
	}}
	tables = append(tables, noPageTable)

	got := buildReplacementsAfterMerge(boxes, tables, nil)
	want := referenceBuildReplacementsAfterMerge(boxes, tables, nil)
	gotSet, wantSet := repSet(got), repSet(want)
	if !maps.Equal(gotSet, wantSet) {
		var onlyGot, onlyWant []repKey
		for k := range gotSet {
			if !wantSet[k] {
				onlyGot = append(onlyGot, k)
			}
		}
		for k := range wantSet {
			if !gotSet[k] {
				onlyWant = append(onlyWant, k)
			}
		}
		t.Errorf("indexed buildReplacementsAfterMerge diverges from brute-force reference:\n only-in-indexed=%v\n only-in-reference=%v", onlyGot, onlyWant)
	}
}

// TestMarkNoMergeTablesCrossPageEquivalence mirrors the above for the NoMerge
// marking path.
func TestMarkNoMergeTablesCrossPageEquivalence(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	const (
		pages      = 10
		nBoxes     = 200
		nTables    = 40
		posPerTab  = 2
		coordRange = 1000.0
	)
	boxes := make([]pdf.TextBox, nBoxes)
	for i := 0; i < nBoxes; i++ {
		p := rng.Intn(pages) + 1
		x0 := rng.Float64() * coordRange
		y0 := rng.Float64() * coordRange
		lt := pdf.LayoutTypeTable
		if i%5 == 0 {
			lt = pdf.LayoutTypeTitle // a caption-like follower
		}
		boxes[i] = pdf.TextBox{
			PageNumber:    p,
			HasPageNumber: true,
			X0:            x0,
			X1:            x0 + rng.Float64()*50,
			Top:           y0,
			Bottom:        y0 + rng.Float64()*50,
			LayoutType:    lt,
			Text:          "caption text",
		}
	}
	tables := make([]pdf.TableItem, nTables)
	for ti := 0; ti < nTables; ti++ {
		start := rng.Intn(pages) + 1
		poss := make([]pdf.Position, 0, posPerTab)
		for k := 0; k < posPerTab; k++ {
			p := ((start + k - 1) % pages) + 1
			x0 := rng.Float64() * coordRange
			y0 := rng.Float64() * coordRange
			poss = append(poss, pdf.Position{
				PageNumbers: []int{p},
				Left:        x0,
				Right:       x0 + rng.Float64()*50,
				Top:         y0,
				Bottom:      y0 + rng.Float64()*50,
			})
		}
		tables[ti] = pdf.TableItem{Positions: poss}
	}

	indexed := cloneTables(tables)
	MarkNoMergeTables(boxes, indexed)
	ref := cloneTables(tables)
	referenceMarkNoMergeTables(boxes, ref)

	for i := range indexed {
		if indexed[i].NoMerge != ref[i].NoMerge {
			t.Errorf("table %d NoMerge mismatch: indexed=%v reference=%v", i, indexed[i].NoMerge, ref[i].NoMerge)
		}
	}
}

// TestMarkNoMergeTablesCrossPageSpanningTable pins the exact behavior the
// optimization must preserve: a table that occupies pages 5 and 7 must be
// matchable by boxes on BOTH of those pages, and must NOT be matched by a box
// on an unrelated page.
func TestMarkNoMergeTablesCrossPageSpanningTable(t *testing.T) {
	spanTable := pdf.TableItem{Positions: []pdf.Position{
		{PageNumbers: []int{5}, Left: 10, Right: 100, Top: 10, Bottom: 50},
		{PageNumbers: []int{7}, Left: 10, Right: 100, Top: 10, Bottom: 50},
	}}
	otherTable := pdf.TableItem{Positions: []pdf.Position{
		{PageNumbers: []int{6}, Left: 10, Right: 100, Top: 10, Bottom: 50},
	}}

	// Box on page 5 overlapping the span table's page-5 position.
	boxP5 := pdf.TextBox{PageNumber: 5, HasPageNumber: true, X0: 10, X1: 100, Top: 10, Bottom: 50, LayoutType: pdf.LayoutTypeTable}
	// Box on page 7 overlapping the span table's page-7 position.
	boxP7 := pdf.TextBox{PageNumber: 7, HasPageNumber: true, X0: 10, X1: 100, Top: 10, Bottom: 50, LayoutType: pdf.LayoutTypeTable}
	// Box on page 6 with identical coordinates but no span-table position there.
	boxP6 := pdf.TextBox{PageNumber: 6, HasPageNumber: true, X0: 10, X1: 100, Top: 10, Bottom: 50, LayoutType: pdf.LayoutTypeTable}
	// A caption following on page 5 should mark the span table NoMerge.
	capP5 := pdf.TextBox{PageNumber: 5, HasPageNumber: true, X0: 10, X1: 100, Top: 60, Bottom: 70, LayoutType: pdf.LayoutTypeTitle, Text: "表 1"}

	boxes := []pdf.TextBox{boxP5, boxP6, boxP7, capP5}
	tables := []pdf.TableItem{spanTable, otherTable}

	MarkNoMergeTables(boxes, tables)

	if !tables[0].NoMerge {
		t.Errorf("spanning table (pages 5&7) should be marked NoMerge by the page-5 caption")
	}
	if tables[1].NoMerge {
		t.Errorf("page-6 table must NOT be marked NoMerge: page-6 box did not match it")
	}
}

// TestMarkNoMergeTablesNoPagePrecedence locks the original semantics of
// MarkNoMergeTables: when a single table-layout box overlaps several tables, the
// caption/title that immediately follows must mark the HIGHEST-indexed matching
// table as NoMerge. This must hold even when one of the overlapping tables is
// page-agnostic (its Position carries no PageNumbers) and sits at a LOWER index
// than a page-specific table. The old (page-blind) candidate ordering appended
// the page-agnostic positions last and overwrote lastTableTI on every match, so
// a low-index page-agnostic table could wrongly win and receive the NoMerge mark
// instead of the higher-index page-specific table.
func TestMarkNoMergeTablesNoPagePrecedence(t *testing.T) {
	// Lower-index table, page-agnostic: its position carries no PageNumbers, so
	// it matches any box that overlaps it in X/Y regardless of page.
	noPageTable := pdf.TableItem{Positions: []pdf.Position{
		{Left: 10, Right: 100, Top: 10, Bottom: 50},
	}}
	// Higher-index table, anchored on page 5.
	pageTable := pdf.TableItem{Positions: []pdf.Position{
		{PageNumbers: []int{5}, Left: 10, Right: 100, Top: 10, Bottom: 50},
	}}
	tables := []pdf.TableItem{noPageTable, pageTable} // index 0 = noPage, index 1 = page 5

	// A table-layout box on page 5 overlapping BOTH table positions.
	box := pdf.TextBox{
		PageNumber: 5, HasPageNumber: true,
		X0: 10, X1: 100, Top: 10, Bottom: 50,
		LayoutType: pdf.LayoutTypeTable,
	}
	// A caption following on the same page: it must mark the highest-indexed
	// matching table (index 1, the page-5 table), not the lower-index noPage one.
	caption := pdf.TextBox{
		PageNumber: 5, HasPageNumber: true,
		X0: 10, X1: 100, Top: 60, Bottom: 70,
		LayoutType: pdf.LayoutTypeTitle, Text: "表 1",
	}

	MarkNoMergeTables([]pdf.TextBox{box, caption}, tables)

	if !tables[1].NoMerge {
		t.Errorf("highest-indexed matching table (page 5, index 1) should be marked NoMerge by the following caption")
	}
	if tables[0].NoMerge {
		t.Errorf("lower-index page-agnostic table must NOT win NoMerge over the higher-index page-specific table")
	}
}
