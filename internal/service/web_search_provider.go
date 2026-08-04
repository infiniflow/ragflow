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

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	webSearchProviderTavily = "tavily"
	webSearchProviderQuerit = "querit"
	queritWebSearchEndpoint = "https://api.querit.ai/v1/search"
)

var queritWebSearchHTTPClient = &http.Client{Timeout: 30 * time.Second}

type webSearchProviderConfig struct {
	Provider string
	APIKey   string
}

func resolveWebSearchProvider(promptConfig map[string]interface{}) *webSearchProviderConfig {
	if promptConfig == nil {
		return nil
	}

	provider := webSearchProviderTavily
	if configuredProvider, exists := promptConfig["web_search_provider"]; exists {
		var ok bool
		provider, ok = configuredProvider.(string)
		if !ok {
			return nil
		}
	}

	apiKeyField := ""
	switch provider {
	case webSearchProviderTavily:
		apiKeyField = "tavily_api_key"
	case webSearchProviderQuerit:
		apiKeyField = "querit_api_key"
	default:
		return nil
	}

	apiKey, _ := promptConfig[apiKeyField].(string)
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil
	}
	return &webSearchProviderConfig{
		Provider: provider,
		APIKey:   apiKey,
	}
}

func (s *ChatPipelineService) retrieveWebSearch(
	ctx context.Context,
	provider *webSearchProviderConfig,
	question string,
) (map[string]interface{}, error) {
	if provider == nil {
		return nil, fmt.Errorf("web search provider is not configured")
	}
	switch provider.Provider {
	case webSearchProviderTavily:
		return s.tavilyRetrieve(ctx, provider.APIKey, question)
	case webSearchProviderQuerit:
		return retrieveQueritWebSearch(
			ctx,
			queritWebSearchHTTPClient,
			queritWebSearchEndpoint,
			provider.APIKey,
			question,
		)
	default:
		return nil, fmt.Errorf("unsupported web search provider %q", provider.Provider)
	}
}

func (dr *DeepResearcher) retrieveWebSearch(
	ctx context.Context,
	provider *webSearchProviderConfig,
	query string,
) (map[string]interface{}, error) {
	if provider == nil {
		return nil, fmt.Errorf("web search provider is not configured")
	}
	switch provider.Provider {
	case webSearchProviderTavily:
		return dr.tavilyRetrieve(ctx, provider.APIKey, query)
	case webSearchProviderQuerit:
		return retrieveQueritWebSearch(
			ctx,
			queritWebSearchHTTPClient,
			queritWebSearchEndpoint,
			provider.APIKey,
			query,
		)
	default:
		return nil, fmt.Errorf("unsupported web search provider %q", provider.Provider)
	}
}

type queritWebSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

func retrieveQueritWebSearch(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	apiKey string,
	query string,
) (map[string]interface{}, error) {
	requestBody, err := json.Marshal(map[string]interface{}{
		"query":        query,
		"count":        6,
		"chunksPerDoc": 1,
	})
	if err != nil {
		return nil, fmt.Errorf("querit: marshal request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("querit: new request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("querit: do request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("querit: status %d", response.StatusCode)
	}

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("querit: read response: %w", err)
	}
	results, err := decodeQueritWebSearchResults(responseBody)
	if err != nil {
		return nil, err
	}

	chunks := make([]map[string]interface{}, 0, len(results))
	docAggs := make([]interface{}, 0, len(results))
	for _, result := range results {
		if result.Snippet == "" {
			continue
		}
		chunkID := "querit-" + result.URL
		chunks = append(chunks, map[string]interface{}{
			"chunk_id":            chunkID,
			"content_ltks":        tokenizeText(result.Snippet),
			"content_with_weight": result.Snippet,
			"doc_id":              chunkID,
			"docnm_kwd":           result.Title,
			"kb_id":               []interface{}{},
			"important_kwd":       []interface{}{},
			"image_id":            "",
			"similarity":          float64(1),
			"vector_similarity":   float64(1),
			"term_similarity":     float64(0),
			"vector":              []float64{},
			"positions":           []interface{}{},
			"url":                 result.URL,
		})
		docAggs = append(docAggs, map[string]interface{}{
			"doc_name": result.Title,
			"doc_id":   chunkID,
			"count":    1,
			"url":      result.URL,
		})
	}

	return map[string]interface{}{
		"chunks":   chunks,
		"doc_aggs": docAggs,
	}, nil
}

func decodeQueritWebSearchResults(responseBody []byte) ([]queritWebSearchResult, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return nil, fmt.Errorf("querit: decode response: %w", err)
	}
	if envelope == nil {
		return nil, fmt.Errorf("querit: response must be an object")
	}

	resultsValue, exists := envelope["results"]
	if !exists {
		return []queritWebSearchResult{}, nil
	}
	if strings.TrimSpace(string(resultsValue)) == "null" {
		return nil, fmt.Errorf("querit: response field results must be an object")
	}

	var resultsContainer map[string]json.RawMessage
	if err := json.Unmarshal(resultsValue, &resultsContainer); err != nil {
		return nil, fmt.Errorf("querit: response field results must be an object: %w", err)
	}
	resultValue, exists := resultsContainer["result"]
	if !exists {
		return []queritWebSearchResult{}, nil
	}
	if strings.TrimSpace(string(resultValue)) == "null" {
		return nil, fmt.Errorf("querit: response field results.result must be an array")
	}

	var results []queritWebSearchResult
	if err := json.Unmarshal(resultValue, &results); err != nil {
		return nil, fmt.Errorf("querit: response field results.result must be an array: %w", err)
	}
	return results, nil
}
