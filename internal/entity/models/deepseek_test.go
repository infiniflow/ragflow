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
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func newDeepSeekTestModel(serverURL string) *DeepSeekModel {
	return NewDeepSeekModel(
		map[string]string{"default": serverURL},
		URLSuffix{Embedding: "embeddings"},
	)
}

func ptr[T any](v T) *T { return &v }

func TestDeepSeekEncode_HappyPathPreservesInputOrder(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotAuth   string
		gotCT     string
		gotBody   map[string]interface{}
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)

		// Return embeddings out of order on purpose to verify Encode
		// re-sorts results by `index` to match input order.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": [
				{"index": 1, "embedding": [0.4, 0.5, 0.6]},
				{"index": 0, "embedding": [0.1, 0.2, 0.3]}
			]
		}`))
	}))
	defer server.Close()

	model := newDeepSeekTestModel(server.URL)

	got, err := model.Encode(
		ptr("text-embedding-v3"),
		[]string{"hello", "world"},
		&APIConfig{ApiKey: ptr("sk-test")},
		&EmbeddingConfig{},
	)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}

	want := [][]float64{{0.1, 0.2, 0.3}, {0.4, 0.5, 0.6}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("embeddings = %v, want %v", got, want)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/embeddings" {
		t.Errorf("path = %q, want /embeddings", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer sk-test")
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotBody["model"] != "text-embedding-v3" {
		t.Errorf("body.model = %v, want text-embedding-v3", gotBody["model"])
	}
	inputs, ok := gotBody["input"].([]interface{})
	if !ok || len(inputs) != 2 || inputs[0] != "hello" || inputs[1] != "world" {
		t.Errorf("body.input = %v, want [hello world]", gotBody["input"])
	}
}

func TestDeepSeekEncode_EmptyTextsSkipsHTTPCall(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	model := newDeepSeekTestModel(server.URL)

	got, err := model.Encode(
		ptr("text-embedding-v3"),
		nil,
		&APIConfig{ApiKey: ptr("sk-test")},
		&EmbeddingConfig{},
	)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(embeddings) = %d, want 0", len(got))
	}
	if called {
		t.Errorf("HTTP server should not be called for empty input")
	}
}

func TestDeepSeekEncode_RequiresAPIKey(t *testing.T) {
	model := newDeepSeekTestModel("http://unused")

	_, err := model.Encode(
		ptr("text-embedding-v3"),
		[]string{"x"},
		&APIConfig{},
		&EmbeddingConfig{},
	)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Fatalf("expected api key error, got %v", err)
	}
}

func TestDeepSeekEncode_RequiresModelName(t *testing.T) {
	model := newDeepSeekTestModel("http://unused")

	_, err := model.Encode(
		nil,
		[]string{"x"},
		&APIConfig{ApiKey: ptr("sk-test")},
		&EmbeddingConfig{},
	)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Fatalf("expected model name error, got %v", err)
	}
}

func TestDeepSeekEncode_NonOKStatusReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "bad key"}`))
	}))
	defer server.Close()

	model := newDeepSeekTestModel(server.URL)

	_, err := model.Encode(
		ptr("text-embedding-v3"),
		[]string{"x"},
		&APIConfig{ApiKey: ptr("sk-test")},
		&EmbeddingConfig{},
	)
	if err == nil || !strings.Contains(err.Error(), "DeepSeek embeddings API error") {
		t.Fatalf("expected upstream-status error, got %v", err)
	}
}

func TestDeepSeekEncode_RejectsOutOfRangeIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Index 5 is invalid for a single-input request and must be
		// rejected — silently dropping it would leave the caller with
		// a nil vector at index 0.
		_, _ = w.Write([]byte(`{"data": [{"index": 5, "embedding": [0.1]}]}`))
	}))
	defer server.Close()

	model := newDeepSeekTestModel(server.URL)

	_, err := model.Encode(
		ptr("text-embedding-v3"),
		[]string{"x"},
		&APIConfig{ApiKey: ptr("sk-test")},
		&EmbeddingConfig{},
	)
	if err == nil || !strings.Contains(err.Error(), "unexpected embedding index") {
		t.Fatalf("expected out-of-range index error, got %v", err)
	}
}

func TestDeepSeekEncode_RejectsMissingIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server returns a vector for input 0 only; input 1 is missing.
		// Encode must fail rather than return a slice with a nil vector.
		_, _ = w.Write([]byte(`{"data": [{"index": 0, "embedding": [0.1]}]}`))
	}))
	defer server.Close()

	model := newDeepSeekTestModel(server.URL)

	_, err := model.Encode(
		ptr("text-embedding-v3"),
		[]string{"a", "b"},
		&APIConfig{ApiKey: ptr("sk-test")},
		&EmbeddingConfig{},
	)
	if err == nil || !strings.Contains(err.Error(), "missing embedding") {
		t.Fatalf("expected missing-embedding error, got %v", err)
	}
}
