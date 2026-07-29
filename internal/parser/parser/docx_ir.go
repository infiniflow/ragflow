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

// Package parser: this file holds the pure-Go office_oxide IR data
// model and JSON helpers for DOCX. It is intentionally cgo-free so
// that postprocessing (docx_postprocess.go) and unit tests run
// without the office_oxide native library. The cgo boundary lives
// entirely in docx_parser.go (officeOxide.OpenFromBytes).

package parser

import (
	"encoding/base64"
	"encoding/json"
	"html"
	"strings"
)

// DOCXFigure represents one embedded image plus its surrounding text
// context, mirroring the chunk-level shape that Python's
// naive_merge_docx produces for vision_figure_parser_docx_wrapper_naive.
type DOCXFigure struct {
	Image        string `json:"image"`         // base64-encoded image bytes
	ContextAbove string `json:"context_above"` // text before the image block
	ContextBelow string `json:"context_below"` // text after the image block
	Marker       string `json:"marker"`        // substring to locate image position in markdown
}

// --- office_oxide IR types (local copy, independent of deepdoc) ---

type docxIRDocument struct {
	Sections []docxIRSection `json:"sections"`
}

type docxIRSection struct {
	Title    string          `json:"title"`
	Elements []docxIRElement `json:"elements"`
}

type docxIRElement struct {
	Type    string           `json:"type"`    // "paragraph", "heading", "table", "image", "list", "text_box", ...
	Level   int              `json:"level"`   // heading level (1-6) or list nesting level
	Style   string           `json:"style"`   // Word style name (e.g. "Normal", "Heading 1")
	Content json.RawMessage  `json:"content"` // rich text runs or block-level content; decoded per type
	Data    []byte           `json:"data"`    // raw image bytes (for "image" type)
	Rows    []docxIRRow      `json:"rows"`    // table rows
	Ordered bool             `json:"ordered"` // true=numbered list, false=bullet list (for "list" type)
	Items   []docxIRListItem `json:"items"`   // list items (for "list" type)
}

// contentRuns decodes Content as flat text runs (paragraph/heading type).
func (e docxIRElement) contentRuns() []docxIRRun {
	var runs []docxIRRun
	if len(e.Content) > 0 {
		_ = json.Unmarshal(e.Content, &runs)
	}
	return runs
}

// contentBlocks decodes Content as block-level elements (text_box type).
func (e docxIRElement) contentBlocks() []docxIRElement {
	var blocks []docxIRElement
	if len(e.Content) > 0 {
		_ = json.Unmarshal(e.Content, &blocks)
	}
	return blocks
}

// docxIRListItem represents one item in an ordered/unordered list.
type docxIRListItem struct {
	Content []docxIRElement `json:"content"`          // block-level content (typically a single Paragraph)
	Nested  *docxIRList     `json:"nested,omitempty"` // optional nested sub-list; null/absent when none
}

// docxIRList mirrors office_oxide's ir::List. Only Items is needed for
// text extraction; the remaining List fields (ordered, start_number,
// style, level) are not consumed here, so they are not modeled.
type docxIRList struct {
	Items []docxIRListItem `json:"items"`
}

type docxIRRun struct {
	Type    string          `json:"type"` // "text", "image"
	Text    string          `json:"text"`
	Content []docxIRElement `json:"content"` // nested elements (used in table cells)
}

type docxIRRow struct {
	Cells []docxIRCell `json:"cells"`
}

type docxIRCell struct {
	Content []docxIRElement `json:"content"` // nested paragraphs inside table cell
}

func joinDOCXIRRuns(runs []docxIRRun) string {
	var b strings.Builder
	for _, r := range runs {
		if r.Type == "text" {
			b.WriteString(r.Text)
		}
	}
	return b.String()
}

// extractTextFromListItem extracts the plain text content from a list item.
// Each list item contains block-level elements (typically a Paragraph),
// whose text runs are concatenated. Nested sub-lists (multi-level
// bullets/numbered items) are decoded and recursed so their text is not
// silently dropped. Mirrors office_oxide ir::ListItem { content, nested }.
func extractTextFromListItem(item docxIRListItem) string {
	var parts []string
	for _, el := range item.Content {
		if el.Type == "paragraph" || el.Type == "heading" {
			t := joinDOCXIRRuns(el.contentRuns())
			if t != "" {
				parts = append(parts, t)
			}
		}
	}
	if item.Nested != nil {
		for _, sub := range item.Nested.Items {
			if t := extractTextFromListItem(sub); t != "" {
				parts = append(parts, t)
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// extractTextFromBlockElements extracts text from a slice of block-level
// elements (paragraphs/headings), used by text_box and other compound
// element types.
func extractTextFromBlockElements(blocks []docxIRElement) string {
	var parts []string
	for _, el := range blocks {
		if el.Type == "paragraph" || el.Type == "heading" {
			t := joinDOCXIRRuns(el.contentRuns())
			if t != "" {
				parts = append(parts, t)
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// joinCellText concatenates all paragraph texts inside a table cell,
// joined by newlines.
func joinCellText(cell docxIRCell) string {
	var parts []string
	for _, el := range cell.Content {
		if text := joinDOCXIRRuns(el.contentRuns()); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

// docxIRTableToHTML converts a table IR element to an HTML table string.
func docxIRTableToHTML(el docxIRElement) string {
	var sb strings.Builder
	sb.WriteString("<table>")
	for _, row := range el.Rows {
		sb.WriteString("<tr>")
		for _, cell := range row.Cells {
			sb.WriteString("<td>")
			sb.WriteString(html.EscapeString(joinCellText(cell)))
			sb.WriteString("</td>")
		}
		sb.WriteString("</tr>")
	}
	sb.WriteString("</table>")
	return sb.String()
}

// docxElementText returns the plain-text rendering of any supported
// IR element type. Used by extractDOCXFiguresFromIR so that tables,
// lists, and text boxes contribute to image surrounding context
// instead of becoming empty flatBlocks (which would drop adjacent
// VLM context). Returns "" for image and unknown types.
func docxElementText(el docxIRElement) string {
	switch el.Type {
	case "paragraph", "heading":
		return joinDOCXIRRuns(el.contentRuns())
	case "table":
		var lines []string
		for _, row := range el.Rows {
			for _, cell := range row.Cells {
				if t := joinCellText(cell); t != "" {
					lines = append(lines, t)
				}
			}
		}
		return strings.Join(lines, "\n")
	case "list":
		var lines []string
		for _, item := range el.Items {
			if t := extractTextFromListItem(item); t != "" {
				lines = append(lines, t)
			}
		}
		return strings.Join(lines, "\n")
	case "text_box":
		return extractTextFromBlockElements(el.contentBlocks())
	default:
		return ""
	}
}

// buildDOCXJSONSections converts an office_oxide IR JSON string into a
// slice of structured items compatible with the chunker's JSON input
// contract. Each item carries at least text and doc_type_kwd.
func buildDOCXJSONSections(irJSON string) []map[string]any {
	var ir docxIRDocument
	if err := json.Unmarshal([]byte(irJSON), &ir); err != nil {
		return nil
	}
	var sections []map[string]any
	for _, sec := range ir.Sections {
		for _, el := range sec.Elements {
			switch el.Type {
			case "paragraph", "heading":
				text := joinDOCXIRRuns(el.contentRuns())
				if strings.TrimSpace(text) == "" {
					continue
				}
				item := map[string]any{
					"text":         text,
					"image":        nil,
					"doc_type_kwd": "text",
				}
				if el.Type == "heading" {
					item["ck_type"] = "heading"
				}
				sections = append(sections, item)

			case "image":
				b64 := base64.StdEncoding.EncodeToString(el.Data)
				sections = append(sections, map[string]any{
					"text":         "",
					"image":        b64,
					"doc_type_kwd": "image",
				})

			case "table":
				html := docxIRTableToHTML(el)
				if html == "<table></table>" {
					continue
				}
				sections = append(sections, map[string]any{
					"text":         html,
					"image":        nil,
					"doc_type_kwd": "table",
				})

			case "list":
				for _, item := range el.Items {
					text := extractTextFromListItem(item)
					if text == "" {
						continue
					}
					sections = append(sections, map[string]any{
						"text":         text,
						"image":        nil,
						"doc_type_kwd": "text",
					})
				}

			case "text_box":
				text := extractTextFromBlockElements(el.contentBlocks())
				if text == "" {
					continue
				}
				sections = append(sections, map[string]any{
					"text":         text,
					"image":        nil,
					"doc_type_kwd": "text",
				})
			}
		}
	}
	return sections
}

// --- figure extraction (used by the cgo parser path) ---

// extractDOCXFiguresFromIR parses the office_oxide IR JSON and
// returns every embedded image block together with the plain text
// immediately surrounding it. The context matches what Python's
// naive_merge_docx attaches as context_above / context_below on
// each chunk that carries an image.
//
// Reuses the IR already obtained from the doc handle in
// ParseWithResult so the binary is not opened twice.
func extractDOCXFiguresFromIR(irJSON string) []DOCXFigure {
	var ir docxIRDocument
	if err := json.Unmarshal([]byte(irJSON), &ir); err != nil {
		return nil
	}

	var flat []flatBlock
	for _, sec := range ir.Sections {
		for _, el := range sec.Elements {
			if el.Type == "image" {
				b64 := base64.StdEncoding.EncodeToString(el.Data)
				flat = append(flat, flatBlock{image: b64})
				continue
			}
			text := docxElementText(el)
			flat = append(flat, flatBlock{text: text})
		}
	}

	var figures []DOCXFigure
	for i, block := range flat {
		if block.image == "" {
			continue
		}
		fig := DOCXFigure{Image: block.image}

		// Collect text above (backward scan up to docxContextWindow
		// chars, or until another image is hit).
		above := collectDOCXPrevText(flat, i, 512)
		fig.ContextAbove = strings.TrimSpace(above)

		// Collect text below (forward scan up to docxContextWindow
		// chars, or until another image is hit).
		below := collectDOCXNextText(flat, i, 512)
		fig.ContextBelow = strings.TrimSpace(below)

		// Marker: text of the immediately preceding flat block,
		// used by the vision dispatcher to locate the image position
		// in the rendered markdown for inline insertion.
		for j := i - 1; j >= 0; j-- {
			if flat[j].text != "" {
				fig.Marker = flat[j].text
				break
			}
		}

		figures = append(figures, fig)
	}
	return figures
}

// --- internal types ---

// flatBlock is a flattened IR element used internally to collect
// text / image context around embedded figures.
type flatBlock struct {
	text  string
	image string // base64-encoded image data (empty for non-image)
}

const docxContextWindow = 512

func collectDOCXPrevText(flat []flatBlock, idx, maxLen int) string {
	var parts []string
	remaining := maxLen
	for i := idx - 1; i >= 0 && remaining > 0; i-- {
		if flat[i].image != "" {
			break // stop at previous image
		}
		if flat[i].text == "" {
			continue
		}
		r := []rune(flat[i].text)
		if len(r) > remaining {
			r = r[len(r)-remaining:]
		}
		parts = append([]string{string(r)}, parts...)
		remaining -= len(r)
	}
	// parts are in document order (farthest first, closest last);
	// keep the tail = text nearest the image when truncating for the
	// newline separators that Join inserts (they are not counted by
	// `remaining`, so the joined length can otherwise exceed maxLen).
	joined := strings.Join(parts, "\n")
	if r := []rune(joined); len(r) > maxLen {
		joined = string(r[len(r)-maxLen:])
	}
	return joined
}

func collectDOCXNextText(flat []flatBlock, idx, maxLen int) string {
	var parts []string
	remaining := maxLen
	for i := idx + 1; i < len(flat) && remaining > 0; i++ {
		if flat[i].image != "" {
			break // stop at next image
		}
		if flat[i].text == "" {
			continue
		}
		r := []rune(flat[i].text)
		if len(r) > remaining {
			r = r[:remaining]
		}
		parts = append(parts, string(r))
		remaining -= len(r)
	}
	// parts are in document order (closest first, farthest last);
	// keep the head = text nearest the image when truncating for the
	// newline separators that Join inserts.
	joined := strings.Join(parts, "\n")
	if r := []rune(joined); len(r) > maxLen {
		joined = string(r[:maxLen])
	}
	return joined
}

// buildFiguresMap converts the internal DOCXFigure slice to the
// map form attached to fileMeta["figures"].
func buildFiguresMap(figures []DOCXFigure) []map[string]any {
	figs := make([]map[string]any, 0, len(figures))
	for _, f := range figures {
		figs = append(figs, map[string]any{
			"image":         f.Image,
			"context_above": f.ContextAbove,
			"context_below": f.ContextBelow,
			"marker":        f.Marker,
		})
	}
	return figs
}
