package parser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	models "ragflow/internal/entity/models"
)

func parseSpreadsheetWithTCADP(filename string, data []byte, fileType string, tcadpAPIServer, tcadpAPIKey, tableResultType, markdownImageResponseType string, outputFormat string) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	baseURL := strings.TrimSpace(tcadpAPIServer)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("TCADP_APISERVER"))
	}
	if baseURL == "" {
		return ParseResult{Err: fmt.Errorf("parser: TCADP requires tcadp_apiserver or TCADP_APISERVER")}
	}
	apiKey := strings.TrimSpace(tcadpAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("TCADP_API_KEY"))
	}
	requestBody := map[string]any{
		"file_type":              fileType,
		"file_base64":            base64.StdEncoding.EncodeToString(data),
		"file_start_page_number": 1,
		"file_end_page_number":   1000,
		"config": map[string]any{
			"TableResultType":           tableResultType,
			"MarkdownImageResponseType": markdownImageResponseType,
		},
	}
	resp, err := models.PostJSONRequest(context.Background(), models.NewDriverHTTPClient(), strings.TrimRight(baseURL, "/")+"/reconstruct_document", bearer(apiKey), requestBody)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: TCADP submit: %w", err)}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: TCADP read submit: %w", err)}
	}
	if resp.StatusCode >= 300 {
		return ParseResult{Err: fmt.Errorf("parser: TCADP HTTP %d: %s", resp.StatusCode, string(raw))}
	}
	var payload struct {
		DocumentRecognizeResultURL string `json:"DocumentRecognizeResultUrl"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ParseResult{Err: fmt.Errorf("parser: TCADP decode submit: %w", err)}
	}
	if payload.DocumentRecognizeResultURL == "" {
		return ParseResult{Err: fmt.Errorf("parser: TCADP returned no DocumentRecognizeResultUrl")}
	}
	downloadReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, payload.DocumentRecognizeResultURL, nil)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: TCADP download request: %w", err)}
	}
	if auth := bearer(apiKey); auth != "" {
		downloadReq.Header.Set("Authorization", auth)
	}
	downloadResp, err := models.NewDriverHTTPClient().Do(downloadReq)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: TCADP download: %w", err)}
	}
	defer downloadResp.Body.Close()
	zipBytes, err := io.ReadAll(downloadResp.Body)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: TCADP read zip: %w", err)}
	}
	items, pageCount, err := tcadpItemsFromZip(zipBytes)
	if err != nil {
		return ParseResult{Err: err}
	}
	return pdfItemsToResult(filename, items, outputFormat, pageCount)
}
