package parser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	models "ragflow/internal/entity/models"
)

func parsePDFWithOpenDataLoader(filename string, data []byte, parser *PDFParser) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	baseURL := strings.TrimSpace(parser.OpenDataLoaderAPIServer)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("OPENDATALOADER_APISERVER"))
	}
	if baseURL == "" {
		return ParseResult{Err: fmt.Errorf("parser: OpenDataLoader requires opendataloader_apiserver or OPENDATALOADER_APISERVER")}
	}
	apiKey := strings.TrimSpace(parser.OpenDataLoaderAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENDATALOADER_API_KEY"))
	}

	bodyReader, contentType, err := openDataLoaderMultipart(filename, data, parser)
	if err != nil {
		return ParseResult{Err: err}
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, strings.TrimRight(baseURL, "/")+"/file_parse", bodyReader)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: OpenDataLoader request: %w", err)}
	}
	req.Header.Set("Content-Type", contentType)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := models.NewDriverHTTPClient().Do(req)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: OpenDataLoader submit: %w", err)}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: OpenDataLoader read: %w", err)}
	}
	if resp.StatusCode >= 300 {
		return ParseResult{Err: fmt.Errorf("parser: OpenDataLoader HTTP %d: %s", resp.StatusCode, string(raw))}
	}
	var payload struct {
		JSONDoc any    `json:"json_doc"`
		MDText  string `json:"md_text"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ParseResult{Err: fmt.Errorf("parser: OpenDataLoader decode: %w", err)}
	}
	if payload.JSONDoc != nil {
		items := openDataLoaderItems(payload.JSONDoc)
		if len(items) > 0 {
			return pdfItemsToResult(filename, items, parser.OutputFormat, openDataLoaderPageCount(payload.JSONDoc))
		}
	}
	if strings.TrimSpace(payload.MDText) != "" {
		return parseMinerUMarkdownResult(filename, payload.MDText, parser.OutputFormat, 1)
	}
	return ParseResult{Err: fmt.Errorf("parser: OpenDataLoader returned no parsed content")}
}

func openDataLoaderMultipart(filename string, data []byte, parser *PDFParser) (io.Reader, string, error) {
	var body strings.Builder
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, "", fmt.Errorf("parser: OpenDataLoader create form file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return nil, "", fmt.Errorf("parser: OpenDataLoader write PDF: %w", err)
	}
	if parser.OpenDataLoaderHybrid != "" {
		_ = writer.WriteField("hybrid", parser.OpenDataLoaderHybrid)
	}
	if parser.OpenDataLoaderImageOutput != "" {
		_ = writer.WriteField("image_output", parser.OpenDataLoaderImageOutput)
	}
	if parser.OpenDataLoaderSanitize != nil {
		if *parser.OpenDataLoaderSanitize {
			_ = writer.WriteField("sanitize", "true")
		} else {
			_ = writer.WriteField("sanitize", "false")
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("parser: OpenDataLoader finalize form: %w", err)
	}
	return strings.NewReader(body.String()), writer.FormDataContentType(), nil
}

func openDataLoaderItems(root any) []map[string]any {
	items := []map[string]any{}
	var walk func(any)
	walk = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			if item := openDataLoaderNodeToItem(v); item != nil {
				items = append(items, item)
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(root)
	return items
}

func openDataLoaderNodeToItem(el map[string]any) map[string]any {
	rawType, _ := el["type"].(string)
	t := strings.ToLower(strings.TrimSpace(rawType))
	if t == "" {
		return nil
	}
	text := strings.TrimSpace(stringValue(el["content"]))
	if text == "" {
		text = strings.TrimSpace(stringValue(el["text"]))
	}
	switch t {
	case "table":
		html := strings.TrimSpace(stringValue(el["html"]))
		if html == "" {
			html = strings.TrimSpace(stringValue(el["html_content"]))
		}
		if html != "" {
			text = html
		}
		if text == "" {
			text = openDataLoaderCellsText(el["cells"])
		}
		if text == "" {
			return nil
		}
		return map[string]any{"text": text, "doc_type_kwd": "table", "layout": "table"}
	case "image", "picture", "figure":
		if text == "" {
			text = "[Image]"
		}
		return map[string]any{"text": text, "doc_type_kwd": "image", "layout": "figure"}
	case "formula", "equation":
		if text == "" {
			return nil
		}
		return map[string]any{"text": text, "doc_type_kwd": "text", "layout": "equation"}
	default:
		if text == "" {
			return nil
		}
		layout := "text"
		if t == "title" || t == "heading" {
			layout = "title"
		}
		return map[string]any{"text": text, "doc_type_kwd": "text", "layout": layout}
	}
}

func openDataLoaderCellsText(raw any) string {
	cells, ok := raw.([]any)
	if !ok {
		return ""
	}
	rows := make(map[int][]string)
	maxRow := -1
	for _, cellRaw := range cells {
		cell, ok := cellRaw.(map[string]any)
		if !ok {
			continue
		}
		row := int(numberValue(cell["row"]))
		if row == 0 {
			row = int(numberValue(cell["row_index"]))
		}
		rows[row] = append(rows[row], stringValue(cell["content"]))
		if row > maxRow {
			maxRow = row
		}
	}
	if len(rows) == 0 || maxRow < 0 {
		return ""
	}
	parts := make([]string, 0, len(rows))
	for i := 0; i <= maxRow; i++ {
		if cols, ok := rows[i]; ok {
			parts = append(parts, strings.Join(cols, " | "))
		}
	}
	return strings.Join(parts, "\n")
}

func openDataLoaderPageCount(root any) int {
	pages := collectPDFPageNumbers(root)
	if len(pages) > 0 {
		return len(pages)
	}
	if root != nil {
		return 1
	}
	return 0
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func numberValue(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}
