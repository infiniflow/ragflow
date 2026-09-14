package parser

import (
	"context"
	"fmt"
	"ragflow/internal/common"
	"strings"
)

func parsePDFWithPaddleOCR(ctx context.Context, filename string, data []byte, parser *PDFParser) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	baseURL := strings.TrimSpace(parser.PaddleOCRBaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(common.GetEnv(common.EnvPaddleOCRBaseUrl))
	}
	if baseURL == "" {
		baseURL = strings.TrimSpace(common.GetEnv(common.EnvPaddleOCRAPIURL))
	}
	if baseURL == "" {
		return ParseResult{Err: fmt.Errorf("parser: PaddleOCR requires paddleocr_base_url or PADDLEOCR_BASE_URL")}
	}
	apiKey := strings.TrimSpace(parser.PaddleOCRAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(common.GetEnv(common.EnvPaddleOCRAccessToken))
	}
	algorithm := strings.TrimSpace(parser.PaddleOCRAlgorithm)
	if algorithm == "" {
		algorithm = strings.TrimSpace(common.GetEnv(common.EnvPaddleOCRAlgorithm))
	}
	if algorithm == "" {
		algorithm = "PaddleOCR-VL"
	}

	// The async Job API, exactly like Python's PaddleOCRParser.parse_pdf():
	// submit {base}/api/v2/ocr/jobs, poll until done, download the JSONL
	// results. No synchronous local driver is involved here.
	client := newPaddleOCRClient(baseURL, apiKey, algorithm)
	text, err := client.ParsePDF(data, filename)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: PaddleOCR ParsePDF: %w", err)}
	}
	pageCount := 1
	if strings.TrimSpace(text) == "" {
		pageCount = 0
	}
	return parseMinerUMarkdownResult(ctx, filename, text, parser.OutputFormat, pageCount)
}
