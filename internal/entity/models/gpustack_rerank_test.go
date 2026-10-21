package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
)

// GPUStack exposes rerank at v1/rerank (gpustack/routes/rerank.py) and proxies
// it to the serving backend (vLLM, ...). The conf/models/gpustack.json url_suffix
// gained the same mapping; the constructor mirrors what the factory builds.
func newGPUStackRerankForTest(baseURL string) *GPUStackModel {
	return NewGPUStackModel(
		map[string]string{"default": baseURL},
		URLSuffix{Rerank: "v1/rerank"},
	)
}

func newGPUStackRerankServer(t *testing.T, expectAuth string, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
			return
		}
		if r.URL.Path != "/v1/rerank" {
			t.Errorf("expected path=/v1/rerank, got %s", r.URL.Path)
			return
		}
		if got := r.Header.Get("Authorization"); got != expectAuth {
			t.Errorf("expected Authorization=%q, got %q", expectAuth, got)
			return
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("expected Content-Type=application/json, got %q", got)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
			return
		}
		var body map[string]interface{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("invalid JSON body: %v\n%s", err, string(raw))
			return
		}
		handler(t, body, w)
	}))
}

func TestGPUStackRerankHappyPath(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newGPUStackRerankServer(t, "Bearer test-key", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "qwen3-reranker" {
			t.Errorf("expected model=qwen3-reranker, got %v", body["model"])
		}
		if body["query"] != "What is RAPTOR?" {
			t.Errorf("expected query=What is RAPTOR?, got %v", body["query"])
		}
		docs, ok := body["documents"].([]interface{})
		if !ok || len(docs) != 3 {
			t.Errorf("expected 3 documents, got %v", body["documents"])
			return
		}
		for i, want := range []string{"doc-zero", "doc-one", "doc-two"} {
			if docs[i] != want {
				t.Errorf("documents[%d]=%v, want %s", i, docs[i], want)
			}
		}
		if body["top_n"] != float64(3) {
			t.Errorf("expected top_n=3 (matching len(documents)), got %v", body["top_n"])
		}
		// GPUStack echoes each document's text back; the driver must ignore it.
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"model": "qwen3-reranker",
			"results": []map[string]interface{}{
				{"index": 2, "document": map[string]string{"text": "doc-two"}, "relevance_score": 0.95},
				{"index": 0, "document": map[string]string{"text": "doc-zero"}, "relevance_score": 0.42},
				{"index": 1, "document": map[string]string{"text": "doc-one"}, "relevance_score": 0.78},
			},
			"usage": map[string]interface{}{"prompt_tokens": 21, "total_tokens": 21},
		})
	})
	defer srv.Close()

	model := newGPUStackRerankForTest(srv.URL)
	apiKey := "test-key"
	modelName := "qwen3-reranker"
	usage := &common.ModelUsage{}
	resp, err := model.Rerank(
		ctx,
		&modelName,
		RerankRequest{Query: "What is RAPTOR?", Documents: []string{"doc-zero", "doc-one", "doc-two"}},
		&APIConfig{ApiKey: &apiKey},
		&RerankConfig{},
		usage,
	)
	if err != nil {
		t.Fatalf("Rerank failed: %v", err)
	}
	if len(resp.Data) != 3 {
		t.Fatalf("expected 3 results, got %d", len(resp.Data))
	}
	want := map[int]float64{0: 0.42, 1: 0.78, 2: 0.95}
	for _, r := range resp.Data {
		if got, ok := want[r.Index]; !ok || got != r.RelevanceScore {
			t.Errorf("unexpected result Index=%d RelevanceScore=%v", r.Index, r.RelevanceScore)
		}
	}
	if usage.InputTokens != 21 || usage.TotalTokens != 21 {
		t.Errorf("usage=(%d,%d), want (21,21)", usage.InputTokens, usage.TotalTokens)
	}
	if usage.Type != "rerank" {
		t.Errorf("usage.Type=%q, want rerank", usage.Type)
	}
}

func TestGPUStackRerankTopNClamp(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newGPUStackRerankServer(t, "Bearer test-key", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["top_n"] != float64(2) {
			t.Errorf("expected top_n clamp to RerankConfig.TopN=2, got %v", body["top_n"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []map[string]interface{}{}})
	})
	defer srv.Close()

	model := newGPUStackRerankForTest(srv.URL)
	apiKey := "test-key"
	modelName := "qwen3-reranker"
	if _, err := model.Rerank(
		ctx,
		&modelName,
		RerankRequest{Query: "q", Documents: []string{"a", "b", "c", "d"}},
		&APIConfig{ApiKey: &apiKey},
		&RerankConfig{TopN: 2},
		nil,
	); err != nil {
		t.Fatalf("Rerank failed: %v", err)
	}
}

func TestGPUStackRerankEmptyDocuments(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	model := newGPUStackRerankForTest("http://unused")
	apiKey := "test-key"
	modelName := "qwen3-reranker"
	resp, err := model.Rerank(ctx, &modelName, RerankRequest{Query: "q", Documents: nil}, &APIConfig{ApiKey: &apiKey}, &RerankConfig{}, nil)
	if err != nil {
		t.Fatalf("expected nil error for empty documents, got %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected empty Data, got %d entries", len(resp.Data))
	}
}

// GPUStack is a local driver (AllowEmptyAPIKey); the Authorization header must
// be omitted when no APIConfig.ApiKey is configured.
func TestGPUStackRerankWithoutAPIKey(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newGPUStackRerankServer(t, "", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{"index": 0, "relevance_score": 0.5},
			},
		})
	})
	defer srv.Close()

	model := newGPUStackRerankForTest(srv.URL)
	modelName := "qwen3-reranker"
	resp, err := model.Rerank(ctx, &modelName, RerankRequest{Query: "q", Documents: []string{"a"}}, &APIConfig{}, &RerankConfig{}, nil)
	if err != nil {
		t.Fatalf("Rerank failed without api key: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Index != 0 {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestGPUStackRerankRequiresModelName(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	model := newGPUStackRerankForTest("http://unused")
	apiKey := "test-key"
	_, err := model.Rerank(ctx, nil, RerankRequest{Query: "q", Documents: []string{"a"}}, &APIConfig{ApiKey: &apiKey}, &RerankConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("expected model-name error, got %v", err)
	}
}

func TestGPUStackRerankRejectsHTTPError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newGPUStackRerankServer(t, "Bearer test-key", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	})
	defer srv.Close()

	model := newGPUStackRerankForTest(srv.URL)
	apiKey := "test-key"
	modelName := "qwen3-reranker"
	_, err := model.Rerank(ctx, &modelName, RerankRequest{Query: "q", Documents: []string{"a"}}, &APIConfig{ApiKey: &apiKey}, &RerankConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "gpustack rerank API error") {
		t.Errorf("expected API error, got %v", err)
	}
}

func TestGPUStackRerankRejectsOutOfRangeIndex(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newGPUStackRerankServer(t, "Bearer test-key", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{"index": 5, "relevance_score": 0.9},
			},
		})
	})
	defer srv.Close()

	model := newGPUStackRerankForTest(srv.URL)
	apiKey := "test-key"
	modelName := "qwen3-reranker"
	_, err := model.Rerank(ctx, &modelName, RerankRequest{Query: "q", Documents: []string{"a", "b"}}, &APIConfig{ApiKey: &apiKey}, &RerankConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("expected out-of-range error, got %v", err)
	}
}

func TestGPUStackRerankRejectsDuplicateIndex(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newGPUStackRerankServer(t, "Bearer test-key", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{"index": 0, "relevance_score": 0.9},
				{"index": 0, "relevance_score": 0.4},
			},
		})
	})
	defer srv.Close()

	model := newGPUStackRerankForTest(srv.URL)
	apiKey := "test-key"
	modelName := "qwen3-reranker"
	_, err := model.Rerank(ctx, &modelName, RerankRequest{Query: "q", Documents: []string{"a", "b"}}, &APIConfig{ApiKey: &apiKey}, &RerankConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "duplicate rerank index") {
		t.Errorf("expected duplicate-index error, got %v", err)
	}
}

func TestGPUStackRerankRequiresURLSuffix(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	model := NewGPUStackModel(map[string]string{"default": "http://unused"}, URLSuffix{})
	apiKey := "test-key"
	modelName := "qwen3-reranker"
	_, err := model.Rerank(ctx, &modelName, RerankRequest{Query: "q", Documents: []string{"a"}}, &APIConfig{ApiKey: &apiKey}, &RerankConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "rerank URL suffix is not configured") {
		t.Errorf("expected missing-suffix error, got %v", err)
	}
}

// Users habitually paste a base URL that already ends in /v1; joinURL must
// collapse the duplicated version segment so the request still hits /v1/rerank.
func TestGPUStackRerankBaseURLWithVersionSegment(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newGPUStackRerankServer(t, "", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{{"index": 0, "relevance_score": 0.5}},
		})
	})
	defer srv.Close()

	model := NewGPUStackModel(map[string]string{"default": srv.URL + "/v1"}, URLSuffix{Rerank: "v1/rerank"})
	modelName := "qwen3-reranker"
	if _, err := model.Rerank(ctx, &modelName, RerankRequest{Query: "q", Documents: []string{"a"}}, &APIConfig{}, &RerankConfig{}, nil); err != nil {
		t.Fatalf("Rerank failed with /v1 base URL: %v", err)
	}
}
