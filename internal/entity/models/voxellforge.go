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
	"unicode/utf16"
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

// Forge's edge answers 413 for any single input over 32,000 characters or any
// request over 256,000 characters in total, counted in UTF-16 code units. Forge
// itself reads only the first 2048 tokens of each input, so capping an input
// at 32,000 characters never drops text the model would have used.
const (
	voxellForgeMaxInputChars   = 32000
	voxellForgeMaxRequestChars = 256000
)

// voxellForgeCapInput cuts text to at most voxellForgeMaxInputChars UTF-16 code
// units, on a rune boundary. It returns the cut text and its length in code units.
func voxellForgeCapInput(text string) (string, int) {
	n := 0
	for i, r := range text {
		w := utf16.RuneLen(r)
		if w < 0 {
			w = 1
		}
		if n+w > voxellForgeMaxInputChars {
			return text[:i], n
		}
		n += w
	}
	return text, n
}

// Embed calls POST /embeddings. Forge embeds queries and documents
// differently, so input_type is sent on both paths: "query" for
// encode_queries, "document" for everything indexed. Each input is capped at
// the edge's per-input limit and the texts are split into as many requests as
// the per-request limit needs; indexes in the result refer to request.Texts.
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

	texts := make([]string, len(request.Texts))
	sizes := make([]int, len(request.Texts))
	for i, text := range request.Texts {
		texts[i], sizes[i] = voxellForgeCapInput(text)
	}

	embeddings := make([]EmbeddingData, 0, len(texts))
	for start := 0; start < len(texts); {
		end, total := start, 0
		for end < len(texts) && (end == start || total+sizes[end] <= voxellForgeMaxRequestChars) {
			total += sizes[end]
			end++
		}
		batch, err := m.embedBatch(ctx, url, *modelName, inputType, texts[start:end], apiConfig, embeddingConfig)
		if err != nil {
			return nil, err
		}
		for _, item := range batch {
			item.Index += start
			embeddings = append(embeddings, item)
		}
		start = end
	}

	return embeddings, nil
}

// embedBatch sends one POST /embeddings request for texts, which must already
// fit Forge's per-input and per-request limits.
func (m *VoxellForgeModel) embedBatch(ctx context.Context, url, modelName, inputType string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	reqBody := map[string]interface{}{
		"model":      modelName,
		"input":      texts,
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
