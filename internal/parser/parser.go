package parser

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"math"
	"math/rand/v2"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// dlaDPI is the DPI used for rendering page images for DeepDoc DLA/OCR.
const dlaDPI = 216

// dlaScale is the scale factor from PDF points (72 DPI) to DLA image space.
const dlaScale = dlaDPI / 72.0

// Parser is the main PDF text/layout extraction pipeline.
// It corresponds to RAGFlowPdfParser in pdf_parser.py.
// Parser is stateless after construction — safe to reuse across documents.
type Parser struct {
	Config ParserConfig

	// DeepDoc is the required document layout / OCR / table recognition
	// service. Set at construction time by NewParser.
	DeepDoc DocAnalyzer

	// SampleChars samples up to n chars from a page for English detection.
	// Defaults to random sampling (matching Python's random.choices).
	// Inject a deterministic sampler for reproducible tests.
	SampleChars SampleFunc

	// tableBuilder is the TSR model adapter. Set at construction time
	// by NewParser from DeepDoc.ModelType(). Callers can inject a
	// different implementation via Config.TableBuilder.
	tableBuilder TableBuilder

	// debugDLA and debugTSR collect intermediates for comparison with Python.
	// Set before Parse(), read from ParseResult after, cleared by Parse().
	debugDLA []DLAPageRegions
	debugTSR []TSRRawCell
}

// PDFEngine abstracts page extraction capabilities.
// Calling code provides the implementation (pdfplumber-rs, etc.).
type PDFEngine interface {
	// ExtractChars returns all characters on a page with position data.
	// pageNum is 0-indexed.
	ExtractChars(pageNum int) ([]TextChar, error)

	// RenderPage renders a page to PNG bytes at the given DPI.
	RenderPage(pageNum int, dpi float64) ([]byte, error)

	// RenderPageImage renders a page as image.Image at the given DPI.
	// Used by DeepDoc DLA/TSR/OCR which need width/height metadata.
	RenderPageImage(pageNum int, dpi float64) (image.Image, error)

	// RawData returns the original PDF bytes, used by the pdfium
	// rendering path.  Must return the full, unmodified PDF content.
	RawData() []byte

	// PageCount returns the total number of pages.
	PageCount() (int, error)

	// Close releases resources held by the engine.
	Close() error
}

// Tokenizer provides text tokenization matching rag_tokenizer.
// Used by MergeSameBullet to detect Chinese characters.
type Tokenizer interface {
	Tag(token string) string // POS tag
}

// SampleFunc samples up to n characters from a page's chars,
// returning them concatenated as a single string.
// The default implementation uses random sampling (matching Python's
// random.choices).  Tests can inject a deterministic sampler.
type SampleFunc func(chars []TextChar, n int) string

// NewParser creates a new Parser with the required DeepDoc service.
func NewParser(cfg ParserConfig, doc DocAnalyzer) *Parser {
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

// Parse runs the full PDF extraction pipeline: chars → boxes →
// column assignment → text merge → vertical merge → sections.
//
// For documents larger than Config.ChunkSize pages, processes in chunks
// to bound memory usage (matching Python's batch_size=50).
//
// Returns a ParseResult containing sections, tables, page images, figures,
// and pipeline stage metrics. Parser itself remains stateless.
func (p *Parser) Parse(ctx context.Context, engine PDFEngine) (*ParseResult, error) {
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
		return &ParseResult{PageImages: make(map[int]image.Image)}, nil
	}

	totalPages := toPage - fromPage + 1
	chunkSize := p.Config.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 50 // default, matching Python's batch_size
	}

	// ── Prescan: lightweight char extraction for language/noise detection ──
	// No rendering, no OCR — just raw chars for global decisions.
	prescanChars := make(map[int][]TextChar)
	prescanMedianH := make(map[int]float64)
	prescanMedianW := make(map[int]float64)
	for pg := fromPage; pg <= toPage; pg++ {
		chars, extractErr := engine.ExtractChars(pg)
		if extractErr != nil {
			slog.Warn("prescan: ExtractChars failed", "page", pg, "err", extractErr)
			chars = nil // skip broken pages (matching old behavior)
		}
		prescanChars[pg] = chars
		prescanMedianH[pg] = MedianCharHeight(chars)
		prescanMedianW[pg] = MedianCharWidth(chars)
	}
	isEnglish := detectEnglish(prescanChars, totalPages, p.SampleChars)
	scanNoise := isScanNoise(fullTextFromChars(prescanChars))

	// ── Small document: process all at once (no chunking overhead) ──
	if totalPages <= chunkSize {
		return p.processPages(ctx, engine, fromPage, toPage,
			prescanChars, prescanMedianH, prescanMedianW, isEnglish, scanNoise)
	}

	// ── Large document: process in chunks to bound memory ──
	slog.Info("chunked processing", "pages", totalPages, "chunkSize", chunkSize)
	result := &ParseResult{PageImages: make(map[int]image.Image)}
	for start := fromPage; start <= toPage; start += chunkSize {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("cancelled at chunk starting page %d: %w", start, err)
		}
		end := min(start+chunkSize-1, toPage)

		// Slice prescan data for this chunk.
		chunkChars := make(map[int][]TextChar, end-start+1)
		chunkMH := make(map[int]float64, end-start+1)
		chunkMW := make(map[int]float64, end-start+1)
		for pg := start; pg <= end; pg++ {
			chunkChars[pg] = prescanChars[pg]
			chunkMH[pg] = prescanMedianH[pg]
			chunkMW[pg] = prescanMedianW[pg]
		}

		chunk, err := p.processPages(ctx, engine, start, end,
			chunkChars, chunkMH, chunkMW, isEnglish, scanNoise)
		if err != nil {
			return nil, err
		}

		// Merge chunk results.
		result.Sections = append(result.Sections, chunk.Sections...)
		result.Tables = append(result.Tables, chunk.Tables...)
		result.Figures = append(result.Figures, chunk.Figures...)
		for pg, img := range chunk.PageImages {
			result.PageImages[pg] = img
		}
		result.Metrics.BoxesInitial += chunk.Metrics.BoxesInitial
		result.Metrics.BoxesTextMerge += chunk.Metrics.BoxesTextMerge
		result.Metrics.BoxesVertMerge += chunk.Metrics.BoxesVertMerge
		result.Metrics.BoxesFinal += chunk.Metrics.BoxesFinal
		result.Metrics.TablesCount += chunk.Metrics.TablesCount
	}
	return result, nil
}

// extractPages runs per-page OCR (detect + recognize) for the given page
// range, returning text boxes, char data, whether any page used OCR, and
// any errors encountered.  Partial results are returned even when some
// pages fail — callers should inspect the error for diagnostics but may
// still use the returned boxes and chars.
func (p *Parser) extractPages(ctx context.Context, engine PDFEngine,
	fromPage, toPage int,
	prescanChars map[int][]TextChar,
	medianHeights, medianWidths map[int]float64,
	pageImages map[int]image.Image,
) ([]TextBox, map[int][]TextChar, bool, error) {
	var boxes []TextBox
	pageChars := make(map[int][]TextChar)
	ocrUsedAny := false

	type pr struct {
		pg       int
		ocrBoxes []TextBox
		chars    []TextChar
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
		if len(chars) > 0 && !isGarbledPage(chars) {
			pageImg, renderErr := renderPageToImage(engine, pg)
			if renderErr == nil && pageImg != nil {
				pageImages[pg] = pageImg
			}
			var ocrBoxes []TextBox
			ocrUsed := false
			if !p.Config.SkipOCR && renderErr == nil && pageImg != nil {
				ocrBoxes = ocrMergeChars(ctx, pageImg, chars, p.DeepDoc, pg)
				if ocrBoxes == nil {
					ocrBoxes = charsToBoxes(chars, pg, p.Config.SortByTop)
				} else {
					ocrUsed = true
					ocrUsedAny = true
				}
			} else {
				ocrBoxes = charsToBoxes(chars, pg, p.Config.SortByTop)
			}
			results[i] = pr{pg: pg, ocrBoxes: ocrBoxes, chars: chars, ocrUsed: ocrUsed}
			continue
		}

		// OCR path: render + detect + recognize (potentially parallel).
		wg.Add(1)
		go func(i, pg int, chars []TextChar) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			pageImg, err := renderPageToImage(engine, pg)
			if err != nil {
				results[i] = pr{pg: pg, err: err}
				return
			}

			var ocrBoxes []TextBox
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
							chars = append(chars, TextChar{Text: string(r), PageNumber: pg})
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
					ocrBoxes = charsToBoxes(chars, pg, p.Config.SortByTop)
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
			medianHeights[r.pg] = MedianCharHeight(r.chars)
			medianWidths[r.pg] = MedianCharWidth(r.chars)
		}
	}
	return boxes, pageChars, ocrUsedAny, errors.Join(errs...)
}

// retryScanNoise re-runs OCR on all pages when prescan detects scan noise,
// overwriting page-level state with fresh detect+recognize results.
func (p *Parser) retryScanNoise(ctx context.Context, engine PDFEngine,
	fromPage, toPage int,
	pageImages map[int]image.Image,
	pageChars map[int][]TextChar,
	medianHeights, medianWidths map[int]float64,
	ocrUsedAny bool,
) ([]TextBox, map[int][]TextChar, bool) {
	slog.Warn("scan noise: OCR retry", "from", fromPage, "to", toPage)
	var boxes []TextBox
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
		var chars []TextChar
		for _, b := range ocrBoxes {
			for _, r := range b.Text {
				chars = append(chars, TextChar{Text: string(r), Top: b.Top, Bottom: b.Bottom, PageNumber: pg})
				break
			}
		}
		pageChars[pg] = chars
		medianHeights[pg] = MedianCharHeight(chars)
		medianWidths[pg] = MedianCharWidth(chars)
	}
	slog.Debug("scan noise OCR retry complete", "pages", toPage-fromPage+1, "boxes", len(boxes))
	return boxes, pageChars, true
}

// retryZoom re-renders pages at higher resolution and re-runs OCR when the
// initial extraction produced zero boxes.  Box coordinates are scaled back
// to Config.Zoom space.  Matches Python's __images__ retry.
func (p *Parser) retryZoom(ctx context.Context, engine PDFEngine,
	fromPage, toPage int,
	pageImages map[int]image.Image,
	boxes []TextBox, ocrUsedAny bool,
) ([]TextBox, bool) {
	retryZoom := p.Config.Zoom * dlaScale
	retryDPI := retryZoom * 72
	slog.Info("zoom retry: re-rendering", "oldZoom", p.Config.Zoom, "newZoom", retryZoom)
	for pg := fromPage; pg <= toPage; pg++ {
		img, err := engine.RenderPageImage(pg, retryDPI)
		if err != nil {
			slog.Warn("zoom retry: render failed", "page", pg, "err", err)
			continue
		}
		pageImages[pg] = img
		// Downstream DLA/TSR assumes dlaDPI. Re-render at standard
		// resolution so layout coordinates are scaled correctly.
		if retryDPI != dlaDPI {
			if dlaImg, dlaErr := engine.RenderPageImage(pg, dlaDPI); dlaErr == nil {
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

// buildLayout runs the DLA → TSR → Column → TextMerge → VM → Section
// pipeline and populates result.Metrics, result.Tables, result.Sections,
// and result.Figures.  Matches Python's _parse_loaded_window_into_bboxes
// order.
func (p *Parser) buildLayout(ctx context.Context,
	result *ParseResult, engine PDFEngine,
	boxes []TextBox, pageChars map[int][]TextChar,
	medianHeights, medianWidths map[int]float64,
	fromPage, toPage int, ocrUsedAny bool, isEnglish bool,
) {
	_ = ctx
	result.Metrics.BoxesInitial = len(boxes)

	result.Tables = p.enrichWithDeepDoc(ctx, engine, boxes, result.PageImages)
	result.Metrics.TablesCount = len(result.Tables)

	boxes = AssignColumn(boxes, p.Config.Zoom)
	boxes = TextMerge(boxes, medianHeights, p.Config.Zoom)
	result.Metrics.BoxesTextMerge = len(boxes)

	sortByPageThenY(boxes, p.Config.SortByTop)

	if ocrUsedAny {
		isEnglish = detectEnglish(pageChars, toPage-fromPage+1, p.SampleChars)
	}
	boxes = NaiveVerticalMerge(boxes, medianHeights, medianWidths, isEnglish)
	result.Metrics.BoxesVertMerge = len(boxes)

	boxes = extractTableAndReplace(boxes, result.Tables)
	boxes = consolidateFigures(boxes)

	pageHeights := make(map[int]float64, len(result.PageImages))
	for pg, img := range result.PageImages {
		pageHeights[pg] = float64(img.Bounds().Dy()) / p.Config.Zoom
	}
	result.Sections = boxesToSections(boxes, pageHeights)
	result.Metrics.BoxesFinal = len(result.Sections)
	result.Figures = CollectFigures(result.Sections)
	result.Sections = mergeCaptions(result.Sections, result.Figures)
}

// processPages runs the full pipeline on pages [fromPage, toPage].
// prescanChars provides pre-extracted chars (avoids double extraction).
func (p *Parser) processPages(ctx context.Context, engine PDFEngine,
	fromPage, toPage int,
	prescanChars map[int][]TextChar,
	medianHeights, medianWidths map[int]float64,
	isEnglish, isScanNoiseDoc bool,
) (*ParseResult, error) {
	result := &ParseResult{PageImages: make(map[int]image.Image)}

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
	p.buildLayout(ctx, result, engine, boxes, pageChars,
		medianHeights, medianWidths, fromPage, toPage, ocrUsedAny, isEnglish)
	// Text sections use cropSectionImage based on their PositionTag.
	if len(result.PageImages) > 0 {
		// Build lookup: DLA region → TableItem index for image matching.
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
			if result.Sections[i].LayoutType == "table" && len(result.Sections[i].Positions) > 0 {
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
			if result.Sections[i].LayoutType == "figure" && len(result.Sections[i].Positions) > 0 {
				if dlaImg := cropSectionByDLA(result.Sections[i], p.debugDLA, result.PageImages); dlaImg != "" {
					result.Sections[i].Image = dlaImg
					continue
				}
			}
			img := cropSectionImage(result.Sections[i].PositionTag, result.PageImages, p.Config.Zoom)
			result.Sections[i].Image = img
			if img == "" && result.Sections[i].Text != "" {
				tag := result.Sections[i].PositionTag
				slog.Debug("cropSectionImage empty for non-empty section",
					"section", i, "posTag", tag[:min(80, len(tag))])
			}
		}
	}

	// Collect DLA/TSR debug intermediates if available.
	result.DLADebug = p.debugDLA
	result.TSRDebug = p.debugTSR
	p.debugDLA = nil
	p.debugTSR = nil
	return result, nil
}

// isASCIIPrintable returns true for characters that match Python's
// is_english regex: [ a-zA-Z0-9,/¸;:'\[\]\(\)!@#$%^&*\"?<>._-]
func isASCIIPrintable(r rune) bool {
	if r == ' ' {
		return true
	}
	if r >= 'a' && r <= 'z' {
		return true
	}
	if r >= 'A' && r <= 'Z' {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	// Additional ASCII symbols from the Python regex
	switch r {
	case ',', '/', '¸', ';', ':', '\'', '[', ']', '(', ')',
		'!', '@', '#', '$', '%', '^', '&', '*', '"', '?',
		'<', '>', '.', '_', '-':
		return true
	}
	return false
}

// defaultSampleChars returns a random sample of up to n character texts,
// concatenated.  Matches Python's random.choices([c["text"] for c in
// page_chars], k=min(100, len(page_chars))).
func defaultSampleChars(chars []TextChar, n int) string {
	if n <= 0 || len(chars) == 0 {
		return ""
	}
	m := min(n, len(chars))
	// Fisher-Yates shuffle on indices, then take first m.
	indices := make([]int, len(chars))
	for i := range indices {
		indices[i] = i
	}
	rand.Shuffle(len(indices), func(i, j int) {
		indices[i], indices[j] = indices[j], indices[i]
	})
	var buf strings.Builder
	for i := 0; i < m; i++ {
		buf.WriteString(chars[indices[i]].Text)
	}
	return buf.String()
}

// fullTextFromChars concatenates all chars text across pages for scan noise detection.
func fullTextFromChars(pageChars map[int][]TextChar) string {
	var sb strings.Builder
	for _, chars := range pageChars {
		for _, c := range chars {
			sb.WriteString(c.Text)
		}
	}
	return sb.String()
}

// detectEnglish detects whether a PDF is primarily English by per-page
// majority vote, matching Python's is_english logic in __images__
// (pdf_parser.py:1519-1526).
//
// Each page: sample up to 100 character texts via sampler, join into one
// string, check if there is a run of 30+ consecutive ASCII characters
// (letters, digits, spaces, punctuation).  Pages with such a run vote
// "English".  Returns true when a strict majority of pages vote yes.
//
// totalPages is the denominator (len(self.page_images) in Python), including
// image-only pages that have zero chars.  This matches Python's behavior
// where empty pages dilute the majority.
func detectEnglish(pageChars map[int][]TextChar, totalPages int, sample SampleFunc) bool {
	if totalPages == 0 || len(pageChars) == 0 {
		return false
	}
	if sample == nil {
		sample = defaultSampleChars
	}
	pagesWithSeq := 0

	for _, chars := range pageChars {
		if len(chars) == 0 {
			continue
		}
		sampleText := sample(chars, 100)
		run := 0
		for _, r := range sampleText {
			if isASCIIPrintable(r) {
				run++
				if run >= 30 {
					pagesWithSeq++
					break
				}
			} else {
				run = 0
			}
		}
	}

	return pagesWithSeq > totalPages/2
}

// charsToBoxes converts raw characters to initial text boxes by grouping
// characters into lines based on vertical overlap.
//
// Python: pdf_parser.__images__ producing self.boxes
func charsToBoxes(chars []TextChar, pageNum int, sortByTop bool) []TextBox {
	if len(chars) == 0 {
		return nil
	}

	lines := groupCharsToLines(chars, sortByTop)

	// Page-level column gap threshold from ALL inter-char gaps.
	// Falls back to per-line threshold when page has too few gaps.
	threshold := pageXGapThreshold(lines)

	boxes := make([]TextBox, 0, len(lines))
	for _, line := range lines {
		thr := threshold
		if thr > 100 {
			// No significant column gaps on this page → use per-line threshold.
			thr = perLineXGapThreshold(line)
		}
		subLines := splitLineByXGap(line, thr)
		for _, sub := range subLines {
			box := lineToTextBox(sub)
			box.PageNumber = pageNum
			boxes = append(boxes, box)
		}
	}
	return boxes
}

// perLineXGapThreshold computes a dynamic X-gap threshold for column
// splitting within a single line (fallback when page has few gaps).
func perLineXGapThreshold(chars []TextChar) float64 {
	if len(chars) <= 1 {
		return 1e9
	}
	var gaps []float64
	for i := 1; i < len(chars); i++ {
		g := chars[i].X0 - chars[i-1].X1
		gaps = append(gaps, g)
	}
	if len(gaps) == 0 {
		return 1e9
	}
	sort.Float64s(gaps)
	medianGap := gaps[len(gaps)/2]
	if medianGap < 6 {
		medianGap = 6
	}
	return medianGap * 2.5
}

// pageXGapThreshold computes a global X-gap column threshold from all
// inter-char gaps across all lines on the page.  95th percentile catches
// column boundaries while excluding word-level gaps.
// Returns a value > 100 when there are too few gaps for reliable p95,
// signalling the caller to fall back to perLineXGapThreshold.
func pageXGapThreshold(lines [][]TextChar) float64 {
	var allGaps []float64
	for _, line := range lines {
		for i := 1; i < len(line); i++ {
			g := line[i].X0 - line[i-1].X1
			allGaps = append(allGaps, g)
		}
	}
	if len(allGaps) < 10 {
		return 1e9 // too few gaps for reliable p95 → fall back to per-line
	}
	sort.Float64s(allGaps)
	// 95th percentile: only the largest 5% of gaps are column boundaries.
	p95 := allGaps[len(allGaps)*95/100]
	if p95 < 30 {
		p95 = 30 // floor: column gaps are ≥30pt in practice
	}
	return p95
}

// splitLineByXGap splits a character line into sub-lines where X gaps
// meet or exceed the threshold (column boundaries).  Uses >= to match the
// p95 boundary value — a gap exactly at the 95th percentile is a column gap,
// not a word gap.
func splitLineByXGap(chars []TextChar, threshold float64) [][]TextChar {
	if len(chars) <= 1 {
		return [][]TextChar{chars}
	}
	var result [][]TextChar
	start := 0
	for i := 1; i < len(chars); i++ {
		gap := chars[i].X0 - chars[i-1].X1
		if gap >= threshold {
			result = append(result, chars[start:i])
			start = i
		}
	}
	result = append(result, chars[start:])
	return result
}

// resolvePageSpan computes the ending page and bottom coordinate for a box
// that may span multiple pages.  When pageHeights is nil or the box fits
// within its starting page the returned (toPage, bottom) equal the inputs.
//
// Zero or negative page heights are treated as invalid: the span stops at
// the preceding page, guarding against infinite loops caused by corrupted
// page images.
func resolvePageSpan(pageNum int, bottom float64, pageHeights map[int]float64) (toPage int, newBottom float64) {
	toPage = pageNum
	newBottom = bottom
	if pageHeights == nil {
		return
	}
	ph, ok := pageHeights[pageNum]
	if !ok || ph <= 0 || bottom <= ph {
		return
	}
	remaining := bottom
	for remaining > ph && ph > 0 {
		nextPh, ok := pageHeights[toPage+1]
		if !ok || nextPh <= 0 {
			// Unknown or invalid next page height — extend by the
			// last known height once and stop (Python: _line_tag
			// while-loop break path).
			remaining -= ph
			toPage++
			break
		}
		remaining -= ph
		ph = nextPh
		toPage++
	}
	newBottom = remaining
	return
}

// boxesToSections converts layout boxes to section format with position tags.
//
// pageHeights provides the PDF-point height of each page (image height / zoom).
// Boxes that extend beyond their page produce multi-page position tags
// (Python's _line_tag while-loop detection via resolvePageSpan).
//
// Python equivalent: output consumed by naive.py::chunk()
func boxesToSections(boxes []TextBox, pageHeights map[int]float64) []Section {
	sections := make([]Section, 0, len(boxes))
	for _, b := range boxes {
		t := strings.TrimSpace(b.Text)
		if t == "" {
			continue
		}
		toPage, bottom := resolvePageSpan(b.PageNumber, b.Bottom, pageHeights)

		var posTag string
		var pageNums []int
		if b.PageNumber == toPage {
			posTag = FormatPositionTag(b.PageNumber, b.X0, b.X1, b.Top, bottom)
			pageNums = []int{b.PageNumber}
		} else {
			posTag = FormatPositionTagRange(b.PageNumber, toPage, b.X0, b.X1, b.Top, bottom)
			pageNums = make([]int, 0, toPage-b.PageNumber+1)
			for p := b.PageNumber; p <= toPage; p++ {
				pageNums = append(pageNums, p)
			}
		}
		sections = append(sections, Section{
			Text:        t,
			PositionTag: posTag,
			LayoutType:  b.LayoutType,
			Positions:   []Position{{PageNumbers: pageNums, Left: b.X0, Right: b.X1, Top: b.Top, Bottom: bottom}},
		})
	}
	return sections
}

// mergeCaptions finds "figure caption" and "table caption" sections,
// appends their text to the nearest figure/table, then removes the
// caption sections.  Matches Python _extract_table_figure caption
// matching (pdf_parser.py:1196-1232).
// Also uses isCaptionBox to detect captions that DLA mislabeled as
// "text" — matching Python's is_caption(text) pattern matching.
func mergeCaptions(sections []Section, figures []Section) []Section {
	captions := make([]int, 0, 4)
	for i, s := range sections {
		captionType := captionKind(s)
		if captionType == "" {
			continue
		}
		target := findNearestParent(i, s, sections, figures, captionType)
		if target >= 0 {
			// For table sections, prepend caption before the HTML table
			// (matching Python's _extract_table_figure caption->construct_table).
			if sections[target].LayoutType == "table" && sections[target].Text != "" {
				sections[target].Text = s.Text + sections[target].Text
			} else if sections[target].Text != "" {
				sections[target].Text += " " + s.Text
			} else {
				sections[target].Text = s.Text
			}
		}
		captions = append(captions, i)
	}
	// Remove caption sections in reverse order.
	n := len(sections)
	out := make([]Section, 0, n-len(captions))
	capSet := make(map[int]bool, len(captions))
	for _, idx := range captions {
		capSet[idx] = true
	}
	for i, s := range sections {
		if !capSet[i] {
			out = append(out, s)
		}
	}
	return out
}

// findNearestParent finds the nearest figure (for figure caption) or
// table (for table caption) section by position proximity.
// captionType is "table" or "figure" (from captionKind).
// Returns the index in `sections` (for tables) or a virtual index mapping
// to `figures` (negative offset for figures).
func findNearestParent(captionIdx int, caption Section, sections []Section, figures []Section, captionType string) int {
	find := func(targets []Section, skipIdx int) (int, float64) {
		bestIdx := -1
		bestDist := 1e9
		for i, t := range targets {
			if i == skipIdx {
				continue // don't match caption to itself
			}
			if len(t.Positions) == 0 || len(caption.Positions) == 0 {
				continue
			}
			tp := t.Positions[0]
			cp := caption.Positions[0]
			// Squared Euclidean distance (Python _extract_table_figure:1196).
			// Caption is typically below. Use center-point distance.
			cx := (tp.Left + tp.Right) / 2
			cy := (tp.Top + tp.Bottom) / 2
			ccx := (cp.Left + cp.Right) / 2
			ccy := (cp.Top + cp.Bottom) / 2
			dist := (cx-ccx)*(cx-ccx) + (cy-ccy)*(cy-ccy)
			if dist < bestDist {
				bestDist = dist
				bestIdx = i
			}
		}
		return bestIdx, bestDist
	}

	const maxCaptionGap = 40000.0 // PDF points (~7cm) — beyond this, don't attach.
	if captionType == "figure" && len(figures) > 0 {
		idx, dist := find(figures, -1) // figures don't contain the caption itself
		if idx >= 0 && dist < maxCaptionGap {
			// Match by position coordinates, not PositionTag strings.
			f := figures[idx]
			for i, s := range sections {
				if s.LayoutType != "figure" || len(s.Positions) == 0 || len(f.Positions) == 0 {
					continue
				}
				sp, fp := s.Positions[0], f.Positions[0]
				if sp.Left == fp.Left && sp.Right == fp.Right &&
					sp.Top == fp.Top && sp.Bottom == fp.Bottom {
					return i
				}
			}
		}
	}
	if captionType == "table" {
		idx, dist := find(sections, captionIdx)
		if idx >= 0 && dist < maxCaptionGap && sections[idx].LayoutType == "table" {
			return idx
		}
	}
	return -1
}

// sortByPageThenY sorts boxes by page → vertical key → x0.
func sortByPageThenY(boxes []TextBox, sortByTop bool) {
	key := func(b TextBox) float64 { return b.Bottom }
	if sortByTop {
		key = func(b TextBox) float64 { return b.Top }
	}
	sort.Slice(boxes, func(i, j int) bool {
		if boxes[i].PageNumber != boxes[j].PageNumber {
			return boxes[i].PageNumber < boxes[j].PageNumber
		}
		if key(boxes[i]) != key(boxes[j]) {
			return key(boxes[i]) < key(boxes[j])
		}
		return boxes[i].X0 < boxes[j].X0
	})
}

// ---- internal helpers ----

// groupCharsToLines groups characters into horizontal lines based on vertical overlap.
func groupCharsToLines(chars []TextChar, sortByTop bool) [][]TextChar {
	if len(chars) == 0 {
		return nil
	}

	key := func(c TextChar) float64 { return c.Bottom }
	if sortByTop {
		key = func(c TextChar) float64 { return c.Top }
	}

	// Sort by vertical key (Bottom or Top) then x0 using sort.SliceStable.
	// Guard against NaN: a NaN key sorts after everything else.
	sort.SliceStable(chars, func(i, j int) bool {
		ki, kj := key(chars[i]), key(chars[j])
		if ki != kj && !math.IsNaN(ki) && !math.IsNaN(kj) {
			return ki < kj
		}
		if math.IsNaN(ki) != math.IsNaN(kj) {
			return !math.IsNaN(ki) // non-NaN before NaN
		}
		return chars[i].X0 < chars[j].X0
	})

	var lines [][]TextChar
	var currentLine []TextChar

	for _, c := range chars {
		if len(currentLine) == 0 {
			currentLine = append(currentLine, c)
			continue
		}
		if verticalOverlap(currentLine[len(currentLine)-1], c) {
			currentLine = append(currentLine, c)
		} else {
			if len(currentLine) > 0 {
				lines = append(lines, currentLine)
			}
			currentLine = []TextChar{c}
		}
	}
	if len(currentLine) > 0 {
		lines = append(lines, currentLine)
	}
	return lines
}

// verticalOverlap checks if two characters are on the same horizontal line.
func verticalOverlap(a, b TextChar) bool {
	mh := math.Max(CharHeight(a), CharHeight(b))
	if mh <= 0 {
		mh = 1.0
	}
	return math.Abs(a.Top-b.Top) < mh*0.5
}

// lineToTextBox converts a line of characters to a single TextBox.
// asciiWordPattern matches strings composed entirely of ASCII word
// characters. Python uses re.match (prefix match) — the stricter
// full-string match here is equivalent in practice because each
// TextChar.Text is a single rune, so prevText+currText ≤ 2 chars.
// Python: pdf_parser.py:1528 re.match(r"[0-9a-zA-Z,.:;!%]+", ...)
var asciiWordPattern = regexp.MustCompile(`^[0-9a-zA-Z,.:;!%]+$`)

func lineToTextBox(chars []TextChar) TextBox {
	if len(chars) == 0 {
		return TextBox{}
	}
	box := TextBox{
		X0:     chars[0].X0,
		X1:     chars[0].X1,
		Top:    chars[0].Top,
		Bottom: chars[0].Bottom,
	}
	var textParts []string
	for i, c := range chars {
		// Insert space between adjacent ASCII words with a visible gap.
		// Python: pdf_parser.py:1524-1532 __img_ocr space insertion.
		if i > 0 {
			prev := chars[i-1]
			prevText := strings.TrimSpace(prev.Text)
			currText := strings.TrimSpace(c.Text)
			if prevText != "" && currText != "" {
				gap := c.X0 - prev.X1
				minWidth := math.Min(c.X1-c.X0, prev.X1-prev.X0)
				if gap >= minWidth/2 &&
					asciiWordPattern.MatchString(prevText+currText) {
					textParts = append(textParts, " ")
				}
			}
		}
		box.X0 = math.Min(box.X0, c.X0)
		box.X1 = math.Max(box.X1, c.X1)
		box.Top = math.Min(box.Top, c.Top)
		box.Bottom = math.Max(box.Bottom, c.Bottom)
		textParts = append(textParts, c.Text)
		if c.LayoutType != "" {
			box.LayoutType = c.LayoutType
		}
		if c.LayoutNo != "" {
			box.LayoutNo = c.LayoutNo
		}
	}
	box.Text = strings.Join(textParts, "")
	return box
}
