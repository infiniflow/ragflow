//go:build !cgo

package pdf

import (
	"context"
	"fmt"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// Parse is the no-CGO stub for the DeepDOC PDF pipeline. The surrounding
// parser package converts this into parser.ErrPDFEngineUnavailable.
func (p *Parser) Parse(ctx context.Context, data []byte, docAnalyzer pdf.DocAnalyzer) (*pdf.ParseResult, error) {
	_ = ctx
	_ = data
	_ = docAnalyzer
	return nil, fmt.Errorf("deepdoc/pdf: cgo required")
}
