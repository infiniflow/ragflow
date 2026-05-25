package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newVllmRerankServer(t *testing.T, expectAuth string, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
			return
		}
		if r.URL.Path != "/rerank" {
			t.Errorf("expected path=/rerank, got %s", r.URL.Path)
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

func newVllmModelForTest(baseURL string) *VllmModel {
	return NewVllmModel(
		map[string]string{"default": baseURL},
		URLSuffix{Rerank: "rerank"},
	)
}

func TestVllmRerankHappyPath(t *testing.T) {
	srv := newVllmRerankServer(t, "Bearer test-key", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "BAAI/bge-reranker-v2-m3" {
			t.Errorf("expected model=BAAI/bge-reranker-v2-m3, got %v", body["model"])
		}
		if body["query"] != "What is RAPTOR?" {
			t.Errorf("expected query=What is RAPTOR?, got %v", body["query"])
		}
		// vLLM differs from NVIDIA: documents is a flat []string, not [{text}].
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
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{"index": 2, "relevance_score": 0.95},
				{"index": 0, "relevance_score": 0.42},
				{"index": 1, "relevance_score": 0.78},
			},
		})
	})
	defer srv.Close()

	model := newVllmModelForTest(srv.URL)
	apiKey := "test-key"
	modelName := "BAAI/bge-reranker-v2-m3"
	resp, err := model.Rerank(
		&modelName,
		"What is RAPTOR?",
		[]string{"doc-zero", "doc-one", "doc-two"},
		&APIConfig{ApiKey: &apiKey},
		&RerankConfig{},
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
}

func TestVllmRerankTopNClamp(t *testing.T) {
	srv := newVllmRerankServer(t, "Bearer test-key", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["top_n"] != float64(2) {
			t.Errorf("expected top_n clamp to RerankConfig.TopN=2, got %v", body["top_n"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []map[string]interface{}{}})
	})
	defer srv.Close()

	model := newVllmModelForTest(srv.URL)
	apiKey := "test-key"
	modelName := "BAAI/bge-reranker-v2-m3"
	if _, err := model.Rerank(
		&modelName, "q",
		[]string{"a", "b", "c", "d"},
		&APIConfig{ApiKey: &apiKey},
		&RerankConfig{TopN: 2},
	); err != nil {
		t.Fatalf("Rerank failed: %v", err)
	}
}

func TestVllmRerankEmptyDocuments(t *testing.T) {
	model := newVllmModelForTest("http://unused")
	apiKey := "test-key"
	modelName := "BAAI/bge-reranker-v2-m3"
	resp, err := model.Rerank(&modelName, "q", nil, &APIConfig{ApiKey: &apiKey}, &RerankConfig{})
	if err != nil {
		t.Fatalf("expected nil error for empty documents, got %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected empty Data, got %d entries", len(resp.Data))
	}
}

// vLLM is a local driver; the Authorization header must be omitted when
// no APIConfig.ApiKey is configured. This diverges from the NVIDIA driver
// which requires an API key.
func TestVllmRerankWithoutAPIKey(t *testing.T) {
	srv := newVllmRerankServer(t, "", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{"index": 0, "relevance_score": 0.5},
			},
		})
	})
	defer srv.Close()

	model := newVllmModelForTest(srv.URL)
	modelName := "BAAI/bge-reranker-v2-m3"
	resp, err := model.Rerank(&modelName, "q", []string{"a"}, &APIConfig{}, &RerankConfig{})
	if err != nil {
		t.Fatalf("Rerank failed without api key: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Index != 0 {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestVllmRerankRequiresModelName(t *testing.T) {
	model := newVllmModelForTest("http://unused")
	apiKey := "test-key"
	_, err := model.Rerank(nil, "q", []string{"a"}, &APIConfig{ApiKey: &apiKey}, &RerankConfig{})
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("expected model-name error, got %v", err)
	}
}

func TestVllmRerankRejectsHTTPError(t *testing.T) {
	srv := newVllmRerankServer(t, "Bearer test-key", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	})
	defer srv.Close()

	model := newVllmModelForTest(srv.URL)
	apiKey := "test-key"
	modelName := "BAAI/bge-reranker-v2-m3"
	_, err := model.Rerank(&modelName, "q", []string{"a"}, &APIConfig{ApiKey: &apiKey}, &RerankConfig{})
	if err == nil || !strings.Contains(err.Error(), "vLLM rerank API error") {
		t.Errorf("expected API error, got %v", err)
	}
}

func TestVllmRerankRejectsOutOfRangeIndex(t *testing.T) {
	srv := newVllmRerankServer(t, "Bearer test-key", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{"index": 5, "relevance_score": 0.9},
			},
		})
	})
	defer srv.Close()

	model := newVllmModelForTest(srv.URL)
	apiKey := "test-key"
	modelName := "BAAI/bge-reranker-v2-m3"
	_, err := model.Rerank(&modelName, "q", []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, &RerankConfig{})
	if err == nil || !strings.Contains(err.Error(), "unexpected rerank index") {
		t.Errorf("expected out-of-range error, got %v", err)
	}
}
