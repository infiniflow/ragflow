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

// Providers, endpoints and clients are listed alphabetically by provider id
// (brave, exa, firecrawl, linkup, parallel, querit, serply, tavily, youcom) so
// a new provider has exactly one obvious place in each list. Tavily's endpoint
// lives with its retrieval code in chat_pipeline.go, which is why it has no
// entry here.
const (
	webSearchProviderBrave     = "brave"
	webSearchProviderExa       = "exa"
	webSearchProviderFirecrawl = "firecrawl"
	webSearchProviderLinkup    = "linkup"
	webSearchProviderParallel  = "parallel"
	webSearchProviderQuerit    = "querit"
	webSearchProviderSerply    = "serply"
	webSearchProviderTavily    = "tavily"
	webSearchProviderYouCom    = "youcom"

	braveWebSearchEndpoint = "https://api.search.brave.com/res/v1/web/search"
	exaWebSearchEndpoint   = "https://api.exa.ai/search"
	// v2 is Firecrawl's current search shape; the v1 endpoint is deprecated.
	firecrawlWebSearchEndpoint = "https://api.firecrawl.dev/v2/search"
	linkupWebSearchEndpoint    = "https://api.linkup.so/v1/search"
	parallelWebSearchEndpoint  = "https://api.parallel.ai/v1/search"
	queritWebSearchEndpoint    = "https://api.querit.ai/v1/search"
	serplyWebSearchEndpoint    = "https://api.serply.io/v1/search/"
	// You.com serves the same response shape from two endpoints. The keyless
	// one is rate-limited but needs no credentials; the keyed one lifts those
	// limits. The keyless endpoint rejects an X-API-Key header, so the endpoint
	// and the headers are always chosen together.
	youComWebSearchEndpoint        = "https://api.you.com/v1/search"
	youComKeylessWebSearchEndpoint = "https://api.you.com/v1/agents/search"
	// Identifies RAGFlow to You.com. On the keyless endpoint there is no key to
	// attribute traffic to, so this is the only signal available.
	youComWebSearchUserAgent = "RAGFlow youdotcom-integration/infiniflow-ragflow"
	// webSearchResultCount is how many hits every provider is asked for. Six
	// keeps the web block in a prompt the size of one corpus chunk set — the
	// point is to give the model something to cite, not to mirror a full SERP.
	webSearchResultCount = 6
	// exaWebSearchMaxCharacters caps the page text Exa returns per hit: Exa
	// bills for extracted content and an uncapped page would swamp the prompt.
	exaWebSearchMaxCharacters = 2000
	// webSearchMaxResponseBytes caps how much of a provider's response body is
	// read: six hits of JSON are a few KiB, so 4 MiB is far above any legitimate
	// answer while still bounding what a misbehaving provider can make us allocate.
	webSearchMaxResponseBytes = 4 << 20
)

var (
	braveWebSearchHTTPClient     = &http.Client{Timeout: 30 * time.Second}
	exaWebSearchHTTPClient       = &http.Client{Timeout: 30 * time.Second}
	firecrawlWebSearchHTTPClient = &http.Client{Timeout: 30 * time.Second}
	linkupWebSearchHTTPClient    = &http.Client{Timeout: 30 * time.Second}
	parallelWebSearchHTTPClient  = &http.Client{Timeout: 30 * time.Second}
	queritWebSearchHTTPClient    = &http.Client{Timeout: 30 * time.Second}
	serplyWebSearchHTTPClient    = &http.Client{Timeout: 30 * time.Second}
	youComWebSearchHTTPClient    = &http.Client{Timeout: 30 * time.Second}
	// Tavily is reached from two call sites (the chat pipeline and the deep
	// researcher) and they used different timeouts; keeping one client per
	// call site preserves both while allowing connection reuse — a fresh
	// http.Client per request on a hot path shares no connections at all.
	tavilyWebSearchHTTPClient    = &http.Client{Timeout: 30 * time.Second}
	tavilyDeepResearchHTTPClient = &http.Client{Timeout: 15 * time.Second}
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
	// keyOptional marks providers usable with no key. Only You.com: it serves a
	// dedicated keyless endpoint (a different path, same response shape, rate
	// limited per source IP, and it rejects an X-API-Key header — see
	// youComEndpointFor). A key moves to the keyed endpoint and lifts the limit.
	//
	// Free tier != keyless: Exa's free 1,000 requests/month still requires a key
	// on every call, so it does not get this carve-out.
	keyOptional := false
	switch provider {
	case webSearchProviderBrave:
		apiKeyField = "brave_api_key"
	case webSearchProviderExa:
		apiKeyField = "exa_api_key"
	case webSearchProviderFirecrawl:
		apiKeyField = "firecrawl_api_key"
	case webSearchProviderLinkup:
		apiKeyField = "linkup_api_key"
	case webSearchProviderParallel:
		apiKeyField = "parallel_api_key"
	case webSearchProviderQuerit:
		apiKeyField = "querit_api_key"
	case webSearchProviderSerply:
		apiKeyField = "serply_api_key"
	case webSearchProviderTavily:
		apiKeyField = "tavily_api_key"
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

// retrieveWebSearch dispatches one query to the configured provider for the chat
// pipeline.
func (s *ChatPipelineService) retrieveWebSearch(
	ctx context.Context,
	provider *webSearchProviderConfig,
	question string,
) (map[string]interface{}, error) {
	return retrieveWebSearchWithTavily(ctx, provider, question, s.tavilyRetrieve)
}

// retrieveWebSearch dispatches one query to the configured provider for the deep
// researcher.
func (dr *DeepResearcher) retrieveWebSearch(
	ctx context.Context,
	provider *webSearchProviderConfig,
	query string,
) (map[string]interface{}, error) {
	return retrieveWebSearchWithTavily(ctx, provider, query, dr.tavilyRetrieve)
}

// retrieveWebSearchWithTavily dispatches one web-search query to the configured
// provider. The chat pipeline and the deep researcher share it: the only difference
// between their former near-identical copies was which tavilyRetrieve receiver they
// called, and Tavily is the one provider implemented outside this file, so that call
// is the parameter.
func retrieveWebSearchWithTavily(
	ctx context.Context,
	provider *webSearchProviderConfig,
	question string,
	tavilyRetrieve func(context.Context, string, string) (map[string]interface{}, error),
) (map[string]interface{}, error) {
	if provider == nil {
		return nil, fmt.Errorf("web search provider is not configured")
	}
	switch provider.Provider {
	case webSearchProviderBrave:
		return retrieveBraveWebSearch(
			ctx,
			braveWebSearchHTTPClient,
			braveWebSearchEndpoint,
			provider.APIKey,
			question,
		)
	case webSearchProviderExa:
		return retrieveExaWebSearch(
			ctx,
			exaWebSearchHTTPClient,
			exaWebSearchEndpoint,
			provider.APIKey,
			question,
		)
	case webSearchProviderFirecrawl:
		return retrieveFirecrawlWebSearch(
			ctx,
			firecrawlWebSearchHTTPClient,
			firecrawlWebSearchEndpoint,
			provider.APIKey,
			question,
		)
	case webSearchProviderLinkup:
		return retrieveLinkupWebSearch(
			ctx,
			linkupWebSearchHTTPClient,
			linkupWebSearchEndpoint,
			provider.APIKey,
			question,
		)
	case webSearchProviderParallel:
		return retrieveParallelWebSearch(
			ctx,
			parallelWebSearchHTTPClient,
			parallelWebSearchEndpoint,
			provider.APIKey,
			question,
		)
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
	case webSearchProviderTavily:
		return tavilyRetrieve(ctx, provider.APIKey, question)
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

// webSearchHit is what every provider's response collapses into: the three
// fields a model can quote and cite.
type webSearchHit struct {
	Title   string
	URL     string
	Content string
}

// webSearchPayload turns web hits into the {chunks, doc_aggs} map retrieval
// expects — the same shape a corpus chunk would have had, because everything
// downstream (the prompt, the citation markers, the reference payload) only
// knows that shape. A web hit has no knowledge base, no positions and no
// embedding, and its "document" is the URL itself; idPrefix keeps chunk_id and
// doc_id scoped per provider so two providers cannot collide on one URL.
//
// Hits with no usable text are dropped: a result the model cannot quote is
// noise in the context and a citation it cannot defend.
func webSearchPayload(idPrefix string, hits []webSearchHit) map[string]interface{} {
	chunks := make([]map[string]interface{}, 0, len(hits))
	docAggs := make([]interface{}, 0, len(hits))
	for _, hit := range hits {
		content := strings.TrimSpace(hit.Content)
		if content == "" || hit.URL == "" {
			continue
		}
		// The cap lives HERE, after the filter: a provider's hit list can carry
		// unusable entries (You.com merges web and news results), and stopping the
		// collection loop on a raw count would skip the usable hits behind them.
		if len(chunks) >= webSearchResultCount {
			break
		}
		chunkID := idPrefix + "-" + hit.URL
		chunks = append(chunks, map[string]interface{}{
			"chunk_id":            chunkID,
			"content_ltks":        tokenizeText(content),
			"content_with_weight": content,
			"doc_id":              chunkID,
			"docnm_kwd":           hit.Title,
			"kb_id":               []interface{}{},
			"important_kwd":       []interface{}{},
			"image_id":            "",
			"similarity":          float64(1),
			"vector_similarity":   float64(1),
			"term_similarity":     float64(0),
			"vector":              []float64{},
			"positions":           []interface{}{},
			"url":                 hit.URL,
		})
		docAggs = append(docAggs, map[string]interface{}{
			"doc_name": hit.Title,
			"doc_id":   chunkID,
			"count":    1,
			"url":      hit.URL,
		})
	}
	return map[string]interface{}{
		"chunks":   chunks,
		"doc_aggs": docAggs,
	}
}

// webSearchRequest performs one provider call and returns the response body.
// Providers differ only in method, headers and body — the status check and the
// read are identical everywhere, and a request that never reached the provider
// must be reported the same way regardless of which one was called. Callers
// wrap the error with their own provider name.
func webSearchRequest(
	ctx context.Context,
	client *http.Client,
	method string,
	endpoint string,
	headers map[string]string,
	body io.Reader,
) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("status %d", response.StatusCode)
	}
	// Cap the body: a misbehaving or compromised provider must not be able to make
	// the server allocate an unbounded buffer. webSearchMaxResponseBytes is far above
	// the largest legitimate response (six hits, and Exa's excerpts are capped at
	// 2000 characters each), and exceeding it is an error rather than a silent
	// truncation that would surface later as an unparseable JSON body.
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, webSearchMaxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if len(responseBody) > webSearchMaxResponseBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes", webSearchMaxResponseBytes)
	}
	return responseBody, nil
}

// --- Brave Search -----------------------------------------------------------

type braveWebSearchResult struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type braveWebSearchResponse struct {
	Web struct {
		Results []braveWebSearchResult `json:"results"`
	} `json:"web"`
}

func decodeBraveWebSearchResults(responseBody []byte) ([]braveWebSearchResult, error) {
	var decoded braveWebSearchResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return nil, fmt.Errorf("brave: decode response: %w", err)
	}
	return decoded.Web.Results, nil
}

func retrieveBraveWebSearch(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	apiKey string,
	query string,
) (map[string]interface{}, error) {
	parameters := url.Values{}
	parameters.Set("q", query)
	parameters.Set("count", strconv.Itoa(webSearchResultCount))

	responseBody, err := webSearchRequest(ctx, client, http.MethodGet,
		endpoint+"?"+parameters.Encode(), map[string]string{
			"Accept":               "application/json",
			"X-Subscription-Token": apiKey,
		}, nil)
	if err != nil {
		return nil, fmt.Errorf("brave: %w", err)
	}
	results, err := decodeBraveWebSearchResults(responseBody)
	if err != nil {
		return nil, err
	}

	hits := make([]webSearchHit, 0, len(results))
	for _, result := range results {
		hits = append(hits, webSearchHit{
			Title:   result.Title,
			URL:     result.URL,
			Content: result.Description,
		})
	}
	return webSearchPayload("brave", hits), nil
}

// --- Exa --------------------------------------------------------------------

type exaWebSearchResult struct {
	Title      string   `json:"title"`
	URL        string   `json:"url"`
	Text       string   `json:"text"`
	Summary    string   `json:"summary"`
	Highlights []string `json:"highlights"`
}

func decodeExaWebSearchResults(responseBody []byte) ([]exaWebSearchResult, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return nil, fmt.Errorf("exa: decode response: %w", err)
	}
	if envelope == nil {
		return nil, fmt.Errorf("exa: response must be an object")
	}

	resultsValue, exists := envelope["results"]
	if !exists || strings.TrimSpace(string(resultsValue)) == "null" {
		return []exaWebSearchResult{}, nil
	}
	var results []exaWebSearchResult
	if err := json.Unmarshal(resultsValue, &results); err != nil {
		return nil, fmt.Errorf("exa: response field results must be an array: %w", err)
	}
	return results, nil
}

// exaContent prefers the page text Exa extracted, then its summary, then the
// matched highlights — whichever the response actually carries.
func exaContent(result exaWebSearchResult) string {
	if text := strings.TrimSpace(result.Text); text != "" {
		return text
	}
	if summary := strings.TrimSpace(result.Summary); summary != "" {
		return summary
	}
	passages := make([]string, 0, len(result.Highlights))
	for _, highlight := range result.Highlights {
		if strings.TrimSpace(highlight) != "" {
			passages = append(passages, highlight)
		}
	}
	return strings.Join(passages, "\n")
}

func retrieveExaWebSearch(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	apiKey string,
	query string,
) (map[string]interface{}, error) {
	requestBody, err := json.Marshal(map[string]interface{}{
		"query":      query,
		"numResults": webSearchResultCount,
		// Cap the extracted text: a full page would blow the web block of the
		// prompt, and the model only needs enough to quote.
		"contents": map[string]interface{}{
			"text": map[string]interface{}{"maxCharacters": exaWebSearchMaxCharacters},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("exa: marshal request: %w", err)
	}

	// Exa authenticates every request, including on its free tier — there is no
	// unauthenticated search path, so the header is unconditional here.
	responseBody, err := webSearchRequest(ctx, client, http.MethodPost, endpoint, map[string]string{
		"Accept":       "application/json",
		"Content-Type": "application/json",
		"x-api-key":    apiKey,
	}, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("exa: %w", err)
	}
	results, err := decodeExaWebSearchResults(responseBody)
	if err != nil {
		return nil, err
	}

	hits := make([]webSearchHit, 0, len(results))
	for _, result := range results {
		hits = append(hits, webSearchHit{
			Title:   result.Title,
			URL:     result.URL,
			Content: exaContent(result),
		})
	}
	return webSearchPayload("exa", hits), nil
}

// --- Firecrawl --------------------------------------------------------------

type firecrawlWebSearchResult struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Markdown    string `json:"markdown"`
}

type firecrawlWebSearchResponse struct {
	Data struct {
		Web []firecrawlWebSearchResult `json:"web"`
	} `json:"data"`
}

func decodeFirecrawlWebSearchResults(responseBody []byte) ([]firecrawlWebSearchResult, error) {
	var decoded firecrawlWebSearchResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return nil, fmt.Errorf("firecrawl: decode response: %w", err)
	}
	return decoded.Data.Web, nil
}

// firecrawlContent prefers the snippet the search already returned. Markdown is
// only present when the caller asked the endpoint to scrape the hits, which
// costs extra credits per result — this integration does not.
func firecrawlContent(result firecrawlWebSearchResult) string {
	if description := strings.TrimSpace(result.Description); description != "" {
		return description
	}
	return strings.TrimSpace(result.Markdown)
}

func retrieveFirecrawlWebSearch(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	apiKey string,
	query string,
) (map[string]interface{}, error) {
	requestBody, err := json.Marshal(map[string]interface{}{
		"query": query,
		"limit": webSearchResultCount,
	})
	if err != nil {
		return nil, fmt.Errorf("firecrawl: marshal request: %w", err)
	}

	responseBody, err := webSearchRequest(ctx, client, http.MethodPost, endpoint, map[string]string{
		"Accept":        "application/json",
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + apiKey,
	}, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("firecrawl: %w", err)
	}
	results, err := decodeFirecrawlWebSearchResults(responseBody)
	if err != nil {
		return nil, err
	}

	hits := make([]webSearchHit, 0, len(results))
	for _, result := range results {
		hits = append(hits, webSearchHit{
			Title:   result.Title,
			URL:     result.URL,
			Content: firecrawlContent(result),
		})
	}
	return webSearchPayload("firecrawl", hits), nil
}

// --- Linkup -----------------------------------------------------------------

type linkupWebSearchResult struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

func decodeLinkupWebSearchResults(responseBody []byte) ([]linkupWebSearchResult, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return nil, fmt.Errorf("linkup: decode response: %w", err)
	}
	if envelope == nil {
		return nil, fmt.Errorf("linkup: response must be an object")
	}

	resultsValue, exists := envelope["results"]
	if !exists || strings.TrimSpace(string(resultsValue)) == "null" {
		return []linkupWebSearchResult{}, nil
	}
	var results []linkupWebSearchResult
	if err := json.Unmarshal(resultsValue, &results); err != nil {
		return nil, fmt.Errorf("linkup: response field results must be an array: %w", err)
	}
	return results, nil
}

func retrieveLinkupWebSearch(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	apiKey string,
	query string,
) (map[string]interface{}, error) {
	requestBody, err := json.Marshal(map[string]interface{}{
		"q":          query,
		"depth":      "standard",
		"outputType": "searchResults",
	})
	if err != nil {
		return nil, fmt.Errorf("linkup: marshal request: %w", err)
	}

	responseBody, err := webSearchRequest(ctx, client, http.MethodPost, endpoint, map[string]string{
		"Accept":        "application/json",
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + apiKey,
	}, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("linkup: %w", err)
	}
	results, err := decodeLinkupWebSearchResults(responseBody)
	if err != nil {
		return nil, err
	}

	hits := make([]webSearchHit, 0, len(results))
	for _, result := range results {
		hits = append(hits, webSearchHit{
			Title:   result.Name,
			URL:     result.URL,
			Content: result.Content,
		})
	}
	return webSearchPayload("linkup", hits), nil
}

// --- Parallel ---------------------------------------------------------------

type parallelWebSearchResult struct {
	URL      string   `json:"url"`
	Title    string   `json:"title"`
	Excerpts []string `json:"excerpts"`
}

func decodeParallelWebSearchResults(responseBody []byte) ([]parallelWebSearchResult, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return nil, fmt.Errorf("parallel: decode response: %w", err)
	}
	if envelope == nil {
		return nil, fmt.Errorf("parallel: response must be an object")
	}

	resultsValue, exists := envelope["results"]
	if !exists || strings.TrimSpace(string(resultsValue)) == "null" {
		return []parallelWebSearchResult{}, nil
	}
	var results []parallelWebSearchResult
	if err := json.Unmarshal(resultsValue, &results); err != nil {
		return nil, fmt.Errorf("parallel: response field results must be an array: %w", err)
	}
	return results, nil
}

// parallelContent joins the excerpts Parallel extracted for a page. It returns
// nothing when a result carries no passage at all — such a hit gives the model
// a URL to cite but no text to quote.
func parallelContent(result parallelWebSearchResult) string {
	passages := make([]string, 0, len(result.Excerpts))
	for _, excerpt := range result.Excerpts {
		if strings.TrimSpace(excerpt) != "" {
			passages = append(passages, excerpt)
		}
	}
	return strings.Join(passages, "\n")
}

func retrieveParallelWebSearch(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	apiKey string,
	query string,
) (map[string]interface{}, error) {
	// search_queries carries the literal terms; objective is the same question
	// in natural language and steers which of the matched pages come back.
	requestBody, err := json.Marshal(map[string]interface{}{
		"search_queries": []string{query},
		"objective":      query,
	})
	if err != nil {
		return nil, fmt.Errorf("parallel: marshal request: %w", err)
	}

	responseBody, err := webSearchRequest(ctx, client, http.MethodPost, endpoint, map[string]string{
		"Accept":       "application/json",
		"Content-Type": "application/json",
		"x-api-key":    apiKey,
	}, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("parallel: %w", err)
	}
	results, err := decodeParallelWebSearchResults(responseBody)
	if err != nil {
		return nil, err
	}

	hits := make([]webSearchHit, 0, len(results))
	for _, result := range results {
		hits = append(hits, webSearchHit{
			Title:   result.Title,
			URL:     result.URL,
			Content: parallelContent(result),
		})
	}
	return webSearchPayload("parallel", hits), nil
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
		"count":        webSearchResultCount,
		"chunksPerDoc": 1,
	})
	if err != nil {
		return nil, fmt.Errorf("querit: marshal request: %w", err)
	}

	responseBody, err := webSearchRequest(ctx, client, http.MethodPost, endpoint, map[string]string{
		"Accept":        "application/json",
		"Authorization": "Bearer " + apiKey,
		"Content-Type":  "application/json",
	}, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("querit: %w", err)
	}
	results, err := decodeQueritWebSearchResults(responseBody)
	if err != nil {
		return nil, err
	}

	hits := make([]webSearchHit, 0, len(results))
	for _, result := range results {
		hits = append(hits, webSearchHit{
			Title:   result.Title,
			URL:     result.URL,
			Content: result.Snippet,
		})
	}
	return webSearchPayload("querit", hits), nil
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
	parameters.Set("num", strconv.Itoa(webSearchResultCount))

	// Serply sits behind Cloudflare, which rejects requests without an
	// explicit User-Agent, so always send one.
	responseBody, err := webSearchRequest(ctx, client, http.MethodGet,
		endpoint+"?"+parameters.Encode(), map[string]string{
			"Accept":     "application/json",
			"X-Api-Key":  apiKey,
			"User-Agent": "ragflow-web-search",
		}, nil)
	if err != nil {
		return nil, fmt.Errorf("serply: %w", err)
	}
	results, err := decodeSerplyWebSearchResults(responseBody)
	if err != nil {
		return nil, err
	}

	hits := make([]webSearchHit, 0, len(results))
	for _, result := range results {
		hits = append(hits, webSearchHit{
			Title:   result.Title,
			URL:     result.Link,
			Content: result.Description,
		})
	}
	return webSearchPayload("serply", hits), nil
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
	queryParams := url.Values{}
	queryParams.Set("query", query)
	queryParams.Set("count", strconv.Itoa(webSearchResultCount))

	headers := map[string]string{
		"Accept":     "application/json",
		"User-Agent": youComWebSearchUserAgent,
	}
	// The keyless endpoint rejects an X-API-Key header, so the key rides only
	// when one is configured.
	if trimmedKey := strings.TrimSpace(apiKey); trimmedKey != "" {
		headers["X-API-Key"] = trimmedKey
	}
	responseBody, err := webSearchRequest(ctx, client, http.MethodGet,
		endpoint+"?"+queryParams.Encode(), headers, nil)
	if err != nil {
		return nil, fmt.Errorf("youcom: %w", err)
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

	hits := make([]webSearchHit, 0, len(merged))
	for _, result := range merged {
		hits = append(hits, webSearchHit{
			Title:   result.Title,
			URL:     result.URL,
			Content: youComContent(result),
		})
	}
	return webSearchPayload("youcom", hits), nil
}
