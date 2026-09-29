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

// docxExtract is a test seam for the native office_oxide extraction. It
// defaults to the deepdoc/docx backend (which is the only place that depends
// on office_oxide) and is overridden in tests to capture the effective
// container format passed to the engine.
var docxExtract = deepdocoffice.OpenAndExtract

// DOCXParser is the DOCX/DOC parser. It is a CGO-free facade: magic-byte
// sniffing and post-processing live here, while the native extraction lives
// in internal/deepdoc/parser/office. The IR data model and postprocessing live
// in cgo-free files (docx_ir.go, docx_postprocess.go, office_table_render.go)
// so they compile and test without native libraries.
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
// ParseWithResult produces JSON items (structured) or Markdown.
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
// docx document. The JSON path emits bounded image items; the
// Markdown path attaches bounded figure metadata to the file result.
//
// JSON path mirrors python parser.py:_docx() output_format == "json".
// Markdown path mirrors python naive.py: Docx() → naive_merge_docx().
//
// File["format"] in the returned ParseResult reflects the effective
// container format after magic-byte sniffing (e.g. "doc" for an OLE
// payload uploaded as .docx), not merely the file extension. See
// office_detect.go:officeContainer for the detection contract.
func (p *DOCXParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	// office_oxide's OpenFromBytes takes the container format as given
	// and does no magic-byte detection, so a legacy .doc uploaded with a
	// .docx extension would be parsed as ZIP/OOXML and fail. Sniff the
	// real container and pass the matching format so mislabeled files
	// still parse (mirrors the magic-byte correction office_oxide::Open
	// performs on file paths).
	format := "docx"
	if officeContainer(data) == "ole" {
		format = "doc"
	}
	// Note: when format == "doc" (OLE fallback), header/footer and TOC
	// post-processing below degrades gracefully: extractDOCXHeaderFooterTexts
	// opens the payload as ZIP and fails closed, so RemoveHeaderFooter becomes
	// a no-op for legacy OLE inputs. This is acceptable — legacy DOC has no
	// OOXML header/footer parts to strip.
	irJSON, md, _, mdErr, err := docxExtract(data, format)
	if err != nil {
		if errors.Is(err, deepdocoffice.ErrOfficeCGORequired) {
			return ParseResult{Err: fmt.Errorf("%w: %s", ErrOfficeCGORequired, filename)}
		}
		return ParseResult{Err: fmt.Errorf("docx extract: %w", err)}
	}

	fileMeta := map[string]any{
		"name":   filename,
		"format": format,
	}

	// Extract IR JSON for section building (JSON path) and
	// embedded-image extraction (both paths). The budget bounds image
	// payloads so the downstream VLM enhancement stays within limits.
	budget := newEmbeddedMediaBudget()
	if irJSON != "" {
		figures := extractDOCXFiguresFromIR(irJSON, budget)
		if len(figures) > 0 {
			fileMeta["figures"] = buildFiguresMap(figures)
		}
	}

	if p.outputFormat == "json" {
		// The JSON path is built entirely from the structured IR, so an
		// empty IR is a fatal parse error for this output format — the
		// Markdown path below still has the plain-text fallback. The
		// OpenAndExtract contract swallows per-view errors, so we cannot
		// distinguish "IR failed" from "IR legitimately empty"; either way
		// the JSON path has nothing to build from, so we fail loud
		// (Finding B) rather than emit empty/garbage JSON sections.
		if irJSON == "" {
			return ParseResult{Err: fmt.Errorf("docx extract: empty IR, cannot build JSON output")}
		}
		sections := buildDOCXJSONSections(irJSON, budget)
		if err := ctx.Err(); err != nil {
			return ParseResult{Err: err}
		}
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
			sections = removeTOCWord(sections, outlines, isEnglishItems(sections))
		}
		if len(sections) == 0 {
			sections = []map[string]any{{"text": "", "doc_type_kwd": "text"}}
		}
		return ParseResult{
			OutputFormat: "json",
			File:         fileMeta,
			JSON:         sections,
			Warnings:     budget.warnings(),
		}
	}

	// Default / Markdown path.
	if mdErr != nil {
		// The Markdown view failed but other views survived. Surface the
		// Markdown render error rather than silently emitting empty markdown
		// (PR review Finding 3). The JSON path above is unaffected because it
		// is built from the IR view.
		return ParseResult{Err: fmt.Errorf("docx extract: markdown render failed: %w", mdErr)}
	}
	markdownPayload := md
	// remove_header_footer on Markdown: filter lines by exact match
	// (mirrors Python parser.py:923-926 split lines → filter → rejoin).
	if p.RemoveHeaderFooter {
		hfTexts := extractDOCXHeaderFooterTexts(data)
		lines := strings.Split(markdownPayload, "\n")
		lineItems := make([]map[string]any, 0, len(lines))
		for _, ln := range lines {
			lineItems = append(lineItems, map[string]any{"text": ln})
		}
		lineItems = removeDOCXHeaderFooterSections(lineItems, hfTexts)
		rebuilt := make([]string, 0, len(lineItems))
		for _, item := range lineItems {
			rebuilt = append(rebuilt, itemText(item))
		}
		markdownPayload = strings.Join(rebuilt, "\n")
	}
	// remove_toc on Markdown: split lines, filter, rejoin
	// (mirrors Python parser.py:927-928 remove_toc_word on Markdown).
	if p.RemoveTOC && irJSON != "" {
		outlines := extractDOCXOutlines(irJSON)
		lines := strings.Split(markdownPayload, "\n")
		lineItems := make([]map[string]any, 0, len(lines))
		for _, ln := range lines {
			lineItems = append(lineItems, map[string]any{"text": ln})
		}
		filtered := removeTOCWord(lineItems, outlines, isEnglishItems(lineItems))
		rebuilt := make([]string, 0, len(filtered))
		for _, item := range filtered {
			rebuilt = append(rebuilt, itemText(item))
		}
		markdownPayload = strings.Join(rebuilt, "\n")
	}
	return ParseResult{
		OutputFormat: "markdown",
		File:         fileMeta,
		Markdown:     markdownPayload,
		Warnings:     budget.warnings(),
	}
}

func (p *DOCXParser) String() string {
	return "DOCXParser"
}
