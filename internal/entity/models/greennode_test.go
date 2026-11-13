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
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"ragflow/internal/common"
)

func newGreenNodeForTest(baseURL string) *GreenNodeModel {
	return NewGreenNodeModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models", Embedding: "embeddings", Rerank: "rerank"},
	)
}

// newGreenNodeServer serves one request through handler after checking the
// bearer token every GreenNode call must carry, and decodes the JSON body.
func newGreenNodeServer(t *testing.T, wantPath string, handler func(body map[string]any, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	withSSRFBypass(t)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Errorf("path=%s, want %s", r.URL.Path, wantPath)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testAPIKey {
			t.Errorf("Authorization=%q", got)
		}
		body := map[string]any{}
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request: %v", err)
			}
		}
		handler(body, w)
	}))
}

func greenNodeAPIConfig() *APIConfig {
	return &APIConfig{ApiKey: &testAPIKey}
}

func TestGreenNodeNameAndFactory(t *testing.T) {
	if got := newGreenNodeForTest("http://unused").Name(); got != "greennode" {
		t.Errorf("Name()=%q", got)
	}
	driver, err := NewModelFactory().CreateModelDriver("GreenNode", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*GreenNodeModel); !ok {
		t.Fatalf("driver type=%T, want *GreenNodeModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*GreenNodeModel); !ok {
		t.Fatal("NewInstance did not return *GreenNodeModel")
	}
}

// The shipped config must resolve to the GreenNode driver and describe the
// models GreenNode serves: no speech or image-generation types, text-only GLM
// models without vision, and fixed-dimension embedding models.
func TestGreenNodeProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "greennode.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}
	provider := GetProviderManager().FindProvider("GreenNode")
	if provider == nil {
		t.Fatal("GreenNode provider not found")
	}
	if _, ok := provider.ModelDriver.(*GreenNodeModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.GreenNodeModel", provider.ModelDriver)
	}
	if got := provider.URL["default"]; got != "https://maas-llm-aiplatform-hcm.api.vngcloud.vn/v1" {
		t.Errorf("default URL=%q", got)
	}
	if provider.URLSuffix.Embedding == "" || provider.URLSuffix.Rerank == "" {
		t.Errorf("URLSuffix=%+v must declare embedding and rerank", provider.URLSuffix)
	}

	types := map[string][]string{
		"qwen/qwen3.8-27b":             {"chat", "vision"},
		"qwen/qwen3.8-max":             {"chat", "vision"},
		"z-ai/glm-5.3-flash":           {"chat", "vision"},
		"z-ai/glm-5.3":                 {"chat"},
		"z-ai/glm-5.2":                 {"chat"},
		"deepseek/deepseek-v4.1-flash": {"chat", "vision"},
		"minimax/minimax-m3":           {"chat", "vision"},
		"google/gemma-4-31b-it":        {"chat", "vision"},
		"baai/bge-m3":                  {"embedding"},
		"qwen/qwen3-embedding-8b":      {"embedding"},
		"baai/bge-reranker-v2-m3":      {"rerank"},
		"qwen/qwen3-reranker-8b":       {"rerank"},
	}
	if len(provider.Models) != len(types) {
		t.Errorf("got %d models, want %d", len(provider.Models), len(types))
	}
	for _, model := range provider.Models {
		want, ok := types[model.Name]
		if !ok {
			t.Errorf("unexpected model %q", model.Name)
			continue
		}
		if !slices.Equal(model.ModelTypes, want) {
			t.Errorf("%s: model_types=%v, want %v", model.Name, model.ModelTypes, want)
		}
		if slices.Contains(want, "embedding") {
			if len(model.Dimensions) != 1 || model.MaxDimension == nil || *model.MaxDimension != model.Dimensions[0] {
				t.Errorf("%s: dimensions=%v max_dimension=%v, want one fixed dimension", model.Name, model.Dimensions, model.MaxDimension)
			}
			if model.MaxBatchSize == nil {
				t.Errorf("%s: max_batch_size missing", model.Name)
			}
		}
	}

	gemma := GetProviderManager().FindModel(provider, "google/gemma-4-31b-it")
	if gemma == nil || gemma.MaxOutput == nil || *gemma.MaxOutput != 8192 {
		t.Errorf("gemma=%+v, want max_output 8192", gemma)
	}
}

// Reasoning models answer with reasoning_content next to content.
func TestGreenNodeChatReturnsReasoningContent(t *testing.T) {
	srv := newGreenNodeServer(t, "/chat/completions", func(body map[string]any, w http.ResponseWriter) {
		if body["model"] != "z-ai/glm-5.2" {
			t.Errorf("model=%v, want the vendor/model id unchanged", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v", body["stream"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chat-greennode",
			"choices": []map[string]any{{"message": map[string]any{
				"content":           "pong",
				"reasoning_content": "thinking",
			}}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	})
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newGreenNodeForTest(srv.URL).ChatWithMessages(t.Context(), "z-ai/glm-5.2",
		[]Message{{Role: "user", Content: "ping"}}, greenNodeAPIConfig(), nil, usage)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Errorf("Answer=%v", resp.Answer)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "thinking" {
		t.Errorf("ReasonContent=%v", resp.ReasonContent)
	}
	assertModelUsage(t, usage, 3, 5, 8)
}

func TestGreenNodeChatStreamSplitsReasoningAndContent(t *testing.T) {
	srv := newGreenNodeServer(t, "/chat/completions", func(body map[string]any, w http.ResponseWriter) {
		if body["stream"] != true {
			t.Errorf("stream=%v", body["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"reasoning_content":"step. "}}]}`+"\n\n"+
				`data: {"choices":[{"delta":{"content":"Hello"}}]}`+"\n\n"+
				`data: {"choices":[{"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n\n"+
				`data: [DONE]`+"\n\n")
	})
	defer srv.Close()

	var content, reasoning []string
	err := newGreenNodeForTest(srv.URL).ChatStreamlyWithSender(t.Context(), "z-ai/glm-5.2",
		[]Message{{Role: "user", Content: "hi"}}, greenNodeAPIConfig(), nil, nil,
		func(c *string, r *string) error {
			if r != nil && *r != "" {
				reasoning = append(reasoning, *r)
			}
			if c != nil && *c != "" && *c != "[DONE]" {
				content = append(content, *c)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if got := strings.Join(content, ""); got != "Hello world" {
		t.Errorf("content=%q", got)
	}
	if got := strings.Join(reasoning, ""); got != "step. " {
		t.Errorf("reasoning=%q", got)
	}
}

func TestGreenNodeListModels(t *testing.T) {
	srv := newGreenNodeServer(t, "/models", func(_ map[string]any, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "qwen/qwen3.8-max"},
				{"id": "baai/bge-m3"},
				{"id": "baai/bge-reranker-v2-m3"},
			},
		})
	})
	defer srv.Close()

	model := newGreenNodeForTest(srv.URL)
	list, err := model.ListModels(t.Context(), greenNodeAPIConfig())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if got := joinModelNames(list, ","); got != "qwen/qwen3.8-max,baai/bge-m3,baai/bge-reranker-v2-m3" {
		t.Errorf("models=%s", got)
	}
	wantTypes := [][]string{{"chat"}, {"embedding"}, {"rerank"}}
	for i, m := range list {
		if !slices.Equal(m.ModelTypes, wantTypes[i]) {
			t.Errorf("%s: types=%v, want %v", m.Name, m.ModelTypes, wantTypes[i])
		}
		if !m.ModelTypesInferred {
			t.Errorf("%s: ModelTypesInferred=false, want true so catalog types win in the merge", m.Name)
		}
	}
	if err := model.CheckConnection(t.Context(), greenNodeAPIConfig()); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

// A Token Plan key points the instance at another host through the per-instance
// base URL; the request must follow it instead of the catalog default.
func TestGreenNodeInstanceBaseURLOverridesDefault(t *testing.T) {
	srv := newGreenNodeServer(t, "/v1/models", func(_ map[string]any, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "glm-5.2"}}})
	})
	defer srv.Close()

	override := srv.URL + "/v1"
	list, err := newGreenNodeForTest("http://127.0.0.1:1").ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey, BaseURL: &override})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if got := joinModelNames(list, ","); got != "glm-5.2" {
		t.Errorf("models=%s", got)
	}
}

func TestGreenNodeRequiresAPIKey(t *testing.T) {
	withSSRFBypass(t)
	model := newGreenNodeForTest("http://127.0.0.1:1")
	name := "baai/bge-m3"
	empty := ""
	for _, cfg := range []*APIConfig{nil, {ApiKey: &empty}} {
		if _, err := model.ListModels(t.Context(), cfg); err == nil || !strings.Contains(err.Error(), "api key is required") {
			t.Errorf("ListModels err=%v, want api key is required", err)
		}
		if _, err := model.Embed(t.Context(), &name, EmbedRequest{Texts: []string{"x"}}, cfg, nil, nil); err == nil || !strings.Contains(err.Error(), "api key is required") {
			t.Errorf("Embed err=%v, want api key is required", err)
		}
	}
}

// bge-m3 ignores `dimensions` and qwen3-embedding-8b rejects it, so the
// request carries only model and input even when a dimension is requested.
func TestGreenNodeEmbedNeverSendsDimensions(t *testing.T) {
	srv := newGreenNodeServer(t, "/embeddings", func(body map[string]any, w http.ResponseWriter) {
		if body["model"] != "qwen/qwen3-embedding-8b" {
			t.Errorf("model=%v", body["model"])
		}
		if inputs, _ := body["input"].([]any); len(inputs) != 2 {
			t.Errorf("input=%v", body["input"])
		}
		for _, key := range []string{"dimensions", "encoding_format"} {
			if _, ok := body[key]; ok {
				t.Errorf("request carries %q: %v", key, body)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "emb-1",
			"data": []map[string]any{
				{"index": 0, "embedding": []float64{0.1, 0.2}},
				{"index": 1, "embedding": []float64{0.3, 0.4}},
			},
			"usage": map[string]any{"prompt_tokens": 7, "total_tokens": 7},
		})
	})
	defer srv.Close()

	name := "qwen/qwen3-embedding-8b"
	usage := &common.ModelUsage{}
	got, err := newGreenNodeForTest(srv.URL).Embed(t.Context(), &name, EmbedRequest{Texts: []string{"a", "b"}},
		greenNodeAPIConfig(), &EmbeddingConfig{Dimension: 1024}, usage)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != 2 || got[1].Index != 1 || !slices.Equal(got[1].Embedding, []float64{0.3, 0.4}) {
		t.Errorf("embeddings=%v", got)
	}
	assertModelUsage(t, usage, 7, 0, 7)
}

func TestGreenNodeRerankRequestAndResponse(t *testing.T) {
	srv := newGreenNodeServer(t, "/rerank", func(body map[string]any, w http.ResponseWriter) {
		if body["model"] != "baai/bge-reranker-v2-m3" || body["query"] != "q" {
			t.Errorf("body=%v", body)
		}
		if docs, _ := body["documents"].([]any); len(docs) != 3 || docs[0] != "a" {
			t.Errorf("documents=%v, want the plain strings", body["documents"])
		}
		if body["top_n"] != float64(3) {
			t.Errorf("top_n=%v, want len(documents)", body["top_n"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "rr-1",
			"results": []map[string]any{
				{"index": 2, "relevance_score": 0.9},
				{"index": 0, "relevance_score": 0.4},
			},
			"usage": map[string]any{"prompt_tokens": 11, "total_tokens": 11},
		})
	})
	defer srv.Close()

	name := "baai/bge-reranker-v2-m3"
	usage := &common.ModelUsage{}
	got, err := newGreenNodeForTest(srv.URL).Rerank(t.Context(), &name,
		RerankRequest{Query: "q", Documents: []string{"a", "b", "c"}}, greenNodeAPIConfig(), nil, usage)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(got.Data) != 2 || got.Data[0].Index != 2 || got.Data[0].RelevanceScore != 0.9 || got.Data[1].Index != 0 {
		t.Errorf("results=%v", got.Data)
	}
	assertModelUsage(t, usage, 11, 0, 11)
}

func TestGreenNodeRerankTopNAndBadIndex(t *testing.T) {
	var topN any
	srv := newGreenNodeServer(t, "/rerank", func(body map[string]any, w http.ResponseWriter) {
		topN = body["top_n"]
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{{"index": 5, "relevance_score": 1}}})
	})
	defer srv.Close()

	name := "baai/bge-reranker-v2-m3"
	_, err := newGreenNodeForTest(srv.URL).Rerank(t.Context(), &name,
		RerankRequest{Query: "q", Documents: []string{"a", "b", "c"}}, greenNodeAPIConfig(), &RerankConfig{TopN: 2}, nil)
	if topN != float64(2) {
		t.Errorf("top_n=%v, want 2", topN)
	}
	if err == nil || !strings.Contains(err.Error(), "unexpected rerank index 5") {
		t.Errorf("err=%v, want unexpected rerank index", err)
	}
}

// Qwen3-Reranker scores only inside the model's prompt template.
func TestGreenNodeRerankWrapsQwen3Template(t *testing.T) {
	const wantQuery = "<|im_start|>system\nJudge whether the Document meets the requirements based on the Query and the Instruct provided. " +
		"Note that the answer can only be \"yes\" or \"no\".<|im_end|>\n<|im_start|>user\n" +
		"<Instruct>: Given a web search query, retrieve relevant passages that answer the query\n<Query>: what is rag\n"
	const wantSuffix = "<|im_end|>\n<|im_start|>assistant\n<think>\n\n</think>\n\n"

	srv := newGreenNodeServer(t, "/rerank", func(body map[string]any, w http.ResponseWriter) {
		if body["query"] != wantQuery {
			t.Errorf("query=%q", body["query"])
		}
		docs, _ := body["documents"].([]any)
		if len(docs) != 2 || docs[0] != "<Document>: first"+wantSuffix || docs[1] != "<Document>: second"+wantSuffix {
			t.Errorf("documents=%q", docs)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{{"index": 1, "relevance_score": 0.8}}})
	})
	defer srv.Close()

	name := "qwen/qwen3-reranker-8b"
	got, err := newGreenNodeForTest(srv.URL).Rerank(t.Context(), &name,
		RerankRequest{Query: "what is rag", Documents: []string{"first", "second"}}, greenNodeAPIConfig(), nil, nil)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(got.Data) != 1 || got.Data[0].Index != 1 {
		t.Errorf("results=%v", got.Data)
	}
}

// 401 and 429 reach callers as APIStatusError on every endpoint, so failover
// and retry logic can act on the status instead of the message.
func TestGreenNodeMapsHTTPErrorsToAPIStatusError(t *testing.T) {
	withSSRFBypass(t)
	for _, status := range []int{http.StatusUnauthorized, http.StatusTooManyRequests} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"message":"nope"}}`)
		}))
		model := newGreenNodeForTest(srv.URL)
		name := "baai/bge-m3"
		calls := map[string]func() error{
			"chat": func() error {
				_, err := model.ChatWithMessages(t.Context(), "z-ai/glm-5.2", []Message{{Role: "user", Content: "x"}}, greenNodeAPIConfig(), nil, nil)
				return err
			},
			"stream": func() error {
				return model.ChatStreamlyWithSender(t.Context(), "z-ai/glm-5.2", []Message{{Role: "user", Content: "x"}}, greenNodeAPIConfig(), nil, nil,
					func(*string, *string) error { return nil })
			},
			"embed": func() error {
				_, err := model.Embed(t.Context(), &name, EmbedRequest{Texts: []string{"x"}}, greenNodeAPIConfig(), nil, nil)
				return err
			},
			"rerank": func() error {
				_, err := model.Rerank(t.Context(), &name, RerankRequest{Query: "q", Documents: []string{"x"}}, greenNodeAPIConfig(), nil, nil)
				return err
			},
		}
		for endpoint, call := range calls {
			var statusErr *APIStatusError
			if err := call(); !errors.As(err, &statusErr) || statusErr.Status != status {
				t.Errorf("%s with %d: err=%v, want APIStatusError %d", endpoint, status, err, status)
			}
		}
		if _, err := model.ListModels(t.Context(), greenNodeAPIConfig()); err == nil || !strings.Contains(err.Error(), strconv.Itoa(status)) {
			t.Errorf("ListModels with %d: err=%v, want the status in the message", status, err)
		}
		srv.Close()
	}
}
