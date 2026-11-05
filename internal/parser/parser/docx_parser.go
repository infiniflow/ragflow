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
// defaults to the deepdoc/office backend, which owns the native dependency.
var docxExtract = deepdocoffice.OpenAndExtract

// DOCXParser parses DOCX documents. Native extraction lives in
// internal/deepdoc/parser/office. The IR data model and postprocessing live
// in cgo-free files (docx_ir.go, docx_postprocess.go, office_table_render.go)
// so they compile and test without native libraries.
type DOCXParser struct {
	libType            string
	textOnly           bool
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
// Embedded images are stored in the run media cache and emitted as media_ref;
// text-only parsers skip them instead.
func (p *DOCXParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	irJSON, _, _, _, err := docxExtract(data, "docx")
	if err != nil {
		if errors.Is(err, deepdocoffice.ErrOfficeCGORequired) {
			return ParseResult{Err: fmt.Errorf("%w: %s", ErrOfficeCGORequired, filename)}
		}
		return ParseResult{Err: fmt.Errorf("docx extract: %w", err)}
	}

	fileMeta := map[string]any{
		"name":   filename,
		"format": "docx",
	}

	var sink *officeMediaSink
	if !p.textOnly {
		sink = newOfficeMediaSink(ctx, officeMediaFormatDOCX)
	}

	// The result is built entirely from the structured IR, so an empty IR
	// is a fatal parse error. The OpenAndExtract contract swallows per-view
	// errors, so we cannot distinguish "IR failed" from "IR legitimately
	// empty"; either way there is nothing to build from, so fail loud
	// rather than emit empty/garbage JSON sections.
	if irJSON == "" {
		return ParseResult{Err: fmt.Errorf("docx extract: empty IR, cannot build JSON output")}
	}
	// office_oxide 0.1.16 exposes the Word paragraph style name (e.g.
	// "Caption") in the IR "style" field (yfedoseev/office_oxide#455); a
	// Caption-styled paragraph is attached to the table that follows it and is
	// not emitted as a standalone text section. This replaces the temporary
	// word/document.xml re-parse that the Go parser used before the field
	// existed.
	sections := buildDOCXJSONSections(irJSON, sink)
	if sink != nil && sink.err != nil {
		return ParseResult{Err: sink.err}
	}
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
		Warnings: sink.warnings(),
	}
}

func (p *DOCXParser) String() string {
	return "DOCXParser"
}
