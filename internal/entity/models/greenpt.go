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
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"ragflow/internal/common"
	"strings"
)

// GreenPTModel implements GreenPT's OpenAI-compatible chat, embedding,
// reranking, and model-list APIs, plus its Deepgram-compatible STT endpoint.
type GreenPTModel struct {
	*OpenAIAPICompatibleModel
}

// NewGreenPTModel creates a GreenPT model driver.
func NewGreenPTModel(baseURL map[string]string, urlSuffix URLSuffix) *GreenPTModel {
	return &GreenPTModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

func (m *GreenPTModel) Name() string {
	return "GreenPT"
}

func (m *GreenPTModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewGreenPTModel(baseURL, m.baseModel.URLSuffix)
}

// ListModels uses GreenPT's live /v1/models endpoint and corrects the two
// speech model IDs, whose names do not contain an ASR hint.
func (m *GreenPTModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	models, err := m.OpenAIAPICompatibleModel.ListModels(ctx, apiConfig)
	if err != nil {
		return nil, err
	}
	for i := range models {
		switch models[i].Name {
		case "green-s", "green-s-pro":
			models[i].ModelTypes = []string{"asr"}
		}
	}
	return models, nil
}

// ChatWithMessages sends multiple messages with roles and returns response
func (m *GreenPTModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := fmt.Sprintf("%s/%s", resolvedBaseURL, m.baseModel.URLSuffix.Chat)

	// Build request body
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)

	if chatModelConfig != nil && chatModelConfig.Thinking != nil {
		if *chatModelConfig.Thinking {
			reqBody["thinking"] = map[string]interface{}{
				"type": "enabled",
			}
		} else {
			reqBody["thinking"] = map[string]interface{}{
				"type": "disabled",
			}
		}
	}

	body, err := m.baseModel.doRequest(ctx, baseURL, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return HandleNonStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

// ChatStreamlyWithSender sends messages and streams response via sender function
func (m *GreenPTModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if chatModelConfig != nil {
		chatModelConfig.ToolCallsResult = nil
		chatModelConfig.UsageResult = nil
	}

	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	baseURL := fmt.Sprintf("%s/%s", resolvedBaseURL, m.baseModel.URLSuffix.Chat)

	// Build request body with streaming enabled
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, true)
	reqBody["stream_options"] = map[string]interface{}{
		"include_usage": true,
	}

	if chatModelConfig != nil && chatModelConfig.Thinking != nil {
		if *chatModelConfig.Thinking {
			reqBody["thinking"] = map[string]interface{}{
				"type": "enabled",
			}
		} else {
			reqBody["thinking"] = map[string]interface{}{
				"type": "disabled",
			}
		}
	}

	return m.baseModel.doStreamRequest(ctx, baseURL, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		return HandleStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig, sender)
	})
}

// TranscribeAudio calls GreenPT's Deepgram-compatible /v1/listen endpoint.
func (m *GreenPTModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if file == nil || strings.TrimSpace(*file) == "" {
		return nil, fmt.Errorf("file is missing")
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	audio, err := os.Open(*file)
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer audio.Close()

	baseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), strings.TrimPrefix(m.baseModel.URLSuffix.ASR, "/"))
	query := url.Values{"model": {*modelName}}
	if asrConfig != nil {
		for key, value := range asrConfig.Params {
			query.Set(key, fmt.Sprintf("%v", value))
		}
	}
	endpoint += "?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, audio)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(*file)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Token "+*apiConfig.ApiKey)

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
		return nil, fmt.Errorf("GreenPT ASR API error: %s, body: %s", resp.Status, string(body))
	}

	var result struct {
		Results struct {
			Channels []struct {
				Alternatives []struct {
					Transcript string `json:"transcript"`
				} `json:"alternatives"`
			} `json:"channels"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse GreenPT ASR response: %w", err)
	}
	if len(result.Results.Channels) == 0 || len(result.Results.Channels[0].Alternatives) == 0 {
		return nil, fmt.Errorf("GreenPT ASR response contains no transcript")
	}
	return &ASRResponse{Text: result.Results.Channels[0].Alternatives[0].Transcript}, nil
}
