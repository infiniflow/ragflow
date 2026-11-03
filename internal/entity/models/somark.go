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
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ragflow/internal/common"

	"go.uber.org/zap"
)

// SoMarkModel drives the SoMark document-understanding service
// (https://somark.cn / https://somark.ai). The provider exposes an
// asynchronous parse protocol: POST {base}/parse/async uploads the document
// and returns a task_id, POST {base}/parse/async_check polls the task until
// it reaches SUCCESS and carries the parsed block structure in
// data.result.outputs.json. Authentication is a form field (api_key), not an
// Authorization header, and self-hosted deployments may run without a key -
// hence AllowEmptyAPIKey. The same protocol is implemented by the DeepDoc
// parser path (internal/parser/parser/pdf_parser_somark.go); this driver
// serves the model-provider surface (connection verification and the
// /v1/file/ocr route).
type SoMarkModel struct {
	baseModel BaseModel
}

func NewSoMarkModel(baseURL map[string]string, urlSuffix URLSuffix) *SoMarkModel {
	return &SoMarkModel{
		baseModel: BaseModel{
			BaseURL:          baseURL,
			URLSuffix:        urlSuffix,
			AllowEmptyAPIKey: true,
			httpClient:       common.GetSSRFHTTPClient(),
		},
	}
}

func (s *SoMarkModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewSoMarkModel(baseURL, s.baseModel.URLSuffix)
}

func (s *SoMarkModel) Name() string {
	return "somark"
}

// GetBaseURL resolves the configured region when an instance has no custom URL.
func (s *SoMarkModel) GetBaseURL(apiConfig *APIConfig) (string, error) {
	return s.baseModel.GetBaseURL(apiConfig)
}

func (s *SoMarkModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}

func (s *SoMarkModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", s.Name())
}

func (s *SoMarkModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}

func (s *SoMarkModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}

func (s *SoMarkModel) Decide(ctx context.Context, modelName *string, request *DecideRequest, apiConfig *APIConfig, modelUsage *common.ModelUsage) (*DecideResponse, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}

func (s *SoMarkModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}

func (s *SoMarkModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", s.Name())
}

func (s *SoMarkModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}

func (s *SoMarkModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", s.Name())
}

type soMarkSubmitResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		TaskID string `json:"task_id"`
	} `json:"data"`
}

type soMarkPollResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Status string         `json:"status"`
		Result map[string]any `json:"result"`
	} `json:"data"`
}

// soMarkFormFileName picks the multipart filename the SoMark API uses for
// content-type detection. OCRFile has no filename parameter, so the driver
// sniffs the well-known magic prefixes; anything else defaults to PDF.
func soMarkFormFileName(content []byte) string {
	switch {
	case len(content) >= 4 && string(content[:4]) == "%PDF":
		return "document.pdf"
	case len(content) >= 8 && content[0] == 0x89 && content[1] == 'P' && content[2] == 'N' && content[3] == 'G':
		return "image.png"
	case len(content) >= 2 && content[0] == 0xFF && content[1] == 0xD8:
		return "image.jpg"
	default:
		return "document.pdf"
	}
}

// soMarkSubmit uploads the document and returns the async task id. The form
// mirrors parser.SoMarkSubmit's wire format with the parser defaults for the
// element/feature switches; the configurable variants stay on the parser
// path, which reads the knowledge-base setup.
func (s *SoMarkModel) soMarkSubmit(ctx context.Context, submitURL string, content []byte, apiKey string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", soMarkFormFileName(content))
	if err != nil {
		return "", fmt.Errorf("SoMark create file: %w", err)
	}
	if _, err := part.Write(content); err != nil {
		return "", fmt.Errorf("SoMark write file: %w", err)
	}
	_ = writer.WriteField("output_formats", "json")
	elementFormats, _ := json.Marshal(map[string]any{
		"image":   "url",
		"formula": "latex",
		"table":   "html",
		"cs":      "image",
	})
	featureConfig, _ := json.Marshal(map[string]any{
		"enable_text_cross_page":         false,
		"enable_table_cross_page":        false,
		"enable_title_level_recognition": true,
		"enable_inline_image":            true,
		"enable_table_image":             true,
		"enable_image_understanding":     true,
		"keep_header_footer":             false,
	})
	_ = writer.WriteField("element_formats", string(elementFormats))
	_ = writer.WriteField("feature_config", string(featureConfig))
	if strings.TrimSpace(apiKey) != "" {
		_ = writer.WriteField("api_key", apiKey)
	}
	if err = writer.Close(); err != nil {
		return "", fmt.Errorf("SoMark finalize form: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, submitURL, &body)
	if err != nil {
		return "", fmt.Errorf("SoMark request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := s.baseModel.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("SoMark submit: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("SoMark read submit: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("SoMark submit failed with status %d: %s", resp.StatusCode, soMarkErrorBody(raw))
	}
	var payload soMarkSubmitResponse
	if err = json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("SoMark decode submit: %w", err)
	}
	if payload.Code != 0 {
		return "", fmt.Errorf("SoMark submit business error code=%d message=%s", payload.Code, payload.Message)
	}
	if payload.Data.TaskID == "" {
		return "", fmt.Errorf("SoMark submit returned no task_id")
	}
	return payload.Data.TaskID, nil
}

// soMarkPoll polls the async task until it reaches SUCCESS and returns the
// result payload. Non-SUCCESS terminal statuses fail the task; anything else
// keeps polling until the caller's deadline.
func (s *SoMarkModel) soMarkPoll(ctx context.Context, pollURL, taskID, apiKey string) (map[string]any, error) {
	pollInterval := 2 * time.Second
	const pollMultiplier = 1.5
	maxPollInterval := 15 * time.Second

	for attempt := 1; ; attempt++ {
		form := url.Values{"task_id": {taskID}}
		if strings.TrimSpace(apiKey) != "" {
			form.Set("api_key", apiKey)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, pollURL, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, fmt.Errorf("SoMark poll request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := s.baseModel.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("SoMark poll: %w", err)
		}
		raw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("SoMark read poll: %w", readErr)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("SoMark poll failed with status %d: %s", resp.StatusCode, soMarkErrorBody(raw))
		}
		var payload soMarkPollResponse
		if err = json.Unmarshal(raw, &payload); err != nil {
			return nil, fmt.Errorf("SoMark decode poll: %w", err)
		}
		if payload.Code != 0 {
			return nil, fmt.Errorf("SoMark poll business error code=%d message=%s", payload.Code, payload.Message)
		}
		switch strings.ToUpper(strings.TrimSpace(payload.Data.Status)) {
		case "SUCCESS":
			if payload.Data.Result == nil {
				return nil, fmt.Errorf("SoMark task %s SUCCESS without result", taskID)
			}
			return payload.Data.Result, nil
		case "FAILED", "ERROR":
			return nil, fmt.Errorf("SoMark task %s failed with status=%s message=%s", taskID, payload.Data.Status, payload.Message)
		}

		common.Debug("somark poll: in progress",
			zap.String("task_id", taskID),
			zap.Int("attempt", attempt),
			zap.String("status", payload.Data.Status),
			zap.Duration("next_poll_in", pollInterval))

		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			common.Error("somark poll: context done while waiting",
				ctx.Err(),
				zap.String("task_id", taskID),
				zap.Int("attempt", attempt),
				zap.String("last_status", payload.Data.Status))
			return nil, ctx.Err()
		}
		pollInterval = min(time.Duration(float64(pollInterval)*pollMultiplier), maxPollInterval)
	}
}

func soMarkErrorBody(raw []byte) string {
	const maxErrorBodyBytes = 1024
	if len(raw) > maxErrorBodyBytes {
		return string(raw[:maxErrorBodyBytes]) + "..."
	}
	return string(raw)
}

// soMarkMarkdown flattens the parsed block structure into markdown: titles
// become ATX headings at their recognized level, tables keep their HTML,
// figures fall back to a placeholder caption, and everything else passes its
// content through. Mirrors parser.soMarkBlockToItem's text mapping.
func soMarkMarkdown(result map[string]any) string {
	outputs, _ := result["outputs"].(map[string]any)
	jsonPayload, _ := outputs["json"].(map[string]any)
	pages, _ := jsonPayload["pages"].([]any)
	var markdown strings.Builder
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
			blockType := strings.ToLower(strings.TrimSpace(soMarkStringValue(block["type"])))
			switch blockType {
			case "cate", "cate_item", "blank":
				continue
			}
			content := strings.TrimSpace(soMarkStringValue(block["content"]))
			switch blockType {
			case "title":
				if content == "" {
					continue
				}
				level := int(soMarkNumberValue(block["title_level"]))
				if level < 1 || level > 6 {
					level = 1
				}
				markdown.WriteString(strings.Repeat("#", level) + " " + content)
			case "figure", "cs", "qrcode", "stamp":
				if content == "" {
					content = "[Image]"
				}
				markdown.WriteString(content)
			default:
				if content == "" {
					continue
				}
				markdown.WriteString(content)
			}
			markdown.WriteString("\n\n")
		}
	}
	return strings.TrimSpace(markdown.String())
}

func soMarkStringValue(value any) string {
	text, _ := value.(string)
	return text
}

func soMarkNumberValue(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	default:
		return 0
	}
}

func (s *SoMarkModel) OCRFile(ctx context.Context, modelName *string, content []byte, fileURL *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	if err := s.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if len(content) == 0 {
		// The SoMark API has no fetch-by-URL mode; the parser path uploads
		// the file bytes directly.
		return nil, fmt.Errorf("SoMark requires file content")
	}

	apiKey := ""
	if apiConfig != nil && apiConfig.ApiKey != nil {
		apiKey = *apiConfig.ApiKey
	}

	// One generous deadline bounds the whole operation (submit + poll), so
	// the poll loop below cannot spin forever.
	ctx, cancel := context.WithTimeout(ctx, longOpCallTimeout)
	defer cancel()

	submitURL, err := s.baseModel.EndpointURL(apiConfig, s.baseModel.URLSuffix.OCR)
	if err != nil {
		return nil, err
	}
	pollURL, err := s.baseModel.EndpointURL(apiConfig, s.baseModel.URLSuffix.Tasks)
	if err != nil {
		return nil, err
	}

	modelNameValue := ""
	if modelName != nil {
		modelNameValue = *modelName
	}
	common.Info("somark submit: sending task",
		zap.String("driver", s.Name()),
		zap.String("model", modelNameValue),
		zap.Int("content_bytes", len(content)))

	taskID, err := s.soMarkSubmit(ctx, submitURL, content, apiKey)
	if err != nil {
		common.Error("somark submit: request failed", err, zap.String("driver", s.Name()))
		return nil, err
	}
	common.Info("somark submit: ok",
		zap.String("driver", s.Name()),
		zap.String("task_id", taskID))

	result, err := s.soMarkPoll(ctx, pollURL, taskID, apiKey)
	if err != nil {
		common.Error("somark poll: failed", err, zap.String("task_id", taskID))
		return nil, err
	}

	// A SUCCESS task with no usable blocks still proves connectivity, which
	// is what the provider connection check keys on (verifyOCRModel accepts
	// any non-error response), so an empty extraction is returned as-is
	// rather than as an error.
	text := soMarkMarkdown(result)
	common.Info("somark result: parsed",
		zap.String("task_id", taskID),
		zap.Int("text_len", len(text)))
	return &OCRFileResponse{Text: &text}, nil
}

func (s *SoMarkModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}

func (s *SoMarkModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}

func (s *SoMarkModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}

func (s *SoMarkModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	return fmt.Errorf("%s, no such method", s.Name())
}

func (s *SoMarkModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}

func (s *SoMarkModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}
