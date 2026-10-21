package parser

import (
	"context"
	"maps"
	"math"
	"strings"

	"golang.org/x/net/html"

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
		return ParseResult{Err: err}
	}
	if len(result.JSON) == 0 {
		switch {
		case result.Markdown != "":
			parsed := parsePDFMarkdown(ctx, filename, result.Markdown)
			if parsed.Err != nil {
				return ParseResult{Err: parsed.Err}
			}
			result.Warnings = append(result.Warnings, parsed.Warnings...)
			if len(parsed.JSON) == 0 {
				result.JSON = []DocItem{NewTextJSONItem(result.Markdown)}
			} else {
				result.JSON = parsed.JSON
			}
		case result.Text != "":
			result.JSON = []DocItem{NewTextJSONItem(result.Text)}
		}
	}
	items := make([]DocItem, 0, len(result.JSON))
	for _, item := range result.JSON {
		if err := ctx.Err(); err != nil {
			return ParseResult{Err: err}
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
		positions := validPDFPositions(fields["_pdf_positions"])
		if len(positions) == 0 {
			positions = validPDFPositions(fields["positions"])
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
			if table, ok := fields["table"].(*entity.TableData); ok && table != nil {
				text, _ := fields["text"].(string)
				fields["text"] = strings.TrimSpace(tableutil.RenderTableHTML(table) + "\n" + text)
				delete(fields, "table")
			}
			fields["doc_type_kwd"] = "text"
		}
		if fields["doc_type_kwd"] == "text" && (fields["ck_type"] == "table" || fields["ck_type"] == "image") {
			delete(fields, "ck_type")
		}
		items = appendPDFItem(items, fields)
	}
	result.JSON = items
	result.OutputFormat, result.Markdown, result.Text = "json", "", ""
	result.File = maps.Clone(result.File)
	if result.File == nil {
		result.File = map[string]any{}
	}
	if _, ok := result.File["name"]; !ok {
		result.File["name"] = filename
	}
	return result
}

// appendPDFItem emits adjacent tables separately and keeps their descriptions
// in text items. Chunkers materialize a table only when its text field is empty.
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
		note := TextItem{Text: text, Extra: map[string]any{"layout": "text"}}
		for _, key := range []string{"page_number", "_preserve_order"} {
			if value, ok := fields[key]; ok {
				note.Extra[key] = value
			}
		}
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
func validPDFPositions(raw any) [][]any {
	var rows []any
	switch v := raw.(type) {
	case [][]any:
		for _, row := range v {
			rows = append(rows, row)
		}
	case []any:
		rows = v
	case [][]float64:
		for _, row := range v {
			rows = append(rows, row)
		}
	}
	var result [][]any
	for _, rawRow := range rows {
		row, ok := rawRow.([]any)
		if values, floats := rawRow.([]float64); floats {
			row = make([]any, len(values))
			for i, v := range values {
				row[i] = v
			}
			ok = true
		}
		if !ok || len(row) != 5 {
			continue
		}
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
		if !valid || page < 1 || page >= float64(^uint(0)>>1) || math.Trunc(page) != page || values[2] <= values[1] || values[4] <= values[3] {
			continue
		}
		result = append(result, []any{int(page), values[1], values[2], values[3], values[4]})
	}
	return result
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
