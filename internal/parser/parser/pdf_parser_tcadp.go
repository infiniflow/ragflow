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
	"os"
	"strings"

	models "ragflow/internal/entity/models"
)

func parsePDFWithTCADP(filename string, data []byte, parser *PDFParser) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	baseURL := strings.TrimSpace(parser.TCADPAPIServer)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("TCADP_APISERVER"))
	}
	if baseURL == "" {
		return ParseResult{Err: fmt.Errorf("parser: TCADP requires tcadp_apiserver or TCADP_APISERVER")}
	}
	apiKey := strings.TrimSpace(parser.TCADPAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("TCADP_API_KEY"))
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
		switch contentType {
		case "table":
			if text == "" {
				text = tcadpTableRowsText(v["table_data"])
			}
			if text == "" {
				return nil
			}
			return []map[string]any{{"text": text, "doc_type_kwd": "table", "layout": "table"}}
		case "image":
			caption := strings.TrimSpace(stringValue(v["caption"]))
			if caption == "" {
				caption = "[Image]"
			}
			return []map[string]any{{"text": caption, "doc_type_kwd": "image", "layout": "figure"}}
		case "equation":
			if text == "" {
				return nil
			}
			return []map[string]any{{"text": "$$" + text + "$$", "doc_type_kwd": "text", "layout": "equation"}}
		default:
			if text == "" {
				return nil
			}
			return []map[string]any{{"text": text, "doc_type_kwd": "text", "layout": "text"}}
		}
	}
	return nil
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
