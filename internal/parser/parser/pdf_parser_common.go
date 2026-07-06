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
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"os"
	"sort"
	"strings"
	"time"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	"ragflow/internal/deepdoc/parser/pdf/inference"
	pdflayout "ragflow/internal/deepdoc/parser/pdf/layout"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
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

	FlattenMediaToText                bool
	RemoveTOC                         bool
	RemoveHeaderFooter                bool
	EnableMultiColumn                 bool
	OutputFormat                      string
	ParseMethod                       string
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
		SoMarkBaseURL:                  "https://somark.tech/api/v1",
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

func deepDocAnalyzerFromEnv() deepdoctype.DocAnalyzer {
	baseURL := strings.TrimSpace(os.Getenv("DEEPDOC_URL"))
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("OSSDEEPDOC_URL"))
	}
	if baseURL == "" {
		return &deepdocpdf.MockDocAnalyzer{Healthy: true}
	}
	client, err := inference.NewClient(baseURL)
	if err != nil {
		return &deepdocpdf.MockDocAnalyzer{Healthy: true}
	}
	if !client.Health() {
		return &deepdocpdf.MockDocAnalyzer{Healthy: true}
	}
	return client
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
		opts.pageWidth = firstPDFPageWidth(processed.PageImages, opts.zoom)
	}
	applyPDFPostProcess(&processed, opts)

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
			items[i]["image"] = "data:image/png;base64," + img
		}
	}
	return ParseResult{
		OutputFormat: "json",
		File: map[string]any{
			"name":       filename,
			"page_count": len(processed.PageImages),
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
		opts.pageWidth = firstPDFPageWidth(processed.PageImages, opts.zoom)
	}
	applyPDFPostProcess(&processed, opts)

	return ParseResult{
		OutputFormat: "markdown",
		File: map[string]any{
			"name":       filename,
			"page_count": len(processed.PageImages),
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

func inlinePNGDataURL(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "data:image/") {
		return raw
	}
	if _, err := base64.StdEncoding.DecodeString(raw); err != nil {
		return raw
	}
	return "data:image/png;base64," + raw
}

func sectionsToMarkdown(sections []deepdoctype.Section) string {
	var b strings.Builder
	for _, section := range sections {
		layoutType := strings.TrimSpace(section.LayoutType)
		if layoutType == deepdoctype.LayoutTypeTitle {
			b.WriteString("\n## ")
		}
		if layoutType == deepdoctype.LayoutTypeFigure && section.Image != "" {
			b.WriteString("\n![Image](")
			b.WriteString(inlinePNGDataURL(section.Image))
			b.WriteString(")")
			continue
		}
		b.WriteString(section.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

func firstPDFPageWidth(pageImages map[int]image.Image, zoom float64) float64 {
	if len(pageImages) == 0 {
		return 0
	}
	if zoom <= 0 {
		zoom = deepdoctype.DefaultParserConfig().Zoom
	}
	pages := make([]int, 0, len(pageImages))
	for page := range pageImages {
		pages = append(pages, page)
	}
	sort.Ints(pages)
	img := pageImages[pages[0]]
	if img == nil {
		return 0
	}
	return float64(img.Bounds().Dx()) / zoom
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

func normalizePDFPageNumber(raw any) (int, bool) {
	switch v := raw.(type) {
	case int:
		if v <= 0 {
			return v + 1, true
		}
		return v, true
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
	parsed, err := parseFn(ctx, data, deepDocAnalyzerFromEnv())
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
			res.JSON[i]["image"] = inlinePNGDataURL(img)
		}
	}
	return res
}
