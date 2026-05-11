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
	"time"
)

// DeepSeekModel implements ModelDriver for DeepSeek
type DeepSeekModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client // Reusable HTTP client with connection pool
}

// NewDeepSeekModel creates a new DeepSeek model instance
func NewDeepSeekModel(baseURL map[string]string, urlSuffix URLSuffix) *DeepSeekModel {
	return &DeepSeekModel{
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

// Chat sends a message and returns response
func (z *DeepSeekModel) Chat(modelName, message *string, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

// ChatStreamlyWithSender sends a message and streams response via sender function (best performance, no channel)
func (z *DeepSeekModel) ChatStreamlyWithSender(modelName, message *string, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("not implemented")
}

// EncodeToEmbedding encodes a list of texts into embeddings
func (z *DeepSeekModel) EncodeToEmbedding(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([][]float64, error) {
	return nil, fmt.Errorf("not implemented")
}

/*
{
  "object": "list",
  "data": [
    {
      "id": "deepseek-chat",
      "object": "model",
      "owned_by": "deepseek"
    },
    {
      "id": "deepseek-reasoner",
      "object": "model",
      "owned_by": "deepseek"
    }
  ]
}
*/

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

type ModelList struct {
	Object string  `json:"object"`
	Models []Model `json:"data"`
}

func (z *DeepSeekModel) ListModels(apiConfig *APIConfig) ([]string, error) {
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
	var modelList ModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var models []string
	for _, model := range modelList.Models {
		models = append(models, model.ID)
	}

	return models, nil
}
