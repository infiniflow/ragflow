package component

import (
	"fmt"
	"strings"

	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/service"
	"ragflow/internal/utility"
)

type pdfVisionPage struct {
	PageNumber int
	WidthPts   float64
	HeightPts  float64
	ImageURL   string
}

var (
	pdfVisionPromptLoader  = service.LoadPrompt
	pdfVisionPageRenderer  = defaultRenderPDFVisionPages
	pdfVisionModelResolver = defaultPDFVisionModelResolver
	pdfVisionChatInvoker   = defaultPDFVisionChatInvoker
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
	case strings.HasSuffix(method, "@mineru"),
		strings.HasSuffix(method, "@paddleocr"),
		strings.HasSuffix(method, "@somark"),
		strings.HasSuffix(method, "@opendataloader"):
		return true
	}
	switch method {
	case "deepdoc", "plain_text", "plaintext", "mineru", "paddleocr", "docling", "opendataloader", "somark", "tcadp", "tcadp parser":
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
		prompt := service.RenderPrompt(promptTemplate, map[string]interface{}{"page": page.PageNumber})
		resp, err := pdfVisionChatInvoker(driver, resolvedModelName, buildPDFVisionMessages(prompt, page.ImageURL), apiConfig)
		if err != nil {
			return parserDispatchResult{}, fmt.Errorf("Parser: pdf vision page %d: %w", page.PageNumber, err)
		}
		text := extractPDFVisionAnswer(resp)
		if text == "" {
			continue
		}

		positions := [][]any{{page.PageNumber, 0.0, page.WidthPts, 0.0, page.HeightPts}}
		items = append(items, map[string]any{
			"text":           text,
			"doc_type_kwd":   "text",
			"page_number":    page.PageNumber,
			"_pdf_positions": positions,
			"positions":      positions,
		})
		markdownParts = append(markdownParts, text)
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
	svc := service.NewModelProviderService()
	if strings.TrimSpace(modelID) == "" {
		driver, modelName, apiConfig, _, err := svc.GetTenantDefaultModelByType(tenantID, entity.ModelTypeImage2Text)
		return driver, modelName, apiConfig, err
	}
	driver, modelName, apiConfig, _, err := svc.GetModelConfigFromProviderInstance(tenantID, entity.ModelTypeImage2Text, modelID)
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
