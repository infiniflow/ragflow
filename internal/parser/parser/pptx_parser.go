//go:build cgo

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
	"fmt"
	"strings"

	officeOxide "github.com/yfedoseev/office_oxide/go"
)

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
// plain text. Mirrors the python parser.py:slides branch which
// forces output_format="json" for the slide family.
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
		return parsePresentationWithTCADP(ctx,
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
	doc, err := officeOxide.OpenFromBytes(data, p.format)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("presentation open: %w", err)}
	}
	defer doc.Close()

	text, err := doc.PlainText()
	if err != nil {
		return ParseResult{Err: fmt.Errorf("pptx plain-text: %w", err)}
	}

	// Split on form-feed (the python TxtParser convention used by
	// RAGFlow's slide parser) — each block becomes a JSON item.
	var items []map[string]any
	for i, raw := range strings.Split(text, "\f") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		items = append(items, map[string]any{
			"text":         trimmed,
			"doc_type_kwd": "text",
			"slide_number": i + 1,
		})
	}
	if items == nil {
		items = []map[string]any{{"text": strings.TrimSpace(text), "doc_type_kwd": "text"}}
	}

	return ParseResult{
		OutputFormat: "json",
		File:         map[string]any{"name": filename, "format": p.format},
		JSON:         items,
	}
}
