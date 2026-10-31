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
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

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

// officeIRTableData projects native Office rows directly onto the table contract.
func officeIRTableData(el docxIRElement, caption string) *entity.TableData {
	rows := make([][]tableutil.CellInput, 0, len(el.Rows))
	for _, row := range el.Rows {
		cells := make([]tableutil.CellInput, 0, len(row.Cells))
		for _, cell := range row.Cells {
			cells = append(cells, tableutil.CellInput{Text: joinCellText(cell)})
		}
		rows = append(rows, cells)
	}
	return tableutil.BuildTableData(rows, nil, strings.TrimSpace(caption))
}

type docxTableImage struct {
	data   []byte
	ref    *entity.MediaRef
	row    int
	column int
}

func forEachDOCXTableImage(el docxIRElement, visit func(docxTableImage) bool) bool {
	for rowIndex, row := range el.Rows {
		for columnIndex, cell := range row.Cells {
			if !forEachDOCXIRImage(cell.Content, func(data []byte) bool {
				return visit(docxTableImage{data: data, row: rowIndex + 1, column: columnIndex + 1})
			}) {
				return false
			}
		}
	}
	return true
}

// forEachDOCXIRImage stops when a visitor reports a fatal failure. Resource
// omissions keep walking so later text and repeated admitted images survive.
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
			if !forEachDOCXTableImage(el, func(image docxTableImage) bool { return visit(image.data) }) {
				return false
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
		if item.Nested != nil && !forEachDOCXIRListImage(*item.Nested, visit) {
			return false
		}
	}
	return true
}

// docxElementText returns the plain-text rendering of any supported
// IR element type. Used by buildPPTXJSONSections to flatten one slide
// section. A bare "text" block is not part of the current IR schema, but is
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
//
// A nil sink means a text-only parse: embedded images are skipped and no
// media cache is required. Normal DOCX/DOC parsing always passes a sink.
func buildDOCXJSONSections(irJSON string, sink *officeMediaSink) []map[string]any {
	return buildDOCXJSONSectionsCore(irJSON, sink, nil)
}

// buildDOCXJSONSectionsWithCaptions is buildDOCXJSONSections plus the
// TEMPORARY Caption-styled-paragraph detection. tableCaptions must be aligned
// by position with the tables in the IR (see docx_caption_scan.go).
//
// TEMPORARY: remove once office_oxide emits paragraph styles
// (yfedoseev/office_oxide#455).
func buildDOCXJSONSectionsWithCaptions(irJSON string, sink *officeMediaSink, tableCaptions []string) []map[string]any {
	return buildDOCXJSONSectionsCore(irJSON, sink, tableCaptions)
}

// buildDOCXJSONSectionsCore is the shared implementation of the two entry
// points above. tableCaptions, when non-nil, supplies a descriptive caption
// for each table indexed by reading order; an explicit caption is preferred
// over the synthesized heading-location caption. The position alignment
// between tableCaptions and the IR tables is approximate (document-body order
// vs office_oxide IR order) and is a known limitation of the temporary
// Caption-style stopgap in docx_caption_scan.go.
func buildDOCXJSONSectionsCore(irJSON string, sink *officeMediaSink, tableCaptions []string) []map[string]any {
	var ir docxIRDocument
	if err := json.Unmarshal([]byte(irJSON), &ir); err != nil {
		return nil
	}
	var sections []map[string]any
	tableSequence := 0
	captionSequence := 0
	var headings []docxHeading
	var emitElements func([]docxIRElement, bool)
	emitElements = func(elements []docxIRElement, topLevel bool) {
		if !topLevel {
			parentHeadings := headings
			headings = append([]docxHeading(nil), headings...)
			defer func() { headings = parentHeadings }()
		}
		for ei := 0; ei < len(elements); ei++ {
			if sink != nil && sink.err != nil {
				return
			}
			el := elements[ei]
			// TEMPORARY: this merge is a stopgap. Revisit it once docxIRRow
			// carries a three-state header field and office_oxide populates
			// `TableRow.is_header` for .doc tables (upstream issue
			// yfedoseev/office_oxide#456) — at that point prefer rejoining via
			// the real header flag and drop the structural proxy below.
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
			if topLevel && el.Type == "table" && len(el.Rows) == 1 && ei+1 < len(elements) {
				if nxt := elements[ei+1]; nxt.Type == "table" &&
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
				if topLevel && captionSequence < len(tableCaptions) {
					if explicit := strings.TrimSpace(tableCaptions[captionSequence]); explicit != "" &&
						strings.TrimSpace(docxElementText(el, " ")) == explicit {
						continue
					}
				}
				sections = appendDOCXParagraphSections(sections, el, el.Type == "heading", sink)

			case "image":
				sections = appendDOCXImageSection(sections, el.Data, sink, nil)

			case "table":
				tableSequence++
				if topLevel {
					captionSequence++
				}
				if len(el.Rows) == 0 {
					continue
				}
				// Prefer an explicit Caption-styled paragraph (temporary
				// stopgap, see docx_caption_scan.go) over the synthesized
				// heading-location caption.
				caption := docxHeadingLocationCaption(headings)
				if idx := captionSequence - 1; topLevel && idx >= 0 && idx < len(tableCaptions) {
					if explicit := strings.TrimSpace(tableCaptions[idx]); explicit != "" {
						caption = explicit
					}
				}
				sections = appendOfficeTableSections(sections, el, caption, fmt.Sprintf("docx-table-%d", tableSequence), sink)

			case "list":
				for _, item := range el.Items {
					emitElements(item.Content, false)
					if item.Nested != nil {
						emitElements([]docxIRElement{{Type: "list", Items: item.Nested.Items}}, false)
					}
				}
			case "text_box":
				emitElements(el.contentBlocks(), false)
			}
		}
	}
	for _, sec := range ir.Sections {
		emitElements(sec.Elements, true)
	}
	return sections
}

// appendOfficeTableSections emits a table and its embedded images together.
// Images skipped by the media sink are not emitted; the table and its text
// remain so a cache-limit omission never loses surrounding content.
func appendOfficeTableSections(sections []map[string]any, el docxIRElement, caption, tableID string, sink *officeMediaSink) []map[string]any {
	table := NewTableDataJSONItem(officeIRTableData(el, caption), "", nil).ToMap()

	var tableImages []docxTableImage
	forEachDOCXTableImage(el, func(tableImage docxTableImage) bool {
		ref, ok := sink.put(tableImage.data, "")
		if !ok {
			return sink == nil || sink.err == nil
		}
		tableImage.ref = ref
		tableImages = append(tableImages, tableImage)
		return true
	})
	if len(tableImages) > 0 {
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
		sections = appendDOCXImagePayload(sections, tableImage.ref, metadata)
	}

	return sections
}

// appendDOCXParagraphSections emits text and inline-image runs in their IR
// order. A paragraph is not necessarily a single text unit: office_oxide can
// place an image between two text runs, and dropping that run would make the
// downstream DOCX chunker lose both media order and adjacency context.
func appendDOCXParagraphSections(sections []map[string]any, el docxIRElement, heading bool, sink *officeMediaSink) []map[string]any {
	var text strings.Builder
	flushText := func() {
		value := strings.TrimSpace(text.String())
		text.Reset()
		if value == "" {
			return
		}
		item := map[string]any{
			"text":         value,
			"doc_type_kwd": "text",
		}
		if heading {
			item["layout"] = "heading"
		}
		sections = append(sections, item)
	}
	appendImage := func(data []byte) {
		sections = appendDOCXImageSection(sections, data, sink, nil)
	}
	for _, run := range el.contentRuns() {
		if sink != nil && sink.err != nil {
			return sections
		}
		switch run.Type {
		case "text":
			text.WriteString(run.Text)
		case "line_break":
			text.WriteByte('\n')
		case "image":
			flushText()
			appendImage(run.Data)
		}
		if !forEachDOCXIRImage(run.Content, func(data []byte) bool { flushText(); appendImage(data); return sink == nil || sink.err == nil }) {
			return sections
		}
	}
	flushText()
	return sections
}

func appendDOCXImageSection(sections []map[string]any, data []byte, sink *officeMediaSink, metadata map[string]any) []map[string]any {
	ref, ok := sink.put(data, "")
	if !ok {
		return sections
	}
	return appendDOCXImagePayload(sections, ref, metadata)
}

func appendDOCXImagePayload(sections []map[string]any, ref *entity.MediaRef, metadata map[string]any) []map[string]any {
	item := map[string]any{
		"text":         "",
		"doc_type_kwd": "image",
		"media_ref":    ref,
	}
	for key, value := range metadata {
		item[key] = value
	}
	return append(sections, item)
}
