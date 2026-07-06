//go:build cgo

package parser

import (
	"fmt"

	"ragflow/internal/deepdoc/parser/pdf/pdfoxide"
)

func parsePDFWithPlainText(filename string, data []byte, parser *PDFParser) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	doc, err := pdfoxide.OpenBytes(data)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: plain_text open: %w", err)}
	}
	defer doc.Close()

	pageCount, err := doc.PageCount()
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: plain_text page count: %w", err)}
	}
	items := make([]map[string]any, 0, pageCount)
	for page := 0; page < pageCount; page++ {
		text, err := doc.GetPageText(page)
		if err != nil {
			return ParseResult{Err: fmt.Errorf("parser: plain_text page %d: %w", page+1, err)}
		}
		items = append(items, map[string]any{
			"text":         text,
			"doc_type_kwd": "text",
			"page_number":  page + 1,
		})
	}
	return pdfItemsToResult(filename, items, parser.OutputFormat, pageCount)
}
