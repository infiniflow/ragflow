//go:build cgo

package parser

import (
	"context"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

func (p *PDFParser) ParseWithResult(filename string, data []byte) ParseResult {
	cfg := deepdoctype.DefaultParserConfig()
	cfg.SkipOCR = false
	parser := deepdocpdf.NewParser(cfg)
	return parsePDFWithDeepDoc(context.Background(), filename, data, parser.Parse)
}
