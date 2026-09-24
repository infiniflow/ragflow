package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"ragflow/internal/deepdoc/parser/pdf/table"
	"strings"
)

// parsePDFWithTCADP sends PDF binary data to the TCADP cloud reconstruction
// service. Thin wrapper over the shared parseWithTCADP core — env-fallbacks
// and request construction live there.
func parsePDFWithTCADP(filename string, data []byte, parser *PDFParser) ParseResult {
	return parseWithTCADP(context.Background(), filename, data, "PDF",
		parser.TCADPAPIServer, parser.TCADPAPIKey,
		parser.TCADPTableResultType, parser.TCADPMarkdownImageResponseType,
		parser.OutputFormat)
}

func tcadpItemsFromZip(zipBytes []byte) ([]map[string]any, int, error) {
	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, 0, fmt.Errorf("parser: TCADP zip: %w", err)
	}
	items := make([]map[string]any, 0)
	pageCount := 0
	for _, file := range reader.File {
		if strings.HasSuffix(file.Name, ".md") {
			rc, err := file.Open()
			if err != nil {
				return nil, 0, err
			}
			body, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, 0, err
			}
			items = append(items, map[string]any{"text": strings.TrimSpace(string(body)), "doc_type_kwd": "text", "layout": "text"})
			if strings.TrimSpace(string(body)) != "" && pageCount == 0 {
				pageCount = 1
			}
			continue
		}
		if !strings.HasSuffix(file.Name, ".json") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, 0, err
		}
		var raw any
		err = json.NewDecoder(rc).Decode(&raw)
		rc.Close()
		if err != nil {
			return nil, 0, err
		}
		items = append(items, tcadpAnyToItems(raw)...)
		if pages := collectPDFPageNumbers(raw); len(pages) > pageCount {
			pageCount = len(pages)
		}
	}
	if len(items) == 0 {
		return nil, 0, fmt.Errorf("parser: TCADP zip contained no supported content")
	}
	return items, pageCount, nil
}

func tcadpAnyToItems(raw any) []map[string]any {
	switch v := raw.(type) {
	case []any:
		items := make([]map[string]any, 0)
		for _, item := range v {
			items = append(items, tcadpAnyToItems(item)...)
		}
		return items
	case map[string]any:
		text := strings.TrimSpace(stringValue(v["content"]))
		contentType := strings.ToLower(strings.TrimSpace(stringValue(v["type"])))
		page := extractTCADPPage(v)
		emit := func(text, docType, layout string) []map[string]any {
			m := map[string]any{"text": text, "doc_type_kwd": docType, "layout": layout}
			if page > 0 {
				// 1-indexed 5-tuple. AddPositions is a passthrough so
				// the final position_int / page_num_int carry the same
				// 1-indexed page number the caller passes. Mirrors
				// Python presentation.py:148-149.
				m["positions"] = []float64{float64(page), 0, 0, 0, 0}
			}
			return []map[string]any{m}
		}
		switch contentType {
		case "table":
			if rows := tcadpTableRows(v["table_data"]); rowsHaveText(rows) {
				text = table.SimpleRowsToHTML(rows)
			}
			if text == "" {
				return nil
			}
			return emit(text, pdfTableDocType(text), "table")
		case "image":
			caption := strings.TrimSpace(stringValue(v["caption"]))
			if caption == "" {
				caption = "[Image]"
			}
			return emit(caption, "image", "figure")
		case "equation":
			if text == "" {
				return nil
			}
			return emit("$$"+text+"$$", "text", "equation")
		default:
			if text == "" {
				return nil
			}
			return emit(text, "text", "text")
		}
	}
	return nil
}

// extractTCADPPage returns the 1-indexed page number carried by a raw TCADP
// element, using the same key set collectPDFPageNumbers walks
// (pdf_parser_remote_common.go). It returns 0 when the element has no page
// information (e.g. spreadsheet TCADP), so callers can skip position emission
// and remain parity-correct with Python (table.py sets no page either).
func extractTCADPPage(v map[string]any) int {
	for _, key := range []string{"page_number", "page_num", "page_no", "page_index", "page_idx", "page"} {
		if page := int(numberValue(v[key])); page > 0 {
			return page
		}
	}
	return 0
}

func tcadpTableRows(raw any) [][]string {
	data, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	rawRows, ok := data["rows"].([]any)
	if !ok {
		return nil
	}
	rows := make([][]string, 0, len(rawRows))
	for _, rowRaw := range rawRows {
		row, ok := rowRaw.([]any)
		if !ok {
			continue
		}
		cols := make([]string, 0, len(row))
		for _, col := range row {
			cols = append(cols, stringValue(col))
		}
		rows = append(rows, cols)
	}
	return rows
}

func bearer(apiKey string) string {
	if strings.TrimSpace(apiKey) == "" {
		return ""
	}
	return "Bearer " + apiKey
}
