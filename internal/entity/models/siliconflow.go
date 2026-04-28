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
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"ragflow/internal/logger"
	"strings"
	"time"
)

// SiliconflowModel implements ModelDriver for Siliconflow
type SiliconflowModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client // Reusable HTTP client with connection pool
}

// NewSiliconflowModel creates a new Siliconflow model instance
func NewSiliconflowModel(baseURL map[string]string, urlSuffix URLSuffix) *SiliconflowModel {
	return &SiliconflowModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
			},
		},
	}
}

func (z *SiliconflowModel) Name() string {
	return "siliconflow"
}


// SiliconflowRerankRequest represents SILICONFLOW rerank request
type SiliconflowRerankRequest struct {
	Model           string   `json:"model"`
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	TopN            int      `json:"top_n"`
	ReturnDocuments bool     `json:"return_documents"`
	MaxChunksPerDoc int      `json:"max_chunks_per_doc"`
	OverlapTokens   int      `json:"overlap_tokens"`
}

// SiliconflowRerankResponse represents SILICONFLOW rerank response
type SiliconflowRerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
}

// Chat sends a message and returns response
func (z *SiliconflowModel) Chat(modelName, message *string, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if message == nil {
		return nil, fmt.Errorf("message is nil")
	}

	var region = "default"
	if apiConfig.Region != nil {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", z.BaseURL[region], z.URLSuffix.Chat)

	// I need to get the model type, such as qwen3 is the prefix, the model type will be qwen. glm is the prefix, the model type will be glm. such as the model name: qwen3-0.6b, the model type will be qwen3
	// the model name is glm-4.7, the model type will be glm
	modelType := strings.Split(*modelName, "-")[0]
	if modelType == "qwen" || modelType == "glm" {
		url = fmt.Sprintf("%s/%s", z.BaseURL[region], z.URLSuffix.AsyncChat)
	}

	// Build request body
	reqBody := map[string]interface{}{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "user", "content": *message},
		},
		"stream":      false,
		"temperature": 1,
	}

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

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := z.httpClient.Do(req)
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

	thinking, answer := GetThinkingAndAnswer(chatModelConfig.ModelType, &content)

	chatResponse := &ChatResponse{
		Answer:        answer,
		ReasonContent: thinking,
	}

	return chatResponse, nil
}

// ChatWithMessages sends multiple messages with roles and returns response
func (z *SiliconflowModel) ChatWithMessages(modelName string, apiKey *string, messages []Message, chatModelConfig *ChatConfig) (string, error) {
	return "", fmt.Errorf("%s, ChatWithMessages not implemented", z.Name())
}

// ChatStreamlyWithSender sends a message and streams response via sender function (best performance, no channel)
func (z *SiliconflowModel) ChatStreamlyWithSender(modelName, message *string, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	var region = "default"
	if apiConfig.Region != nil {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/chat/completions", z.BaseURL[region])

	// Build request body with streaming enabled
	reqBody := map[string]interface{}{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "user", "content": *message},
		},
		"stream":      false,
		"temperature": 1,
	}

	if chatModelConfig.Stream != nil {
		reqBody["stream"] = *chatModelConfig.Stream
	}

	if chatModelConfig.MaxTokens != nil {
		reqBody["max_tokens"] = *chatModelConfig.MaxTokens
	}

	if chatModelConfig.Temperature != nil {
		reqBody["temperature"] = *chatModelConfig.Temperature
	}

	if chatModelConfig.DoSample != nil {
		reqBody["do_sample"] = *chatModelConfig.DoSample
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

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := z.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	reserveText := ""
	thinkingPhase := false
	answerPhase := false

	// SSE parsing: read line by line
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		logger.Info(line)

		// SSE data line starts with "data:"
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		// Extract JSON after "data:"
		data := strings.TrimSpace(line[5:])

		// [DONE] marks the end of stream
		if data == "[DONE]" {
			break
		}

		// Parse the JSON event
		var event map[string]interface{}
		if err = json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		choices, ok := event["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			continue
		}

		firstChoice, ok := choices[0].(map[string]interface{})
		if !ok {
			continue
		}

		delta, ok := firstChoice["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		content, ok := delta["content"].(string)
		if ok && content != "" {
			if content == "<think>" {
				thinkingPhase = true
				continue

			} else if content == "</think>" {
				thinkingPhase = false
				answerPhase = true
				continue
			}

			if thinkingPhase {
				if err = sender(nil, &content); err != nil {
					return err
				}
				reserveText = ""
			} else if answerPhase {
				if err = sender(&content, nil); err != nil {
					return err
				}
				reserveText = ""
			} else {
				content = strings.Trim(content, "\n")
				content = strings.Trim(content, " ")
				if content != "" {
					reserveText += content
				}
			}
		}

		finishReason, ok := firstChoice["finish_reason"].(string)
		if ok && finishReason != "" {
			break
		}
	}

	if reserveText != "" {
		if err = sender(&reserveText, nil); err != nil {
			return err
		}
	}

	// Send [DONE] marker for OpenAI compatibility
	endOfStream := "[DONE]"
	if err = sender(&endOfStream, nil); err != nil {
		return err
	}

	return scanner.Err()
}

// EncodeToEmbedding encodes a list of texts into embeddings
func (s *SiliconflowModel) EncodeToEmbedding(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(s.BaseURL[region], "/"), s.URLSuffix.Embedding)

	apiKey := ""
	if apiConfig != nil && apiConfig.ApiKey != nil {
		apiKey = *apiConfig.ApiKey
	}

	embeddings := make([][]float64, len(texts))

	for i, text := range texts {
		reqBody := map[string]interface{}{
			"model": modelName,
			"input": text,
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("SILICONFLOW API error: %s, body: %s", resp.Status, string(body))
		}

		// Parse response
		var result map[string]interface{}
		if err = json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		data, ok := result["data"].([]interface{})
		if !ok || len(data) == 0 {
			return nil, fmt.Errorf("no data in response")
		}

		firstData, ok := data[0].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid data format")
		}

		embeddingSlice, ok := firstData["embedding"].([]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid embedding format")
		}

		embedding := make([]float64, len(embeddingSlice))
		for j, v := range embeddingSlice {
			switch val := v.(type) {
			case float64:
				embedding[j] = val
			case float32:
				embedding[j] = float64(val)
			default:
				return nil, fmt.Errorf("unexpected embedding value type")
			}
		}

		embeddings[i] = embedding
	}

	return embeddings, nil
}

// Encode encodes a list of texts into embeddings (convenience method)
func (s *SiliconflowModel) Encode(modelName *string, texts []string, apiConfig *APIConfig) ([][]float64, error) {
	return s.EncodeToEmbedding(modelName, texts, apiConfig, nil)
}

// EncodeQuery encodes a single query string into embedding (convenience method)
func (s *SiliconflowModel) EncodeQuery(modelName *string, query string, apiConfig *APIConfig) ([]float64, error) {
	embeddings, err := s.Encode(modelName, []string{query}, apiConfig)
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

func (z *SiliconflowModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	var region = "default"
	if apiConfig.Region != nil {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", z.BaseURL[region], z.URLSuffix.Models)

	// Build request body
	reqBody := map[string]interface{}{}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("GET", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := z.httpClient.Do(req)
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
	var modelList DSModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var models []string
	for _, model := range modelList.Models {
		modelName := model.ID
		if model.OwnedBy != "" {
			modelName = model.ID + "@" + model.OwnedBy
		}
		models = append(models, modelName)
	}

	return models, nil
}

func (z *SiliconflowModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *SiliconflowModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := z.ListModels(apiConfig)
	if err != nil {
		return err
	}
	return nil
}

// Rerank calculates similarity scores between query and texts
func (s *SiliconflowModel) Rerank(modelName *string, query string, texts []string, apiConfig *APIConfig) ([]float64, error) {
	if len(texts) == 0 {
		return []float64{}, nil
	}

	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil {
		region = *apiConfig.Region
	}

	apiKey := ""
	if apiConfig != nil && apiConfig.ApiKey != nil {
		apiKey = *apiConfig.ApiKey
	}

	reqBody := SiliconflowRerankRequest{
		Model:           *modelName,
		Query:           query,
		Documents:       texts,
		TopN:            len(texts),
		ReturnDocuments: false,
		MaxChunksPerDoc: 1024,
		OverlapTokens:   80,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(s.BaseURL[region], "/"), s.URLSuffix.Rerank)

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SiliconFlow Rerank API error: %s, body: %s", resp.Status, string(body))
	}

	body, _ := io.ReadAll(resp.Body)

	var rerankResp SiliconflowRerankResponse
	if err := json.Unmarshal(body, &rerankResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	scores := make([]float64, len(texts))
	for _, result := range rerankResp.Results {
		if result.Index >= 0 && result.Index < len(texts) {
			scores[result.Index] = result.RelevanceScore
		}
	}

	return scores, nil
}
