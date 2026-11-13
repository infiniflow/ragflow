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

// Package youcom implements the You.com web-search request/response contract.
// It is a dependency-light leaf package so that both the chat pipeline
// (internal/service) and the agent tool layer (internal/agent/tool) can depend
// on it without an import cycle — mirroring the internal/service/wikisearch and
// internal/service/nav pattern.
package youcom

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	// WebSearchEndpoint is the keyed endpoint; a key lifts the keyless
	// endpoint's per-IP rate limit.
	WebSearchEndpoint = "https://api.you.com/v1/search"
	// KeylessWebSearchEndpoint is rate-limited but needs no credentials, and it
	// rejects an X-API-Key header, so the endpoint and the headers are always
	// chosen together (see EndpointFor).
	KeylessWebSearchEndpoint = "https://api.you.com/v1/agents/search"
	// UserAgent identifies RAGFlow to You.com. On the keyless endpoint there is
	// no key to attribute traffic to, so this is the only signal available.
	UserAgent = "RAGFlow youdotcom-integration/infiniflow-ragflow"
)

// Hit is one normalized search result: the three fields a model can quote and
// cite.
type Hit struct {
	Title   string
	URL     string
	Content string
}

// Fetcher performs the HTTP GET for one request and returns the raw response
// body. Callers own the transport concerns (status check, response-body cap),
// so the chat pipeline's webSearchRequest and the agent tool layer's HTTPHelper
// can both drive this package.
type Fetcher func(ctx context.Context, endpoint string, headers map[string]string) ([]byte, error)

// Request carries one search query and its node-level configuration.
type Request struct {
	// Endpoint is the already-resolved search URL (see EndpointFor).
	Endpoint string
	// APIKey is only sent when non-blank, and it selects the keyed endpoint
	// via EndpointFor.
	APIKey string
	Query  string
	// Freshness restricts results by recency; "" and "any" send nothing.
	Freshness string
	// Count is the per-section result count requested from You.com.
	Count int
}

// EndpointFor picks the keyless endpoint when no key is configured. The keyless
// endpoint rejects an X-API-Key header, so callers must never send a key to it.
func EndpointFor(apiKey string) string {
	if strings.TrimSpace(apiKey) == "" {
		return KeylessWebSearchEndpoint
	}
	return WebSearchEndpoint
}

// Search issues one You.com search and returns the merged web + news hits.
func Search(ctx context.Context, fetch Fetcher, req Request) ([]Hit, error) {
	queryParams := url.Values{}
	queryParams.Set("query", req.Query)
	queryParams.Set("count", strconv.Itoa(req.Count))
	if freshness := normalizeFreshness(req.Freshness); freshness != "" {
		queryParams.Set("freshness", freshness)
	}

	headers := map[string]string{
		"Accept":     "application/json",
		"User-Agent": UserAgent,
	}
	// The keyless endpoint rejects an X-API-Key header, so the key rides only
	// when one is configured.
	if trimmedKey := strings.TrimSpace(req.APIKey); trimmedKey != "" {
		headers["X-API-Key"] = trimmedKey
	}

	responseBody, err := fetch(ctx, req.Endpoint+"?"+queryParams.Encode(), headers)
	if err != nil {
		return nil, fmt.Errorf("youcom: %w", err)
	}

	var decoded webSearchResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return nil, fmt.Errorf("youcom: decode response: %w", err)
	}

	// `count` applies per response section, so web and news together can exceed
	// it. Web results lead; the caller trims the merged list if it needs to.
	merged := make([]webSearchResult, 0, len(decoded.Results.Web)+len(decoded.Results.News))
	merged = append(merged, decoded.Results.Web...)
	merged = append(merged, decoded.Results.News...)

	hits := make([]Hit, 0, len(merged))
	for _, result := range merged {
		hits = append(hits, Hit{
			Title:   result.Title,
			URL:     result.URL,
			Content: content(result),
		})
	}
	return hits, nil
}

// normalizeFreshness lower-cases the requested window and drops the values that
// mean "no limit". Unknown values are forwarded as-is; the tool layer validates
// them against the supported set before calling here.
func normalizeFreshness(value string) string {
	freshness := strings.ToLower(strings.TrimSpace(value))
	if freshness == "" || freshness == "any" {
		return ""
	}
	return freshness
}

// content prefers the extracted page passages. News hits carry only a
// description.
func content(result webSearchResult) string {
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

type webSearchResult struct {
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Snippets    []string `json:"snippets"`
}

type webSearchResponse struct {
	Results struct {
		Web  []webSearchResult `json:"web"`
		News []webSearchResult `json:"news"`
	} `json:"results"`
}
