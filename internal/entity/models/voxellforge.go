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

// VoxellForgeModel drives Voxell Forge, a hosted text-embedding API that
// speaks the OpenAI embeddings wire format. The embedded
// OpenAIAPICompatibleModel covers every call except Embed, which also sends
// Forge's input_type.
type VoxellForgeModel struct {
	*OpenAIAPICompatibleModel
}

// NewVoxellForgeModel creates a Voxell Forge driver.
func NewVoxellForgeModel(baseURL map[string]string, urlSuffix URLSuffix) *VoxellForgeModel {
	return &VoxellForgeModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *VoxellForgeModel) Name() string {
	return "voxellforge"
}

// NewInstance returns a driver bound to the given base URL.
func (m *VoxellForgeModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewVoxellForgeModel(baseURL, m.baseModel.URLSuffix)
}

// ListModels reads GET /v1/models and marks every entry as an embedding
// model. Forge serves embeddings only, and its model ids carry no hint that
// name-based inference would recognise, so inference would file them as chat.
func (m *VoxellForgeModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	models, err := m.VllmModel.ListModels(ctx, apiConfig)
	if err != nil {
		return nil, err
	}
	for i := range models {
		models[i].ModelTypes = []string{modelTypeEmbedding}
	}
	return models, nil
}

// Embed calls POST /embeddings. Forge embeds queries and documents
// differently, so input_type is sent on both paths: "query" for
// encode_queries, "document" for everything indexed.
func (m *VoxellForgeModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(request.Texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	baseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), m.baseModel.URLSuffix.Embedding)

	inputType := "document"
	if request.Query {
		inputType = "query"
	}
	reqBody := map[string]interface{}{
		"model":      *modelName,
		"input":      request.Texts,
		"input_type": inputType,
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

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Voxell Forge embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed vllmEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	embeddings := make([]EmbeddingData, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		embeddings = append(embeddings, EmbeddingData{
			Embedding: item.Embedding,
			Index:     item.Index,
		})
	}

	return embeddings, nil
}
