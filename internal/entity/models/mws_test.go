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
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/common"
)

func newMWSTestDriver(serverURL string) (*MWSModel, *APIConfig) {
	token := "token"
	baseURL := "https://gpt.mwsapis.ru/projects/test-project"
	driver := NewMWSModel(map[string]string{"default": baseURL}, URLSuffix{})
	driver.baseModel.httpClient = &http.Client{
		Transport:     mwsTestRoundTripper{serverURL: serverURL},
		CheckRedirect: rejectMWSRedirect,
	}
	return driver, &APIConfig{ApiKey: &token, BaseURL: &baseURL}
}

type mwsTestRoundTripper struct {
	serverURL string
}

func (t mwsTestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := validateMWSRequestURL(request.URL); err != nil {
		return nil, err
	}
	targetURL, err := url.Parse(t.serverURL)
	if err != nil {
		return nil, err
	}
	request = request.Clone(request.Context())
	target := *request.URL
	target.Scheme = targetURL.Scheme
	target.Host = targetURL.Host
	request.URL = &target
	request.Host = targetURL.Host
	return http.DefaultTransport.RoundTrip(request)
}

func decodeMWSRequest(t *testing.T, request *http.Request) map[string]any {
	t.Helper()
	if request.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", request.Method)
	}
	if request.Header.Get("Authorization") != "Bearer token" {
		t.Fatalf("unexpected authorization: %q", request.Header.Get("Authorization"))
	}
	if request.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected content type: %q", request.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	var payload map[string]any
	if err = json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return payload
}

func TestNormalizeMWSProjectURL(t *testing.T) {
	got, err := normalizeMWSProjectURL("https://gpt.mwsapis.ru/projects/demo/")
	if err != nil {
		t.Fatalf("normalize URL: %v", err)
	}
	if got != "https://gpt.mwsapis.ru/projects/demo" {
		t.Fatalf("unexpected normalized URL: %s", got)
	}
	if _, err = normalizeMWSProjectURL("https://gpt.mwsapis.ru/projects/demo/openai/v1"); err == nil {
		t.Fatal("expected a non-root URL to be rejected")
	}
	for _, unsafeURL := range []string{
		"http://gpt.mwsapis.ru/projects/demo",
		"https://attacker.example/projects/demo",
		"https://127.0.0.1/projects/demo",
		"https://gpt.mwsapis.ru:8443/projects/demo",
		"https://user:password@gpt.mwsapis.ru/projects/demo",
		"https://gpt.mwsapis.ru/projects/../demo",
	} {
		if _, err = normalizeMWSProjectURL(unsafeURL); err == nil {
			t.Errorf("expected unsafe URL %q to be rejected", unsafeURL)
		}
	}
}

func TestMWSHTTPClientRejectsPrivateDNSResult(t *testing.T) {
	previousLookup := common.LookupHost
	previousAllowAny := common.AllowAnyHostForTest
	common.LookupHost = func(string) ([]string, error) { return []string{"127.0.0.1"}, nil }
	common.AllowAnyHostForTest = false
	t.Cleanup(func() {
		common.LookupHost = previousLookup
		common.AllowAnyHostForTest = previousAllowAny
	})

	request, err := http.NewRequest(http.MethodGet, "https://gpt.mwsapis.ru/projects/demo/openai/v1/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = newMWSHTTPClient().Do(request)
	if err == nil || !strings.Contains(err.Error(), "non-public address") {
		t.Fatalf("expected a private DNS result to be rejected, got %v", err)
	}
}

type recordingMWSDialer struct {
	network string
	address string
}

func (d *recordingMWSDialer) DialContext(_ context.Context, network, address string) (net.Conn, error) {
	d.network = network
	d.address = address
	return nil, errors.New("recorded dial")
}

func TestMWSDialContextUsesValidatedIP(t *testing.T) {
	dialer := &recordingMWSDialer{}
	ctx := context.WithValue(context.Background(), mwsDialTargetKey{}, mwsDialTarget{
		hostname:   mwsAPIHost,
		resolvedIP: "203.0.113.10",
	})
	_, err := newMWSDialContext(dialer)(ctx, "tcp", mwsAPIHost+":443")
	if err == nil || err.Error() != "recorded dial" {
		t.Fatalf("expected the dialer to be called, got %v", err)
	}
	if dialer.network != "tcp" || dialer.address != "203.0.113.10:443" {
		t.Fatalf("dial target = %s %s, want tcp 203.0.113.10:443", dialer.network, dialer.address)
	}

	dialer.address = ""
	if _, err = newMWSDialContext(dialer)(ctx, "tcp", "attacker.example:443"); err == nil {
		t.Fatal("expected an unvalidated host to be rejected")
	}
	if dialer.address != "" {
		t.Fatalf("unvalidated host reached dialer: %s", dialer.address)
	}
}

func TestMWSHTTPClientDoesNotFollowRedirects(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		http.Redirect(response, request, "https://attacker.example/collect", http.StatusFound)
	}))
	defer server.Close()

	driver, _ := newMWSTestDriver(server.URL)
	request, err := http.NewRequest(http.MethodGet, "https://gpt.mwsapis.ru/projects/demo/openai/v1/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := driver.baseModel.httpClient.Do(request)
	if err != nil {
		t.Fatalf("request should return the redirect response without following it: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusFound)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("server request count = %d, want 1", got)
	}
}

func TestMWSListModelsUsesOpenAIEndpointAndFiltersTypes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/projects/test-project/openai/v1/models" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("unexpected authorization: %q", request.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(request.Body)
		if len(body) != 0 {
			t.Fatalf("GET models request must not have a body: %q", body)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"object":"list","data":[{"id":"bge-m3"},{"id":"bge-reranker-v2-m3"},{"id":"qwen3-32b"},{"id":"qwen-vl"}]}`))
	}))
	defer server.Close()

	driver, apiConfig := newMWSTestDriver(server.URL)
	models, err := driver.ListModels(t.Context(), apiConfig)
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	want := []ListModelResponse{
		{Name: "bge-m3", ModelTypes: []string{"embedding"}},
		{Name: "bge-reranker-v2-m3", ModelTypes: []string{"rerank"}},
		{Name: "qwen3-32b", ModelTypes: []string{"chat"}},
	}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("unexpected models: %#v", models)
	}
}

func TestMWSChatSendsOnlyDocumentedFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/projects/test-project/openai/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		payload := decodeMWSRequest(t, request)
		want := map[string]any{
			"model": "qwen3-32b",
			"messages": []any{
				map[string]any{"role": "system", "content": "Be concise."},
				map[string]any{"role": "user", "content": "Hello"},
			},
			"temperature":           0.25,
			"max_completion_tokens": float64(128),
		}
		if !reflect.DeepEqual(payload, want) {
			t.Fatalf("unexpected chat payload: %#v", payload)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"id":"chat-1","model":"qwen3-32b","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"Hi"}}],"usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}}`))
	}))
	defer server.Close()

	driver, apiConfig := newMWSTestDriver(server.URL)
	temperature := 0.25
	maxTokens := 128
	topP := 0.9
	stop := []string{"ignored"}
	response, err := driver.ChatWithMessages(
		t.Context(),
		"qwen3-32b",
		[]Message{
			{Role: "system", Content: "Be concise."},
			{Role: "user", Content: "Hello", ToolCallID: "ignored"},
		},
		apiConfig,
		&ChatConfig{Temperature: &temperature, MaxTokens: &maxTokens, TopP: &topP, Stop: &stop, Tools: map[string]any{"ignored": true}},
		nil,
	)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if response.Answer == nil || *response.Answer != "Hi" || response.Usage == nil || response.Usage.TotalTokens != 5 {
		t.Fatalf("unexpected chat response: %#v", response)
	}
}

func TestMWSChatStreamingUsesDocumentedFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/projects/test-project/openai/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		payload := decodeMWSRequest(t, request)
		want := map[string]any{
			"model":    "qwen3-32b",
			"messages": []any{map[string]any{"role": "user", "content": "Hello"}},
			"stream":   true,
			"stream_options": map[string]any{
				"include_usage": true,
			},
		}
		if !reflect.DeepEqual(payload, want) {
			t.Fatalf("unexpected streaming chat payload: %#v", payload)
		}
		response.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(response, "data: {\"model\":\"qwen3-32b\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hel\"}}]}\n\n")
		_, _ = io.WriteString(response, "data: {\"model\":\"qwen3-32b\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"lo\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":1,\"total_tokens\":5}}\n\n")
		_, _ = io.WriteString(response, "data: [DONE]\n\n")
	}))
	defer server.Close()

	driver, apiConfig := newMWSTestDriver(server.URL)
	stream := true
	config := &ChatConfig{Stream: &stream}
	var chunks []string
	err := driver.ChatStreamlyWithSender(
		t.Context(),
		"qwen3-32b",
		[]Message{{Role: "user", Content: "Hello"}},
		apiConfig,
		config,
		nil,
		func(content, _ *string) error {
			if content != nil {
				chunks = append(chunks, *content)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("stream chat: %v", err)
	}
	if !reflect.DeepEqual(chunks, []string{"Hel", "lo", "[DONE]"}) {
		t.Fatalf("unexpected stream chunks: %#v", chunks)
	}
	if config.UsageResult == nil || config.UsageResult.TotalTokens != 5 {
		t.Fatalf("unexpected stream usage: %#v", config.UsageResult)
	}
}

func TestMWSEmbedSendsOnlyDocumentedFieldsAndOrdersVectors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/projects/test-project/openai/v1/embeddings" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		payload := decodeMWSRequest(t, request)
		want := map[string]any{"model": "bge-m3", "input": []any{"first", "second"}}
		if !reflect.DeepEqual(payload, want) {
			t.Fatalf("unexpected embedding payload: %#v", payload)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"data":[{"index":1,"embedding":[0.3,0.4]},{"index":0,"embedding":[0.1,0.2]}],"usage":{"prompt_tokens":7,"total_tokens":7}}`))
	}))
	defer server.Close()

	driver, apiConfig := newMWSTestDriver(server.URL)
	modelName := "bge-m3"
	embeddings, err := driver.Embed(t.Context(), &modelName, EmbedRequest{Texts: []string{"first", "second"}}, apiConfig, &EmbeddingConfig{Dimension: 256}, nil)
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(embeddings) != 2 || embeddings[0].Index != 0 || embeddings[1].Index != 1 || embeddings[0].Embedding[0] != 0.1 || embeddings[1].Embedding[0] != 0.3 {
		t.Fatalf("unexpected embeddings: %#v", embeddings)
	}
}

func TestMWSRerankUsesCohereEndpointAndOriginalIndexOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/projects/test-project/cohere/v2/rerank" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		payload := decodeMWSRequest(t, request)
		want := map[string]any{
			"model":     "bge-reranker-v2-m3",
			"query":     "query",
			"documents": []any{"first", "second"},
			"top_n":     float64(2),
		}
		if !reflect.DeepEqual(payload, want) {
			t.Fatalf("unexpected rerank payload: %#v", payload)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"id":"score-1","results":[{"index":1,"relevance_score":0.9},{"index":0,"relevance_score":0.2}],"meta":{"tokens":{"input_tokens":9}}}`))
	}))
	defer server.Close()

	driver, apiConfig := newMWSTestDriver(server.URL)
	modelName := "bge-reranker-v2-m3"
	result, err := driver.Rerank(t.Context(), &modelName, RerankRequest{Query: "query", Documents: []string{"first", "second"}}, apiConfig, &RerankConfig{TopN: 1}, nil)
	if err != nil {
		t.Fatalf("rerank: %v", err)
	}
	if len(result.Data) != 2 || result.Data[0].Index != 0 || result.Data[0].RelevanceScore != 0.2 || result.Data[1].Index != 1 || result.Data[1].RelevanceScore != 0.9 {
		t.Fatalf("unexpected rerank result: %#v", result.Data)
	}
}

func TestMWSRejectsEmptyTokenWithoutRequest(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()

	driver, apiConfig := newMWSTestDriver(server.URL)
	empty := "  "
	apiConfig.ApiKey = &empty
	modelName := "bge-m3"
	_, err := driver.Embed(t.Context(), &modelName, EmbedRequest{Texts: []string{"text"}}, apiConfig, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Fatalf("expected an API key error, got %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("unexpected HTTP calls: %d", calls.Load())
	}
}

func TestMWSEmptyInputsDoNotSendRequests(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()

	driver, apiConfig := newMWSTestDriver(server.URL)
	modelName := "bge-m3"
	embeddings, err := driver.Embed(t.Context(), &modelName, EmbedRequest{Texts: []string{}}, apiConfig, nil, nil)
	if err != nil || len(embeddings) != 0 {
		t.Fatalf("empty embedding input: data=%#v err=%v", embeddings, err)
	}
	ranked, err := driver.Rerank(t.Context(), &modelName, RerankRequest{Query: "query", Documents: []string{}}, apiConfig, nil, nil)
	if err != nil || len(ranked.Data) != 0 {
		t.Fatalf("empty rerank input: data=%#v err=%v", ranked, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("unexpected HTTP calls: %d", calls.Load())
	}
}

func TestMWSErrorResponseIsReturned(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		http.Error(response, "MWS unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	driver, apiConfig := newMWSTestDriver(server.URL)
	modelName := "bge-m3"
	_, err := driver.Embed(t.Context(), &modelName, EmbedRequest{Texts: []string{"text"}}, apiConfig, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "status 503") || strings.Contains(err.Error(), "MWS unavailable") {
		t.Fatalf("unexpected MWS error: %v", err)
	}
	var statusErr *APIStatusError
	if !errors.As(err, &statusErr) || statusErr.Status != http.StatusServiceUnavailable {
		t.Fatalf("expected status error, got %T: %v", err, err)
	}
	if !strings.Contains(statusErr.Body, "MWS unavailable") {
		t.Fatalf("error body was not retained for internal diagnostics: %q", statusErr.Body)
	}
}

func TestMWSFactoryRegistration(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("MWS", map[string]string{"default": "https://gpt.mwsapis.ru/projects/demo"}, URLSuffix{})
	if err != nil {
		t.Fatalf("create driver: %v", err)
	}
	if driver.Name() != "MWS" {
		t.Fatalf("unexpected driver: %s", driver.Name())
	}
}
