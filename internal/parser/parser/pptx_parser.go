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
	format   string
	textOnly bool

	// TCADP cloud-parsing configuration
	ParseMethod                    string
	TCADPAPIServer                 string
	TCADPAPIKey                    string
	TCADPTableResultType           string
	TCADPMarkdownImageResponseType string
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
}

// ParseWithResult emits one JSON item per slide with the slide's plain text,
// tables and media_ref image items. A text-only parser skips images.
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
		return parseWithTCADP(ctx,
			filename, data, strings.ToUpper(p.format),
			p.TCADPAPIServer, p.TCADPAPIKey,
			p.TCADPTableResultType, p.TCADPMarkdownImageResponseType,
		)
	case "", "deepdoc":
		// Continue with the local office_oxide parser.
	default:
		// PDF-specific methods like "paddleocr" / "mineru" are
		// meaningless for PPTX; treat as default path.
	}

	irJSON, _, plainText, _, err := pptxExtract(data, p.format)
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
			File: map[string]any{"name": filename, "format": p.format},
			JSON: MapsToItems(itemsFromPlainText(plainText)),
		}
	}
	var sink *officeMediaSink
	if !p.textOnly {
		sink = newOfficeMediaSink(ctx, officeMediaFormatPPTX)
	}
	items, err := buildPPTXJSONSections(irJSON, sink)
	if err != nil {
		return ParseResult{Err: err}
	}
	if sink != nil && sink.err != nil {
		return ParseResult{Err: sink.err}
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
		File:     map[string]any{"name": filename, "format": p.format},
		JSON:     MapsToItems(items),
		Warnings: sink.warnings(),
	}
}
