package component

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/parser/tableutil"
	"ragflow/internal/rag/prompts"
	"ragflow/internal/utility"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// minerUDefaultLanguage is MinerU's own OCR language default, applied when
// neither mineru_lang nor the family setup names one. It deliberately does not
// consult the knowledge base: it is an engine input, not a caption setting.
const minerUDefaultLanguage = "Chinese"

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
	pdfVisionPromptLoader  = (prompts.EmbeddedPromptLoader{}).Load
	pdfVisionPageRenderer  = defaultRenderPDFVisionPages
	pdfVisionModelResolver = defaultPDFVisionModelResolver
	pdfVisionChatInvoker   = defaultPDFVisionChatInvoker
)

// maybeDispatchPDFVision handles selected tenant PDF/OCR parsers before generic
// VLM parsing. A handled result, including an error, must not fall back to the
// ordinary PDF parser.
func maybeDispatchPDFVision(
	ctx context.Context,
	db *gorm.DB,
	fileType utility.FileType,
	filename string,
	binary []byte,
	inputs map[string]any,
	setups map[string]schema.ParserSetup,
) (parser.ParseResult, bool, error) {
	if fileType != utility.FileTypePDF {
		return parser.ParseResult{}, false, nil
	}
	setup, ok := setups["pdf"]
	if !ok {
		return parser.ParseResult{}, false, nil
	}
	tenantID := getStringOr(inputs, "tenant_id", "")
	// The tenant-selected parser and the built-in backends share one
	// normalization boundary so a future normalization fix cannot miss one
	// of the two paths. PDF markdown engines (MonkeyOCRv2, MinerU V1
	// fallback) are converted to JSON by NormalizeMarkdown before dispatch
	// returns, so the NormalizeResult closure below only ever applies item
	// policies to already-normalized JSON and stays idempotent.
	flatten, _ := setup["flatten_media_to_text"].(bool)
	normalize := func(result parser.ParseResult) parser.ParseResult {
		pdf := parser.PDFParser{FlattenMediaToText: flatten}
		return parser.AttachPDFOutlines(pdf.NormalizeResult(ctx, filename, result), binary)
	}
	if result, handled, err := maybeDispatchTenantPDFParser(ctx, db, filename, binary, tenantID, setup); handled {
		if err != nil {
			return result, true, err
		}
		result = normalize(result)
		return result, true, result.Err
	}
	result, handled, err := dispatchPDFBackend(ctx, db, filename, binary, tenantID, setup)
	if err != nil || !handled {
		return result, handled, err
	}
	result = normalize(result)
	return result, true, result.Err
}

func dispatchPDFBackend(ctx context.Context, db *gorm.DB, filename string, binary []byte, tenantID string, setup schema.ParserSetup) (parser.ParseResult, bool, error) {
	method := getStringOr(setup, "parse_method", "")
	layout := getStringOr(setup, "layout_recognizer", "")
	layoutLower := strings.ToLower(strings.TrimSpace(layout))

	monkeySelector := layout
	if strings.TrimSpace(monkeySelector) == "" {
		monkeySelector = method
	}
	isMonkeyMatch := strings.EqualFold(strings.TrimSpace(method), "monkeyocrv2") ||
		strings.HasPrefix(layoutLower, "monkeyocrv2") ||
		strings.Contains(layoutLower, "@monkeyocrv2")
	isMonkeyByUUID := false
	if !isMonkeyMatch && tenantID != "" && strings.TrimSpace(monkeySelector) != "" && !parser.IsPDFParseMethod(monkeySelector) {
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

	// MinerU dispatch: parse_method "mineru", a layout_recognizer naming
	// MinerU, a composite model@instance@provider selector naming MinerU in
	// either field, or a bare tenant model UUID (in either field) that
	// resolves to a MinerU OCR model.
	methodLower := strings.ToLower(strings.TrimSpace(method))
	isMinerUMatch := methodLower == "mineru" ||
		strings.HasPrefix(layoutLower, "mineru") ||
		strings.Contains(layoutLower, "@mineru") ||
		strings.Contains(methodLower, "@mineru")
	minerUSelector := layout
	if strings.TrimSpace(minerUSelector) == "" {
		minerUSelector = method
	}
	isMinerUByUUID := false
	if !isMinerUMatch && strings.TrimSpace(minerUSelector) != "" &&
		!parser.IsPDFParseMethod(minerUSelector) {
		isMinerUByUUID = isMinerULayoutModelID(ctx, db, tenantID, minerUSelector)
	}
	if isMinerUMatch || isMinerUByUUID {
		common.Info("pdf vision dispatch: MinerU branch matched",
			zap.String("parse_method", method),
			zap.String("layout_recognizer", layout),
			zap.String("tenant_id", tenantID))
		if tenantID == "" {
			return parser.ParseResult{}, true,
				fmt.Errorf("parser: MinerU requires tenant_id")
		}
		dispatchModelID := ""
		if isMinerUByUUID || strings.Contains(minerUSelector, "@") {
			dispatchModelID = minerUSelector
		}
		res, err := dispatchMinerUPDF(ctx, db, filename, binary, tenantID, setup, dispatchModelID)
		if err != nil {
			return parser.ParseResult{}, true, err
		}
		return res, true, nil
	}

	// PaddleOCR dispatch: parse_method "paddleocr", a layout_recognizer whose
	// provider/selectors name PaddleOCR, or a bare tenant model UUID (in
	// either parse_method or layout_recognizer) that resolves to a PaddleOCR
	// OCR model. A bare UUID carries no provider spelling in the string, so it
	// is resolved first - mirroring Python's get_composite_model_name_by_id +
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
		!parser.IsPDFParseMethod(paddleOCRSelector) {
		isPaddleOCRByUUID = isPaddleOCRLayoutModelID(ctx, db, tenantID, paddleOCRSelector)
	}
	if isPaddleOCRMatch || isPaddleOCRByUUID {
		if tenantID == "" {
			return parser.ParseResult{}, true,
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
			return parser.ParseResult{}, true, err
		}
		return res, true, nil
	}

	// SoMark dispatch: parse_method "somark", a layout_recognizer naming
	// SoMark, or a bare tenant model UUID (in either parse_method or
	// layout_recognizer) that resolves to a SoMark OCR model. A resolved
	// tenant instance overrides the parser's setup/env SoMark configuration
	// with the instance's base URL and API key and runs the same SoMark
	// parser path; when the tenant has no SoMark instance the branch falls
	// through so the setup/env-driven parser path (SOMARK_BASE_URL et al.)
	// keeps working unchanged.
	isSoMarkMatch := strings.EqualFold(strings.TrimSpace(method), "somark") ||
		strings.HasPrefix(layoutLower, "somark") ||
		strings.Contains(layoutLower, "@somark")
	soMarkSelector := layout
	if strings.TrimSpace(soMarkSelector) == "" {
		soMarkSelector = method
	}
	isSoMarkByUUID := false
	if !isSoMarkMatch && tenantID != "" && strings.TrimSpace(soMarkSelector) != "" &&
		!parser.IsPDFParseMethod(soMarkSelector) {
		isSoMarkByUUID = isSoMarkLayoutModelID(ctx, db, tenantID, soMarkSelector)
	}
	if isSoMarkMatch || isSoMarkByUUID {
		if tenantID != "" {
			dispatchModelID := ""
			if isSoMarkByUUID || strings.Contains(soMarkSelector, "@") {
				dispatchModelID = soMarkSelector
			}
			if res, handled, err := dispatchSoMarkPDF(ctx, db, filename, binary, tenantID, setup, dispatchModelID); handled {
				return res, true, err
			}
		}
		// No tenant SoMark instance: fall through to the setup/env-driven
		// parser path.
	}

	modelID, useVision := resolvePDFVisionModelID(setup)
	if !useVision {
		return parser.ParseResult{}, false, nil
	}
	if tenantID == "" {
		return parser.ParseResult{}, true, fmt.Errorf(
			`parser: pdf parse_method %q requires tenant_id to resolve VLM model`, modelID)
	}
	pages, err := utility.NormalizePDFPages(setup["pages"])
	if err != nil {
		return parser.ParseResult{}, true, fmt.Errorf("parser: pdf vision pages: %w", err)
	}
	res, err := dispatchPDFVision(ctx, db, filename, binary, tenantID, modelID, pages)
	if err != nil {
		return parser.ParseResult{}, true, err
	}
	return res, true, nil
}

func dispatchMonkeyOCRv2PDF(ctx context.Context, db *gorm.DB, filename string, binary []byte, tenantID string, setup schema.ParserSetup, modelRef string) (parser.ParseResult, error) {
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
				return parser.ParseResult{}, fmt.Errorf("parser: MonkeyOCRv2 requires a MonkeyOCRv2 OCR model; found %q", driver.Name())
			}
			if apiConfig != nil && apiConfig.BaseURL != nil && strings.TrimSpace(*apiConfig.BaseURL) != "" {
				baseURL = strings.TrimRight(*apiConfig.BaseURL, "/")
			} else if configuredURL := modelModule.ProviderJSONConfigValueFromAPIConfig(apiConfig, "monkeyocrv2_server_url"); configuredURL != "" {
				baseURL = strings.TrimRight(configuredURL, "/")
			}
			timeout = monkeyOCRv2RequestTimeout(setup, apiConfig)
		} else if baseURL == "" {
			return parser.ParseResult{}, fmt.Errorf("parser: MonkeyOCRv2 model: %w", err)
		}
	}
	if baseURL == "" {
		return parser.ParseResult{}, fmt.Errorf("parser: MonkeyOCRv2 requires monkeyocrv2_server_url or MONKEYOCRV2_SERVER_URL")
	}
	zipBytes, err := monkeyOCRv2ParseRequest(ctx, baseURL+"/parse", filename, binary, timeout)
	if err != nil {
		return parser.ParseResult{}, err
	}
	sections, err := monkeyOCRv2ExtractSections(zipBytes)
	if err != nil {
		return parser.ParseResult{}, err
	}
	md := strings.Join(sections, "\n\n")
	return normalizePDFMarkdown(ctx, filename, md, setup), nil
}

// normalizePDFMarkdown converts a PDF backend Markdown response (MonkeyOCRv2,
// MinerU V1 fallback) into the structured JSON contract via the parser
// package's NormalizeMarkdown. This keeps the markdown→JSON boundary in the
// parser package, matching the NormalizeResult path used for JSON backends, and
// lets the downstream NormalizeResult closure stay idempotent for JSON.
func normalizePDFMarkdown(ctx context.Context, filename, md string, setup schema.ParserSetup) parser.ParseResult {
	flatten, _ := setup["flatten_media_to_text"].(bool)
	return (&parser.PDFParser{FlattenMediaToText: flatten}).NormalizeMarkdown(ctx, filename, md)
}

var isMonkeyOCRv2LayoutModelID = defaultIsMonkeyOCRv2LayoutModelID

func defaultIsMonkeyOCRv2LayoutModelID(ctx context.Context, db *gorm.DB, tenantID, modelID string) bool {
	if db == nil || strings.TrimSpace(modelID) == "" {
		return false
	}
	resolved, err := resolveModelConfigByID(ctx, db, tenantID, entity.ModelTypeOCR, modelID)
	return err == nil && strings.EqualFold(resolved.driver.Name(), "monkeyocrv2")
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

func monkeyOCRv2RequestTimeout(setup schema.ParserSetup, apiConfig *modelModule.APIConfig) time.Duration {
	value := ""
	if raw, ok := setup["monkeyocrv2_timeout"]; ok {
		value = strings.TrimSpace(fmt.Sprint(raw))
	}
	if value == "" {
		value = modelModule.ProviderJSONConfigValueFromAPIConfig(apiConfig, "monkeyocrv2_timeout", common.EnvMonkeyOCRv2Timeout)
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

type mineruExtractionOptions struct {
	formulaEnable bool
	tableEnable   bool
}

// readMinerUExtractionOptions keeps defaults and validation identical for
// component construction and dispatch. Only absence means "enabled".
func readMinerUExtractionOptions(setup schema.ParserSetup) (mineruExtractionOptions, error) {
	options := mineruExtractionOptions{formulaEnable: true, tableEnable: true}
	for _, field := range []struct {
		key   string
		value *bool
	}{
		{"mineru_formula_enable", &options.formulaEnable},
		{"mineru_table_enable", &options.tableEnable},
	} {
		if raw, exists := setup[field.key]; exists {
			value, ok := raw.(bool)
			if !ok {
				return options, fmt.Errorf("pdf.%s must be a boolean", field.key)
			}
			*field.value = value
		}
	}
	return options, nil
}

// dispatchMinerUPDF submits a PDF to the selected MinerU OCR model.
// MinerU 4.0 uses the V1 upload/job API; 3.x still POSTs /file_parse with
// stream=True and returns a zip body (Python mineru_parser.py:parse_PDF).
func dispatchMinerUPDF(
	ctx context.Context,
	db *gorm.DB,
	filename string,
	binary []byte,
	tenantID string,
	setup schema.ParserSetup,
	modelID string,
) (parser.ParseResult, error) {
	extraction, err := readMinerUExtractionOptions(setup)
	if err != nil {
		return parser.ParseResult{}, err
	}
	driver, _, apiConfig, err := resolveMinerUModelForDispatch(ctx, db, tenantID, modelID)
	if err != nil {
		return parser.ParseResult{}, fmt.Errorf("parser: MinerU model: %w", err)
	}
	if !isMinerUDriver(driver) {
		return parser.ParseResult{}, fmt.Errorf(
			"parser: MinerU requires a MinerU OCR model; found %q. Please add a MinerU OCR model to your tenant", driver.Name())
	}

	apiKeyRaw := ""
	if apiConfig.ApiKey != nil {
		apiKeyRaw = *apiConfig.ApiKey
	}

	baseURL := modelModule.ResolveMinerUAPIServer(apiConfig)
	if baseURL == "" {
		baseURL, _ = resolveMinerUBaseURL(driver, apiConfig)
	}
	if strings.TrimSpace(baseURL) == "" {
		return parser.ParseResult{}, fmt.Errorf("parser: MinerU requires a base URL (instance base_url, mineru_apiserver, or MINERU_APISERVER)")
	}

	parseMethod := mineruAPIParseMethod(getStringOr(setup, "mineru_parse_method", ""))
	// MinerU's language is an OCR engine parameter, not the caption language:
	// mineru_lang wins, then the family lang, then the engine's own default.
	// That default used to arrive implicitly through defaultSetups() - which
	// also fed captions, so a knowledge base that never chose a language got
	// Chinese captions (issue #20727). Keeping it here confines it to the engine.
	lang := firstNonEmpty(getStringOr(setup, "mineru_lang", ""), getStringOr(setup, "lang", ""))
	if lang == "" {
		lang = minerUDefaultLanguage
	}
	mineruLang := mineruLangCode(lang)
	backend := modelModule.ResolveMinerUBackend(getStringOr(setup, "mineru_backend", ""), apiKeyRaw)
	serverURL := modelModule.ResolveMinerUServerURL(getStringOr(setup, "mineru_server_url", ""), apiKeyRaw)

	if modelModule.IsMinerUCloudBase(baseURL) {
		if !extraction.formulaEnable || !extraction.tableEnable {
			return parser.ParseResult{}, fmt.Errorf("parser: MinerU cloud does not support disabling formula or table extraction")
		}
		if err := modelModule.ValidateMinerUConfigForAPI(backend, serverURL, true); err != nil {
			return parser.ParseResult{}, err
		}
		result, err := modelModule.ParseMinerUCloud(ctx, baseURL, apiKeyRaw, filename, binary, backend, parseMethod, mineruLang)
		if err != nil {
			return parser.ParseResult{}, fmt.Errorf("parser: MinerU cloud: %w", err)
		}
		return finishMinerURemoteResult(ctx, filename, binary, result, setup)
	}

	v1, err := modelModule.MinerUSupportsV1(ctx, baseURL, apiKeyRaw)
	if err != nil {
		return parser.ParseResult{}, fmt.Errorf("parser: MinerU probe: %w", err)
	}
	if err := modelModule.ValidateMinerUConfigForAPI(backend, serverURL, v1); err != nil {
		return parser.ParseResult{}, err
	}

	if v1 {
		// V1's strict CreateJobRequest has no extraction switches. Reject
		// unsupported choices before creating an upload or parse job.
		if !extraction.formulaEnable || !extraction.tableEnable {
			return parser.ParseResult{}, fmt.Errorf("parser: MinerU V1 does not support disabling formula or table extraction; use a /file_parse server to configure these switches")
		}
		result, err := modelModule.ParseMinerUV1(ctx, baseURL, apiKeyRaw, filename, binary, backend, parseMethod, 0)
		if err != nil {
			return parser.ParseResult{}, fmt.Errorf("parser: MinerU V1: %w", err)
		}
		return finishMinerURemoteResult(ctx, filename, binary, result, setup)
	}

	apiURL := strings.TrimRight(baseURL, "/") + "/file_parse"
	zipBytes, err := mineruStreamParse(ctx, apiURL, apiKeyRaw, binary, parseMethod, mineruLang, backend, serverURL, extraction)
	if err != nil {
		return parser.ParseResult{}, fmt.Errorf("parser: MinerU stream: %w", err)
	}

	sizes, geometryErr := mineruPageSizes(ctx, binary)
	items, err := mineruExtractJSONItems(zipBytes, sizes)
	if err != nil {
		return parser.ParseResult{}, fmt.Errorf("parser: MinerU extract: %w", err)
	}

	res := mineruJSONParseResult(filename, items)
	appendMineruGeometryWarning(&res, geometryErr)
	return res, nil
}

func finishMinerURemoteResult(ctx context.Context, filename string, binary []byte, result *modelModule.MinerUV1Result, setup schema.ParserSetup) (parser.ParseResult, error) {
	markdown := ""
	var zipBytes []byte
	if result != nil {
		markdown = strings.TrimSpace(result.Markdown)
		zipBytes = result.Zip
	}
	var sizes map[int][2]float64
	var geometryErr error
	if len(zipBytes) > 0 {
		sizes, geometryErr = mineruPageSizes(ctx, binary)
	}
	items, extractErr := mineruExtractJSONItems(zipBytes, sizes)
	if extractErr == nil && len(items) > 0 {
		res := mineruJSONParseResult(filename, items)
		appendMineruGeometryWarning(&res, geometryErr)
		return res, nil
	}
	if markdown != "" {
		return normalizePDFMarkdown(ctx, filename, result.Markdown, setup), nil
	}
	if extractErr != nil {
		return parser.ParseResult{}, fmt.Errorf("parser: MinerU extract: %w", extractErr)
	}
	return parser.ParseResult{}, fmt.Errorf("parser: MinerU extract: no sections")
}

func mineruHTTPErrorText(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "<!doctype") || strings.HasPrefix(lower, "<html") {
		return "HTML error page; the MinerU base URL is not an API endpoint"
	}
	return text
}

// appendMineruGeometryWarning records a failed page-size lookup. Without the
// lookup MinerU items keep their text and page identity but carry no
// positions, because content_list boxes are page-normalized, not PDF points.
func appendMineruGeometryWarning(res *parser.ParseResult, err error) {
	if err != nil {
		res.Warnings = append(res.Warnings, "MinerU PDF geometry unavailable: "+err.Error())
	}
}

// resolveMinerUModelForDispatch resolves the OCR model used by the MinerU PDF
// dispatch. modelID is the raw selector value: a bare tenant model UUID (no
// "@") selects that exact model, a composite model@instance@provider name
// resolves through the provider-instance chain, and an empty value falls back
// to the tenant's first MinerU OCR model - mirroring Python's by_mineru,
// which falls back to get_first_provider_model_name(tenant_id, "MinerU",
// LLMType.OCR) for the named "mineru" selector.
var resolveMinerUModelForDispatch = defaultResolveMinerUModelForDispatch

func defaultResolveMinerUModelForDispatch(ctx context.Context, db *gorm.DB, tenantID, modelID string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID != "" && !strings.Contains(modelID, "@") {
		resolved, err := resolveModelConfigByID(ctx, db, tenantID, entity.ModelTypeOCR, modelID)
		if err != nil {
			return nil, "", nil, err
		}
		return resolved.driver, resolved.model.ModelName, resolved.apiConfig, nil
	}
	if modelID != "" {
		driver, modelName, apiConfig, _, err := resolveModelConfig(ctx, db, tenantID, entity.ModelTypeOCR, modelID)
		return driver, modelName, apiConfig, err
	}
	driver, modelName, apiConfig, _, err := resolveTenantOCRModelByProvider(ctx, db, tenantID, "MinerU")
	return driver, modelName, apiConfig, err
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
// returns the tenant's first active PaddleOCR OCR model - potentially the
// local one - when both the local ("PaddleOCR.local") and the cloud
// ("PaddleOCR") providers are configured. The Python path behaves the same
// way; if exact selection is required, pass the model's tenant-model UUID
// instead.
var resolvePaddleOCRModelForDispatch = defaultResolvePaddleOCRModelForDispatch

func defaultResolvePaddleOCRModelForDispatch(ctx context.Context, db *gorm.DB, tenantID, modelID string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
	if strings.TrimSpace(modelID) != "" && !strings.Contains(modelID, "@") {
		resolved, err := resolveModelConfigByID(ctx, db, tenantID, entity.ModelTypeOCR, modelID)
		if err != nil {
			return nil, "", nil, err
		}
		return resolved.driver, resolved.model.ModelName, resolved.apiConfig, nil
	}
	driver, modelName, apiConfig, _, err := resolveTenantOCRModelByProvider(ctx, db, tenantID, "PaddleOCR")
	return driver, modelName, apiConfig, err
}

// dispatchPaddleOCRPdf resolves the tenant's cloud or local OCR model and
// converts its page blocks to the structured PDF parser output.
func dispatchPaddleOCRPdf(
	ctx context.Context,
	db *gorm.DB,
	filename string,
	binary []byte,
	tenantID string,
	setup schema.ParserSetup,
	modelID string,
) (parser.ParseResult, error) {
	driver, modelName, apiConfig, err := resolvePaddleOCRModelForDispatch(ctx, db, tenantID, modelID)
	if err != nil {
		return parser.ParseResult{}, fmt.Errorf("parser: PaddleOCR model: %w", err)
	}
	if !isPaddleOCRDriver(driver) {
		return parser.ParseResult{}, fmt.Errorf(
			"parser: PaddleOCR requires a PaddleOCR OCR model; found %q. Please add a PaddleOCR OCR model to your tenant", driver.Name())
	}

	resp, err := driver.OCRFile(ctx, &modelName, binary, &filename, apiConfig, &modelModule.OCRConfig{
		Algorithm: strings.TrimSpace(getStringOr(setup, "paddleocr_algorithm", "")),
	}, nil)
	if err != nil {
		return parser.ParseResult{}, fmt.Errorf("parser: PaddleOCR OCRFile: %w", err)
	}

	result := parser.PaddleOCRPDFResult(ctx, filename, binary, resp)
	return result, result.Err
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

// mineruAPIParseMethod constrains the /file_parse form field to the values
// the MinerU API accepts (auto, txt, ocr), mirroring Python's
// MinerUParseMethod. The value comes from the parser setup's dedicated
// mineru_parse_method option; the setup's parse_method is a dispatch
// selector ("mineru", a composite selector, or a tenant model UUID), never
// an API method, so anything unrecognized falls back to the API default.
func mineruAPIParseMethod(raw string) string {
	method := strings.ToLower(strings.TrimSpace(raw))
	switch method {
	case "auto", "txt", "ocr":
		return method
	}
	return "auto"
}

// mineruStreamTimeout bounds a single /file_parse upload. It matches the
// MinerU 4 V1 parse budget so both API generations give up at the same point.
const mineruStreamTimeout = 30 * time.Minute

// mineruStreamParse POSTs the PDF binary to the MinerU /file_parse
// endpoint with streaming and returns the zip response body.
// Mirrors Python's mineru_parser.py._run_mineru_api with stream=True.
func mineruStreamParse(ctx context.Context, apiURL string, apiKey string, binary []byte, parseMethod, lang, backend, serverURL string, extraction mineruExtractionOptions) ([]byte, error) {
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
	_ = writer.WriteField("formula_enable", strconv.FormatBool(extraction.formulaEnable))
	_ = writer.WriteField("table_enable", strconv.FormatBool(extraction.tableEnable))
	if strings.TrimSpace(serverURL) != "" {
		_ = writer.WriteField("server_url", strings.TrimRight(strings.TrimSpace(serverURL), "/"))
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finalize form: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, mineruStreamTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, &body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token := modelModule.MinerUBearerTokenFromAPIKey(apiKey); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("HTTP %d: read error response: %w", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, mineruHTTPErrorText(raw))
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

const (
	mineruMaxImageBytes = 20 << 20
	// mineruMaxMiddleBytes bounds middle.json, which carries the full
	// layout/span payload and can legitimately reach tens of megabytes on
	// long documents, while still refusing a zip-bomb-sized member.
	mineruMaxMiddleBytes = 64 << 20
)

// mineruContentBBoxScale is the divisor MinerU applies when writing
// content_list bboxes: each edge is normalized to 0-1000 over the page box.
const mineruContentBBoxScale = 1000.0

// mineruExtractJSONItems reads MinerU content_list.json, or MinerU 4
// structured_content.json when the zip has no content list, and emits JSON
// parser items with captions, embedded zip images, and 1-indexed
// _pdf_positions so jump-to-source works like DeepDoc. pdfPageSizes comes
// from the source PDF (pdfium); middle.json in the zip fills any missing
// pages. content_list bboxes are normalized to 0-1000, so positions require
// a known page box in PDF points. Structured content boxes are 0-1 and are
// scaled onto that same 0-1000 space before conversion.
func mineruExtractJSONItems(zipBytes []byte, pdfPageSizes map[int][2]float64) ([]map[string]any, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}

	images := make(map[string]*zip.File)
	var contentList, middleJSON, structured []byte
	for _, f := range zipReader.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		switch {
		case strings.HasSuffix(name, "content_list.json") && len(contentList) == 0:
			contentList, err = mineruReadZipFile(f)
		case strings.HasSuffix(name, "structured_content.json") && len(structured) == 0:
			structured, err = mineruReadZipFile(f)
		}
		if err != nil {
			return nil, err
		}
		// middle.json is optional: it only carries page_size, which lets
		// positions be converted to points. A zip without it keeps the
		// pre-existing passthrough behavior instead of failing the parse,
		// and so does an oversized (or lying-header) member.
		if strings.HasSuffix(name, "middle.json") && len(middleJSON) == 0 {
			if data, ok := mineruReadBounded(f, mineruMaxMiddleBytes); ok {
				middleJSON = data
			}
		}
		if strings.HasPrefix(strings.ToLower(mime.TypeByExtension(path.Ext(name))), "image/") {
			clean := strings.TrimPrefix(path.Clean(name), "./")
			images[path.Base(name)] = f
			images[clean] = f
			images[name] = f
		}
	}

	var rawItems []map[string]any
	switch {
	case len(contentList) > 0:
		if err = json.Unmarshal(contentList, &rawItems); err != nil {
			return nil, fmt.Errorf("parse content_list.json: %w", err)
		}
	case len(structured) > 0:
		rawItems, err = mineruItemsFromStructuredContent(structured)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("content_list.json not found in MinerU zip")
	}

	pageSizes := mergeMineruPageSizes(pdfPageSizes, mineruPageSizesFromMiddleJSON(middleJSON))
	out := make([]map[string]any, 0, len(rawItems))
	for _, item := range rawItems {
		converted := mineruContentItemToJSON(item, images, pageSizes)
		if converted != nil {
			out = append(out, converted)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("MinerU zip contained no usable content_list items")
	}
	return out, nil
}

func mineruReadZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", f.Name, err)
	}
	body, err := io.ReadAll(rc)
	if closeErr := rc.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", f.Name, err)
	}
	return body, nil
}

func mineruItemsFromStructuredContent(raw []byte) ([]map[string]any, error) {
	var doc struct {
		Pages []struct {
			PageIdx int              `json:"page_idx"`
			Blocks  []map[string]any `json:"blocks"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse structured_content.json: %w", err)
	}
	items := make([]map[string]any, 0)
	for _, page := range doc.Pages {
		for _, block := range page.Blocks {
			if item := mineruStructuredBlockToContentItem(page.PageIdx, block); item != nil {
				items = append(items, item)
			}
		}
	}
	return items, nil
}

func mineruStructuredBlockToContentItem(pageIdx int, block map[string]any) map[string]any {
	typ, _ := block["type"].(string)
	typ = strings.ToLower(strings.TrimSpace(typ))
	switch typ {
	case "header", "footer", "page_number", "page-header", "page-footer", "page_header", "page_footer", "aside_text", "discarded":
		return nil
	}
	content, _ := block["content"].(string)
	captions := mineruAnnotationTexts(block["captions"])
	footnotes := mineruAnnotationTexts(block["footnotes"])
	item := map[string]any{"page_idx": pageIdx}
	if bbox := mineruScaleStructuredBBox(block["bbox"]); bbox != nil {
		item["bbox"] = bbox
	}
	switch typ {
	case "doc_title", "paragraph_title":
		item["type"] = "title"
		item["text"] = content
	case "table":
		item["type"] = "table"
		item["table_body"] = content
		item["table_caption"] = captions
		item["table_footnote"] = footnotes
	case "image":
		item["type"] = "image"
		item["image_caption"] = captions
		item["image_footnote"] = footnotes
		if strings.TrimSpace(content) != "" {
			item["vlm_description"] = content
		}
		mineruApplyImageSource(item, block["image_source"])
	case "chart":
		item["type"] = "chart"
		item["chart_caption"] = captions
		item["chart_footnote"] = footnotes
		if strings.TrimSpace(content) != "" {
			item["vlm_description"] = content
		}
		mineruApplyImageSource(item, block["image_source"])
	case "list":
		item["type"] = "list"
		if content != "" {
			item["list_items"] = []any{content}
		}
	case "code":
		item["type"] = "code"
		item["code_body"] = content
	default:
		item["type"] = typ
		item["text"] = content
		mineruApplyImageSource(item, block["image_source"])
	}
	return item
}

func mineruAnnotationTexts(raw any) []any {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		switch value := item.(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				out = append(out, value)
			}
		case map[string]any:
			if text, _ := value["content"].(string); strings.TrimSpace(text) != "" {
				out = append(out, text)
			}
		}
	}
	return out
}

func mineruApplyImageSource(item map[string]any, raw any) {
	source, _ := raw.(string)
	source = strings.TrimSpace(source)
	if source == "" {
		return
	}
	if strings.HasPrefix(source, "data:") {
		item["image_data_uri"] = source
		return
	}
	item["img_path"] = source
}

// mineruScaleStructuredBBox maps MinerU 4 structured_content boxes (0-1) onto
// the 0-1000 space used by content_list before PDF-point conversion.
func mineruScaleStructuredBBox(raw any) []any {
	bbox := mineruJSONFloatSlice(raw)
	if len(bbox) < 4 {
		return nil
	}
	maxValue := bbox[0]
	for _, value := range bbox[1:] {
		if value > maxValue {
			maxValue = value
		}
	}
	if maxValue <= 1 {
		for i, value := range bbox {
			bbox[i] = value * mineruContentBBoxScale
		}
	}
	out := make([]any, len(bbox))
	for i, value := range bbox {
		out[i] = value
	}
	return out
}

// mineruPageSizesFromMiddleJSON parses the per-page page_size ([width, height]
// in PDF points, written by MinerU's pdfium page.get_size()) out of
// middle.json, keyed by 0-based page_idx. A map rather than a slice because
// partial jobs are legal and preserve the source page indices, so pdf_info can
// be sparse (entries for pages 5 and 6 only). Entries without a usable page_idx
// fall back to their position in pdf_info. Returns nil when middle.json is
// absent or unparseable.
func mineruPageSizesFromMiddleJSON(middleJSON []byte) map[int][2]float64 {
	if len(middleJSON) == 0 {
		return nil
	}
	var parsed struct {
		PdfInfo []struct {
			PageIdx  *int      `json:"page_idx"`
			PageSize []float64 `json:"page_size"`
		} `json:"pdf_info"`
	}
	if err := json.Unmarshal(middleJSON, &parsed); err != nil || len(parsed.PdfInfo) == 0 {
		return nil
	}
	sizes := make(map[int][2]float64, len(parsed.PdfInfo))
	for i, page := range parsed.PdfInfo {
		if len(page.PageSize) < 2 {
			continue
		}
		idx := i
		if page.PageIdx != nil && *page.PageIdx >= 0 {
			idx = *page.PageIdx
		}
		sizes[idx] = [2]float64{page.PageSize[0], page.PageSize[1]}
	}
	if len(sizes) == 0 {
		return nil
	}
	return sizes
}

func mergeMineruPageSizes(pdfSizes, middleSizes map[int][2]float64) map[int][2]float64 {
	if len(pdfSizes) == 0 {
		return middleSizes
	}
	if len(middleSizes) == 0 {
		return pdfSizes
	}
	out := make(map[int][2]float64, len(pdfSizes)+len(middleSizes))
	for idx, size := range middleSizes {
		out[idx] = size
	}
	for idx, size := range pdfSizes {
		out[idx] = size
	}
	return out
}

func mineruContentItemToJSON(item map[string]any, images map[string]*zip.File, pageSizes map[int][2]float64) map[string]any {
	typ, _ := item["type"].(string)
	typ = strings.ToLower(strings.TrimSpace(typ))
	switch typ {
	case "header", "footer", "page_number", "page-header", "page-footer", "discarded":
		return nil
	}

	text := mineruItemText(item, typ)
	imageURI := ""
	docType := "text"
	// External MinerU labels are mapped explicitly. Unknown labels must not
	// acquire parser block-kind semantics implicitly; list/code are real
	// structural types and stay structural.
	layout := "text"
	switch typ {
	case "title":
		layout = "title"
	case "list":
		layout = "list"
	case "code":
		layout = "code"
	case "equation":
		layout = "equation"
	case "image", "chart":
		docType = "image"
		layout = "figure"
		if imgPath, _ := item["img_path"].(string); imgPath != "" {
			imageURI = mineruZipImageDataURI(images, imgPath)
		}
		if imageURI == "" {
			if dataURI, _ := item["image_data_uri"].(string); strings.HasPrefix(dataURI, "data:") {
				imageURI = dataURI
			}
		}
		if text == "" && imageURI == "" {
			return nil
		}
	case "table":
		docType = "table"
		layout = "table"
		if text == "" {
			return nil
		}
	default:
		if text == "" {
			return nil
		}
	}

	jsonItem := map[string]any{
		"text":         text,
		"doc_type_kwd": docType,
		"layout":       layout,
	}
	// MinerU folds its own VLM description into text for image/chart items.
	// Mark them so the shared vision enhancement skips them instead of
	// appending a second model description.
	if typ == "image" || typ == "chart" {
		if desc, _ := item["vlm_description"].(string); strings.TrimSpace(desc) != "" {
			jsonItem[backendDescribedKey] = true
		}
	}
	if imageURI != "" {
		jsonItem["image"] = imageURI
	}
	// Page identity does not depend on geometry: it drives table placement and
	// page_count even when positions cannot be produced.
	if pageIdx, ok := mineruJSONInt(item["page_idx"]); ok && pageIdx >= 0 {
		jsonItem["page_number"] = pageIdx + 1
	}
	if positions := mineruItemPositions(item, pageSizes); len(positions) > 0 {
		jsonItem["_pdf_positions"] = positions
		jsonItem["positions"] = positions
	}
	if typ == "table" {
		body, _ := item["table_body"].(string)
		if table, err := tableutil.ParseTableHTML(body); err == nil && table != nil && len(table.Grid) > 0 {
			caption := strings.Join(stringSlice(item["table_caption"]), "\n")
			if caption != "" && caption != table.Caption {
				table.Caption = strings.TrimSpace(table.Caption + "\n" + caption)
			}
			if page, ok := jsonItem["page_number"].(int); ok {
				table.Page = page
			}
			jsonItem["table"] = table
			jsonItem["text"] = strings.Join(stringSlice(item["table_footnote"]), "\n")
		}
	}
	return jsonItem
}

func mineruItemText(item map[string]any, typ string) string {
	var parts []string
	switch typ {
	case "table":
		if tb, ok := item["table_body"].(string); ok {
			parts = append(parts, tb)
		}
		parts = append(parts, stringSlice(item["table_caption"])...)
		parts = append(parts, stringSlice(item["table_footnote"])...)
	case "image":
		parts = append(parts, stringSlice(item["image_caption"])...)
		parts = append(parts, stringSlice(item["image_footnote"])...)
		if desc, ok := item["vlm_description"].(string); ok && strings.TrimSpace(desc) != "" {
			parts = append(parts, desc)
		}
	case "chart":
		parts = append(parts, stringSlice(item["chart_caption"])...)
		parts = append(parts, stringSlice(item["chart_footnote"])...)
		if desc, ok := item["vlm_description"].(string); ok && strings.TrimSpace(desc) != "" {
			parts = append(parts, desc)
		}
	case "list":
		parts = append(parts, stringSlice(item["list_items"])...)
	case "code":
		if body, ok := item["code_body"].(string); ok && body != "" {
			parts = append(parts, body)
		} else if text, ok := item["text"].(string); ok {
			parts = append(parts, text)
		}
		parts = append(parts, stringSlice(item["code_caption"])...)
	default:
		if text, ok := item["text"].(string); ok {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func mineruItemPositions(item map[string]any, pageSizes map[int][2]float64) [][]any {
	if raw, ok := item["_mineru_positions"].([]any); ok {
		var out [][]any
		for _, entry := range raw {
			posMap, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if pos := mineruBBoxPosition(posMap["page_idx"], posMap["bbox"], pageSizes); pos != nil {
				out = append(out, pos)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if pos := mineruBBoxPosition(item["page_idx"], item["bbox"], pageSizes); pos != nil {
		return [][]any{pos}
	}
	return nil
}

// mineruBBoxPosition converts a content_list bbox ([x0, top, x1, bottom],
// 0-1000 normalized) into the 1-based [page, left, right, top, bottom] row in
// PDF points. The page size is required: emitting the normalized values as-is
// misaligns jump-to-source highlights (issues #21220, #21488).
func mineruBBoxPosition(pageRaw, bboxRaw any, pageSizes map[int][2]float64) []any {
	pageIdx, ok := mineruJSONInt(pageRaw)
	if !ok || pageIdx < 0 {
		return nil
	}
	size, ok := pageSizes[pageIdx]
	if !ok || size[0] <= 0 || size[1] <= 0 {
		return nil
	}
	bbox := mineruJSONFloatSlice(bboxRaw)
	if len(bbox) < 4 {
		return nil
	}
	x0, top, x1, bott := bbox[0], bbox[1], bbox[2], bbox[3]
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if top > bott {
		top, bott = bott, top
	}
	scaleX := size[0] / mineruContentBBoxScale
	scaleY := size[1] / mineruContentBBoxScale
	left := math.Max(0, math.Min(size[0], x0*scaleX))
	right := math.Max(0, math.Min(size[0], x1*scaleX))
	topPt := math.Max(0, math.Min(size[1], top*scaleY))
	bottomPt := math.Max(0, math.Min(size[1], bott*scaleY))
	if right <= left || bottomPt <= topPt {
		return nil
	}
	return []any{pageIdx + 1, left, right, topPt, bottomPt}
}

func mineruJSONInt(raw any) (int, bool) {
	switch v := raw.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		return int(n), err == nil
	default:
		return 0, false
	}
}

func mineruJSONFloatSlice(raw any) []float64 {
	switch v := raw.(type) {
	case []any:
		out := make([]float64, 0, len(v))
		for _, item := range v {
			switch n := item.(type) {
			case float64:
				out = append(out, n)
			case int:
				out = append(out, float64(n))
			case int64:
				out = append(out, float64(n))
			case json.Number:
				f, err := n.Float64()
				if err != nil {
					return nil
				}
				out = append(out, f)
			default:
				return nil
			}
		}
		return out
	case []float64:
		return v
	case []int:
		out := make([]float64, len(v))
		for i, n := range v {
			out[i] = float64(n)
		}
		return out
	default:
		return nil
	}
}

// mineruReadBounded reads a zip member, refusing anything that is empty or
// larger than limit bytes. The size is checked both against the declared
// uncompressed size and against the bytes actually produced, since a
// hostile member can understate the former.
func mineruReadBounded(f *zip.File, limit int64) ([]byte, bool) {
	if f == nil || f.UncompressedSize64 > uint64(limit) {
		return nil, false
	}
	rc, err := f.Open()
	if err != nil {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	rc.Close()
	if err != nil || len(data) == 0 || int64(len(data)) > limit {
		return nil, false
	}
	return data, true
}

func mineruZipImageDataURI(images map[string]*zip.File, imgPath string) string {
	imgPath = strings.TrimSpace(strings.ReplaceAll(imgPath, "\\", "/"))
	if imgPath == "" {
		return ""
	}
	candidates := []string{imgPath, path.Base(imgPath), strings.TrimPrefix(path.Clean(imgPath), "./")}
	var file *zip.File
	for _, key := range candidates {
		if f, ok := images[key]; ok {
			file = f
			break
		}
	}
	data, ok := mineruReadBounded(file, mineruMaxImageBytes)
	if !ok {
		return ""
	}
	mediaType := mime.TypeByExtension(path.Ext(file.Name))
	if mediaType == "" {
		mediaType = http.DetectContentType(data)
	}
	return fmt.Sprintf("data:%s;base64,%s", mediaType, base64.StdEncoding.EncodeToString(data))
}

func mineruJSONParseResult(filename string, items []map[string]any) parser.ParseResult {
	return parser.ParseResult{
		File: map[string]any{
			"name":         filename,
			"page_count":   mineruItemPageCount(items),
			"outline":      []map[string]any{},
			"parse_method": "mineru",
		},
		JSON: parser.MapsToItems(items),
	}
}

func mineruItemPageCount(items []map[string]any) int {
	maxPage := 0
	for _, item := range items {
		if page, ok := mineruJSONInt(item["page_number"]); ok && page > maxPage {
			maxPage = page
		}
	}
	return maxPage
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
		if method != "" && !parser.IsPDFParseMethod(method) {
			return method, true
		}
	}
	if raw, ok := setup["layout_recognizer"].(string); ok {
		method := strings.TrimSpace(raw)
		if method != "" && !parser.IsPDFParseMethod(method) {
			return method, true
		}
	}
	return "", false
}

func dispatchPDFVision(
	ctx context.Context,
	db *gorm.DB,
	filename string,
	binary []byte,
	tenantID string,
	modelID string,
	pages [][]int,
) (parser.ParseResult, error) {
	renderedPages, err := pdfVisionPageRenderer(binary, pages)
	if err != nil {
		return parser.ParseResult{}, fmt.Errorf("parser: pdf vision render: %w", err)
	}
	driver, resolvedModelName, apiConfig, err := pdfVisionModelResolver(ctx, db, tenantID, modelID)
	if err != nil {
		return parser.ParseResult{}, fmt.Errorf("parser: pdf vision model %q: %w", modelID, err)
	}
	promptTemplate, err := pdfVisionPromptLoader("vision_llm_describe_prompt")
	if err != nil {
		return parser.ParseResult{}, fmt.Errorf("parser: load vision prompt: %w", err)
	}

	items := make([]map[string]any, 0, len(renderedPages))
	for _, page := range renderedPages {
		prompt := renderPDFVisionPrompt(promptTemplate, page.PageNumber)
		resp, err := pdfVisionChatInvoker(ctx, driver, resolvedModelName, buildPDFVisionMessages(prompt, page.ImageURL), apiConfig)
		if err != nil {
			return parser.ParseResult{}, fmt.Errorf("parser: pdf vision page %d: %w", page.PageNumber, err)
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
	}

	fileMeta := map[string]any{
		"name":         filename,
		"page_count":   len(renderedPages),
		"outline":      []map[string]any{},
		"parse_method": modelID,
	}
	return parser.ParseResult{
		File: fileMeta,
		JSON: parser.MapsToItems(items),
	}, nil
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
	chatModel := modelModule.NewChatModel(driver, &modelName, apiConfig)
	return chatModel.ChatWithMessages(ctx, messages, &modelModule.ChatConfig{Vision: &vision}, nil)
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

// isMinerULayoutModelID reports whether selector - a bare tenant model UUID
// with no "model@instance@provider" composite hint - resolves to an active
// OCR model driven by a MinerU provider (remote mineru.net or local mineru).
// The raw UUID carries no provider spelling, so it must be resolved before
// the dispatch path is chosen instead of falling through to the image2text
// VLM path.
var isMinerULayoutModelID = defaultIsMinerULayoutModelID

func defaultIsMinerULayoutModelID(ctx context.Context, db *gorm.DB, tenantID, selector string) bool {
	selector = strings.TrimSpace(selector)
	if db == nil {
		return false
	}
	if selector == "" || strings.Contains(selector, "@") {
		return false
	}
	driver, _, _, err := defaultResolveMinerUModelForDispatch(ctx, db, tenantID, selector)
	if err != nil {
		return false
	}
	return isMinerUDriver(driver)
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

// isPaddleOCRLayoutModelID reports whether layout - a bare tenant model UUID
// with no "model@instance@provider" composite hint - resolves to an active
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

// resolveSoMarkModelForDispatch resolves the OCR model used by the SoMark PDF
// dispatch. modelID is the raw selector value: a bare tenant model UUID (no
// "@") selects that exact model, a composite model@instance@provider name
// resolves through the provider-instance chain, and an empty value falls back
// to the tenant's first SoMark OCR model.
var resolveSoMarkModelForDispatch = defaultResolveSoMarkModelForDispatch

func defaultResolveSoMarkModelForDispatch(ctx context.Context, db *gorm.DB, tenantID, modelID string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID != "" && !strings.Contains(modelID, "@") {
		resolved, err := resolveModelConfigByID(ctx, db, tenantID, entity.ModelTypeOCR, modelID)
		if err != nil {
			return nil, "", nil, err
		}
		return resolved.driver, resolved.model.ModelName, resolved.apiConfig, nil
	}
	if modelID != "" {
		driver, modelName, apiConfig, _, err := resolveModelConfig(ctx, db, tenantID, entity.ModelTypeOCR, modelID)
		return driver, modelName, apiConfig, err
	}
	driver, modelName, apiConfig, _, err := resolveTenantOCRModelByProvider(ctx, db, tenantID, "SoMark")
	return driver, modelName, apiConfig, err
}

// isSoMarkLayoutModelID reports whether selector - a bare tenant model UUID
// with no "model@instance@provider" composite hint - resolves to an active
// OCR model driven by the SoMark provider. Mirrors the PaddleOCR/MinerU
// probes: the raw UUID carries no provider spelling, so it must be resolved
// before the dispatch path is chosen instead of falling through to the
// image2text VLM path.
var isSoMarkLayoutModelID = defaultIsSoMarkLayoutModelID

func defaultIsSoMarkLayoutModelID(ctx context.Context, db *gorm.DB, tenantID, selector string) bool {
	selector = strings.TrimSpace(selector)
	if db == nil {
		return false
	}
	if selector == "" || strings.Contains(selector, "@") {
		return false
	}
	driver, _, _, err := defaultResolveSoMarkModelForDispatch(ctx, db, tenantID, selector)
	if err != nil {
		return false
	}
	return strings.EqualFold(driver.Name(), "somark")
}

// dispatchSoMarkPDF runs the SoMark parser path with the tenant model
// provider instance's endpoint and key overriding the parser setup/env
// configuration, so instance-driven parsing yields the same structured
// result (titles, tables, figures with layout) as the env-driven path.
// handled reports whether the dispatch consumed the request: resolution
// failures for an explicit model selector are handled errors; when no tenant
// model was selected, the caller falls through to the setup/env-driven path.
func dispatchSoMarkPDF(
	ctx context.Context,
	db *gorm.DB,
	filename string,
	binary []byte,
	tenantID string,
	setup schema.ParserSetup,
	modelID string,
) (parser.ParseResult, bool, error) {
	driver, _, apiConfig, err := resolveSoMarkModelForDispatch(ctx, db, tenantID, modelID)
	if err != nil {
		if strings.TrimSpace(modelID) != "" {
			return parser.ParseResult{}, true, fmt.Errorf("parser: resolve selected SoMark model %q: %w", modelID, err)
		}
		// No tenant SoMark instance (or unresolvable selector): let the
		// setup/env-driven parser path handle the request.
		return parser.ParseResult{}, false, nil
	}
	if !strings.EqualFold(driver.Name(), "somark") {
		return parser.ParseResult{}, true, fmt.Errorf(
			"parser: SoMark requires a SoMark OCR model; found %q. Please add a SoMark OCR model to your tenant", driver.Name())
	}

	p := parser.NewPDFParser()
	p.ConfigureFromSetup(setup)
	p.ParseMethod = "somark"
	if somarkDriver, ok := driver.(interface {
		GetBaseURL(*modelModule.APIConfig) (string, error)
	}); ok {
		if baseURL, err := somarkDriver.GetBaseURL(apiConfig); err == nil && strings.TrimSpace(baseURL) != "" {
			p.SoMarkBaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
		}
	}
	if apiConfig != nil {
		if apiConfig.ApiKey != nil && strings.TrimSpace(*apiConfig.ApiKey) != "" {
			p.SoMarkAPIKey = strings.TrimSpace(*apiConfig.ApiKey)
		}
	}
	res := p.ParseWithResult(ctx, filename, binary)
	if res.Err != nil {
		return parser.ParseResult{}, true, res.Err
	}
	return res, true, nil
}
