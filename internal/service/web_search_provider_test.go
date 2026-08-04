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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestResolveWebSearchProviderRequiresKeyForSelectedProvider(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]interface{}
	}{
		{name: "tavily", config: map[string]interface{}{"web_search_provider": "tavily"}},
		{name: "querit", config: map[string]interface{}{"web_search_provider": "querit"}},
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
		context.Background(),
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
