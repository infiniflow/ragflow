//go:build !cgo

package pdf

import (
	"context"
	"fmt"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// Parse is a stub that returns an error when CGO (pdfium/pdf_oxide) is not available.
// The real implementation in parse_cgo.go requires CGO for the pdf_oxide PDF engine.
func (p *Parser) Parse(ctx context.Context, data []byte, docAnalyzer pdf.DocAnalyzer) (*pdf.ParseResult, error) {
	return nil, fmt.Errorf("pdfoxide/pdfium engine requires CGO: rebuild with CGO_ENABLED=1")
}
