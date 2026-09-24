package pdf

import (
	"context"
	"image"
	"strings"
	"testing"

	pdftype "ragflow/internal/deepdoc/parser/pdf/type"
)

type tableCandidateAnalyzer struct {
	*MockDocAnalyzer
	cellsByTable [][]pdftype.TSRCell
}

func (a *tableCandidateAnalyzer) TSR(ctx context.Context, _ image.Image) ([]pdftype.TSRCell, error) {
	index, ok := ctx.Value(tableIdxCtxKey).(int)
	if !ok || index < 0 || index >= len(a.cellsByTable) {
		return nil, nil
	}
	return a.cellsByTable[index], nil
}

func TestEnrichOnePageWithDeepDoc_UsesSplitRowsFromContainedCandidate(t *testing.T) {
	boxes := []pdftype.TextBox{
		{PageNumber: 0, X0: 10, X1: 80, Top: 20, Bottom: 30, Text: "A"},
		{PageNumber: 0, X0: 10, X1: 80, Top: 110, Bottom: 120, Text: "B"},
		{PageNumber: 0, X0: 10, X1: 80, Top: 200, Bottom: 210, Text: "C"},
	}
	analyzer := &tableCandidateAnalyzer{
		MockDocAnalyzer: &MockDocAnalyzer{Healthy: true, DLARegions: []pdftype.DLARegion{
			{X0: 0, Y0: 0, X1: 200, Y1: 250, Label: pdftype.LayoutTypeTable, Confidence: 0.9},
			{X0: 0, Y0: 90, X1: 200, Y1: 250, Label: pdftype.LayoutTypeTable, Confidence: 0.8},
		}},
		cellsByTable: [][]pdftype.TSRCell{
			{
				{X0: 0, Y0: 10, X1: 200, Y1: 40, Label: "table row"},
				{X0: 0, Y0: 100, X1: 200, Y1: 230, Label: "table row"},
				{X0: 0, Y0: 10, X1: 200, Y1: 230, Label: "table column"},
			},
			{
				{X0: 0, Y0: 40, X1: 200, Y1: 70, Label: "table row"},
				{X0: 0, Y0: 130, X1: 200, Y1: 160, Label: "table row"},
				{X0: 0, Y0: 40, X1: 200, Y1: 160, Label: "table column"},
			},
		},
	}
	parser := NewParser(pdftype.DefaultParserConfig())
	img := image.NewRGBA(image.Rect(0, 0, 300, 300))
	_, tables, _ := parser.enrichOnePageWithDeepDoc(t.Context(), img, boxes, 0, nil, analyzer, NewTableBuilderFor(analyzer), 1, nil)
	if len(tables) != 1 {
		for i, item := range tables {
			t.Logf("candidate %d positions=%d grid=%v", i, len(item.Positions), item.Grid)
		}
		t.Fatalf("overlapping crops of one table must produce one table, got %d", len(tables))
	}
	if len(tables[0].Grid) != 3 {
		t.Fatalf("split crop must repair the parent's merged B/C row, got %d rows", len(tables[0].Grid))
	}
	for i, want := range []string{"A", "B", "C"} {
		if got := strings.TrimSpace(tables[0].Grid[i][0].Text); got != want {
			t.Errorf("row %d = %q, want %q", i, got, want)
		}
	}
}

func TestEnrichOnePageWithDeepDoc_RemovesSharedBoundaryRow(t *testing.T) {
	boxes := []pdftype.TextBox{
		{PageNumber: 0, X0: 10, X1: 80, Top: 20, Bottom: 30, Text: "A"},
		{PageNumber: 0, X0: 10, X1: 80, Top: 110, Bottom: 120, Text: "88"},
		{PageNumber: 0, X0: 10, X1: 80, Top: 200, Bottom: 210, Text: "89"},
	}
	analyzer := &tableCandidateAnalyzer{
		MockDocAnalyzer: &MockDocAnalyzer{Healthy: true, DLARegions: []pdftype.DLARegion{
			{X0: 0, Y0: 0, X1: 200, Y1: 150, Label: pdftype.LayoutTypeTable, Confidence: 0.9},
			{X0: 0, Y0: 90, X1: 200, Y1: 250, Label: pdftype.LayoutTypeTable, Confidence: 0.8},
		}},
		cellsByTable: [][]pdftype.TSRCell{
			{
				{X0: 0, Y0: 10, X1: 200, Y1: 40, Label: "table row"},
				{X0: 0, Y0: 100, X1: 200, Y1: 130, Label: "table row"},
				{X0: 0, Y0: 10, X1: 200, Y1: 130, Label: "table column"},
			},
			{
				{X0: 0, Y0: 40, X1: 200, Y1: 70, Label: "table row"},
				{X0: 0, Y0: 130, X1: 200, Y1: 160, Label: "table row"},
				{X0: 0, Y0: 40, X1: 200, Y1: 160, Label: "table column"},
			},
		},
	}
	parser := NewParser(pdftype.DefaultParserConfig())
	img := image.NewRGBA(image.Rect(0, 0, 300, 300))
	_, tables, _ := parser.enrichOnePageWithDeepDoc(t.Context(), img, boxes, 0, nil, analyzer, NewTableBuilderFor(analyzer), 1, nil)
	if len(tables) != 2 {
		t.Fatalf("adjacent tables with unique rows must remain distinct, got %d", len(tables))
	}
	if len(tables[1].Grid) != 1 || strings.TrimSpace(tables[1].Grid[0][0].Text) != "89" {
		t.Fatalf("shared 88 row must be emitted only by the first table, got second grid %v", tables[1].Grid)
	}
	if len(tables[1].Positions) != 1 || tables[1].Positions[0].Top != 200 {
		t.Fatalf("second table must own only the 89 OCR box, got positions %v", tables[1].Positions)
	}
}

func TestEnrichOnePageWithDeepDoc_KeepsContainedTableWithDifferentColumns(t *testing.T) {
	boxes := []pdftype.TextBox{
		{PageNumber: 0, X0: 10, X1: 80, Top: 20, Bottom: 30, Text: "A"},
		{PageNumber: 0, X0: 10, X1: 80, Top: 110, Bottom: 120, Text: "B1"},
		{PageNumber: 0, X0: 120, X1: 190, Top: 110, Bottom: 120, Text: "B2"},
		{PageNumber: 0, X0: 10, X1: 80, Top: 200, Bottom: 210, Text: "C1"},
		{PageNumber: 0, X0: 120, X1: 190, Top: 200, Bottom: 210, Text: "C2"},
	}
	analyzer := &tableCandidateAnalyzer{
		MockDocAnalyzer: &MockDocAnalyzer{Healthy: true, DLARegions: []pdftype.DLARegion{
			{X0: 0, Y0: 0, X1: 200, Y1: 250, Label: pdftype.LayoutTypeTable, Confidence: 0.9},
			{X0: 0, Y0: 90, X1: 200, Y1: 250, Label: pdftype.LayoutTypeTable, Confidence: 0.8},
		}},
		cellsByTable: [][]pdftype.TSRCell{
			{
				{X0: 0, Y0: 10, X1: 200, Y1: 40, Label: "table row"},
				{X0: 0, Y0: 100, X1: 200, Y1: 230, Label: "table row"},
				{X0: 0, Y0: 10, X1: 200, Y1: 230, Label: "table column"},
			},
			{
				{X0: 0, Y0: 40, X1: 200, Y1: 70, Label: "table row"},
				{X0: 0, Y0: 130, X1: 200, Y1: 160, Label: "table row"},
				{X0: 0, Y0: 40, X1: 100, Y1: 160, Label: "table column"},
				{X0: 100, Y0: 40, X1: 200, Y1: 160, Label: "table column"},
			},
		},
	}
	parser := NewParser(pdftype.DefaultParserConfig())
	img := image.NewRGBA(image.Rect(0, 0, 300, 300))
	_, tables, _ := parser.enrichOnePageWithDeepDoc(t.Context(), img, boxes, 0, nil, analyzer, NewTableBuilderFor(analyzer), 1, nil)
	if len(tables) != 2 {
		t.Fatalf("a contained table with its own two-column structure must remain distinct, got %d tables", len(tables))
	}
}

func TestMergeContainedCandidateRows_SplitOCRBoxAcrossRows(t *testing.T) {
	parent := pdftype.TableItem{Scale: 1, Grid: [][]pdftype.TSRCell{
		{{Text: "A", Y0: 0, Y1: 10}},
		{{Text: "BC", Y0: 10, Y1: 30}},
	}}
	child := pdftype.TableItem{Scale: 1, Grid: [][]pdftype.TSRCell{
		{{Text: "B", Y0: 10, Y1: 20}},
		{{Text: "C", Y0: 20, Y1: 30}},
	}}
	if !mergeContainedCandidateRows(&parent, child, true) {
		t.Fatal("split rows with exactly the same characters should repair the coarse row")
	}
	if len(parent.Grid) != 3 || parent.Grid[1][0].Text != "B" || parent.Grid[2][0].Text != "C" {
		t.Fatalf("coarse row was not replaced by its lossless split: %v", parent.Grid)
	}
}

func TestMergeContainedCandidateRows_PreservesDifferentText(t *testing.T) {
	parent := pdftype.TableItem{Scale: 1, Grid: [][]pdftype.TSRCell{{{Text: "BC", Y0: 10, Y1: 30}}}}
	child := pdftype.TableItem{Scale: 1, Grid: [][]pdftype.TSRCell{{{Text: "B", Y0: 10, Y1: 20}}, {{Text: "D", Y0: 20, Y1: 30}}}}
	if mergeContainedCandidateRows(&parent, child, true) {
		t.Fatal("a candidate carrying different text must remain available")
	}
}

func TestCompatibleTableColumns_AllowsExtraAndDuplicateTSRColumns(t *testing.T) {
	makeItem := func(spans ...[2]float64) pdftype.TableItem {
		item := pdftype.TableItem{Scale: 1}
		for _, span := range spans {
			item.Cells = append(item.Cells, pdftype.TSRCell{X0: span[0], X1: span[1], Label: "table column"})
		}
		return item
	}
	parent := makeItem([2]float64{0, 20}, [2]float64{20, 40}, [2]float64{40, 60}, [2]float64{60, 80})
	fragment := makeItem([2]float64{1, 21}, [2]float64{21, 41}, [2]float64{41, 61}, [2]float64{41, 61}, [2]float64{60, 80})
	if !compatibleTableColumns(parent, fragment) {
		t.Fatal("an extra or repeated TSR column must not obscure matching structure")
	}
	independent := makeItem([2]float64{0, 40}, [2]float64{40, 80})
	if compatibleTableColumns(parent, independent) {
		t.Fatal("different column structure must keep the nested candidate")
	}
}

func TestMergeContainedCandidateRows_DropsDisjointRowsAlreadyInParent(t *testing.T) {
	parent := pdftype.TableItem{Scale: 1, Grid: [][]pdftype.TSRCell{
		{{Text: "A", Y0: 0, Y1: 10}}, {{Text: "B", Y0: 10, Y1: 20}},
		{{Text: "X", Y0: 20, Y1: 30}}, {{Text: "C", Y0: 30, Y1: 40}},
	}}
	child := pdftype.TableItem{Scale: 1, Grid: [][]pdftype.TSRCell{
		{{Text: "B", Y0: 10, Y1: 20}}, {{Text: "C", Y0: 30, Y1: 40}},
	}}
	if !mergeContainedCandidateRows(&parent, child, false) {
		t.Fatal("coarser crop with only existing rows should be dropped")
	}
	if len(parent.Grid) != 4 {
		t.Fatal("dropping duplicate rows must leave parent grid unchanged")
	}
}

func TestMergeContainedCandidateRows_DropsSubstringRowsWithMatchingColumns(t *testing.T) {
	parent := pdftype.TableItem{Scale: 1, Grid: [][]pdftype.TSRCell{{{Text: "ABC", Y0: 10, Y1: 20}}}}
	child := pdftype.TableItem{Scale: 1, Grid: [][]pdftype.TSRCell{{{Text: "BC", Y0: 10, Y1: 20}}}}
	if !mergeContainedCandidateRows(&parent, child, true) {
		t.Fatal("a cropped row already contained in the same parent row is duplicate")
	}
	if mergeContainedCandidateRows(&parent, child, false) {
		t.Fatal("different column structure requires a full-row text match")
	}
}

func TestMergeContainedCandidateRows_DoesNotReuseParentRow(t *testing.T) {
	parent := pdftype.TableItem{Scale: 1, Grid: [][]pdftype.TSRCell{{{Text: "B", Y0: 10, Y1: 30}}}}
	child := pdftype.TableItem{Scale: 1, Grid: [][]pdftype.TSRCell{{{Text: "B", Y0: 10, Y1: 20}}, {{Text: "B", Y0: 20, Y1: 30}}}}
	if mergeContainedCandidateRows(&parent, child, false) {
		t.Fatal("two child rows must not both claim the same parent row")
	}
}

func TestMergeContainedCandidateRows_DoesNotMoveValuesBetweenRows(t *testing.T) {
	parent := pdftype.TableItem{Scale: 1, Grid: [][]pdftype.TSRCell{
		{{Text: "12", Y0: 0, Y1: 10}}, {{Text: "34", Y0: 10, Y1: 20}},
	}}
	child := pdftype.TableItem{Scale: 1, Grid: [][]pdftype.TSRCell{
		{{Text: "13", Y0: 0, Y1: 6}}, {{Text: "2", Y0: 6, Y1: 12}}, {{Text: "4", Y0: 12, Y1: 20}},
	}}
	if mergeContainedCandidateRows(&parent, child, true) {
		t.Fatal("matching character totals alone cannot justify moving digits between rows")
	}
}

func TestMergeContainedCandidateRows_DifferentColumnsNeedMatchingRows(t *testing.T) {
	parent := pdftype.TableItem{Scale: 1, Grid: [][]pdftype.TSRCell{
		{{Text: "AB", Y0: 0, Y1: 10}}, {{Text: "CD", Y0: 10, Y1: 20}},
	}}
	child := pdftype.TableItem{Scale: 1, Grid: [][]pdftype.TSRCell{
		{{Text: "AC", Y0: 0, Y1: 10}}, {{Text: "BD", Y0: 10, Y1: 20}},
	}}
	if mergeContainedCandidateRows(&parent, child, false) {
		t.Fatal("a different column structure needs each row represented by the parent")
	}
}

func TestReconcileContainedPageTables_DropsCoarserShortFragment(t *testing.T) {
	columns := func(n int) []pdftype.TSRCell {
		out := make([]pdftype.TSRCell, n)
		for i := range out {
			out[i] = pdftype.TSRCell{X0: float64(i * 20), X1: float64((i + 1) * 20), Label: "table column"}
		}
		return out
	}
	region := pdftype.DLARegion{X0: 0, X1: 200, Y0: 0, Y1: 100}
	parent := pageTableCandidate{
		item: pdftype.TableItem{Scale: 1, Cells: columns(4), Grid: [][]pdftype.TSRCell{
			{{Text: "header", Y0: 0, Y1: 10}}, {{Text: "value", Y0: 20, Y1: 30}},
		}},
		boxIdx: []int{0, 1, 2}, region: region,
	}
	childColumns := []pdftype.TSRCell{
		{X0: 0, X1: 40, Label: "table column"}, {X0: 40, X1: 80, Label: "table column"},
	}
	child := pageTableCandidate{
		item: pdftype.TableItem{Scale: 1, Cells: childColumns, Grid: [][]pdftype.TSRCell{
			{{Text: "header", Y0: 0, Y1: 10}, {Y0: 0, Y1: 10}},
			{{Text: "value", Y0: 20, Y1: 30}, {Y0: 20, Y1: 30}},
		}},
		boxIdx: []int{1, 2}, region: region,
	}
	if compatibleTableColumns(parent.item, child.item) || !coarserShortCandidate(parent.item, child.item) {
		t.Fatal("fixture must exercise the coarser-column fallback")
	}
	if got := reconcileContainedPageTables([]pageTableCandidate{parent, child}, nil); len(got) != 1 {
		t.Fatalf("a short crop with coarser columns and identical rows is redundant, got %d candidates", len(got))
	}
}
