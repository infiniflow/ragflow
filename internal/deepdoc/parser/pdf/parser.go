package parser

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"sync"

	inf "ragflow/internal/deepdoc/parser/pdf/inference"
	lyt "ragflow/internal/deepdoc/parser/pdf/layout"
	tbl "ragflow/internal/deepdoc/parser/pdf/table"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// Parser is the main PDF text/layout extraction pipeline.
// It corresponds to RAGFlowPdfParser in pdf_parser.py.
// Parser is stateless after construction — safe to reuse across documents.
type Parser struct {
	Config pdf.ParserConfig

	// DeepDoc is the required document layout / OCR / table recognition
	// service. Set at construction time by NewParser.
	DeepDoc pdf.DocAnalyzer

	// SampleChars samples up to n chars from a page for English detection.
	// Defaults to random sampling (matching Python's random.choices).
	// Inject a deterministic sampler for reproducible tests.
	SampleChars pdf.SampleFunc

	// tableBuilder is the TSR model adapter. Set at construction time
	//
	// different implementation via Config.TableBuilder.
	tableBuilder pdf.TableBuilder
}

// NewParser creates a new Parser with the required DeepDoc service.
func NewParser(cfg pdf.ParserConfig, doc pdf.DocAnalyzer) *Parser {
	tb := cfg.TableBuilder
	if tb == nil {
		tb = NewTableBuilderFor(doc)
	}
	return &Parser{
		Config:       cfg,
		DeepDoc:      doc,
		tableBuilder: tb,
	}
}

// ── TableBuilder factory ───────────────────────────────────────────────────

// tableBuilderFactory holds a model-specific TableBuilder factory registered
// by EE packages via RegisterTableBuilder. If nil, the default OSS
// implementation is used.
var tableBuilderFactory func(pdf.DocAnalyzer) pdf.TableBuilder

// RegisterTableBuilder registers a TableBuilder factory for the PDF parser.
// EE packages call this from init() to inject EE-specific implementations.
func RegisterTableBuilder(factory func(pdf.DocAnalyzer) pdf.TableBuilder) {
	tableBuilderFactory = factory
}

// NewTableBuilderFor creates the right TableBuilder, chosen by the registry.
// Checks the registry first for EE-registered implementations, falling back
// to the default OSS DeepDocTableBuilder. Label taxonomies are injected
// before construction.
func NewTableBuilderFor(doc pdf.DocAnalyzer) pdf.TableBuilder {
	if tableBuilderFactory != nil {
		return tableBuilderFactory(doc)
	}
	if c, ok := doc.(*inf.InferenceClient); ok {
		c.DLALabels = inf.DefaultDLALabels()
		c.TSRLabels = inf.DefaultTSRLabels()
	}
	return tbl.NewDeepDocTableBuilder(doc)
}

// Parse runs the full PDF extraction pipeline: chars → boxes →
// column assignment → text merge → vertical merge → sections.
//
// For documents larger than Config.BatchSize pages, processes in batches
// to bound memory usage (matching Python's batch_size=50).
//
// Returns a pdf.ParseResult containing sections, tables, page images, figures,
// and pipeline stage metrics. Parser itself remains stateless.
func (p *Parser) Parse(ctx context.Context, engine pdf.PDFEngine) (*pdf.ParseResult, error) {
	// Normalize page range
	pageCount, err := engine.PageCount()
	if err != nil {
		return nil, fmt.Errorf("page count: %w", err)
	}
	toPage := p.Config.ToPage
	if toPage < 0 || toPage >= pageCount {
		toPage = pageCount - 1
	}
	fromPage := p.Config.FromPage
	if toPage < fromPage {
		return &pdf.ParseResult{PageImages: make(map[int]image.Image)}, nil
	}

	totalPages := toPage - fromPage + 1
	batchSize := p.Config.BatchSize
	if batchSize <= 0 {
		batchSize = 50 // default, matching Python's batch_size
	}

	// ── Prescan: lightweight char extraction for language/noise detection ──
	// No rendering, no OCR — just raw chars for global decisions.
	prescanChars := make(map[int][]pdf.TextChar)
	prescanMedianH := make(map[int]float64)
	prescanMedianW := make(map[int]float64)
	for pg := fromPage; pg <= toPage; pg++ {
		chars, extractErr := engine.ExtractChars(pg)
		if extractErr != nil {
			slog.Warn("prescan: ExtractChars failed", "page", pg, "err", extractErr)
			chars = nil // skip broken pages (matching old behavior)
		}
		prescanChars[pg] = chars
		prescanMedianH[pg] = util.MedianCharHeight(chars)
		prescanMedianW[pg] = util.MedianCharWidth(chars)
	}
	isEnglish := util.DetectEnglish(prescanChars, totalPages, p.SampleChars)
	scanNoise := util.IsScanNoise(util.FullTextFromChars(prescanChars))

	// ── Extract PDF outlines/bookmarks (best-effort, non-fatal) ──
	outlines, outlineErr := engine.Outlines()
	if outlineErr != nil {
		slog.Warn("Failed to extract PDF outlines; continuing without them", "err", outlineErr)
		outlines = nil
	}

	// ── Small document: process all at once (no batching overhead) ──
	if totalPages <= batchSize {
		result, err := p.processPages(ctx, engine, fromPage, toPage,
			prescanChars, prescanMedianH, prescanMedianW, isEnglish, scanNoise)
		if err != nil {
			return nil, err
		}
		result.Outlines = outlines
		return result, nil
	}

	// ── Large document: process in batches to bound memory ──
	slog.Info("batched processing", "pages", totalPages, "batchSize", batchSize)
	result := &pdf.ParseResult{PageImages: make(map[int]image.Image)}
	for start := fromPage; start <= toPage; start += batchSize {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("cancelled at batch starting page %d: %w", start, err)
		}
		end := min(start+batchSize-1, toPage)

		// Slice prescan data for this batch.
		batchChars := make(map[int][]pdf.TextChar, end-start+1)
		batchMH := make(map[int]float64, end-start+1)
		batchMW := make(map[int]float64, end-start+1)
		for pg := start; pg <= end; pg++ {
			batchChars[pg] = prescanChars[pg]
			batchMH[pg] = prescanMedianH[pg]
			batchMW[pg] = prescanMedianW[pg]
		}

		batch, err := p.processPages(ctx, engine, start, end,
			batchChars, batchMH, batchMW, isEnglish, scanNoise)
		if err != nil {
			return nil, err
		}

		// Merge batch results.
		result.Sections = append(result.Sections, batch.Sections...)
		result.Tables = append(result.Tables, batch.Tables...)
		// Figures() is computed on demand from Sections.
		for pg, img := range batch.PageImages {
			result.PageImages[pg] = img
		}
		result.Metrics.BoxesInitial += batch.Metrics.BoxesInitial
		result.Metrics.BoxesTextMerge += batch.Metrics.BoxesTextMerge
		result.Metrics.BoxesVertMerge += batch.Metrics.BoxesVertMerge
		result.Metrics.BoxesFinal += batch.Metrics.BoxesFinal
		result.Metrics.TablesCount += batch.Metrics.TablesCount
	}
	result.Outlines = outlines
	return result, nil
}

// extractPages runs per-page OCR (detect + recognize) for the given page
// range, returning text boxes, char data, whether any page used OCR, and
// any errors encountered.  Partial results are returned even when some
// pages fail — callers should inspect the error for diagnostics but may
// still use the returned boxes and chars.
func (p *Parser) extractPages(ctx context.Context, engine pdf.PDFEngine,
	fromPage, toPage int,
	prescanChars map[int][]pdf.TextChar,
	medianHeights, medianWidths map[int]float64,
	pageImages map[int]image.Image,
) ([]pdf.TextBox, map[int][]pdf.TextChar, bool, error) {
	var boxes []pdf.TextBox
	pageChars := make(map[int][]pdf.TextChar)
	ocrUsedAny := false

	type pr struct {
		pg       int
		ocrBoxes []pdf.TextBox
		chars    []pdf.TextChar
		ocrUsed  bool
		pageImg  image.Image
		err      error
	}
	pageCount := toPage - fromPage + 1
	results := make([]pr, pageCount)

	// Semaphore cap: 0 → sequential; >0 → bounded parallelism.
	cap := p.Config.MaxOCRConcurrency
	if cap <= 0 {
		cap = 1
	}
	sem := make(chan struct{}, cap)
	var wg sync.WaitGroup

	for i := 0; i < pageCount; i++ {
		pg := fromPage + i
		chars := prescanChars[pg]

		// Fast path: pages with embedded chars → sequential inline (no HTTP OCR).
		if len(chars) > 0 && !util.IsGarbledPage(chars) {
			pageImg, renderErr := renderPageToImage(engine, pg)
			if renderErr == nil && pageImg != nil {
				pageImages[pg] = pageImg
			}
			var ocrBoxes []pdf.TextBox
			ocrUsed := false
			if !p.Config.SkipOCR && renderErr == nil && pageImg != nil {
				ocrBoxes = ocrMergeChars(ctx, pageImg, chars, p.DeepDoc, pg)
				if ocrBoxes == nil {
					ocrBoxes = lyt.CharsToBoxes(chars, pg, p.Config.SortByTop)
				} else {
					ocrUsed = true
					ocrUsedAny = true
				}
			} else {
				ocrBoxes = lyt.CharsToBoxes(chars, pg, p.Config.SortByTop)
			}
			results[i] = pr{pg: pg, ocrBoxes: ocrBoxes, chars: chars, ocrUsed: ocrUsed}
			continue
		}

		// OCR path: render + detect + recognize (potentially parallel).
		wg.Add(1)
		go func(i, pg int, chars []pdf.TextChar) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				results[i] = pr{pg: pg, err: ctx.Err()}
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			pageImg, err := renderPageToImage(engine, pg)
			if err != nil {
				results[i] = pr{pg: pg, err: err}
				return
			}
			// Check if context was cancelled during render.
			if err := ctx.Err(); err != nil {
				results[i] = pr{pg: pg, err: err}
				return
			}

			var ocrBoxes []pdf.TextBox
			ocrUsed := false
			if !p.Config.SkipOCR {
				label := "scan page"
				if len(chars) > 0 {
					label = "garbled page"
				}
				ocrBoxes = ocrDetectAndRecognize(ctx, pageImg, p.DeepDoc, pg, label)
				if ocrBoxes != nil {
					for j := range ocrBoxes {
						for _, r := range ocrBoxes[j].Text {
							chars = append(chars, pdf.TextChar{Text: string(r), PageNumber: pg})
							break
						}
					}
					ocrUsed = true
				}
			}
			// Merged OCR path for pages with both embedded and OCR chars.
			if !ocrUsed && len(chars) > 0 && !p.Config.SkipOCR {
				ocrBoxes = ocrMergeChars(ctx, pageImg, chars, p.DeepDoc, pg)
				if ocrBoxes != nil {
					ocrUsed = true
				}
			}
			if !ocrUsed {
				if len(chars) > 0 {
					ocrBoxes = lyt.CharsToBoxes(chars, pg, p.Config.SortByTop)
				}
			}
			results[i] = pr{pg: pg, ocrBoxes: ocrBoxes, chars: chars, ocrUsed: ocrUsed, pageImg: pageImg}
		}(i, pg, chars)
	}
	wg.Wait()

	// Merge results in page order.
	var errs []error
	for i := 0; i < pageCount; i++ {
		r := results[i]
		if r.err != nil {
			slog.Warn("page OCR failed", "page", r.pg, "err", r.err)
			errs = append(errs, fmt.Errorf("page %d: %w", r.pg, r.err))
			continue
		}
		if r.ocrUsed {
			boxes = append(boxes, r.ocrBoxes...)
			ocrUsedAny = true
		} else if len(r.ocrBoxes) > 0 {
			boxes = append(boxes, r.ocrBoxes...)
		}
		if r.pageImg != nil {
			pageImages[r.pg] = r.pageImg
		}
		pageChars[r.pg] = r.chars
		if r.ocrUsed {
			medianHeights[r.pg] = util.MedianCharHeight(r.chars)
			medianWidths[r.pg] = util.MedianCharWidth(r.chars)
		}
	}
	return boxes, pageChars, ocrUsedAny, errors.Join(errs...)
}

// retryScanNoise re-runs OCR on all pages when prescan detects scan noise,
// overwriting page-level state with fresh detect+recognize results.
func (p *Parser) retryScanNoise(ctx context.Context, engine pdf.PDFEngine,
	fromPage, toPage int,
	pageImages map[int]image.Image,
	pageChars map[int][]pdf.TextChar,
	medianHeights, medianWidths map[int]float64,
	ocrUsedAny bool,
) ([]pdf.TextBox, map[int][]pdf.TextChar, bool) {
	slog.Warn("scan noise: OCR retry", "from", fromPage, "to", toPage)
	var boxes []pdf.TextBox
	for pg := fromPage; pg <= toPage; pg++ {
		img := pageImages[pg]
		if img == nil {
			var err error
			img, err = renderPageToImage(engine, pg)
			if err != nil {
				slog.Warn("scan noise: page render failed", "page", pg, "err", err)
				continue
			}
			pageImages[pg] = img
		}
		ocrBoxes := ocrDetectAndRecognize(ctx, img, p.DeepDoc, pg, "scan page")
		if ocrBoxes == nil {
			slog.Warn("scan noise: page OCR empty", "page", pg)
			continue
		}
		boxes = append(boxes, ocrBoxes...)
		var chars []pdf.TextChar
		for _, b := range ocrBoxes {
			for _, r := range b.Text {
				chars = append(chars, pdf.TextChar{Text: string(r), Top: b.Top, Bottom: b.Bottom, PageNumber: pg})
				break
			}
		}
		pageChars[pg] = chars
		medianHeights[pg] = util.MedianCharHeight(chars)
		medianWidths[pg] = util.MedianCharWidth(chars)
	}
	slog.Debug("scan noise OCR retry complete", "pages", toPage-fromPage+1, "boxes", len(boxes))
	return boxes, pageChars, true
}

// retryZoom re-renders pages at higher resolution and re-runs OCR when the
// initial extraction produced zero boxes.  Box coordinates are scaled back
// to Config.Zoom space.  Matches Python's __images__ retry.
func (p *Parser) retryZoom(ctx context.Context, engine pdf.PDFEngine,
	fromPage, toPage int,
	pageImages map[int]image.Image,
	boxes []pdf.TextBox, ocrUsedAny bool,
) ([]pdf.TextBox, bool) {
	retryZoom := p.Config.Zoom * pdf.DlaScale
	retryDPI := retryZoom * 72
	slog.Info("zoom retry: re-rendering", "oldZoom", p.Config.Zoom, "newZoom", retryZoom)
	for pg := fromPage; pg <= toPage; pg++ {
		img, err := engine.RenderPageImage(pg, retryDPI)
		if err != nil {
			slog.Warn("zoom retry: render failed", "page", pg, "err", err)
			continue
		}
		pageImages[pg] = img
		// Downstream DLA/TSR assumes pdf.DlaDPI. Re-render at standard
		// resolution so layout coordinates are scaled correctly.
		if retryDPI != pdf.DlaDPI {
			if dlaImg, dlaErr := engine.RenderPageImage(pg, pdf.DlaDPI); dlaErr == nil {
				pageImages[pg] = dlaImg
			}
		}
		ocrBoxes := ocrDetectAndRecognize(ctx, img, p.DeepDoc, pg, "zoom retry")
		if ocrBoxes == nil {
			continue
		}
		scaleFactor := retryZoom / p.Config.Zoom
		for i := range ocrBoxes {
			ocrBoxes[i].X0 /= scaleFactor
			ocrBoxes[i].X1 /= scaleFactor
			ocrBoxes[i].Top /= scaleFactor
			ocrBoxes[i].Bottom /= scaleFactor
		}
		boxes = append(boxes, ocrBoxes...)
		ocrUsedAny = true
	}
	return boxes, ocrUsedAny
}

// buildLayout runs the DLA → TSR → Column → TextMerge → VM → pdf.Section
// pipeline and populates result.Metrics, result.Tables, result.Sections,
// and result.Sections.  Matches Python's _parse_loaded_window_into_bboxes
// order.
func (p *Parser) buildLayout(ctx context.Context,
	result *pdf.ParseResult, engine pdf.PDFEngine,
	boxes []pdf.TextBox, pageChars map[int][]pdf.TextChar,
	medianHeights, medianWidths map[int]float64,
	fromPage, toPage int, ocrUsedAny bool, isEnglish bool,
) error {
	result.Metrics.BoxesInitial = len(boxes)

	result.Tables = p.enrichWithDeepDoc(ctx, result, engine, boxes, result.PageImages)
	result.Metrics.TablesCount = len(result.Tables)
	if err := ctx.Err(); err != nil {
		return err
	}

	boxes = lyt.AssignColumn(boxes, p.Config.Zoom)
	boxes = lyt.TextMerge(boxes, medianHeights, p.Config.Zoom)
	result.Metrics.BoxesTextMerge = len(boxes)

	lyt.SortByPageThenY(boxes, p.Config.SortByTop)

	if ocrUsedAny {
		isEnglish = util.DetectEnglish(pageChars, toPage-fromPage+1, p.SampleChars)
	}
	boxes = lyt.NaiveVerticalMerge(boxes, medianHeights, medianWidths, isEnglish)
	result.Metrics.BoxesVertMerge = len(boxes)
	if err := ctx.Err(); err != nil {
		return err
	}

	boxes = tbl.ExtractTableAndReplace(boxes, result.Tables)
	boxes = tbl.ConsolidateFigures(boxes)

	pageHeights := make(map[int]float64, len(result.PageImages))
	for pg, img := range result.PageImages {
		pageHeights[pg] = float64(img.Bounds().Dy()) / p.Config.Zoom
	}
	result.Sections = lyt.BoxesToSections(boxes, pageHeights)
	result.Metrics.BoxesFinal = len(result.Sections)
	result.Sections = tbl.MergeCaptions(result.Sections, result.Figures())
	return nil
}

// processPages runs the full pipeline on pages [fromPage, toPage].
// prescanChars provides pre-extracted chars (avoids double extraction).
func (p *Parser) processPages(ctx context.Context, engine pdf.PDFEngine,
	fromPage, toPage int,
	prescanChars map[int][]pdf.TextChar,
	medianHeights, medianWidths map[int]float64,
	isEnglish, isScanNoiseDoc bool,
) (*pdf.ParseResult, error) {
	result := &pdf.ParseResult{PageImages: make(map[int]image.Image)}

	// 1. OCR extraction — per-page detect + recognize + char merge.
	boxes, pageChars, ocrUsedAny, ocrErr := p.extractPages(ctx, engine,
		fromPage, toPage, prescanChars,
		medianHeights, medianWidths, result.PageImages)
	if ocrErr != nil {
		slog.Warn("extractPages: some pages failed OCR", "err", ocrErr)
	}
	// 2. Scan noise retry — re-OCR all pages when prescan detects scan noise.
	if isScanNoiseDoc {
		boxes, pageChars, ocrUsedAny = p.retryScanNoise(ctx, engine,
			fromPage, toPage, result.PageImages,
			pageChars, medianHeights, medianWidths, ocrUsedAny)
	}

	// 3. Zoom retry — re-render at higher resolution if OCR produced zero boxes.
	if len(boxes) == 0 && p.Config.Zoom < 9 && !p.Config.SkipOCR {
		boxes, ocrUsedAny = p.retryZoom(ctx, engine, fromPage, toPage,
			result.PageImages, boxes, ocrUsedAny)
	}

	if len(boxes) == 0 {
		return result, nil
	}

	// 4. Layout pipeline — DLA → TSR → Column → TextMerge → VM → Sections.
	if err := p.buildLayout(ctx, result, engine, boxes, pageChars,
		medianHeights, medianWidths, fromPage, toPage, ocrUsedAny, isEnglish); err != nil {
		return nil, fmt.Errorf("buildLayout: %w", err)
	}
	// 5. Crop section images from page renders.
	p.fillSectionImages(result)

	return result, nil
}

// fillSectionImages populates result.Sections[i].Image with cropped
// page images. Table sections are matched to their TableItem image;
// figure sections try DLA-aware cropping first, then fall back to
// position-tag-based cropping.
func (p *Parser) fillSectionImages(result *pdf.ParseResult) {
	if len(result.PageImages) == 0 {
		return
	}
	// Build lookup: DLA region -> table image (base64).
	tableImgByRegion := make(map[string]string, len(result.Tables))
	for _, tbl := range result.Tables {
		if tbl.ImageB64 == "" {
			continue
		}
		pg := 0
		if len(tbl.Positions) > 0 && len(tbl.Positions[0].PageNumbers) > 0 {
			pg = tbl.Positions[0].PageNumbers[0]
		}
		key := fmt.Sprintf("%d_%.1f_%.1f_%.1f_%.1f",
			pg, tbl.RegionLeft, tbl.RegionRight, tbl.RegionTop, tbl.RegionBottom)
		tableImgByRegion[key] = tbl.ImageB64
	}
	for i := range result.Sections {
		if result.Sections[i].LayoutType == pdf.LayoutTypeTable && len(result.Sections[i].Positions) > 0 {
			pos := result.Sections[i].Positions[0]
			pg := 0
			if len(pos.PageNumbers) > 0 {
				pg = pos.PageNumbers[0]
			}
			key := fmt.Sprintf("%d_%.1f_%.1f_%.1f_%.1f",
				pg, pos.Left, pos.Right, pos.Top, pos.Bottom)
			if img, ok := tableImgByRegion[key]; ok {
				result.Sections[i].Image = img
				continue
			}
		}
		// Try DLA-aware cropping for figure sections (matching Python's
		// cropout which uses DLA region boundaries instead of text boxes).
		if result.Sections[i].LayoutType == pdf.LayoutTypeFigure && len(result.Sections[i].Positions) > 0 {
			if dlaImg := util.CropSectionByDLA(result.Sections[i], result.DLADebug, result.PageImages); dlaImg != "" {
				result.Sections[i].Image = dlaImg
				continue
			}
		}
		img := util.CropSectionImage(result.Sections[i].PositionTag, result.PageImages, p.Config.Zoom)
		result.Sections[i].Image = img
		if img == "" && result.Sections[i].Text != "" {
			tag := result.Sections[i].PositionTag
			slog.Warn("cropSectionImage empty for non-empty section",
				"section", i, "posTag", tag[:min(80, len(tag))])
		}
	}
}
