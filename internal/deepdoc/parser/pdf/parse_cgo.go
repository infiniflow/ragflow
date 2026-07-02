//go:build cgo

package pdf

import (
	"context"
	"fmt"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// Parse runs the full PDF extraction pipeline from raw bytes.
// Creates and manages the PDF engine lifecycle internally.
func (p *Parser) Parse(ctx context.Context, data []byte, docAnalyzer pdf.DocAnalyzer) (*pdf.ParseResult, error) {
	engine, err := NewEngine(data)
	if err != nil {
		return nil, fmt.Errorf("pdfoxide.NewEngine: %w", err)
	}
	defer engine.Close()

	return p.ParseRaw(ctx, engine, docAnalyzer)
}
