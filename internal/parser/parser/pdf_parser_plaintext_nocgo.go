//go:build !cgo

package parser

import "fmt"

func parsePDFWithPlainText(filename string, data []byte, parser *PDFParser) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	return ParseResult{Err: fmt.Errorf("%w: %s", ErrPDFEngineUnavailable, filename)}
}
