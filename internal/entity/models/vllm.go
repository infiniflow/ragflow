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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// VllmModel implements ModelDriver for Vllm AI
type VllmModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client // Reusable HTTP client with connection pool
}

// NewVllmModel creates a new Vllm AI model instance
func NewVllmModel(baseURL map[string]string, urlSuffix URLSuffix) *VllmModel {
	return &VllmModel{
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

func (z *VllmModel) NewInstance(baseURL map[string]string) ModelDriver {
	return &VllmModel{
		BaseURL:   baseURL,
		URLSuffix: z.URLSuffix,
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

func (z *VllmModel) Name() string {
	return "vllm"
}

// Chat sends a message and returns response
func (z *VllmModel) Chat(modelName, message *string, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
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

	thinking, answer := GetThinkingAndAnswer(chatModelConfig.ModelClass, &content)

	chatResponse := &ChatResponse{
		Answer:        answer,
		ReasonContent: thinking,
	}

	return chatResponse, nil
}

// ChatWithMessages sends multiple messages with roles and returns response
func (z *VllmModel) ChatWithMessages(modelName string, apiKey *string, messages []Message, modelConfig *ChatConfig) (string, error) {
	return "", fmt.Errorf("not implemented")
}

// ChatStreamlyWithSender sends a message and streams response via sender function (best performance, no channel)
func (z *VllmModel) ChatStreamlyWithSender(modelName, message *string, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("not implemented")
}

// Encode encodes a list of texts into embeddings
func (z *VllmModel) Encode(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([][]float64, error) {
	return nil, fmt.Errorf("not implemented")
}

func (z *VllmModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (z *VllmModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

func (z *VllmModel) CheckConnection(apiConfig *APIConfig) error {
	return fmt.Errorf("no such method")
}

// Rerank calculates similarity scores between query and texts
func (z *VllmModel) Rerank(modelName *string, query string, texts []string, apiConfig *APIConfig) ([]float64, error) {
	return nil, fmt.Errorf("%s, Rerank not implemented", z.Name())
}
