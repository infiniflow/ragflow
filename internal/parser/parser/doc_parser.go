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

type DOCParser struct {
	OutputFormat string
}

func NewDOCParser() *DOCParser {
	return &DOCParser{
		OutputFormat: "json",
	}
}

func (p *DOCParser) String() string {
	return "DOCParser"
}

func (p *DOCParser) ConfigureFromSetup(setup map[string]any) {
	if p == nil || setup == nil {
		return
	}
	if v, ok := setup["output_format"].(string); ok && v != "" {
		p.OutputFormat = v
	}
}

// ParseWithResult extracts text for the DOC family and converts it to the
// requested output format (defaulting to "json" to match Python's default).
func (p *DOCParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	doc, err := officeOxide.OpenFromBytes(data, "doc")
	if err != nil {
		return ParseResult{Err: fmt.Errorf("doc open: %w", err)}
	}
	defer doc.Close()

	text, mdText, err := extractDocText(doc)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("doc extract: %w", err)}
	}

	outFmt := p.OutputFormat
	if outFmt == "" {
		outFmt = "json"
	}

	markdownPayload := text
	if strings.TrimSpace(mdText) != "" {
		markdownPayload = mdText
	}

	res := ParseResult{
		OutputFormat: outFmt,
		File:         map[string]any{"name": filename, "format": "doc"},
		Text:         text,
		Markdown:     markdownPayload,
	}

	if strings.EqualFold(outFmt, "markdown") {
		return res
	}
	if strings.EqualFold(outFmt, "text") {
		return res
	}

	// Default: "json" payload with canonical text items
	lines := strings.Split(text, "\n")
	var items []map[string]any
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			items = append(items, NewTextJSONItem(trimmed))
		}
	}
	if len(items) == 0 && strings.TrimSpace(text) != "" {
		items = append(items, NewTextJSONItem(strings.TrimSpace(text)))
	}
	if len(items) == 0 {
		items = []map[string]any{NewTextJSONItem("")}
	}

	res.OutputFormat = "json"
	res.JSON = items
	return res
}

// extractDocText returns the best-effort plain text for a legacy .doc
// document. office_oxide exposes three text views:
//   - ToIRJSON: a structured DocumentIR, flattened to plain text via
//     flattenDocIR. Recovers table structure (office_oxide/go v0.1.9, #116)
//     and, when the reader surfaces them, list structure.
//   - ToMarkdown: paragraphs separated by blank lines.
//   - PlainText: the raw concatenated text.
//
// The IR is preferred when it carries table/list structure (that content is
// unique to the IR view and would be silently dropped under a pure length
// comparison — a prose-heavy document can make PlainText longer than the IR
// even though the IR is more complete). Otherwise the longest non-empty view
// is chosen so a sparser view never shadows a more complete one. A failure at
// every stage degrades to the PlainText error, preserving the original "no
// text at all" failure semantics.
func extractDocText(doc *officeOxide.Document) (string, string, error) {
	var irJSON, irText, mdText, plainText string
	if j, err := doc.ToIRJSON(); err == nil {
		irJSON = j
		irText = flattenDocIR(j)
	}
	if md, err := doc.ToMarkdown(); err == nil {
		mdText = md
	}
	if plain, err := doc.PlainText(); err == nil {
		plainText = plain
	} else if strings.TrimSpace(irText) == "" && strings.TrimSpace(mdText) == "" {
		// Every view failed (or produced nothing): keep the original
		// "no text at all" failure semantics.
		return "", "", err
	}
	return selectDocTextView(irJSON, irText, mdText, plainText), mdText, nil
}

// selectDocTextView chooses the best plain-text rendering from office_oxide's
// three views. irJSON is the raw IR (used only to detect structured elements);
// irText is its flattened form. When the IR recovers table or list structure
// it is preferred, because that content is unique to the IR view. Otherwise the
// longest non-empty view wins, so a sparser view never shadows a more complete
// one.
func selectDocTextView(irJSON, irText, mdText, plainText string) string {
	if strings.TrimSpace(irText) != "" && irHasStructuredContent(irJSON) {
		return irText
	}
	best := irText
	if len(strings.TrimSpace(mdText)) > len(strings.TrimSpace(best)) {
		best = mdText
	}
	if len(strings.TrimSpace(plainText)) > len(strings.TrimSpace(best)) {
		best = plainText
	}
	return best
}

// irHasStructuredContent reports whether the office_oxide IR JSON carries
// table or list elements. Those structures are only surfaced by the IR view;
// ToMarkdown and PlainText flatten them away. Detection keys on the canonical
// `"type":"table"` / `"type":"list"` field emitted by office_oxide.
func irHasStructuredContent(irJSON string) bool {
	return strings.Contains(irJSON, `"type":"table"`) ||
		strings.Contains(irJSON, `"type":"list"`)
}
