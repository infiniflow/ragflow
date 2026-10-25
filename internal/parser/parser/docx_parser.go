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
	RemoveTOC          bool
	RemoveHeaderFooter bool
}

func NewDOCXParser() *DOCXParser {
	return &DOCXParser{}
}

// ConfigureFromSetup implements parserSetupConfigurer, receiving the
// DSL "docx" family setup map.
func (p *DOCXParser) ConfigureFromSetup(setup map[string]any) {
	if p == nil || setup == nil {
		return
	}
	if v, ok := setup["remove_toc"].(bool); ok {
		p.RemoveTOC = v
	}
	if v, ok := setup["remove_header_footer"].(bool); ok {
		p.RemoveHeaderFooter = v
	}
}

// ParseWithResult produces structured JSON items from a docx document.
// It emits bounded image items and attaches figure metadata to the file
// result.
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
	irJSON, _, _, _, err := docxExtract(data, format)
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

	// Extract IR JSON for section building and embedded-image extraction.
	// The budget bounds image payloads so the downstream VLM enhancement
	// stays within limits.
	budget := newEmbeddedMediaBudget()
	if irJSON != "" {
		figures := extractDOCXFiguresFromIR(irJSON, budget)
		if len(figures) > 0 {
			fileMeta["figures"] = buildFiguresMap(figures)
		}
	}

	// The result is built entirely from the structured IR, so an empty IR
	// is a fatal parse error. The OpenAndExtract contract swallows per-view
	// errors, so we cannot distinguish "IR failed" from "IR legitimately
	// empty"; either way there is nothing to build from, so fail loud
	// rather than emit empty/garbage JSON sections.
	if irJSON == "" {
		return ParseResult{Err: fmt.Errorf("docx extract: empty IR, cannot build JSON output")}
	}
	// TEMPORARY Caption-styled-paragraph detection (office_oxide does
	// not yet expose paragraph styles; yfedoseev/office_oxide#455).
	// Remove once office_oxide is upgraded to emit them.
	tableCaptions, capErr := extractDOCXTableCaptions(data)
	if capErr != nil {
		tableCaptions = nil
	}
	sections := buildDOCXJSONSectionsWithCaptions(irJSON, budget, tableCaptions)
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
		File:     fileMeta,
		JSON:     MapsToItems(sections),
		Warnings: budget.warnings(),
	}
}

func (p *DOCXParser) String() string {
	return "DOCXParser"
}
