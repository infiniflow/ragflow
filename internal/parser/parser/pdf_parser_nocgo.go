//go:build !cgo

package parser

import (
	"fmt"
)

func (p *PDFParser) ParseWithResult(filename string, data []byte) ParseResult {
	if err := p.validateParseMethod(); err != nil {
		return ParseResult{Err: err}
	}
	switch normalizePDFParseMethod(p.ParseMethod) {
	case "plain_text":
		return parsePDFWithPlainText(filename, data, p)
	case "mineru":
		return parsePDFWithMinerU(filename, data, p)
	case "paddleocr":
		return parsePDFWithPaddleOCR(filename, data, p)
	case "docling":
		return parsePDFWithDocling(filename, data, p)
	case "opendataloader":
		return parsePDFWithOpenDataLoader(filename, data, p)
	case "somark":
		return parsePDFWithSoMark(filename, data, p)
	case "tcadp":
		return parsePDFWithTCADP(filename, data, p)
	}
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	return ParseResult{
		Err: fmt.Errorf("%w: %s", ErrPDFEngineUnavailable, filename),
	}
}
