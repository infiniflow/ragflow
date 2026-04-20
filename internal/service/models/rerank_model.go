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
	"net/url"
	"ragflow/internal/entity"
	"strings"
)

// openAIAPIRerankModel implements RerankModel for OpenAI-API-compatible rerank endpoints
type openAIAPIRerankModel struct {
	apiKey     string
	apiBase    string
	model      string
	httpClient *http.Client
}

// OpenAIRerankRequest represents OpenAI-API-compatible rerank request
type OpenAIRerankRequest struct {
	Model      string   `json:"model"`
	Query      string   `json:"query"`
	Documents  []string `json:"documents"`
	TopN       int      `json:"top_n"`
	ReturnDocs bool     `json:"return_documents,omitempty"`
}

// OpenAIRerankResponse represents OpenAI-API-compatible rerank response
type OpenAIRerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
}

// Similarity calculates similarity scores between query and texts
func (m *openAIAPIRerankModel) Similarity(query string, texts []string) ([]float64, error) {
	if len(texts) == 0 {
		return []float64{}, nil
	}

	reqBody := OpenAIRerankRequest{
		Model:      m.model,
		Query:      query,
		Documents:  texts,
		TopN:       len(texts),
		ReturnDocs: false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build URL - append /rerank if not already present
	reqURL := m.apiBase
	if !strings.HasSuffix(reqURL, "/rerank") {
		if !strings.HasSuffix(reqURL, "/") {
			reqURL += "/"
		}
		reqURL += "rerank"
	}

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Rerank API error: %s, body: %s", resp.Status, string(body))
	}

	var rerankResp OpenAIRerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&rerankResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Build scores array aligned with input order
	scores := make([]float64, len(texts))
	for _, result := range rerankResp.Results {
		if result.Index >= 0 && result.Index < len(texts) {
			scores[result.Index] = result.RelevanceScore
		}
	}

	return scores, nil
}

// normalizeRerankScores normalizes rerank scores to [0, 1] range
func normalizeRerankScores(scores []float64) []float64 {
	if len(scores) == 0 {
		return scores
	}

	minScore := scores[0]
	maxScore := scores[0]
	for _, s := range scores[1:] {
		if s < minScore {
			minScore = s
		}
		if s > maxScore {
			maxScore = s
		}
	}

	// Avoid division by zero
	if maxScore-minScore < 1e-3 {
		return make([]float64, len(scores))
	}

	normalized := make([]float64, len(scores))
	for i, s := range scores {
		normalized[i] = (s - minScore) / (maxScore - minScore)
	}
	return normalized
}

// init registers the OpenAI-API-compatible rerank model factory
func init() {
	RegisterRerankModelFactory("OpenAI-API-Compatible", func(apiKey, apiBase, modelName string, httpClient *http.Client) entity.RerankModel {
		return &openAIAPIRerankModel{
			apiKey:     apiKey,
			apiBase:    apiBase,
			model:      modelName,
			httpClient: httpClient,
		}
	})
}

// jinaRerankModel implements RerankModel for Jina AI rerank API
type jinaRerankModel struct {
	apiKey     string
	apiBase    string
	model      string
	httpClient *http.Client
}

// Similarity calculates similarity scores between query and texts using Jina API
func (m *jinaRerankModel) Similarity(query string, texts []string) ([]float64, error) {
	if len(texts) == 0 {
		return []float64{}, nil
	}

	// Truncate texts to 8196 chars (Jina limit)
	truncatedTexts := make([]string, len(texts))
	for i, t := range texts {
		if len(t) > 8196 {
			truncatedTexts[i] = t[:8196]
		} else {
			truncatedTexts[i] = t
		}
	}

	reqBody := map[string]interface{}{
		"model":     m.model,
		"query":     query,
		"documents": truncatedTexts,
		"top_n":     len(texts),
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	reqURL := m.apiBase
	if !strings.HasSuffix(reqURL, "/rerank") {
		if !strings.HasSuffix(reqURL, "/") {
			reqURL += "/"
		}
		reqURL += "rerank"
	}

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Jina Rerank API error: %s, body: %s", resp.Status, string(body))
	}

	var rerankResp OpenAIRerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&rerankResp); err != nil {
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

// init registers the Jina rerank model factory
func init() {
	RegisterRerankModelFactory("Jina", func(apiKey, apiBase, modelName string, httpClient *http.Client) entity.RerankModel {
		apiBaseURL := apiBase
		if apiBaseURL == "" {
			apiBaseURL = "https://api.jina.ai/v1"
		}
		model := modelName
		if model == "" {
			model = "jina-reranker-v2-base-multilingual"
		}
		return &jinaRerankModel{
			apiKey:     apiKey,
			apiBase:    apiBaseURL,
			model:      model,
			httpClient: httpClient,
		}
	})
}

// siliconFlowRerankModel implements RerankModel for SiliconFlow rerank API
type siliconFlowRerankModel struct {
	apiKey     string
	apiBase    string
	model      string
	httpClient *http.Client
}

// Similarity calculates similarity scores between query and texts using SiliconFlow API
func (m *siliconFlowRerankModel) Similarity(query string, texts []string) ([]float64, error) {
	if len(texts) == 0 {
		return []float64{}, nil
	}

	payload := map[string]interface{}{
		"model":              m.model,
		"query":              query,
		"documents":          texts,
		"top_n":              len(texts),
		"return_documents":   false,
		"max_chunks_per_doc": 1024,
		"overlap_tokens":     80,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	reqURL := m.apiBase
	if !strings.Contains(reqURL, "/rerank") {
		if !strings.HasSuffix(reqURL, "/") {
			reqURL += "/"
		}
		reqURL += "rerank"
	}

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SiliconFlow Rerank API error: %s, body: %s", resp.Status, string(body))
	}

	var rerankResp struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rerankResp); err != nil {
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

// init registers the SiliconFlow rerank model factory
func init() {
	RegisterRerankModelFactory("SILICONFLOW", func(apiKey, apiBase, modelName string, httpClient *http.Client) entity.RerankModel {
		apiBaseURL := apiBase
		if apiBaseURL == "" {
			apiBaseURL = "https://api.siliconflow.cn/v1"
		}
		if !strings.Contains(apiBaseURL, "/rerank") {
			u, _ := url.Parse(apiBaseURL)
			u.Path = strings.TrimSuffix(u.Path, "/") + "/rerank"
			apiBaseURL = u.String()
		}
		return &siliconFlowRerankModel{
			apiKey:     apiKey,
			apiBase:    apiBaseURL,
			model:      modelName,
			httpClient: httpClient,
		}
	})
}

// xinferenceRerankModel implements RerankModel for Xinference rerank API
type xinferenceRerankModel struct {
	apiKey     string
	apiBase    string
	model      string
	httpClient *http.Client
}

// Similarity calculates similarity scores between query and texts using Xinference API
func (m *xinferenceRerankModel) Similarity(query string, texts []string) ([]float64, error) {
	if len(texts) == 0 {
		return []float64{}, nil
	}

	// Truncate texts to 4096 chars (Xinference limit)
	truncatedTexts := make([]string, len(texts))
	for i, t := range texts {
		if len(t) > 4096 {
			truncatedTexts[i] = t[:4096]
		} else {
			truncatedTexts[i] = t
		}
	}

	payload := map[string]interface{}{
		"model":            m.model,
		"query":            query,
		"documents":        truncatedTexts,
		"return_documents": "true",
		"return_len":       "true",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	reqURL := m.apiBase
	if !strings.Contains(reqURL, "/v1") {
		u, _ := url.Parse(reqURL)
		u.Path = strings.TrimSuffix(u.Path, "/") + "/v1/rerank"
		reqURL = u.String()
	} else if !strings.Contains(reqURL, "/rerank") {
		reqURL = strings.TrimSuffix(reqURL, "/") + "/rerank"
	}

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if m.apiKey != "" && m.apiKey != "x" {
		req.Header.Set("Authorization", "Bearer "+m.apiKey)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Xinference Rerank API error: %s, body: %s", resp.Status, string(body))
	}

	var rerankResp struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rerankResp); err != nil {
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

// init registers the Xinference rerank model factory
func init() {
	RegisterRerankModelFactory("Xinference", func(apiKey, apiBase, modelName string, httpClient *http.Client) entity.RerankModel {
		apiBaseURL := apiBase
		if apiBaseURL == "" {
			apiBaseURL = "http://localhost:9997/v1"
		}
		return &xinferenceRerankModel{
			apiKey:     apiKey,
			apiBase:    apiBaseURL,
			model:      modelName,
			httpClient: httpClient,
		}
	})
}

// gteeRerankModel implements RerankModel for Gitee AI rerank API
type giteeRerankModel struct {
	apiKey     string
	apiBase    string
	model      string
	httpClient *http.Client
}

// Similarity calculates similarity scores between query and texts using Gitee AI API
func (m *giteeRerankModel) Similarity(query string, texts []string) ([]float64, error) {
	if len(texts) == 0 {
		return []float64{}, nil
	}

	reqBody := map[string]interface{}{
		"model":     m.model,
		"query":     query,
		"documents": texts,
		"top_n":     len(texts),
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	reqURL := m.apiBase
	if !strings.HasSuffix(reqURL, "/rerank") {
		if !strings.HasSuffix(reqURL, "/") {
			reqURL += "/"
		}
		reqURL += "rerank"
	}

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gitee Rerank API error: %s, body: %s", resp.Status, string(body))
	}

	var rerankResp struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rerankResp); err != nil {
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

// init registers the Gitee AI rerank model factory
func init() {
	RegisterRerankModelFactory("GiteeAI", func(apiKey, apiBase, modelName string, httpClient *http.Client) entity.RerankModel {
		apiBaseURL := apiBase
		if apiBaseURL == "" {
			apiBaseURL = "https://ai.gitee.com/v1"
		}
		model := modelName
		if model == "" {
			model = "GTE-Rerank"
		}
		return &giteeRerankModel{
			apiKey:     apiKey,
			apiBase:    apiBaseURL,
			model:      model,
			httpClient: httpClient,
		}
	})
}
