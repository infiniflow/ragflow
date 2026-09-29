//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parser

import (
	"context"
	"errors"
	"fmt"
	"strings"

	deepdocoffice "ragflow/internal/deepdoc/parser/office"
)

// pptxExtract is a test seam for the native office_oxide extraction. It
// defaults to the deepdoc/docx backend and is overridden in tests to capture
// the effective container format passed to the engine.
var pptxExtract = deepdocoffice.OpenAndExtract

// PPTXParser parses both .pptx (OOXML) and .ppt (OLE binary)
// files via the office_oxide backend. The format field controls
// the container format passed to OpenFromBytes — "pptx" for
// ZIP-based OOXML presentations and "ppt" for the legacy binary
// OLE format.
type PPTXParser struct {
	format string

	// TCADP cloud-parsing configuration
	ParseMethod                    string
	TCADPAPIServer                 string
	TCADPAPIKey                    string
	TCADPTableResultType           string
	TCADPMarkdownImageResponseType string
	OutputFormat                   string
}

func NewPPTXParser() *PPTXParser {
	return &PPTXParser{format: "pptx"}
}

func (p *PPTXParser) String() string {
	return "PPTXParser"
}

// ConfigureFromSetup reads the slides-family setup map. Mirrors the
// XLSXParser ConfigureFromSetup pattern
func (p *PPTXParser) ConfigureFromSetup(setup map[string]any) {
	if p == nil || setup == nil {
		return
	}
	if v, ok := setup["parse_method"].(string); ok {
		p.ParseMethod = v
	}
	if v, ok := setup["tcadp_apiserver"].(string); ok {
		p.TCADPAPIServer = v
	}
	if v, ok := setup["tcadp_api_key"].(string); ok {
		p.TCADPAPIKey = v
	}
	if v, ok := setup["table_result_type"].(string); ok {
		p.TCADPTableResultType = v
	}
	if v, ok := setup["markdown_image_response_type"].(string); ok {
		p.TCADPMarkdownImageResponseType = v
	}
	if v, ok := setup["output_format"].(string); ok {
		p.OutputFormat = v
	}
	if p.OutputFormat == "" {
		p.OutputFormat = "json"
	}
}

// ParseWithResult emits one JSON item per slide with the slide's
// plain text. Forces OutputFormat to "json" and emits one JSON item
// per slide section.
func (p *PPTXParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	// p == nil guard: the struct is embedded by value in PPTParser and
	// always created via NewPPTXParser or the "ppt"-format constructor in
	// PPTParser, so this branch is unreachable from normal call paths.
	// Kept as defensive guard — a nil dereference here would obscure the
	// root cause behind a nil-pointer panic.
	if p == nil {
		return ParseResult{Err: fmt.Errorf("PPTXParser is nil")}
	}
	method := strings.ToLower(strings.TrimSpace(p.ParseMethod))
	switch method {
	case "tcadp":
		// TCADP file_type is intentionally derived from the parser's
		// configured family (p.format == "pptx" → "PPTX", "ppt" → "PPT"),
		// not from magic-byte sniffing. The pipeline routes purely on
		// extension → parser family, and TCADP reconstructs by family.
		// The OLE fallback below therefore only applies to the local
		// office_oxide path.
		return parseWithTCADP(ctx,
			filename, data, strings.ToUpper(p.format),
			p.TCADPAPIServer, p.TCADPAPIKey,
			p.TCADPTableResultType, p.TCADPMarkdownImageResponseType,
			p.OutputFormat,
		)
	case "", "deepdoc":
		// Continue with the local office_oxide parser.
	default:
		// PDF-specific methods like "paddleocr" / "mineru" are
		// meaningless for PPTX; treat as default path.
	}

	// office_oxide's OpenFromBytes takes the container format as given
	// and does no magic-byte detection, so a legacy .ppt uploaded with a
	// .pptx extension would be parsed as ZIP/OOXML and fail. Sniff the
	// real container and pass the matching format so mislabeled files
	// still parse (mirrors the magic-byte correction office_oxide::Open
	// performs on file paths).
	// File["format"] in the success case reflects effFormat, i.e. the
	// real container ("ppt" for OLE fallback, "pptx" for OOXML fallback),
	// not just the extension.
	effFormat := p.format
	switch officeContainer(data) {
	case "ole":
		if effFormat == "pptx" {
			effFormat = "ppt"
		}
	case "ooxml":
		if effFormat == "ppt" {
			effFormat = "pptx"
		}
	}
	irJSON, _, plainText, _, err := pptxExtract(data, effFormat)
	if err != nil {
		if errors.Is(err, deepdocoffice.ErrOfficeCGORequired) {
			return ParseResult{Err: fmt.Errorf("%w: %s", ErrOfficeCGORequired, filename)}
		}
		return ParseResult{Err: fmt.Errorf("presentation extract: %w", err)}
	}

	// office_oxide's PlainText renders slides back-to-back with no page
	// delimiter, so per-slide splitting must go through the structured
	// IR, which carries one section per slide. When the IR is empty, fall
	// back to whole-document plain text (worst case: the entire deck as one
	// chunk) so a still-readable deck yields content instead of a hard
	// parse error.
	if strings.TrimSpace(irJSON) == "" {
		if strings.TrimSpace(plainText) == "" {
			return ParseResult{Err: fmt.Errorf("presentation ir-json: empty result")}
		}
		return ParseResult{
			OutputFormat: "json",
			File:         map[string]any{"name": filename, "format": effFormat},
			JSON:         itemsFromPlainText(plainText),
		}
	}
	budget := newEmbeddedMediaBudget()
	items, err := buildPPTXJSONSections(irJSON, budget)
	if err != nil {
		return ParseResult{Err: err}
	}
	if err := ctx.Err(); err != nil {
		return ParseResult{Err: err}
	}
	if len(items) == 0 || itemsAllEmpty(items) {
		// A deck whose IR carries no sections at all, or whose sections
		// all flatten to empty text (text the IR walker does not cover):
		// keep the whole-document text as a single item so a
		// still-readable file yields content instead of none. Without
		// the all-empty check, empty items flow to the Tokenizer, which
		// filters them as empty text, silently yielding 0 chunks.
		if strings.TrimSpace(plainText) != "" {
			items = itemsFromPlainText(plainText)
		}
	}

	return ParseResult{
		OutputFormat: "json",
		File:         map[string]any{"name": filename, "format": effFormat},
		JSON:         items,
		Warnings:     budget.warnings(),
	}
}
