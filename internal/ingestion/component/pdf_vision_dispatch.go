package component

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/utility"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type pdfVisionPage struct {
	PageNumber int
	WidthPts   float64
	HeightPts  float64
	ImageURL   string
}

const (
	monkeyOCRv2MaxResponseBytes     = 512 << 20
	monkeyOCRv2MaxZIPMembers        = 10000
	monkeyOCRv2MaxUncompressedBytes = 2 << 30
	monkeyOCRv2MaxImageBytes        = 64 << 20
	monkeyOCRv2DefaultTimeout       = 600 * time.Second
)

var monkeyOCRv2ImagePattern = regexp.MustCompile(`!\[([^]]*)\]\(([^)]+)\)`)

var (
	pdfVisionPromptLoader  = loadPDFVisionPrompt
	pdfVisionPageRenderer  = defaultRenderPDFVisionPages
	pdfVisionModelResolver = defaultPDFVisionModelResolver
	pdfVisionChatInvoker   = defaultPDFVisionChatInvoker
)

var (
	pdfVisionPromptCache   = make(map[string]string)
	pdfVisionPromptCacheMu sync.RWMutex
	pdfVisionPrompts       promptDirState
)

func maybeDispatchPDFVision(
	ctx context.Context,
	db *gorm.DB,
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
	tenantID := getStringOr(inputs, "tenant_id", "")
	layoutLower := strings.ToLower(strings.TrimSpace(layout))

	monkeySelector := layout
	if strings.TrimSpace(monkeySelector) == "" {
		monkeySelector = method
	}
	isMonkeyMatch := strings.EqualFold(strings.TrimSpace(method), "monkeyocrv2") ||
		strings.HasPrefix(layoutLower, "monkeyocrv2") ||
		strings.Contains(layoutLower, "@monkeyocrv2")
	isMonkeyByUUID := false
	if !isMonkeyMatch && tenantID != "" && strings.TrimSpace(monkeySelector) != "" && !isNamedPDFParseMethod(monkeySelector) {
		isMonkeyByUUID = isMonkeyOCRv2LayoutModelID(ctx, db, tenantID, monkeySelector)
	}
	if isMonkeyMatch || isMonkeyByUUID {
		modelRef := ""
		if isMonkeyByUUID {
			modelRef = monkeySelector
		} else if strings.Contains(monkeySelector, "@") {
			modelRef = monkeySelector
		}
		res, err := dispatchMonkeyOCRv2PDF(ctx, db, filename, binary, tenantID, setup, modelRef)
		return res, true, err
	}

	// MinerU dispatch: parse_method "mineru" or layout_recognizer "@MinerU"
	if strings.EqualFold(strings.TrimSpace(method), "mineru") ||
		strings.HasPrefix(layoutLower, "mineru") ||
		strings.Contains(layoutLower, "@mineru") {
		common.Info("pdf vision dispatch: MinerU branch matched",
			zap.String("parse_method", method),
			zap.String("layout_recognizer", layout),
			zap.String("tenant_id", tenantID))
		if tenantID == "" {
			return parserDispatchResult{}, true,
				fmt.Errorf("parser: MinerU requires tenant_id")
		}
		res, err := dispatchMinerUPDF(ctx, db, filename, binary, tenantID, setup)
		if err != nil {
			return parserDispatchResult{}, true, err
		}
		return res, true, nil
	}

	// PaddleOCR dispatch: parse_method "paddleocr", a layout_recognizer whose
	// provider/selectors name PaddleOCR, or a bare tenant model UUID (in
	// either parse_method or layout_recognizer) that resolves to a PaddleOCR
	// OCR model. A bare UUID carries no provider spelling in the string, so it
	// is resolved first — mirroring Python's get_composite_model_name_by_id +
	// normalize_layout_recognizer chain, which converts the raw model UUID
	// into model@instance@provider before choosing the dispatch path.
	isPaddleOCRMatch := strings.EqualFold(strings.TrimSpace(method), "paddleocr") ||
		strings.HasPrefix(layoutLower, "paddleocr") ||
		strings.Contains(layoutLower, "@paddleocr")
	// The UUID may be placed in either parse_method or layout_recognizer;
	// probe the non-empty one (layout takes precedence, then parse_method).
	// The selector is used only for the UUID probe: a string-matched
	// "paddleocr"/"@paddleocr" selector is a method name, not a model UUID,
	// so it must never reach the model resolver. Named parse methods (e.g.
	// "deepdoc") are likewise never probed as model UUIDs.
	paddleOCRSelector := layout
	if strings.TrimSpace(paddleOCRSelector) == "" {
		paddleOCRSelector = method
	}
	isPaddleOCRByUUID := false
	if !isPaddleOCRMatch && strings.TrimSpace(paddleOCRSelector) != "" &&
		!isNamedPDFParseMethod(paddleOCRSelector) {
		isPaddleOCRByUUID = isPaddleOCRLayoutModelID(ctx, db, tenantID, paddleOCRSelector)
	}
	if isPaddleOCRMatch || isPaddleOCRByUUID {
		if tenantID == "" {
			return parserDispatchResult{}, true,
				fmt.Errorf("parser: PaddleOCR requires tenant_id")
		}
		// Only a resolved UUID may be passed to the model resolver; a string
		// match keeps the empty modelID so the tenant's PaddleOCR model is
		// resolved by provider instead.
		dispatchModelID := ""
		if isPaddleOCRByUUID {
			dispatchModelID = paddleOCRSelector
		}
		res, err := dispatchPaddleOCRPdf(ctx, db, filename, binary, tenantID, setup, dispatchModelID)
		if err != nil {
			return parserDispatchResult{}, true, err
		}
		return res, true, nil
	}

	modelID, useVision := resolvePDFVisionModelID(setup)
	if !useVision {
		return parserDispatchResult{}, false, nil
	}
	if tenantID == "" {
		return parserDispatchResult{}, true, fmt.Errorf(
			`parser: pdf parse_method %q requires tenant_id to resolve VLM model`, modelID)
	}
	res, err := dispatchPDFVision(ctx, db, filename, binary, tenantID, modelID, setup)
	if err != nil {
		return parserDispatchResult{}, true, err
	}
	return res, true, nil
}

func dispatchMonkeyOCRv2PDF(ctx context.Context, db *gorm.DB, filename string, binary []byte, tenantID string, setup schema.ParserSetup, modelRef string) (parserDispatchResult, error) {
	baseURL := strings.TrimRight(getStringOr(setup, "monkeyocrv2_server_url", os.Getenv(common.EnvMonkeyOCRv2ServerURL)), "/")
	timeout := monkeyOCRv2RequestTimeout(setup, nil)
	if tenantID != "" {
		var driver modelModule.ModelDriver
		var apiConfig *modelModule.APIConfig
		var err error
		if modelRef != "" {
			driver, _, apiConfig, _, err = resolveModelConfig(ctx, db, tenantID, entity.ModelTypeOCR, modelRef)
		} else {
			driver, _, apiConfig, _, err = resolveTenantOCRModelByProvider(ctx, db, tenantID, "MonkeyOCRv2")
		}
		if err == nil {
			if !strings.EqualFold(driver.Name(), "monkeyocrv2") {
				return parserDispatchResult{}, fmt.Errorf("parser: MonkeyOCRv2 requires a MonkeyOCRv2 OCR model; found %q", driver.Name())
			}
			if apiConfig != nil && apiConfig.BaseURL != nil && strings.TrimSpace(*apiConfig.BaseURL) != "" {
				baseURL = strings.TrimRight(*apiConfig.BaseURL, "/")
			} else if configuredURL := monkeyOCRv2APIConfigValue(apiConfig, "monkeyocrv2_server_url", common.EnvMonkeyOCRv2ServerURL); configuredURL != "" {
				baseURL = strings.TrimRight(configuredURL, "/")
			}
			timeout = monkeyOCRv2RequestTimeout(setup, apiConfig)
		} else if baseURL == "" {
			return parserDispatchResult{}, fmt.Errorf("parser: MonkeyOCRv2 model: %w", err)
		}
	}
	if baseURL == "" {
		return parserDispatchResult{}, fmt.Errorf("parser: MonkeyOCRv2 requires monkeyocrv2_server_url or MONKEYOCRV2_SERVER_URL")
	}
	zipBytes, err := monkeyOCRv2ParseRequest(ctx, baseURL+"/parse", filename, binary, timeout)
	if err != nil {
		return parserDispatchResult{}, err
	}
	sections, err := monkeyOCRv2ExtractSections(zipBytes)
	if err != nil {
		return parserDispatchResult{}, err
	}
	return parserDispatchResult{OutputFormat: "markdown", Markdown: strings.Join(sections, "\n\n")}, nil
}

var isMonkeyOCRv2LayoutModelID = defaultIsMonkeyOCRv2LayoutModelID

func defaultIsMonkeyOCRv2LayoutModelID(ctx context.Context, db *gorm.DB, tenantID, modelID string) bool {
	if db == nil || strings.TrimSpace(modelID) == "" {
		return false
	}
	driver, _, _, _, err := resolveModelConfigByID(ctx, db, tenantID, entity.ModelTypeOCR, modelID)
	return err == nil && strings.EqualFold(driver.Name(), "monkeyocrv2")
}

func monkeyOCRv2ParseRequest(ctx context.Context, apiURL, filename string, binary []byte, timeout time.Duration) ([]byte, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("files", filepath.Base(filename))
	if err != nil {
		return nil, err
	}
	if _, err = part.Write(binary); err != nil {
		return nil, err
	}
	_ = writer.WriteField("start_page_id", "0")
	_ = writer.WriteField("end_page_id", "99999")
	if err = writer.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := *http.DefaultClient
	client.Timeout = timeout
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("MonkeyOCRv2 request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, monkeyOCRv2MaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > monkeyOCRv2MaxResponseBytes {
		return nil, fmt.Errorf("MonkeyOCRv2 response exceeds %d bytes", monkeyOCRv2MaxResponseBytes)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MonkeyOCRv2 HTTP %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

func monkeyOCRv2APIConfigValue(apiConfig *modelModule.APIConfig, keys ...string) string {
	if apiConfig == nil || apiConfig.ApiKey == nil || strings.TrimSpace(*apiConfig.ApiKey) == "" {
		return ""
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(*apiConfig.ApiKey), &config); err != nil {
		return ""
	}
	if nested, ok := config["api_key"].(map[string]any); ok {
		config = nested
	}
	for _, key := range keys {
		if value, ok := config[key]; ok && value != nil {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func monkeyOCRv2RequestTimeout(setup schema.ParserSetup, apiConfig *modelModule.APIConfig) time.Duration {
	value := ""
	if raw, ok := setup["monkeyocrv2_timeout"]; ok {
		value = strings.TrimSpace(fmt.Sprint(raw))
	}
	if value == "" {
		value = monkeyOCRv2APIConfigValue(apiConfig, "monkeyocrv2_timeout", common.EnvMonkeyOCRv2Timeout)
	}
	if value == "" {
		value = os.Getenv(common.EnvMonkeyOCRv2Timeout)
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return monkeyOCRv2DefaultTimeout
	}
	return time.Duration(seconds) * time.Second
}

func monkeyOCRv2ExtractSections(zipBytes []byte) ([]string, error) {
	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("MonkeyOCRv2 returned invalid ZIP: %w", err)
	}
	if len(reader.File) > monkeyOCRv2MaxZIPMembers {
		return nil, fmt.Errorf("MonkeyOCRv2 ZIP contains too many files")
	}
	var uncompressed uint64
	for _, file := range reader.File {
		uncompressed += file.UncompressedSize64
		if uncompressed > monkeyOCRv2MaxUncompressedBytes {
			return nil, fmt.Errorf("MonkeyOCRv2 ZIP is too large after extraction")
		}
	}
	var canonicalJSON, legacyJSON []*zip.File
	images := make(map[string]*zip.File)
	for _, file := range reader.File {
		if strings.HasPrefix(strings.ToLower(mime.TypeByExtension(path.Ext(file.Name))), "image/") {
			images[path.Base(file.Name)] = file
		}
		cleanName := strings.TrimPrefix(path.Clean(file.Name), "./")
		parts := strings.Split(cleanName, "/")
		if len(parts) == 2 && parts[1] == parts[0]+".json" {
			canonicalJSON = append(canonicalJSON, file)
		}
		if strings.Contains("/"+file.Name, "/jsons/") && strings.HasSuffix(file.Name, ".json") {
			legacyJSON = append(legacyJSON, file)
		}
	}
	candidates := canonicalJSON
	if len(candidates) == 0 {
		candidates = legacyJSON
	}
	if len(candidates) == 0 {
		for _, file := range reader.File {
			if strings.HasSuffix(file.Name, "all_results.json") {
				candidates = append(candidates, file)
			}
		}
	}
	var sections []string
	for _, file := range candidates {
		rc, openErr := file.Open()
		if openErr != nil {
			return nil, openErr
		}
		data, readErr := io.ReadAll(rc)
		rc.Close()
		if readErr != nil {
			return nil, readErr
		}
		var raw any
		if err = json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parse %s: %w", file.Name, err)
		}
		sections = append(sections, monkeyOCRv2Texts(raw, images)...)
	}
	if len(sections) == 0 {
		return nil, fmt.Errorf("MonkeyOCRv2 ZIP contains no textual layouts")
	}
	return sections, nil
}

func monkeyOCRv2Texts(raw any, images map[string]*zip.File) []string {
	if object, ok := raw.(map[string]any); ok {
		for _, key := range []string{"layouts", "content_list", "results", "items"} {
			if value, exists := object[key]; exists {
				return monkeyOCRv2Texts(value, images)
			}
		}
		label := strings.ToLower(strings.ReplaceAll(getAnyString(object, "type", "label"), "_", "-"))
		switch label {
		case "page-header", "page-footer", "page-number", "discarded":
			return nil
		}
		if label == "picture" || label == "figure" || label == "image" {
			return monkeyOCRv2EmbeddedImage(getAnyString(object, "text", "content", "markdown"), images)
		}
		for _, key := range []string{"text", "content", "markdown", "table_body"} {
			if text, ok := object[key].(string); ok && strings.TrimSpace(text) != "" {
				return []string{text}
			}
		}
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	var result []string
	for _, item := range items {
		result = append(result, monkeyOCRv2Texts(item, images)...)
	}
	return result
}

func monkeyOCRv2EmbeddedImage(markdown string, images map[string]*zip.File) []string {
	match := monkeyOCRv2ImagePattern.FindStringSubmatch(markdown)
	if len(match) != 3 {
		return nil
	}
	file := images[path.Base(strings.Trim(strings.TrimSpace(match[2]), `"'`))]
	if file == nil || file.UncompressedSize64 > monkeyOCRv2MaxImageBytes {
		return nil
	}
	rc, err := file.Open()
	if err != nil {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(rc, monkeyOCRv2MaxImageBytes+1))
	rc.Close()
	if err != nil || len(data) > monkeyOCRv2MaxImageBytes {
		return nil
	}
	mediaType := mime.TypeByExtension(path.Ext(file.Name))
	if mediaType == "" {
		mediaType = http.DetectContentType(data)
	}
	return []string{fmt.Sprintf("![%s](data:%s;base64,%s)", match[1], mediaType, base64.StdEncoding.EncodeToString(data))}
}

func getAnyString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key].(string); ok {
			return value
		}
	}
	return ""
}

// dispatchMinerUPDF submits a PDF to the tenant's MinerU OCR model
// via the streaming /file_parse endpoint and returns parsed sections.
// Mirrors Python's mineru_parser.py:parse_PDF which POSTs with
// stream=True and reads the zip response body directly (no polling).
func dispatchMinerUPDF(
	ctx context.Context,
	db *gorm.DB,
	_ string,
	binary []byte,
	tenantID string,
	setup schema.ParserSetup,
) (parserDispatchResult, error) {
	driver, _, apiConfig, _, err := resolveTenantModelByType(ctx, db, tenantID, entity.ModelTypeOCR)
	if err != nil {
		return parserDispatchResult{}, fmt.Errorf("parser: MinerU model: %w", err)
	}
	if !isMinerUDriver(driver) {
		return parserDispatchResult{}, fmt.Errorf(
			"parser: MinerU requires a MinerU OCR model; found %q. Please add a MinerU OCR model to your tenant", driver.Name())
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
		return parserDispatchResult{}, fmt.Errorf("parser: MinerU stream: %w", err)
	}

	sections, err := mineruExtractSections(zipBytes)
	if err != nil {
		return parserDispatchResult{}, fmt.Errorf("parser: MinerU extract: %w", err)
	}

	var parts []string
	for _, s := range sections {
		if s != "" {
			parts = append(parts, s)
		}
	}
	md := strings.Join(parts, "\n")

	// MinerU always returns rendered markdown text (md), regardless of
	// the requested output_format; label the payload as markdown so the
	// downstream chunker consumes it instead of a nil JSONResult.
	if format := strings.TrimSpace(getStringOr(setup, "output_format", "markdown")); !strings.EqualFold(format, "markdown") {
		common.Warn("mineru parser: output_format %q requested but backend only returns markdown; treating result as markdown",
			zap.String("output_format", format))
	}
	return parserDispatchResult{
		OutputFormat: "markdown",
		Markdown:     md,
	}, nil
}

// resolvePaddleOCRModelForDispatch resolves the OCR model used by the
// PaddleOCR PDF dispatch. modelID is the raw layout_recognizer value: a bare
// tenant model UUID (no "@") selects that exact model regardless of provider
// spelling ("PaddleOCR" or "PaddleOCR.local"); a composite name or empty value
// falls back to the tenant's first PaddleOCR OCR model, mirroring Python's
// by_paddleocr which uses get_first_provider_model_name(tenant, "PaddleOCR").
//
// Known limitation: composite names such as "some-model@instance@PaddleOCR.local"
// (an explicit local selection) are not recognized here. Any value containing
// "@" falls through to resolveTenantOCRModelByProvider("PaddleOCR"), which
// returns the tenant's first active PaddleOCR OCR model — potentially the
// local one — when both the local ("PaddleOCR.local") and the cloud
// ("PaddleOCR") providers are configured. The Python path behaves the same
// way; if exact selection is required, pass the model's tenant-model UUID
// instead.
var resolvePaddleOCRModelForDispatch = defaultResolvePaddleOCRModelForDispatch

func defaultResolvePaddleOCRModelForDispatch(ctx context.Context, db *gorm.DB, tenantID, modelID string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
	if strings.TrimSpace(modelID) != "" && !strings.Contains(modelID, "@") {
		driver, modelName, apiConfig, _, err := resolveModelConfigByID(ctx, db, tenantID, entity.ModelTypeOCR, modelID)
		return driver, modelName, apiConfig, err
	}
	driver, modelName, apiConfig, _, err := resolveTenantOCRModelByProvider(ctx, db, tenantID, "PaddleOCR")
	return driver, modelName, apiConfig, err
}

// dispatchPaddleOCRPdf submits a PDF to the tenant's PaddleOCR OCR model and
// returns parsed sections. The resolved driver runs the protocol it knows:
// the cloud "PaddleOCR" driver submits a job and polls the v2/ocr/jobs
// endpoint, the local "PaddleOCR.local" driver POSTs synchronously to
// layout-parsing — both mirror the Python paddleocr_parser paths.
func dispatchPaddleOCRPdf(
	ctx context.Context,
	db *gorm.DB,
	filename string,
	binary []byte,
	tenantID string,
	setup schema.ParserSetup,
	modelID string,
) (parserDispatchResult, error) {
	driver, modelName, apiConfig, err := resolvePaddleOCRModelForDispatch(ctx, db, tenantID, modelID)
	if err != nil {
		return parserDispatchResult{}, fmt.Errorf("parser: PaddleOCR model: %w", err)
	}
	if !isPaddleOCRDriver(driver) {
		return parserDispatchResult{}, fmt.Errorf(
			"parser: PaddleOCR requires a PaddleOCR OCR model; found %q. Please add a PaddleOCR OCR model to your tenant", driver.Name())
	}

	// Align with Python's PaddleOCROcrModel: the tenant api_key for the cloud
	// PaddleOCR provider is a JSON payload carrying paddleocr_base_url /
	// paddleocr_api_url, paddleocr_access_token and paddleocr_algorithm, while
	// the instance base_url field stays empty. PaddleOCR.local keeps a
	// plain-text bearer token in api_key and its base url in the instance
	// extra, so a non-JSON api_key passes through untouched.
	keyBaseURL, keyAccessToken, keyAlgorithm := "", "", ""
	if apiConfig.ApiKey != nil {
		keyBaseURL, keyAccessToken, keyAlgorithm = modelModule.PaddleOCRConfigFromAPIKey(*apiConfig.ApiKey)
	}

	baseURL := ""
	if apiConfig.BaseURL != nil {
		baseURL = *apiConfig.BaseURL
	}
	if baseURL == "" {
		baseURL = keyBaseURL
	}
	if baseURL == "" {
		baseURL = strings.TrimSpace(common.GetEnv(common.EnvPaddleOCRBaseUrl))
	}
	if baseURL == "" {
		baseURL = strings.TrimSpace(common.GetEnv(common.EnvPaddleOCRAPIURL))
	}
	if baseURL == "" {
		return parserDispatchResult{}, fmt.Errorf(
			"parser: PaddleOCR requires a base url from the tenant PaddleOCR OCR model or PADDLEOCR_BASE_URL")
	}

	apiKey := ""
	if apiConfig.ApiKey != nil {
		apiKey = *apiConfig.ApiKey
	}
	if keyAccessToken != "" {
		apiKey = keyAccessToken
	}
	algorithm := strings.TrimSpace(getStringOr(setup, "paddleocr_algorithm", ""))
	if algorithm == "" {
		algorithm = keyAlgorithm
	}
	if algorithm == "" {
		algorithm = strings.TrimSpace(common.GetEnv(common.EnvPaddleOCRAlgorithm))
	}
	if algorithm == "" {
		algorithm = "PaddleOCR-VL"
	}
	ocrAPIConfig := &modelModule.APIConfig{BaseURL: &baseURL}
	if apiKey != "" {
		ocrAPIConfig.ApiKey = &apiKey
	}

	resp, err := driver.OCRFile(ctx, &modelName, binary, &filename, ocrAPIConfig, &modelModule.OCRConfig{
		Algorithm: algorithm,
	}, nil)
	if err != nil {
		return parserDispatchResult{}, fmt.Errorf("parser: PaddleOCR OCRFile: %w", err)
	}
	if resp == nil || resp.Text == nil {
		return parserDispatchResult{}, fmt.Errorf("parser: PaddleOCR returned empty text")
	}
	if strings.TrimSpace(*resp.Text) == "" {
		return parserDispatchResult{}, fmt.Errorf("parser: PaddleOCR returned empty text")
	}

	// PaddleOCR backends always return rendered markdown text
	// (OCRFile.Text), regardless of the requested output_format. The
	// payload MUST be labelled markdown so the downstream chunker
	// consumes the text; a non-markdown setup value only means this
	// backend cannot produce the requested layout format.
	if format := strings.TrimSpace(getStringOr(setup, "output_format", "markdown")); !strings.EqualFold(format, "markdown") {
		common.Warn("paddleocr parser: output_format %q requested but backend only returns markdown; treating result as markdown",
			zap.String("output_format", format))
	}
	return parserDispatchResult{
		OutputFormat: "markdown",
		Markdown:     *resp.Text,
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
	if err = json.Unmarshal(contentList, &items); err != nil {
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
			for _, caption := range stringSlice(item["table_caption"]) {
				sections = append(sections, caption)
			}
		case "image":
			for _, caption := range stringSlice(item["image_caption"]) {
				sections = append(sections, caption)
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

// isNamedPDFParseMethod reports whether raw is a recognized named PDF
// parse method (as opposed to a CustomVLM model name). Its membership set
// MUST stay aligned with the PDF whitelist enforced by
// (*ParserComponent).Check() (parser.go:200-203):
//
//	deepdoc, plain_text, mineru, docling,
//	opendataloader, tcadp parser, paddleocr, somark
//
// A parse_method that Check() rejects must not be treated as a named method
// here, otherwise it silently falls through to the CustomVLM vision path
// instead of failing fast at construction.
//
// Note: "@"-suffixed spellings such as "foo@mineru" are layout_recognizer
// selectors, not parse_method values. Check() rejects them as parse_method,
// and the MinerU layout branch is resolved from the layout_recognizer field
// separately (pdf_vision_dispatch.go:62-68), so they must NOT be recognized
// here.
func isNamedPDFParseMethod(raw string) bool {
	method := strings.ToLower(strings.TrimSpace(raw))
	switch method {
	case "deepdoc", "plain_text", "mineru", "monkeyocrv2", "docling", "opendataloader", "tcadp parser", "paddleocr", "somark":
		return true
	}
	return false
}

func dispatchPDFVision(
	ctx context.Context,
	db *gorm.DB,
	filename string,
	binary []byte,
	tenantID string,
	modelID string,
	setup schema.ParserSetup,
) (parserDispatchResult, error) {
	renderedPages, err := pdfVisionPageRenderer(binary)
	if err != nil {
		return parserDispatchResult{}, fmt.Errorf("parser: pdf vision render: %w", err)
	}
	driver, resolvedModelName, apiConfig, err := pdfVisionModelResolver(ctx, db, tenantID, modelID)
	if err != nil {
		return parserDispatchResult{}, fmt.Errorf("parser: pdf vision model %q: %w", modelID, err)
	}
	promptTemplate, err := pdfVisionPromptLoader("vision_llm_describe_prompt")
	if err != nil {
		return parserDispatchResult{}, fmt.Errorf("parser: load vision prompt: %w", err)
	}

	items := make([]map[string]any, 0, len(renderedPages))
	markdownParts := make([]string, 0, len(renderedPages))
	for _, page := range renderedPages {
		prompt := renderPDFVisionPrompt(promptTemplate, page.PageNumber)
		resp, err := pdfVisionChatInvoker(ctx, driver, resolvedModelName, buildPDFVisionMessages(prompt, page.ImageURL), apiConfig)
		if err != nil {
			return parserDispatchResult{}, fmt.Errorf("parser: pdf vision page %d: %w", page.PageNumber, err)
		}
		text := extractVisionAnswer(resp)
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
		return parserDispatchResult{}, fmt.Errorf("parser: unsupported PDF output_format %q for vision parse_method %q", outputFormat, modelID)
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

func defaultPDFVisionModelResolver(
	ctx context.Context,
	db *gorm.DB,
	tenantID string,
	modelID string,
) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
	if strings.TrimSpace(modelID) == "" {
		driver, modelName, apiConfig, _, err := resolveTenantModelByType(ctx, db, tenantID, entity.ModelTypeImage2Text)
		return driver, modelName, apiConfig, err
	}
	driver, modelName, apiConfig, _, err := resolveModelConfig(ctx, db, tenantID, entity.ModelTypeImage2Text, modelID)
	return driver, modelName, apiConfig, err
}

func defaultPDFVisionChatInvoker(
	ctx context.Context,
	driver modelModule.ModelDriver,
	modelName string,
	messages []modelModule.Message,
	apiConfig *modelModule.APIConfig,
) (*modelModule.ChatResponse, error) {
	vision := true
	return driver.ChatWithMessages(ctx, modelName, messages, apiConfig, &modelModule.ChatConfig{Vision: &vision}, nil)
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
	return pdfVisionPrompts.resolve(utility.GetProjectRoot())
}

// renderPDFVisionPrompt only renders page metadata. The full-page PDF vision
// prompt is a transcription contract that preserves the document's original
// language; dataset-language instructions apply to figure descriptions in
// maybeDispatchVisionEnhancement instead.
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

// isPaddleOCRDriver reports whether the model driver is a PaddleOCR variant
// (cloud "paddleocr" or local "paddleocr.local").
func isPaddleOCRDriver(driver modelModule.ModelDriver) bool {
	switch strings.ToLower(driver.Name()) {
	case "paddleocr", "paddleocr.local":
		return true
	}
	return false
}

// isPaddleOCRLayoutModelID reports whether layout — a bare tenant model UUID
// with no "model@instance@provider" composite hint — resolves to an active
// OCR model driven by a PaddleOCR provider (cloud "PaddleOCR" or local
// "PaddleOCR.local"). The web UI stores the tenant model UUID directly in
// layout_recognizer, so the raw value carries no provider spelling; this
// mirrors Python's get_composite_model_name_by_id resolution of the same
// UUID before the PaddleOCR path is chosen.
var isPaddleOCRLayoutModelID = defaultIsPaddleOCRLayoutModelID

func defaultIsPaddleOCRLayoutModelID(ctx context.Context, db *gorm.DB, tenantID, layout string) bool {
	layout = strings.TrimSpace(layout)
	if db == nil {
		return false
	}
	if layout == "" || strings.Contains(layout, "@") {
		return false
	}
	driver, _, _, err := defaultResolvePaddleOCRModelForDispatch(ctx, db, tenantID, layout)
	if err != nil {
		return false
	}
	return isPaddleOCRDriver(driver)
}
