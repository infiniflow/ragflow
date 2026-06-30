package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newNvidiaRerankServer(t *testing.T, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	// Use t.Errorf + return inside the handler goroutine; t.Fatalf would
	// only Goexit the handler goroutine and the test would silently pass.
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
			return
		}
		if r.URL.Path != "/ranking" {
			t.Errorf("expected path=/ranking, got %s", r.URL.Path)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization=Bearer test-key, got %q", got)
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

func newNvidiaModelForTest(baseURL string) *NvidiaModel {
	return NewNvidiaModel(
		map[string]string{"default": baseURL},
		URLSuffix{Rerank: "ranking"},
	)
}

func TestNvidiaRerankHappyPath(t *testing.T) {
	srv := newNvidiaRerankServer(t, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "nvidia/nv-rerankqa-mistral-4b-v3" {
			t.Errorf("expected model=nvidia/nv-rerankqa-mistral-4b-v3, got %v", body["model"])
		}
		query, ok := body["query"].(map[string]interface{})
		if !ok || query["text"] != "What is RAPTOR?" {
			t.Errorf("expected query.text=What is RAPTOR?, got %v", body["query"])
		}
		passages, ok := body["passages"].([]interface{})
		if !ok || len(passages) != 3 {
			t.Errorf("expected 3 passages, got %v", body["passages"])
			return
		}
		if body["truncate"] != "END" {
			t.Errorf("expected truncate=END, got %v", body["truncate"])
		}
		if body["top_n"] != float64(3) {
			t.Errorf("expected top_n=3 (matching len(documents)), got %v", body["top_n"])
		}
		// Return rankings out of input order to verify Index preservation.
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"rankings": []map[string]interface{}{
				{"index": 2, "logit": 9.5},
				{"index": 0, "logit": 4.25},
				{"index": 1, "logit": 7.8},
			},
		})
	})
	defer srv.Close()

	model := newNvidiaModelForTest(srv.URL)
	apiKey := "test-key"
	modelName := "nvidia/nv-rerankqa-mistral-4b-v3"
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
	want := map[int]float64{0: 4.25, 1: 7.8, 2: 9.5}
	for _, r := range resp.Data {
		if got, ok := want[r.Index]; !ok || got != r.RelevanceScore {
			t.Errorf("unexpected result Index=%d RelevanceScore=%v", r.Index, r.RelevanceScore)
		}
	}
}

func TestNvidiaRerankTopNClamp(t *testing.T) {
	srv := newNvidiaRerankServer(t, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["top_n"] != float64(2) {
			t.Errorf("expected top_n clamp to RerankConfig.TopN=2, got %v", body["top_n"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"rankings": []map[string]interface{}{}})
	})
	defer srv.Close()

	model := newNvidiaModelForTest(srv.URL)
	apiKey := "test-key"
	modelName := "nvidia/nv-rerankqa-mistral-4b-v3"
	if _, err := model.Rerank(
		&modelName, "q",
		[]string{"a", "b", "c", "d"},
		&APIConfig{ApiKey: &apiKey},
		&RerankConfig{TopN: 2},
	); err != nil {
		t.Fatalf("Rerank failed: %v", err)
	}
}

func TestNvidiaRerankEmptyDocuments(t *testing.T) {
	model := newNvidiaModelForTest("http://unused")
	apiKey := "test-key"
	modelName := "nvidia/nv-rerankqa-mistral-4b-v3"
	resp, err := model.Rerank(&modelName, "q", nil, &APIConfig{ApiKey: &apiKey}, &RerankConfig{})
	if err != nil {
		t.Fatalf("expected nil error for empty documents, got %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected empty Data, got %d entries", len(resp.Data))
	}
}

func TestNvidiaRerankRequiresAPIKey(t *testing.T) {
	model := newNvidiaModelForTest("http://unused")
	modelName := "nvidia/nv-rerankqa-mistral-4b-v3"
	_, err := model.Rerank(&modelName, "q", []string{"a"}, &APIConfig{}, &RerankConfig{})
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestNvidiaRerankRequiresModelName(t *testing.T) {
	model := newNvidiaModelForTest("http://unused")
	apiKey := "test-key"
	_, err := model.Rerank(nil, "q", []string{"a"}, &APIConfig{ApiKey: &apiKey}, &RerankConfig{})
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("expected model-name error, got %v", err)
	}
}

func TestNvidiaRerankRejectsHTTPError(t *testing.T) {
	srv := newNvidiaRerankServer(t, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})
	defer srv.Close()

	model := newNvidiaModelForTest(srv.URL)
	apiKey := "test-key"
	modelName := "nvidia/nv-rerankqa-mistral-4b-v3"
	_, err := model.Rerank(&modelName, "q", []string{"a"}, &APIConfig{ApiKey: &apiKey}, &RerankConfig{})
	if err == nil || !strings.Contains(err.Error(), "Nvidia rerank API error") {
		t.Errorf("expected API error, got %v", err)
	}
}

func TestNvidiaRerankRejectsOutOfRangeIndex(t *testing.T) {
	srv := newNvidiaRerankServer(t, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"rankings": []map[string]interface{}{
				{"index": 5, "logit": 1.0}, // out of range for 2-input request
			},
		})
	})
	defer srv.Close()

	model := newNvidiaModelForTest(srv.URL)
	apiKey := "test-key"
	modelName := "nvidia/nv-rerankqa-mistral-4b-v3"
	_, err := model.Rerank(&modelName, "q", []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, &RerankConfig{})
	if err == nil || !strings.Contains(err.Error(), "unexpected rerank index") {
		t.Errorf("expected out-of-range error, got %v", err)
	}
}
