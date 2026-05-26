package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newDeepInfraForTest builds a DeepInfra driver pointed at the test server URL.
func newDeepInfraForTest(baseURL string) *DeepInfraModel {
	return NewDeepInfraModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:      "v1/chat/completions",
			Embedding: "v1/embeddings",
			Rerank:    "v1/inference",
		},
	)
}

// TestDeepInfraRerankHappyPath verifies request shape and score mapping.
func TestDeepInfraRerankHappyPath(t *testing.T) {
	const modelPath = "/v1/inference/Qwen/Qwen3-Reranker-4B"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != modelPath {
			t.Errorf("path=%s want %s", r.URL.Path, modelPath)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q", got)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		var body map[string]interface{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("unmarshal: %v", err)
			return
		}
		if body["query"] != "capital of France?" {
			t.Errorf("query=%v", body["query"])
		}
		docs, ok := body["documents"].([]interface{})
		if !ok || len(docs) != 2 {
			t.Errorf("documents=%v", body["documents"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"scores": []float64{0.9, 0.1},
		})
	}))
	defer srv.Close()

	apiKey := "test-key"
	model := "Qwen/Qwen3-Reranker-4B"
	resp, err := newDeepInfraForTest(srv.URL).Rerank(
		&model,
		"capital of France?",
		[]string{"Paris is the capital.", "Berlin is the capital."},
		&APIConfig{ApiKey: &apiKey},
		&RerankConfig{TopN: 1},
	)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(resp.Data) != 1 ||
		resp.Data[0].RelevanceScore != 0.9 || resp.Data[0].Index != 0 {
		t.Errorf("resp=%+v", resp.Data)
	}
}

// TestDeepInfraRerankNoTopNLimit returns every scored document when TopN is unset.
func TestDeepInfraRerankNoTopNLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"scores": []float64{0.9, 0.1},
		})
	}))
	defer srv.Close()

	apiKey := "test-key"
	model := "Qwen/Qwen3-Reranker-4B"
	resp, err := newDeepInfraForTest(srv.URL).Rerank(
		&model,
		"capital of France?",
		[]string{"Paris is the capital.", "Berlin is the capital."},
		&APIConfig{ApiKey: &apiKey},
		nil,
	)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(resp.Data) != 2 ||
		resp.Data[0].RelevanceScore != 0.9 || resp.Data[0].Index != 0 ||
		resp.Data[1].RelevanceScore != 0.1 || resp.Data[1].Index != 1 {
		t.Errorf("resp=%+v", resp.Data)
	}
}

// TestDeepInfraRerankEmptyDocuments returns an empty result without calling the API.
func TestDeepInfraRerankEmptyDocuments(t *testing.T) {
	apiKey := "test-key"
	model := "Qwen/Qwen3-Reranker-4B"
	resp, err := newDeepInfraForTest("http://unused").Rerank(&model, "q", nil, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("len=%d want 0", len(resp.Data))
	}
}

// TestDeepInfraRerankRequiresAPIKey rejects requests without an API key.
func TestDeepInfraRerankRequiresAPIKey(t *testing.T) {
	model := "Qwen/Qwen3-Reranker-4B"
	_, err := newDeepInfraForTest("http://unused").Rerank(&model, "q", []string{"a"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

// TestDeepInfraRerankRejectsScoreCountMismatch errors when scores length mismatches documents.
func TestDeepInfraRerankRejectsScoreCountMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"scores": []float64{0.5}})
	}))
	defer srv.Close()

	apiKey := "test-key"
	model := "cross-encoder/ms-marco-MiniLM-L-12-v2"
	_, err := newDeepInfraForTest(srv.URL).Rerank(
		&model, "q", []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "expected 2 scores") {
		t.Errorf("expected score-count error, got %v", err)
	}
}
