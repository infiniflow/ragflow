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
	"fmt"
	"io"
	"net/http"
	"time"
)

// VolcEngine implements ModelDriver for VolcEngine
type VolcEngine struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client // Reusable HTTP client with connection pool
}

// NewVolcEngine creates a new VolcEngine model instance
func NewVolcEngine(baseURL map[string]string, urlSuffix URLSuffix) *VolcEngine {
	return &VolcEngine{
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

func (z *VolcEngine) Name() string {
	return "volcengine"
}

// Chat sends a message and returns response
func (z *VolcEngine) Chat(modelName, message *string, apiConfig *APIConfig, modelConfig *ChatConfig) (*ChatResponse, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

// ChatWithMessages sends multiple messages with roles and returns response
func (z *VolcEngine) ChatWithMessages(modelName string, apiKey *string, messages []Message, chatModelConfig *ChatConfig) (string, error) {
	return "", fmt.Errorf("%s, ChatWithMessages not implemented", z.Name())
}

// ChatStreamlyWithSender sends a message and streams response via sender function (best performance, no channel)
func (z *VolcEngine) ChatStreamlyWithSender(modelName, message *string, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

// Encode encodes a list of texts into embeddings
func (z *VolcEngine) Encode(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([][]float64, error) {
	return nil, fmt.Errorf("not implemented")
}

// Rerank calculates similarity scores between query and texts
func (z *VolcEngine) Rerank(modelName *string, query string, texts []string, apiConfig *APIConfig) ([]float64, error) {
	return nil, fmt.Errorf("%s, Rerank not implemented", z.Name())
}

func (z *VolcEngine) ListModels(apiConfig *APIConfig) ([]string, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *VolcEngine) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *VolcEngine) CheckConnection(apiConfig *APIConfig) error {
	var region = "default"
	if apiConfig.Region != nil {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", z.BaseURL[region], z.URLSuffix.Files)

	req, err := http.NewRequest("GET", url, nil)
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
