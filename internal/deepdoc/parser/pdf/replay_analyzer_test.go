//go:build cgo && manual

package pdf

import (
	"context"
	"image"
	"path/filepath"
	"sort"

	"ragflow/internal/deepdoc/parser/pdf/table"
	"ragflow/internal/deepdoc/parser/pdf/tool"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// PythonIntermediateDocAnalyzer replays Python's pre-computed DLA layout
// regions so the parity harness can exercise Go's assembly logic against
// Python's inference output. It is the DocAnalyzer half of the
// intermediate-replay paradigm (see pipeline_parity_test.go).
//
// DLA and page-level OCR are wired here. TSR is delivered by the paired
// PythonIntermediateTableBuilder (selected via the TableBuilder factory),
// and table-rotation OCR (self.ocr(img) directly in Python) is out of scope.
// OCRDetect returns the Python-dumped detect boxes for the page; OCRRecognize
// returns the Python-dumped recognized text for the box identified by the
// ocrBoxIdxCtxKey stamped in ocrDetectAndRecognize.
type PythonIntermediateDocAnalyzer struct {
	// name is the PDF base name INCLUDING the .pdf extension, matching the
	// dump file naming (e.g. "06_table_content.pdf").
	name   string
	dlaDir string
	tsrDir string
	ocrDir string
	// dims carries per-page image dimensions [w,h] in ZM pixels (from
	// charspy), used by TSR replay to derive Python's page_cum_height.
	dims    map[int][2]int
	healthy bool
}

// NewPythonIntermediateDocAnalyzer builds a replay analyzer rooted at the
// given dump directories. dlaDir/tsrDir/ocrDir point at
// output/py/ocr/{dla,tsr_raw,ocr}; dims is the charspy per-page image
// dimensions (PythonCharEngine.PageDims).
func NewPythonIntermediateDocAnalyzer(name, dlaDir, tsrDir, ocrDir string, dims map[int][2]int) *PythonIntermediateDocAnalyzer {
	return &PythonIntermediateDocAnalyzer{name: name, dlaDir: dlaDir, tsrDir: tsrDir, ocrDir: ocrDir, dims: dims, healthy: true}
}

func (a *PythonIntermediateDocAnalyzer) DLA(ctx context.Context, _ image.Image) ([]pdf.DLARegion, error) {
	pg, _ := ctx.Value(pageNumCtxKey).(int)
	pages, err := tool.LoadPythonDLA(filepath.Join(a.dlaDir, a.name+".json"))
	if err != nil {
		return nil, err
	}
	for _, p := range pages {
		if p.Page != pg {
			continue
		}
		out := make([]pdf.DLARegion, len(p.Regions))
		for i, r := range p.Regions {
			out[i] = r.ToDLARegion()
		}
		return out, nil
	}
	return nil, nil
}

// TSR is unused: the pipeline reaches TSR via the TableBuilder, not here.
func (a *PythonIntermediateDocAnalyzer) TSR(_ context.Context, _ image.Image) ([]pdf.TSRCell, error) {
	return nil, nil
}

// OCRDetect returns the Python-dumped raw detect boxes for the current page
// (image-pixel space, matching the page image handed to the analyzer).
// Missing dump or page → nil, letting the pipeline fall back to the
// char-derived path just as it does when OCR returns nothing.
func (a *PythonIntermediateDocAnalyzer) OCRDetect(ctx context.Context, _ image.Image) ([]pdf.OCRBox, error) {
	pg, _ := ctx.Value(pageNumCtxKey).(int)
	pages, err := tool.LoadPythonOCR(filepath.Join(a.ocrDir, a.name+".json"))
	if err != nil {
		return nil, err
	}
	// OCR dump Y is page-cumulative (page_cum_height added in __ocr), like
	// the TSR dump; derive the cumulative offset from charspy page dims and
	// subtract so boxes land on the current page before the crop shift.
	cumOffsetPx := 0.0
	for p := 0; p < pg; p++ {
		if d, ok := a.dims[p]; ok {
			cumOffsetPx += float64(d[1])
		}
	}
	// The OCR dump keys pages 1-based (Python's __ocr receives i+1), while
	// ctx pageNumCtxKey is 0-based — match pg+1.
	for _, p := range pages {
		if p.Page != pg+1 {
			continue
		}
		out := make([]pdf.OCRBox, len(p.Boxes))
		for i, b := range p.Boxes {
			out[i] = b.ToOCRBox(cumOffsetPx)
		}
		return out, nil
	}
	return nil, nil
}

// OCRRecognize returns the Python-dumped recognized text for the detect box
// whose index is stamped in ctx (ocrBoxIdxCtxKey) by ocrDetectAndRecognize.
// All layer-2 rotation candidates resolve to the same text, so the
// score-based rotation selection is a no-op over identical inputs.
func (a *PythonIntermediateDocAnalyzer) OCRRecognize(ctx context.Context, _ image.Image) ([]pdf.OCRText, error) {
	pg, _ := ctx.Value(pageNumCtxKey).(int)
	bi, _ := ctx.Value(ocrBoxIdxCtxKey).(int)
	if bi < 0 {
		return nil, nil
	}
	pages, err := tool.LoadPythonOCR(filepath.Join(a.ocrDir, a.name+".json"))
	if err != nil {
		return nil, err
	}
	// OCR dump pages are 1-based; pageNumCtxKey is 0-based (see OCRDetect).
	for _, p := range pages {
		if p.Page != pg+1 {
			continue
		}
		if bi >= len(p.Boxes) {
			return nil, nil
		}
		b := p.Boxes[bi]
		return []pdf.OCRText{{Text: b.Text, Confidence: b.Conf}}, nil
	}
	return nil, nil
}
func (a *PythonIntermediateDocAnalyzer) Health() bool { return a.healthy }

// PythonIntermediateTableBuilder replays Python's raw TSR components for a
// single (page, table) identified via ctx. DetectCells returns the
// components in Go's crop space; GroupCells delegates to the production
// grouping so the parity harness measures Go's assembly, not a re-implementaion.
type PythonIntermediateTableBuilder struct {
	analyzer *PythonIntermediateDocAnalyzer
}

// DetectCells returns the Python TSR components for the (page, table_index)
// carried in ctx, mapped from PDF-point space into Go's crop-image space
// using the crop origin stamped by processOneTable.
//
// Two Python→Go mapping quirks are handled here:
//  1. Python's table_index is a document-global counter (pdf_parser.py:590),
//     while Go's tableIdxCtxKey is the per-page ordinal (table_extract.go:97).
//     The ordinal is mapped to the Nth distinct global index on this page.
//  2. Python adds page_cum_height to TSR Y (pdf_parser.py:572-573), so the
//     cumulative offset is derived from charspy page dims and subtracted
//     before the crop shift, keeping cells in page-local space.
func (b *PythonIntermediateTableBuilder) DetectCells(ctx context.Context, _ image.Image) ([]pdf.TSRCell, error) {
	pg, _ := ctx.Value(pageNumCtxKey).(int)
	ti, _ := ctx.Value(tableIdxCtxKey).(int)
	cropOffX, _ := ctx.Value(cropOffXKey).(float64)
	cropOffY, _ := ctx.Value(cropOffYKey).(float64)

	cells, err := tool.LoadPythonTSR(filepath.Join(b.analyzer.tsrDir, b.analyzer.name+".json"))
	if err != nil {
		return nil, err
	}

	// Map the per-page ordinal ti to the Nth distinct global table index on
	// this page. The dump's global indices are monotonic per page, so sorting
	// and de-duplicating yields ordinal order.
	var globalIdx []int
	for _, c := range cells {
		if c.Page == pg {
			globalIdx = append(globalIdx, c.TableIndex)
		}
	}
	sort.Ints(globalIdx)
	globalIdx = compactInts(globalIdx)
	if ti < 0 || ti >= len(globalIdx) {
		return nil, nil
	}
	wantIdx := globalIdx[ti]

	// page_cum_height[pg] = sum of prior page image heights ÷ ZM
	// (pdf_parser.py:1690). dims hold the ZM-pixel page image heights, so the
	// cumulative offset in image pixels is the raw sum.
	cumOffsetPx := 0.0
	for p := 0; p < pg; p++ {
		if d, ok := b.analyzer.dims[p]; ok {
			cumOffsetPx += float64(d[1])
		}
	}

	var out []pdf.TSRCell
	for _, c := range cells {
		if c.Page != pg || c.TableIndex != wantIdx {
			continue
		}
		out = append(out, c.ToTSRCell(cropOffX, cropOffY, cumOffsetPx))
	}
	return out, nil
}

// compactInts removes consecutive duplicates from a sorted slice.
func compactInts(in []int) []int {
	out := in[:0]
	var prev int
	first := true
	for _, v := range in {
		if first || v != prev {
			out = append(out, v)
			prev = v
			first = false
		}
	}
	return out
}

// GroupCells uses the production cross-product grouping so the replay
// measures Go's grid assembly against Python's raw TSR components.
func (b *PythonIntermediateTableBuilder) GroupCells(cells []pdf.TSRCell) [][]pdf.TSRCell {
	// DeepDocTableBuilder.GroupCells is receiver-independent (it only reads
	// the passed cells), so a zero builder delegates to production logic.
	return (&table.DeepDocTableBuilder{}).GroupCells(cells)
}

func (b *PythonIntermediateTableBuilder) Name() string { return "py-intermediate" }

// ApplyRCToResult rewrites each table's Grid in result using Python's
// authoritative per-char R/C labels (from the table_boxes/ dump) instead of
// Go's TSR-line cross-product grouping. This is the row-segmentation fix
// under replay: Go's production path (DeepDocTableBuilder.GroupCells) counts
// rows from TSR "table row" structural lines, which over-segments relative
// to Python's R/C view; feeding the dumped R/C into GroupBoxesByRC aligns
// Go's row count to TSR truth.
//
// Mapping: result tables carry Page + Region (page-local PDF points). The
// dump boxes carry page_number (1-based) + R/C with PAGE-CUMULATIVE Y
// (page_cum_height added in Python's _table_transformer_job), so each group's
// bbox is converted back to page-local PDF points (cumOffsetPdf) before the
// region-overlap match. Boxes with no real R/C annotation are skipped so
// Go's line-based grid is left untouched for that table.
//
// A missing dump (file not found) is a no-op: PDFs without tables, or the
// legacy ocr_real dump captured before the R/C capture was added, keep their
// original grids.
func ApplyRCToResult(result *pdf.ParseResult, tableBoxesDir, name string, pageDims map[int][2]int) {
	boxes, err := tool.LoadPythonTableBoxes(filepath.Join(tableBoxesDir, name+".json"))
	if err != nil {
		return // no R/C dump → leave Go's line-based grids as-is
	}

	// Group dump boxes by (page, layoutno) — Python's authoritative per-table
	// key (layoutno = f"{page}-{layoutno}", pdf_parser.py:1298). R resets per
	// table, so region-overlap splitting mis-assigns boxes when Go's DLA
	// over-detects tables (e.g. icbccs: Go 14 vs Python 4). Grouping by
	// layoutno is the only correct split.
	type tkey struct {
		page     int
		layoutno string
	}
	groups := make(map[tkey][]pdf.TextBox)
	for _, b := range boxes {
		k := tkey{page: b.PageNumber - 1, layoutno: b.LayoutNo}
		groups[k] = append(groups[k], b)
	}

	// pageSet returns the 0-based pages this table spans AFTER cross-page
	// merge. Go's MergeTablesAcrossPages mirrors Python's merge decision, so
	// the Positions it appends are the authoritative set of continuation
	// pages: a cross-page table (13_crosspage: pages 1-3) includes its
	// continuation pages' dump groups, while two same-layoutno tables that
	// Python keeps separate (icbccs page5/6) do NOT end up in one page set.
	pageSetOf := func(item *pdf.TableItem) map[int]bool {
		ps := map[int]bool{item.Page: true}
		for _, p := range item.Positions {
			for _, pn := range p.PageNumbers {
				ps[pn] = true
			}
		}
		return ps
	}

	// cumOffsetPdf returns the cumulative height (PDF points) added to box Y
	// before page `page`. Python's table boxes carry page-cumulative Y
	// (page_cum_height added in _table_transformer_job), while Go's
	// TableItem.Region is page-local, so routing overlap must subtract this
	// offset to land in Go's page-local point space.
	cumOffsetPdf := func(page int) float64 {
		s := 0.0
		for p := 0; p < page; p++ {
			if d, ok := pageDims[p]; ok {
				s += float64(d[1])
			}
		}
		return s / pdf.DlaScale
	}

	// groupBBox computes a dump group's axis-aligned bounding box in
	// page-local PDF points (converting the dump's page-cumulative Y).
	groupBBox := func(gb []pdf.TextBox) (x0, y0, x1, y1 float64) {
		x0, y0, x1, y1 = gb[0].X0, gb[0].Top-cumOffsetPdf(gb[0].PageNumber-1), gb[0].X1, gb[0].Bottom-cumOffsetPdf(gb[0].PageNumber-1)
		for _, b := range gb[1:] {
			off := cumOffsetPdf(b.PageNumber - 1)
			x0 = minf(x0, b.X0)
			y0 = minf(y0, b.Top-off)
			x1 = maxf(x1, b.X1)
			y1 = maxf(y1, b.Bottom-off)
		}
		return x0, y0, x1, y1
	}

	// For each result table, find the dump group on its page set whose bbox
	// overlaps its region most (this fixes the layoutno identity), then
	// rebuild the grid from every group on those pages with that layoutno —
	// a cross-page table consumes all its continuation pages' R/C at once.
	for i := range result.Tables {
		item := &result.Tables[i]
		pages := pageSetOf(item)
		var bestK tkey
		bestOv := 0.0
		for k, gb := range groups {
			if !pages[k.page] {
				continue
			}
			gx0, gy0, gx1, gy1 := groupBBox(gb)
			ov := regionOverlapFrac(item.RegionLeft, item.RegionRight, item.RegionTop, item.RegionBottom, gx0, gy0, gx1, gy1)
			if ov > bestOv {
				bestOv, bestK = ov, k
			}
		}
		if bestOv <= 0 || len(groups[bestK]) == 0 || !table.BoxesHaveAnnotations(groups[bestK]) {
			continue // no R/C signal for this table → keep Go grid
		}
		// Collect this logical table's boxes: the primary group plus any
		// same-layoutno continuation groups on the table's other pages.
		var tableBoxes []pdf.TextBox
		for k, gb := range groups {
			if !pages[k.page] || k.layoutno != bestK.layoutno {
				continue
			}
			tableBoxes = append(tableBoxes, gb...)
			delete(groups, k) // consumed — avoid double-assigning one group
		}
		grid := table.GroupBoxesByRC(tableBoxes)
		if len(grid) == 0 {
			continue
		}
		// Mirror Python's construct_table post-processing: drop empty rows and
		// clean up orphan rows so the row count matches Python's golden.
		grid = table.DropAllEmptyRows(grid)
		grid = table.CleanupOrphanRows(grid)
		if len(grid) == 0 {
			continue
		}
		// Only override Go's assembled grid when it actually over/under-
		// segments relative to TSR's R/C truth. Where the row counts agree,
		// Go's production grid is kept: it is assembled by the line-based
		// path that also reproduces Python's _naive_vertical_merge cell fill
		// (e.g. 13_crosspage, whose line-based grid already holds gridSim=100%
		// and structSim=100%, while a blind R/C rebuild loses the merged-cell
		// text and drops to gridSim=99.8%).
		if len(grid) == len(item.Grid) {
			continue
		}
		// Python's construct_table emits a uniform-width grid: every row spans
		// the table-wide max column count, with missing cells rendered as empty
		// (<td></td>). GroupBoxesByRC sizes each row to its own max C, so pad
		// shorter rows to the table-wide max for structure parity.
		maxCols := 0
		for _, r := range grid {
			if len(r) > maxCols {
				maxCols = len(r)
			}
		}
		for ri := range grid {
			for len(grid[ri]) < maxCols {
				grid[ri] = append(grid[ri], pdf.TSRCell{})
			}
		}
		item.Grid = grid
	}
}

// regionOverlapFrac returns the overlap fraction between the axis-aligned
// region (L,R,T,B) and a bounding box (x0,y0,x1,y1), all in page-local PDF
// points.
func regionOverlapFrac(L, R, T, B, x0, y0, x1, y1 float64) float64 {
	ix0, ix1 := maxf(x0, L), minf(x1, R)
	iy0, iy1 := maxf(y0, T), minf(y1, B)
	iw, ih := ix1-ix0, iy1-iy0
	if iw <= 0 || ih <= 0 {
		return 0
	}
	gw, gh := x1-x0, y1-y0
	if gw <= 0 || gh <= 0 {
		return 0
	}
	return (iw * ih) / (gw * gh)
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// RegisterReplayTableBuilder installs a TableBuilder factory that returns the
// replay builder whenever the DocAnalyzer is a PythonIntermediateDocAnalyzer,
// and otherwise falls back to the production DeepDoc builder. Safe to call
// repeatedly; the factory is keyed on the analyzer type so it never alters
// behavior for non-replay parses.
func RegisterReplayTableBuilder() {
	RegisterTableBuilder(func(doc pdf.DocAnalyzer) pdf.TableBuilder {
		if a, ok := doc.(*PythonIntermediateDocAnalyzer); ok {
			return &PythonIntermediateTableBuilder{analyzer: a}
		}
		return table.NewDeepDocTableBuilder(doc)
	})
}
