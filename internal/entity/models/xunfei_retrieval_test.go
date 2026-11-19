// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"
)

func TestXunFeiRetrievalRequests(t *testing.T) {
	withSSRFBypass(t)
	dir, restore := setupProviderTestDir(t, "xunfei_retrieval.json")
	defer restore()
	if err := InitProviderManager(dir); err != nil {
		t.Fatal(err)
	}
	manager := GetProviderManager()
	provider := manager.FindProvider("XunFei Retrieval")
	if provider == nil || provider.ModelDriver.Name() != "XunFei Retrieval" || len(provider.Models) != 2 {
		t.Fatalf("unexpected provider configuration: %#v", provider)
	}
	if got := provider.URL["default"]; got != "https://maas-api.cn-huabei-1.xf-yun.com/v2" {
		t.Errorf("base URL = %q", got)
	}
	embeddingModel := "xop3qwen8bembedding"
	rerankModel := "xop3qwen8breranker"
	for modelName, modelType := range map[string]string{embeddingModel: "embedding", rerankModel: "rerank"} {
		model := manager.FindModel(provider, modelName)
		if model == nil || !reflect.DeepEqual(model.ModelTypes, []string{modelType}) {
			t.Fatalf("unexpected model configuration for %s: %#v", modelName, model)
		}
		if catalogModel := manager.GetModelByNameOrAlias(modelName); catalogModel == nil || catalogModel.Tokenizer == nil || *catalogModel.Tokenizer != "qwen-bpe" {
			t.Errorf("Qwen tokenizer missing for %s", modelName)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected method or request headers on %s", r.URL.Path)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		var want map[string]any
		var response string
		switch r.URL.Path {
		case "/v2/embeddings":
			want = map[string]any{"model": embeddingModel, "input": []any{"西瓜", "草莓"}, "dimensions": float64(2)}
			response = `{"id":"embed-1","data":[{"index":1,"embedding":[0.3,0.4]},{"index":0,"embedding":[0.1,0.2]}],"usage":{"prompt_tokens":11,"total_tokens":11}}`
		case "/v2/rerank":
			want = map[string]any{"model": rerankModel, "query": "夏天最好吃的水果是？", "documents": []any{"西瓜", "草莓", "荔枝"}}
			response = `{"id":"rerank-1","results":[{"index":1,"relevance_score":0.2},{"index":0,"relevance_score":0.66},{"index":2,"relevance_score":0.8}],"usage":{"prompt_tokens":17,"total_tokens":17}}`
		default:
			t.Errorf("unexpected request path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if !reflect.DeepEqual(request, want) {
			t.Errorf("request to %s = %#v, want %#v", r.URL.Path, request, want)
		}
		_, _ = io.WriteString(w, response)
	}))
	defer server.Close()

	driver := provider.ModelDriver.NewInstance(map[string]string{"default": server.URL + "/v2/"})
	apiConfig := &APIConfig{ApiKey: strPtr("test-key")}
	usage := &common.ModelUsage{}
	embeddings, err := driver.Embed(t.Context(), &embeddingModel, EmbedRequest{Texts: []string{"西瓜", "草莓"}}, apiConfig, &EmbeddingConfig{Dimension: 2}, usage)
	if err != nil {
		t.Fatal(err)
	}
	if want := []EmbeddingData{{Index: 0, Embedding: []float64{0.1, 0.2}}, {Index: 1, Embedding: []float64{0.3, 0.4}}}; !reflect.DeepEqual(embeddings, want) {
		t.Errorf("embeddings = %#v, want %#v", embeddings, want)
	}
	if usage.RequestID != "embed-1" || usage.InputTokens != 11 || usage.TotalTokens != 11 || usage.Type != "embedding" {
		t.Errorf("unexpected embedding usage: %+v", usage)
	}

	usage = &common.ModelUsage{}
	request := RerankRequest{Query: "夏天最好吃的水果是？", Documents: []string{"西瓜", "草莓", "荔枝"}}
	reranked, err := driver.Rerank(t.Context(), &rerankModel, request, apiConfig, &RerankConfig{TopN: 2}, usage)
	if err != nil {
		t.Fatal(err)
	}
	if want := []RerankResult{{Index: 2, RelevanceScore: 0.8}, {Index: 0, RelevanceScore: 0.66}}; !reflect.DeepEqual(reranked.Data, want) {
		t.Errorf("rerank = %#v, want %#v", reranked.Data, want)
	}
	if usage.RequestID != "rerank-1" || usage.InputTokens != 17 || usage.TotalTokens != 17 || usage.Type != "rerank" {
		t.Errorf("unexpected rerank usage: %+v", usage)
	}
}

func TestXunFeiRetrievalRejectsMalformedResponses(t *testing.T) {
	withSSRFBypass(t)
	tests := []struct {
		name    string
		body    string
		wantErr string
		rerank  bool
	}{
		{name: "missing embedding", body: `{"data":[{"index":0,"embedding":[0.1]}]}`, wantErr: "1 embeddings for 2 inputs"},
		{name: "missing embedding index", body: `{"data":[{"embedding":[0.1]},{"index":1,"embedding":[0.2]}]}`, wantErr: "invalid XunFei Retrieval embedding index"},
		{name: "duplicate embedding index", body: `{"data":[{"index":0,"embedding":[0.1]},{"index":0,"embedding":[0.2]}]}`, wantErr: "duplicate XunFei Retrieval embedding index"},
		{name: "embedding length mismatch", body: `{"data":[{"index":0,"embedding":[0.1]},{"index":1,"embedding":[0.2,0.3]}]}`, wantErr: "invalid XunFei Retrieval embedding length"},
		{name: "missing rerank result", body: `{"results":[{"index":0,"relevance_score":0}]}`, wantErr: "1 rerank scores for 2 documents", rerank: true},
		{name: "rerank index out of range", body: `{"results":[{"index":0,"relevance_score":0},{"index":2,"relevance_score":0.4}]}`, wantErr: "invalid XunFei Retrieval rerank index", rerank: true},
		{name: "duplicate rerank index", body: `{"results":[{"index":0,"relevance_score":0},{"index":0,"relevance_score":0.4}]}`, wantErr: "duplicate XunFei Retrieval rerank index", rerank: true},
		{name: "missing rerank score", body: `{"results":[{"index":0,"relevance_score":0},{"index":1}]}`, wantErr: "missing XunFei Retrieval relevance score", rerank: true},
		{name: "invalid rerank score", body: `{"results":[{"index":0,"relevance_score":0},{"index":1,"relevance_score":1e400}]}`, wantErr: "invalid XunFei Retrieval rerank response", rerank: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			driver := newXunFeiRetrievalModel(map[string]string{"default": server.URL + "/v2"}, URLSuffix{Embedding: "embeddings", Rerank: "rerank"})
			apiConfig := &APIConfig{ApiKey: strPtr("test-key")}
			texts := []string{"a", "b"}

			var err error
			if test.rerank {
				_, err = driver.Rerank(t.Context(), strPtr("xop3qwen8breranker"), RerankRequest{Query: "q", Documents: texts}, apiConfig, nil, nil)
			} else {
				_, err = driver.Embed(t.Context(), strPtr("xop3qwen8bembedding"), EmbedRequest{Texts: texts}, apiConfig, nil, nil)
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}
