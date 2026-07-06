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
	cfg := deepdoctype.DefaultParserConfig()
	cfg.SkipOCR = false
	parser := deepdocpdf.NewParser(cfg)
	res := parsePDFWithDeepDocOptions(context.Background(), filename, data, pdfPostProcessOptions{
		outputFormat:       p.OutputFormat,
		zoom:               cfg.Zoom,
		enableMultiColumn:  p.EnableMultiColumn,
		flattenMediaToText: p.FlattenMediaToText,
		removeTOC:          p.RemoveTOC,
		removeHeaderFooter: p.RemoveHeaderFooter,
	}, parser.Parse)
	if res.Err != nil && errors.Is(res.Err, deepdocpdf.ErrNoPDFData) {
		return ParseResult{Err: fmt.Errorf("%w: %s", ErrPDFEngineUnavailable, filename)}
	}
	if res.Err != nil && res.Err.Error() == "deepdoc/pdf: cgo required" {
		return ParseResult{Err: fmt.Errorf("%w: %s", ErrPDFEngineUnavailable, filename)}
	}
	return res
}
