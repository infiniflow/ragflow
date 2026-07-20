package pdf

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"math"
	"sync"

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
	// deepInfOnce lazily initializes deepInf exactly once, so a Parser shared
	// across goroutines does not race on the DeepDoc inference slot channel.
	deepInfOnce sync.Once
	// deepInf bounds concurrent DeepDoc (deepdoc) inference calls. Created
	// on first use via limiters(); see parser_concurrency.go.
	deepInf *deepInfLimiter
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
	outlines := p.extractOutlines(engine)

	result, err := p.processPages(ctx, engine, docAnalyzer)
	if err != nil {
		return nil, err
	}
	result.Outlines = outlines
	return result, nil
}

// ── ParseRaw helper functions ───────────────────────────────────────────────

func documentPages(pageCount int) []int {
	pages := make([]int, pageCount)
	for pg := range pages {
		pages[pg] = pg
	}
	return pages
}

// extractOutlines extracts the PDF outlines, returning nil on error.
func (p *Parser) extractOutlines(engine pdf.PDFEngine) []pdf.Outline {
	outlines, outlineErr := engine.Outlines()
	if outlineErr != nil {
		slog.Warn("Failed to extract PDF outlines; continuing without them", "err", outlineErr)
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
// conditions allow (Zoom < 9, !SkipOCR, render succeeded), the page is
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
	chars, extractErr := engine.ExtractChars(pg)
	if extractErr != nil {
		slog.Warn("processPage: ExtractChars failed", "page", pg, "err", extractErr)
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
	pageImg, renderErr := p.renderPageToImage(ctx, engine, pg)
	pageZoom := pdf.DlaScale
	var ocrBoxes []pdf.TextBox
	var updatedChars []pdf.TextChar
	var ocrUsed bool
	var annotated []pdf.TextBox
	var pageTables []pdf.TableItem
	var dlaRegions []pdf.DLAPageRegions

	if pageImg != nil && renderErr == nil {
		ocrBoxes, updatedChars, ocrUsed = p.processPageBoxes(ctx, pageImg, chars, pg, renderErr, isScanNoise, docAnalyzer)
		annotated, pageTables, dlaRegions = p.enrichOnePageWithDeepDoc(
			ctx, pageImg, ocrBoxes, pg, renderErr, docAnalyzer, tb, pageZoom)
	}

	if renderErr != nil {
		slog.Warn("processPage: RenderPageToImage failed", "page", pg, "err", renderErr)
	}

	// Per-page zoom retry: if no boxes were produced at the default zoom
	// and conditions allow, re-render at a higher zoom and re-run OCR/DLA.
	if len(annotated) == 0 && p.Config.Zoom >= 1.0 && !p.Config.SkipOCR && renderErr == nil {
		// Cap the retry zoom so a large Config.Zoom cannot drive the retry
		// render to an unsafe DPI and spike memory on large pages.
		const maxRetryZoom = 9.0
		retryZoom := math.Min(p.Config.Zoom*pdf.DlaScale, maxRetryZoom)
		slog.Debug("per-page zoom retry", "page", pg, "zoom", retryZoom)
		retryImg, retryRenderErr := p.renderAtDPI(ctx, engine, pg, retryZoom*72)
		if retryRenderErr == nil && retryImg != nil {
			ocrBoxes, updatedChars, ocrUsed = p.processPageBoxes(ctx, retryImg, chars, pg, retryRenderErr, isScanNoise, docAnalyzer)
			annotated, pageTables, dlaRegions = p.enrichOnePageWithDeepDoc(
				ctx, retryImg, ocrBoxes, pg, retryRenderErr, docAnalyzer, tb, retryZoom)
			pageImg = retryImg
			pageZoom = retryZoom
		} else if retryRenderErr != nil {
			slog.Warn("processPage: retry-zoom render failed", "page", pg, "err", retryRenderErr)
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
	renderErr error, isScanNoise bool, docAnalyzer pdf.DocAnalyzer,
) ([]pdf.TextBox, []pdf.TextChar, bool) {
	var ocrBoxes []pdf.TextBox
	ocrUsed := false

	if !p.Config.SkipOCR && renderErr == nil && pageImg != nil {
		hasCleanChars := len(chars) > 0 && !isScanNoise && !util.IsGarbledPage(chars)
		if hasCleanChars {
			ocrBoxes = p.ocrMergeChars(ctx, pageImg, chars, docAnalyzer, pg)
			ocrUsed = ocrBoxes != nil
		} else {
			label := "scan page"
			if len(chars) > 0 && !isScanNoise {
				label = "garbled page"
			}
			ocrBoxes = p.ocrDetectAndRecognize(ctx, pageImg, docAnalyzer, pg, label)
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
				ocrBoxes = p.ocrMergeChars(ctx, pageImg, chars, docAnalyzer, pg)
				ocrUsed = ocrBoxes != nil
			}
		}
	}

	if !ocrUsed && len(chars) > 0 {
		if ocrBoxes == nil {
			ocrBoxes = lyt.CharsToBoxes(chars, pg, p.Config.SortByTop)
		}
	}

	return ocrBoxes, chars, ocrUsed
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

	submitted := 0
	for _, pg := range pages {
		task := pageTask{
			parser:      p,
			engine:      engine,
			pageNumber:  pg,
			docAnalyzer: docAnalyzer,
			tb:          tb,
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
func (p *Parser) assembleDocument(ctx context.Context, pages []int, pageResults []*pageResult) (*pdf.ParseResult, error) {
	result := &pdf.ParseResult{
		PageHeight: make(map[int]float64),
		PageWidth:  make(map[int]float64),
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
		if r.Err != nil {
			slog.Warn("page worker failed", "page", r.PageNumber, "err", r.Err)
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

	if err := p.buildLayout(ctx, result, boxes, pageChars,
		medianHeights, medianWidths, pageEnglish); err != nil {
		return nil, fmt.Errorf("buildLayout: %w", err)
	}
	return result, nil
}

// buildLayout consumes the assembled boxes + page-level artifacts and runs
// the global layout/table/figure pipeline. It is the only step that runs
// AssignColumn, TextMerge, FinalReadingOrderMerge, NaiveVerticalMerge, table
// merge, figure consolidation, BoxesToSections, and caption merge.
func (p *Parser) buildLayout(ctx context.Context,
	result *pdf.ParseResult,
	boxes []pdf.TextBox, pageChars map[int][]pdf.TextChar,
	medianHeights, medianWidths map[int]float64,
	pageEnglish map[int]bool,
) error {
	result.Metrics.BoxesInitial = len(boxes)

	boxes = lyt.AssignColumn(boxes)
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
		result.Tables = tbl.MergeTablesAcrossPages(result.Tables, nil)
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
	if pageCount == 0 {
		return &pdf.ParseResult{
			PageHeight: make(map[int]float64),
			PageWidth:  make(map[int]float64),
		}, nil
	}

	tb := NewTableBuilderFor(docAnalyzer)
	pages := documentPages(pageCount)

	pageResults, pageErr := p.runPageWorkers(ctx, engine, pages, docAnalyzer, tb)
	if pageErr != nil {
		slog.Warn("runPageWorkers: some pages failed", "err", pageErr)
	}

	result, err := p.assembleDocument(ctx, pages, pageResults)
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
