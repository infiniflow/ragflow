package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"

	models "ragflow/internal/entity/models"
)

func parsePDFWithSoMark(filename string, data []byte, parser *PDFParser) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	baseURL := strings.TrimSpace(parser.SoMarkBaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("SOMARK_BASE_URL"))
	}
	if baseURL == "" {
		return ParseResult{Err: fmt.Errorf("parser: SoMark requires somark_base_url or SOMARK_BASE_URL")}
	}
	apiKey := parser.SoMarkAPIKey
	if strings.TrimSpace(apiKey) == "" {
		apiKey = strings.TrimSpace(os.Getenv("SOMARK_API_KEY"))
	}
	taskID, err := soMarkSubmit(strings.TrimRight(baseURL, "/"), filename, data, parser, apiKey)
	if err != nil {
		return ParseResult{Err: err}
	}
	result, err := soMarkPoll(strings.TrimRight(baseURL, "/"), taskID, apiKey)
	if err != nil {
		return ParseResult{Err: err}
	}
	items, pageCount := soMarkItems(result, parser.SoMarkKeepHeaderFooter)
	if len(items) == 0 {
		return ParseResult{Err: fmt.Errorf("parser: SoMark returned no usable blocks")}
	}
	return pdfItemsToResult(filename, items, parser.OutputFormat, pageCount)
}

func soMarkSubmit(baseURL, filename string, data []byte, parser *PDFParser, apiKey string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("parser: SoMark create file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("parser: SoMark write file: %w", err)
	}
	_ = writer.WriteField("output_formats", "json")
	elementFormats, _ := json.Marshal(map[string]any{
		"image":   envOrDefault("SOMARK_IMAGE_FORMAT", parser.SoMarkImageFormat, "url"),
		"formula": envOrDefault("SOMARK_FORMULA_FORMAT", parser.SoMarkFormulaFormat, "latex"),
		"table":   envOrDefault("SOMARK_TABLE_FORMAT", parser.SoMarkTableFormat, "html"),
		"cs":      envOrDefault("SOMARK_CS_FORMAT", parser.SoMarkCSFormat, "image"),
	})
	featureConfig, _ := json.Marshal(map[string]any{
		"enable_text_cross_page":         envOrBool("SOMARK_ENABLE_TEXT_CROSS_PAGE", parser.SoMarkEnableTextCrossPage),
		"enable_table_cross_page":        envOrBool("SOMARK_ENABLE_TABLE_CROSS_PAGE", parser.SoMarkEnableTableCrossPage),
		"enable_title_level_recognition": envOrBool("SOMARK_ENABLE_TITLE_LEVEL_RECOGNITION", parser.SoMarkEnableTitleLevelRecognition),
		"enable_inline_image":            envOrBool("SOMARK_ENABLE_INLINE_IMAGE", parser.SoMarkEnableInlineImage),
		"enable_table_image":             envOrBool("SOMARK_ENABLE_TABLE_IMAGE", parser.SoMarkEnableTableImage),
		"enable_image_understanding":     envOrBool("SOMARK_ENABLE_IMAGE_UNDERSTANDING", parser.SoMarkEnableImageUnderstanding),
		"keep_header_footer":             envOrBool("SOMARK_KEEP_HEADER_FOOTER", parser.SoMarkKeepHeaderFooter),
	})
	_ = writer.WriteField("element_formats", string(elementFormats))
	_ = writer.WriteField("feature_config", string(featureConfig))
	if apiKey != "" {
		_ = writer.WriteField("api_key", apiKey)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("parser: SoMark finalize form: %w", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/parse/async", &body)
	if err != nil {
		return "", fmt.Errorf("parser: SoMark request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := models.NewDriverHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("parser: SoMark submit: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("parser: SoMark read submit: %w", err)
	}
	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("parser: SoMark decode submit: %w", err)
	}
	if payload.Code != 0 {
		return "", fmt.Errorf("parser: SoMark submit business error code=%d message=%s", payload.Code, payload.Message)
	}
	if payload.Data.TaskID == "" {
		return "", fmt.Errorf("parser: SoMark submit returned no task_id")
	}
	return payload.Data.TaskID, nil
}

func soMarkPoll(baseURL, taskID, apiKey string) (map[string]any, error) {
	form := url.Values{"task_id": {taskID}}
	if apiKey != "" {
		form.Set("api_key", apiKey)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/parse/async_check", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("parser: SoMark poll request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := models.NewDriverHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("parser: SoMark poll: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parser: SoMark read poll: %w", err)
	}
	var payload struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parser: SoMark decode poll: %w", err)
	}
	if payload.Code != 0 {
		return nil, fmt.Errorf("parser: SoMark poll business error code=%d message=%s", payload.Code, payload.Message)
	}
	status, _ := payload.Data["status"].(string)
	if status != "SUCCESS" {
		return nil, fmt.Errorf("parser: SoMark task %s status=%s", taskID, status)
	}
	result, _ := payload.Data["result"].(map[string]any)
	if result == nil {
		return nil, fmt.Errorf("parser: SoMark SUCCESS without result")
	}
	return result, nil
}

func soMarkItems(result map[string]any, keepHeaderFooter bool) ([]map[string]any, int) {
	outputs, _ := result["outputs"].(map[string]any)
	jsonPayload, _ := outputs["json"].(map[string]any)
	pages, _ := jsonPayload["pages"].([]any)
	items := make([]map[string]any, 0)
	for _, pageRaw := range pages {
		page, ok := pageRaw.(map[string]any)
		if !ok {
			continue
		}
		blocks, _ := page["blocks"].([]any)
		for _, blockRaw := range blocks {
			block, ok := blockRaw.(map[string]any)
			if !ok {
				continue
			}
			if item := soMarkBlockToItem(block, keepHeaderFooter); item != nil {
				items = append(items, item)
			}
		}
	}
	return items, len(pages)
}

func soMarkBlockToItem(block map[string]any, keepHeaderFooter bool) map[string]any {
	blockType := strings.ToLower(strings.TrimSpace(stringValue(block["type"])))
	switch blockType {
	case "cate", "cate_item", "blank":
		return nil
	case "header", "footer":
		if !keepHeaderFooter {
			return nil
		}
	}
	content := strings.TrimSpace(stringValue(block["content"]))
	switch blockType {
	case "figure", "cs", "qrcode", "stamp":
		if content == "" {
			content = "[Image]"
		}
		return map[string]any{"text": content, "doc_type_kwd": "image", "layout": "figure"}
	case "table":
		if content == "" {
			return nil
		}
		return map[string]any{"text": content, "doc_type_kwd": "table", "layout": "table"}
	case "equation":
		if content == "" {
			return nil
		}
		return map[string]any{"text": content, "doc_type_kwd": "text", "layout": "equation"}
	case "title":
		level := int(numberValue(block["title_level"]))
		if level < 1 || level > 6 {
			level = 1
		}
		if content == "" {
			return nil
		}
		return map[string]any{"text": strings.Repeat("#", level) + " " + content, "doc_type_kwd": "text", "layout": "title"}
	default:
		if content == "" {
			return nil
		}
		return map[string]any{"text": content, "doc_type_kwd": "text", "layout": "text"}
	}
}

func envOrDefault(envKey, configured, fallback string) string {
	if configured != "" {
		return configured
	}
	if raw := strings.TrimSpace(os.Getenv(envKey)); raw != "" {
		return raw
	}
	return fallback
}

func envOrBool(envKey string, configured bool) bool {
	if raw := strings.TrimSpace(os.Getenv(envKey)); raw != "" {
		switch strings.ToLower(raw) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return configured
}

func urlEncoded(values map[string]string) string {
	parts := make([]string, 0, len(values))
	for k, v := range values {
		if strings.TrimSpace(v) == "" {
			if k == "api_key" {
				continue
			}
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, "&")
}
