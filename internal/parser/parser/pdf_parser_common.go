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
	"os"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/common"
	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	pdflayout "ragflow/internal/deepdoc/parser/pdf/layout"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/utility"
)

// ErrPDFEngineUnavailable is returned by PDFParser.ParseWithResult
// when the current build cannot construct the DeepDOC PDF backend.
// The normal reason is a non-cgo build, because the pdfoxide bridge
// is compiled behind `//go:build cgo`.
var ErrPDFEngineUnavailable = errors.New("parser: PDF backend unavailable in this build")

// supportedPDFParseMethods is the set of canonical tokens PDFParser can
// execute; see pdfParseMethodSpellings for the accepted spellings and
// TestPDFParseMethodTablesAgree for dispatcher-handled methods.
var supportedPDFParseMethods = map[string]struct{}{
	"":               {},
	"deepdoc":        {},
	"plain_text":     {},
	"mineru":         {},
	"docling":        {},
	"opendataloader": {},
	"somark":         {},
	"tcadp":          {},
}

type PDFParser struct {
	ParserType string // DeepDoc, MinerU
	Model      string // DeepDoc@buildin@ragflow
	LibType    string // pdf_oxide, used by DeepDoc

	FlattenMediaToText bool
	RemoveTOC          bool
	RemoveHeaderFooter bool
	EnableMultiColumn  bool
	ParseMethod        string
	// UseTenantModelConfig disables global endpoint/credential/option fallback
	// for explicitly resolved SoMark/OpenDataLoader instances. Runtime-only;
	// ConfigureFromSetup does not accept this flag from persisted settings.
	UseTenantModelConfig bool
	// Pages restricts parsing to these 1-indexed inclusive page ranges.
	// nil/empty means parse all pages. Populated by ConfigureFromSetup from
	// the filetype setup map. DeepDOC reads it through its ParserConfig, and
	// plain_text skips the pages outside it.
	Pages [][]int
	// OnPageDone, when set, is forwarded to the deepdoc ParserConfig so the
	// caller observes per-page parse progress (done/total). Only the deepdoc
	// backend invokes it; remote engines poll opaque HTTP jobs and never call
	// it, so the fraction simply stays where it was.
	OnPageDone                        func(done, total int)
	MinerUAPIServer                   string
	MinerUAPIKey                      string
	MinerUBackend                     string
	MinerUServerURL                   string
	MinerUPollTimeout                 time.Duration
	DoclingServerURL                  string
	DoclingAPIKey                     string
	DoclingDoOCR                      *bool
	DoclingPDFBackend                 string
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
		MinerUBackend:                  "",
		MinerUPollTimeout:              minerUPollTimeout,
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
	if v, ok := setup["mineru_server_url"].(string); ok && v != "" {
		p.MinerUServerURL = v
	}
	if v, ok := setup["mineru_timeout_seconds"].(int); ok && v > 0 {
		p.MinerUPollTimeout = time.Duration(v) * time.Second
	}
	if v, ok := setup["mineru_timeout_seconds"].(float64); ok && v > 0 {
		p.MinerUPollTimeout = time.Duration(v * float64(time.Second))
	}
	if v, ok := setup["docling_server_url"].(string); ok && v != "" {
		p.DoclingServerURL = v
	}
	if v, ok := setup["docling_api_key"].(string); ok {
		p.DoclingAPIKey = v
	}
	if v, ok := setup["docling_do_ocr"].(bool); ok {
		p.DoclingDoOCR = &v
	}
	if v, ok := setup["docling_pdf_backend"].(string); ok && v != "" {
		p.DoclingPDFBackend = v
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
			common.Warn("ConfigureFromSetup: invalid pages range, falling back to all pages",
				zap.Any("raw", raw), zap.Error(err))
		} else {
			p.Pages = pages
		}
	}
}

// pdfParseMethodSpellings is the single vocabulary of PDF parse methods:
// every accepted spelling (lower-cased and trimmed) mapped to its canonical
// token. Consumers must not keep their own copies of this set — tell a parse
// method apart from a VLM model selector via IsPDFParseMethod, and resolve
// its canonical token via normalizePDFParseMethod.
//
// "plaintext" / "plain text" are the spellings the dataset configuration UI
// persists for its plain-text option (ParseDocumentType.PlainText), so they
// are parse methods rather than model names.
var pdfParseMethodSpellings = map[string]string{
	"deepdoc":        "deepdoc",
	"plain_text":     "plain_text",
	"plaintext":      "plain_text",
	"plain text":     "plain_text",
	"mineru":         "mineru",
	"monkeyocrv2":    "monkeyocrv2",
	"docling":        "docling",
	"opendataloader": "opendataloader",
	"tcadp parser":   "tcadp",
	"paddleocr":      "paddleocr",
	"somark":         "somark",
}

// IsPDFParseMethod reports whether raw names a PDF parse method rather than
// a VLM model selector. "@"-suffixed spellings such as "foo@mineru" are
// layout_recognizer selectors resolved separately, so they report false.
func IsPDFParseMethod(raw string) bool {
	_, ok := pdfParseMethodSpellings[strings.ToLower(strings.TrimSpace(raw))]
	return ok
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
	if canonical, ok := pdfParseMethodSpellings[method]; ok {
		return canonical
	}
	return method
}

func (p *PDFParser) validateParseMethod() error {
	method := normalizePDFParseMethod(p.ParseMethod)
	// A method backed by a registered external analyzer factory (see
	// RegisterAnalyzerFactory) is a supported method even though it is not in
	// the built-in whitelist — it is routed to the DeepDoc dispatch path, which
	// selects the external factory. Without this, a registered method would be
	// rejected here before dispatch ever ran.
	if _, ok := analyzerFactories[method]; ok {
		return nil
	}
	if _, ok := supportedPDFParseMethods[method]; ok {
		return nil
	}
	return fmt.Errorf("parser: unsupported PDF parse_method %q (standalone PDFParser supports: deepdoc, plain_text, mineru, docling, opendataloader, somark, tcadp; methods backed by a registered external analyzer factory are supported; tenant-resolved custom IMAGE2TEXT/VLM model names are not supported in the parser layer)", p.ParseMethod)
}

// ParseWithResult parses a PDF into a ParseResult, dispatching on the
// configured ParseMethod. The native extraction — the deepdoc default and the
// plain_text strategy — lives in the internal/deepdoc/parser/pdf engine; this
// method is the facade that wires strategies and post-processing. It is
// build-tag agnostic: under !cgo the deepdoc path transparently reports
// ErrPDFEngineUnavailable while plain_text falls back to the pure-Go engine.
func (p *PDFParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	result := p.parseWithResult(ctx, filename, data)
	if result.Err == nil && result.OutputFormat == "json" {
		return p.NormalizeResult(ctx, filename, result)
	}
	return result
}

func (p *PDFParser) parseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	if err := p.validateParseMethod(); err != nil {
		return ParseResult{Err: err}
	}
	common.Info(fmt.Sprintf("------------file: %s, parse_method: %s", filename, p.ParseMethod))
	switch normalizePDFParseMethod(p.ParseMethod) {
	case "plain_text":
		if len(data) == 0 {
			return emptyPDFResult(filename)
		}
		items, pageCount, err := deepdocpdf.PlainText(data, p.Pages)
		if err != nil {
			return ParseResult{Err: fmt.Errorf("parser: plain_text: %w", err)}
		}
		return pdfItemsToResult(filename, items, pageCount)
	case "mineru":
		return parsePDFWithMinerU(ctx, filename, data, p)
	case "docling":
		return parsePDFWithDocling(ctx, filename, data, p)
	case "opendataloader":
		return parsePDFWithOpenDataLoader(ctx, filename, data, p)
	case "somark":
		return parsePDFWithSoMark(filename, data, p)
	case "tcadp":
		return parsePDFWithTCADP(ctx, filename, data, p)
	}
	cfg := deepdoctype.DefaultParserConfig()
	cfg.Pages = p.Pages
	cfg.RemoveTOC = p.RemoveTOC
	cfg.RemoveHeaderFooter = p.RemoveHeaderFooter
	cfg.OnPageDone = p.OnPageDone
	res := parsePDFWithDeepDocOptions(ctx, filename, data, pdfPostProcessOptions{
		zoom:               cfg.Zoom,
		enableMultiColumn:  p.EnableMultiColumn,
		flattenMediaToText: p.FlattenMediaToText,
		parseMethod:        p.ParseMethod,
	}, deepdocpdf.NewParser(cfg).Parse)
	if res.Err != nil {
		if errors.Is(res.Err, deepdocpdf.ErrNoPDFData) ||
			strings.Contains(res.Err.Error(), "deepdoc/pdf: cgo required") ||
			strings.Contains(res.Err.Error(), "no in-process DeepDoc backend") {
			return ParseResult{Err: fmt.Errorf("%w: %s", ErrPDFEngineUnavailable, filename)}
		}
	}
	return res
}

func emptyPDFResult(filename string) ParseResult {
	return ParseResult{
		OutputFormat: "json",
		File: map[string]any{
			"name":       filename,
			"page_count": 0,
			"outline":    []map[string]any{},
		},
		JSON: []DocItem{NewTextJSONItem("")},
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

// analyzerFactories maps a canonical PDF parse method to a factory that builds
// an out-of-process DocAnalyzer from a base URL. It is empty by default, so the
// OSS build stays in-process only (see resolveDocAnalyzer). A deployment that
// serves a parse method over an external inference service registers a factory
// for it via RegisterAnalyzerFactory; resolveDocAnalyzerForMethod then routes
// that method to the factory, reading the endpoint from the <METHOD>_URL
// environment variable — a generic convention, so no per-service name is baked
// into this package.
//
// Registration is expected at process start-up (init), before parsing begins;
// the map is only read while parsing, so concurrent resolution is safe.
var analyzerFactories = map[string]func(baseURL string) (deepdoctype.DocAnalyzer, error){}

// RegisterAnalyzerFactory registers an external DocAnalyzer backend for a
// canonical parse method. The method is normalized (aliases collapse to the
// canonical token) so registration and resolution agree; registering the same
// method again replaces the previous factory. It mirrors
// deepdoctype.SetNativeDocAnalyzerFactory, but for out-of-process backends
// selected per parse method.
//
// Reserved registrations are rejected (panic):
//   - a built-in method that ParseWithResult dispatches to an internal engine
//     (every switch case other than the fall-through to DeepDoc) — the switch
//     intercepts it before resolution runs, so the factory would never be used;
//   - "paddleocr" — this method requires tenant model dispatch;
//   - the empty (default) method — routing it externally would hijack every
//     caller that omits parse_method and divert the default in-process path.
//
// The only built-in that may be overridden externally is "deepdoc", which falls
// through to the DeepDoc path and is the DEEPDOC_URL convention. Any
// non-built-in custom method name is also accepted. Registration is expected at
// process start-up (init); the map is only read while parsing.
func RegisterAnalyzerFactory(method string, factory func(baseURL string) (deepdoctype.DocAnalyzer, error)) {
	m := normalizePDFParseMethod(method)
	if m == "" {
		panic("parser: RegisterAnalyzerFactory: cannot register an external analyzer for the empty (default) parse method; use an explicit method name")
	}
	if m != "deepdoc" {
		if _, builtin := supportedPDFParseMethods[m]; builtin || m == "paddleocr" {
			panic(fmt.Sprintf("parser: RegisterAnalyzerFactory: cannot register an external analyzer for reserved method %q; it requires a built-in parser or tenant model dispatch", m))
		}
	}
	analyzerFactories[m] = factory
}

// externalAnalyzerURL derives the endpoint for a registered external method
// from the generic <METHOD>_URL environment convention (upper-cased canonical
// token). The value is trimmed and otherwise handed to the factory verbatim —
// the dispatcher never interprets it as a path.
func externalAnalyzerURL(method string) string {
	return strings.TrimSpace(os.Getenv(strings.ToUpper(method) + "_URL"))
}

// resolveDocAnalyzerForMethod selects the DeepDoc backend for a parse method:
//   - a method with a registered external factory (RegisterAnalyzerFactory)
//     routes to that factory with the <METHOD>_URL endpoint, failing loudly
//     when the URL is unset rather than silently producing empty results;
//   - every other method (including "") uses the in-process native factory,
//     preserving the historical in-process-only production path.
//
// It takes the in-process factory explicitly (like resolveDocAnalyzer) so the
// policy is unit-testable without ONNX Runtime, and it never returns a mock.
func resolveDocAnalyzerForMethod(method string, nativeFactory func() (deepdoctype.DocAnalyzer, bool)) (deepdoctype.DocAnalyzer, error) {
	m := normalizePDFParseMethod(method)
	if factory, ok := analyzerFactories[m]; ok {
		url := externalAnalyzerURL(m)
		if url == "" {
			return nil, fmt.Errorf("parser: parse method %q is backed by an external analyzer but %s_URL is not set", m, strings.ToUpper(m))
		}
		return factory(url)
	}
	return resolveDocAnalyzer(nativeFactory)
}

// GetDocAnalyzer returns the configured in-process DeepDoc analyzer. It is the
// in-process entry point used by PDF (deepdoc method) and non-PDF image parsers.
// External, out-of-process backends are now available too, but only per parse
// method via RegisterAnalyzerFactory (resolved in resolveDocAnalyzerForMethod);
// GetDocAnalyzer itself stays in-process only and is never the external path.
func GetDocAnalyzer() (deepdoctype.DocAnalyzer, error) {
	return deepDocAnalyzerFromEnv()
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
	// JSON media policy is applied once at the PDFParser result boundary.
	opts.flattenMediaToText = false
	applyPDFPostProcess(&processed, opts)
	defer processed.Close()
	// NOTE: PDF media (figure/table) is intentionally NOT cropped here under
	// cgo. The parser no longer inlines base64 images — that is what bounded
	// the parser-phase memory peak. Image/table sections keep only their PDF
	// positions; the chunker re-acquires the source PDF and crops on demand at
	// index time, and the VLM path crops on demand when it needs to describe a
	// figure/table.

	items := pdflayout.SectionsToJSON(processed.Sections)
	if len(items) == 0 {
		items = []map[string]any{{"text": "", "doc_type_kwd": "text"}}
	}
	for i := range items {
		if layoutType, _ := items[i]["layout_type"].(string); layoutType != "" {
			items[i]["layout"] = layoutType
		}
		if normalized := normalizeDeepDocPositions(items[i]["_pdf_positions"]); len(normalized) > 0 {
			items[i]["_pdf_positions"] = normalized
			items[i]["positions"] = normalized
			if _, ok := items[i]["page_number"]; !ok {
				items[i]["page_number"] = firstPageNumber(normalized)
			}
		}
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
		JSON: MapsToItems(items),
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

// normalizeDeepDocPositions converts DeepDoc source positions, including its
// zero-based pages. Other engines must not pass canonical positions here.
func normalizeDeepDocPositions(raw any) [][]any {
	positions, ok := raw.([][]any)
	if !ok || len(positions) == 0 {
		return nil
	}
	normalized := make([][]any, 0, len(positions))
	for _, pos := range positions {
		if len(pos) < 5 {
			continue
		}
		pageNumber, ok := normalizeDeepDocPageNumber(pos[0])
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

// normalizeDeepDocPageNumber converts a DeepDoc 0-indexed page number to the
// 1-indexed form stored in _pdf_positions / positions. It is the SINGLE
// 0→1 conversion point: DeepDoc (pdf_oxide/pdfium) emits 0-indexed pages,
// and every downstream consumer (addPDFPositions for ES storage,
// PositionsFromMatrix for the PDFium render path) expects 1-indexed input.
// Adding +1 unconditionally — instead of only for v<=0 — keeps all pages
// consistent; the old heuristic left page>=1 unconverted, which addPDFPositions
// then double-incremented and PositionsFromMatrix mis-decremented.
func normalizeDeepDocPageNumber(raw any) (int, bool) {
	switch v := raw.(type) {
	case int:
		return v + 1, true
	case int64:
		return normalizeDeepDocPageNumber(int(v))
	case float64:
		return normalizeDeepDocPageNumber(int(v))
	case []any:
		if len(v) == 0 {
			return 0, false
		}
		return normalizeDeepDocPageNumber(v[len(v)-1])
	case []int:
		if len(v) == 0 {
			return 0, false
		}
		return normalizeDeepDocPageNumber(v[len(v)-1])
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

// ExtractPDFPositions is the single source of truth for "does this parsed item
// carry PDF positions". It returns the position rows (under the canonical
// _pdf_positions key or the legacy positions key), decoded through
// PDFPositionRows so every wire shape is understood. The bool reports whether
// any row was present.
//
// The parser (doc-type classification in normalizePDFDocType) and the on-demand
// croppers (VLM vision_enhancement and the chunker) MUST agree on this
// contract, so all call sites delegate here instead of re-implementing the
// check. A loose "non-nil" test previously misclassified items whose positions
// were empty or not actually a matrix (e.g. an empty []any or a stray scalar),
// which escaped the figure-caption → image normalization and the crop path.
func ExtractPDFPositions(item map[string]any) ([][]any, bool) {
	for _, key := range []string{"_pdf_positions", "positions"} {
		if rows := PDFPositionRows(item[key]); len(rows) > 0 {
			return rows, true
		}
	}
	return nil, false
}

func parsePDFWithDeepDocOptions(ctx context.Context, filename string, data []byte, opts pdfPostProcessOptions, parseFn func(context.Context, []byte, deepdoctype.DocAnalyzer) (*deepdoctype.ParseResult, error)) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	analyzer, aerr := resolveDocAnalyzerForMethod(opts.parseMethod, deepdoctype.NativeDocAnalyzerFactory)
	if aerr != nil {
		return ParseResult{Err: aerr}
	}
	parsed, err := parseFn(ctx, data, analyzer)
	if err != nil {
		return ParseResult{Err: err}
	}
	res := pdfParseResultToJSONWithOptions(filename, parsed, opts)
	for i := range res.JSON {
		m := res.JSON[i].ToMap()
		if img, _ := m["image"].(string); img != "" {
			m["image"] = pdflayout.InlinePNGDataURL(img)
			res.JSON[i] = DocItemFromMap(m)
		}
	}
	return res
}
