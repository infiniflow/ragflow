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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"ragflow/internal/model"
	"strings"
)

// giteeEmbeddingModel implements EmbeddingModel for GiteeAI API (assumed OpenAI-compatible)
type giteeEmbeddingModel struct {
	apiKey     string
	apiBase    string
	model      string
	httpClient *http.Client
}

// GiteeEmbeddingRequest represents GiteeAI embedding request
type GiteeEmbeddingRequest struct {
	Model        string   `json:"model"`
	Input        []string `json:"input"`
	EncodeFormat string   `json:"encode_format"`
}

// GiteeEmbeddingResponse represents GiteeAI embedding response
type GiteeEmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// Encode encodes a list of texts into embeddings using GiteeAI API
func (m *giteeEmbeddingModel) Encode(texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	reqBody := GiteeEmbeddingRequest{
		Model:        m.model,
		Input:        texts,
		EncodeFormat: "float",
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", m.apiBase, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GiteeAI API error: %s, body: %s", resp.Status, string(body))
	}

	var embeddingResp GiteeEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Sort embeddings by index to ensure correct order
	embeddings := make([][]float64, len(texts))
	for _, data := range embeddingResp.Data {
		if data.Index < len(embeddings) {
			embeddings[data.Index] = data.Embedding
		}
	}

	return embeddings, nil
}

// EncodeQuery encodes a single query string into embedding
func (m *giteeEmbeddingModel) EncodeQuery(query string) ([]float64, error) {
	embeddings, err := m.Encode([]string{query})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

// init registers the GiteeAI embedding model factory
func init() {
	RegisterEmbeddingModelFactory("GiteeAI", func(apiKey, apiBase, modelName string, httpClient *http.Client) model.EmbeddingModel {
		return &giteeEmbeddingModel{
			apiKey:     apiKey,
			apiBase:    apiBase,
			model:      modelName,
			httpClient: httpClient,
		}
	})
}
