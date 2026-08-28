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
	"ragflow/internal/common"
	"strings"
)

// azureAPIVersion is the Azure OpenAI REST API version sent as the
const azureAPIVersion = "2024-10-21"

// AzureOpenAIModel implements ModelDriver for Azure OpenAI.
type AzureOpenAIModel struct {
	baseModel BaseModel
}

// NewAzureOpenAIModel creates a new Azure OpenAI model instance.
func NewAzureOpenAIModel(baseURL map[string]string, urlSuffix URLSuffix) *AzureOpenAIModel {
	return &AzureOpenAIModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(false),
			// Azure OpenAI authenticates with the non-standard "api-key"
			// header instead of "Authorization: Bearer".
			authHeader: func(cfg *APIConfig) (string, string) {
				return "api-key", *cfg.ApiKey
			},
		},
	}
}

func (a *AzureOpenAIModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewAzureOpenAIModel(baseURL, a.baseModel.URLSuffix)
}

func (a *AzureOpenAIModel) Name() string {
	return "azure-openai"
}

// deploymentURL builds a deployment-scoped data-plane URL of the form
func (a *AzureOpenAIModel) deploymentURL(baseURL, deployment, op string) string {
	return fmt.Sprintf("%s/deployments/%s/%s?api-version=%s",
		strings.TrimRight(baseURL, "/"), deployment, op, azureAPIVersion)
}

// ChatWithMessages sends multiple messages with roles and returns the response.
func (a *AzureOpenAIModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	if modelName == "" {
		return nil, fmt.Errorf("deployment name is required")
	}

	baseURL, err := a.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := a.deploymentURL(baseURL, modelName, a.baseModel.URLSuffix.Chat)

	// Azure preserves its own body shape (no "model" field; deployment is in
	// the URL; temperature defaults to 1) rather than using buildRequestBody
	// which would inject an unknown "model" field Azure rejects.
	reqBody := map[string]interface{}{
		"messages":    buildChatMessages(messages),
		"stream":      false,
		"temperature": 1,
	}

	if chatModelConfig != nil {
		if chatModelConfig.Temperature != nil {
			reqBody["temperature"] = *chatModelConfig.Temperature
		}
		if chatModelConfig.TopP != nil {
			reqBody["top_p"] = *chatModelConfig.TopP
		}
		if chatModelConfig.Stop != nil {
			reqBody["stop"] = *chatModelConfig.Stop
		}
	}

	body, err := a.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return HandleNonStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

// ChatStreamlyWithSender sends messages and streams the response via the
// sender function. Used for streaming chat responses with no extra channel.
func (a *AzureOpenAIModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	if modelName == "" {
		return fmt.Errorf("deployment name is required")
	}

	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	baseURL, err := a.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := a.deploymentURL(baseURL, modelName, a.baseModel.URLSuffix.Chat)

	reqBody := map[string]interface{}{
		"messages": buildChatMessages(messages),
		"stream":   true,
	}

	if chatModelConfig != nil {
		if chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
			return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
		}
		if chatModelConfig.Temperature != nil {
			reqBody["temperature"] = *chatModelConfig.Temperature
		}
		if chatModelConfig.TopP != nil {
			reqBody["top_p"] = *chatModelConfig.TopP
		}
		if chatModelConfig.Stop != nil {
			reqBody["stop"] = *chatModelConfig.Stop
		}
	}

	// Request token usage in streaming response. Azure OpenAI only
	// includes usage in the final chunk when stream_options.include_usage
	// is set.
	reqBody["stream_options"] = map[string]any{"include_usage": true}

	return a.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		return HandleStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig, sender)
	})
}

type azureEmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// Embed turns a list of texts into embedding vectors
func (a *AzureOpenAIModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(request.Texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("deployment name is required")
	}

	baseURL, err := a.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := a.deploymentURL(baseURL, *modelName, a.baseModel.URLSuffix.Embedding)

	// As with chat, the deployment is in the URL path, so no "model" field.
	reqBody := map[string]interface{}{
		"input": request.Texts,
	}
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		reqBody["dimensions"] = embeddingConfig.Dimension
	}

	body, err := a.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	var parsed azureEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	embeddings := make([]EmbeddingData, len(request.Texts))
	for _, d := range parsed.Data {
		if d.Index < 0 || d.Index >= len(embeddings) {
			return nil, fmt.Errorf("embedding index %d out of range [0,%d)", d.Index, len(embeddings))
		}
		embeddings[d.Index] = EmbeddingData{
			Embedding: d.Embedding,
			Index:     d.Index,
		}
	}

	return embeddings, nil
}

// ListModels returns the deployment names visible to the configured API key.
func (a *AzureOpenAIModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	baseURL, err := a.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s?api-version=%s",
		strings.TrimRight(baseURL, "/"), a.baseModel.URLSuffix.Models, azureAPIVersion)

	body, err := a.baseModel.doGetRequest(ctx, url, apiConfig, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	// Parse response
	var modelList ModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if modelList.Models == nil {
		return nil, fmt.Errorf("invalid models list format")
	}

	return ParseListModel(modelList), nil
}

// CheckConnection runs a lightweight ListModels call to verify the endpoint
func (a *AzureOpenAIModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := a.ListModels(ctx, apiConfig)
	return err
}

// Balance is not exposed by the Azure OpenAI API.
func (a *AzureOpenAIModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

// Rerank is not exposed by the Azure OpenAI API.
func (a *AzureOpenAIModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AzureOpenAIModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AzureOpenAIModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", a.Name())
}

func (a *AzureOpenAIModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AzureOpenAIModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", a.Name())
}

func (a *AzureOpenAIModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AzureOpenAIModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AzureOpenAIModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AzureOpenAIModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}
