//go:build !cgo

package parser

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

func parsePDFWithPlainText(filename string, data []byte, parser *PDFParser) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}

	reader := bytes.NewReader(data)
	pdfReader, err := pdf.NewReader(reader, int64(len(data)))
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: plain_text open: %w", err)}
	}

	pageCount := pdfReader.NumPage()
	items := make([]map[string]any, 0, pageCount)
	for pageNum := 1; pageNum <= pageCount; pageNum++ {
		p := pdfReader.Page(pageNum)
		if p.V.IsNull() {
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			return ParseResult{Err: fmt.Errorf("parser: plain_text page %d: %w", pageNum, err)}
		}
		items = append(items, map[string]any{
			"text":         strings.TrimRight(text, "\n"),
			"doc_type_kwd": "text",
			"page_number":  pageNum,
		})
	}
	return pdfItemsToResult(filename, items, parser.OutputFormat, pageCount)
}
