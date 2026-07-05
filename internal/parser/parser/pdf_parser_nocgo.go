//go:build !cgo

package parser

import (
	"fmt"
)

func (p *PDFParser) ParseWithResult(filename string, data []byte) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	return ParseResult{
		Err: fmt.Errorf("%w: %s", ErrPDFEngineUnavailable, filename),
	}
}
