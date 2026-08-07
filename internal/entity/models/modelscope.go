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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"ragflow/internal/common"
	"strings"
)

// ModelScopeModel implements ModelDriver for ModelScope chat models.
type ModelScopeModel struct {
	baseModel BaseModel
}

type modelscopeModelListResponse struct {
	Data []ModelListItem `json:"data"`
}

// NewModelScopeModel creates a new ModelScope model instance.
func NewModelScopeModel(baseURL map[string]string, urlSuffix URLSuffix) *ModelScopeModel {
	return &ModelScopeModel{
		baseModel: BaseModel{
			BaseURL:          baseURL,
			URLSuffix:        urlSuffix,
			AllowEmptyAPIKey: true,
			httpClient:       NewDriverHTTPClient(true),
		},
	}
}

func (m *ModelScopeModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewModelScopeModel(baseURL, m.baseModel.URLSuffix)
}

func (m *ModelScopeModel) Name() string {
	return "ModelScope"
}

func normalizeModelScopeBaseURL(base string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if trimmed == "" {
		return trimmed
	}
	if strings.HasSuffix(trimmed, "/v1") {
		return strings.TrimSuffix(trimmed, "/v1")
	}
	return trimmed
}

// ChatWithMessages sends multiple messages with roles and returns the response.
func (m *ModelScopeModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	baseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = normalizeModelScopeBaseURL(baseURL)
	url := fmt.Sprintf("%s/%s", baseURL, m.baseModel.URLSuffix.Chat)

	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)

	body, err := m.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return HandleNonStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

// ChatStreamlyWithSender sends messages and streams response via sender.
func (m *ModelScopeModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if err := validateStreamConfig(chatModelConfig); err != nil {
		return err
	}

	baseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	baseURL = normalizeModelScopeBaseURL(baseURL)
	url := fmt.Sprintf("%s/%s", baseURL, m.baseModel.URLSuffix.Chat)

	reqBody := buildRequestBody(chatModelConfig, modelName, messages, true)
	reqBody["stream_options"] = map[string]interface{}{"include_usage": true}

	return m.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		return HandleStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig, sender)
	})
}

func (m *ModelScopeModel) Embed(ctx context.Context, modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *ModelScopeModel) Rerank(ctx context.Context, modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *ModelScopeModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *ModelScopeModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

func (m *ModelScopeModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, asrConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *ModelScopeModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

func (m *ModelScopeModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *ModelScopeModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

// ListModels returns the model IDs exposed by ModelScope's OpenAI-compatible
// /v1/models endpoint.
func (m *ModelScopeModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	baseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = normalizeModelScopeBaseURL(baseURL)
	url := fmt.Sprintf("%s/%s", baseURL, m.baseModel.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result modelscopeModelListResponse
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return ParseListModel(ModelList{Models: result.Data}), nil
}

func (m *ModelScopeModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *ModelScopeModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := m.ListModels(ctx, apiConfig)
	return err
}

func (m *ModelScopeModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *ModelScopeModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}
