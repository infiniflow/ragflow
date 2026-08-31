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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestResolveWebSearchProviderUsesExistingTavilyConfig(t *testing.T) {
	provider := resolveWebSearchProvider(map[string]interface{}{
		"tavily_api_key": "tvly-test",
	})

	if provider == nil {
		t.Fatal("provider is nil")
	}
	if provider.Provider != webSearchProviderTavily {
		t.Fatalf("provider = %q, want %q", provider.Provider, webSearchProviderTavily)
	}
	if provider.APIKey != "tvly-test" {
		t.Fatalf("api key = %q, want %q", provider.APIKey, "tvly-test")
	}
}

func TestResolveWebSearchProviderReturnsNilWithoutTavilyKey(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]interface{}
	}{
		{name: "nil config", config: nil},
		{name: "empty config", config: map[string]interface{}{}},
		{name: "empty key", config: map[string]interface{}{"tavily_api_key": ""}},
		{name: "whitespace key", config: map[string]interface{}{"tavily_api_key": "   "}},
		{name: "non-string key", config: map[string]interface{}{"tavily_api_key": 1}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if provider := resolveWebSearchProvider(tc.config); provider != nil {
				t.Fatalf("provider = %+v, want nil", provider)
			}
		})
	}
}

func TestResolveWebSearchProviderUsesSelectedQueritConfig(t *testing.T) {
	provider := resolveWebSearchProvider(map[string]interface{}{
		"web_search_provider": "querit",
		"querit_api_key":      "querit-test",
		"tavily_api_key":      "tvly-test",
	})

	if provider == nil {
		t.Fatal("provider is nil")
	}
	if provider.Provider != webSearchProviderQuerit {
		t.Fatalf("provider = %q, want %q", provider.Provider, webSearchProviderQuerit)
	}
	if provider.APIKey != "querit-test" {
		t.Fatalf("api key = %q, want %q", provider.APIKey, "querit-test")
	}
}

func TestResolveWebSearchProviderTrimsSelectedKey(t *testing.T) {
	provider := resolveWebSearchProvider(map[string]interface{}{
		"web_search_provider": "querit",
		"querit_api_key":      "  querit-test  ",
	})

	if provider == nil {
		t.Fatal("provider is nil")
	}
	if provider.APIKey != "querit-test" {
		t.Fatalf("api key = %q, want %q", provider.APIKey, "querit-test")
	}
}

func TestResolveWebSearchProviderUsesSelectedSerplyConfig(t *testing.T) {
	provider := resolveWebSearchProvider(map[string]interface{}{
		"web_search_provider": "serply",
		"serply_api_key":      "serply-test",
		"tavily_api_key":      "tvly-test",
	})

	if provider == nil {
		t.Fatal("provider is nil")
	}
	if provider.Provider != webSearchProviderSerply {
		t.Fatalf("provider = %q, want %q", provider.Provider, webSearchProviderSerply)
	}
	if provider.APIKey != "serply-test" {
		t.Fatalf("api key = %q, want %q", provider.APIKey, "serply-test")
	}
}

func TestResolveWebSearchProviderRequiresKeyForSelectedProvider(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]interface{}
	}{
		{name: "tavily", config: map[string]interface{}{"web_search_provider": "tavily"}},
		{name: "querit", config: map[string]interface{}{"web_search_provider": "querit"}},
		{name: "serply", config: map[string]interface{}{"web_search_provider": "serply"}},
		{
			name: "serply does not fall back to tavily",
			config: map[string]interface{}{
				"web_search_provider": "serply",
				"tavily_api_key":      "tvly-test",
			},
		},
		{
			name: "querit whitespace key",
			config: map[string]interface{}{
				"web_search_provider": "querit",
				"querit_api_key":      "   ",
			},
		},
		{
			name: "querit does not fall back to tavily",
			config: map[string]interface{}{
				"web_search_provider": "querit",
				"tavily_api_key":      "tvly-test",
			},
		},
		{
			name: "unsupported provider",
			config: map[string]interface{}{
				"web_search_provider": "unsupported",
				"querit_api_key":      "querit-test",
				"tavily_api_key":      "tvly-test",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if provider := resolveWebSearchProvider(tc.config); provider != nil {
				t.Fatalf("provider = %+v, want nil", provider)
			}
		})
	}
}

func TestRetrieveQueritWebSearchUsesChatDefaultsAndReturnsReferenceShape(t *testing.T) {
	ctx := t.Context()
	var requestBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer querit-test" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer querit-test")
		}
		if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{
			"results": {
				"result": [{
					"title": "RAGFlow",
					"url": "https://example.com/ragflow",
					"snippet": "RAGFlow is an open-source RAG engine."
				}]
			}
		}`))
	}))
	defer server.Close()

	result, err := retrieveQueritWebSearch(
		ctx,
		server.Client(),
		server.URL,
		"querit-test",
		"What is RAGFlow?",
	)
	if err != nil {
		t.Fatalf("retrieve Querit web search: %v", err)
	}

	if requestBody["query"] != "What is RAGFlow?" {
		t.Fatalf("query = %#v, want %q", requestBody["query"], "What is RAGFlow?")
	}
	if requestBody["count"] != float64(6) {
		t.Fatalf("count = %#v, want 6", requestBody["count"])
	}
	if requestBody["chunksPerDoc"] != float64(1) {
		t.Fatalf("chunksPerDoc = %#v, want 1", requestBody["chunksPerDoc"])
	}

	chunks, ok := result["chunks"].([]map[string]interface{})
	if !ok || len(chunks) != 1 {
		t.Fatalf("chunks = %#v, want one chunk", result["chunks"])
	}
	if chunks[0]["content_with_weight"] != "RAGFlow is an open-source RAG engine." {
		t.Fatalf("content = %#v", chunks[0]["content_with_weight"])
	}
	if chunks[0]["docnm_kwd"] != "RAGFlow" {
		t.Fatalf("title = %#v", chunks[0]["docnm_kwd"])
	}
	if chunks[0]["url"] != "https://example.com/ragflow" {
		t.Fatalf("url = %#v", chunks[0]["url"])
	}
	if chunks[0]["similarity"] != float64(1) {
		t.Fatalf("similarity = %#v, want 1", chunks[0]["similarity"])
	}

	aggs, ok := result["doc_aggs"].([]interface{})
	if !ok || len(aggs) != 1 {
		t.Fatalf("doc_aggs = %#v, want one aggregate", result["doc_aggs"])
	}
}

func TestDecodeQueritWebSearchResultsRejectsMalformedContainers(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "null response", body: `null`},
		{name: "null results", body: `{"results":null}`},
		{name: "array results", body: `{"results":[]}`},
		{name: "null result list", body: `{"results":{"result":null}}`},
		{name: "object result list", body: `{"results":{"result":{}}}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeQueritWebSearchResults([]byte(tc.body)); err == nil {
				t.Fatal("error is nil")
			}
		})
	}
}

func TestRetrieveSerplyWebSearchSendsHeadersAndReturnsReferenceShape(t *testing.T) {
	ctx := t.Context()
	var requestQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("X-Api-Key"); got != "serply-test" {
			t.Errorf("X-Api-Key = %q, want %q", got, "serply-test")
		}
		if got := request.Header.Get("User-Agent"); got == "" {
			t.Error("User-Agent is empty; Serply rejects requests without one")
		}
		requestQuery = request.URL.Query()
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{
			"results": [{
				"title": "RAGFlow",
				"link": "https://example.com/ragflow",
				"description": "RAGFlow is an open-source RAG engine."
			}]
		}`))
	}))
	defer server.Close()

	result, err := retrieveSerplyWebSearch(
		ctx,
		server.Client(),
		server.URL,
		"serply-test",
		"What is RAGFlow?",
	)
	if err != nil {
		t.Fatalf("retrieve Serply web search: %v", err)
	}

	if got := requestQuery.Get("q"); got != "What is RAGFlow?" {
		t.Fatalf("q = %q, want %q", got, "What is RAGFlow?")
	}
	if got := requestQuery.Get("num"); got != "6" {
		t.Fatalf("num = %q, want %q", got, "6")
	}

	chunks, ok := result["chunks"].([]map[string]interface{})
	if !ok || len(chunks) != 1 {
		t.Fatalf("chunks = %#v, want one chunk", result["chunks"])
	}
	if chunks[0]["content_with_weight"] != "RAGFlow is an open-source RAG engine." {
		t.Fatalf("content = %#v", chunks[0]["content_with_weight"])
	}
	if chunks[0]["docnm_kwd"] != "RAGFlow" {
		t.Fatalf("title = %#v", chunks[0]["docnm_kwd"])
	}
	if chunks[0]["url"] != "https://example.com/ragflow" {
		t.Fatalf("url = %#v", chunks[0]["url"])
	}
	if chunks[0]["similarity"] != float64(1) {
		t.Fatalf("similarity = %#v, want 1", chunks[0]["similarity"])
	}

	aggs, ok := result["doc_aggs"].([]interface{})
	if !ok || len(aggs) != 1 {
		t.Fatalf("doc_aggs = %#v, want one aggregate", result["doc_aggs"])
	}
}

func TestRetrieveSerplyWebSearchSkipsResultsWithoutDescription(t *testing.T) {
	ctx := t.Context()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{
			"results": [
				{"title": "No snippet", "link": "https://example.com/empty", "description": ""},
				{"title": "Blank snippet", "link": "https://example.com/blank", "description": " \t\n"},
				{"title": "RAGFlow", "link": "https://example.com/ragflow", "description": " \tAn open-source RAG engine.\n"}
			]
		}`))
	}))
	defer server.Close()

	result, err := retrieveSerplyWebSearch(ctx, server.Client(), server.URL, "serply-test", "ragflow")
	if err != nil {
		t.Fatalf("retrieve Serply web search: %v", err)
	}

	chunks, ok := result["chunks"].([]map[string]interface{})
	if !ok || len(chunks) != 1 {
		t.Fatalf("chunks = %#v, want one chunk", result["chunks"])
	}
	if chunks[0]["docnm_kwd"] != "RAGFlow" {
		t.Fatalf("title = %#v", chunks[0]["docnm_kwd"])
	}
	if chunks[0]["content_with_weight"] != "An open-source RAG engine." {
		t.Fatalf("content = %#v", chunks[0]["content_with_weight"])
	}
}

func TestDecodeSerplyWebSearchResultsRejectsMalformedContainers(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "null response", body: `null`},
		{name: "null results", body: `{"results":null}`},
		{name: "object results", body: `{"results":{}}`},
		{name: "string results", body: `{"results":"nope"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeSerplyWebSearchResults([]byte(tc.body)); err == nil {
				t.Fatal("error is nil")
			}
		})
	}
}

func TestDecodeSerplyWebSearchResultsAcceptsMissingResults(t *testing.T) {
	results, err := decodeSerplyWebSearchResults([]byte(`{"total": 0}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("results = %#v, want empty", results)
	}
}

func TestResolveWebSearchProviderSelectsYouComWithoutAKey(t *testing.T) {
	// You.com is the only provider usable with no credentials at all.
	provider := resolveWebSearchProvider(map[string]interface{}{
		"web_search_provider": "youcom",
	})

	if provider == nil {
		t.Fatal("provider is nil")
	}
	if provider.Provider != webSearchProviderYouCom {
		t.Fatalf("provider = %q, want %q", provider.Provider, webSearchProviderYouCom)
	}
	if provider.APIKey != "" {
		t.Fatalf("api key = %q, want empty", provider.APIKey)
	}
}

func TestResolveWebSearchProviderTrimsOptionalYouComKey(t *testing.T) {
	provider := resolveWebSearchProvider(map[string]interface{}{
		"web_search_provider": "youcom",
		"youcom_api_key":      "  ydc-test  ",
		"tavily_api_key":      "tvly-test",
	})

	if provider == nil {
		t.Fatal("provider is nil")
	}
	if provider.APIKey != "ydc-test" {
		t.Fatalf("api key = %q, want %q", provider.APIKey, "ydc-test")
	}
}

func TestResolveWebSearchProviderStillRequiresKeysForKeyedProviders(t *testing.T) {
	// The You.com carve-out must not relax any other provider.
	for _, provider := range []string{"tavily", "querit", "serply"} {
		t.Run(provider, func(t *testing.T) {
			if got := resolveWebSearchProvider(map[string]interface{}{
				"web_search_provider": provider,
			}); got != nil {
				t.Fatalf("provider = %+v, want nil", got)
			}
		})
	}
}

func TestYouComEndpointForPicksKeylessWithoutAKey(t *testing.T) {
	cases := []struct {
		name   string
		apiKey string
		want   string
	}{
		{name: "no key", apiKey: "", want: youComKeylessWebSearchEndpoint},
		{name: "whitespace key", apiKey: "   ", want: youComKeylessWebSearchEndpoint},
		{name: "key set", apiKey: "ydc-test", want: youComWebSearchEndpoint},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := youComEndpointFor(tc.apiKey); got != tc.want {
				t.Fatalf("endpoint = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestYouComContentDiscardsBlankSnippetsAndDescriptions(t *testing.T) {
	cases := []struct {
		name   string
		result youComWebSearchResult
		want   string
	}{
		{
			name:   "joins non-blank passages",
			result: youComWebSearchResult{Snippets: []string{"a", "   ", "b"}, Description: "ignored"},
			want:   "a\nb",
		},
		{
			name:   "falls back to the description",
			result: youComWebSearchResult{Snippets: []string{"  "}, Description: " desc "},
			want:   "desc",
		},
		{
			name:   "whitespace-only description yields nothing",
			result: youComWebSearchResult{Description: "   "},
			want:   "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := youComContent(tc.result); got != tc.want {
				t.Fatalf("content = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRetrieveYouComWebSearchSendsNoAuthHeaderWhenKeyless(t *testing.T) {
	var gotAuth string
	var gotUserAgent string
	var gotQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("X-API-Key")
		gotUserAgent = r.Header.Get("User-Agent")
		gotQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":{"web":[]}}`))
	}))
	defer server.Close()

	if _, err := retrieveYouComWebSearch(
		t.Context(),
		server.Client(),
		server.URL,
		"",
		"What is RAGFlow?",
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The keyless endpoint rejects an auth header, so none may be sent.
	if gotAuth != "" {
		t.Fatalf("X-API-Key = %q, want empty", gotAuth)
	}
	if gotUserAgent != youComWebSearchUserAgent {
		t.Fatalf("User-Agent = %q, want %q", gotUserAgent, youComWebSearchUserAgent)
	}
	if gotQuery != "What is RAGFlow?" {
		t.Fatalf("query = %q, want %q", gotQuery, "What is RAGFlow?")
	}
}

func TestRetrieveYouComWebSearchReturnsReferenceShape(t *testing.T) {
	var gotAuth string
	var gotCount string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("X-API-Key")
		gotCount = r.URL.Query().Get("count")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":{"web":[{"url":"https://example.com/ragflow","title":"RAGFlow","description":"Meta description.","snippets":["First passage.","Second passage."]}],"news":[{"url":"https://news.example.com/ragflow","title":"RAGFlow ships","description":"News description only."}]}}`))
	}))
	defer server.Close()

	result, err := retrieveYouComWebSearch(
		t.Context(),
		server.Client(),
		server.URL,
		"ydc-test",
		"What is RAGFlow?",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotAuth != "ydc-test" {
		t.Fatalf("X-API-Key = %q, want %q", gotAuth, "ydc-test")
	}
	if gotCount != "6" {
		t.Fatalf("count = %q, want %q", gotCount, "6")
	}

	chunks, ok := result["chunks"].([]map[string]interface{})
	if !ok {
		t.Fatalf("chunks type = %T, want []map[string]interface{}", result["chunks"])
	}
	if len(chunks) != 2 {
		t.Fatalf("chunks = %d, want 2", len(chunks))
	}
	// Web hits carry extracted passages; news hits fall back to the description.
	if got := chunks[0]["content_with_weight"]; got != "First passage.\nSecond passage." {
		t.Fatalf("web content = %q", got)
	}
	if got := chunks[1]["content_with_weight"]; got != "News description only." {
		t.Fatalf("news content = %q", got)
	}
	if got := chunks[0]["url"]; got != "https://example.com/ragflow" {
		t.Fatalf("url = %q", got)
	}

	docAggs, ok := result["doc_aggs"].([]interface{})
	if !ok {
		t.Fatalf("doc_aggs type = %T, want []interface{}", result["doc_aggs"])
	}
	if len(docAggs) != 2 {
		t.Fatalf("doc_aggs = %d, want 2", len(docAggs))
	}
}

func TestRetrieveYouComWebSearchSkipsBlankContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":{"web":[{"url":"https://example.com/blank","title":"Blank","description":"   "}]}}`))
	}))
	defer server.Close()

	result, err := retrieveYouComWebSearch(t.Context(), server.Client(), server.URL, "", "q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if chunks := result["chunks"].([]map[string]interface{}); len(chunks) != 0 {
		t.Fatalf("chunks = %d, want 0", len(chunks))
	}
}

func TestRetrieveYouComWebSearchCapsMergedSections(t *testing.T) {
	web := make([]map[string]string, 0, 6)
	news := make([]map[string]string, 0, 6)
	for i := 0; i < 6; i++ {
		web = append(web, map[string]string{"url": fmt.Sprintf("https://example.com/w%d", i), "description": "d"})
		news = append(news, map[string]string{"url": fmt.Sprintf("https://example.com/n%d", i), "description": "d"})
	}
	payload, err := json.Marshal(map[string]interface{}{
		"results": map[string]interface{}{"web": web, "news": news},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	result, err := retrieveYouComWebSearch(t.Context(), server.Client(), server.URL, "", "q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// `count` applies per section, so the merged list is trimmed back to 6.
	chunks := result["chunks"].([]map[string]interface{})
	if len(chunks) != youComWebSearchResultCount {
		t.Fatalf("chunks = %d, want %d", len(chunks), youComWebSearchResultCount)
	}
}

func TestRetrieveYouComWebSearchRejectsErrorStatuses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
	}))
	defer server.Close()

	if _, err := retrieveYouComWebSearch(t.Context(), server.Client(), server.URL, "", "q"); err == nil {
		t.Fatal("expected an error for a non-2xx status")
	}
}
