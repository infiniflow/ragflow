package table

import (
	"math/rand"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// buildSyntheticDoc constructs a multi-page document with tables that span a few
// consecutive pages, mirroring a real PDF: the number of (table box, table
// position) pairs the unindexed scan would enumerate is pages*boxes*tables,
// while the per-page index only tests boxes against positions on their own page.
func buildSyntheticDoc(pages, nBoxes, nTables, posPerTab int, seed int64) ([]pdf.TextBox, []pdf.TableItem) {
	rng := rand.New(rand.NewSource(seed))
	boxes := make([]pdf.TextBox, nBoxes)
	for i := 0; i < nBoxes; i++ {
		p := rng.Intn(pages) + 1
		x0 := rng.Float64() * 1000
		y0 := rng.Float64() * 1000
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
		start := rng.Intn(pages) + 1
		poss := make([]pdf.Position, 0, posPerTab)
		for k := 0; k < posPerTab; k++ {
			p := ((start + k - 1) % pages) + 1
			x0 := rng.Float64() * 1000
			y0 := rng.Float64() * 1000
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
	return boxes, tables
}

func BenchmarkBuildReplacementsAfterMergeIndexed(b *testing.B) {
	const pages, nBoxes, nTables, posPerTab = 200, 5000, 400, 3
	boxes, tables := buildSyntheticDoc(pages, nBoxes, nTables, posPerTab, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildReplacementsAfterMerge(boxes, tables, nil)
	}
}

func BenchmarkBuildReplacementsAfterMergeBrute(b *testing.B) {
	const pages, nBoxes, nTables, posPerTab = 200, 5000, 400, 3
	boxes, tables := buildSyntheticDoc(pages, nBoxes, nTables, posPerTab, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = referenceBuildReplacementsAfterMerge(boxes, tables, nil)
	}
}

func BenchmarkMarkNoMergeTablesIndexed(b *testing.B) {
	const pages, nBoxes, nTables, posPerTab = 200, 5000, 400, 3
	boxes, tables := buildSyntheticDoc(pages, nBoxes, nTables, posPerTab, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MarkNoMergeTables(boxes, tables)
	}
}
