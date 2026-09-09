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

// LongCatModel implements ModelDriver for LongCat (Meituan).
type LongCatModel struct {
	baseModel BaseModel
}

// NewLongCatModel creates a new LongCat model instance.
func NewLongCatModel(baseURL map[string]string, urlSuffix URLSuffix) *LongCatModel {
	return &LongCatModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(false),
		},
	}
}

func (l *LongCatModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewLongCatModel(baseURL, l.baseModel.URLSuffix)
}

func (l *LongCatModel) Name() string {
	return "longcat"
}

// LongCatChatResponse mirrors the OpenAI-compatible response returned by
// LongCat's POST /openai/v1/chat/completions endpoint.
type LongCatChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Role             string           `json:"role"`
			Content          string           `json:"content"`
			ReasoningContent string           `json:"reasoning_content"`
			ToolCalls        []map[string]any `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens            int `json:"prompt_tokens"`
		CompletionTokens        int `json:"completion_tokens"`
		TotalTokens             int `json:"total_tokens"`
		CompletionTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
}

// ChatWithMessages sends multiple messages with roles and returns the response.
func (l *LongCatModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := l.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	baseURL, err := l.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, l.baseModel.URLSuffix.Chat)

	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)
	delete(reqBody, "stop")

	body, err := l.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return HandleNonStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

// ChatStreamlyWithSender sends messages and streams the response via the
func (l *LongCatModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := l.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	baseURL, err := l.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, l.baseModel.URLSuffix.Chat)
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, true)
	delete(reqBody, "stop")
	reqBody["stream_options"] = map[string]any{"include_usage": true}

	if chatModelConfig != nil {
		if chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
			return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
		}
		chatModelConfig.ToolCallsResult = nil
		chatModelConfig.UsageResult = nil
	}

	return l.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		return HandleStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig, sender)
	})
}

type longCatModelInfo struct {
	ID string `json:"id"`
}

type longCatListModelsResponse struct {
	Data  []ModelListItem `json:"data"`
	Error interface{}     `json:"error"`
}

const longCatMaxListModelsResponseBytes = 1 << 20

func (l *LongCatModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := l.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	baseURL, err := l.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, l.baseModel.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := l.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, longCatMaxListModelsResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if len(body) > longCatMaxListModelsResponseBytes {
		return nil, fmt.Errorf("longcat: models response exceeds %d bytes", longCatMaxListModelsResponseBytes)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result longCatListModelsResponse
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("longcat: upstream error: %v", result.Error)
	}
	if result.Data == nil {
		return nil, fmt.Errorf("invalid models list format")
	}

	return ParseListModel(ModelList{Models: result.Data}), nil
}

func (l *LongCatModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := l.ListModels(ctx, apiConfig)
	return err
}

func (l *LongCatModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}

// Rerank is not exposed by the LongCat API.
func (l *LongCatModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}

// Balance is not exposed by the LongCat API.
func (l *LongCatModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}

// TranscribeAudio (ASR) is not exposed by the LongCat API.
func (l *LongCatModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}

func (l *LongCatModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", l.Name())
}

// AudioSpeech (TTS) is not exposed by the LongCat API.
func (l *LongCatModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}

func (l *LongCatModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", l.Name())
}

func (l *LongCatModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}

// ParseFile parse file
func (l *LongCatModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}

func (l *LongCatModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}

func (l *LongCatModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}
