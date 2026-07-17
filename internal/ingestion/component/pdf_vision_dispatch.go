package component

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/utility"
)

type pdfVisionPage struct {
	PageNumber int
	WidthPts   float64
	HeightPts  float64
	ImageURL   string
}

var (
	pdfVisionPromptLoader  = loadPDFVisionPrompt
	pdfVisionPageRenderer  = defaultRenderPDFVisionPages
	pdfVisionModelResolver = defaultPDFVisionModelResolver
	pdfVisionChatInvoker   = defaultPDFVisionChatInvoker
)

var (
	pdfVisionPromptCache   = make(map[string]string)
	pdfVisionPromptCacheMu sync.RWMutex
	pdfVisionPromptsBase   string
	pdfVisionPromptsOnce   sync.Once
)

func maybeDispatchPDFVision(
	fileType utility.FileType,
	filename string,
	binary []byte,
	inputs map[string]any,
	setups map[string]schema.ParserSetup,
) (parserDispatchResult, bool, error) {
	if fileType != utility.FileTypePDF {
		return parserDispatchResult{}, false, nil
	}
	setup, ok := setups["pdf"]
	if !ok {
		return parserDispatchResult{}, false, nil
	}

	method := getStringOr(setup, "parse_method", "")
	layout := getStringOr(setup, "layout_recognizer", "")

	// MinerU dispatch: parse_method "mineru" or layout_recognizer "@MinerU"
	layoutLower := strings.ToLower(strings.TrimSpace(layout))
	if strings.EqualFold(strings.TrimSpace(method), "mineru") ||
		strings.HasPrefix(layoutLower, "mineru") ||
		strings.Contains(layoutLower, "@mineru") {
		tenantID := getStringOr(inputs, "tenant_id", "")
		if tenantID == "" {
			return parserDispatchResult{}, true,
				fmt.Errorf("Parser: mineru requires tenant_id")
		}
		res, err := dispatchMinerUPDF(filename, binary, tenantID, setup)
		if err != nil {
			return parserDispatchResult{}, true, err
		}
		return res, true, nil
	}

	modelID, useVision := resolvePDFVisionModelID(setup)
	if !useVision {
		return parserDispatchResult{}, false, nil
	}
	tenantID := getStringOr(inputs, "tenant_id", "")
	if tenantID == "" {
		return parserDispatchResult{}, true, fmt.Errorf(
			`Parser: pdf parse_method %q requires tenant_id to resolve IMAGE2TEXT model`, modelID)
	}
	res, err := dispatchPDFVision(filename, binary, tenantID, modelID, setup)
	if err != nil {
		return parserDispatchResult{}, true, err
	}
	return res, true, nil
}

// dispatchMinerUPDF submits a PDF to the tenant's MinerU OCR model
// via the streaming /file_parse endpoint and returns parsed sections.
// Mirrors Python's mineru_parser.py:parse_pdf which POSTs with
// stream=True and reads the zip response body directly (no polling).
func dispatchMinerUPDF(
	_ string,
	binary []byte,
	tenantID string,
	setup schema.ParserSetup,
) (parserDispatchResult, error) {
	driver, _, apiConfig, _, err := resolveTenantModelByType(tenantID, entity.ModelTypeOCR)
	if err != nil {
		return parserDispatchResult{}, fmt.Errorf("Parser: mineru model: %w", err)
	}
	if !isMinerUDriver(driver) {
		return parserDispatchResult{}, fmt.Errorf(
			"Parser: mineru requires a MinerU OCR model; found %q. Please add a MinerU OCR model to your tenant.", driver.Name())
	}

	baseURL := ""
	if apiConfig.BaseURL != nil {
		baseURL = *apiConfig.BaseURL
	}
	if baseURL == "" {
		baseURL, _ = resolveMinerUBaseURL(driver, apiConfig)
	}
	apiURL := strings.TrimRight(baseURL, "/") + "/file_parse"

	// Parse method: "raw", "auto", "ocr", "txt" matching Python's MinerUParseMethod.
	parseMethod := getStringOr(setup, "parse_method", "auto")
	lang := getStringOr(setup, "mineru_lang", "English")
	mineruLang := mineruLangCode(lang)
	backend := getStringOr(setup, "mineru_backend", "pipeline")

	zipBytes, err := mineruStreamParse(apiURL, apiConfig.ApiKey, binary, parseMethod, mineruLang, backend)
	if err != nil {
		return parserDispatchResult{}, fmt.Errorf("Parser: mineru stream: %w", err)
	}

	sections, err := mineruExtractSections(zipBytes)
	if err != nil {
		return parserDispatchResult{}, fmt.Errorf("Parser: mineru extract: %w", err)
	}

	var parts []string
	for _, s := range sections {
		if s != "" {
			parts = append(parts, s)
		}
	}
	md := strings.Join(parts, "\n")

	outputFormat := getStringOr(setup, "output_format", "markdown")
	return parserDispatchResult{
		OutputFormat: outputFormat,
		Markdown:     md,
	}, nil
}

// resolveMinerUBaseURL extracts the resolved base URL from a model driver.
func resolveMinerUBaseURL(driver modelModule.ModelDriver, apiConfig *modelModule.APIConfig) (string, error) {
	type baseURLGetter interface {
		GetBaseURL(*modelModule.APIConfig) (string, error)
	}
	if g, ok := driver.(baseURLGetter); ok {
		return g.GetBaseURL(apiConfig)
	}
	return "", fmt.Errorf("driver %q does not expose GetBaseURL", driver.Name())
}

// mineruLangCode maps a human-readable language name to a MinerU lang code,
// mirroring Python's LANGUAGE_TO_MINERU_MAP in mineru_parser.py.
func mineruLangCode(lang string) string {
	switch strings.ToLower(lang) {
	case "english":
		return "en"
	case "chinese":
		return "ch"
	case "traditional chinese":
		return "chinese_cht"
	case "japanese":
		return "japan"
	case "korean":
		return "korean"
	case "russian", "ukrainian":
		return "east_slavic"
	default:
		return "ch"
	}
}

// mineruStreamParse POSTs the PDF binary to the MinerU /file_parse
// endpoint with streaming and returns the zip response body.
// Mirrors Python's mineru_parser.py._run_mineru_api with stream=True.
func mineruStreamParse(apiURL string, apiKey *string, binary []byte, parseMethod, lang, backend string) ([]byte, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("files", "document.pdf")
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(binary); err != nil {
		return nil, fmt.Errorf("write pdf: %w", err)
	}

	_ = writer.WriteField("backend", backend)
	_ = writer.WriteField("parse_method", parseMethod)
	_ = writer.WriteField("lang_list", lang)
	_ = writer.WriteField("return_md", "true")
	_ = writer.WriteField("return_content_list", "true")
	_ = writer.WriteField("response_format_zip", "true")
	_ = writer.WriteField("start_page_id", "0")
	_ = writer.WriteField("end_page_id", "99999")
	_ = writer.WriteField("return_images", "true")
	_ = writer.WriteField("return_middle_json", "true")
	_ = writer.WriteField("return_model_output", "true")
	_ = writer.WriteField("formula_enable", "true")
	_ = writer.WriteField("table_enable", "true")

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finalize form: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, &body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if apiKey != nil && *apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+*apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}

	zipBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if len(zipBytes) == 0 {
		return nil, fmt.Errorf("empty response from MinerU")
	}
	return zipBytes, nil
}

// mineruExtractSections reads the MinerU content_list.json from a zip
// archive and extracts section text blocks, mirroring Python's
// _transfer_to_sections.
func mineruExtractSections(zipBytes []byte) ([]string, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}

	var contentList []byte
	for _, f := range zipReader.File {
		if strings.HasSuffix(f.Name, "content_list.json") {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			contentList, _ = io.ReadAll(rc)
			rc.Close()
			break
		}
	}
	if len(contentList) == 0 {
		return nil, fmt.Errorf("content_list.json not found in MinerU zip")
	}

	var items []map[string]any
	if err := json.Unmarshal(contentList, &items); err != nil {
		return nil, fmt.Errorf("parse content_list.json: %w", err)
	}

	var sections []string
	for _, item := range items {
		typ, _ := item["type"].(string)
		switch typ {
		case "text":
			if text, ok := item["text"].(string); ok {
				sections = append(sections, text)
			}
		case "table":
			if tb, ok := item["table_body"].(string); ok {
				sections = append(sections, tb)
			}
			for _, cap := range stringSlice(item["table_caption"]) {
				sections = append(sections, cap)
			}
		case "image":
			for _, cap := range stringSlice(item["image_caption"]) {
				sections = append(sections, cap)
			}
			if desc, ok := item["vlm_description"].(string); ok && desc != "" {
				sections = append(sections, desc)
			}
		case "equation", "code":
			if text, ok := item["text"].(string); ok {
				sections = append(sections, text)
			}
		case "list":
			for _, li := range stringSlice(item["list_items"]) {
				sections = append(sections, li)
			}
		default:
			if text, ok := item["text"].(string); ok {
				sections = append(sections, text)
			}
		}
	}
	return sections, nil
}

func stringSlice(raw any) []string {
	switch v := raw.(type) {
	case []any:
		var out []string
		for _, s := range v {
			if str, ok := s.(string); ok {
				out = append(out, str)
			}
		}
		return out
	case []string:
		return v
	}
	return nil
}

func resolvePDFVisionModelID(setup schema.ParserSetup) (string, bool) {
	if setup == nil {
		return "", false
	}
	if raw, ok := setup["parse_method"].(string); ok {
		method := strings.TrimSpace(raw)
		if method != "" && !isNamedPDFParseMethod(method) {
			return method, true
		}
	}
	if raw, ok := setup["layout_recognizer"].(string); ok {
		method := strings.TrimSpace(raw)
		if method == "" || strings.EqualFold(method, "plain text") || strings.EqualFold(method, "plaintext") {
			return "", false
		}
		if !isNamedPDFParseMethod(method) {
			return method, true
		}
	}
	return "", false
}

func isNamedPDFParseMethod(raw string) bool {
	method := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasSuffix(method, "@paddleocr"),
		strings.HasSuffix(method, "@somark"),
		strings.HasSuffix(method, "@opendataloader"):
		return true
	}
	switch method {
	case "deepdoc", "mineru", "plain_text", "plain text", "plaintext", "paddleocr", "docling", "opendataloader", "somark", "tcadp", "tcadp parser":
		return true
	}
	return false
}

func dispatchPDFVision(
	filename string,
	binary []byte,
	tenantID string,
	modelID string,
	setup schema.ParserSetup,
) (parserDispatchResult, error) {
	renderedPages, err := pdfVisionPageRenderer(binary)
	if err != nil {
		return parserDispatchResult{}, fmt.Errorf("Parser: pdf vision render: %w", err)
	}
	driver, resolvedModelName, apiConfig, err := pdfVisionModelResolver(tenantID, modelID)
	if err != nil {
		return parserDispatchResult{}, fmt.Errorf("Parser: pdf vision model %q: %w", modelID, err)
	}
	promptTemplate, err := pdfVisionPromptLoader("vision_llm_describe_prompt")
	if err != nil {
		return parserDispatchResult{}, fmt.Errorf("Parser: load vision prompt: %w", err)
	}

	items := make([]map[string]any, 0, len(renderedPages))
	markdownParts := make([]string, 0, len(renderedPages))
	for _, page := range renderedPages {
		prompt := renderPDFVisionPrompt(promptTemplate, page.PageNumber)
		resp, err := pdfVisionChatInvoker(driver, resolvedModelName, buildPDFVisionMessages(prompt, page.ImageURL), apiConfig)
		if err != nil {
			return parserDispatchResult{}, fmt.Errorf("Parser: pdf vision page %d: %w", page.PageNumber, err)
		}
		text := extractPDFVisionAnswer(resp)
		positions := [][]any{{page.PageNumber, 0.0, page.WidthPts, 0.0, page.HeightPts}}
		items = append(items, map[string]any{
			"text":           text,
			"doc_type_kwd":   "text",
			"page_number":    page.PageNumber,
			"_pdf_positions": positions,
			"positions":      positions,
		})
		if text != "" {
			markdownParts = append(markdownParts, text)
		}
	}

	outputFormat := "json"
	if v, ok := setup["output_format"].(string); ok && strings.TrimSpace(v) != "" {
		outputFormat = strings.ToLower(strings.TrimSpace(v))
	}
	fileMeta := map[string]any{
		"name":         filename,
		"page_count":   len(renderedPages),
		"outline":      []map[string]any{},
		"parse_method": modelID,
	}
	switch outputFormat {
	case "json":
		return parserDispatchResult{
			OutputFormat: "json",
			File:         fileMeta,
			JSON:         items,
		}, nil
	case "markdown":
		return parserDispatchResult{
			OutputFormat: "markdown",
			File:         fileMeta,
			Markdown:     strings.TrimSpace(strings.Join(markdownParts, "\n\n")),
		}, nil
	default:
		return parserDispatchResult{}, fmt.Errorf("Parser: unsupported PDF output_format %q for vision parse_method %q", outputFormat, modelID)
	}
}

func buildPDFVisionMessages(prompt string, imageURL string) []modelModule.Message {
	return []modelModule.Message{{
		Role: "user",
		Content: []interface{}{
			map[string]any{"type": "text", "text": prompt},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}},
		},
	}}
}

func extractPDFVisionAnswer(resp *modelModule.ChatResponse) string {
	if resp == nil || resp.Answer == nil {
		return ""
	}
	return strings.TrimSpace(*resp.Answer)
}

func defaultPDFVisionModelResolver(
	tenantID string,
	modelID string,
) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
	if strings.TrimSpace(modelID) == "" {
		driver, modelName, apiConfig, _, err := resolveTenantModelByType(tenantID, entity.ModelTypeImage2Text)
		return driver, modelName, apiConfig, err
	}
	driver, modelName, apiConfig, _, err := resolveModelConfig(tenantID, entity.ModelTypeImage2Text, modelID)
	return driver, modelName, apiConfig, err
}

func defaultPDFVisionChatInvoker(
	driver modelModule.ModelDriver,
	modelName string,
	messages []modelModule.Message,
	apiConfig *modelModule.APIConfig,
) (*modelModule.ChatResponse, error) {
	vision := true
	return driver.ChatWithMessages(modelName, messages, apiConfig, &modelModule.ChatConfig{Vision: &vision})
}

func loadPDFVisionPrompt(name string) (string, error) {
	pdfVisionPromptCacheMu.RLock()
	if cached, ok := pdfVisionPromptCache[name]; ok {
		pdfVisionPromptCacheMu.RUnlock()
		return cached, nil
	}
	pdfVisionPromptCacheMu.RUnlock()

	baseDir, err := pdfVisionPromptsBaseDir()
	if err != nil {
		return "", err
	}
	promptPath := filepath.Join(baseDir, "rag", "prompts", fmt.Sprintf("%s.md", name))
	content, err := os.ReadFile(promptPath)
	if err != nil {
		return "", fmt.Errorf("prompt file %q not found: %w", name, err)
	}
	cached := strings.TrimSpace(string(content))
	pdfVisionPromptCacheMu.Lock()
	pdfVisionPromptCache[name] = cached
	pdfVisionPromptCacheMu.Unlock()
	return cached, nil
}

func pdfVisionPromptsBaseDir() (string, error) {
	var initErr error
	pdfVisionPromptsOnce.Do(func() {
		root := utility.GetProjectRoot()
		if _, statErr := os.Stat(filepath.Join(root, "rag", "prompts")); statErr == nil {
			pdfVisionPromptsBase = root
			return
		}
		initErr = fmt.Errorf("rag/prompts not found under project root %q", root)
	})
	if initErr != nil {
		return "", initErr
	}
	return pdfVisionPromptsBase, nil
}

func renderPDFVisionPrompt(template string, page int) string {
	rendered := strings.ReplaceAll(template, "{{ page }}", fmt.Sprintf("%d", page))
	rendered = strings.ReplaceAll(rendered, "{{page}}", fmt.Sprintf("%d", page))
	return rendered
}

// isMinerUDriver reports whether the model driver is a MinerU variant
// (remote mineru.net or local mineru).
func isMinerUDriver(driver modelModule.ModelDriver) bool {
	switch strings.ToLower(driver.Name()) {
	case "mineru", "mineru.net":
		return true
	}
	return false
}
