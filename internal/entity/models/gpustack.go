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
	"net/http"
	"ragflow/internal/common"
	"strings"
)

// GPUStackModel implements ModelDriver for GPUStack
type GPUStackModel struct {
	baseModel BaseModel
}

func NewGPUStackModel(baseURL map[string]string, urlSuffix URLSuffix) *GPUStackModel {
	return &GPUStackModel{
		baseModel: BaseModel{
			BaseURL:          baseURL,
			URLSuffix:        urlSuffix,
			AllowEmptyAPIKey: true,
			httpClient:       common.GetSchemeSafeHTTPClient(),
		},
	}
}

func (g *GPUStackModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewGPUStackModel(baseURL, g.baseModel.URLSuffix)
}

func (g *GPUStackModel) Name() string {
	return "gpustack"
}

// ChatWithMessages sends multiple messages and returns the response.
func (g *GPUStackModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	url, err := g.baseModel.EndpointURL(apiConfig, g.baseModel.URLSuffix.Chat)
	if err != nil {
		return nil, err
	}
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)

	body, err := g.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return HandleNonStreamingResponse(ctx, body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

// ChatStreamlyWithSender streams the response via the sender.
func (g *GPUStackModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	if strings.TrimSpace(modelName) == "" {
		return fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if err := validateStreamConfig(chatModelConfig); err != nil {
		return err
	}

	url, err := g.baseModel.EndpointURL(apiConfig, g.baseModel.URLSuffix.Chat)
	if err != nil {
		return err
	}
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, true)
	reqBody["stream_options"] = map[string]any{"include_usage": true}

	return g.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		return HandleStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig, sender)
	})
}

type gpustackModelInfo struct {
	ID string `json:"id"`
}

type gpustackModelsResponse struct {
	Data []ModelListItem `json:"data"`
}

func (g *GPUStackModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	url, err := g.baseModel.EndpointURL(apiConfig, g.baseModel.URLSuffix.Models)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := g.baseModel.httpClient.Do(req)
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

	var parsed gpustackModelsResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return ParseListModel(ModelList{Models: parsed.Data}), nil
}

func (g *GPUStackModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := g.ListModels(ctx, apiConfig)
	return err
}

// gpustackEmbeddingData is one element in a GPUStack embeddings response.
type gpustackEmbeddingData struct {
	Embedding []float64 `json:"embedding"`
	Index     *int      `json:"index"`
}

// gpustackEmbeddingResponse is the JSON body returned by GPUStack embeddings API.
type gpustackEmbeddingResponse struct {
	Data []gpustackEmbeddingData `json:"data"`
}

// Embed requests embedding vectors via GPUStack's v1-openai/embeddings endpoint.
func (g *GPUStackModel) Embed(
	ctx context.Context,
	modelName *string,
	request EmbedRequest,
	apiConfig *APIConfig,
	embeddingConfig *EmbeddingConfig,
	modelUsage *common.ModelUsage,
) ([]EmbeddingData, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(request.Texts) == 0 {
		return []EmbeddingData{}, nil
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}

	baseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	if g.baseModel.URLSuffix.Embedding == "" {
		return nil, fmt.Errorf("gpustack: embedding URL suffix is not configured")
	}
	url := joinURL(baseURL, g.baseModel.URLSuffix.Embedding)

	reqBody := map[string]interface{}{
		"model": *modelName,
		"input": request.Texts,
	}
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		reqBody["dimensions"] = embeddingConfig.Dimension
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := g.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gpustack embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed gpustackEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	embeddings := make([]EmbeddingData, len(request.Texts))
	filled := make([]bool, len(request.Texts))
	for _, item := range parsed.Data {
		if item.Index == nil {
			return nil, fmt.Errorf("gpustack: missing embedding index in response item")
		}
		idx := *item.Index
		if idx < 0 || idx >= len(request.Texts) {
			return nil, fmt.Errorf("gpustack: embedding response index %d out of range for %d inputs", idx, len(request.Texts))
		}
		if filled[idx] {
			return nil, fmt.Errorf("gpustack: duplicate embedding index %d in response", idx)
		}
		if len(item.Embedding) == 0 {
			return nil, fmt.Errorf("gpustack: empty embedding vector for input index %d", idx)
		}
		embeddings[idx] = EmbeddingData{
			Embedding: item.Embedding,
			Index:     idx,
		}
		filled[idx] = true
	}
	for i, ok := range filled {
		if !ok {
			return nil, fmt.Errorf("gpustack: missing embedding for input index %d", i)
		}
	}
	return embeddings, nil
}

// gpustackRerankRequest is the body GPUStack's v1/rerank route accepts
// (gpustack/routes/rerank.py RerankRequest); return_documents defaults to true
// and the echoed document text is ignored on parse.
type gpustackRerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n"`
}

// gpustackRerankResponse is the JSON body returned by GPUStack's v1/rerank
// endpoint (gpustack/routes/rerank.py RerankResponse); per-result `document`
// echo is ignored.
type gpustackRerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
	// GPUStack reports the backend's token usage alongside the scores.
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// Rerank scores documents against the query via GPUStack's v1/rerank endpoint,
// which proxies to the serving backend (vLLM, ...). Python ragflow had this as
// GPUStackRerank; the Go rewrite stubbed it out with "no such method" even
// though llm_factories.json still advertises TEXT RE-RANK for GPUStack.
func (g *GPUStackModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	documents := request.Documents
	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if g.baseModel.URLSuffix.Rerank == "" {
		return nil, fmt.Errorf("gpustack: rerank URL suffix is not configured")
	}

	baseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := joinURL(baseURL, g.baseModel.URLSuffix.Rerank)

	topN := len(documents)
	if rerankConfig != nil && rerankConfig.TopN > 0 && rerankConfig.TopN < topN {
		topN = rerankConfig.TopN
	}

	reqBody := gpustackRerankRequest{
		Model:     *modelName,
		Query:     request.Query,
		Documents: documents,
		TopN:      topN,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := g.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gpustack rerank API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed gpustackRerankResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	rerankResponse := RerankResponse{Data: make([]RerankResult, 0, len(parsed.Results))}
	seen := make([]bool, len(documents))
	for _, item := range parsed.Results {
		if item.Index < 0 || item.Index >= len(documents) {
			return nil, fmt.Errorf("gpustack: rerank index %d out of range for %d inputs", item.Index, len(documents))
		}
		if seen[item.Index] {
			return nil, fmt.Errorf("gpustack: duplicate rerank index %d in response", item.Index)
		}
		seen[item.Index] = true
		rerankResponse.Data = append(rerankResponse.Data, RerankResult{
			Index:          item.Index,
			RelevanceScore: item.RelevanceScore,
		})
	}

	// Report the provider's token usage, mirroring this driver's Embed and the
	// vLLM driver's Rerank, so rerank calls are counted instead of reporting 0.
	recordResponseUsage(modelUsage, "", &TokenUsage{
		PromptTokens: parsed.Usage.PromptTokens,
		TotalTokens:  parsed.Usage.TotalTokens,
	}, "rerank")

	return &rerankResponse, nil
}

func (g *GPUStackModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GPUStackModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GPUStackModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", g.Name())
}

func (g *GPUStackModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GPUStackModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", g.Name())
}

func (g *GPUStackModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GPUStackModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GPUStackModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GPUStackModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GPUStackModel) Decide(ctx context.Context, modelName *string, request *DecideRequest, apiConfig *APIConfig, modelUsage *common.ModelUsage) (*DecideResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}
