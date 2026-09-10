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
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	webSearchProviderTavily = "tavily"
	webSearchProviderQuerit = "querit"
	webSearchProviderSerply = "serply"
	webSearchProviderYouCom = "youcom"
	queritWebSearchEndpoint = "https://api.querit.ai/v1/search"
	serplyWebSearchEndpoint = "https://api.serply.io/v1/search/"
	// You.com serves the same response shape from two endpoints. The keyless
	// one is rate-limited but needs no credentials; the keyed one lifts those
	// limits. The keyless endpoint rejects an X-API-Key header, so the endpoint
	// and the headers are always chosen together.
	youComWebSearchEndpoint        = "https://api.you.com/v1/search"
	youComKeylessWebSearchEndpoint = "https://api.you.com/v1/agents/search"
	youComWebSearchResultCount     = 6
	// Identifies RAGFlow to You.com. On the keyless endpoint there is no key to
	// attribute traffic to, so this is the only signal available.
	youComWebSearchUserAgent = "RAGFlow youdotcom-integration/infiniflow-ragflow"
)

var (
	queritWebSearchHTTPClient = &http.Client{Timeout: 30 * time.Second}
	serplyWebSearchHTTPClient = &http.Client{Timeout: 30 * time.Second}
	youComWebSearchHTTPClient = &http.Client{Timeout: 30 * time.Second}
)

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
	// You.com is usable with no credentials at all; every other provider here
	// requires a key before it can be selected.
	keyOptional := false
	switch provider {
	case webSearchProviderTavily:
		apiKeyField = "tavily_api_key"
	case webSearchProviderQuerit:
		apiKeyField = "querit_api_key"
	case webSearchProviderSerply:
		apiKeyField = "serply_api_key"
	case webSearchProviderYouCom:
		apiKeyField = "youcom_api_key"
		keyOptional = true
	default:
		return nil
	}

	apiKey, _ := promptConfig[apiKeyField].(string)
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" && !keyOptional {
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
	case webSearchProviderSerply:
		return retrieveSerplyWebSearch(
			ctx,
			serplyWebSearchHTTPClient,
			serplyWebSearchEndpoint,
			provider.APIKey,
			question,
		)
	case webSearchProviderYouCom:
		return retrieveYouComWebSearch(
			ctx,
			youComWebSearchHTTPClient,
			youComEndpointFor(provider.APIKey),
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
	case webSearchProviderSerply:
		return retrieveSerplyWebSearch(
			ctx,
			serplyWebSearchHTTPClient,
			serplyWebSearchEndpoint,
			provider.APIKey,
			query,
		)
	case webSearchProviderYouCom:
		return retrieveYouComWebSearch(
			ctx,
			youComWebSearchHTTPClient,
			youComEndpointFor(provider.APIKey),
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

type serplyWebSearchResult struct {
	Title       string `json:"title"`
	Link        string `json:"link"`
	Description string `json:"description"`
}

func retrieveSerplyWebSearch(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	apiKey string,
	query string,
) (map[string]interface{}, error) {
	parameters := url.Values{}
	parameters.Set("q", query)
	parameters.Set("num", "6")

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+parameters.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("serply: new request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Api-Key", apiKey)
	// Serply sits behind Cloudflare, which rejects requests without an
	// explicit User-Agent, so always send one.
	request.Header.Set("User-Agent", "ragflow-web-search")

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("serply: do request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("serply: status %d", response.StatusCode)
	}

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("serply: read response: %w", err)
	}
	results, err := decodeSerplyWebSearchResults(responseBody)
	if err != nil {
		return nil, err
	}

	chunks := make([]map[string]interface{}, 0, len(results))
	docAggs := make([]interface{}, 0, len(results))
	for _, result := range results {
		description := strings.TrimSpace(result.Description)
		if description == "" {
			continue
		}
		chunkID := "serply-" + result.Link
		chunks = append(chunks, map[string]interface{}{
			"chunk_id":            chunkID,
			"content_ltks":        tokenizeText(description),
			"content_with_weight": description,
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
			"url":                 result.Link,
		})
		docAggs = append(docAggs, map[string]interface{}{
			"doc_name": result.Title,
			"doc_id":   chunkID,
			"count":    1,
			"url":      result.Link,
		})
	}

	return map[string]interface{}{
		"chunks":   chunks,
		"doc_aggs": docAggs,
	}, nil
}

func decodeSerplyWebSearchResults(responseBody []byte) ([]serplyWebSearchResult, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return nil, fmt.Errorf("serply: decode response: %w", err)
	}
	if envelope == nil {
		return nil, fmt.Errorf("serply: response must be an object")
	}

	resultsValue, exists := envelope["results"]
	if !exists {
		return []serplyWebSearchResult{}, nil
	}
	if strings.TrimSpace(string(resultsValue)) == "null" {
		return nil, fmt.Errorf("serply: response field results must be an array")
	}

	var results []serplyWebSearchResult
	if err := json.Unmarshal(resultsValue, &results); err != nil {
		return nil, fmt.Errorf("serply: response field results must be an array: %w", err)
	}
	return results, nil
}

type youComWebSearchResult struct {
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Snippets    []string `json:"snippets"`
}

type youComWebSearchResponse struct {
	Results struct {
		Web  []youComWebSearchResult `json:"web"`
		News []youComWebSearchResult `json:"news"`
	} `json:"results"`
}

// youComEndpointFor picks the keyless endpoint when no key is configured. The
// keyless endpoint rejects an X-API-Key header, so callers must never send a
// key to it.
func youComEndpointFor(apiKey string) string {
	if strings.TrimSpace(apiKey) == "" {
		return youComKeylessWebSearchEndpoint
	}
	return youComWebSearchEndpoint
}

// youComContent prefers the extracted page passages. News hits carry only a
// description.
func youComContent(result youComWebSearchResult) string {
	passages := make([]string, 0, len(result.Snippets))
	for _, snippet := range result.Snippets {
		if strings.TrimSpace(snippet) != "" {
			passages = append(passages, snippet)
		}
	}
	if len(passages) > 0 {
		return strings.Join(passages, "\n")
	}
	return strings.TrimSpace(result.Description)
}

func retrieveYouComWebSearch(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	apiKey string,
	query string,
) (map[string]interface{}, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("youcom: new request: %w", err)
	}
	queryParams := request.URL.Query()
	queryParams.Set("query", query)
	queryParams.Set("count", strconv.Itoa(youComWebSearchResultCount))
	request.URL.RawQuery = queryParams.Encode()

	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", youComWebSearchUserAgent)
	if trimmedKey := strings.TrimSpace(apiKey); trimmedKey != "" {
		request.Header.Set("X-API-Key", trimmedKey)
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("youcom: do request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("youcom: status %d", response.StatusCode)
	}

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("youcom: read response: %w", err)
	}

	var decoded youComWebSearchResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return nil, fmt.Errorf("youcom: decode response: %w", err)
	}

	// `count` applies per response section, so web and news together can exceed
	// it. Web results lead; the merged list is trimmed back afterwards.
	merged := make([]youComWebSearchResult, 0, len(decoded.Results.Web)+len(decoded.Results.News))
	merged = append(merged, decoded.Results.Web...)
	merged = append(merged, decoded.Results.News...)

	chunks := make([]map[string]interface{}, 0, len(merged))
	docAggs := make([]interface{}, 0, len(merged))
	for _, result := range merged {
		if len(chunks) >= youComWebSearchResultCount {
			break
		}
		content := youComContent(result)
		if content == "" {
			continue
		}
		chunkID := "youcom-" + result.URL
		chunks = append(chunks, map[string]interface{}{
			"chunk_id":            chunkID,
			"content_ltks":        tokenizeText(content),
			"content_with_weight": content,
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
