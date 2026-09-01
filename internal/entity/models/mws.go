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
	"net/url"
	"strings"

	"ragflow/internal/common"
)

// MWSModel implements the MWS GPT Model Hub chat, embedding, and reranking APIs.
type MWSModel struct {
	*DummyModel
}

// NewMWSModel creates an MWS model driver.
func NewMWSModel(baseURL map[string]string, urlSuffix URLSuffix) *MWSModel {
	driver := NewDummyModel(baseURL, urlSuffix)
	driver.baseModel.httpClient = NewDriverHTTPClient(true)
	return &MWSModel{DummyModel: driver}
}

// Name returns the public provider identifier.
func (m *MWSModel) Name() string {
	return "MWS"
}

// NewInstance creates an MWS driver with tenant-specific base URLs.
func (m *MWSModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewMWSModel(baseURL, m.baseModel.URLSuffix)
}

func normalizeMWSProjectURL(rawURL string) (string, error) {
	value := strings.TrimSuffix(strings.TrimSpace(rawURL), "/")
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return "", fmt.Errorf("MWS API URL must be a project root in the form https://gpt.mwsapis.ru/projects/<project>")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != "projects" || parts[1] == "" {
		return "", fmt.Errorf("MWS API URL must be a project root in the form https://gpt.mwsapis.ru/projects/<project>")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.User != nil {
		return "", fmt.Errorf("MWS API URL must not contain credentials, a query string, or a fragment")
	}
	parsed.Path = "/projects/" + parts[1]
	parsed.RawPath = ""
	return parsed.String(), nil
}

func buildMWSEndpoint(projectURL, endpoint string) (string, error) {
	root, err := normalizeMWSProjectURL(projectURL)
	if err != nil {
		return "", err
	}
	return root + "/" + strings.Trim(endpoint, "/"), nil
}

func (m *MWSModel) endpoint(apiConfig *APIConfig, endpoint string) (string, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return "", err
	}
	baseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return "", err
	}
	return buildMWSEndpoint(baseURL, endpoint)
}

func (m *MWSModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	endpoint, err := m.endpoint(apiConfig, "openai/v1/models")
	if err != nil {
		return nil, err
	}
	body, err := m.baseModel.doGetRequest(ctx, endpoint, apiConfig, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	var modelList ModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse MWS model list: %w", err)
	}
	if modelList.Models == nil {
		return nil, fmt.Errorf("invalid MWS models list format")
	}

	models := make([]ListModelResponse, 0, len(modelList.Models))
	for _, model := range modelList.Models {
		modelTypes := InferModelTypes(model.ID)
		if len(modelTypes) != 1 || (modelTypes[0] != "chat" && modelTypes[0] != "embedding" && modelTypes[0] != "rerank") {
			continue
		}
		models = append(models, ListModelResponse{Name: model.ID, ModelTypes: modelTypes})
	}
	return models, nil
}

func buildMWSChatMessages(messages []Message) ([]map[string]any, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages are required")
	}
	result := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		if message.Role != "system" && message.Role != "user" && message.Role != "assistant" {
			return nil, fmt.Errorf("unsupported MWS chat message role %q", message.Role)
		}
		content, ok := message.Content.(string)
		if !ok {
			return nil, fmt.Errorf("MWS chat message content must be a string")
		}
		result = append(result, map[string]any{
			"role":    message.Role,
			"content": content,
		})
	}
	return result, nil
}

func buildMWSChatRequest(modelName string, messages []Message, config *ChatConfig, stream bool) (map[string]any, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}
	chatMessages, err := buildMWSChatMessages(messages)
	if err != nil {
		return nil, err
	}
	request := map[string]any{
		"model":    modelName,
		"messages": chatMessages,
	}
	if config != nil {
		if config.Temperature != nil {
			request["temperature"] = *config.Temperature
		}
		if config.MaxTokens != nil {
			request["max_completion_tokens"] = *config.MaxTokens
		}
	}
	if stream {
		request["stream"] = true
		request["stream_options"] = map[string]any{"include_usage": true}
	}
	return request, nil
}

// ChatWithMessages sends a non-streaming MWS Chat Completions request.
func (m *MWSModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	endpoint, err := m.endpoint(apiConfig, "openai/v1/chat/completions")
	if err != nil {
		return nil, err
	}
	request, err := buildMWSChatRequest(modelName, messages, chatConfig, false)
	if err != nil {
		return nil, err
	}
	body, err := m.baseModel.doRequest(ctx, endpoint, apiConfig, request, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}
	return HandleNonStreamingResponse(body, modelUsage, chatConfig, OpenAIParserConfig)
}

// ChatStreamlyWithSender sends a streaming MWS Chat Completions request.
func (m *MWSModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	if err := validateStreamConfig(chatConfig); err != nil {
		return err
	}
	endpoint, err := m.endpoint(apiConfig, "openai/v1/chat/completions")
	if err != nil {
		return err
	}
	request, err := buildMWSChatRequest(modelName, messages, chatConfig, true)
	if err != nil {
		return err
	}
	return m.baseModel.doStreamRequest(ctx, endpoint, apiConfig, request, streamCallTimeout, func(body io.ReadCloser) error {
		return HandleStreamingResponse(body, modelUsage, chatConfig, OpenAIParserConfig, sender)
	})
}

type mwsEmbeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Usage TokenUsage `json:"usage"`
}

func (m *MWSModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, _ *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	endpoint, err := m.endpoint(apiConfig, "openai/v1/embeddings")
	if err != nil {
		return nil, err
	}
	if len(request.Texts) == 0 {
		return []EmbeddingData{}, nil
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}

	body, err := m.baseModel.doRequest(ctx, endpoint, apiConfig, map[string]any{
		"model": *modelName,
		"input": request.Texts,
	}, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	var response mwsEmbeddingResponse
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse MWS embedding response: %w", err)
	}
	if len(response.Data) != len(request.Texts) {
		return nil, fmt.Errorf("MWS embedding response returned %d vectors for %d inputs", len(response.Data), len(request.Texts))
	}

	embeddings := make([]EmbeddingData, len(request.Texts))
	seen := make([]bool, len(request.Texts))
	for _, item := range response.Data {
		if item.Index < 0 || item.Index >= len(request.Texts) || seen[item.Index] {
			return nil, fmt.Errorf("unexpected MWS embedding index %d for %d inputs", item.Index, len(request.Texts))
		}
		seen[item.Index] = true
		embeddings[item.Index] = EmbeddingData{Index: item.Index, Embedding: item.Embedding}
	}
	recordResponseUsage(modelUsage, "", &response.Usage, "embedding")
	return embeddings, nil
}

type mwsRerankResponse struct {
	ID      string `json:"id"`
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
	Meta struct {
		Tokens struct {
			InputTokens int `json:"input_tokens"`
		} `json:"tokens"`
	} `json:"meta"`
}

func (m *MWSModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, _ *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	endpoint, err := m.endpoint(apiConfig, "cohere/v2/rerank")
	if err != nil {
		return nil, err
	}
	documents := request.Documents
	query := request.Query
	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}

	body, err := m.baseModel.doRequest(ctx, endpoint, apiConfig, map[string]any{
		"model":     *modelName,
		"query":     query,
		"documents": documents,
		"top_n":     len(documents),
	}, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	var response mwsRerankResponse
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse MWS rerank response: %w", err)
	}
	if len(response.Results) != len(documents) {
		return nil, fmt.Errorf("MWS rerank response returned %d results for %d documents", len(response.Results), len(documents))
	}
	ranked := make([]RerankResult, len(documents))
	seen := make([]bool, len(documents))
	for i := range ranked {
		ranked[i].Index = i
	}
	for _, item := range response.Results {
		if item.Index < 0 || item.Index >= len(documents) || seen[item.Index] {
			return nil, fmt.Errorf("unexpected MWS rerank index %d for %d documents", item.Index, len(documents))
		}
		seen[item.Index] = true
		ranked[item.Index].RelevanceScore = item.RelevanceScore
	}
	usage := TokenUsage{PromptTokens: response.Meta.Tokens.InputTokens, TotalTokens: response.Meta.Tokens.InputTokens}
	recordResponseUsage(modelUsage, response.ID, &usage, "rerank")
	return &RerankResponse{Data: ranked}, nil
}

func (m *MWSModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := m.ListModels(ctx, apiConfig)
	return err
}
