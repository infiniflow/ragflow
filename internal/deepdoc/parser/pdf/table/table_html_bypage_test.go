package table

import (
	"maps"
	"math/rand"
	"reflect"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// ── brute-force oracles (the pre-optimization behavior) ─────────────────────

// referenceCollectTableBoxes is the unindexed O(boxes*positions) scan that
// buildTableHTMLs used before the page-bucketing optimization. It returns the
// table-layout boxes overlapping tbl, in ascending box-index order.
func referenceCollectTableBoxes(boxes []pdf.TextBox, tbl pdf.TableItem) []pdf.TextBox {
	var out []pdf.TextBox
	for i := range boxes {
		if boxes[i].LayoutType != pdf.LayoutTypeTable {
			continue
		}
		for pi := range tbl.Positions {
			if boxOverlapsPositionPage(boxes[i], tbl.Positions[pi]) {
				out = append(out, boxes[i])
				break
			}
		}
	}
	return out
}

// referenceBuildTableHTMLs is the full pre-optimization buildTableHTMLs, copied
// verbatim as an oracle so the optimized version is provably equivalent.
func referenceBuildTableHTMLs(boxes []pdf.TextBox, tables []pdf.TableItem) map[int]string {
	htmls := make(map[int]string)
	for ti := range tables {
		if len(tables[ti].Cells) == 0 {
			continue
		}
		s := tables[ti].Scale
		pageGlobalCells := CellSliceToPageSpace(tables[ti].Cells, tables[ti].CropOffX, tables[ti].CropOffY, s)
		var tableBoxes []pdf.TextBox
		for i := range boxes {
			if boxes[i].LayoutType != pdf.LayoutTypeTable {
				continue
			}
			for _, tp := range tables[ti].Positions {
				if boxOverlapsPositionPage(boxes[i], tp) {
					tableBoxes = append(tableBoxes, boxes[i])
					break
				}
			}
		}
		htmls[ti] = ConstructTable(pageGlobalCells, tableBoxes, tables[ti].Caption, &tables[ti])
	}
	return htmls
}

// ── synthetic document generators ──────────────────────────────────────────

// buildBoxesForCollectTest builds a mix of table-layout and non-table boxes,
// some with page metadata and some without. For every table position a matching
// table box is planted on the same page with overlapping coordinates, so the
// optimized and brute-force scans both exercise non-empty, order-sensitive
// selections.
func buildBoxesForCollectTest(rng *rand.Rand, pages, nBoxes int, tables []pdf.TableItem) []pdf.TextBox {
	boxes := make([]pdf.TextBox, 0, nBoxes)
	// Plant one overlapping box per (table, position) so overlaps actually occur.
	for ti := range tables {
		for pi := range tables[ti].Positions {
			pos := tables[ti].Positions[pi]
			if len(pos.PageNumbers) == 0 {
				// No-page position: plant a no-page box so it can still overlap.
				boxes = append(boxes, pdf.TextBox{
					X0:         pos.Left - 1,
					X1:         pos.Right + 1,
					Top:        pos.Top - 1,
					Bottom:     pos.Bottom + 1,
					LayoutType: pdf.LayoutTypeTable,
				})
				continue
			}
			p := pos.PageNumbers[0]
			boxes = append(boxes, pdf.TextBox{
				PageNumber:    p,
				HasPageNumber: true,
				X0:            pos.Left - 1,
				X1:            pos.Right + 1,
				Top:           pos.Top - 1,
				Bottom:        pos.Bottom + 1,
				LayoutType:    pdf.LayoutTypeTable,
			})
		}
	}
	// Fill the rest with random boxes (table + non-table, paged + page-less).
	// Page-less boxes are rare in practice (most boxes carry page metadata), so
	// only ~5% lack a page number — this keeps the noPage bucket small and the
	// benchmark representative of real large PDFs.
	for len(boxes) < nBoxes {
		p := rng.Intn(pages) + 1
		// Page-less boxes are very rare in practice (almost every box carries
		// page metadata). Keep them at ~0.5% so the noPage bucket stays small and
		// the benchmark exercises the dense, many-boxes-per-page case where the
		// page-bucketed scan wins big over the brute-force O(boxes*positions).
		hasPage := rng.Intn(200) < 199
		isTable := rng.Intn(2) == 0
		lt := pdf.LayoutTypeText
		if isTable {
			lt = pdf.LayoutTypeTable
		}
		x0 := rng.Float64() * 1000
		y0 := rng.Float64() * 1000
		b := pdf.TextBox{
			X0:         x0,
			X1:         x0 + rng.Float64()*50,
			Top:        y0,
			Bottom:     y0 + rng.Float64()*50,
			LayoutType: lt,
		}
		if hasPage {
			b.PageNumber = p
			b.HasPageNumber = true
		}
		boxes = append(boxes, b)
	}
	return boxes
}

func buildTablesForTest(rng *rand.Rand, pages, nTables, posPerTab int) []pdf.TableItem {
	tables := make([]pdf.TableItem, nTables)
	for ti := 0; ti < nTables; ti++ {
		poss := make([]pdf.Position, 0, posPerTab)
		// Occasionally make a position page-agnostic (empty PageNumbers) to cover
		// the fallback path in collectTableBoxes.
		for k := 0; k < posPerTab; k++ {
			if rng.Intn(7) == 0 {
				poss = append(poss, pdf.Position{
					Left:   rng.Float64() * 1000,
					Right:  rng.Float64()*50 + 50,
					Top:    rng.Float64() * 1000,
					Bottom: rng.Float64()*50 + 50,
				})
				continue
			}
			p := rng.Intn(pages) + 1
			poss = append(poss, pdf.Position{
				PageNumbers: []int{p},
				Left:        rng.Float64() * 1000,
				Right:       rng.Float64()*50 + 50,
				Top:         rng.Float64() * 1000,
				Bottom:      rng.Float64()*50 + 50,
			})
		}
		tables[ti] = pdf.TableItem{Positions: poss}
	}
	return tables
}

// cloneTablesForHTML deep-copies the TableItem fields ConstructTable reads, so
// the two implementations can run against independent copies (ConstructTable
// mutates item.Grid / item.Rows as a side effect).
func cloneTablesForHTML(in []pdf.TableItem) []pdf.TableItem {
	out := make([]pdf.TableItem, len(in))
	for i := range in {
		t := pdf.TableItem{
			Scale:    1.0,
			CropOffX: 0,
			CropOffY: 0,
			Caption:  in[i].Caption,
		}
		t.Positions = append(t.Positions, in[i].Positions...)
		// Cells with text so ConstructTable takes the grid path and emits HTML.
		t.Cells = []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 10, Y1: 10, Text: "a"},
			{X0: 0, Y0: 12, X1: 10, Y1: 22, Text: "b"},
		}
		out[i] = t
	}
	return out
}

// ── equivalence tests ──────────────────────────────────────────────────────

// TestCollectTableBoxesByPageEquivalence asserts the page-bucketed box selection
// returns exactly the same boxes, in the same order, as the brute-force scan,
// over randomized multi-page documents that include page-agnostic positions and
// page-less boxes.
func TestCollectTableBoxesByPageEquivalence(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const pages = 12
	for seed := int64(0); seed < 50; seed++ {
		rng.Seed(seed)
		tables := buildTablesForTest(rng, pages, 60, 3)
		boxes := buildBoxesForCollectTest(rng, pages, 800, tables)
		byPage, noPage := indexTableLayoutBoxes(boxes)
		for ti := range tables {
			got := collectTableBoxes(boxes, tables[ti], byPage, noPage)
			want := referenceCollectTableBoxes(boxes, tables[ti])
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("seed=%d table=%d: collectTableBoxes mismatch\n got=%d boxes\nwant=%d boxes",
					seed, ti, len(got), len(want))
			}
		}
	}
}

// TestBuildTableHTMLsByPageEquivalence asserts the optimized buildTableHTMLs
// produces byte-identical HTML to the brute-force reference, end to end.
func TestBuildTableHTMLsByPageEquivalence(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	const pages = 10
	for seed := int64(0); seed < 20; seed++ {
		rng.Seed(seed)
		tables := buildTablesForTest(rng, pages, 40, 3)
		boxes := buildBoxesForCollectTest(rng, pages, 500, tables)
		gotTables := cloneTablesForHTML(tables)
		wantTables := cloneTablesForHTML(tables)
		got := buildTableHTMLs(boxes, gotTables)
		want := referenceBuildTableHTMLs(boxes, wantTables)
		if !maps.Equal(got, want) {
			t.Fatalf("seed=%d: buildTableHTMLs html mismatch\ngot =%v\nwant=%v", seed, got, want)
		}
	}
}

// ── benchmarks ─────────────────────────────────────────────────────────────

// benchDocWithCells builds a large multi-page document whose tables carry cells,
// so the benchmark measures the real box-scan path inside buildTableHTMLs.
func benchDocWithCells(pages, nBoxes, nTables, posPerTab int, seed int64) ([]pdf.TextBox, []pdf.TableItem) {
	rng := rand.New(rand.NewSource(seed))
	tables := buildTablesForTest(rng, pages, nTables, posPerTab)
	for ti := range tables {
		tables[ti].Scale = 1.0
		tables[ti].Cells = []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 10, Y1: 10, Text: "a"},
			{X0: 0, Y0: 12, X1: 10, Y1: 22, Text: "b"},
		}
	}
	return buildBoxesForCollectTest(rng, pages, nBoxes, tables), tables
}

func BenchmarkCollectTableBoxesIndexed(b *testing.B) {
	// Dense document: ~1000 boxes/page across 200 pages, 600 tables. This
	// mirrors a large PDF where the brute-force scan (every box, per table) is
	// the expensive path; the page-bucketed scan only touches the boxes on the
	// table's own pages.
	const pages, nBoxes, nTables, posPerTab = 200, 200000, 400, 3
	rng := rand.New(rand.NewSource(1))
	tables := buildTablesForTest(rng, pages, nTables, posPerTab)
	boxes := buildBoxesForCollectTest(rng, pages, nBoxes, tables)
	byPage, noPage := indexTableLayoutBoxes(boxes)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for ti := range tables {
			_ = collectTableBoxes(boxes, tables[ti], byPage, noPage)
		}
	}
}

func BenchmarkCollectTableBoxesBrute(b *testing.B) {
	const pages, nBoxes, nTables, posPerTab = 200, 200000, 400, 3
	rng := rand.New(rand.NewSource(1))
	tables := buildTablesForTest(rng, pages, nTables, posPerTab)
	boxes := buildBoxesForCollectTest(rng, pages, nBoxes, tables)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for ti := range tables {
			_ = referenceCollectTableBoxes(boxes, tables[ti])
		}
	}
}

func BenchmarkBuildTableHTMLsIndexed(b *testing.B) {
	const pages, nBoxes, nTables, posPerTab = 200, 200000, 400, 3
	boxes, tables := benchDocWithCells(pages, nBoxes, nTables, posPerTab, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildTableHTMLs(boxes, tables)
	}
}

func BenchmarkBuildTableHTMLsBrute(b *testing.B) {
	const pages, nBoxes, nTables, posPerTab = 200, 200000, 400, 3
	boxes, tables := benchDocWithCells(pages, nBoxes, nTables, posPerTab, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = referenceBuildTableHTMLs(boxes, tables)
	}
}
