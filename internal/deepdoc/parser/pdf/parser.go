package pdf

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"sort"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/common"
	lyt "ragflow/internal/deepdoc/parser/pdf/layout"
	tbl "ragflow/internal/deepdoc/parser/pdf/table"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
	"ragflow/internal/utility"
)

// Parser is the core PDF text/layout extraction pipeline.
// It corresponds to RAGFlowPdfParser in pdf_parser.py.
// Stateless after construction — safe to reuse across documents.
type Parser struct {
	Config pdf.ParserConfig
}

// pageResult holds per-page worker-local artifacts produced by
// processAllPagesParallel. The struct is keyed by page number in a
// map so worker completion order does not affect collection order;
// final assembly sorts by page number before merging into ParseResult.
//
// Fatal vs recoverable policy:
//   - RenderPageToImage failure → recoverable: page may still produce
//     boxes from charsToBoxes fallback. The Err field carries the
//     render error for logging visibility.
//   - OCR detect/recognize failure → recoverable: page falls back to
//     embedded chars; Err carries the underlying failure if any.
//   - DLA/TSR failure → recoverable: page produces no tables for that
//     page; tables remain empty in the worker artifact.
//   - Worker-pool orchestration bugs → fatal: panic surfaces immediately.
type pageResult struct {
	PageNumber  int
	Boxes       []pdf.TextBox
	Chars       []pdf.TextChar
	MedianH     float64
	MedianW     float64
	IsEnglish   bool
	IsScanNoise bool
	OCRUsed     bool
	// PageHeight and PageWidth record the PDF-point dimensions of the
	// winning page render (pixel dimensions divided by the per-page
	// zoom). They are used by buildLayout without needing a zoom map.
	PageHeight float64
	PageWidth  float64
	Tables     []pdf.TableItem
	DLARegions []pdf.DLAPageRegions
	Err        error
}

// New creates a new Parser with the given config.
func NewParser(cfg pdf.ParserConfig) *Parser {
	return &Parser{Config: cfg}
}

// ── TableBuilder factory ───────────────────────────────────────────────────

var tableBuilderFactory func(pdf.DocAnalyzer) pdf.TableBuilder

// RegisterTableBuilder registers a TableBuilder factory for the PDF parser.
// EE packages call this from init() to inject EE-specific implementations.
func RegisterTableBuilder(factory func(pdf.DocAnalyzer) pdf.TableBuilder) {
	tableBuilderFactory = factory
}

func NewTableBuilderFor(doc pdf.DocAnalyzer) pdf.TableBuilder {
	if tableBuilderFactory != nil {
		return tableBuilderFactory(doc)
	}
	return tbl.NewDeepDocTableBuilder(doc)
}

// ── Public API ─────────────────────────────────────────────────────────────

// ParseRaw is the internal entry point: runs the core pipeline on an
// already-opened engine. Exported for tests that inject mock engines.
func (p *Parser) ParseRaw(ctx context.Context, engine pdf.PDFEngine, docAnalyzer pdf.DocAnalyzer) (*pdf.ParseResult, error) {
	return p.processPages(ctx, engine, docAnalyzer)
}

// ── ParseRaw helper functions ───────────────────────────────────────────────

func documentPages(pageCount int) []int {
	pages := make([]int, pageCount)
	for pg := range pages {
		pages[pg] = pg
	}
	return pages
}

// resolvePagesToProcess converts the 1-indexed inclusive Config.Pages ranges
// into a sorted, de-duplicated slice of 0-indexed page numbers clamped to
// [0, pageCount-1]. Empty/nil ranges fall back to all pages (the historical
// behavior), so callers that leave Pages unset are unaffected.
func resolvePagesToProcess(ranges [][]int, pageCount int) []int {
	if len(ranges) == 0 {
		return documentPages(pageCount)
	}
	seen := make(map[int]struct{}, pageCount)
	out := make([]int, 0, pageCount)
	for _, r := range ranges {
		if len(r) != 2 {
			continue
		}
		from0 := r[0] - 1
		to0 := r[1] - 1
		if from0 < 0 {
			from0 = 0
		}
		if from0 > pageCount-1 {
			continue
		}
		if to0 > pageCount-1 {
			to0 = pageCount - 1
		}
		if to0 < from0 {
			continue
		}
		for pg := from0; pg <= to0; pg++ {
			if _, dup := seen[pg]; dup {
				continue
			}
			seen[pg] = struct{}{}
			out = append(out, pg)
		}
	}
	sort.Ints(out)
	return out
}

// extractOutlines extracts the PDF outlines, returning nil on error.
func (p *Parser) extractOutlines(engine pdf.PDFEngine) []pdf.Outline {
	outlines, outlineErr := engine.Outlines()
	if outlineErr != nil {
		common.Warn("deepdoc pdf parse: extract outlines failed; continuing without them",
			zap.Error(outlineErr))
		outlines = nil
	}
	return outlines
}

// ── Page worker pool ─────────────────────────────────────────────────────────

// processPage executes a single page through the unified page-local path.
// Both clean and garbled/empty pages flow through this function; the OCR
// strategy is selected page-locally based on character quality. Worker
// artifacts are returned in pageResult for the global assembly phase.
//
// Zoom retry is per-page: if the default zoom produces no boxes and
// conditions allow (Zoom < 9, render succeeded), the page is
// re-rendered at Config.Zoom × DlaScale and OCR/DLA are re-run. This
// replaces the old document-wide retryZoom pass and ensures only pages
// that actually need a higher zoom pay the memory cost.
//
// Cancellation: workers honor ctx.Err() before issuing expensive work.
// Page-local OCR detection / recognition failures are recoverable; the
// page falls back to charsToBoxes. RenderPageToImage failure is also
// recoverable — the page may still emit chars-derived boxes.
func (p *Parser) processPage(ctx context.Context, engine pdf.PDFEngine, pg int,
	docAnalyzer pdf.DocAnalyzer, tb pdf.TableBuilder,
) pageResult {
	// Stamp the page number up front so replay DocAnalyzers can route their
	// per-page lookups in BOTH the OCR path (processPageBoxes -> OCRDetect/
	// OCRRecognize) and the DLA/TSR path (enrichOnePageWithDeepDoc also stamps
	// it later; this covers the earlier OCR calls). Production analyzers ignore
	// the key, so this is behavior-neutral.
	ctx = context.WithValue(ctx, pageNumCtxKey, pg)
	chars, extractErr := engine.ExtractChars(pg)
	if extractErr != nil {
		common.Warn("deepdoc pdf parse: processPage ExtractChars failed",
			zap.Int("page", pg), zap.Error(extractErr))
		chars = nil
	}
	medianH := util.MedianCharHeight(chars)
	medianW := util.MedianCharWidth(chars)
	isEnglish := util.DetectEnglishPage(chars, nil)
	isScanNoise := util.IsScanNoise(util.FullTextFromChars(map[int][]pdf.TextChar{pg: chars}))

	if err := ctx.Err(); err != nil {
		return pageResult{
			PageNumber:  pg,
			Chars:       chars,
			MedianH:     medianH,
			MedianW:     medianW,
			IsEnglish:   isEnglish,
			IsScanNoise: isScanNoise,
			Err:         err,
		}
	}

	// First pass: render at the default DLA DPI (216 DPI).
	common.Info("deepdoc pdf parse: stage", zap.Int("page", pg), zap.String("stage", "render start"))
	pageImg, renderErr := p.renderPageToImage(ctx, engine, pg)
	common.Info("deepdoc pdf parse: stage", zap.Int("page", pg), zap.String("stage", "render done"))
	pageZoom := pdf.DlaScale
	var ocrBoxes []pdf.TextBox
	var updatedChars []pdf.TextChar
	var ocrUsed bool
	var annotated []pdf.TextBox
	var pageTables []pdf.TableItem
	var dlaRegions []pdf.DLAPageRegions

	if pageImg != nil && renderErr == nil {
		common.Info("deepdoc pdf parse: stage", zap.Int("page", pg), zap.String("stage", "ocr start"))
		ocrBoxes, updatedChars, ocrUsed = p.processPageBoxes(ctx, pageImg, chars, pg, renderErr, isScanNoise, docAnalyzer, pageZoom)
		common.Info("deepdoc pdf parse: stage", zap.Int("page", pg), zap.String("stage", "ocr done"))
		annotated, pageTables, dlaRegions = p.enrichOnePageWithDeepDoc(
			ctx, pageImg, ocrBoxes, pg, renderErr, docAnalyzer, tb, pageZoom)
		common.Info("deepdoc pdf parse: stage", zap.Int("page", pg), zap.String("stage", "dla_tsr done"))
	}

	if renderErr != nil {
		common.Warn("deepdoc pdf parse: processPage RenderPageToImage failed",
			zap.Int("page", pg), zap.Error(renderErr))
	}

	// Per-page zoom retry: if no boxes were produced at the default zoom
	// and conditions allow, re-render at a higher zoom and re-run OCR/DLA.
	if len(annotated) == 0 && p.Config.Zoom >= 1.0 && renderErr == nil {
		// Cap the retry zoom so a large Config.Zoom cannot drive the retry
		// render to an unsafe DPI and spike memory on large pages.
		const maxRetryZoom = 9.0
		retryZoom := math.Min(p.Config.Zoom*pdf.DlaScale, maxRetryZoom)
		common.Debug("deepdoc pdf parse: per-page zoom retry",
			zap.Int("page", pg), zap.Float64("zoom", retryZoom))
		retryImg, retryRenderErr := p.renderAtDPI(ctx, engine, pg, retryZoom*72)
		if retryRenderErr == nil && retryImg != nil {
			ocrBoxes, updatedChars, ocrUsed = p.processPageBoxes(ctx, retryImg, chars, pg, retryRenderErr, isScanNoise, docAnalyzer, retryZoom)
			annotated, pageTables, dlaRegions = p.enrichOnePageWithDeepDoc(
				ctx, retryImg, ocrBoxes, pg, retryRenderErr, docAnalyzer, tb, retryZoom)
			pageImg = retryImg
			pageZoom = retryZoom
		} else if retryRenderErr != nil {
			common.Warn("deepdoc pdf parse: processPage retry-zoom render failed",
				zap.Int("page", pg), zap.Error(retryRenderErr))
		}
	}

	// Compute PDF-point page dimensions from the winning render so
	// buildLayout can use them without needing a per-page zoom map.
	var pageHeight, pageWidth float64
	if pageImg != nil && pageZoom > 0 {
		pageHeight = float64(pageImg.Bounds().Dy()) / pageZoom
		pageWidth = float64(pageImg.Bounds().Dx()) / pageZoom
	}

	return pageResult{
		PageNumber:  pg,
		Boxes:       annotated,
		Chars:       updatedChars,
		MedianH:     medianH,
		MedianW:     medianW,
		IsEnglish:   isEnglish,
		IsScanNoise: isScanNoise,
		OCRUsed:     ocrUsed,
		PageHeight:  pageHeight,
		PageWidth:   pageWidth,
		Tables:      pageTables,
		DLARegions:  dlaRegions,
		Err:         renderErr,
	}
}

// processPageBoxes picks the OCR strategy for a single page based on
// character quality. There is no async/sync split: the decision is
// page-local and the OCR call path is selected here.
//
//   - Clean embedded chars (count > 0 and not garbled):
//     use ocrMergeChars, which detect-merges chars into boxes and falls
//     back to single-image OCR recognize for empty boxes.
//   - Garbled or empty chars: use ocrDetectAndRecognize for full OCR,
//     then fall back to ocrMergeChars if detect succeeds but recognize
//     produced no useful output. Synthetic chars are appended to support
//     subsequent median calculations.
//
// updatedChars includes synthetic OCR chars when detect+recognize was
// used; callers must use the returned slice instead of the original.
// processPageBoxes builds the page's text boxes — line/word-level, NOT
// per-rune — either from embedded chars or via OCR, chosen by page quality.
//
// Parameters:
//   - pageImg: the page bitmap rendered at the DLA DPI (216 by default),
//     consumed by OCR detection/recognition.
//   - chars: per-glyph characters from ExtractChars (0-based page pg); the
//     finest-grained text unit. nil when the page has no embedded text.
//   - pg: page number (0-based), stamped onto every produced box.
//   - renderErr: non-nil skips OCR entirely and falls back to chars.
//   - isScanNoise: true marks a scanned/noisy page → prefer full OCR
//     detect+recognize over the embedded-char merge path.
//   - docAnalyzer: the DLA/OCR/Tensor backend (DeepDOC model) used for
//     detection and recognition.
//
// Returns:
//   - ocrBoxes: line/word-level []pdf.TextBox in PDF-point space; this is
//     the granularity fed into enrichOnePageWithDeepDoc as pageBoxes
//     (one box per line or per column-subline, never per rune).
//   - updatedChars: chars, possibly augmented with synthetic OCR-derived
//     glyphs so downstream median-height/width stats stay meaningful.
//   - bool: whether OCR was actually used to produce ocrBoxes.
func (p *Parser) processPageBoxes(ctx context.Context, pageImg image.Image, chars []pdf.TextChar, pg int,
	renderErr error, isScanNoise bool, docAnalyzer pdf.DocAnalyzer, zoom float64,
) ([]pdf.TextBox, []pdf.TextChar, bool) {
	var ocrBoxes []pdf.TextBox
	ocrUsed := false

	if renderErr == nil && pageImg != nil {
		hasCleanChars := len(chars) > 0 && !isScanNoise && !util.IsGarbledPage(chars)
		if hasCleanChars {
			ocrBoxes = p.ocrMergeChars(ctx, pageImg, chars, docAnalyzer, pg, zoom)
			ocrUsed = ocrBoxes != nil
			if ocrUsed {
				ocrBoxes = rescueUnmatchedChars(ocrBoxes, chars, pg)
			}
		} else {
			label := "scan page"
			if len(chars) > 0 && !isScanNoise {
				label = "garbled page"
			}
			ocrBoxes = p.ocrDetectAndRecognize(ctx, pageImg, docAnalyzer, pg, label, zoom)
			ocrUsed = ocrBoxes != nil
			if ocrUsed {
				// Synthetic OCR chars feed downstream median calculations.
				for j := range ocrBoxes {
					for _, r := range ocrBoxes[j].Text {
						chars = append(chars, pdf.TextChar{Text: string(r), PageNumber: pg})
						break
					}
				}
			} else if len(chars) > 0 {
				// Detect failed but chars exist: try the merge path.
				ocrBoxes = p.ocrMergeChars(ctx, pageImg, chars, docAnalyzer, pg, zoom)
				ocrUsed = ocrBoxes != nil
			}
		}
	}

	if !ocrUsed && len(chars) > 0 {
		if ocrBoxes == nil {
			ocrBoxes = lyt.CharsToBoxes(chars, pg, p.Config.SortByTop)
		}
	}

	// Mark OCR-derived boxes so OCR-only post-processing (layout.Dedup*)
	// can scope itself to them and never de-duplicate char-path content.
	if ocrUsed {
		for i := range ocrBoxes {
			ocrBoxes[i].IsOCR = true
		}
	}

	return ocrBoxes, chars, ocrUsed
}

// rescueUnmatchedChars recovers visible embedded characters that were missed
// by OCR detection (e.g. isolated table digits, minus signs, percentages) and
// packages them into text boxes so they are not lost from table and text extraction.
func rescueUnmatchedChars(boxes []pdf.TextBox, chars []pdf.TextChar, pg int) []pdf.TextBox {
	if len(chars) == 0 {
		return boxes
	}
	var unmatched []pdf.TextChar
	for _, c := range chars {
		if strings.TrimSpace(c.Text) == "" {
			continue
		}
		covered := false
		for _, b := range boxes {
			if charBoxOverlapRatio(c, b.X0, b.X1, b.Top, b.Bottom) > 0.3 {
				covered = true
				break
			}
		}
		if !covered {
			unmatched = append(unmatched, c)
		}
	}
	if len(unmatched) == 0 {
		return boxes
	}
	added := 0
	for _, rb := range rescueBoxes(unmatched, pg) {
		if majorityCovered(rb.chars, boxes) {
			continue // true duplicate: most constituent glyphs already sit inside a retained box
		}
		boxes = append(boxes, rb.box)
		added++
	}
	if added > 0 {
		common.Debug("rescueUnmatchedChars", zap.Int("page", pg), zap.Int("unmatchedChars", len(unmatched)), zap.Int("rescuedBoxes", added))
	}
	return boxes
}

// rescuedBox is a rescue TextBox together with the glyphs it groups, so the
// redundancy test can run on the actual constituents instead of the bbox.
type rescuedBox struct {
	box   pdf.TextBox
	chars []pdf.TextChar
}

// rescueBoxes packs unmatched chars into text boxes WITHOUT CharsToBoxes:
// that builder derives its split threshold from the rescued sample's own
// inter-char gaps, but rescued chars are by construction sparse (mostly
// single glyphs in different table cells) — a gap-derived threshold on few
// samples is meaningless, either merging several cells into one wide box or
// shredding one value into one box per digit. Instead: group per text line
// and cut only where a gap exceeds the line's median gap by 8pt — enough to
// cross a rendered cell whitespace, tight enough to keep a single value
// ("1", "5") of an interrupted OCR box together.
func rescueBoxes(chars []pdf.TextChar, pg int) []rescuedBox {
	var out []rescuedBox
	for _, line := range lyt.GroupCharsToLines(chars, false) {
		sorted := append([]pdf.TextChar(nil), line...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].X0 < sorted[j].X0 })
		var gaps []float64
		for i := 1; i < len(sorted); i++ {
			gaps = append(gaps, sorted[i].X0-sorted[i-1].X1)
		}
		thr := math.Inf(1)
		if len(gaps) > 0 {
			srt := append([]float64(nil), gaps...)
			sort.Float64s(srt)
			thr = srt[len(srt)/2] + 8
		}
		start := 0
		flush := func(end int) {
			sub := sorted[start:end]
			box := lyt.LineToTextBox(sub)
			box.PageNumber = pg
			out = append(out, rescuedBox{box: box, chars: sub})
		}
		for i := 1; i < len(sorted); i++ {
			if sorted[i].X0-sorted[i-1].X1 > thr {
				flush(i)
				start = i
			}
		}
		flush(len(sorted))
	}
	return out
}

// majorityCovered reports whether more than half of the glyphs already lie
// inside a single retained box (over 50% of each glyph's own area). The old
// bbox-level filter discarded a whole rescued group whenever its SPANNING
// bbox happened to sit over an existing box — losing the very chars this
// function exists to rescue (e.g. OCR kept the middle digit "4" of "345" and
// the rescued "3" and "5" merged into one group straddling it).
func majorityCovered(chars []pdf.TextChar, boxes []pdf.TextBox) bool {
	for i := range boxes {
		covered := 0
		for _, c := range chars {
			if charBoxOverlapRatio(c, boxes[i].X0, boxes[i].X1, boxes[i].Top, boxes[i].Bottom) > 0.5 {
				covered++
			}
		}
		if covered*2 > len(chars) {
			return true
		}
	}
	return false
}

// runPageWorkers executes pages through the single process-wide worker
// pool (parserPageWorkerPool). Page concurrency is bounded by that pool's
// worker count — there is no per-Parser parallelism knob; callers tune
// throughput via SetPageWorkerPoolSize. Each page produces one pageResult
// stored by page number so worker completion order does not affect
// collection order. Workers record the first page-level error for caller
// visibility, while context cancellation still stops new dispatch and lets
// in-flight work observe ctx.Err().
//
// OnPageDone is reported by the worker itself as its page finishes (see
// pageProgress), not by the collection loop below: the loop only starts after
// every page has been submitted, and SubmitTo blocks once the worker queue is
// full, so collector-side reporting would stay silent until submission
// finished, then deliver every completion in one burst.
//
// Pages are returned sorted by page number so callers can stream them
// directly into downstream assembly without re-sorting.
func (p *Parser) runPageWorkers(ctx context.Context, engine pdf.PDFEngine,
	pages []int,
	docAnalyzer pdf.DocAnalyzer, tb pdf.TableBuilder,
) ([]*pageResult, error) {
	if len(pages) == 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	resultMap := make(map[int]*pageResult, len(pages))
	var firstErr error
	recordErr := func(e error) {
		if e != nil && firstErr == nil {
			firstErr = e
		}
	}

	type pageTaskResult = utility.WorkerPoolResult[pageTask, pageResult]
	resultCh := make(chan pageTaskResult, len(pages))
	progress := &pageProgress{total: len(pages), onDone: p.Config.OnPageDone}

	submitted := 0
	for _, pg := range pages {
		task := pageTask{
			parser:      p,
			engine:      engine,
			pageNumber:  pg,
			docAnalyzer: docAnalyzer,
			tb:          tb,
			progress:    progress,
		}
		if err := parserPageWorkerPool().SubmitTo(ctx, task, resultCh); err != nil {
			recordErr(err)
			break
		}
		submitted++
	}

	for i := 0; i < submitted; i++ {
		taskResult := <-resultCh
		r := taskResult.Value
		r.PageNumber = taskResult.Input.pageNumber
		if taskResult.Err != nil && r.Err == nil {
			r.Err = taskResult.Err
		}
		if r.Err != nil {
			recordErr(r.Err)
		}
		resultMap[r.PageNumber] = &r
		common.Info("deepdoc pdf parse: page finished",
			zap.Int("page", r.PageNumber),
			zap.Int("done", i+1),
			zap.Int("total", submitted))
	}

	results := make([]*pageResult, 0, len(pages))
	for _, pg := range pages {
		if r, ok := resultMap[pg]; ok {
			results = append(results, r)
		}
	}
	return results, firstErr
}

// ── Internal pipeline steps ────────────────────────────────────────────────

// runAssembly merges per-page worker artifacts into the final document-wide
// state. Document-wide layout, table merge/replace, cross-page figures, and
// metrics aggregation happen here so page workers never mutate shared state.
// pageResults are expected to be sorted by page number.
func (p *Parser) assembleDocument(ctx context.Context, pages []int, pageResults []*pageResult, outlines []pdf.Outline) (*pdf.ParseResult, error) {
	result := &pdf.ParseResult{
		PageHeight: make(map[int]float64),
		PageWidth:  make(map[int]float64),
		Outlines:   outlines,
	}

	var boxes []pdf.TextBox
	pageChars := make(map[int][]pdf.TextChar)
	medianHeights := make(map[int]float64, len(pageResults))
	medianWidths := make(map[int]float64, len(pageResults))
	pageEnglish := make(map[int]bool, len(pageResults))

	for _, r := range pageResults {
		if r == nil {
			continue
		}
		if r.Err != nil && !errors.Is(r.Err, context.Canceled) {
			common.Warn("deepdoc pdf parse: page worker failed",
				zap.Int("page", r.PageNumber), zap.Error(r.Err))
		}
		// Store per-page PDF-point dimensions for buildLayout.
		if r.PageHeight > 0 {
			result.PageHeight[r.PageNumber] = r.PageHeight
		}
		if r.PageWidth > 0 {
			result.PageWidth[r.PageNumber] = r.PageWidth
		}
		// Worker Boxes already carry DLA/TSR annotations (worker-local
		// write-back happened inside processPage). Concatenate in page
		// order to rebuild the document-wide boxes slice.
		boxes = append(boxes, r.Boxes...)
		pageChars[r.PageNumber] = r.Chars
		medianHeights[r.PageNumber] = r.MedianH
		medianWidths[r.PageNumber] = r.MedianW
		pageEnglish[r.PageNumber] = r.IsEnglish
		if r.OCRUsed {
			medianHeights[r.PageNumber] = util.MedianCharHeight(r.Chars)
			medianWidths[r.PageNumber] = util.MedianCharWidth(r.Chars)
			pageEnglish[r.PageNumber] = util.DetectEnglishPage(r.Chars, nil)
		}
		// Tables and DLARegions are accumulated across pages in order.
		if len(r.Tables) > 0 {
			result.Tables = append(result.Tables, r.Tables...)
		}
		if len(r.DLARegions) > 0 {
			result.DLARegions = append(result.DLARegions, r.DLARegions...)
		}
	}

	if len(boxes) == 0 {
		return result, nil
	}

	// A TOC is a document prefix, so the box-shape signal is only meaningful
	// when this parse covers the document's first page.
	coversDocumentStart := len(pages) > 0 && pages[0] == 0
	if err := p.buildLayout(ctx, result, boxes, pageChars,
		medianHeights, medianWidths, pageEnglish, coversDocumentStart); err != nil {
		return nil, fmt.Errorf("buildLayout: %w", err)
	}
	return result, nil
}

// buildLayout consumes the assembled boxes + page-level artifacts and runs
// the global layout/table/figure pipeline. It is the only step that runs
// AssignColumn, TextMerge, FinalReadingOrderMerge, NaiveVerticalMerge, table
// merge, figure consolidation, BoxesToSections, and caption merge.
//
// coversDocumentStart reports whether `pages` started at the document's first
// page, which is what the TOC box-shape signal requires (see RemoveTOCBoxes).
func (p *Parser) buildLayout(ctx context.Context,
	result *pdf.ParseResult,
	boxes []pdf.TextBox, pageChars map[int][]pdf.TextChar,
	medianHeights, medianWidths map[int]float64,
	pageEnglish map[int]bool,
	coversDocumentStart bool,
) error {
	result.Metrics.BoxesInitial = len(boxes)

	// Assign columns BEFORE dedup so DedupSubstringOverlaps can tell a real
	// OCR double-detection fragment (same column as its container) apart from
	// an independent short line in a DIFFERENT column whose text merely
	// happens to be a substring of a wide cross-gutter OCR box. Without the
	// column tag, double-column pages (e.g. 1例3个月) lose left-column lines
	// to the substring collapse. AssignColumn only reads box geometry, so it
	// is safe before any text merge.
	boxes = lyt.AssignColumn(boxes)

	// Collapse OCR duplicates BEFORE any merge step: overlapping same-text /
	// substring boxes must be dropped while still independent, otherwise
	// TextMerge/NaiveVerticalMerge concatenate them into duplicated text. The
	// same-column guard above keeps cross-column lines intact.
	boxes = lyt.DedupIdenticalText(boxes)
	boxes = lyt.DedupSubstringOverlaps(boxes)

	// Drop single-character ASCII boxes that repeat verbatim many times on
	// the SAME page — these are the rotated watermark glyphs from
	// templated PDFs (issue #18145). Post-process placement so the same
	// signal covers both the char-path (passed through CharsToBoxes) and
	// the OCR-merge path (passed through ocrMergeChars); a layout-stage
	// filter would have only caught the former.
	boxes = lyt.FilterWatermarkBoxes(boxes)

	// Page-level content removal on intact box geometry. These MUST run before
	// TextMerge: RemoveTOCBoxes relies on leader-dot boxes (which TextMerge
	// folds into adjacent text) and RemoveHeaderFooterBoxes relies on header
	// boxes that TextMerge would otherwise merge into the first body section.
	//
	// Header/footer removal runs FIRST because its evidence is cross-page: it
	// counts how many of the document's pages carry the same margin text, while
	// TOC removal deletes whole pages. Running the TOC pass first would take
	// that margin text off the pages it deletes, dropping the count below the
	// half-the-pages bar, so a document with both options on would keep a
	// running header that either option removes on its own.
	//
	// The TOC pass is nearly indifferent to the order. It decides on page shapes
	// and absolute bookmark numbers, and the only boxes header/footer removal
	// takes from a TOC page are margin boxes, which never carried an entry
	// marker. They do count towards tocMinShortBoxes, so a TOC page sitting
	// exactly on that threshold can fall below it and be kept — a missed TOC
	// page, which is the direction this detector errs in regardless.
	//
	// coversDocumentStart gates the TOC box-shape signal: a TOC is a document
	// prefix, so a parse restricted to a later page range must not read its own
	// first page as one. The outline signal uses absolute page numbers and needs
	// no such gate.
	boxesBefore := len(boxes)
	if p.Config.RemoveHeaderFooter {
		boxes = lyt.RemoveHeaderFooterBoxes(boxes, result.PageHeight)
		result.Metrics.BoxesHeaderFooterRemoved = boxesBefore - len(boxes)
		boxesBefore = len(boxes)
	}
	if p.Config.RemoveTOC {
		boxes = lyt.RemoveTOCBoxes(boxes, lyt.TOCPageRangeFromOutlines(result.Outlines), coversDocumentStart)
		result.Metrics.BoxesTOCRemoved = boxesBefore - len(boxes)
	}

	boxes = lyt.TextMerge(boxes, medianHeights)
	result.Metrics.BoxesTextMerge = len(boxes)

	// Preserve column-major content order while NaiveVerticalMerge promotes a
	// page-leading title isolated in its own column.
	boxes = lyt.FinalReadingOrderMerge(boxes)

	boxes = lyt.NaiveVerticalMerge(boxes, medianHeights, medianWidths, pageEnglish)
	result.Metrics.BoxesVertMerge = len(boxes)
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(result.Tables) > 0 {
		result.Tables = tbl.MergeTablesAcrossPages(result.Tables, medianHeights, result.PageHeight)
	}

	boxes = tbl.ExtractTableAndReplace(boxes, result.Tables)
	result.Metrics.TablesCount = len(result.Tables)
	boxes = tbl.ConsolidateFigures(boxes)

	// PageHeight is already in PDF-point space — computed in processPage.
	result.Sections = lyt.BoxesToSections(boxes, result.PageHeight)
	result.Metrics.BoxesFinal = len(result.Sections)
	result.Sections = tbl.MergeCaptions(result.Sections, result.Figures())
	return nil
}

// processPages drives the page worker pool and the global assembly step.
// Page-local work (including per-page zoom retry) happens inside processPage;
// document-wide work happens in assembleDocument.
func (p *Parser) processPages(ctx context.Context, engine pdf.PDFEngine, docAnalyzer pdf.DocAnalyzer) (*pdf.ParseResult, error) {
	pageCount, err := engine.PageCount()
	if err != nil {
		return nil, fmt.Errorf("page count: %w", err)
	}
	// The outlines are read before the layout is built: buildLayout uses them to
	// select TOC pages on the intact box geometry (see Parser.buildLayout), so
	// they cannot be attached to the result afterwards.
	outlines := p.extractOutlines(engine)
	if pageCount == 0 {
		return &pdf.ParseResult{
			PageHeight: make(map[int]float64),
			PageWidth:  make(map[int]float64),
			Outlines:   outlines,
		}, nil
	}

	tb := NewTableBuilderFor(docAnalyzer)
	pages := resolvePagesToProcess(p.Config.Pages, pageCount)
	common.Info("deepdoc pdf parse: total pages",
		zap.Int("page_count", pageCount),
		zap.Int("pages_to_parse", len(pages)))
	if len(p.Config.Pages) > 0 {
		common.Info("deepdoc pdf parse: page ranges applied",
			zap.Any("configured_ranges", p.Config.Pages),
			zap.Int("page_count", pageCount),
			zap.Ints("pages_to_parse", pages))
	} else {
		common.Info("deepdoc pdf parse: parsing all pages", zap.Int("page_count", pageCount))
	}

	pageResults, pageErr := p.runPageWorkers(ctx, engine, pages, docAnalyzer, tb)
	if pageErr != nil && !errors.Is(pageErr, context.Canceled) {
		common.Warn("deepdoc pdf parse: runPageWorkers some pages failed",
			zap.Error(pageErr))
	}

	result, err := p.assembleDocument(ctx, pages, pageResults, outlines)
	if err != nil {
		return nil, err
	}
	// Preserve a hard failure from the page workers (e.g. context
	// cancellation) — assembleDocument may still succeed on an empty
	// result, which would otherwise swallow the error.
	if pageErr != nil {
		return result, pageErr
	}
	// Carry the engine on the result so the JSON/markdown serialization
	// step can crop section images on demand, then release it. This also
	// fixes the previous engine leak (the engine was discarded here and
	// re-created unnecessarily downstream).
	result.Engine = engine
	return result, nil
}
