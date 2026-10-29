package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"ragflow/internal/common"
	"strings"

	pdfreader "github.com/ledongthuc/pdf"
)

// parsePDFWithSoMark submits a PDF and converts the task result into parser
// output. Explicit tenant configuration never borrows a global endpoint or key.
func parsePDFWithSoMark(ctx context.Context, filename string, data []byte, parser *PDFParser) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	baseURL := strings.TrimSpace(parser.SoMarkBaseURL)
	if baseURL == "" && !parser.UseTenantModelConfig {
		baseURL = strings.TrimSpace(common.GetEnv(common.EnvSOMarkBaseUrl))
	}
	if baseURL == "" {
		return ParseResult{Err: fmt.Errorf("parser: SoMark requires somark_base_url or SOMARK_BASE_URL")}
	}
	apiKey := parser.SoMarkAPIKey
	if strings.TrimSpace(apiKey) == "" && !parser.UseTenantModelConfig {
		apiKey = strings.TrimSpace(common.GetEnv(common.EnvSOMarkAPIKey))
	}
	taskID, err := soMarkSubmit(ctx, strings.TrimRight(baseURL, "/"), filename, data, parser, apiKey)
	if err != nil {
		return ParseResult{Err: err}
	}
	result, err := soMarkPoll(ctx, strings.TrimRight(baseURL, "/"), taskID, apiKey)
	if err != nil {
		return ParseResult{Err: err}
	}
	items, pageCount, warnings := soMarkItems(ctx, result, parser.SoMarkKeepHeaderFooter, soMarkPagePointSizes(data))
	if err := ctx.Err(); err != nil {
		return ParseResult{Err: err}
	}
	if len(items) == 0 {
		return ParseResult{Err: fmt.Errorf("parser: SoMark returned no usable blocks")}
	}
	parsed := pdfItemsToResult(filename, items, pageCount)
	parsed.Warnings = warnings
	return parsed
}

// soMarkSubmit uploads a PDF with the supplied credentials and parser options,
// applying environment defaults only when no tenant instance was selected.
func soMarkSubmit(ctx context.Context, baseURL, filename string, data []byte, parser *PDFParser, apiKey string) (string, error) {
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
		"image":   parser.envOrDefault(common.EnvSOMarkImageFormat, parser.SoMarkImageFormat, "url"),
		"formula": parser.envOrDefault(common.EnvSOMarkFormulaFormat, parser.SoMarkFormulaFormat, "latex"),
		"table":   parser.envOrDefault(common.EnvSOMarkTableFormat, parser.SoMarkTableFormat, "html"),
		"cs":      parser.envOrDefault(common.EnvSOMarkCSFormat, parser.SoMarkCSFormat, "image"),
	})
	featureConfig, _ := json.Marshal(map[string]any{
		"enable_text_cross_page":         parser.envOrBool(common.EnvSOMarkEnableTextCrossPage, parser.SoMarkEnableTextCrossPage),
		"enable_table_cross_page":        parser.envOrBool(common.EnvSOMarkEnableTableCrossPage, parser.SoMarkEnableTableCrossPage),
		"enable_title_level_recognition": parser.envOrBool(common.EnvSOMarkEnableTitleLevelRecognition, parser.SoMarkEnableTitleLevelRecognition),
		"enable_inline_image":            parser.envOrBool(common.EnvSOMarkEnableInlineImage, parser.SoMarkEnableInlineImage),
		"enable_table_image":             parser.envOrBool(common.EnvSOMarkEnableTableImage, parser.SoMarkEnableTableImage),
		"enable_image_understanding":     parser.envOrBool(common.EnvSOMarkEnableImageUnderstanding, parser.SoMarkEnableImageUnderstanding),
		"keep_header_footer":             parser.envOrBool(common.EnvSOMarkKeepHeaderFooter, parser.SoMarkKeepHeaderFooter),
	})
	_ = writer.WriteField("element_formats", string(elementFormats))
	_ = writer.WriteField("feature_config", string(featureConfig))
	if apiKey != "" {
		_ = writer.WriteField("api_key", apiKey)
	}
	if err = writer.Close(); err != nil {
		return "", fmt.Errorf("parser: SoMark finalize form: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/parse/async", &body)
	if err != nil {
		return "", fmt.Errorf("parser: SoMark request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := common.GetSSRFHTTPClient().Do(req)
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
	if err = json.Unmarshal(raw, &payload); err != nil {
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

func soMarkPoll(ctx context.Context, baseURL, taskID, apiKey string) (map[string]any, error) {
	form := url.Values{"task_id": {taskID}}
	if apiKey != "" {
		form.Set("api_key", apiKey)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/parse/async_check", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("parser: SoMark poll request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := common.GetSSRFHTTPClient().Do(req)
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
	if err = json.Unmarshal(raw, &payload); err != nil {
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

func soMarkItems(ctx context.Context, result map[string]any, keepHeaderFooter bool, pageSizes [][2]float64) ([]map[string]any, int, []string) {
	outputs, _ := result["outputs"].(map[string]any)
	jsonPayload, _ := outputs["json"].(map[string]any)
	pages, _ := jsonPayload["pages"].([]any)
	items := make([]map[string]any, 0)
	state := &htmlWalkState{}
	var warnings []string

	// Positions reference real document pages: prefer the source PDF's page
	// count when it is known, falling back to the number of pages the
	// service returned.
	pageCount := len(pages)
	if len(pageSizes) > 0 {
		pageCount = len(pageSizes)
	}

	// Pixel dimensions of each page, keyed by the same resolved page number
	// positions use, so a block whose page_idx points at another page scales
	// against THAT page's pixel space instead of the enclosing page's.
	pixByPage := make(map[int][2]float64, len(pages))
	for pageIndex, pageRaw := range pages {
		page, ok := pageRaw.(map[string]any)
		if !ok {
			continue
		}
		// SoMark pages carry a 0-based page_num; positions and page_number
		// are 1-based.
		pageNumber := pageIndex + 1
		if n, ok := soMarkPageIndex(page["page_num"]); ok {
			pageNumber = n + 1
		}
		if w, h := soMarkPagePixelSize(page["page_size"]); w > 0 && h > 0 {
			pixByPage[pageNumber] = [2]float64{w, h}
		}
	}

	for pageIndex, pageRaw := range pages {
		page, ok := pageRaw.(map[string]any)
		if !ok {
			continue
		}
		// SoMark pages carry a 0-based page_num; positions and page_number
		// are 1-based.
		pageNumber := pageIndex + 1
		if n, ok := soMarkPageIndex(page["page_num"]); ok {
			pageNumber = n + 1
		}
		blocks, _ := page["blocks"].([]any)
		for _, blockRaw := range blocks {
			block, ok := blockRaw.(map[string]any)
			if !ok {
				continue
			}
			blockItems, blockWarnings := soMarkBlockToItems(ctx, block, keepHeaderFooter, state)
			warnings = append(warnings, blockWarnings...)
			for _, item := range blockItems {
				item["page_number"] = pageNumber
				// A block may carry its own 0-based page_idx; it wins over the
				// enclosing page value, which stays as the fallback. An index
				// beyond the known page range (the source PDF's pages when
				// readable, else the returned pages) falls back too: emitting
				// it would point at a page that does not exist and inflate
				// page_count downstream.
				blockPage := pageNumber
				pageBound := len(pages)
				if len(pageSizes) > 0 {
					pageBound = len(pageSizes)
				}
				if n, ok := soMarkPageIndex(block["page_idx"]); ok && n < pageBound {
					blockPage = n + 1
				}
				if len(pageSizes) > 0 && blockPage > len(pageSizes) {
					// The page is not in the source document; a position for
					// it cannot be correct, so the item goes out without one.
					items = append(items, item)
					continue
				}
				src := pixByPage[blockPage]
				if pos := soMarkBlockPosition(block, blockPage, src[0], src[1], pageSizes); pos != nil {
					item["positions"] = [][]any{pos}
					item["_pdf_positions"] = [][]any{pos}
				}
				items = append(items, item)
			}
		}
	}
	return items, pageCount, append(state.warnings(), warnings...)
}

// soMarkBlockPosition converts a SoMark block bbox ([x0, top, x1, bottom]) and
// its 1-based page number into the [page, left, right, top, bottom] position
// row used by _pdf_positions.
// soMarkPageIndex validates a SoMark page index before use: JSON numbers
// arrive as float64, so reject fractional, negative, non-finite, and
// out-of-int-range values instead of truncating them into a wrong page.
func soMarkPageIndex(raw any) (int, bool) {
	f, ok := numericAny(raw)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f != math.Trunc(f) || f > float64(math.MaxInt32) {
		return 0, false
	}
	return int(f), true
}

func soMarkBlockPosition(block map[string]any, pageNumber int, srcW, srcH float64, pageSizes [][2]float64) []any {
	bbox, _ := block["bbox"].([]any)
	if len(bbox) != 4 {
		return nil
	}
	var v [4]float64
	for i, raw := range bbox {
		n, ok := numericAny(raw)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return nil
		}
		v[i] = n
	}
	x0, top, x1, bott := v[0], v[1], v[2], v[3]
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if top > bott {
		top, bott = bott, top
	}
	// SoMark reports bboxes in its own pixel space (page["page_size"]), while
	// RAGFlow positions are PDF points. Rescale into the page's point size and
	// clamp to the page, mirroring the Python converter's pdfplumber-at-72dpi
	// mapping. When either side is unknown the raw box is kept, matching the
	// Python fallback for pages it could not render.
	idx := pageNumber - 1
	if srcW > 0 && srcH > 0 && idx >= 0 && idx < len(pageSizes) {
		dst := pageSizes[idx]
		if dst[0] > 0 && dst[1] > 0 {
			x0 = min(max(x0/srcW*dst[0], 0), dst[0])
			x1 = min(max(x1/srcW*dst[0], 0), dst[0])
			top = min(max(top/srcH*dst[1], 0), dst[1])
			bott = min(max(bott/srcH*dst[1], 0), dst[1])
		}
	}
	return []any{float64(pageNumber), x0, x1, top, bott}
}

// soMarkPagePixelSize reads a page's SoMark pixel dimensions ({w, h}); a zero
// value means unknown.
func soMarkPagePixelSize(raw any) (float64, float64) {
	m, _ := raw.(map[string]any)
	w, _ := numericAny(m["w"])
	h, _ := numericAny(m["h"])
	return w, h
}

// soMarkPagePointSizes reads each page's [width, height] in PDF points from
// the source document (CropBox, falling back to MediaBox), the space RAGFlow
// positions and crops use. A nil result or a zero entry means the page size
// is unknown and positions stay in the service's pixel space, matching the
// Python converter's fallback when a page cannot be rendered.
func soMarkPagePointSizes(data []byte) [][2]float64 {
	r, err := pdfreader.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil
	}
	numPage := r.NumPage()
	// A corrupt or malicious catalog /Count can be negative (panics make) or
	// enormous (allocates/iterates without bound). Treat it as unreadable and
	// fall back to the caller's pixel-space scaling, mirroring the
	// maxPDFPageCount guard deepdoc/pdf/plaintext_nocgo.go applies to the
	// same library.
	if numPage < 0 || numPage > maxSoMarkPageCount {
		return nil
	}
	sizes := make([][2]float64, 0, numPage)
	for p := 1; p <= numPage; p++ {
		box := soMarkInheritedPageValue(r.Page(p), "CropBox")
		if box.IsNull() || box.Len() != 4 {
			box = soMarkInheritedPageValue(r.Page(p), "MediaBox")
		}
		if box.IsNull() || box.Len() != 4 {
			sizes = append(sizes, [2]float64{})
			continue
		}
		x0, y0 := box.Index(0).Float64(), box.Index(1).Float64()
		x1, y1 := box.Index(2).Float64(), box.Index(3).Float64()
		w, h := math.Abs(x1-x0), math.Abs(y1-y0)
		// A quarter-turn /Rotate (also inheritable) swaps the dimensions of
		// the rendered page SoMark sees; scaling and clamping must use the
		// rotated size.
		if rot := soMarkInheritedPageValue(r.Page(p), "Rotate"); !rot.IsNull() {
			switch deg := ((int(rot.Float64()) % 360) + 360) % 360; deg {
			case 90, 270:
				w, h = h, w
			}
		}
		sizes = append(sizes, [2]float64{w, h})
	}
	return sizes
}

// maxSoMarkPageCount mirrors deepdoc/pdf/plaintext_nocgo.go: one million
// pages is far beyond any real document, so larger claimed counts are
// malformed.
const maxSoMarkPageCount = 1_000_000

// maxSoMarkParentDepth bounds the /Parent walk: ledongthuc/pdf detects no
// reference cycles, so a self-referencing /Parent would loop forever.
const maxSoMarkParentDepth = 32

// soMarkInheritedPageValue resolves a page dictionary key through the Parent
// chain, the inheritance the PDF spec defines for page boxes.
func soMarkInheritedPageValue(page pdfreader.Page, key string) pdfreader.Value {
	depth := 0
	for v := page.V; !v.IsNull() && depth < maxSoMarkParentDepth; v = v.Key("Parent") {
		if r := v.Key(key); !r.IsNull() {
			return r
		}
		depth++
	}
	return pdfreader.Value{}
}

func soMarkBlockToItems(ctx context.Context, block map[string]any, keepHeaderFooter bool, state *htmlWalkState) ([]map[string]any, []string) {
	blockType := strings.ToLower(strings.TrimSpace(stringValue(block["type"])))
	switch blockType {
	case "cate", "cate_item", "blank":
		return nil, nil
	case "header", "footer":
		if !keepHeaderFooter {
			return nil, nil
		}
	}
	content := strings.TrimSpace(stringValue(block["content"]))
	switch blockType {
	case "figure", "cs", "qrcode", "stamp":
		if content == "" {
			content = "[Image]"
		}
		return []map[string]any{{"text": content, "doc_type_kwd": "image", "layout": "figure"}}, nil
	case "table":
		if content == "" {
			return nil, nil
		}
		markdown := &MarkdownParser{libType: GoMarkdown}
		parsed := markdown.parseWithState(ctx, "", []byte(content), state)
		items := ItemsToMaps(parsed.JSON)
		for _, item := range items {
			if item["doc_type_kwd"] == "table" {
				item["layout"] = "table"
			}
		}
		return items, parsed.Warnings
	case "equation":
		if content == "" {
			return nil, nil
		}
		return []map[string]any{{"text": content, "doc_type_kwd": "text", "layout": "equation"}}, nil
	case "title":
		level := int(numberValue(block["title_level"]))
		if level < 1 || level > 6 {
			level = 1
		}
		if content == "" {
			return nil, nil
		}
		return []map[string]any{{"text": strings.Repeat("#", level) + " " + content, "doc_type_kwd": "text", "layout": "title"}}, nil
	default:
		if content == "" {
			return nil, nil
		}
		return []map[string]any{{"text": content, "doc_type_kwd": "text", "layout": "text"}}, nil
	}
}

// envOrDefault prefers configured values, then the environment for named
// engines only, then the built-in fallback.
func (p *PDFParser) envOrDefault(envKey, configured, fallback string) string {
	if configured != "" {
		return configured
	}
	if !p.UseTenantModelConfig {
		if raw := strings.TrimSpace(common.GetEnv(envKey)); raw != "" {
			return raw
		}
	}
	return fallback
}

// envOrBool preserves tenant instance values, including explicit false, while
// named engines retain their environment override.
func (p *PDFParser) envOrBool(envKey string, configured bool) bool {
	if p.UseTenantModelConfig {
		return configured
	}
	if raw := strings.TrimSpace(common.GetEnv(envKey)); raw != "" {
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
