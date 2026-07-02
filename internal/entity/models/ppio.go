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
	"sort"
	"strings"
)

// PPIOModel implements ModelDriver for PPIO.
//
// PPIO exposes OpenAI-compatible chat completions and model listing endpoints.
type PPIOModel struct {
	baseModel BaseModel
}

func NewPPIOModel(baseURL map[string]string, urlSuffix URLSuffix) *PPIOModel {
	return &PPIOModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (p *PPIOModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewPPIOModel(baseURL, p.baseModel.URLSuffix)
}

func (p *PPIOModel) Name() string {
	return "ppio"
}

func (p *PPIOModel) endpoint(apiConfig *APIConfig, suffix string) (string, error) {
	baseURL, err := p.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return "", err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	return fmt.Sprintf("%s/%s", baseURL, strings.TrimPrefix(suffix, "/")), nil
}

func ppioChatPayload(modelName string, messages []Message, stream bool, chatModelConfig *ChatConfig) map[string]interface{} {
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	reqBody := map[string]interface{}{
		"model":    modelName,
		"messages": apiMessages,
		"stream":   stream,
	}

	if chatModelConfig != nil {
		if chatModelConfig.MaxTokens != nil {
			reqBody["max_tokens"] = *chatModelConfig.MaxTokens
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

	return reqBody
}

type ppioChatMessage struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
	Reasoning        string `json:"reasoning"`
}

type ppioChatChoice struct {
	Message      ppioChatMessage `json:"message"`
	Delta        ppioChatMessage `json:"delta"`
	FinishReason string          `json:"finish_reason"`
}

type ppioChatResponse struct {
	Choices      []ppioChatChoice `json:"choices"`
	Error        interface{}      `json:"error"`
	FinishReason string           `json:"finish_reason"`
}

func (p *PPIOModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if err := p.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	url, err := p.endpoint(apiConfig, p.baseModel.URLSuffix.Chat)
	if err != nil {
		return nil, err
	}

	jsonData, err := json.Marshal(ppioChatPayload(modelName, messages, false, chatModelConfig))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := p.baseModel.httpClient.Do(req)
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

	var result ppioChatResponse
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("ppio: upstream error: %v", result.Error)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	content := result.Choices[0].Message.Content
	reasonContent := result.Choices[0].Message.ReasoningContent
	if reasonContent == "" {
		reasonContent = result.Choices[0].Message.Reasoning
	}

	return &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}, nil
}

func (p *PPIOModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if err := p.baseModel.APIConfigCheck(apiConfig); err != nil {
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
	if chatModelConfig != nil && chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
		return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
	}

	url, err := p.endpoint(apiConfig, p.baseModel.URLSuffix.Chat)
	if err != nil {
		return err
	}

	jsonData, err := json.Marshal(ppioChatPayload(modelName, messages, true, chatModelConfig))
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), streamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	req.Header.Set("Accept", "text/event-stream")

	resp, err := p.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	sawTerminal := false
	done, err := ParseSSEStream[ppioChatResponse](resp.Body, func(event ppioChatResponse) error {
		if event.Error != nil {
			return fmt.Errorf("ppio: upstream stream error: %v", event.Error)
		}
		if len(event.Choices) == 0 {
			return nil
		}

		choice := event.Choices[0]
		reasoning := choice.Delta.ReasoningContent
		if reasoning == "" {
			reasoning = choice.Delta.Reasoning
		}
		if reasoning != "" {
			if err := sender(nil, &reasoning); err != nil {
				return err
			}
		}
		if choice.Delta.Content != "" {
			if err := sender(&choice.Delta.Content, nil); err != nil {
				return err
			}
		}
		if choice.FinishReason != "" || event.FinishReason != "" {
			sawTerminal = true
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !done && !sawTerminal {
		return fmt.Errorf("ppio: stream ended before [DONE] or finish_reason")
	}

	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

type ppioModelInfo struct {
	ID string `json:"id"`
}

type ppioListModelsResponse struct {
	Data  []DSModel   `json:"data"`
	Error interface{} `json:"error"`
}

func (p *PPIOModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := p.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	url, err := p.endpoint(apiConfig, p.baseModel.URLSuffix.Models)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := p.baseModel.httpClient.Do(req)
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

	var result ppioListModelsResponse
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("ppio: upstream error: %v", result.Error)
	}

	return ParseListModel(ModelList{Models: result.Data}), nil
}

func (p *PPIOModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := p.ListModels(apiConfig)
	return err
}

// ppioEmbeddingData is one element in a PPIO /embeddings response.
type ppioEmbeddingData struct {
	Embedding []float64 `json:"embedding"`
	Index     *int      `json:"index"`
}

// ppioEmbeddingResponse is the JSON body returned by PPIO embeddings API.
type ppioEmbeddingResponse struct {
	Data  []ppioEmbeddingData `json:"data"`
	Error interface{}         `json:"error,omitempty"`
}

// Embed requests embedding vectors via the PPIO OpenAI-compatible /embeddings endpoint.
func (p *PPIOModel) Embed(
	modelName *string,
	texts []string,
	apiConfig *APIConfig,
	embeddingConfig *EmbeddingConfig,
) ([]EmbeddingData, error) {
	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if p.URLSuffix.Embedding == "" {
		return nil, fmt.Errorf("ppio: embedding URL suffix is not configured")
	}

	url, err := p.endpoint(apiConfig, p.URLSuffix.Embedding)
	if err != nil {
		return nil, err
	}

	reqBody := map[string]interface{}{
		"model": *modelName,
		"input": texts,
	}
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		reqBody["dimensions"] = embeddingConfig.Dimension
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ppio embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed ppioEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("ppio: upstream error: %v", parsed.Error)
	}

	embeddings := make([]EmbeddingData, len(texts))
	filled := make([]bool, len(texts))
	for _, item := range parsed.Data {
		if item.Index == nil {
			return nil, fmt.Errorf("ppio: missing embedding index in response item")
		}
		idx := *item.Index
		if idx < 0 || idx >= len(texts) {
			return nil, fmt.Errorf("ppio: embedding response index %d out of range for %d inputs", idx, len(texts))
		}
		if filled[idx] {
			return nil, fmt.Errorf("ppio: duplicate embedding index %d in response", idx)
		}
		embeddings[idx] = EmbeddingData{
			Embedding: item.Embedding,
			Index:     idx,
		}
		filled[idx] = true
	}
	for i, ok := range filled {
		if !ok {
			return nil, fmt.Errorf("ppio: missing embedding for input index %d", i)
		}
	}
	return embeddings, nil
}

// ppioRerankResult is one scored document in a PPIO /rerank response.
type ppioRerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

// ppioRerankResponse is the JSON body returned by PPIO rerank API.
type ppioRerankResponse struct {
	Results []ppioRerankResult `json:"results"`
	Error   interface{}        `json:"error,omitempty"`
}

// Rerank scores documents against a query via PPIO's OpenAI-compatible
// POST /v1/rerank endpoint (see https://ppio.com/docs/models/reference-llm-create-rerank).
func (p *PPIOModel) Rerank(
	modelName *string,
	query string,
	documents []string,
	apiConfig *APIConfig,
	rerankConfig *RerankConfig,
) (*RerankResponse, error) {
	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if p.URLSuffix.Rerank == "" {
		return nil, fmt.Errorf("ppio: rerank URL suffix is not configured")
	}

	url, err := p.endpoint(apiConfig, p.URLSuffix.Rerank)
	if err != nil {
		return nil, err
	}

	topN := len(documents)
	if rerankConfig != nil && rerankConfig.TopN > 0 && rerankConfig.TopN < topN {
		topN = rerankConfig.TopN
	}

	reqBody := map[string]interface{}{
		"model":     *modelName,
		"query":     query,
		"documents": documents,
		"top_n":     topN,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ppio rerank API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed ppioRerankResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("ppio: upstream error: %v", parsed.Error)
	}

	rerankResponse := RerankResponse{Data: make([]RerankResult, 0, len(parsed.Results))}
	seen := make([]bool, len(documents))
	for _, item := range parsed.Results {
		if item.Index < 0 || item.Index >= len(documents) {
			return nil, fmt.Errorf("ppio: rerank index %d out of range for %d inputs", item.Index, len(documents))
		}
		if seen[item.Index] {
			return nil, fmt.Errorf("ppio: duplicate rerank index %d in response", item.Index)
		}
		rerankResponse.Data = append(rerankResponse.Data, RerankResult{
			Index:          item.Index,
			RelevanceScore: item.RelevanceScore,
		})
		seen[item.Index] = true
	}
	// Normalize output by input index so the mapping is deterministic
	// regardless of the order PPIO returns scored results in.
	sort.Slice(rerankResponse.Data, func(i, j int) bool {
		return rerankResponse.Data[i].Index < rerankResponse.Data[j].Index
	})
	return &rerankResponse, nil
}

func (p *PPIOModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PPIOModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PPIOModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", p.Name())
}

func (p *PPIOModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PPIOModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", p.Name())
}

func (p *PPIOModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PPIOModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PPIOModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PPIOModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}
