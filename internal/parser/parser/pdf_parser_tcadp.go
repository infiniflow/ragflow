package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"ragflow/internal/common"
	"strings"

	models "ragflow/internal/entity/models"
)

func parsePDFWithTCADP(filename string, data []byte, parser *PDFParser) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	baseURL := strings.TrimSpace(parser.TCADPAPIServer)
	if baseURL == "" {
		baseURL = strings.TrimSpace(common.GetEnv(common.EnvTCADPApiServerURL))
	}
	if baseURL == "" {
		return ParseResult{Err: fmt.Errorf("parser: TCADP requires tcadp_apiserver or TCADP_APISERVER")}
	}
	apiKey := strings.TrimSpace(parser.TCADPAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(common.GetEnv(common.EnvTCADPApiKey))
	}
	requestBody := map[string]any{
		"file_type":              "PDF",
		"file_base64":            base64.StdEncoding.EncodeToString(data),
		"file_start_page_number": 1,
		"file_end_page_number":   1000,
		"config": map[string]any{
			"TableResultType":           parser.TCADPTableResultType,
			"MarkdownImageResponseType": parser.TCADPMarkdownImageResponseType,
		},
	}
	resp, err := models.PostJSONRequest(context.Background(), models.NewDriverHTTPClient(false), strings.TrimRight(baseURL, "/")+"/reconstruct_document", bearer(apiKey), requestBody)
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
	return pdfItemsToResult(filename, items, parser.OutputFormat, pageCount)
}

func tcadpItemsFromZip(zipBytes []byte) ([]map[string]any, int, error) {
	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, 0, fmt.Errorf("parser: TCADP zip: %w", err)
	}
	items := make([]map[string]any, 0)
	pageCount := 0
	for _, file := range reader.File {
		if strings.HasSuffix(file.Name, ".md") {
			rc, err := file.Open()
			if err != nil {
				return nil, 0, err
			}
			body, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, 0, err
			}
			items = append(items, map[string]any{"text": strings.TrimSpace(string(body)), "doc_type_kwd": "text", "layout": "text"})
			if strings.TrimSpace(string(body)) != "" && pageCount == 0 {
				pageCount = 1
			}
			continue
		}
		if !strings.HasSuffix(file.Name, ".json") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, 0, err
		}
		var raw any
		err = json.NewDecoder(rc).Decode(&raw)
		rc.Close()
		if err != nil {
			return nil, 0, err
		}
		items = append(items, tcadpAnyToItems(raw)...)
		if pages := collectPDFPageNumbers(raw); len(pages) > pageCount {
			pageCount = len(pages)
		}
	}
	if len(items) == 0 {
		return nil, 0, fmt.Errorf("parser: TCADP zip contained no supported content")
	}
	return items, pageCount, nil
}

func tcadpAnyToItems(raw any) []map[string]any {
	switch v := raw.(type) {
	case []any:
		items := make([]map[string]any, 0)
		for _, item := range v {
			items = append(items, tcadpAnyToItems(item)...)
		}
		return items
	case map[string]any:
		text := strings.TrimSpace(stringValue(v["content"]))
		contentType := strings.ToLower(strings.TrimSpace(stringValue(v["type"])))
		page := extractTCADPPage(v)
		emit := func(text, docType, layout string) []map[string]any {
			m := map[string]any{"text": text, "doc_type_kwd": docType, "layout": layout}
			if page > 0 {
				// 1-indexed 5-tuple. AddPositions is a passthrough so
				// the final position_int / page_num_int carry the same
				// 1-indexed page number the caller passes. Mirrors
				// Python presentation.py:148-149.
				m["positions"] = []float64{float64(page), 0, 0, 0, 0}
			}
			return []map[string]any{m}
		}
		switch contentType {
		case "table":
			if text == "" {
				text = tcadpTableRowsText(v["table_data"])
			}
			if text == "" {
				return nil
			}
			return emit(text, "table", "table")
		case "image":
			caption := strings.TrimSpace(stringValue(v["caption"]))
			if caption == "" {
				caption = "[Image]"
			}
			return emit(caption, "image", "figure")
		case "equation":
			if text == "" {
				return nil
			}
			return emit("$$"+text+"$$", "text", "equation")
		default:
			if text == "" {
				return nil
			}
			return emit(text, "text", "text")
		}
	}
	return nil
}

// extractTCADPPage returns the 1-indexed page number carried by a raw TCADP
// element, using the same key set collectPDFPageNumbers walks
// (pdf_parser_remote_common.go). It returns 0 when the element has no page
// information (e.g. spreadsheet TCADP), so callers can skip position emission
// and remain parity-correct with Python (table.py sets no page either).
func extractTCADPPage(v map[string]any) int {
	for _, key := range []string{"page_number", "page_num", "page_no", "page_index", "page_idx", "page"} {
		if page := int(numberValue(v[key])); page > 0 {
			return page
		}
	}
	return 0
}

func tcadpTableRowsText(raw any) string {
	table, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	rows, ok := table["rows"].([]any)
	if !ok {
		return ""
	}
	lines := make([]string, 0, len(rows))
	for _, rowRaw := range rows {
		row, ok := rowRaw.([]any)
		if !ok {
			continue
		}
		cols := make([]string, 0, len(row))
		for _, col := range row {
			cols = append(cols, stringValue(col))
		}
		lines = append(lines, strings.Join(cols, " | "))
	}
	return strings.Join(lines, "\n")
}

func bearer(apiKey string) string {
	if strings.TrimSpace(apiKey) == "" {
		return ""
	}
	return "Bearer " + apiKey
}
