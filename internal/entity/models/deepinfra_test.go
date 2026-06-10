package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
			Models:    "models/list",
			Rerank:    "v1/inference",
		},
	)
}

func TestDeepInfraListModelsHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/list" {
			t.Errorf("path=%s, want /models/list", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q, want Bearer test-key", got)
		}
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{
			{"model_name": " deepseek-ai/DeepSeek-V3.2 ", "reported_type": "text-generation", "max_tokens": 160000},
			{"model_name": "", "reported_type": "text-generation"},
			{"model_name": "Qwen/Qwen3-Embedding-8B", "reported_type": "embeddings"},
		})
	}))
	defer srv.Close()

	apiKey := "test-key"
	models, err := newDeepInfraForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("len(models)=%d, want 2: %#v", len(models), models)
	}
	if models[0].Name != "deepseek-ai/DeepSeek-V3.2" {
		t.Errorf("models[0].Name=%q", models[0].Name)
	}
	if models[0].MaxTokens == nil || *models[0].MaxTokens != 160000 {
		t.Errorf("models[0].MaxTokens=%v", models[0].MaxTokens)
	}
	if models[1].Name != "Qwen/Qwen3-Embedding-8B" {
		t.Errorf("models[1].Name=%q", models[1].Name)
	}
}

func TestDeepInfraListModelsRequiresAPIKey(t *testing.T) {
	_, err := newDeepInfraForTest("http://unused").ListModels(&APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Fatalf("err=%v, want api-key error", err)
	}
}

func TestDeepInfraListModelsRejectsInvalidResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []string{"bad"}})
	}))
	defer srv.Close()

	apiKey := "test-key"
	_, err := newDeepInfraForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "failed to unmarshal response") {
		t.Fatalf("err=%v, want unmarshal error", err)
	}
}

func TestDeepInfraListModelsIntegration(t *testing.T) {
	if os.Getenv("DEEPINFRA_LIST_MODELS_INTEGRATION") != "1" {
		t.Skip("set DEEPINFRA_LIST_MODELS_INTEGRATION=1 to call the DeepInfra models API")
	}

	apiKey := strings.TrimSpace(os.Getenv("DEEPINFRA_API_KEY"))
	if apiKey == "" {
		t.Fatal("DEEPINFRA_API_KEY is required")
	}

	models, err := newDeepInfraForTest("https://api.deepinfra.com").ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected at least one DeepInfra model")
	}

	samples := make([]string, 0, 3)
	for _, model := range models {
		if model.Name == "" {
			t.Fatal("model name should not be empty")
		}
		if len(samples) < 3 {
			samples = append(samples, model.Name)
		}
	}
	t.Logf("DeepInfra ListModels returned %d models; samples: %s", len(models), strings.Join(samples, ", "))
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
