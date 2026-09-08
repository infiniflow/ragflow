//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parser

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"sort"
	"strings"
	"time"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	pdflayout "ragflow/internal/deepdoc/parser/pdf/layout"
	"ragflow/internal/deepdoc/parser/pdf/util"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/utility"
)

// ErrPDFEngineUnavailable is returned by PDFParser.ParseWithResult
// when the current build cannot construct the DeepDOC PDF backend.
// The normal reason is a non-cgo build, because the pdfoxide bridge
// is compiled behind `//go:build cgo`.
var ErrPDFEngineUnavailable = errors.New("parser: PDF backend unavailable in this build")

var supportedPDFParseMethods = map[string]struct{}{
	"":               {},
	"deepdoc":        {},
	"plain_text":     {},
	"mineru":         {},
	"paddleocr":      {},
	"docling":        {},
	"opendataloader": {},
	"somark":         {},
	"tcadp":          {},
}

type PDFParser struct {
	ParserType string // DeepDoc, PaddleOCR, MinerU
	Model      string // DeepDoc@buildin@ragflow
	LibType    string // pdf_oxide, used by DeepDoc

	FlattenMediaToText bool
	RemoveTOC          bool
	RemoveHeaderFooter bool
	EnableMultiColumn  bool
	OutputFormat       string
	ParseMethod        string
	// Pages restricts parsing to these 1-indexed inclusive page ranges.
	// nil/empty means parse all pages. Populated by ConfigureFromSetup from
	// the filetype setup map and forwarded to the deepdoc ParserConfig.
	Pages                             [][]int
	MinerUAPIServer                   string
	MinerUAPIKey                      string
	MinerUBackend                     string
	MinerUPollTimeout                 time.Duration
	PaddleOCRBaseURL                  string
	PaddleOCRAPIKey                   string
	PaddleOCRAlgorithm                string
	DoclingServerURL                  string
	DoclingAPIKey                     string
	OpenDataLoaderAPIServer           string
	OpenDataLoaderAPIKey              string
	OpenDataLoaderTimeout             int
	OpenDataLoaderHybrid              string
	OpenDataLoaderImageOutput         string
	OpenDataLoaderSanitize            *bool
	SoMarkBaseURL                     string
	SoMarkAPIKey                      string
	SoMarkImageFormat                 string
	SoMarkFormulaFormat               string
	SoMarkTableFormat                 string
	SoMarkCSFormat                    string
	SoMarkEnableTextCrossPage         bool
	SoMarkEnableTableCrossPage        bool
	SoMarkEnableTitleLevelRecognition bool
	SoMarkEnableInlineImage           bool
	SoMarkEnableTableImage            bool
	SoMarkEnableImageUnderstanding    bool
	SoMarkKeepHeaderFooter            bool
	TCADPAPIServer                    string
	TCADPAPIKey                       string
	TCADPTableResultType              string
	TCADPMarkdownImageResponseType    string
}

func NewPDFParser() *PDFParser {
	return &PDFParser{
		ParserType:                     "DeepDoc",
		Model:                          "DeepDoc@buildin@ragflow",
		LibType:                        "pdf_oxide",
		ParseMethod:                    "deepdoc",
		OutputFormat:                   "json",
		MinerUBackend:                  "pipeline",
		MinerUPollTimeout:              minerUPollTimeout,
		PaddleOCRAlgorithm:             "PaddleOCR-VL",
		OpenDataLoaderTimeout:          600,
		SoMarkBaseURL:                  "https://somark.cn/api/v1",
		SoMarkImageFormat:              "url",
		SoMarkFormulaFormat:            "latex",
		SoMarkTableFormat:              "html",
		SoMarkCSFormat:                 "image",
		SoMarkEnableInlineImage:        true,
		SoMarkEnableTableImage:         true,
		SoMarkEnableImageUnderstanding: true,
		TCADPTableResultType:           "1",
		TCADPMarkdownImageResponseType: "1",
	}
}

func (p *PDFParser) String() string {
	return "PDFParser"
}

func (p *PDFParser) ConfigureFromSetup(setup map[string]any) {
	if p == nil || setup == nil {
		return
	}
	if v, ok := setup["flatten_media_to_text"].(bool); ok {
		p.FlattenMediaToText = v
	}
	if v, ok := setup["remove_toc"].(bool); ok {
		p.RemoveTOC = v
	}
	if v, ok := setup["remove_header_footer"].(bool); ok {
		p.RemoveHeaderFooter = v
	}
	if v, ok := setup["enable_multi_column"].(bool); ok {
		p.EnableMultiColumn = v
	}
	if v, ok := setup["parse_method"].(string); ok && v != "" {
		p.ParseMethod = v
	}
	if v, ok := setup["mineru_apiserver"].(string); ok && v != "" {
		p.MinerUAPIServer = v
	}
	if v, ok := setup["mineru_api_key"].(string); ok {
		p.MinerUAPIKey = v
	}
	if v, ok := setup["mineru_backend"].(string); ok && v != "" {
		p.MinerUBackend = v
	}
	if v, ok := setup["mineru_timeout_seconds"].(int); ok && v > 0 {
		p.MinerUPollTimeout = time.Duration(v) * time.Second
	}
	if v, ok := setup["mineru_timeout_seconds"].(float64); ok && v > 0 {
		p.MinerUPollTimeout = time.Duration(v * float64(time.Second))
	}
	if v, ok := setup["output_format"].(string); ok && v != "" {
		p.OutputFormat = v
	}
	if v, ok := setup["paddleocr_base_url"].(string); ok && v != "" {
		p.PaddleOCRBaseURL = v
	}
	if v, ok := setup["paddleocr_api_key"].(string); ok {
		p.PaddleOCRAPIKey = v
	}
	if v, ok := setup["paddleocr_algorithm"].(string); ok && v != "" {
		p.PaddleOCRAlgorithm = v
	}
	if v, ok := setup["docling_server_url"].(string); ok && v != "" {
		p.DoclingServerURL = v
	}
	if v, ok := setup["docling_api_key"].(string); ok {
		p.DoclingAPIKey = v
	}
	if v, ok := setup["opendataloader_apiserver"].(string); ok && v != "" {
		p.OpenDataLoaderAPIServer = v
	}
	if v, ok := setup["opendataloader_api_key"].(string); ok {
		p.OpenDataLoaderAPIKey = v
	}
	if v, ok := setup["opendataloader_timeout"].(int); ok && v > 0 {
		p.OpenDataLoaderTimeout = v
	}
	if v, ok := setup["opendataloader_timeout"].(float64); ok && v > 0 {
		p.OpenDataLoaderTimeout = int(v)
	}
	if v, ok := setup["hybrid"].(string); ok && v != "" {
		p.OpenDataLoaderHybrid = v
	}
	if v, ok := setup["image_output"].(string); ok && v != "" {
		p.OpenDataLoaderImageOutput = v
	}
	if v, ok := setup["sanitize"].(bool); ok {
		p.OpenDataLoaderSanitize = &v
	}
	if v, ok := setup["somark_base_url"].(string); ok && v != "" {
		p.SoMarkBaseURL = v
	}
	if v, ok := setup["somark_api_key"].(string); ok {
		p.SoMarkAPIKey = v
	}
	if v, ok := setup["somark_image_format"].(string); ok && v != "" {
		p.SoMarkImageFormat = v
	}
	if v, ok := setup["somark_formula_format"].(string); ok && v != "" {
		p.SoMarkFormulaFormat = v
	}
	if v, ok := setup["somark_table_format"].(string); ok && v != "" {
		p.SoMarkTableFormat = v
	}
	if v, ok := setup["somark_cs_format"].(string); ok && v != "" {
		p.SoMarkCSFormat = v
	}
	if v, ok := setup["somark_enable_text_cross_page"].(bool); ok {
		p.SoMarkEnableTextCrossPage = v
	}
	if v, ok := setup["somark_enable_table_cross_page"].(bool); ok {
		p.SoMarkEnableTableCrossPage = v
	}
	if v, ok := setup["somark_enable_title_level_recognition"].(bool); ok {
		p.SoMarkEnableTitleLevelRecognition = v
	}
	if v, ok := setup["somark_enable_inline_image"].(bool); ok {
		p.SoMarkEnableInlineImage = v
	}
	if v, ok := setup["somark_enable_table_image"].(bool); ok {
		p.SoMarkEnableTableImage = v
	}
	if v, ok := setup["somark_enable_image_understanding"].(bool); ok {
		p.SoMarkEnableImageUnderstanding = v
	}
	if v, ok := setup["somark_keep_header_footer"].(bool); ok {
		p.SoMarkKeepHeaderFooter = v
	}
	if v, ok := setup["tcadp_apiserver"].(string); ok && v != "" {
		p.TCADPAPIServer = v
	}
	if v, ok := setup["tcadp_api_key"].(string); ok {
		p.TCADPAPIKey = v
	}
	if v, ok := setup["table_result_type"].(string); ok && v != "" {
		p.TCADPTableResultType = v
	}
	if v, ok := setup["markdown_image_response_type"].(string); ok && v != "" {
		p.TCADPMarkdownImageResponseType = v
	}
	if raw, ok := setup["pages"]; ok {
		// Request-layer validation (NormalizeParserConfigPages) already
		// rejects invalid ranges at the API boundary. At parse time the input
		// should already be normalized; degrade to "parse all pages" rather
		// than failing the parse if an unexpected shape slips through.
		if pages, err := utility.NormalizePDFPages(raw); err != nil {
			slog.Warn("ConfigureFromSetup: invalid pages range, falling back to all pages",
				"raw", raw, "err", err)
		} else {
			p.Pages = pages
		}
	}
}

func normalizePDFParseMethod(raw string) string {
	method := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasSuffix(method, "@mineru"):
		return "mineru"
	case strings.HasSuffix(method, "@paddleocr"):
		return "paddleocr"
	case strings.HasSuffix(method, "@somark"):
		return "somark"
	case strings.HasSuffix(method, "@opendataloader"):
		return "opendataloader"
	}
	switch method {
	case "plaintext":
		return "plain_text"
	case "tcadp parser":
		return "tcadp"
	}
	return method
}

func (p *PDFParser) validateParseMethod() error {
	method := normalizePDFParseMethod(p.ParseMethod)
	if _, ok := supportedPDFParseMethods[method]; ok {
		return nil
	}
	return fmt.Errorf("parser: unsupported PDF parse_method %q (Go currently supports: deepdoc, plain_text, mineru, paddleocr, docling, opendataloader, somark, tcadp; tenant-resolved custom IMAGE2TEXT/VLM model names are not supported in the Go parser layer)", p.ParseMethod)
}

func emptyPDFResult(filename string) ParseResult {
	return ParseResult{
		OutputFormat: "json",
		File: map[string]any{
			"name":       filename,
			"page_count": 0,
			"outline":    []map[string]any{},
		},
		JSON: []map[string]any{{"text": "", "doc_type_kwd": "text"}},
	}
}

// deepDocAnalyzerFromEnv resolves the configured DeepDoc analyzer. It is a thin
// wrapper over resolveDocAnalyzer that feeds the registered in-process factory
// (production is in-process only). The factory itself is owned by the
// dependency-free deepdoctype package so the native backend can register
// without the parser importing onnxruntime.
func deepDocAnalyzerFromEnv() (deepdoctype.DocAnalyzer, error) {
	return resolveDocAnalyzer(deepdoctype.NativeDocAnalyzerFactory)
}

// resolveDocAnalyzer applies the DeepDoc backend policy:
//   - the in-process factory is the ONLY production backend; it is used
//     directly when registered (serving),
//   - if it is unavailable it returns an error so parsing fails loudly
//     instead of silently producing empty layout/table/OCR results.
//
// The external Python HTTP service (formerly selected via DEEPDOC_URL) has
// been removed entirely from both the production path and the test suite, so
// production is in-process only.
//
// It takes its inputs explicitly (the factory) rather than reading
// globals, so the policy is unit-testable in isolation. It never returns a
// mock: if no backend is available it returns an error (MockDocAnalyzer is
// test-only infrastructure and must never sit in this production path).
func resolveDocAnalyzer(factory func() (deepdoctype.DocAnalyzer, bool)) (deepdoctype.DocAnalyzer, error) {
	if factory != nil {
		if a, ok := factory(); ok {
			return a, nil
		}
	}
	return nil, fmt.Errorf("deepdoc: no in-process DeepDoc backend available: build with -tags cgo and provide ORT + models")
}

// GetDocAnalyzer returns the configured in-process DeepDoc analyzer. It is the
// single production entry point now that the external HTTP service is no longer
// a backend. Callers outside the parser package (e.g. standalone image OCR in
// the ingestion component) use this instead of constructing a client.
func GetDocAnalyzer() (deepdoctype.DocAnalyzer, error) {
	return deepDocAnalyzerFromEnv()
}

func pdfParseResultToJSON(filename string, parsed *deepdoctype.ParseResult) ParseResult {
	return pdfParseResultToJSONWithOptions(filename, parsed, pdfPostProcessOptions{})
}

func pdfParseResultToJSONWithOptions(filename string, parsed *deepdoctype.ParseResult, opts pdfPostProcessOptions) ParseResult {
	if parsed == nil {
		return ParseResult{Err: fmt.Errorf("parser: nil DeepDOC PDF result for %s", filename)}
	}
	processed := *parsed
	processed.Sections = append([]deepdoctype.Section(nil), parsed.Sections...)
	processed.Outlines = append([]deepdoctype.Outline(nil), parsed.Outlines...)
	if opts.enableMultiColumn && opts.pageWidth <= 0 {
		opts.pageWidth = firstPDFPageWidth(processed.PageWidth)
	}
	applyPDFPostProcess(&processed, opts)
	defer processed.Close()
	cropMediaSections(&processed)

	items := pdflayout.SectionsToJSON(processed.Sections)
	if len(items) == 0 {
		items = []map[string]any{{"text": "", "doc_type_kwd": "text"}}
	}
	for i := range items {
		if layoutType, _ := items[i]["layout_type"].(string); layoutType != "" {
			items[i]["layout"] = layoutType
		}
		if normalized := normalizePDFPositions(items[i]["_pdf_positions"]); len(normalized) > 0 {
			items[i]["_pdf_positions"] = normalized
			items[i]["positions"] = normalized
			if _, ok := items[i]["page_number"]; !ok {
				items[i]["page_number"] = firstPageNumber(normalized)
			}
		}
		normalizePDFDocType(items[i])
		if img, _ := items[i]["image"].(string); img != "" {
			items[i]["image"] = pdflayout.InlinePNGDataURL(img)
		}
	}
	return ParseResult{
		OutputFormat: "json",
		File: map[string]any{
			"name":       filename,
			"page_count": len(processed.PageHeight),
			"outline":    outlinesToFileMeta(processed.Outlines),
		},
		JSON: items,
	}
}

func pdfParseResultToMarkdownWithOptions(filename string, parsed *deepdoctype.ParseResult, opts pdfPostProcessOptions) ParseResult {
	if parsed == nil {
		return ParseResult{Err: fmt.Errorf("parser: nil DeepDOC PDF result for %s", filename)}
	}
	processed := *parsed
	processed.Sections = append([]deepdoctype.Section(nil), parsed.Sections...)
	processed.Outlines = append([]deepdoctype.Outline(nil), parsed.Outlines...)
	if opts.enableMultiColumn && opts.pageWidth <= 0 {
		opts.pageWidth = firstPDFPageWidth(processed.PageWidth)
	}
	applyPDFPostProcess(&processed, opts)
	defer processed.Close()
	cropMediaSections(&processed)

	return ParseResult{
		OutputFormat: "markdown",
		File: map[string]any{
			"name":       filename,
			"page_count": len(processed.PageHeight),
			"outline":    outlinesToFileMeta(processed.Outlines),
		},
		Markdown: sectionsToMarkdown(processed.Sections),
	}
}

func outlinesToFileMeta(outlines []deepdoctype.Outline) []map[string]any {
	if len(outlines) == 0 {
		return []map[string]any{}
	}
	result := make([]map[string]any, 0, len(outlines))
	for _, o := range outlines {
		result = append(result, map[string]any{
			"title":       o.Title,
			"level":       o.Level,
			"page_number": o.PageNumber,
		})
	}
	return result
}

func firstPageNumber(raw any) int {
	positions, ok := raw.([][]any)
	if !ok || len(positions) == 0 || len(positions[0]) == 0 {
		return 0
	}
	switch v := positions[0][0].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func sectionsToMarkdown(sections []deepdoctype.Section) string {
	return pdflayout.SectionsToMarkdown(sections)
}

// cropMediaSections crops figure and table sections from rendered PDF page
// images while the engine is still alive, populating sec.Image.
// It is used by both the JSON and Markdown serialization paths.
func cropMediaSections(result *deepdoctype.ParseResult) {
	engine := result.Engine
	if engine == nil || len(result.PageHeight) == 0 {
		return
	}
	// Render each page at most once across all media sections. A PDF can have many
	// figures/tables on the same page, so a cache shared across the section loop
	// avoids re-rendering the page for every section.
	//
	// result.Sections is ordered by page, so once we advance to a section whose
	// minimum page is P, no later section references a page < P. We therefore
	// keep only a sliding window of page images: pages strictly below the
	// current section's minimum page are evicted. This bounds memory to the
	// current section's page span (typically one page, a few at most for a
	// cross-page section) instead of caching the whole PDF.
	pageCache := make(map[int]image.Image)
	var lastMinPage = -1
	sectionsOrdered := true
	renderPage := func(pn int) image.Image {
		if img, ok := pageCache[pn]; ok {
			return img
		}
		img, err := deepdocpdf.RenderPageToImage(engine, pn)
		if err != nil || img == nil {
			slog.Warn("cropMediaSections: render failed, skipping section",
				"page", pn, "err", err)
			pageCache[pn] = nil
			return nil
		}
		pageCache[pn] = img
		return img
	}

	for i := range result.Sections {
		sec := &result.Sections[i]
		if strings.TrimSpace(sec.Image) != "" {
			continue
		}
		if sec.LayoutType != deepdoctype.LayoutTypeFigure &&
			sec.LayoutType != deepdoctype.LayoutTypeTable &&
			strings.TrimSpace(sec.LayoutType) != "image" &&
			sec.DocTypeKwd != "image" && sec.DocTypeKwd != "table" {
			continue
		}
		if len(sec.Positions) == 0 {
			continue
		}
		// Minimum page this section touches; used both to prune stale cache
		// entries and to bound the window.
		minPage := -1
		pages := make(map[int]struct{})
		for _, pos := range sec.Positions {
			for _, pn := range pos.PageNumbers {
				pages[pn] = struct{}{}
				if pn < minPage || minPage < 0 {
					minPage = pn
				}
			}
		}
		if lastMinPage >= 0 && minPage < lastMinPage {
			if sectionsOrdered {
				slog.Warn("cropMediaSections: sections out of page order; disabling page cache eviction",
					"min_page", minPage, "last_min_page", lastMinPage)
			}
			sectionsOrdered = false
		}
		lastMinPage = minPage
		// Evict page images that no later section can reference (all future
		// sections start at page >= minPage).
		if sectionsOrdered {
			for pn := range pageCache {
				if pn < minPage {
					delete(pageCache, pn)
				}
			}
		}
		// Collect every distinct page this section spans so CropSectionByDLA
		// can crop and vertically concatenate each page (mirroring Python's
		// cropout multi-page branch). Single-page sections still render exactly
		// one page, and the pageCache above guarantees a page is rendered at
		// most once even when several sections share it.
		single := make(map[int]image.Image, len(pages))
		for pn := range pages {
			if img := renderPage(pn); img != nil {
				single[pn] = img
			}
		}
		if len(single) == 0 {
			continue
		}
		if sec.LayoutType == deepdoctype.LayoutTypeFigure {
			if dla := util.CropSectionByDLA(*sec, result.DLARegions, single); dla != "" {
				sec.Image = dla
				continue
			}
		}
		if len(sec.Positions) > 0 {
			if img := util.CropSectionPositions(sec.Positions, single, deepdoctype.DlaScale); img != "" {
				sec.Image = img
				continue
			}
		}
		sec.Image = util.CropSectionImage(sec.PositionTag, single, deepdoctype.DlaScale)
	}
}

// firstPDFPageWidth returns the first page's width from a map of
// per-page PDF-point widths. The map is guaranteed to store values
// already converted to PDF-point space by processPage.
func firstPDFPageWidth(pageWidths map[int]float64) float64 {
	if len(pageWidths) == 0 {
		return 0
	}
	pages := make([]int, 0, len(pageWidths))
	for pg := range pageWidths {
		pages = append(pages, pg)
	}
	sort.Ints(pages)
	pg := pages[0]
	return pageWidths[pg]
}

func normalizePDFPositions(raw any) [][]any {
	positions, ok := raw.([][]any)
	if !ok || len(positions) == 0 {
		return nil
	}
	normalized := make([][]any, 0, len(positions))
	for _, pos := range positions {
		if len(pos) < 5 {
			continue
		}
		pageNumber, ok := normalizePDFPageNumber(pos[0])
		if !ok {
			continue
		}
		left, lok := numericAny(pos[1])
		right, rok := numericAny(pos[2])
		top, tok := numericAny(pos[3])
		bottom, bok := numericAny(pos[4])
		if !lok || !rok || !tok || !bok {
			continue
		}
		normalized = append(normalized, []any{pageNumber, left, right, top, bottom})
	}
	return normalized
}

// normalizePDFPageNumber converts a DeepDoc 0-indexed page number to the
// 1-indexed form stored in _pdf_positions / positions. It is the SINGLE
// 0→1 conversion point: DeepDoc (pdf_oxide/pdfium) emits 0-indexed pages,
// and every downstream consumer (AddPositions for ES storage,
// PositionsFromMatrix for the PDFium render path) expects 1-indexed input.
// Adding +1 unconditionally — instead of only for v<=0 — keeps all pages
// consistent; the old heuristic left page>=1 unconverted, which AddPositions
// then double-incremented and PositionsFromMatrix mis-decremented.
func normalizePDFPageNumber(raw any) (int, bool) {
	switch v := raw.(type) {
	case int:
		return v + 1, true
	case int64:
		return normalizePDFPageNumber(int(v))
	case float64:
		return normalizePDFPageNumber(int(v))
	case []any:
		if len(v) == 0 {
			return 0, false
		}
		return normalizePDFPageNumber(v[len(v)-1])
	case []int:
		if len(v) == 0 {
			return 0, false
		}
		return normalizePDFPageNumber(v[len(v)-1])
	default:
		return 0, false
	}
}

func numericAny(raw any) (float64, bool) {
	switch v := raw.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}

func normalizePDFDocType(item map[string]any) {
	if item == nil {
		return
	}
	if docType, _ := item["doc_type_kwd"].(string); docType != "" {
		return
	}
	layoutType, _ := item["layout_type"].(string)
	switch layoutType {
	case "table":
		item["doc_type_kwd"] = "table"
	case "figure", "image":
		item["doc_type_kwd"] = "image"
	default:
		if img, _ := item["image"].(string); img != "" {
			item["doc_type_kwd"] = "image"
			return
		}
		item["doc_type_kwd"] = "text"
	}
}

func parsePDFWithDeepDoc(ctx context.Context, filename string, data []byte, parseFn func(context.Context, []byte, deepdoctype.DocAnalyzer) (*deepdoctype.ParseResult, error)) ParseResult {
	return parsePDFWithDeepDocOptions(ctx, filename, data, pdfPostProcessOptions{}, parseFn)
}

func parsePDFWithDeepDocOptions(ctx context.Context, filename string, data []byte, opts pdfPostProcessOptions, parseFn func(context.Context, []byte, deepdoctype.DocAnalyzer) (*deepdoctype.ParseResult, error)) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	analyzer, aerr := deepDocAnalyzerFromEnv()
	if aerr != nil {
		return ParseResult{Err: aerr}
	}
	parsed, err := parseFn(ctx, data, analyzer)
	if err != nil {
		return ParseResult{Err: err}
	}
	var res ParseResult
	switch strings.ToLower(strings.TrimSpace(opts.outputFormat)) {
	case "", "json":
		res = pdfParseResultToJSONWithOptions(filename, parsed, opts)
	case "markdown":
		res = pdfParseResultToMarkdownWithOptions(filename, parsed, opts)
	default:
		return ParseResult{Err: fmt.Errorf("parser: unsupported PDF output_format %q", opts.outputFormat)}
	}
	for i := range res.JSON {
		if img, _ := res.JSON[i]["image"].(string); img != "" {
			res.JSON[i]["image"] = pdflayout.InlinePNGDataURL(img)
		}
	}
	return res
}
