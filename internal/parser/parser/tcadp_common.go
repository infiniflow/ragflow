package parser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ragflow/internal/common"

	models "ragflow/internal/entity/models"
)

// parseWithTCADP submits binary data to the TCADP cloud reconstruction service
// and returns the structured parse result.
//
// Shared by all three TCADP-using parser families (PDF, spreadsheet,
// presentation). Each family wraps this with its own thin function that
// supplies its parameters (env-fallbacks, fileType, table/format config)
// so the TCADP API contract lives in one place.
func parseWithTCADP(
	ctx context.Context,
	filename string,
	data []byte,
	fileType string,
	tcadpAPIServer, tcadpAPIKey string,
	tableResultType, markdownImageResponseType string,
	outputFormat string,
) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	baseURL := strings.TrimSpace(tcadpAPIServer)
	if baseURL == "" {
		// Try the legacy TCADP_APISERVER_URL first so existing PDF
		// deployments that set it for the old parsePDFWithTCADP helper
		// keep working — then fall back to the canonical TCADP_APISERVER
		// used by the other two families. Both env names live in
		// internal/common/environments.go so the source of truth is in
		// one place.
		if v := strings.TrimSpace(common.GetEnv(common.EnvTCADPAPIServerURL)); v != "" {
			baseURL = v
		} else {
			baseURL = strings.TrimSpace(common.GetEnv(common.EnvTCADPAPIServer))
		}
	}
	if baseURL == "" {
		return ParseResult{Err: fmt.Errorf("parser: TCADP requires tcadp_apiserver, TCADP_APISERVER_URL, or TCADP_APISERVER")}
	}
	apiKey := strings.TrimSpace(tcadpAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(common.GetEnv(common.EnvTCADPAPIKey))
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
	resp, err := models.PostJSONRequest(ctx, models.NewDriverHTTPClient(false),
		strings.TrimRight(baseURL, "/")+"/reconstruct_document", bearer(apiKey), requestBody)
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
	downloadReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		payload.DocumentRecognizeResultURL, nil)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: TCADP download request: %w", err)}
	}
	if auth := bearer(apiKey); auth != "" {
		downloadReq.Header.Set("Authorization", auth)
	}
	downloadResp, err := models.NewDriverHTTPClient(false).Do(downloadReq)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: TCADP download: %w", err)}
	}
	defer downloadResp.Body.Close()
	zipBytes, err := io.ReadAll(downloadResp.Body)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: TCADP read zip: %w", err)}
	}
	if downloadResp.StatusCode >= 300 {
		return ParseResult{Err: fmt.Errorf("parser: TCADP download HTTP %d: %s", downloadResp.StatusCode, string(zipBytes))}
	}
	items, pageCount, err := tcadpItemsFromZip(zipBytes)
	if err != nil {
		return ParseResult{Err: err}
	}
	return pdfItemsToResult(filename, items, outputFormat, pageCount)
}
