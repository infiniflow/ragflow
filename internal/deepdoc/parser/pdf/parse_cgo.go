//go:build cgo

package pdf

import (
	"context"
	"fmt"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// Parse runs the full PDF extraction pipeline from raw bytes.
//
// Engine lifetime / ownership: Parse creates the native PDF engine and
// transfers ownership of it to the returned *pdf.ParseResult (result.Engine is
// populated inside ParseRaw/processPages). The engine is intentionally kept
// alive past Parse so the serialization step can still use it — most
// importantly the markdown path (cropMarkdownFigures) renders and crops figure
// images on demand. Parse must therefore NOT close the engine itself.
//
// The caller (the adapter layer: pdfParseResultToJSONWithOptions /
// pdfParseResultToMarkdownWithOptions) releases the engine via result.Close()
// once serialization is complete. The only place Parse closes the engine is on
// its own error return, since in that case no ParseResult is handed back for
// the caller to release.
func (p *Parser) Parse(ctx context.Context, data []byte, docAnalyzer pdf.DocAnalyzer) (*pdf.ParseResult, error) {
	engine, err := NewEngine(data)
	if err != nil {
		return nil, fmt.Errorf("pdfoxide.NewEngine: %w", err)
	}
	res, err := p.ParseRaw(ctx, engine, docAnalyzer)
	if err != nil {
		_ = engine.Close()
		return nil, err
	}
	return res, nil
}
