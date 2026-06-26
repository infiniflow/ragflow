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

// VllmModel implements ModelDriver for Vllm AI
type VllmModel struct {
	baseModel BaseModel
}

// NewVllmModel creates a new Vllm AI model instance
func NewVllmModel(baseURL map[string]string, urlSuffix URLSuffix) *VllmModel {
	return &VllmModel{
		baseModel: BaseModel{
			BaseURL:          baseURL,
			URLSuffix:        urlSuffix,
			AllowEmptyAPIKey: true,
			httpClient:       NewDriverHTTPClient(),
		},
	}
}

func (v *VllmModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewVllmModel(baseURL, v.baseModel.URLSuffix)
}

func (v *VllmModel) Name() string {
	return "vllm"
}

// ChatWithMessages sends multiple messages with roles and returns response
func (v *VllmModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if err := v.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := v.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, v.baseModel.URLSuffix.Chat)

	// For qwen/glm models, use async chat endpoint
	modelType := strings.Split(modelName, "-")[0]
	if modelType == "qwen" || modelType == "glm" {
		url = fmt.Sprintf("%s/%s", resolvedBaseURL, v.baseModel.URLSuffix.AsyncChat)
	}

	// Convert messages to API format
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// Build request body
	reqBody := map[string]interface{}{
		"model":       modelName,
		"messages":    apiMessages,
		"stream":      false,
		"temperature": 1,
	}

	if chatModelConfig != nil {
		if chatModelConfig.Stream != nil {
			reqBody["stream"] = *chatModelConfig.Stream
		}

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

		if chatModelConfig.Thinking != nil {
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
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := v.baseModel.httpClient.Do(req)
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

	// Parse response
	var result map[string]interface{}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	firstChoice, ok := choices[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid choice format")
	}

	messageMap, ok := firstChoice["message"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid message format")
	}

	content, ok := messageMap["content"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid content format")
	}

	var reasonContent string
	if chatModelConfig != nil && chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
		reasonContent, ok = messageMap["reasoning_content"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid content format")
		}
		if reasonContent != "" && reasonContent[0] == '\n' {
			reasonContent = reasonContent[1:]
		}
	}

	chatResponse := &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}

	return chatResponse, nil
}

// ChatStreamlyWithSender sends messages and streams response via sender function (best performance, no channel)
func (v *VllmModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	if err := v.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := v.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, v.baseModel.URLSuffix.Chat)
	modelType := strings.Split(modelName, "-")[0]
	if modelType == "qwen" || modelType == "glm" {
		url = fmt.Sprintf("%s/%s", resolvedBaseURL, v.baseModel.URLSuffix.AsyncChat)
	}

	// Convert messages to API format (supporting multimodal content)
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// Build request body with streaming enabled
	reqBody := map[string]interface{}{
		"model":    modelName,
		"messages": apiMessages,
		"stream":   true,
	}

	if modelConfig.Stream != nil {
		reqBody["stream"] = *modelConfig.Stream
	}

	if modelConfig.MaxTokens != nil {
		reqBody["max_tokens"] = *modelConfig.MaxTokens
	}

	if modelConfig.Temperature != nil {
		reqBody["temperature"] = *modelConfig.Temperature
	}

	if modelConfig.DoSample != nil {
		reqBody["do_sample"] = *modelConfig.DoSample
	}

	if modelConfig.TopP != nil {
		reqBody["top_p"] = *modelConfig.TopP
	}

	if modelConfig.Stop != nil {
		reqBody["stop"] = *modelConfig.Stop
	}

	if modelConfig.Thinking != nil {
		if *modelConfig.Thinking {
			reqBody["thinking"] = map[string]interface{}{
				"type": "enabled",
			}
		} else {
			reqBody["thinking"] = map[string]interface{}{
				"type": "disabled",
			}
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), streamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := v.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// SSE parsing: read line by line
	if _, err := ParseSSEStream[map[string]interface{}](resp.Body, func(event map[string]interface{}) error {
		common.Info(fmt.Sprintf("%v", event))

		choices, ok := event["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			return nil
		}

		firstChoice, ok := choices[0].(map[string]interface{})
		if !ok {
			return nil
		}

		delta, ok := firstChoice["delta"].(map[string]interface{})
		if !ok {
			return nil
		}

		reasoningContent, ok := delta["reasoning_content"].(string)
		if ok && reasoningContent != "" {
			if err := sender(nil, &reasoningContent); err != nil {
				return err
			}
		}

		content, ok := delta["content"].(string)
		if ok && content != "" {
			if err := sender(&content, nil); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}

	// Send [DONE] marker for OpenAI compatibility
	endOfStream := "[DONE]"
	if err = sender(&endOfStream, nil); err != nil {
		return err
	}

	return nil
}

// Encode encodes a list of texts into embeddings
type vllmEmbeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

// Embed embeds a list of texts into embeddings
func (v *VllmModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if err := v.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := v.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := resolvedBaseURL
	if baseURL == "" {
		baseURL = resolvedBaseURL
	}
	if baseURL == "" {
		return nil, fmt.Errorf("missing base URL: please configure the local access address for vLLM (e.g., http://127.0.0.1:8000/v1)")
	}

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), v.baseModel.URLSuffix.Embedding)

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

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := v.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vLLM embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed vllmEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var embeddings []EmbeddingData
	for _, dataElem := range parsed.Data {
		var embeddingData EmbeddingData
		embeddingData.Embedding = dataElem.Embedding
		embeddingData.Index = dataElem.Index
		embeddings = append(embeddings, embeddingData)
	}

	return embeddings, nil
}

func (v *VllmModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := v.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	resolvedBaseURL, err := v.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := resolvedBaseURL
	if baseURL == "" {
		baseURL = resolvedBaseURL
	}
	if baseURL == "" {
		return nil, fmt.Errorf("missing base URL: please configure the local access address for vLLM (e.g., http://127.0.0.1:8000/v1)")
	}

	url := fmt.Sprintf("%s/%s", baseURL, v.baseModel.URLSuffix.Models)

	reqBody := map[string]interface{}{}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := v.baseModel.httpClient.Do(req)
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

	// Parse response
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

func (v *VllmModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

// CheckConnection verifies that the configured vLLM base URL is reachable
func (v *VllmModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := v.ListModels(apiConfig)
	return err
}

// vllmRerankRequest mirrors vLLM's Jina/Cohere-compatible /v1/rerank
// payload. Unlike NVIDIA NIM (which wraps each passage as {text: "..."}),
// vLLM accepts documents as a flat []string.
type vllmRerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n"`
}

// vllmRerankResponse maps the Jina-style results array. The `document`
// field is intentionally ignored — callers reconstruct text from the
// original input via Index.
type vllmRerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
}

// Rerank scores documents against the query using a vLLM rerank model
// served at /v1/rerank (stable since vLLM v0.7). Mirrors the contract
// of NvidiaModel.Rerank: defaults top_n to len(documents) so callers
// get a score per input, shrinks to RerankConfig.TopN only when set
// and smaller. Returned RerankResult entries are in the API's ranking
// order; callers that need original-input order sort by Index. The
// Authorization header is sent only when APIConfig.ApiKey is non-empty,
// matching the existing Embed/ListModels behaviour for this local
// driver.
func (v *VllmModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	if err := v.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := v.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := resolvedBaseURL
	if baseURL == "" {
		baseURL = resolvedBaseURL
	}
	if baseURL == "" {
		return nil, fmt.Errorf("missing base URL: please configure the local access address for vLLM (e.g., http://127.0.0.1:8000/v1)")
	}

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), v.baseModel.URLSuffix.Rerank)

	topN := len(documents)
	if rerankConfig != nil && rerankConfig.TopN > 0 && rerankConfig.TopN < topN {
		topN = rerankConfig.TopN
	}

	reqBody := vllmRerankRequest{
		Model:     *modelName,
		Query:     query,
		Documents: documents,
		TopN:      topN,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := v.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vLLM rerank API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed vllmRerankResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	rerankResponse := RerankResponse{Data: make([]RerankResult, 0, len(parsed.Results))}
	for _, r := range parsed.Results {
		if r.Index < 0 || r.Index >= len(documents) {
			return nil, fmt.Errorf("unexpected rerank index %d for %d inputs", r.Index, len(documents))
		}
		rerankResponse.Data = append(rerankResponse.Data, RerankResult{
			Index:          r.Index,
			RelevanceScore: r.RelevanceScore,
		})
	}

	return &rerankResponse, nil
}

// TranscribeAudio transcribe audio
func (v *VllmModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}

func (v *VllmModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", v.Name())
}

// AudioSpeech convert text to audio
func (v *VllmModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}

func (v *VllmModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", v.Name())
}

// OCRFile OCR file
func (v *VllmModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}

// ParseFile parse file
func (v *VllmModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}

func (v *VllmModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}

func (v *VllmModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}
