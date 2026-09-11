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
