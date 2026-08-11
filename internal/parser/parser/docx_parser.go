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

// DOCXParser is the cgo-backed DOCX parser. It is the only DOCX
// entrypoint that depends on office_oxide; the IR data model and
// postprocessing live in cgo-free files (docx_ir.go, docx_postprocess.go)
// so they compile and test without native libraries. The !cgo build
// provides a stub DOCXParser in office_parsers_no_cgo.go.
type DOCXParser struct {
	libType            string
	outputFormat       string // from DSL config; "json" or "markdown"
	RemoveTOC          bool
	RemoveHeaderFooter bool
}

func NewDOCXParser() *DOCXParser {
	return &DOCXParser{}
}

// ConfigureFromSetup implements parserSetupConfigurer, receiving the
// DSL "docx" family setup map. The output_format key drives whether
// ParseWithResult produces JSON items (structured) or markdown.
func (p *DOCXParser) ConfigureFromSetup(setup map[string]any) {
	if p == nil || setup == nil {
		return
	}
	if v, ok := setup["output_format"].(string); ok && v != "" {
		p.outputFormat = v
	}
	if v, ok := setup["remove_toc"].(bool); ok {
		p.RemoveTOC = v
	}
	if v, ok := setup["remove_header_footer"].(bool); ok {
		p.RemoveHeaderFooter = v
	}
}

// ParseWithResult produces structured JSON items (when
// p.outputFormat == "json") or markdown (default) from a
// docx document. Embedded images are extracted in both paths
// for downstream vision-figure dispatch.
//
// JSON path mirrors python parser.py:_docx() output_format == "json".
// Markdown path mirrors python naive.py: Docx() → naive_merge_docx().
func (p *DOCXParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	doc, err := officeOxide.OpenFromBytes(data, "docx")
	if err != nil {
		return ParseResult{Err: fmt.Errorf("docx open: %w", err)}
	}
	defer doc.Close()

	fileMeta := map[string]any{
		"name":   filename,
		"format": "docx",
	}

	// Extract IR JSON for section building (JSON path) and
	// embedded-image extraction (both paths).
	irJSON, irErr := doc.ToIRJSON()
	var figures []DOCXFigure
	if irErr == nil {
		figures = extractDOCXFiguresFromIR(irJSON)
	}
	if len(figures) > 0 {
		fileMeta["figures"] = buildFiguresMap(figures)
	}

	if p.outputFormat == "json" {
		if irErr != nil {
			return ParseResult{Err: fmt.Errorf("docx to-ir-json: %w", irErr)}
		}
		var sections []map[string]any
		sections = buildDOCXJSONSections(irJSON)
		// remove_header_footer: drop sections whose normalized text
		// matches a docx header/footer entry (mirrors Python
		// parser.py:889-891 extract_docx_header_footer_texts +
		// remove_header_footer_docx_sections).
		if p.RemoveHeaderFooter {
			hfTexts := extractDOCXHeaderFooterTexts(data)
			sections = removeDOCXHeaderFooterSections(sections, hfTexts)
		}
		// remove_toc: filter TOC entries using heading outlines
		// (mirrors Python parser.py:892-893 remove_toc_word).
		if p.RemoveTOC {
			outlines := extractDOCXOutlines(irJSON)
			sections = removeTOCWord(sections, outlines, false)
		}
		if len(sections) == 0 {
			sections = []map[string]any{{"text": "", "doc_type_kwd": "text"}}
		}
		return ParseResult{
			OutputFormat: "json",
			File:         fileMeta,
			JSON:         sections,
		}
	}

	// Default / markdown path.
	md, err := doc.ToMarkdown()
	if err != nil {
		return ParseResult{Err: fmt.Errorf("docx to-markdown: %w", err)}
	}
	// remove_header_footer on markdown: filter lines by exact match
	// (mirrors Python parser.py:923-926 split lines → filter → rejoin).
	if p.RemoveHeaderFooter {
		hfTexts := extractDOCXHeaderFooterTexts(data)
		lines := strings.Split(md, "\n")
		lineItems := make([]map[string]any, 0, len(lines))
		for _, ln := range lines {
			lineItems = append(lineItems, map[string]any{"text": ln})
		}
		lineItems = removeDOCXHeaderFooterSections(lineItems, hfTexts)
		rebuilt := make([]string, 0, len(lineItems))
		for _, item := range lineItems {
			rebuilt = append(rebuilt, itemText(item))
		}
		md = strings.Join(rebuilt, "\n")
	}
	// remove_toc on markdown: split lines, filter, rejoin
	// (mirrors Python parser.py:927-928 remove_toc_word on markdown).
	if p.RemoveTOC && irErr == nil {
		outlines := extractDOCXOutlines(irJSON)
		lines := strings.Split(md, "\n")
		lineItems := make([]map[string]any, 0, len(lines))
		for _, ln := range lines {
			lineItems = append(lineItems, map[string]any{"text": ln})
		}
		filtered := removeTOCWord(lineItems, outlines, false)
		rebuilt := make([]string, 0, len(filtered))
		for _, item := range filtered {
			rebuilt = append(rebuilt, itemText(item))
		}
		md = strings.Join(rebuilt, "\n")
	}
	return ParseResult{
		OutputFormat: "markdown",
		File:         fileMeta,
		Markdown:     md,
	}
}

func (p *DOCXParser) String() string {
	return "DOCXParser"
}
