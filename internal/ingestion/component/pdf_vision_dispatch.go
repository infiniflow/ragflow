package component

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/utility"

	"gorm.io/gorm"
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
	driver, modelName, apiConfig, _, err := resolveModelConfigFromProviderInstance(tenantID, entity.ModelTypeImage2Text, modelID)
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
		cwd, err := os.Getwd()
		if err != nil {
			initErr = err
			return
		}
		for dir := cwd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
			if _, err := os.Stat(filepath.Join(dir, "rag", "prompts")); err == nil {
				pdfVisionPromptsBase = dir
				return
			}
			next := filepath.Dir(dir)
			if next == dir {
				break
			}
		}
		pdfVisionPromptsBase = "/ragflow"
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

type tenantModelExtra struct {
	MaxTokens *int `json:"max_tokens"`
}

func resolveTenantModelByType(tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	tenantDAO := dao.NewTenantDAO()
	tenant, err := tenantDAO.GetByID(tenantID)
	if err != nil {
		return nil, "", nil, 0, err
	}
	var modelID string
	switch modelType {
	case entity.ModelTypeChat:
		modelID = tenant.LLMID
	case entity.ModelTypeEmbedding:
		modelID = tenant.EmbdID
	case entity.ModelTypeRerank:
		modelID = tenant.RerankID
	case entity.ModelTypeSpeech2Text:
		modelID = tenant.ASRID
	case entity.ModelTypeImage2Text:
		modelID = tenant.Img2TxtID
	case entity.ModelTypeTTS:
		modelID = tenant.TTSID
	case entity.ModelTypeOCR:
		modelID = tenant.OCRID
	default:
		return nil, "", nil, 0, fmt.Errorf("invalid model type: %s", modelType)
	}
	if modelID == "" {
		return nil, "", nil, 0, fmt.Errorf("no default %s model is set", modelType)
	}
	return resolveModelConfigFromProviderInstance(tenantID, modelType, modelID)
}

func resolveModelConfigFromProviderInstance(tenantID string, modelType entity.ModelType, modelName string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	pureModelName, instanceName, providerName, err := parseCompositeModelName(modelName)
	if err != nil {
		return nil, "", nil, 0, err
	}

	providerDAO := dao.NewTenantModelProviderDAO()
	instanceDAO := dao.NewTenantModelInstanceDAO()
	modelDAO := dao.NewTenantModelDAO()

	provider, err := providerDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, "", nil, 0, fmt.Errorf("provider %q lookup failed: %w", providerName, err)
	}
	instance, err := instanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return nil, "", nil, 0, fmt.Errorf("instance %q lookup failed: %w", instanceName, err)
	}

	apiKey := instance.APIKey
	var extra map[string]string
	_ = json.Unmarshal([]byte(instance.Extra), &extra)
	region := extra["region"]
	baseURL := extra["base_url"]

	modelObj, modelErr := modelDAO.GetByProviderIDAndInstanceIDAndModelTypeAndModelName(
		provider.ID, instance.ID, string(modelType), pureModelName,
	)
	switch {
	case modelErr == nil:
		if modelObj.Status == "inactive" {
			return nil, "", nil, 0, fmt.Errorf("model %q is disabled", modelName)
		}
		providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
		if providerInfo == nil {
			return nil, "", nil, 0, fmt.Errorf("provider %q driver not found", providerName)
		}
		driver, err := newModelDriverForBaseURLLocal(providerInfo.ModelDriver, providerName, region, baseURL)
		if err != nil {
			return nil, "", nil, 0, err
		}
		maxTokens := 0
		if mi, _ := dao.GetModelProviderManager().GetModelByName(providerName, pureModelName); mi != nil && mi.MaxTokens != nil {
			maxTokens = *mi.MaxTokens
		}
		if modelObj != nil && strings.TrimSpace(modelObj.Extra) != "" {
			var tenantExtra tenantModelExtra
			if err := json.Unmarshal([]byte(modelObj.Extra), &tenantExtra); err != nil {
				return nil, "", nil, 0, err
			}
			if tenantExtra.MaxTokens != nil && *tenantExtra.MaxTokens > 0 {
				maxTokens = *tenantExtra.MaxTokens
			}
		}
		apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}
		return driver, modelObj.ModelName, apiConfig, maxTokens, nil
	case !errorsIsRecordNotFound(modelErr):
		return nil, "", nil, 0, fmt.Errorf("model %q lookup failed: %w", modelName, modelErr)
	}

	targetFactoryName := providerName
	if region == "intl" && strings.EqualFold(providerName, "siliconflow") {
		targetFactoryName = "siliconflow_intl"
	}
	targetProvider := dao.GetModelProviderManager().FindProvider(targetFactoryName)
	if targetProvider == nil {
		return nil, "", nil, 0, fmt.Errorf("model provider config not found: %s", providerName)
	}
	var llmInfo *modelModule.Model
	for i := range targetProvider.Models {
		if strings.EqualFold(targetProvider.Models[i].Name, pureModelName) {
			llmInfo = targetProvider.Models[i]
			break
		}
	}
	if llmInfo == nil {
		return nil, "", nil, 0, fmt.Errorf("model config not found: %s", modelName)
	}
	driver, err := newModelDriverForBaseURLLocal(targetProvider.ModelDriver, providerName, region, baseURL)
	if err != nil {
		return nil, "", nil, 0, err
	}
	apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}
	maxTokens := 0
	if llmInfo.MaxTokens != nil {
		maxTokens = *llmInfo.MaxTokens
	}
	return driver, llmInfo.Name, apiConfig, maxTokens, nil
}

func parseCompositeModelName(compositeName string) (modelName, instanceName, providerName string, err error) {
	parts := strings.Split(compositeName, "@")
	switch len(parts) {
	case 3:
		return parts[0], parts[1], parts[2], nil
	case 2:
		return parts[0], "default", parts[1], nil
	case 1:
		return parts[0], "", "", fmt.Errorf("provider name missing in model name: %s", compositeName)
	default:
		return "", "", "", fmt.Errorf("invalid model name format: %s", compositeName)
	}
}

func newModelDriverForBaseURLLocal(driver modelModule.ModelDriver, providerName, region, baseURL string) (modelModule.ModelDriver, error) {
	if driver == nil {
		return nil, fmt.Errorf("provider %s driver not found", providerName)
	}
	if strings.TrimSpace(baseURL) == "" {
		return driver, nil
	}
	baseURLByRegion := map[string]string{region: baseURL}
	if region == "" {
		baseURLByRegion["default"] = baseURL
	}
	newDriver := driver.NewInstance(baseURLByRegion)
	if newDriver == nil {
		return nil, fmt.Errorf("provider %s does not support custom base_url", providerName)
	}
	return newDriver, nil
}

func errorsIsRecordNotFound(err error) bool {
	return err != nil && (err == gorm.ErrRecordNotFound || strings.Contains(err.Error(), gorm.ErrRecordNotFound.Error()))
}
