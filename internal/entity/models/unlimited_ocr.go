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
	"ragflow/internal/common"
	"strconv"
	"strings"
	"time"
)

// UnlimitedOCRModel wraps MinerULocalModel for Baidu's Unlimited-OCR-2 engine.
// It speaks the high-performance /file_parse and /tasks result extraction protocol
// returning clean Markdown directly into RAGFlow's ingestion pipeline.
type UnlimitedOCRModel struct {
	*MinerULocalModel
}

func NewUnlimitedOCRModel(baseURL map[string]string, urlSuffix URLSuffix) *UnlimitedOCRModel {
	return &UnlimitedOCRModel{MinerULocalModel: NewMinerLocalUModel(baseURL, urlSuffix)}
}

func (m *UnlimitedOCRModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewUnlimitedOCRModel(baseURL, m.baseModel.URLSuffix)
}

func (m *UnlimitedOCRModel) Name() string {
	return "unlimited_ocr"
}

// ResolveUnlimitedOCRServerURL extracts the server address from apiConfig or environment variables.
func ResolveUnlimitedOCRServerURL(apiConfig *APIConfig) string {
	if apiConfig != nil && apiConfig.BaseURL != nil {
		if u := strings.TrimRight(strings.TrimSpace(*apiConfig.BaseURL), "/"); u != "" {
			return u
		}
	}
	if apiConfig != nil && apiConfig.ApiKey != nil {
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(*apiConfig.ApiKey), &cfg); err == nil {
			if u, ok := cfg["unlimited_ocr_server_url"].(string); ok && strings.TrimSpace(u) != "" {
				return strings.TrimRight(strings.TrimSpace(u), "/")
			}
			if u, ok := cfg["unlimited_ocr_apiserver"].(string); ok && strings.TrimSpace(u) != "" {
				return strings.TrimRight(strings.TrimSpace(u), "/")
			}
		}
	}
	if u := strings.TrimSpace(common.GetEnv(common.EnvUnlimitedOCRServerURL)); u != "" {
		return strings.TrimRight(u, "/")
	}
	if u := strings.TrimSpace(common.GetEnv(common.EnvUnlimitedOCRAPIServer)); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://127.0.0.1:18000"
}

// GetBaseURL resolves the effective server endpoint for Unlimited-OCR-2.
func (m *UnlimitedOCRModel) GetBaseURL(apiConfig *APIConfig) (string, error) {
	if u := ResolveUnlimitedOCRServerURL(apiConfig); u != "" {
		return u, nil
	}
	return m.baseModel.GetBaseURL(apiConfig)
}

// CheckConnection probes /v1/health and /health to verify service reachability.
func (m *UnlimitedOCRModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	resolvedBaseURL, err := m.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resolvedBaseURL, "http://") && !strings.HasPrefix(resolvedBaseURL, "https://") {
		resolvedBaseURL = "http://" + resolvedBaseURL
	}
	u, err := url.Parse(resolvedBaseURL)
	if err != nil {
		return fmt.Errorf("invalid base URL: %w", err)
	}
	token := ""
	if apiConfig != nil && apiConfig.ApiKey != nil {
		token = MinerUBearerTokenFromAPIKey(*apiConfig.ApiKey)
	}
	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	v1URL := *u
	v1URL.Path = "/v1/health"
	v1Code, v1Body, v1Err := minerUGET(ctx, m.baseModel.httpClient, v1URL.String(), token)
	if v1Err == nil && (v1Code == http.StatusUnauthorized || v1Code == http.StatusForbidden) {
		return fmt.Errorf("authentication failed (HTTP %d)", v1Code)
	}
	if v1Err == nil && minerUHealthOK(v1Code, v1Body) {
		return nil
	}

	legacy := *u
	legacy.Path = "/health"
	code, body, err := minerUGET(ctx, m.baseModel.httpClient, legacy.String(), token)
	if err != nil {
		if v1Err != nil {
			return fmt.Errorf("connection failed: %w", v1Err)
		}
		return fmt.Errorf("connection failed: %w", err)
	}
	if code == http.StatusUnauthorized || code == http.StatusForbidden {
		return fmt.Errorf("authentication failed (HTTP %d)", code)
	}
	if minerUHealthOK(code, body) {
		return nil
	}
	return fmt.Errorf("not an Unlimited-OCR-2 service: HTTP %d", code)
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

	if apiConfig != nil && apiConfig.ApiKey != nil {
		if token := MinerUBearerTokenFromAPIKey(*apiConfig.ApiKey); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
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
		if time.Now().After(deadline) {
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
