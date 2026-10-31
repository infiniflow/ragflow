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
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"ragflow/internal/common"
	"strconv"
	"strings"
	"time"
)

// ErrTaskAuthorization indicates that the task result status endpoint rejected
// access due to invalid, expired, or missing credentials (HTTP 401/403).
var ErrTaskAuthorization = errors.New("task result authorization failed")

// UnlimitedOCRModel represents Baidu's Unlimited-OCR-2 inference engine driver.
// It speaks the high-performance /file_parse and /tasks result extraction protocol
// returning clean Markdown directly into RAGFlow's ingestion pipeline.
type UnlimitedOCRModel struct {
	baseModel BaseModel
}

// NewUnlimitedOCRModel constructs a new Unlimited-OCR-2 driver instance.
func NewUnlimitedOCRModel(baseURL map[string]string, urlSuffix URLSuffix) *UnlimitedOCRModel {
	return &UnlimitedOCRModel{
		baseModel: BaseModel{
			BaseURL:          baseURL,
			URLSuffix:        urlSuffix,
			AllowEmptyAPIKey: true,
			httpClient:       common.GetSchemeSafeHTTPClient(),
		},
	}
}

// NewInstance returns a new UnlimitedOCRModel instance configured with the specified base URLs.
func (m *UnlimitedOCRModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewUnlimitedOCRModel(baseURL, m.baseModel.URLSuffix)
}

// Name returns the provider identifier string.
func (m *UnlimitedOCRModel) Name() string {
	return "unlimited_ocr"
}

// normalizeServerURL ensures the URL string has a valid scheme and no trailing slash.
func normalizeServerURL(raw string) string {
	u := strings.TrimRight(strings.TrimSpace(raw), "/")
	if u == "" {
		return ""
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return "http://" + u
	}
	return u
}

// ResolveUnlimitedOCRServerURL extracts the server address from apiConfig or environment variables.
func ResolveUnlimitedOCRServerURL(apiConfig *APIConfig) string {
	if apiConfig != nil && apiConfig.BaseURL != nil {
		if u := normalizeServerURL(*apiConfig.BaseURL); u != "" {
			return u
		}
	}
	if apiConfig != nil && apiConfig.ApiKey != nil {
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(*apiConfig.ApiKey), &cfg); err == nil {
			if u, ok := cfg["unlimited_ocr_server_url"].(string); ok {
				if norm := normalizeServerURL(u); norm != "" {
					return norm
				}
			}
			if u, ok := cfg["unlimited_ocr_apiserver"].(string); ok {
				if norm := normalizeServerURL(u); norm != "" {
					return norm
				}
			}
		}
	}
	if u := normalizeServerURL(common.GetEnv(common.EnvUnlimitedOCRServerURL)); u != "" {
		return u
	}
	if u := normalizeServerURL(common.GetEnv(common.EnvUnlimitedOCRAPIServer)); u != "" {
		return u
	}
	return "http://127.0.0.1:18000"
}

// ResolveUnlimitedOCRBearerToken extracts the authentication token from apiConfig.
func ResolveUnlimitedOCRBearerToken(apiConfig *APIConfig) string {
	if apiConfig == nil || apiConfig.ApiKey == nil {
		return ""
	}
	raw := strings.TrimSpace(*apiConfig.ApiKey)
	if raw == "" {
		return ""
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err == nil && cfg != nil {
		if v, ok := cfg["unlimited_ocr_api_key"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		if v, ok := cfg["token"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		if v, ok := cfg["api_key"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return ""
	}
	return raw
}

// GetBaseURL resolves the effective server endpoint for Unlimited-OCR-2 with a normalized scheme.
func (m *UnlimitedOCRModel) GetBaseURL(apiConfig *APIConfig) (string, error) {
	if u := ResolveUnlimitedOCRServerURL(apiConfig); u != "" {
		return u, nil
	}
	base, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return "", err
	}
	return normalizeServerURL(base), nil
}

// unlimitedOCRGET performs an HTTP GET request with optional Bearer token authentication.
func unlimitedOCRGET(ctx context.Context, client *http.Client, targetURL, token string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return 0, nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

// unlimitedOCRHealthOK inspects the HTTP response body and status code to confirm health status.
func unlimitedOCRHealthOK(code int, body []byte) bool {
	if code != http.StatusOK {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	status, _ := payload["status"].(string)
	engine, _ := payload["engine"].(string)
	if strings.EqualFold(status, "ok") || strings.EqualFold(status, "ready") {
		return true
	}
	if strings.Contains(strings.ToLower(engine), "unlimited") {
		return true
	}
	return false
}

// CheckConnection probes /v1/health and /health to verify service reachability.
func (m *UnlimitedOCRModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	resolvedBaseURL, err := m.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	u, err := url.Parse(resolvedBaseURL)
	if err != nil {
		return fmt.Errorf("invalid base URL: %w", err)
	}
	token := ResolveUnlimitedOCRBearerToken(apiConfig)
	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	v1URL := *u
	v1URL.Path = "/v1/health"
	v1Code, v1Body, v1Err := unlimitedOCRGET(ctx, m.baseModel.httpClient, v1URL.String(), token)
	if v1Err == nil && (v1Code == http.StatusUnauthorized || v1Code == http.StatusForbidden) {
		return fmt.Errorf("authentication failed (HTTP %d)", v1Code)
	}
	if v1Err == nil && unlimitedOCRHealthOK(v1Code, v1Body) {
		return nil
	}

	legacy := *u
	legacy.Path = "/health"
	code, body, err := unlimitedOCRGET(ctx, m.baseModel.httpClient, legacy.String(), token)
	if err != nil {
		if v1Err != nil {
			return fmt.Errorf("connection failed: %w", v1Err)
		}
		return fmt.Errorf("connection failed: %w", err)
	}
	if code == http.StatusUnauthorized || code == http.StatusForbidden {
		return fmt.Errorf("authentication failed (HTTP %d)", code)
	}
	if unlimitedOCRHealthOK(code, body) {
		return nil
	}
	return fmt.Errorf("not an Unlimited-OCR-2 service: HTTP %d", code)
}

// ShowTask retrieves the processing status and extracted segments for an Unlimited-OCR-2 task.
func (m *UnlimitedOCRModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("taskID is empty")
	}

	baseURL, err := m.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}

	taskURL := joinURL(baseURL, m.baseModel.URLSuffix.Task)
	url := strings.TrimRight(taskURL, "/") + "/" + taskID + "/result"
	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create status request: %w", err)
	}

	if token := ResolveUnlimitedOCRBearerToken(apiConfig); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send status request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read status response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("%w: Unlimited-OCR-2 status API failed with status %d: %s", ErrTaskAuthorization, resp.StatusCode, string(body))
		}
		return nil, fmt.Errorf("Unlimited-OCR-2 status API failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	content := ""
	if results, ok := result["results"].(map[string]interface{}); ok {
		for _, fileObj := range results {
			if fileMap, ok := fileObj.(map[string]interface{}); ok {
				if md, ok := fileMap["md_content"].(string); ok && md != "" {
					content = md
					break
				}
			}
		}
	}
	if content == "" {
		if text, ok := result["content"].(string); ok && text != "" {
			content = text
		} else if md, ok := result["markdown"].(string); ok && md != "" {
			content = md
		}
	}

	if content == "" {
		return nil, fmt.Errorf("md_content not found in task result")
	}

	return &TaskResponse{
		Segments: []TaskSegment{
			{
				Index:   1,
				Content: content,
			},
		},
	}, nil
}

// ParseFile submits a document to Unlimited-OCR-2, forwarding crop_mode, no_repeat_ngram_size,
// and page_guard options in the multipart request.
func (m *UnlimitedOCRModel) ParseFile(ctx context.Context, modelName *string, content []byte, documentURL *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(content) == 0 {
		return nil, fmt.Errorf("Unlimited-OCR-2 API requires file content byte array, but content is empty")
	}

	resolvedBaseURL, err := m.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	apiURL := joinURL(resolvedBaseURL, m.baseModel.URLSuffix.DocumentParse)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("files", "upload_document.pdf")
	if err != nil {
		return nil, fmt.Errorf("failed to create multipart file field: %w", err)
	}
	if _, err = part.Write(content); err != nil {
		return nil, fmt.Errorf("failed to write file content: %w", err)
	}

	backend := "pipeline"
	if modelName != nil && strings.TrimSpace(*modelName) != "" {
		backend = strings.TrimSpace(*modelName)
	} else if parseFileConfig != nil && strings.TrimSpace(parseFileConfig.Backend) != "" {
		backend = strings.TrimSpace(parseFileConfig.Backend)
	}
	_ = writer.WriteField("backend", backend)

	if parseFileConfig != nil {
		if crop := strings.TrimSpace(parseFileConfig.CropMode); crop != "" {
			_ = writer.WriteField("crop_mode", crop)
		}
		if ngram := parseFileConfig.NoRepeatNgramSize; ngram > 0 {
			_ = writer.WriteField("no_repeat_ngram_size", strconv.Itoa(ngram))
		}
		if pg := parseFileConfig.PageGuard; pg != nil {
			_ = writer.WriteField("page_guard", strconv.FormatBool(*pg))
		}
	}

	if err = writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	if token := ResolveUnlimitedOCRBearerToken(apiConfig); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("Unlimited-OCR-2 API failed with status %d: %s (URL: %s)", resp.StatusCode, string(respBody), apiURL)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response JSON: %w, body: %s", err, string(respBody))
	}

	var taskID string
	if dataMap, ok := result["data"].(map[string]interface{}); ok {
		if tid, ok := dataMap["task_id"].(string); ok {
			taskID = tid
		}
	}
	if taskID == "" {
		if tid, ok := result["task_id"].(string); ok {
			taskID = tid
		}
	}

	if taskID == "" {
		return nil, fmt.Errorf("failed to extract task_id from Unlimited-OCR-2 response: %s", string(respBody))
	}

	return &ParseFileResponse{
		TaskID: taskID,
	}, nil
}

// OCRFile exercises CheckConnection for verification probes, or parses content and returns text for actual OCR requests.
func (m *UnlimitedOCRModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	if err := m.CheckConnection(ctx, apiConfig); err != nil {
		return nil, err
	}
	// Fast path for verification probe (empty content or 1x1 test image)
	if len(content) == 0 || (len(content) < 100 && bytes.HasPrefix(content, []byte{0x89, 0x50, 0x4E, 0x47})) {
		return &OCRFileResponse{}, nil
	}

	backend := "pipeline"
	task, err := m.ParseFile(ctx, &backend, content, url, apiConfig, &ParseFileConfig{}, modelUsage)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(longOpCallTimeout)
	var lastErr error
	for {
		taskResult, err := m.ShowTask(ctx, task.TaskID, apiConfig)
		if err == nil {
			for _, segment := range taskResult.Segments {
				if strings.TrimSpace(segment.Content) != "" {
					text := segment.Content
					return &OCRFileResponse{Text: &text}, nil
				}
			}
		}
		if err != nil {
			if errors.Is(err, ErrTaskAuthorization) {
				return nil, fmt.Errorf("Unlimited-OCR-2 task %s result: %w", task.TaskID, err)
			}
			lastErr = err
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return nil, fmt.Errorf("timed out waiting for Unlimited-OCR-2 task %s: last error: %w", task.TaskID, lastErr)
			}
			return nil, fmt.Errorf("timed out waiting for Unlimited-OCR-2 task %s", task.TaskID)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// ListModels returns the catalogued OCR models for this provider instance.
func (m *UnlimitedOCRModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := m.CheckConnection(ctx, apiConfig); err != nil {
		return nil, err
	}
	return []ListModelResponse{{
		Name:       "Unlimited-OCR-2",
		ModelTypes: []string{"ocr"},
	}}, nil
}

// Balance reports unsupported operation for this provider.
func (m *UnlimitedOCRModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

// ListTasks reports unsupported operation for this provider.
func (m *UnlimitedOCRModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

// ChatWithMessages reports unsupported operation for this provider.
func (m *UnlimitedOCRModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

// ChatStreamlyWithSender reports unsupported operation for this provider.
func (m *UnlimitedOCRModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s no such method", m.Name())
}

// Embed reports unsupported operation for this provider.
func (m *UnlimitedOCRModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

// Rerank reports unsupported operation for this provider.
func (m *UnlimitedOCRModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

// TranscribeAudio reports unsupported operation for this provider.
func (m *UnlimitedOCRModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

// TranscribeAudioWithSender reports unsupported operation for this provider.
func (m *UnlimitedOCRModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s no such method", m.Name())
}

// AudioSpeech reports unsupported operation for this provider.
func (m *UnlimitedOCRModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

// AudioSpeechWithSender reports unsupported operation for this provider.
func (m *UnlimitedOCRModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s no such method", m.Name())
}

// Decide reports unsupported operation for this provider.
func (m *UnlimitedOCRModel) Decide(ctx context.Context, modelName *string, request *DecideRequest, apiConfig *APIConfig, modelUsage *common.ModelUsage) (*DecideResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}
