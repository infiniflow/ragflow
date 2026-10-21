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
// model and JSON helpers shared by the DOCX and presentation parsers.
// It is intentionally cgo-free so that postprocessing
// (docx_postprocess.go), per-slide section building (pptx_ir.go), and
// unit tests run without the office_oxide native library. The cgo
// boundary lives in docx_parser.go / pptx_parser.go
// (officeOxide.OpenFromBytes).

package parser

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	Marker       string `json:"marker"`        // substring to locate image position in Markdown
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
	Text    string           `json:"text"`    // payload of a bare "text" block; not part of the current IR schema
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

// contentBlocks decodes Content as block-level elements (text_box type,
// list-item content, or table-cell content). Blocks may be plain paragraphs
// or compound elements (table, list, nested text_box); callers flatten each
// block via docxElementText so non-paragraph blocks are not silently
// dropped.
func (e docxIRElement) contentBlocks() []docxIRElement {
	var blocks []docxIRElement
	if len(e.Content) > 0 {
		_ = json.Unmarshal(e.Content, &blocks)
	}
	return blocks
}

// docxIRListItem represents one item in an ordered/unordered list.
type docxIRListItem struct {
	Content []docxIRElement `json:"content"`          // block-level content (typically a single Paragraph; may also hold table/list/text_box)
	Nested  *docxIRList     `json:"nested,omitempty"` // optional nested sub-list; null/absent when none
}

// docxIRList mirrors office_oxide's ir::List. Only Items is needed for
// text extraction; the remaining List fields (ordered, start_number,
// style, level) are not consumed here, so they are not modeled.
type docxIRList struct {
	Items []docxIRListItem `json:"items"`
}

type docxIRRun struct {
	Type    string          `json:"type"` // "text", "line_break", "image"
	Text    string          `json:"text"`
	Data    []byte          `json:"data"`    // raw image bytes for inline image runs
	Content []docxIRElement `json:"content"` // nested elements (used in table cells)
}

type docxIRRow struct {
	Cells []docxIRCell `json:"cells"`
}

type docxIRCell struct {
	Content []docxIRElement `json:"content"` // block-level content (typically paragraphs; may also hold list/table)
}

// joinDOCXIRRuns concatenates inline runs into plain text. Hard line
// breaks become newlines so multi-line text keeps its line structure;
// non-text payloads (e.g. inline images) contribute no characters.
func joinDOCXIRRuns(runs []docxIRRun) string {
	var b strings.Builder
	for _, r := range runs {
		switch r.Type {
		case "text":
			b.WriteString(r.Text)
		case "line_break":
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// extractTextFromListItem extracts the plain text content from a list item.
// Each block in the item is flattened via docxElementText so non-paragraph
// blocks (e.g. a table nested in a list item) are not silently dropped.
// Nested sub-lists (multi-level bullets/numbered items) are decoded and
// recursed so their text is not silently dropped.
// Mirrors office_oxide ir::ListItem { content, nested }.
func extractTextFromListItem(item docxIRListItem) string {
	var parts []string
	for _, el := range item.Content {
		if t := strings.TrimSpace(docxElementText(el, "\n")); t != "" {
			parts = append(parts, t)
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
// elements (e.g. the content of a text_box). Each block is flattened via
// docxElementText so non-paragraph blocks (e.g. a table wrapped in a
// text_box, as office_oxide emits for grouped slide shapes) are not
// silently dropped.
func extractTextFromBlockElements(blocks []docxIRElement) string {
	var parts []string
	for _, el := range blocks {
		if t := strings.TrimSpace(docxElementText(el, "\n")); t != "" {
			parts = append(parts, t)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// joinCellText concatenates all block texts inside a table cell, joined by
// newlines. Each block is flattened via docxElementText so non-paragraph
// blocks (e.g. a nested list) are not silently dropped.
func joinCellText(cell docxIRCell) string {
	var parts []string
	for _, el := range cell.Content {
		if text := strings.TrimSpace(docxElementText(el, "\n")); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

// docxIRTableToHTML converts a table IR element to an HTML table string.
// A non-empty caption is emitted as a leading <caption> element so the
// structured TableData contract (ParseTableHTML) carries it through to the
// chunk renderer, which escapes it again on output.
func docxIRTableToHTML(el docxIRElement, caption string) string {
	var sb strings.Builder
	sb.WriteString("<table>")
	if caption != "" {
		sb.WriteString("<caption>")
		sb.WriteString(html.EscapeString(caption))
		sb.WriteString("</caption>")
	}
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

type docxTableImage struct {
	data     []byte
	row      int
	column   int
	included bool
}

func forEachDOCXTableImage(el docxIRElement, visit func(docxTableImage) bool) bool {
	for rowIndex, row := range el.Rows {
		for columnIndex, cell := range row.Cells {
			completed := forEachDOCXIRImage(cell.Content, func(data []byte) bool {
				return visit(docxTableImage{data: data, row: rowIndex + 1, column: columnIndex + 1})
			})
			if !completed {
				return false
			}
		}
	}
	return true
}

// forEachDOCXIRImage visits image payloads without first collecting a second
// document-sized slice of image references. Returning false from visit stops
// traversal as soon as the caller's media budget is exhausted.
func forEachDOCXIRImage(elements []docxIRElement, visit func([]byte) bool) bool {
	for _, el := range elements {
		switch el.Type {
		case "image":
			if len(el.Data) > 0 && !visit(el.Data) {
				return false
			}
		case "paragraph", "heading":
			for _, run := range el.contentRuns() {
				if run.Type == "image" && len(run.Data) > 0 && !visit(run.Data) {
					return false
				}
				if !forEachDOCXIRImage(run.Content, visit) {
					return false
				}
			}
		case "table":
			for _, row := range el.Rows {
				for _, cell := range row.Cells {
					if !forEachDOCXIRImage(cell.Content, visit) {
						return false
					}
				}
			}
		case "list":
			if !forEachDOCXIRListImage(docxIRList{Items: el.Items}, visit) {
				return false
			}
		case "text_box":
			if !forEachDOCXIRImage(el.contentBlocks(), visit) {
				return false
			}
		}
	}
	return true
}

func forEachDOCXIRListImage(list docxIRList, visit func([]byte) bool) bool {
	for _, item := range list.Items {
		if !forEachDOCXIRImage(item.Content, visit) {
			return false
		}
		if item.Nested != nil {
			if !forEachDOCXIRListImage(*item.Nested, visit) {
				return false
			}
		}
	}
	return true
}

// docxElementText returns the plain-text rendering of any supported
// IR element type. Used by buildPPTXJSONSections to flatten one slide
// section, and by extractDOCXFiguresFromIR so that tables, lists, and
// text boxes contribute to image surrounding context instead of
// becoming empty flatBlocks (which would drop adjacent VLM context).
// A bare "text" block is not part of the current IR schema, but is
// passed through so its payload is never silently dropped. Returns ""
// for image and unknown types.
//
// For table elements, cellSep joins the cells within a single row. Pass
// "\n" to put each cell on its own line (the pptx/docx flatten behavior,
// equivalent to the previous flat-all-cells rendering), or " | " to keep a
// row's cells on one line (the legacy .doc viewport).
func docxElementText(el docxIRElement, cellSep string) string {
	switch el.Type {
	case "paragraph", "heading":
		return joinDOCXIRRuns(el.contentRuns())
	case "text":
		return el.Text
	case "table":
		var rows []string
		for _, row := range el.Rows {
			var cells []string
			for _, cell := range row.Cells {
				if t := joinCellText(cell); t != "" {
					cells = append(cells, t)
				}
			}
			rows = append(rows, strings.Join(cells, cellSep))
		}
		return strings.Join(rows, "\n")
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

// docxIRTableCols returns the number of cells in a table's first row, used
// to decide whether two adjacent table elements are fragments of one logical
// table. It returns 0 for an empty table.
func docxIRTableCols(el docxIRElement) int {
	if len(el.Rows) == 0 {
		return 0
	}
	return len(el.Rows[0].Cells)
}

// docxHeading is one entry of the heading outline stack.
type docxHeading struct {
	level int
	text  string
}

// pushDOCXHeading folds a heading into the outline stack: a heading at
// level L supersedes every entry at level >= L (sibling and deeper
// headings end), then becomes the innermost entry. Empty headings are
// ignored so they never truncate the outline.
func pushDOCXHeading(stack []docxHeading, level int, text string) []docxHeading {
	text = strings.TrimSpace(text)
	if text == "" {
		return stack
	}
	for len(stack) > 0 && stack[len(stack)-1].level >= level {
		stack = stack[:len(stack)-1]
	}
	return append(stack, docxHeading{level: level, text: text})
}

// docxHeadingLocationCaption renders the heading outline as a table
// location caption, mirroring Python's Docx.__get_nearest_title, which
// injected "<caption>Table Location: {hierarchy}</caption>" into the table
// HTML. It returns "" when the table sits under no heading.
func docxHeadingLocationCaption(headings []docxHeading) string {
	if len(headings) == 0 {
		return ""
	}
	titles := make([]string, len(headings))
	for i, h := range headings {
		titles[i] = h.text
	}
	return "Table Location: " + strings.Join(titles, " > ")
}

// buildDOCXJSONSections converts an office_oxide IR JSON string into a
// slice of structured items compatible with the chunker's JSON input
// contract. Each item carries at least text and doc_type_kwd.
//
// A table carries the heading hierarchy it sits under as its caption,
// mirroring Python's Docx.__get_nearest_title. office_oxide resolves Word
// heading styles into element type "heading" plus a level, so the outline
// is tracked from those elements alone.
//
// This is the caption-only (heading hierarchy) entry point. Use
// buildDOCXJSONSectionsWithCaptions to also attach Caption-styled paragraph
// text (a temporary stopgap; see docx_caption_scan.go).
func buildDOCXJSONSections(irJSON string, budget *embeddedMediaBudget) []map[string]any {
	return buildDOCXJSONSectionsCore(irJSON, budget, nil)
}

// buildDOCXJSONSectionsWithCaptions is buildDOCXJSONSections plus the
// TEMPORARY Caption-styled-paragraph detection. tableCaptions must be aligned
// by position with the tables in the IR (see docx_caption_scan.go).
//
// TEMPORARY: remove once office_oxide emits paragraph styles
// (yfedoseev/office_oxide#455).
func buildDOCXJSONSectionsWithCaptions(irJSON string, budget *embeddedMediaBudget, tableCaptions []string) []map[string]any {
	return buildDOCXJSONSectionsCore(irJSON, budget, tableCaptions)
}

// buildDOCXJSONSectionsCore is the shared implementation of the two entry
// points above. tableCaptions, when non-nil, supplies a descriptive caption
// for each table indexed by reading order; an explicit caption is preferred
// over the synthesized heading-location caption. The position alignment
// between tableCaptions and the IR tables is approximate (document-body order
// vs office_oxide IR order) and is a known limitation of the temporary
// Caption-style stopgap in docx_caption_scan.go.
func buildDOCXJSONSectionsCore(irJSON string, budget *embeddedMediaBudget, tableCaptions []string) []map[string]any {
	var ir docxIRDocument
	if err := json.Unmarshal([]byte(irJSON), &ir); err != nil {
		return nil
	}
	if budget == nil {
		budget = newEmbeddedMediaBudget()
	}
	var sections []map[string]any
	tableSequence := 0
	var headings []docxHeading
	for si := range ir.Sections {
		sec := &ir.Sections[si]
		for ei := 0; ei < len(sec.Elements); ei++ {
			el := sec.Elements[ei]
			// TEMPORARY: this merge is a stopgap. Revisit it once office_oxide
			// populates `TableRow.is_header` for .doc tables (upstream issue
			// yfedoseev/office_oxide#456) — at that point prefer rejoining via
			// the real `is_header` flag and drop the structural proxy below.
			//
			// office_oxide splits a .doc table whose first row is a styled
			// header/title row into a 1-row "header" table immediately
			// followed by the body table (the legacy binary .doc format
			// stores such a table as two separate Table objects with a blank
			// paragraph between them; walk_paragraphs flushes a new Element::Table
			// at every non-fInTable paragraph, and emit_prose drops the blank
			// one, so the two tables end up adjacent in the IR). Rejoin a
			// 1-row table fragment with the adjacent following table when their
			// first rows carry the same column count, so one logical table is
			// emitted as a single item instead of a header chunk + body chunk
			// (each would otherwise carry the same caption).
			//
			// Why a structural proxy instead of the row's `is_header` flag:
			// office_oxide's IR DOES carry `TableRow.is_header`, but the .doc
			// converter (convert_doc.rs) never populates it — every .doc row
			// arrives with is_header=false, verified on the real document that
			// triggered this bug. Only the .docx converter sets it, and .docx
			// never splits. So `is_header` is a dead signal for .doc and cannot
			// be used to confirm the leading fragment is a header or that the
			// following table lacks one. The 1-row gate is the strongest
			// available proxy: a 1-row table adjacent to a same-width table is
			// overwhelmingly a header fragment, because a genuinely standalone
			// 1-row table immediately followed by another table (with only a
			// blank paragraph between) is rare.
			//
			// Regression guards baked into the merge condition:
			//   - leading fragment is exactly 1 row: two genuinely separate
			//     MULTI-row tables (same width, blank paragraph between) are
			//     never merged.
			//   - same first-row column count: differently-shaped tables stay
			//     separate.
			//   - adjacency (no element between): any heading/paragraph/image
			//     in between keeps the two tables distinct.
			if el.Type == "table" && len(el.Rows) == 1 && ei+1 < len(sec.Elements) {
				if nxt := sec.Elements[ei+1]; nxt.Type == "table" &&
					docxIRTableCols(el) > 0 && docxIRTableCols(el) == docxIRTableCols(nxt) {
					merged := nxt
					rows := make([]docxIRRow, 0, len(nxt.Rows)+len(el.Rows))
					rows = append(rows, el.Rows...)
					rows = append(rows, nxt.Rows...)
					merged.Rows = rows
					el = merged
					ei++ // fold the following table into this one; skip it
				}
			}
			switch el.Type {
			case "paragraph", "heading":
				if el.Type == "heading" {
					level := el.Level
					if level <= 0 {
						level = 1
					}
					headings = pushDOCXHeading(headings, level, docxElementText(el, "\n"))
				}
				// TEMPORARY (docx_caption_scan.go): when a paragraph's text
				// exactly matches the Caption recovered for the next table,
				// suppress it as a standalone text section — it is attached
				// to that table as the caption instead. The text match keeps
				// this safe if the document scan and the IR element order
				// ever desync: a non-matching paragraph is never dropped.
				if tableSequence < len(tableCaptions) {
					if explicit := strings.TrimSpace(tableCaptions[tableSequence]); explicit != "" &&
						strings.TrimSpace(docxElementText(el, " ")) == explicit {
						continue
					}
				}
				sections = appendDOCXParagraphSections(sections, el, el.Type == "heading", budget)

			case "image":
				sections, _ = appendDOCXImageSection(sections, el.Data, budget, nil)

			case "table":
				tableSequence++
				if len(el.Rows) == 0 {
					continue
				}
				// Prefer an explicit Caption-styled paragraph (temporary
				// stopgap, see docx_caption_scan.go) over the synthesized
				// heading-location caption.
				caption := docxHeadingLocationCaption(headings)
				if idx := tableSequence - 1; idx >= 0 && idx < len(tableCaptions) {
					if explicit := strings.TrimSpace(tableCaptions[idx]); explicit != "" {
						caption = explicit
					}
				}
				html := docxIRTableToHTML(el, caption)
				table := map[string]any{
					"text":         html,
					"image":        nil,
					"doc_type_kwd": "table",
				}
				projectPDFTable(table)
				tableID := ""
				foundImage := false
				var tableImages []docxTableImage
				forEachDOCXTableImage(el, func(tableImage docxTableImage) bool {
					foundImage = true
					included, keepWalking := budget.include(tableImage.data)
					if included || keepWalking {
						tableImage.included = included
						tableImages = append(tableImages, tableImage)
					}
					return keepWalking
				})
				if foundImage {
					tableID = fmt.Sprintf("docx-table-%d", tableSequence)
					table["source_table_id"] = tableID
				}
				sections = append(sections, table)
				for mediaOrder, tableImage := range tableImages {
					metadata := map[string]any{
						"parent_table_id": tableID,
						"row_index":       tableImage.row,
						"column_index":    tableImage.column,
						"media_order":     mediaOrder + 1,
					}
					sections = appendDOCXImagePayload(sections, tableImage.data, tableImage.included, metadata)
				}

			case "list":
				for _, item := range el.Items {
					text := extractTextFromListItem(item)
					if text != "" {
						sections = append(sections, map[string]any{
							"text":         text,
							"image":        nil,
							"doc_type_kwd": "text",
						})
					}
					sections = appendDOCXListImageSections(sections, docxIRList{Items: []docxIRListItem{item}}, budget)
				}

			case "text_box":
				text := extractTextFromBlockElements(el.contentBlocks())
				if text != "" {
					sections = append(sections, map[string]any{
						"text":         text,
						"image":        nil,
						"doc_type_kwd": "text",
					})
				}
				sections = appendDOCXIRImageSections(sections, el.contentBlocks(), budget)
			}
		}
	}
	return sections
}

// appendDOCXParagraphSections emits text and inline-image runs in their IR
// order. A paragraph is not necessarily a single text unit: office_oxide can
// place an image between two text runs, and dropping that run would make the
// downstream DOCX chunker lose both media order and adjacency context.
func appendDOCXParagraphSections(sections []map[string]any, el docxIRElement, heading bool, budget *embeddedMediaBudget) []map[string]any {
	var text strings.Builder
	flushText := func() {
		value := strings.TrimSpace(text.String())
		text.Reset()
		if value == "" {
			return
		}
		item := map[string]any{
			"text":         value,
			"image":        nil,
			"doc_type_kwd": "text",
		}
		if heading {
			item["layout"] = "heading"
		}
		sections = append(sections, item)
	}
	appendImage := func(data []byte) {
		sections, _ = appendDOCXImageSection(sections, data, budget, nil)
	}
	for _, run := range el.contentRuns() {
		switch run.Type {
		case "text":
			text.WriteString(run.Text)
		case "line_break":
			text.WriteByte('\n')
		case "image":
			flushText()
			appendImage(run.Data)
			for _, nested := range run.Content {
				if nested.Type == "image" {
					appendImage(nested.Data)
				}
			}
		}
	}
	flushText()
	return sections
}

func appendDOCXImageSection(sections []map[string]any, data []byte, budget *embeddedMediaBudget, metadata map[string]any) ([]map[string]any, bool) {
	if len(data) == 0 {
		return sections, true
	}
	included, keepWalking := budget.include(data)
	if !included && !keepWalking {
		return sections, false
	}
	return appendDOCXImagePayload(sections, data, included, metadata), keepWalking
}

func appendDOCXListImageSections(sections []map[string]any, list docxIRList, budget *embeddedMediaBudget) []map[string]any {
	for _, item := range list.Items {
		sections = appendDOCXIRImageSections(sections, item.Content, budget)
		if item.Nested != nil {
			sections = appendDOCXListImageSections(sections, *item.Nested, budget)
		}
	}
	return sections
}

func appendDOCXIRImageSections(sections []map[string]any, elements []docxIRElement, budget *embeddedMediaBudget) []map[string]any {
	forEachDOCXIRImage(elements, func(data []byte) bool {
		var keepWalking bool
		sections, keepWalking = appendDOCXImageSection(sections, data, budget, nil)
		return keepWalking
	})
	return sections
}

func appendDOCXImagePayload(sections []map[string]any, data []byte, included bool, metadata map[string]any) []map[string]any {
	item := map[string]any{
		"text":         "",
		"image":        nil,
		"doc_type_kwd": "image",
	}
	if included {
		item["image"] = base64.StdEncoding.EncodeToString(data)
	} else {
		item["media_omitted"] = true
	}
	for key, value := range metadata {
		item[key] = value
	}
	return append(sections, item)
}

// --- figure extraction (used by the cgo parser path) ---

// extractDOCXFiguresFromIR parses the office_oxide IR JSON and
// returns the images admitted by budget together with the plain text
// immediately surrounding each image. The context matches what Python's
// naive_merge_docx attaches as context_above / context_below on
// each chunk that carries an image.
//
// Reuses the IR already obtained from the doc handle in
// ParseWithResult so the binary is not opened twice.
func extractDOCXFiguresFromIR(irJSON string, budget *embeddedMediaBudget) []DOCXFigure {
	var ir docxIRDocument
	if err := json.Unmarshal([]byte(irJSON), &ir); err != nil {
		return nil
	}
	if budget == nil {
		budget = newEmbeddedMediaBudget()
	}

	var flat []flatBlock
	for _, sec := range ir.Sections {
		for _, el := range sec.Elements {
			if el.Type == "image" {
				flat = append(flat, flatBlock{imageData: el.Data})
				continue
			}
			if el.Type == "paragraph" || el.Type == "heading" {
				flat = append(flat, docxInlineFlatBlocks(el)...)
				continue
			}
			text := docxElementText(el, "\n")
			flat = append(flat, flatBlock{text: text})
		}
	}

	var figures []DOCXFigure
	for i, block := range flat {
		if len(block.imageData) == 0 {
			continue
		}
		included, keepWalking := budget.include(block.imageData)
		if !included && !keepWalking {
			break
		}
		fig := DOCXFigure{}
		if included {
			fig.Image = base64.StdEncoding.EncodeToString(block.imageData)
		}

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
		// in the rendered Markdown for inline insertion.
		for j := i - 1; j >= 0; j-- {
			if flat[j].text != "" {
				fig.Marker = flat[j].text
				break
			}
		}

		figures = append(figures, fig)
		if !keepWalking {
			break
		}
	}
	return figures
}

// docxInlineFlatBlocks preserves the text/image sequence inside a paragraph
// for figure context extraction. Top-level element order alone is not enough
// for DOCX because office_oxide can encode an inline image as a paragraph run.
func docxInlineFlatBlocks(el docxIRElement) []flatBlock {
	var blocks []flatBlock
	var text strings.Builder
	flushText := func() {
		if text.Len() == 0 {
			return
		}
		blocks = append(blocks, flatBlock{text: text.String()})
		text.Reset()
	}
	appendImage := func(data []byte) {
		if len(data) == 0 {
			return
		}
		blocks = append(blocks, flatBlock{imageData: data})
	}
	for _, run := range el.contentRuns() {
		switch run.Type {
		case "text":
			text.WriteString(run.Text)
		case "line_break":
			text.WriteByte('\n')
		case "image":
			flushText()
			appendImage(run.Data)
			for _, nested := range run.Content {
				if nested.Type == "image" {
					appendImage(nested.Data)
				}
			}
		}
	}
	flushText()
	return blocks
}

// --- internal types ---

// flatBlock is a flattened IR element used internally to collect
// text / image context around embedded figures.
type flatBlock struct {
	text      string
	imageData []byte
}

const docxContextWindow = 512

func collectDOCXPrevText(flat []flatBlock, idx, maxLen int) string {
	var parts []string
	remaining := maxLen
	for i := idx - 1; i >= 0 && remaining > 0; i-- {
		if len(flat[i].imageData) > 0 {
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
		if len(flat[i].imageData) > 0 {
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
