//go:build cgo

package parser

import (
	"context"
	"errors"
	"fmt"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

func (p *PDFParser) ParseWithResult(filename string, data []byte) ParseResult {
	cfg := deepdoctype.DefaultParserConfig()
	cfg.SkipOCR = false
	parser := deepdocpdf.NewParser(cfg)
	res := parsePDFWithDeepDoc(context.Background(), filename, data, parser.Parse)
	if res.Err != nil && errors.Is(res.Err, deepdocpdf.ErrNoPDFData) {
		return ParseResult{Err: fmt.Errorf("%w: %s", ErrPDFEngineUnavailable, filename)}
	}
	if res.Err != nil && res.Err.Error() == "deepdoc/pdf: cgo required" {
		return ParseResult{Err: fmt.Errorf("%w: %s", ErrPDFEngineUnavailable, filename)}
	}
	return res
}
