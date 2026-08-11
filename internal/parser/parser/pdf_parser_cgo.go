//go:build cgo

package parser

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/common"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

func (p *PDFParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	if err := p.validateParseMethod(); err != nil {
		return ParseResult{Err: err}
	}
	common.Info(fmt.Sprintf("------------file: %s, parse_method: %s", filename, p.ParseMethod))
	switch normalizePDFParseMethod(p.ParseMethod) {
	case "plain_text":
		return parsePDFWithPlainText(filename, data, p)
	case "mineru":
		return parsePDFWithMinerU(ctx, filename, data, p)
	case "paddleocr":
		return parsePDFWithPaddleOCR(ctx, filename, data, p)
	case "docling":
		return parsePDFWithDocling(ctx, filename, data, p)
	case "opendataloader":
		return parsePDFWithOpenDataLoader(ctx, filename, data, p)
	case "somark":
		return parsePDFWithSoMark(filename, data, p)
	case "tcadp":
		return parsePDFWithTCADP(filename, data, p)
	}
	cfg := deepdoctype.DefaultParserConfig()
	cfg.SkipOCR = false
	cfg.Pages = p.Pages
	parser := deepdocpdf.NewParser(cfg)
	res := parsePDFWithDeepDocOptions(ctx, filename, data, pdfPostProcessOptions{
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
