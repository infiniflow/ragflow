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
	"slices"
	"sort"
	"strings"

	"go.uber.org/zap"
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

type gpustackModelItem struct {
	ModelListItem
	Meta struct {
		Voices      []string `json:"voices"`
		MaxModelLen *int     `json:"max_model_len"`
	} `json:"meta"`
}

type gpustackModelsResponse struct {
	Data []gpustackModelItem `json:"data"`
}

const (
	// gpustackImageCategory marks image-generation models, which RAGFlow has no
	// model type for.
	gpustackImageCategory = "image"
	gpustackTTSCategory   = "text_to_speech"
)

// gpustackCategoryModelTypes maps GPUStack model categories to RAGFlow model
// types.
var gpustackCategoryModelTypes = map[string]string{
	"llm":               modelTypeChat,
	"embedding":         modelTypeEmbedding,
	"reranker":          modelTypeRerank,
	"speech_to_text":    modelTypeASR,
	gpustackTTSCategory: modelTypeTTS,
}

func (g *GPUStackModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	url, err := g.baseModel.EndpointURL(apiConfig, g.baseModel.URLSuffix.Models)
	if err != nil {
		return nil, err
	}

	items, err := g.fetchModels(ctx, url+"?with_meta=true", apiConfig)
	if err != nil {
		return nil, err
	}

	categories := g.fetchModelCategories(ctx, url, apiConfig, items)
	listItems := make([]ModelListItem, 0, len(items))
	for _, item := range items {
		if item.ContextLength == nil {
			item.ContextLength = item.Meta.MaxModelLen
		}
		listItems = append(listItems, item.ModelListItem)
		// Only TTS models declare voices, even when GPUStack filed them under "llm".
		if len(item.Meta.Voices) > 0 {
			if categories == nil {
				categories = make(map[string][]string, len(items))
			}
			categories[strings.TrimSpace(item.ID)] = []string{gpustackTTSCategory}
		}
	}

	return applyGPUStackCategories(ParseListModel(ModelList{Models: listItems}), categories), nil
}

func (g *GPUStackModel) fetchModels(ctx context.Context, url string, apiConfig *APIConfig) ([]gpustackModelItem, error) {
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
	return parsed.Data, nil
}

// fetchModelCategories returns the GPUStack categories of each listed model.
//
// A GPUStack model is served under a route name its operator chose, so the name
// says nothing reliable about what the model does. /v1/models does not return
// categories but can filter by them, so each category is asked for in turn.
//
// The result is best effort: it is nil when a lookup fails or the server ignores
// the filter, and routes created by hand may have no categories at all. Callers
// fall back to the types derived from the model name for anything not returned.
func (g *GPUStackModel) fetchModelCategories(ctx context.Context, url string, apiConfig *APIConfig, items []gpustackModelItem) map[string][]string {
	if len(items) == 0 {
		return nil
	}

	queried := make([]string, 0, len(gpustackCategoryModelTypes)+1)
	for category := range gpustackCategoryModelTypes {
		queried = append(queried, category)
	}
	queried = append(queried, gpustackImageCategory)
	sort.Strings(queried)

	categories := make(map[string][]string, len(items))
	unfiltered := 0
	for _, category := range queried {
		matched, err := g.fetchModels(ctx, url+"?categories="+category, apiConfig)
		if err != nil {
			common.Warn("gpustack: failed to list models by category", zap.String("category", category), zap.Error(err))
			return nil
		}
		// One full listing is a cluster serving only that category. Two mean
		// the server is not applying the filter.
		if len(matched) == len(items) {
			unfiltered++
			if unfiltered > 1 {
				return nil
			}
		}
		for _, item := range matched {
			id := strings.TrimSpace(item.ID)
			categories[id] = append(categories[id], category)
		}
	}
	return categories
}

// applyGPUStackCategories overrides the name-derived types of models GPUStack
// reported categories for, and drops models RAGFlow cannot use. Vision is kept
// from the name-derived types because GPUStack has no category for it.
func applyGPUStackCategories(listed []ListModelResponse, categories map[string][]string) []ListModelResponse {
	if categories == nil {
		return listed
	}

	result := make([]ListModelResponse, 0, len(listed))
	for _, model := range listed {
		modelCategories := categories[model.Name]
		if len(modelCategories) == 0 {
			result = append(result, model)
			continue
		}

		modelTypes := make([]string, 0, len(modelCategories))
		for _, category := range modelCategories {
			if modelType, ok := gpustackCategoryModelTypes[category]; ok {
				modelTypes = append(modelTypes, modelType)
			}
		}
		if len(modelTypes) == 0 {
			continue
		}
		// GPUStack files architectures it does not recognise under "llm", so that
		// category alone does not outweigh a more specific type from the name.
		if len(modelTypes) == 1 && modelTypes[0] == modelTypeChat && !slices.Contains(model.ModelTypes, modelTypeChat) {
			result = append(result, model)
			continue
		}
		if slices.Contains(modelTypes, modelTypeChat) && slices.Contains(model.ModelTypes, modelTypeVision) {
			modelTypes = append(modelTypes, modelTypeVision)
		}
		model.ModelTypes = normalizeModelTypes(modelTypes)
		result = append(result, model)
	}
	return result
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
	ID    string                  `json:"id"`
	Data  []gpustackEmbeddingData `json:"data"`
	Usage *TokenUsage             `json:"usage"`
}

// Embed requests embedding vectors via GPUStack's v1/embeddings endpoint.
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
	recordResponseUsage(modelUsage, parsed.ID, parsed.Usage, "embedding")
	return embeddings, nil
}

func (g *GPUStackModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if len(request.Documents) == 0 {
		return &RerankResponse{}, nil
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if strings.TrimSpace(g.baseModel.URLSuffix.Rerank) == "" {
		return nil, fmt.Errorf("gpustack: rerank URL suffix is not configured")
	}
	url, err := g.baseModel.EndpointURL(apiConfig, g.baseModel.URLSuffix.Rerank)
	if err != nil {
		return nil, err
	}

	topN := len(request.Documents)
	if rerankConfig != nil && rerankConfig.TopN > 0 && rerankConfig.TopN < topN {
		topN = rerankConfig.TopN
	}
	body, err := g.baseModel.doRequest(ctx, url, apiConfig, map[string]any{
		"model":     *modelName,
		"query":     request.Query,
		"documents": request.Documents,
		"top_n":     topN,
	}, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		ID      string `json:"id"`
		Results []struct {
			Index          *int     `json:"index"`
			RelevanceScore *float64 `json:"relevance_score"`
		} `json:"results"`
		Usage *TokenUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("gpustack: failed to parse rerank response: %w", err)
	}
	if parsed.Results == nil {
		return nil, fmt.Errorf("gpustack: missing rerank results in response")
	}
	result := &RerankResponse{Data: make([]RerankResult, 0, len(parsed.Results))}
	seen := make(map[int]bool, len(parsed.Results))
	for _, item := range parsed.Results {
		if item.Index == nil || item.RelevanceScore == nil {
			return nil, fmt.Errorf("gpustack: rerank result requires index and relevance_score")
		}
		index := *item.Index
		if index < 0 || index >= len(request.Documents) {
			return nil, fmt.Errorf("gpustack: rerank index %d out of range for %d inputs", index, len(request.Documents))
		}
		if seen[index] {
			return nil, fmt.Errorf("gpustack: duplicate rerank index %d in response", index)
		}
		seen[index] = true
		result.Data = append(result.Data, RerankResult{Index: index, RelevanceScore: *item.RelevanceScore})
	}
	recordResponseUsage(modelUsage, parsed.ID, parsed.Usage, "rerank")
	return result, nil
}

func (g *GPUStackModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s: no such method: Balance", g.Name())
}

// audioModel reuses the OpenAI audio client. VoxBox, GPUStack's audio backend,
// answers failures with HTTP 200 and the error in the JSON body.
func (g *GPUStackModel) audioModel() *OpenAIModel {
	return &OpenAIModel{baseModel: g.baseModel, skipTTSVoiceValidation: true, detectEmbeddedAudioErrors: true}
}

func (g *GPUStackModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return g.audioModel().TranscribeAudio(ctx, modelName, file, apiConfig, asrConfig, modelUsage)
}

func (g *GPUStackModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return g.audioModel().TranscribeAudioWithSender(ctx, modelName, file, apiConfig, asrConfig, modelUsage, sender)
}

func (g *GPUStackModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return g.audioModel().AudioSpeech(ctx, modelName, audioContent, apiConfig, g.withDefaultVoice(ctx, modelName, apiConfig, ttsConfig), modelUsage)
}

func (g *GPUStackModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return g.audioModel().AudioSpeechWithSender(ctx, modelName, audioContent, apiConfig, g.withDefaultVoice(ctx, modelName, apiConfig, ttsConfig), modelUsage, sender)
}

// withDefaultVoice fills in the first voice the model declares when the caller
// chose none: VoxBox requires one, while chat read-aloud never sends it. A failed
// lookup leaves the config unchanged so the backend reports the problem.
func (g *GPUStackModel) withDefaultVoice(ctx context.Context, modelName *string, apiConfig *APIConfig, ttsConfig *TTSConfig) *TTSConfig {
	if ttsConfig != nil {
		switch voice := ttsConfig.Params["voice"].(type) {
		case nil:
		case string:
			if strings.TrimSpace(voice) != "" {
				return ttsConfig
			}
		default:
			return ttsConfig
		}
	}
	if modelName == nil || strings.TrimSpace(g.baseModel.URLSuffix.Models) == "" {
		return ttsConfig
	}
	url, err := g.baseModel.EndpointURL(apiConfig, g.baseModel.URLSuffix.Models)
	if err != nil {
		return ttsConfig
	}
	items, err := g.fetchModels(ctx, url+"?with_meta=true", apiConfig)
	if err != nil {
		common.Warn("gpustack: failed to look up model voices", zap.String("model", *modelName), zap.Error(err))
		return ttsConfig
	}
	for _, item := range items {
		if item.ID != *modelName || len(item.Meta.Voices) == 0 {
			continue
		}
		withVoice := TTSConfig{Params: map[string]interface{}{"voice": item.Meta.Voices[0]}}
		if ttsConfig != nil {
			withVoice.Format = ttsConfig.Format
			for key, value := range ttsConfig.Params {
				if key != "voice" {
					withVoice.Params[key] = value
				}
			}
		}
		return &withVoice
	}
	return ttsConfig
}

func (g *GPUStackModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s: no such method: OCRFile", g.Name())
}

func (g *GPUStackModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s: no such method: ParseFile", g.Name())
}

func (g *GPUStackModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s: no such method: ListTasks", g.Name())
}

func (g *GPUStackModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s: no such method: ShowTask", g.Name())
}

func (g *GPUStackModel) Decide(ctx context.Context, modelName *string, request *DecideRequest, apiConfig *APIConfig, modelUsage *common.ModelUsage) (*DecideResponse, error) {
	return nil, fmt.Errorf("%s: no such method: Decide", g.Name())
}
