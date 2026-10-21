package parser

import (
	"context"
	"maps"
	"math"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/net/html"

	"ragflow/internal/common"
	"ragflow/internal/deepdoc/parser/pdf/util"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

// NormalizeResult converts a PDF backend response to JSON and applies common
// item policies. Producers own page-base and coordinate-unit conversion.
func (p *PDFParser) NormalizeResult(ctx context.Context, filename string, result ParseResult) ParseResult {
	if result.Err != nil {
		return result
	}
	if err := ctx.Err(); err != nil {
		result.Err = err
		result.File = ensurePDFResultFile(result.File, filename)
		return result
	}
	if len(result.JSON) == 0 {
		switch {
		case result.Markdown != "":
			parsed := parsePDFMarkdown(ctx, filename, result.Markdown)
			if parsed.Err != nil {
				if ctx.Err() != nil {
					// Cancellation is fatal; preserve file metadata for callers.
					parsed.File = ensurePDFResultFile(result.File, filename)
					return parsed
				}
				// Mirror the historical markdown→JSON fallback: a backend
				// markdown payload that cannot be structured is still indexed
				// as raw text instead of failing the task.
				common.Warn("parser normalization fell back to text",
					zap.String("filename", filename),
					zap.String("normalized_from", "markdown"),
					zap.Error(parsed.Err))
				result.Warnings = append(result.Warnings, parsed.Warnings...)
				result.Warnings = append(result.Warnings, "markdown normalization fell back to text: "+parsed.Err.Error())
				result.JSON = []DocItem{NewTextJSONItem(result.Markdown)}
			} else {
				result.Warnings = append(result.Warnings, parsed.Warnings...)
				if len(parsed.JSON) == 0 {
					result.JSON = []DocItem{NewTextJSONItem(result.Markdown)}
				} else {
					result.JSON = parsed.JSON
				}
			}
		case result.Text != "":
			result.JSON = []DocItem{NewTextJSONItem(result.Text)}
		}
	}
	items := make([]DocItem, 0, len(result.JSON))
	droppedPositions := 0
	for _, item := range result.JSON {
		if err := ctx.Err(); err != nil {
			result.Err = err
			result.File = ensurePDFResultFile(result.File, filename)
			return result
		}
		if item == nil {
			continue
		}
		fields := maps.Clone(item.ToMap())
		if fields == nil {
			continue
		}
		layout, _ := fields["layout"].(string)
		if strings.TrimSpace(layout) == "" {
			layout, _ = fields["layout_type"].(string)
		}
		if layout = strings.ToLower(strings.TrimSpace(layout)); layout != "" {
			fields["layout"] = layout
			if _, ok := fields["layout_type"]; ok {
				fields["layout_type"] = layout
			}
		}
		rawPrimary := fields["_pdf_positions"]
		positions := validPDFPositions(rawPrimary)
		droppedPositions += countPDFPositionRows(rawPrimary) - len(positions)
		if len(positions) == 0 {
			rawFallback := fields["positions"]
			positions = validPDFPositions(rawFallback)
			droppedPositions += countPDFPositionRows(rawFallback) - len(positions)
		}
		delete(fields, "_pdf_positions")
		delete(fields, "positions")
		if len(positions) > 0 {
			fields["_pdf_positions"], fields["positions"] = positions, positions
			if _, ok := fields["page_number"]; !ok {
				fields["page_number"] = positions[0][0]
			}
		}
		normalizePDFDocType(fields)
		projectPDFTable(fields)
		if table, ok := fields["table"].(*entity.TableData); ok && table != nil {
			if page := int(numberValue(fields["page_number"])); page > 0 && page != table.Page {
				normalizedTable := *table
				normalizedTable.Page = page
				fields["table"] = &normalizedTable
			}
		} else if fields["doc_type_kwd"] == "table" && len(positions) == 0 {
			if text, _ := fields["text"].(string); text != "" {
				fields["doc_type_kwd"] = "text"
			}
		}
		if p.FlattenMediaToText {
			// Flattened output keeps one text carrier per item. Structured
			// tables (MinerU content_list, projected HTML) carry cells in
			// `table` and footnotes in `text`; rendering the table preserves
			// cells, caption, and footnote in that order. The render
			// normalizes markup (header inference, escaping, whitespace), so
			// it is not byte-identical to the source HTML, but it is
			// content-preserving. Items without a table keep their text verbatim.
			if table, ok := fields["table"].(*entity.TableData); ok && table != nil {
				text, _ := fields["text"].(string)
				fields["text"] = strings.TrimSpace(tableutil.RenderTableHTML(table) + "\n" + text)
				delete(fields, "table")
			}
			fields["doc_type_kwd"] = "text"
		}
		// ck_type is retired: the content type lives on doc_type_kwd and the
		// block kind on layout. A legacy raw item can still carry the key, so
		// drop it here as well — the typed conversion already does.
		delete(fields, "ck_type")
		items = appendPDFItem(items, fields)
	}
	if droppedPositions > 0 {
		// Zero-area or otherwise invalid boxes are dropped so they never reach
		// the on-demand cropper. Log the count so a parser that stops emitting
		// regions is distinguishable from one that never emitted any.
		common.Warn("PDF normalization dropped invalid positions",
			zap.String("filename", filename),
			zap.Int("dropped_rows", droppedPositions))
	}
	result.JSON = items
	result.OutputFormat, result.Markdown, result.Text = "json", "", ""
	result.File = ensurePDFResultFile(result.File, filename)
	return result
}

// ensurePDFResultFile preserves backend file metadata while guaranteeing the
// filename is present, including on error paths where callers still need to
// report which document failed.
func ensurePDFResultFile(file map[string]any, filename string) map[string]any {
	file = maps.Clone(file)
	if file == nil {
		file = map[string]any{}
	}
	if _, ok := file["name"]; !ok {
		file["name"] = filename
	}
	return file
}

// NewTableNoteItem builds the text item that carries a table's description.
// The note keeps the table's page identity and ordering hint so downstream
// chunking preserves document order. Shared by NormalizeResult's table
// splitting and the vision enhancement path.
func NewTableNoteItem(fields map[string]any, text string) TextItem {
	note := TextItem{Text: text, Extra: map[string]any{"layout": "text"}}
	for _, key := range []string{"page_number", "_preserve_order"} {
		if value, ok := fields[key]; ok {
			note.Extra[key] = value
		}
	}
	return note
}

// appendPDFItem emits adjacent tables separately and keeps their descriptions
// in text items. Chunkers materialize a table only when its text field is empty.
// The isTableHTML branch handles one text carrier holding two adjacent tables
// plus a trailing note (projectPDFTable only projects the leading table, so the
// remainder can itself start with a table). Covered by
// TestPDFNormalizeResultPreservesAdjacentTables.
func appendPDFItem(items []DocItem, fields map[string]any) []DocItem {
	for {
		table, _ := fields["table"].(*entity.TableData)
		text, _ := fields["text"].(string)
		if fields["doc_type_kwd"] != "table" || table == nil || text == "" {
			return append(items, DocItemFromMap(fields))
		}
		fields = maps.Clone(fields)
		delete(fields, "text")
		items = append(items, DocItemFromMap(fields))
		if strings.TrimSpace(text) == "" {
			return items
		}
		if isTableHTML(text) {
			next := maps.Clone(fields)
			delete(next, "table")
			next["text"] = text
			projectPDFTable(next)
			if table, _ := next["table"].(*entity.TableData); table != nil {
				fields = next
				continue
			}
		}
		note := NewTableNoteItem(fields, text)
		return append(items, note)
	}
}

func parsePDFMarkdown(ctx context.Context, filename, markdown string) ParseResult {
	p, err := NewMarkdownParser(GoMarkdown)
	if err != nil {
		return ParseResult{Err: err}
	}
	p.FetchRemoteImages = false
	return p.ParseWithResult(ctx, filename, []byte(markdown))
}

// validPDFPositions validates canonical 1-based PDF positions without changing
// page identity or units. Invalid regions must never reach the cropper.
//
// Every wire shape is decoded through PDFPositionRows: a matrix of 5-tuples
// ([][]any, []any of rows, [][]float64) and the legacy flat 5-tuple
// ([]float64) some engines still emit.
//
// A page-only tuple — a page number with all-zero geometry — is valid: it
// carries page identity for the indexer but no crop region, so the crop
// consumers skip it (see HasPDFCropRegion). A box with real coordinates that is
// zero-area or inverted is invalid and is dropped; the caller logs the dropped
// count so the loss is observable instead of silent.
func validPDFPositions(raw any) [][]any {
	var result [][]any
	for _, row := range PDFPositionRows(raw) {
		var values [5]float64
		valid := true
		for i, value := range row {
			v, ok := numericAny(value)
			if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
				valid = false
				break
			}
			values[i] = v
		}
		page := values[0]
		if !valid || page < 1 || page >= float64(^uint(0)>>1) || math.Trunc(page) != page {
			continue
		}
		if !pdfPositionIsUsable(values) {
			continue
		}
		result = append(result, []any{int(page), values[1], values[2], values[3], values[4]})
	}
	return result
}

// pdfPositionIsPageOnly reports whether a position carries page identity only:
// a page number with all-zero geometry and therefore no crop region.
func pdfPositionIsPageOnly(values [5]float64) bool {
	return values[1] == 0 && values[2] == 0 && values[3] == 0 && values[4] == 0
}

// pdfPositionIsUsable reports whether a position may be kept. A page-only
// tuple carries page identity and is kept; every other box must have a
// positive width and height, so a zero-area or inverted region is dropped.
func pdfPositionIsUsable(values [5]float64) bool {
	if pdfPositionIsPageOnly(values) {
		return true
	}
	return values[2] > values[1] && values[4] > values[3]
}

// pdfPositionHasRegion reports whether a position carries a usable crop region.
// A page-only tuple carries page identity only, so it is not a region.
func pdfPositionHasRegion(values [5]float64) bool {
	if pdfPositionIsPageOnly(values) {
		return false
	}
	return values[2] > values[1] && values[4] > values[3]
}

// HasPDFCropRegion reports whether the item carries at least one PDF crop
// region, i.e. a position with a positive-area box. A page-only position
// carries page identity but no region, so it must not trigger a crop.
//
// The crop consumers (the VLM vision path and the chunker) consult this
// predicate so a page-only position never reaches the cropper, while the
// indexer still receives the page identity through the positions matrix. The
// geometry is decoded exactly as the cropper decodes it, so the two agree on
// what a usable region is.
func HasPDFCropRegion(item map[string]any) bool {
	matrix, ok := ExtractPDFPositions(item)
	if !ok {
		return false
	}
	for _, pos := range util.PositionsFromMatrix(matrix) {
		if pos.Right > pos.Left && pos.Bottom > pos.Top {
			return true
		}
	}
	return false
}

// countPDFPositionRows counts candidate position rows before validation so the
// caller can report how many were dropped. It uses the same decoder as
// validPDFPositions, so the count matches the rows validation sees.
func countPDFPositionRows(raw any) int {
	return len(PDFPositionRows(raw))
}

// splitPDFTable separates the leading table from any following description.
// Tokenization handles nested tables without discarding the trailing text.
func splitPDFTable(text string) (string, string) {
	text = strings.TrimSpace(text)
	if !isTableHTML(text) {
		return "", text
	}
	z := html.NewTokenizer(strings.NewReader(text))
	depth, offset := 0, 0
	for {
		kind := z.Next()
		if kind == html.ErrorToken {
			if depth > 0 {
				return text, ""
			}
			return "", text
		}
		offset += len(z.Raw())
		if kind != html.StartTagToken && kind != html.EndTagToken {
			continue
		}
		name, _ := z.TagName()
		if string(name) != "table" {
			continue
		}
		if kind == html.StartTagToken {
			depth++
		} else {
			depth--
		}
		if depth == 0 {
			return text[:offset], strings.TrimSpace(text[offset:])
		}
	}
}

func normalizePDFDocType(item map[string]any) {
	if item == nil {
		return
	}
	layoutType, _ := item["layout_type"].(string)
	if layoutType == "" {
		layoutType, _ = item["layout"].(string)
	}
	// A figure caption is a media section even when the parser no longer
	// inlines its cropped image (cgo): it still carries PDF positions, so the
	// downstream VLM/chunker crop it on demand. Classify it as image whenever
	// it has positions (the inlined image was only a side effect of cropping).
	_, hasMedia := ExtractPDFPositions(item)
	if docType, _ := item["doc_type_kwd"].(string); docType == "text" || docType == "image" || docType == "table" {
		// A figure caption keeps its media classification so the downstream
		// VLM enhancement and on-demand chunker crop it.
		if docType == "text" && layoutType == deepdoctype.DLALabelFigureCaption && hasMedia {
			item["doc_type_kwd"] = "image"
		}
		return
	}
	if table, ok := item["table"].(*entity.TableData); ok && table != nil {
		item["doc_type_kwd"] = "table"
		return
	}
	switch layoutType {
	case "table":
		item["doc_type_kwd"] = "table"
	case "figure", "image":
		item["doc_type_kwd"] = "image"
	case deepdoctype.DLALabelFigureCaption:
		if hasMedia {
			item["doc_type_kwd"] = "image"
		} else {
			item["doc_type_kwd"] = "text"
		}
	default:
		if img, _ := item["image"].(string); img != "" {
			item["doc_type_kwd"] = "image"
			return
		}
		item["doc_type_kwd"] = "text"
	}
}

// projectPDFTable projects a leading HTML table into TableData, retaining any
// following description and leaving existing structured tables untouched.
func projectPDFTable(item map[string]any) {
	if item == nil {
		return
	}
	if dt, _ := item["doc_type_kwd"].(string); dt != "table" {
		return
	}
	if item["table"] != nil {
		return
	}
	text, _ := item["text"].(string)
	markup, notes := splitPDFTable(text)
	if markup == "" {
		return
	}
	td, err := tableutil.ParseTableHTML(markup)
	if err != nil || td == nil {
		return
	}
	if page := numberValue(item["page_number"]); page > 0 {
		td.Page = int(page)
	}
	item["table"] = td
	if notes == "" {
		delete(item, "text")
	} else {
		item["text"] = notes
	}
}
