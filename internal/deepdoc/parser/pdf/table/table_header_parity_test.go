package table

import (
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"testing"
)

// =============================================================================
// Parity #4: header 行判定机制不同 (registered in project_table_parity_triage.md)
//
// 纯函数测试，无外部依赖 → unit 层（无 build tag）。
//
// Python (deepdoc/vision/table_structure_recognizer.py:336-348) 主路径 =
//   any(a.get("H") for a in arr)            # 单元格内任一 box 与 header 区域
//                                           # 重叠 ≥0.3 (pdf_parser.py:616 打 H)
//   + (max_type=="Nu" && arr[0]["btype"]!="Nu")   # blockType 仅数值表
//   行级 h/cnt > 0.5 多数决。
//
// Go 生产路径 (table_cells.go:248 HeaderSetWithBlockType, table_construct.go:55)
//   主路径仅 maxType=="Nu" && bt!="Nu"；非数值表主路径 0 header；
//   靠 fallback 查 cell.Label 含 "header"(table_cells.go:293-301)。
//
// 以下测试暴露三条不对称：
//   (1) 生产路径无 0.3 几何口径 —— box 几何路径(BoxHeaderSet, 0.3)与 TSR grid
//       路径(HeaderSetWithBlockType)对同一张表得出不同 header 集。
//   (2) label fallback 无 0.5 多数决 —— 数据行内 1 个 cell 被打 header 标签即整行抬成 header。
//   (3) fallback 为互斥门控 —— blockType 已找到 ≥1 header 时 label fallback 被关闭，
//       纯 label 可识别的 header 行被漏掉。
// =============================================================================

// TestHeaderDetection_GeometricPathVsProductionPathDiverge exposes asymmetry (1):
// the box-geometric path (BoxHeaderSet, 0.3 IoU threshold) and the production
// TSR-grid path (HeaderSetWithBlockType) disagree on the SAME table.
//
// Py uses the geometric H flag (box overlaps header region ≥0.3) as a primary
// signal. Go's production path never sees geometric overlap — it only reads
// TSRCell.Label from GroupCells Y-intersection propagation (no 0.3 threshold).
// The result: a non-numeric header row with no TSR "header" label is detected by
// the geometric path but missed by the production path.
func TestHeaderDetection_GeometricPathVsProductionPathDiverge(t *testing.T) {
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

	gridHdrs := HeaderSetWithBlockType(rows, boxes) // production path (now geometric-aware)
	boxHdrs := BoxHeaderSet(rows, boxes)            // geometric path

	if boxHdrs[0] && !gridHdrs[0] {
		t.Errorf("PARITY #4 (asym 1) EXPOSED: box-geometric path flags row 0 (H>0, 0.3 overlap) but production HeaderSetWithBlockType misses it. Same table, divergent header sets: box=%v grid=%v. Py would flag row 0 via geometric H.", boxHdrs, gridHdrs)
	}
	t.Logf("box-geometric header set=%v production header set=%v", boxHdrs, gridHdrs)
}

// TestHeaderSetWithBlockType_LabelFallbackOverDetects_NoMajority exposes
// asymmetry (2): Go's label fallback has NO 0.5 majority vote. Py requires
// h/cnt > 0.5 (majority of non-empty cells) before a row becomes a header.
//
// Scenario: row 0 is a real header (all cells labeled). Row 1 is a DATA row,
// but exactly ONE cell was mislabeled "table column header" (e.g. a GroupCells
// Y-intersection false positive). Py keeps row 1 as data (1/2 < 0.5); Go flips
// the ENTIRE row to header because the fallback marks any row with ≥1 header cell.
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
	// Go: label fallback flags ANY row with ≥1 header cell → row 1 IS header.
	if hdrs[1] {
		t.Errorf("PARITY #4 (asym 2) EXPOSED: Go marks data row 1 as header because the label fallback has no 0.5 majority vote. Py keeps row 1 as data (1/2 < 0.5). header set=%v", hdrs)
	}
	t.Logf("header set=%v", hdrs)
}

// TestHeaderSetWithBlockType_ExclusiveGateSuppressesLabelHeaders exposes
// asymmetry (3): Go's label fallback only runs when len(hdrs)==0 (it is a
// mutually-exclusive gate). Py combines geometric H and blockType per-cell, then
// takes a row-level majority — the two signals are ADDITIVE, not gated.
//
// Scenario: a numeric-dominant (Nu) table. Row 0 header = non-Nu text, detected
// by blockType (maxType==Nu && bt!="Nu"). Row 2 is a header row whose cells are
// NUMERIC, so blockType skips them (correct per Py's Nu rule); it is only
// identifiable via its "header" label. Because blockType already populated hdrs
// (row 0), the label fallback is SUPPRESSED, so row 2 is dropped.
//
// NOTE: Py would flag row 2 via geometric H (its boxes overlap the header
// region ≥0.3). Even setting Py aside, Go's two signals being exclusive rather
// than additive is a structural divergence that drops label-only headers.
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
	// Row 2: blockType skips numeric cells; label would flag it but the
	// mutually-exclusive gate disabled the label fallback once row 0 was found.
	if !hdrs[2] {
		t.Errorf("PARITY #4 (asym 3) EXPOSED: Go misses row 2 header. blockType found row 0 (len(hdrs)!=0) so the label fallback is suppressed; row 2's numeric-but-labeled cells are skipped by blockType. Py would flag row 2 via geometric H. header set=%v", hdrs)
	}
}

// TestHeaderSetWithBlockType_NonNumericUnlabeledDetectsHeaders verifies the
// geometric gap is fixed end-to-end: a non-numeric table whose header row
// carries no TSR "header" label and whose content is non-Nu. Py flags the header
// row via the geometric H signal (box overlaps header region ≥0.3); Go's
// production path must now do the same by accepting the annotated boxes.
//
// Before the fix, HeaderSetWithBlockType had no access to geometric overlap and
// yielded ZERO headers for this table (parity #4, geometric gap). After the fix,
// the geometric signal (box.H > 0, set by AnnotateTableBoxes against the first
// grid row) is combined additively with blockType + label, so row 0 is detected.
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

// TestAnnotateTableBoxes_FirstHeaderCellEncoded covers the box.H = idx+1
// encoding in AnnotateTableBoxes (table_layout.go): a single-column table whose
// header is the FIRST header cell (idx==0).
//
// Before the fix AnnotateTableBoxes stored boxes[i].H = idx. For the first
// header cell idx==0, that wrote H==0, which every reader treats as "no header
// overlap" (b.H > 0). So single-column / first-column header tables were
// silently dropped by the geometric signal. The fix stores idx+1, so a match on
// the first header cell yields H==1 (>0).
//
// This test fails on the old code (expects H==1, old code yields H==0) and turns
// green with the encoding fix. It also confirms the boolean reader BoxHeaderSet
// now flags the row.
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

	// The header box matches the FIRST header cell (idx==0); the fix encodes it
	// as H == idx+1 == 1. The old code wrote H == 0 here.
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
