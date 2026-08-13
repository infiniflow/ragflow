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
	"encoding/json"
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

	// office_oxide's PlainText renders slides back-to-back with no page
	// delimiter, so per-slide splitting must go through the structured
	// IR, which carries one section per slide. This mirrors the python
	// RAGFlowPptParser, which iterates slides and yields one text per
	// slide ("Every page will be treated as a chunk").
	irJSON, err := doc.ToIRJSON()
	if err != nil {
		return ParseResult{Err: fmt.Errorf("presentation ir-json: %w", err)}
	}
	var ir presentationIR
	if err := json.Unmarshal([]byte(irJSON), &ir); err != nil {
		return ParseResult{Err: fmt.Errorf("presentation ir-json decode: %w", err)}
	}

	// One JSON item per slide, including slides without extractable
	// text — the python parser likewise appends one (possibly empty)
	// text per slide.
	items := make([]map[string]any, 0, len(ir.Sections))
	for i, sec := range ir.Sections {
		items = append(items, map[string]any{
			"text":         sec.text(),
			"doc_type_kwd": "text",
			"slide_number": i + 1,
		})
	}
	if len(items) == 0 {
		// A deck whose IR carries no sections at all: keep the
		// whole-document text as a single item so a still-readable
		// file yields one chunk instead of none.
		text, perr := doc.PlainText()
		if perr != nil {
			return ParseResult{Err: fmt.Errorf("presentation plain-text: %w", perr)}
		}
		if trimmed := strings.TrimSpace(text); trimmed != "" {
			items = append(items, map[string]any{"text": trimmed, "doc_type_kwd": "text"})
		}
	}

	return ParseResult{
		OutputFormat: "json",
		File:         map[string]any{"name": filename, "format": p.format},
		JSON:         items,
	}
}

// ── office_oxide presentation IR ─────────────────────────────────────
// Subset of the office_oxide IR JSON schema needed to recover per-slide
// plain text. Each slide is one section whose elements are block nodes
// (headings, paragraphs, text boxes, tables, images); block nodes nest
// (a text box contains paragraphs) and leaf "text" runs carry the
// characters.

type presentationIR struct {
	Sections []irSection `json:"sections"`
}

type irSection struct {
	Elements []irNode `json:"elements"`
}

type irNode struct {
	Type    string   `json:"type"`
	Text    string   `json:"text"`
	Content []irNode `json:"content"`
	Rows    []irRow  `json:"rows"`
}

type irRow struct {
	Cells []irCell `json:"cells"`
}

type irCell struct {
	Content []irNode `json:"content"`
}

// text flattens the slide's IR elements into plain text, one line per
// paragraph-level block — the same shape the python parser produces by
// joining per-shape texts with "\n".
func (s irSection) text() string {
	var lines []string
	for _, el := range s.Elements {
		lines = append(lines, irBlockLines(el)...)
	}
	return strings.Join(lines, "\n")
}

// irBlockLines returns one entry per paragraph-level block under n.
// Leaf text runs concatenate into the enclosing block's line; tables
// yield one line per row with cells joined by "; ".
func irBlockLines(n irNode) []string {
	if len(n.Rows) > 0 {
		lines := make([]string, 0, len(n.Rows))
		for _, row := range n.Rows {
			cells := make([]string, 0, len(row.Cells))
			for _, cell := range row.Cells {
				var parts []string
				for _, el := range cell.Content {
					parts = append(parts, irBlockLines(el)...)
				}
				cells = append(cells, strings.Join(parts, " "))
			}
			lines = append(lines, strings.Join(cells, "; "))
		}
		return lines
	}
	runsOnly := true
	for _, c := range n.Content {
		if c.Type != "text" {
			runsOnly = false
			break
		}
	}
	if runsOnly {
		var b strings.Builder
		for _, c := range n.Content {
			b.WriteString(c.Text)
		}
		if s := strings.TrimSpace(b.String()); s != "" {
			return []string{s}
		}
		return nil
	}
	var lines []string
	for _, c := range n.Content {
		lines = append(lines, irBlockLines(c)...)
	}
	return lines
}
