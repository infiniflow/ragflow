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
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"ragflow/internal/common"
)

type xunFeiRetrievalModel struct {
	*DummyModel
}

func newXunFeiRetrievalModel(baseURL map[string]string, suffix URLSuffix) *xunFeiRetrievalModel {
	driver := NewDummyModel(baseURL, suffix)
	driver.baseModel.httpClient = common.GetSSRFHTTPClient()
	return &xunFeiRetrievalModel{DummyModel: driver}
}

func (m *xunFeiRetrievalModel) Name() string {
	return "XunFei Retrieval"
}

func (m *xunFeiRetrievalModel) NewInstance(baseURL map[string]string) ModelDriver {
	return newXunFeiRetrievalModel(baseURL, m.baseModel.URLSuffix)
}

func (m *xunFeiRetrievalModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, usage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if len(request.Texts) == 0 {
		return []EmbeddingData{}, nil
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	endpoint, err := m.baseModel.EndpointURL(apiConfig, m.baseModel.URLSuffix.Embedding)
	if err != nil {
		return nil, err
	}
	requestBody := map[string]any{
		"model": *modelName,
		"input": request.Texts,
	}
	dimension := 0
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		dimension = embeddingConfig.Dimension
		requestBody["dimensions"] = dimension
	}
	responseBody, err := m.baseModel.doRequest(ctx, endpoint, apiConfig, requestBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	var response struct {
		ID   string `json:"id"`
		Data []struct {
			Index     *int      `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Usage *TokenUsage `json:"usage"`
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, fmt.Errorf("invalid XunFei Retrieval embedding response: %w", err)
	}
	if len(response.Data) != len(request.Texts) {
		return nil, fmt.Errorf("XunFei Retrieval returned %d embeddings for %d inputs", len(response.Data), len(request.Texts))
	}
	if dimension == 0 {
		dimension = len(response.Data[0].Embedding)
	}
	result := make([]EmbeddingData, len(request.Texts))
	for _, item := range response.Data {
		if item.Index == nil || *item.Index < 0 || *item.Index >= len(result) {
			return nil, fmt.Errorf("invalid XunFei Retrieval embedding index for %d inputs", len(result))
		}
		index := *item.Index
		if result[index].Embedding != nil {
			return nil, fmt.Errorf("duplicate XunFei Retrieval embedding index %d", index)
		}
		if len(item.Embedding) == 0 || len(item.Embedding) != dimension {
			return nil, fmt.Errorf("invalid XunFei Retrieval embedding length %d at index %d, expected %d", len(item.Embedding), index, dimension)
		}
		result[index] = EmbeddingData{Index: index, Embedding: item.Embedding}
	}
	recordResponseUsage(usage, response.ID, response.Usage, "embedding")
	return result, nil
}

func (m *xunFeiRetrievalModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, usage *common.ModelUsage) (*RerankResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if len(request.Documents) == 0 {
		return &RerankResponse{}, nil
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	endpoint, err := m.baseModel.EndpointURL(apiConfig, m.baseModel.URLSuffix.Rerank)
	if err != nil {
		return nil, err
	}
	responseBody, err := m.baseModel.doRequest(ctx, endpoint, apiConfig, map[string]any{
		"model":     *modelName,
		"query":     request.Query,
		"documents": request.Documents,
	}, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	var response struct {
		ID      string `json:"id"`
		Results []struct {
			Index          *int     `json:"index"`
			RelevanceScore *float64 `json:"relevance_score"`
		} `json:"results"`
		Usage *TokenUsage `json:"usage"`
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, fmt.Errorf("invalid XunFei Retrieval rerank response: %w", err)
	}
	if len(response.Results) != len(request.Documents) {
		return nil, fmt.Errorf("XunFei Retrieval returned %d rerank scores for %d documents", len(response.Results), len(request.Documents))
	}
	result := make([]RerankResult, len(request.Documents))
	seen := make([]bool, len(request.Documents))
	for _, item := range response.Results {
		if item.Index == nil || *item.Index < 0 || *item.Index >= len(result) {
			return nil, fmt.Errorf("invalid XunFei Retrieval rerank index for %d documents", len(result))
		}
		index := *item.Index
		if seen[index] {
			return nil, fmt.Errorf("duplicate XunFei Retrieval rerank index %d", index)
		}
		if item.RelevanceScore == nil {
			return nil, fmt.Errorf("missing XunFei Retrieval relevance score at index %d", index)
		}
		seen[index] = true
		result[index] = RerankResult{Index: index, RelevanceScore: *item.RelevanceScore}
	}
	// MaaS returns scores in arbitrary order and does not accept a top_n field.
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].RelevanceScore > result[j].RelevanceScore
	})
	if rerankConfig != nil && rerankConfig.TopN > 0 && rerankConfig.TopN < len(result) {
		result = result[:rerankConfig.TopN]
	}
	recordResponseUsage(usage, response.ID, response.Usage, "rerank")
	return &RerankResponse{Data: result}, nil
}
