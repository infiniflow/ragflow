package table

import (
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"reflect"
	"testing"
)

// =============================================================================
// Table header-detection parity with Python (deepdoc/vision/
// table_structure_recognizer.py:336-348). Pure functions, no external deps →
// unit tier (no build tag).
//
// Python marks a header row when, over its columns, more than half carry a
// geometric H flag (box overlaps the header region ≥0.3) OR, for numeric tables,
// a non-numeric blockType — each with a per-row h/cnt > 0.5 majority.
//
// Go's HeaderSetWithBlockType combines THREE additive signals (geometric,
// blockType, TSR label), each with the same >0.5 row-majority, so a row is a
// header if ANY signal flags it.
// =============================================================================

// TestHeaderDetection_GeometricPathAndProductionPathAgree verifies the
// production TSR-grid path (HeaderSetWithBlockType) and the box-geometric path
// (BoxHeaderSet) agree on the same table: a non-numeric, unlabeled header row
// whose boxes overlap the header region must be detected by both.
func TestHeaderDetection_GeometricPathAndProductionPathAgree(t *testing.T) {
	// Same 2-row, 2-col table, two views:
	//  - TSR grid: row 0 cells have NO "header" label (TSR missed them).
	//  - boxes: row 0 boxes carry H>0 (they geometrically overlap the header
	//    region by ≥0.3, exactly what Py's find_overlapped_with_threshold does).
	rows := [][]pdf.TSRCell{
		{
			{Text: "姓名", Label: "table row"},
			{Text: "年龄", Label: "table row"},
		},
		{
			{Text: "张三", Label: "table row"},
			{Text: "25", Label: "table row"},
		},
	}
	boxes := []pdf.TextBox{
		{Text: "姓名", R: 0, C: 0, H: 1}, // geometric overlap with header region
		{Text: "年龄", R: 0, C: 1, H: 1},
		{Text: "张三", R: 1, C: 0, H: -1},
		{Text: "25", R: 1, C: 1, H: -1},
	}

	gridHdrs := HeaderSetWithBlockType(rows, boxes) // production path (geometric-aware)
	boxHdrs := BoxHeaderSet(rows, boxes)            // geometric path

	// The table is all-or-nothing per row (row 0 fully overlaps, row 1 none),
	// so the column-majority production path and the any-box geometric path must
	// agree exactly. Assert equality and that the data row is not a header.
	if !reflect.DeepEqual(boxHdrs, gridHdrs) {
		t.Errorf("geometric path and production path disagree on header set: box=%v grid=%v", boxHdrs, gridHdrs)
	}
	if gridHdrs[1] {
		t.Errorf("data row 1 must not be a header: %v", gridHdrs)
	}
	t.Logf("box-geometric header set=%v production header set=%v", boxHdrs, gridHdrs)
}

// TestHeaderSetWithBlockType_LabelFallbackOverDetects_NoMajority verifies the
// TSR-label signal uses a per-row >0.5 majority: a data row with a single
// mislabeled cell must stay data, not be promoted to a header.
func TestHeaderSetWithBlockType_LabelFallbackOverDetects_NoMajority(t *testing.T) {
	rows := [][]pdf.TSRCell{
		{
			{Text: "姓名", Label: "table column header"},
			{Text: "年龄", Label: "table column header"},
		},
		{
			{Text: "张三", Label: "table row"},
			{Text: "25", Label: "table column header"}, // mislabeled data cell
		},
		{
			{Text: "李四", Label: "table row"},
			{Text: "30", Label: "table row"},
		},
	}

	hdrs := HeaderSetWithBlockType(rows, nil)

	if !hdrs[0] {
		t.Errorf("expected row 0 to be a header")
	}
	// Py: row 1 has 1/2 header-labeled cells → 0.5 not exceeded → still data.
	// Before fix: label fallback flagged ANY row with ≥1 header cell → row 1 IS header.
	if hdrs[1] {
		t.Errorf("PARITY #4 (asym 2) REGRESSED: Go marks data row 1 as header because the label fallback lost its 0.5 majority vote. Py keeps row 1 as data (1/2 < 0.5). header set=%v", hdrs)
	}
	t.Logf("header set=%v", hdrs)
}

// TestHeaderSetWithBlockType_ExclusiveGateSuppressesLabelHeaders verifies the
// three signals are additive: a numeric-dominant table whose row 0 is found by
// blockType must still let a numeric-but-labeled header row (row 2) be detected
// by the label signal.
func TestHeaderSetWithBlockType_ExclusiveGateSuppressesLabelHeaders(t *testing.T) {
	rows := [][]pdf.TSRCell{
		{
			{Text: "项目", Label: "table column header"}, // non-Nu → blockType hits
			{Text: "金额", Label: "table column header"}, // non-Nu → blockType hits
		},
		{
			{Text: "100", Label: "table row"},
			{Text: "200", Label: "table row"},
		},
		{
			{Text: "300", Label: "table column header"}, // numeric but labeled header
			{Text: "400", Label: "table column header"},
		},
	}

	hdrs := HeaderSetWithBlockType(rows, nil)

	if !hdrs[0] {
		t.Errorf("expected row 0 to be a header (blockType)")
	}
	// Row 2: blockType skips numeric cells, but the label signal must still flag it.
	if !hdrs[2] {
		t.Errorf("PARITY #4 (asym 3) REGRESSED: Go misses row 2 header. blockType found row 0 (len(hdrs)!=0) so the label fallback was suppressed; row 2's numeric-but-labeled cells are skipped by blockType. Py would flag row 2 via geometric H. header set=%v", hdrs)
	}
}

// TestHeaderSetWithBlockType_NonNumericUnlabeledDetectsHeaders verifies a
// non-numeric, unlabeled header row is detected via the geometric signal (box
// overlaps the header region ≥0.3) even when blockType and label contribute
// nothing.
func TestHeaderSetWithBlockType_NonNumericUnlabeledDetectsHeaders(t *testing.T) {
	rows := [][]pdf.TSRCell{
		{
			{Text: "姓名", Label: "table row"},
			{Text: "年龄", Label: "table row"},
		},
		{
			{Text: "张三", Label: "table row"},
			{Text: "25", Label: "table row"},
		},
		{
			{Text: "李四", Label: "table row"},
			{Text: "30", Label: "table row"},
		},
	}
	// Row 0 boxes geometrically overlap the header region by ≥0.3 (box.H > 0),
	// exactly what Py's find_overlapped_with_threshold produces.
	boxes := []pdf.TextBox{
		{Text: "姓名", R: 0, C: 0, H: 1},
		{Text: "年龄", R: 0, C: 1, H: 1},
		{Text: "张三", R: 1, C: 0, H: -1},
		{Text: "25", R: 1, C: 1, H: -1},
		{Text: "李四", R: 2, C: 0, H: -1},
		{Text: "30", R: 2, C: 1, H: -1},
	}

	hdrs := HeaderSetWithBlockType(rows, boxes)

	// Geometric signal alone flags row 0 (majority of its boxes overlap the
	// header region). blockType (non-Nu table) and label (no "header" label)
	// contribute nothing here, so this proves the geometric path is wired in.
	if !hdrs[0] {
		t.Errorf("PARITY #4 (geometric gap) FIX REGRESSED: non-numeric, unlabeled header row 0 should be detected via geometric H (box overlaps header region ≥0.3). header set=%v", hdrs)
	}
	if hdrs[1] || hdrs[2] {
		t.Errorf("PARITY #4: data rows 1/2 must NOT be headers. header set=%v", hdrs)
	}
	t.Logf("header set=%v", hdrs)
}

// TestHeaderSetWithBlockType_GeometricColumnMajority verifies the geometric
// signal applies Python's per-row column-majority (>0.5), not the old any-box
// rule: a row with a minority of overlapping boxes must NOT be a header, while a
// row with a majority must.
func TestHeaderSetWithBlockType_GeometricColumnMajority(t *testing.T) {
	t.Run("one of two overlapping is not a header", func(t *testing.T) {
		rows := [][]pdf.TSRCell{
			{
				{Text: "HeaderA", Label: "table row"},
				{Text: "HeaderB", Label: "table row"},
			},
			{
				{Text: "DataA", Label: "table row"},
				{Text: "DataB", Label: "table row"},
			},
		}
		// Row 0: both boxes overlap the header region. Row 1: only ONE of two
		// boxes overlaps (1/2 = 0.5, not > 0.5).
		boxes := []pdf.TextBox{
			{Text: "HeaderA", R: 0, C: 0, H: 1},
			{Text: "HeaderB", R: 0, C: 1, H: 1},
			{Text: "DataA", R: 1, C: 0, H: 1}, // single stray overlap
			{Text: "DataB", R: 1, C: 1, H: -1},
		}
		hdrs := HeaderSetWithBlockType(rows, boxes)
		if !hdrs[0] {
			t.Errorf("row 0 (2/2 overlap) should be a header: %v", hdrs)
		}
		if hdrs[1] {
			t.Errorf("row 1 (1/2 overlap) must NOT be a header: %v", hdrs)
		}
	})

	t.Run("two of three overlapping is a header", func(t *testing.T) {
		rows := [][]pdf.TSRCell{
			{
				{Text: "HeaderA", Label: "table row"},
				{Text: "HeaderB", Label: "table row"},
				{Text: "HeaderC", Label: "table row"},
			},
			{
				{Text: "DataA", Label: "table row"},
				{Text: "DataB", Label: "table row"},
				{Text: "DataC", Label: "table row"},
			},
		}
		// Row 1: two of three boxes overlap (2/3 > 0.5 → header).
		boxes := []pdf.TextBox{
			{Text: "HeaderA", R: 0, C: 0, H: 1},
			{Text: "HeaderB", R: 0, C: 1, H: 1},
			{Text: "HeaderC", R: 0, C: 2, H: 1},
			{Text: "DataA", R: 1, C: 0, H: 1},
			{Text: "DataB", R: 1, C: 1, H: 1},
			{Text: "DataC", R: 1, C: 2, H: -1},
		}
		hdrs := HeaderSetWithBlockType(rows, boxes)
		if !hdrs[0] {
			t.Errorf("row 0 (3/3 overlap) should be a header: %v", hdrs)
		}
		if !hdrs[1] {
			t.Errorf("row 1 (2/3 overlap) should be a header: %v", hdrs)
		}
	})
}

// TestAnnotateTableBoxes_FirstHeaderCellEncoded verifies AnnotateTableBoxes
// encodes a box matching the FIRST header cell (idx==0) as H==1 (not H==0), so
// single-column / first-column header tables are not dropped by the H>0 readers.
func TestAnnotateTableBoxes_FirstHeaderCellEncoded(t *testing.T) {
	// Single-column, two-row table. grid[0] is the header row and contains
	// exactly one cell at index 0.
	cells := []pdf.TSRCell{
		{X0: 10, Y0: 10, X1: 90, Y1: 30, Label: "table column header"}, // header, idx 0
		{X0: 10, Y0: 30, X1: 90, Y1: 50, Label: "table row"},           // data
	}
	// Box exactly overlaps the header cell → R=0 and, against headers (grid[0]),
	// findOverlappedWithThreshold returns idx==0.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 10, Bottom: 30, LayoutType: pdf.LayoutTypeTable, Text: "姓名"},
		{X0: 10, X1: 90, Top: 30, Bottom: 50, LayoutType: pdf.LayoutTypeTable, Text: "张三"}, // data box, no header overlap
	}

	AnnotateTableBoxes(boxes, GroupTSRCellsToRows(cells))

	// The header box matches the FIRST header cell (idx==0); AnnotateTableBoxes
	// encodes it as H == idx+1 == 1.
	if boxes[0].H != 1 {
		t.Errorf("PARITY #4 (idx+1 encoding) REGRESSED: box matching the first header cell (idx==0) should be stored as H=1, got H=%d. Old code wrote H=0 and was dropped by the b.H>0 readers.", boxes[0].H)
	}
	if boxes[0].H <= 0 {
		t.Errorf("header box must read as overlapped via BoxHeaderSet's b.H>0 rule; got H=%d", boxes[0].H)
	}
	// The data box does not overlap the header region, so it must stay H==0.
	if boxes[1].H != 0 {
		t.Errorf("data box must NOT be flagged as header overlap; got H=%d", boxes[1].H)
	}

	// Tie it to the production reader: BoxHeaderSet must now flag row 0.
	rows := GroupTSRCellsToRows(cells)
	hdrs := BoxHeaderSet(rows, boxes)
	if !hdrs[0] {
		t.Errorf("BoxHeaderSet should flag row 0 via box.H>0 (idx+1 encoding); header set=%v", hdrs)
	}
	if hdrs[1] {
		t.Errorf("BoxHeaderSet must NOT flag data row 1; header set=%v", hdrs)
	}
	t.Logf("header box H=%d data box H=%d header set=%v", boxes[0].H, boxes[1].H, hdrs)
}
