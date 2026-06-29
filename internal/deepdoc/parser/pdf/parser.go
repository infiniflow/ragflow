package pdf

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

// Parser is the core PDF text/layout extraction pipeline.
// It corresponds to RAGFlowPdfParser in pdf_parser.py.
// Stateless after construction — safe to reuse across documents.
type Parser struct {
	Config pdf.ParserConfig
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
	if c, ok := doc.(*inf.Client); ok {
		c.DLALabels = inf.DefaultDLALabels()
		c.TSRLabels = inf.DefaultTSRLabels()
	}
	return tbl.NewDeepDocTableBuilder(doc)
}

// ── Public API ─────────────────────────────────────────────────────────────

// ParseRaw is the internal entry point: runs the core pipeline on an
// already-opened engine. Exported for tests that inject mock engines.
func (p *Parser) ParseRaw(ctx context.Context, engine pdf.PDFEngine, docAnalyzer pdf.DocAnalyzer) (*pdf.ParseResult, error) {
	tb := NewTableBuilderFor(docAnalyzer)

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
		batchSize = 50
	}

	// ── Prescan ──
	prescanChars := make(map[int][]pdf.TextChar)
	prescanMedianH := make(map[int]float64)
	prescanMedianW := make(map[int]float64)
	for pg := fromPage; pg <= toPage; pg++ {
		chars, extractErr := engine.ExtractChars(pg)
		if extractErr != nil {
			slog.Warn("prescan: ExtractChars failed", "page", pg, "err", extractErr)
			chars = nil
		}
		prescanChars[pg] = chars
		prescanMedianH[pg] = util.MedianCharHeight(chars)
		prescanMedianW[pg] = util.MedianCharWidth(chars)
	}
	isEnglish := util.DetectEnglish(prescanChars, totalPages, nil)
	scanNoise := util.IsScanNoise(util.FullTextFromChars(prescanChars))

	// ── Outlines ──
	outlines, outlineErr := engine.Outlines()
	if outlineErr != nil {
		slog.Warn("Failed to extract PDF outlines; continuing without them", "err", outlineErr)
		outlines = nil
	}

	// ── Small document ──
	if totalPages <= batchSize {
		result, err := p.processPages(ctx, engine, fromPage, toPage,
			prescanChars, prescanMedianH, prescanMedianW, isEnglish, scanNoise,
			docAnalyzer, tb)
		if err != nil {
			return nil, err
		}
		result.Outlines = outlines
		return result, nil
	}

	// ── Large document: batched ──
	slog.Info("batched processing", "pages", totalPages, "batchSize", batchSize)
	result := &pdf.ParseResult{PageImages: make(map[int]image.Image)}
	for start := fromPage; start <= toPage; start += batchSize {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("cancelled at batch starting page %d: %w", start, err)
		}
		end := min(start+batchSize-1, toPage)

		batchChars := make(map[int][]pdf.TextChar, end-start+1)
		batchMH := make(map[int]float64, end-start+1)
		batchMW := make(map[int]float64, end-start+1)
		for pg := start; pg <= end; pg++ {
			batchChars[pg] = prescanChars[pg]
			batchMH[pg] = prescanMedianH[pg]
			batchMW[pg] = prescanMedianW[pg]
		}

		batch, err := p.processPages(ctx, engine, start, end,
			batchChars, batchMH, batchMW, isEnglish, scanNoise,
			docAnalyzer, tb)
		if err != nil {
			return nil, err
		}

		result.Sections = append(result.Sections, batch.Sections...)
		result.Tables = append(result.Tables, batch.Tables...)
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

// ── Internal pipeline steps ────────────────────────────────────────────────

func (p *Parser) extractPages(ctx context.Context, engine pdf.PDFEngine,
	fromPage, toPage int,
	prescanChars map[int][]pdf.TextChar,
	medianHeights, medianWidths map[int]float64,
	pageImages map[int]image.Image,
	docAnalyzer pdf.DocAnalyzer,
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

	cap := p.Config.MaxOCRConcurrency
	if cap <= 0 {
		cap = 1
	}
	sem := make(chan struct{}, cap)
	var wg sync.WaitGroup

	for i := 0; i < pageCount; i++ {
		pg := fromPage + i
		chars := prescanChars[pg]

		if len(chars) > 0 && !util.IsGarbledPage(chars) {
			pageImg, renderErr := RenderPageToImage(engine, pg)
			if renderErr == nil && pageImg != nil {
				pageImages[pg] = pageImg
			}
			var ocrBoxes []pdf.TextBox
			ocrUsed := false
			if !p.Config.SkipOCR && renderErr == nil && pageImg != nil {
				ocrBoxes = ocrMergeChars(ctx, pageImg, chars, docAnalyzer, pg)
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

			pageImg, err := RenderPageToImage(engine, pg)
			if err != nil {
				results[i] = pr{pg: pg, err: err}
				return
			}
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
				ocrBoxes = ocrDetectAndRecognize(ctx, pageImg, docAnalyzer, pg, label)
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
			if !ocrUsed && len(chars) > 0 && !p.Config.SkipOCR {
				ocrBoxes = ocrMergeChars(ctx, pageImg, chars, docAnalyzer, pg)
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

func (p *Parser) retryScanNoise(ctx context.Context, engine pdf.PDFEngine,
	fromPage, toPage int,
	pageImages map[int]image.Image,
	pageChars map[int][]pdf.TextChar,
	medianHeights, medianWidths map[int]float64,
	ocrUsedAny bool,
	docAnalyzer pdf.DocAnalyzer,
) ([]pdf.TextBox, map[int][]pdf.TextChar, bool) {
	slog.Warn("scan noise: OCR retry", "from", fromPage, "to", toPage)
	var boxes []pdf.TextBox
	for pg := fromPage; pg <= toPage; pg++ {
		img := pageImages[pg]
		if img == nil {
			var err error
			img, err = RenderPageToImage(engine, pg)
			if err != nil {
				slog.Warn("scan noise: page render failed", "page", pg, "err", err)
				continue
			}
			pageImages[pg] = img
		}
		ocrBoxes := ocrDetectAndRecognize(ctx, img, docAnalyzer, pg, "scan page")
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

func (p *Parser) retryZoom(ctx context.Context, engine pdf.PDFEngine,
	fromPage, toPage int,
	pageImages map[int]image.Image,
	boxes []pdf.TextBox, ocrUsedAny bool,
	docAnalyzer pdf.DocAnalyzer,
) ([]pdf.TextBox, bool) {
	retryZoomVal := p.Config.Zoom * pdf.DlaScale
	retryDPI := retryZoomVal * 72
	slog.Info("zoom retry: re-rendering", "oldZoom", p.Config.Zoom, "newZoom", retryZoomVal)
	for pg := fromPage; pg <= toPage; pg++ {
		img, err := engine.RenderPageImage(pg, retryDPI)
		if err != nil {
			slog.Warn("zoom retry: render failed", "page", pg, "err", err)
			continue
		}
		pageImages[pg] = img
		if retryDPI != pdf.DlaDPI {
			if dlaImg, dlaErr := engine.RenderPageImage(pg, pdf.DlaDPI); dlaErr == nil {
				pageImages[pg] = dlaImg
			}
		}
		ocrBoxes := ocrDetectAndRecognize(ctx, img, docAnalyzer, pg, "zoom retry")
		if ocrBoxes == nil {
			continue
		}
		scaleFactor := retryZoomVal / p.Config.Zoom
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

func (p *Parser) buildLayout(ctx context.Context,
	result *pdf.ParseResult, engine pdf.PDFEngine,
	boxes []pdf.TextBox, pageChars map[int][]pdf.TextChar,
	medianHeights, medianWidths map[int]float64,
	fromPage, toPage int, ocrUsedAny bool, isEnglish bool,
	docAnalyzer pdf.DocAnalyzer, tb pdf.TableBuilder,
) error {
	result.Metrics.BoxesInitial = len(boxes)

	result.Tables = p.enrichWithDeepDoc(ctx, result, engine, boxes, result.PageImages, docAnalyzer, tb)
	result.Metrics.TablesCount = len(result.Tables)
	if err := ctx.Err(); err != nil {
		return err
	}

	boxes = lyt.AssignColumn(boxes, p.Config.Zoom)
	boxes = lyt.TextMerge(boxes, medianHeights, p.Config.Zoom)
	result.Metrics.BoxesTextMerge = len(boxes)

	lyt.SortByPageThenY(boxes, p.Config.SortByTop)

	if ocrUsedAny {
		isEnglish = util.DetectEnglish(pageChars, toPage-fromPage+1, nil)
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

func (p *Parser) processPages(ctx context.Context, engine pdf.PDFEngine,
	fromPage, toPage int,
	prescanChars map[int][]pdf.TextChar,
	medianHeights, medianWidths map[int]float64,
	isEnglish, isScanNoiseDoc bool,
	docAnalyzer pdf.DocAnalyzer, tb pdf.TableBuilder,
) (*pdf.ParseResult, error) {
	result := &pdf.ParseResult{PageImages: make(map[int]image.Image)}

	boxes, pageChars, ocrUsedAny, ocrErr := p.extractPages(ctx, engine,
		fromPage, toPage, prescanChars,
		medianHeights, medianWidths, result.PageImages, docAnalyzer)
	if ocrErr != nil {
		slog.Warn("extractPages: some pages failed OCR", "err", ocrErr)
	}

	if isScanNoiseDoc {
		boxes, pageChars, ocrUsedAny = p.retryScanNoise(ctx, engine,
			fromPage, toPage, result.PageImages,
			pageChars, medianHeights, medianWidths, ocrUsedAny, docAnalyzer)
	}

	if len(boxes) == 0 && p.Config.Zoom < 9 && !p.Config.SkipOCR {
		boxes, ocrUsedAny = p.retryZoom(ctx, engine, fromPage, toPage,
			result.PageImages, boxes, ocrUsedAny, docAnalyzer)
	}

	if len(boxes) == 0 {
		return result, nil
	}

	if err := p.buildLayout(ctx, result, engine, boxes, pageChars,
		medianHeights, medianWidths, fromPage, toPage, ocrUsedAny, isEnglish,
		docAnalyzer, tb); err != nil {
		return nil, fmt.Errorf("buildLayout: %w", err)
	}
	p.fillSectionImages(result)
	return result, nil
}

func (p *Parser) fillSectionImages(result *pdf.ParseResult) {
	if len(result.PageImages) == 0 {
		return
	}
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
		if result.Sections[i].LayoutType == pdf.LayoutTypeTable {
			if img, ok := matchTableImage(&result.Sections[i], tableImgByRegion); ok {
				result.Sections[i].Image = img
				continue
			}
		}
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

// matchTableImage looks up a pre-rendered table image for a section.
// Uses Positions if available; falls back to TableItem Region boundaries.
func matchTableImage(sec *pdf.Section, tableImgByRegion map[string]string) (string, bool) {
	pg := 0
	if len(sec.Positions) > 0 {
		pos := sec.Positions[0]
		if len(pos.PageNumbers) > 0 {
			pg = pos.PageNumbers[0]
		}
		key := fmt.Sprintf("%d_%.1f_%.1f_%.1f_%.1f", pg, pos.Left, pos.Right, pos.Top, pos.Bottom)
		if img, ok := tableImgByRegion[key]; ok {
			return img, true
		}
		return "", false
	}
	if sec.TableItem != nil {
		key := fmt.Sprintf("%d_%.1f_%.1f_%.1f_%.1f", pg,
			sec.TableItem.RegionLeft, sec.TableItem.RegionRight,
			sec.TableItem.RegionTop, sec.TableItem.RegionBottom)
		if img, ok := tableImgByRegion[key]; ok {
			return img, true
		}
	}
	return "", false
}
