//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package models

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"ragflow/internal/common"

	"go.uber.org/zap"
)

// Mistral OCR is the dedicated POST /v1/ocr document endpoint, not a vision
// chat model. Both the "Mistral" provider (mistral-ocr-* models) and the
// OCR-only "Mistral OCR" provider reach it through MistralModel.OCRFile.
const (
	// mistralOCRInlineMaxBytes is the largest document sent inline as a
	// base64 data URL; larger documents go through the Files API.
	mistralOCRInlineMaxBytes = 20 << 20
	// mistralOCRMaxPages is the API's per-document page limit.
	mistralOCRMaxPages = 1000
	// mistralOCRMaxResponseBytes bounds a full-document response. Requests
	// set include_image_base64=false, so a page carries only its Markdown,
	// tables and block layout: about 8 KB per text page on the live API,
	// which leaves 8x headroom at the 1000-page limit.
	mistralOCRMaxResponseBytes int64 = 64 << 20
	// mistralOCRSignedURLExpiryHours is how long the uploaded document's
	// signed URL stays valid.
	mistralOCRSignedURLExpiryHours = 24
	mistralOCRCleanupTimeout       = 30 * time.Second
)

// mistralOCRLinkPattern matches the Markdown links Mistral OCR leaves in a
// page's Markdown where it extracted a table ([tbl-0.html](tbl-0.html)) or
// an image (![img-0.jpeg](img-0.jpeg)).
var mistralOCRLinkPattern = regexp.MustCompile(`(!?)\[[^\]]*\]\(([^)\s]+)\)`)

var mistralOCRBlankLines = regexp.MustCompile(`\n{3,}`)

// mistralOCRSettings are the per-tenant options of an OCR call.
type mistralOCRSettings struct {
	apiKey           string
	baseURL          string
	tableFormat      string
	keepHeaderFooter bool
}

// mistralOCRResolveSettings reads the tenant credentials. The stored api_key
// is either the raw Mistral key or a JSON payload: {"api_key", "mistral_ocr_*"}
// from the provider form, or {"MISTRAL_OCR_*"} for tenants provisioned from
// the MISTRAL_OCR_* environment variables. The instance base_url wins over a
// payload base URL, as for the other Go OCR drivers (MinerU reads its payload
// server only when the instance has no base_url); with neither, the
// provider's default endpoint is used.
func mistralOCRResolveSettings(apiConfig *APIConfig) mistralOCRSettings {
	settings := mistralOCRSettings{tableFormat: "html"}
	raw := ""
	if apiConfig != nil && apiConfig.ApiKey != nil {
		raw = strings.TrimSpace(*apiConfig.ApiKey)
	}
	if apiConfig != nil && apiConfig.BaseURL != nil {
		settings.baseURL = strings.TrimSpace(*apiConfig.BaseURL)
	}
	if providerJSONConfigMap(raw) == nil {
		settings.apiKey = raw
		return settings
	}
	settings.apiKey = ProviderJSONConfigValue(raw, "api_key", "mistral_ocr_api_key", "MISTRAL_OCR_API_KEY")
	if settings.baseURL == "" {
		settings.baseURL = ProviderJSONConfigValue(raw, "mistral_ocr_base_url", "MISTRAL_OCR_BASE_URL")
	}
	if strings.EqualFold(ProviderJSONConfigValue(raw, "mistral_ocr_table_format", "MISTRAL_OCR_TABLE_FORMAT"), "markdown") {
		settings.tableFormat = "markdown"
	}
	switch strings.ToLower(ProviderJSONConfigValue(raw, "mistral_ocr_keep_header_footer", "MISTRAL_OCR_KEEP_HEADER_FOOTER")) {
	case "1", "true", "yes", "on":
		settings.keepHeaderFooter = true
	}
	return settings
}

// mistralOCREndpoint joins a base URL and a "v1/..." suffix. Base URLs that
// already end in /v1 (the documented https://api.mistral.ai/v1 form) are
// accepted without producing /v1/v1.
func mistralOCREndpoint(baseURL, suffix string) string {
	base := strings.TrimRight(baseURL, "/")
	suffix = strings.TrimLeft(suffix, "/")
	if strings.HasPrefix(suffix, "v1/") && strings.HasSuffix(base, "/v1") {
		base = strings.TrimSuffix(base, "/v1")
	}
	return base + "/" + suffix
}

// mistralOCRPageIndexes converts 1-indexed inclusive page ranges into the
// zero-based page selector /v1/ocr expects. The selector is bounded by the
// API's page limit; a selection lying entirely past it is an error rather
// than a silent whole-document request. Callers that know the document's
// page count clamp the ranges to it (and omit a whole-document selection),
// since the API silently skips indexes past the last page.
func mistralOCRPageIndexes(ocrConfig *OCRConfig) ([]int, error) {
	if ocrConfig == nil || len(ocrConfig.Pages) == 0 {
		return nil, nil
	}
	seen := make(map[int]struct{})
	indexes := make([]int, 0)
	for _, pageRange := range ocrConfig.Pages {
		if len(pageRange) != 2 {
			continue
		}
		from, to := max(pageRange[0], 1), min(pageRange[1], mistralOCRMaxPages)
		for page := from; page <= to; page++ {
			if _, ok := seen[page-1]; ok {
				continue
			}
			seen[page-1] = struct{}{}
			indexes = append(indexes, page-1)
		}
	}
	if len(indexes) == 0 {
		return nil, fmt.Errorf("Mistral OCR: page selection %v is outside the first %d pages", ocrConfig.Pages, mistralOCRMaxPages)
	}
	sort.Ints(indexes)
	return indexes, nil
}

// OCRFile runs Mistral OCR on content (a PDF or image) or, when content is
// empty, on the document at the http(s) URL. It returns the document as
// Markdown: header/footer blocks are dropped unless the tenant opted to keep
// them, extracted tables are inlined in the configured table format, and
// image placeholders are removed.
func (m *MistralModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	settings := mistralOCRResolveSettings(apiConfig)
	if settings.apiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	remoteURL := ""
	if url != nil {
		remoteURL = strings.TrimSpace(*url)
	}
	if len(content) == 0 && !strings.HasPrefix(remoteURL, "http://") && !strings.HasPrefix(remoteURL, "https://") {
		return nil, fmt.Errorf("file url or content is required")
	}
	pages, err := mistralOCRPageIndexes(ocrConfig)
	if err != nil {
		return nil, err
	}

	urlConfig := &APIConfig{BaseURL: &settings.baseURL}
	if apiConfig != nil {
		urlConfig.Region = apiConfig.Region
	}
	baseURL, err := m.baseModel.GetBaseURL(urlConfig)
	if err != nil {
		return nil, err
	}
	auth := &APIConfig{ApiKey: &settings.apiKey}

	ctx, cancel := context.WithTimeout(ctx, longOpCallTimeout)
	defer cancel()

	var document map[string]any
	switch {
	case len(content) == 0:
		document = map[string]any{"type": "document_url", "document_url": remoteURL}
	case len(content) > mistralOCRInlineMaxBytes:
		signedURL, cleanup, err := m.mistralOCRUpload(ctx, baseURL, auth, content)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		document = map[string]any{"type": "document_url", "document_url": signedURL}
	default:
		mimeType := mistralOCRMimeType(content)
		dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(content))
		if strings.HasPrefix(mimeType, "image/") {
			document = map[string]any{"type": "image_url", "image_url": dataURL}
		} else {
			document = map[string]any{"type": "document_url", "document_url": dataURL}
		}
	}

	payload := map[string]any{
		"model":                strings.TrimSpace(*modelName),
		"document":             document,
		"include_blocks":       true,
		"include_image_base64": false,
		"table_format":         settings.tableFormat,
	}
	if len(pages) > 0 {
		payload["pages"] = pages
	}

	req, err := m.baseModel.newJSONPostRequest(ctx, mistralOCREndpoint(baseURL, m.baseModel.URLSuffix.OCR), auth, payload)
	if err != nil {
		return nil, err
	}
	body, err := m.mistralOCRSend(req, mistralOCRMaxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("Mistral OCR: %w", err)
	}

	var result mistralOCRResponse
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("Mistral OCR: failed to parse response: %w", err)
	}
	text := result.markdown(settings.keepHeaderFooter)
	return &OCRFileResponse{Text: &text}, nil
}

// mistralOCRPDFHeaderWindow is how far into the file a PDF header may start;
// PDF readers accept leading bytes before %PDF- within the first kilobyte.
const mistralOCRPDFHeaderWindow = 1024

// mistralOCRMimeType returns the bare media type of content for the OCR data
// URL. Image signatures are trusted first; otherwise PDFs are recognised even
// when bytes precede the header, which content sniffing alone reports as
// text/plain.
func mistralOCRMimeType(content []byte) string {
	mediaType, _, err := mime.ParseMediaType(http.DetectContentType(content))
	if err == nil && strings.HasPrefix(mediaType, "image/") {
		return mediaType
	}
	if bytes.Contains(content[:min(len(content), mistralOCRPDFHeaderWindow)], []byte("%PDF-")) {
		return "application/pdf"
	}
	if err != nil {
		return "application/octet-stream"
	}
	return mediaType
}

// mistralOCRSend performs req and returns the response body of a 2xx reply.
func (m *MistralModel) mistralOCRSend(req *http.Request, maxBytes int64) ([]byte, error) {
	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, err := readModelErrorBody(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("API request failed with status %d; failed to read error response: %w", resp.StatusCode, err)
		}
		return nil, &APIStatusError{Status: resp.StatusCode, Body: string(body)}
	}
	body, err := readModelResponseBodyLimited(resp.Body, maxBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	return body, nil
}

// mistralOCRUpload stores content through the Files API (purpose "ocr") and
// returns a signed URL for it plus a cleanup that deletes the upload. The
// cleanup runs even after ctx is done so no document is left on Mistral.
func (m *MistralModel) mistralOCRUpload(ctx context.Context, baseURL string, auth *APIConfig, content []byte) (string, func(), error) {
	if m.baseModel.URLSuffix.Files == "" {
		return "", nil, fmt.Errorf("Mistral OCR: document exceeds %d bytes and the provider has no files endpoint", mistralOCRInlineMaxBytes)
	}
	filesURL := mistralOCREndpoint(baseURL, m.baseModel.URLSuffix.Files)

	filename := "document"
	if extensions, _ := mime.ExtensionsByType(mistralOCRMimeType(content)); len(extensions) > 0 {
		filename += extensions[0]
	}
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	_ = writer.WriteField("purpose", "ocr")
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", nil, fmt.Errorf("Mistral OCR upload: %w", err)
	}
	if _, err = part.Write(content); err != nil {
		return "", nil, fmt.Errorf("Mistral OCR upload: %w", err)
	}
	if err = writer.Close(); err != nil {
		return "", nil, fmt.Errorf("Mistral OCR upload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, filesURL, &form)
	if err != nil {
		return "", nil, fmt.Errorf("Mistral OCR upload: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", BearerAuth(auth))
	body, err := m.mistralOCRSend(req, maxModelResponseBodyBytes)
	if err != nil {
		return "", nil, fmt.Errorf("Mistral OCR upload: %w", err)
	}
	var uploaded struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(body, &uploaded); err != nil || uploaded.ID == "" {
		return "", nil, fmt.Errorf("Mistral OCR upload: response carries no file id: %s", string(body))
	}

	fileURL := filesURL + "/" + uploaded.ID
	cleanup := func() {
		deleteCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mistralOCRCleanupTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(deleteCtx, http.MethodDelete, fileURL, nil)
		if err == nil {
			req.Header.Set("Authorization", BearerAuth(auth))
			_, err = m.mistralOCRSend(req, maxModelResponseBodyBytes)
		}
		if err != nil {
			common.Warn("Mistral OCR: failed to delete uploaded document", zap.String("file_id", uploaded.ID), zap.Error(err))
		}
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/url?expiry=%d", fileURL, mistralOCRSignedURLExpiryHours), nil)
	if err == nil {
		req.Header.Set("Authorization", BearerAuth(auth))
		body, err = m.mistralOCRSend(req, maxModelResponseBodyBytes)
	}
	var signed struct {
		URL string `json:"url"`
	}
	if err == nil {
		if err = json.Unmarshal(body, &signed); err == nil && signed.URL == "" {
			err = fmt.Errorf("response carries no url: %s", string(body))
		}
	}
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("Mistral OCR signed url: %w", err)
	}
	return signed.URL, cleanup, nil
}

type mistralOCRResponse struct {
	Pages []mistralOCRPage `json:"pages"`
}

type mistralOCRPage struct {
	Index    int    `json:"index"`
	Markdown string `json:"markdown"`
	Images   []struct {
		ID string `json:"id"`
	} `json:"images"`
	Tables []struct {
		ID      string `json:"id"`
		Content string `json:"content"`
	} `json:"tables"`
	Blocks []struct {
		Type    string `json:"type"`
		Content string `json:"content"`
		TableID string `json:"table_id"`
	} `json:"blocks"`
}

// markdown joins the pages in order. A page with layout blocks is rebuilt
// from them, which lets header/footer blocks be dropped; a page without
// blocks falls back to its Markdown with table and image links resolved.
// Title blocks stay Markdown headings either way: the API sends their content
// with its heading marker ("## Section"), and a bare title becomes "# ".
func (r mistralOCRResponse) markdown(keepHeaderFooter bool) string {
	parts := make([]string, 0, len(r.Pages))
	for _, page := range r.Pages {
		tables := make(map[string]string, len(page.Tables))
		for _, table := range page.Tables {
			tables[table.ID] = strings.TrimSpace(table.Content)
		}
		if len(page.Blocks) == 0 {
			if md := page.resolvedMarkdown(tables); md != "" {
				parts = append(parts, md)
			}
			continue
		}
		for _, block := range page.Blocks {
			text := strings.TrimSpace(block.Content)
			switch block.Type {
			case "header", "footer":
				if !keepHeaderFooter {
					continue
				}
			case "image":
				continue
			case "title":
				if text != "" && !strings.HasPrefix(text, "#") {
					text = "# " + text
				}
			case "table":
				if text == "" {
					text = tables[block.TableID]
				}
			}
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func (p mistralOCRPage) resolvedMarkdown(tables map[string]string) string {
	images := make(map[string]struct{}, len(p.Images))
	for _, image := range p.Images {
		images[image.ID] = struct{}{}
	}
	md := mistralOCRLinkPattern.ReplaceAllStringFunc(p.Markdown, func(link string) string {
		match := mistralOCRLinkPattern.FindStringSubmatch(link)
		target := match[2]
		if table, ok := tables[target]; ok && match[1] == "" {
			return table
		}
		if _, ok := images[target]; ok && match[1] == "!" {
			return ""
		}
		return link
	})
	return strings.TrimSpace(mistralOCRBlankLines.ReplaceAllString(md, "\n\n"))
}
