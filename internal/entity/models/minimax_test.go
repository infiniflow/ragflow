package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMinimaxEncodeDefaultsToDBAndAppendsGroupID(t *testing.T) {
	apiKey := "test-key"
	groupID := "group-123"
	modelName := "embo-01"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Helper()
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("expected /v1/embeddings, got %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("GroupId"); got != groupID {
			t.Fatalf("expected GroupId %q, got %q", groupID, got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			t.Fatalf("expected bearer token, got %q", got)
		}

		var req minimaxEmbeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != modelName {
			t.Fatalf("expected model %q, got %q", modelName, req.Model)
		}
		if req.Type != EmbeddingTypeDB {
			t.Fatalf("expected default type %q, got %q", EmbeddingTypeDB, req.Type)
		}
		if len(req.Texts) != 2 || req.Texts[0] != "alpha" || req.Texts[1] != "beta" {
			t.Fatalf("unexpected texts: %#v", req.Texts)
		}

		if err := json.NewEncoder(w).Encode(minimaxEmbeddingResponse{
			Vectors: [][]float64{{0.1, 0.2}, {0.3, 0.4}},
			BaseResp: minimaxBaseResp{
				StatusCode: 0,
				StatusMsg:  "success",
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	driver := NewMinimaxModel(map[string]string{"default": server.URL}, URLSuffix{Embedding: "v1/embeddings"})
	vectors, err := driver.Encode(&modelName, []string{"alpha", "beta"}, &APIConfig{
		ApiKey:  &apiKey,
		GroupID: &groupID,
	}, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(vectors) != 2 || len(vectors[0]) != 2 || vectors[1][1] != 0.4 {
		t.Fatalf("unexpected vectors: %#v", vectors)
	}
}

func TestMinimaxEncodeSupportsQueryEmbeddingsWithoutGroupIDOnGlobal(t *testing.T) {
	apiKey := "test-key"
	groupID := "group-123"
	modelName := "embo-01"
	region := "global"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Helper()
		if got := r.URL.Query().Get("GroupId"); got != "" {
			t.Fatalf("expected no GroupId for global region, got %q", got)
		}

		var req minimaxEmbeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Type != EmbeddingTypeQuery {
			t.Fatalf("expected query type, got %q", req.Type)
		}

		if err := json.NewEncoder(w).Encode(minimaxEmbeddingResponse{
			Vectors: [][]float64{{0.9, 0.8}},
			BaseResp: minimaxBaseResp{
				StatusCode: 0,
				StatusMsg:  "success",
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	driver := NewMinimaxModel(map[string]string{
		"default": "https://api.minimaxi.com",
		"global":  server.URL,
	}, URLSuffix{Embedding: "v1/embeddings"})
	vectors, err := driver.Encode(&modelName, []string{"question"}, &APIConfig{
		ApiKey:  &apiKey,
		Region:  &region,
		GroupID: &groupID,
	}, &EmbeddingConfig{Type: EmbeddingTypeQuery})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 2 || vectors[0][0] != 0.9 {
		t.Fatalf("unexpected vectors: %#v", vectors)
	}
}

func TestMinimaxEncodeReturnsBaseRespErrors(t *testing.T) {
	apiKey := "test-key"
	modelName := "embo-01"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(minimaxEmbeddingResponse{
			BaseResp: minimaxBaseResp{
				StatusCode: 1004,
				StatusMsg:  "invalid request",
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	driver := NewMinimaxModel(map[string]string{"default": server.URL}, URLSuffix{Embedding: "v1/embeddings"})
	_, err := driver.Encode(&modelName, []string{"alpha"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || err.Error() != "MiniMax embeddings API error 1004: invalid request" {
		t.Fatalf("expected base_resp error, got %v", err)
	}
}
