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
	"os"
	"strings"

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

type PDFParser struct {
	ParserType string // DeepDoc, PaddleOCR, MinerU
	Model      string // DeepDoc@buildin@ragflow
	LibType    string // pdf_oxide, used by DeepDoc
}

func NewPDFParser() *PDFParser {
	return &PDFParser{
		ParserType: "DeepDoc",
		Model:      "DeepDoc@buildin@ragflow",
		LibType:    "pdf_oxide",
	}
}

func (p *PDFParser) String() string {
	return "PDFParser"
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
	if parsed == nil {
		return ParseResult{Err: fmt.Errorf("parser: nil DeepDOC PDF result for %s", filename)}
	}
	items := pdflayout.SectionsToJSON(parsed.Sections)
	if len(items) == 0 {
		items = []map[string]any{{"text": "", "doc_type_kwd": "text"}}
	}
	for i := range items {
		if layoutType, _ := items[i]["layout_type"].(string); layoutType != "" {
			items[i]["layout"] = layoutType
		}
		if _, ok := items[i]["page_number"]; !ok {
			items[i]["page_number"] = firstPageNumber(items[i]["_pdf_positions"])
		}
		if img, _ := items[i]["image"].(string); img != "" {
			items[i]["image"] = "data:image/png;base64," + img
		}
	}
	return ParseResult{
		OutputFormat: "json",
		File: map[string]any{
			"name":       filename,
			"page_count": len(parsed.PageImages),
			"outline":    outlinesToFileMeta(parsed.Outlines),
		},
		JSON: items,
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
	pages, ok := positions[0][0].([]any)
	if !ok || len(pages) == 0 {
		return 0
	}
	switch v := pages[0].(type) {
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

func parsePDFWithDeepDoc(ctx context.Context, filename string, data []byte, parseFn func(context.Context, []byte, deepdoctype.DocAnalyzer) (*deepdoctype.ParseResult, error)) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	parsed, err := parseFn(ctx, data, deepDocAnalyzerFromEnv())
	if err != nil {
		return ParseResult{Err: err}
	}
	res := pdfParseResultToJSON(filename, parsed)
	for i := range res.JSON {
		if img, _ := res.JSON[i]["image"].(string); img != "" {
			res.JSON[i]["image"] = inlinePNGDataURL(img)
		}
	}
	return res
}
