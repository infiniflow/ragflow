package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newVoyageServer(t *testing.T, expectedPath string, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectedPath {
			t.Errorf("expected path=%s, got %s", expectedPath, r.URL.Path)
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
			t.Errorf("read body: %v", err)
			return
		}
		var body map[string]interface{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("unmarshal: %v\nraw=%s", err, string(raw))
			return
		}
		handler(t, body, w)
	}))
}

func newVoyageForTest(baseURL string) *VoyageModel {
	return NewVoyageModel(
		map[string]string{"default": baseURL},
		URLSuffix{Embedding: "v1/embeddings", Rerank: "v1/rerank"},
	)
}

func TestVoyageName(t *testing.T) {
	if got := newVoyageForTest("http://unused").Name(); got != "voyage" {
		t.Errorf("Name()=%q, want %q", got, "voyage")
	}
}

func TestVoyageEmbedHappyPath(t *testing.T) {
	srv := newVoyageServer(t, "/v1/embeddings", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "voyage-3.5" {
			t.Errorf("model=%v", body["model"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"object": "list",
			"data": []map[string]interface{}{
				{"object": "embedding", "embedding": []float64{0.1, 0.2}, "index": 0},
				{"object": "embedding", "embedding": []float64{0.3, 0.4}, "index": 1},
				{"object": "embedding", "embedding": []float64{0.5, 0.6}, "index": 2},
			},
			"model": "voyage-3.5",
		})
	})
	defer srv.Close()

	v := newVoyageForTest(srv.URL)
	apiKey := "test-key"
	model := "voyage-3.5"
	vecs, err := v.Embed(&model, []string{"a", "b", "c"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 3 {
		t.Fatalf("len=%d want 3", len(vecs))
	}
	if vecs[1].Embedding[0] != 0.3 || vecs[1].Index != 1 {
		t.Errorf("vecs[1]=%+v", vecs[1])
	}
}

// TestVoyageEmbedPropagatesOutputDimension pins the docs-spelled
// param name. Voyage 400s on any other key (live-verified — sending
// "dimensions" returns "Argument 'dimensions' is not supported by our
// API"), so this name matters and must not regress.
func TestVoyageEmbedPropagatesOutputDimension(t *testing.T) {
	srv := newVoyageServer(t, "/v1/embeddings", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if got, ok := body["output_dimension"].(float64); !ok || got != 256 {
			t.Errorf("output_dimension=%v want 256", body["output_dimension"])
		}
		for _, wrong := range []string{"dimensions", "output_dimensions", "dimension"} {
			if _, present := body[wrong]; present {
				t.Errorf("must not send %q (Voyage rejects unknown fields)", wrong)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{{"embedding": []float64{0.1}, "index": 0}},
		})
	})
	defer srv.Close()

	v := newVoyageForTest(srv.URL)
	apiKey := "test-key"
	model := "voyage-3.5"
	_, err := v.Embed(&model, []string{"x"}, &APIConfig{ApiKey: &apiKey},
		&EmbeddingConfig{Dimension: 256})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
}

// And when Dimension is zero/unset, the field MUST be absent — Voyage
// would default the vector length, but only if we don't send the key
// at all (sending output_dimension: 0 is a 400).
func TestVoyageEmbedOmitsOutputDimensionWhenUnset(t *testing.T) {
	srv := newVoyageServer(t, "/v1/embeddings", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if _, present := body["output_dimension"]; present {
			t.Errorf("output_dimension must be absent when Dimension is unset, got %v", body["output_dimension"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{{"embedding": []float64{0.1}, "index": 0}},
		})
	})
	defer srv.Close()

	v := newVoyageForTest(srv.URL)
	apiKey := "test-key"
	model := "voyage-3.5"
	_, err := v.Embed(&model, []string{"x"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
}

func TestVoyageEmbedReordersByIndex(t *testing.T) {
	srv := newVoyageServer(t, "/v1/embeddings", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{2}, "index": 2},
				{"embedding": []float64{0}, "index": 0},
				{"embedding": []float64{1}, "index": 1},
			},
		})
	})
	defer srv.Close()

	v := newVoyageForTest(srv.URL)
	apiKey := "test-key"
	model := "voyage-3.5"
	vecs, err := v.Embed(&model, []string{"a", "b", "c"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	for i, vec := range vecs {
		if vec.Index != i || vec.Embedding[0] != float64(i) {
			t.Errorf("slot %d=%+v", i, vec)
		}
	}
}

func TestVoyageEmbedEmptyInputShortCircuits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("Embed([]) made an unexpected HTTP call")
	}))
	defer srv.Close()

	v := newVoyageForTest(srv.URL)
	apiKey := "test-key"
	model := "voyage-3.5"
	vecs, err := v.Embed(&model, []string{}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil || len(vecs) != 0 {
		t.Errorf("Embed([])=(%v,%v)", vecs, err)
	}
}

func TestVoyageEmbedRequiresAPIKey(t *testing.T) {
	v := newVoyageForTest("http://unused")
	model := "voyage-3.5"
	_, err := v.Embed(&model, []string{"a"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestVoyageEmbedRequiresModelName(t *testing.T) {
	v := newVoyageForTest("http://unused")
	apiKey := "test-key"
	_, err := v.Embed(nil, []string{"a"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("expected model-name error, got %v", err)
	}
}

func TestVoyageEmbedRejectsDuplicateIndex(t *testing.T) {
	srv := newVoyageServer(t, "/v1/embeddings", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{1}, "index": 0},
				{"embedding": []float64{2}, "index": 0},
			},
		})
	})
	defer srv.Close()

	v := newVoyageForTest(srv.URL)
	apiKey := "test-key"
	model := "voyage-3.5"
	_, err := v.Embed(&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "duplicate embedding index 0") {
		t.Errorf("expected duplicate error, got %v", err)
	}
}

func TestVoyageEmbedRejectsOutOfRangeIndex(t *testing.T) {
	srv := newVoyageServer(t, "/v1/embeddings", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{1}, "index": 7},
			},
		})
	})
	defer srv.Close()

	v := newVoyageForTest(srv.URL)
	apiKey := "test-key"
	model := "voyage-3.5"
	_, err := v.Embed(&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("expected out-of-range error, got %v", err)
	}
}

func TestVoyageEmbedRejectsMissingSlot(t *testing.T) {
	srv := newVoyageServer(t, "/v1/embeddings", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{1}, "index": 0},
			},
		})
	})
	defer srv.Close()

	v := newVoyageForTest(srv.URL)
	apiKey := "test-key"
	model := "voyage-3.5"
	_, err := v.Embed(&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing embedding for input index 1") {
		t.Errorf("expected missing-slot error, got %v", err)
	}
}

func TestVoyageRerankHappyPath(t *testing.T) {
	srv := newVoyageServer(t, "/v1/rerank", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		// Voyage's request key is top_k (not top_n).
		if body["top_k"] != float64(3) {
			t.Errorf("top_k=%v want 3", body["top_k"])
		}
		if body["query"] != "x" {
			t.Errorf("query=%v", body["query"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"object": "list",
			"data": []map[string]interface{}{
				{"relevance_score": 0.8, "index": 2},
				{"relevance_score": 0.5, "index": 0},
				{"relevance_score": 0.3, "index": 1},
			},
			"model": "rerank-2",
		})
	})
	defer srv.Close()

	v := newVoyageForTest(srv.URL)
	apiKey := "test-key"
	model := "rerank-2"
	resp, err := v.Rerank(&model, "x", []string{"a", "b", "c"},
		&APIConfig{ApiKey: &apiKey}, &RerankConfig{TopN: 3})
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(resp.Data) != 3 {
		t.Fatalf("len=%d want 3", len(resp.Data))
	}
	want := map[int]float64{0: 0.5, 1: 0.3, 2: 0.8}
	for _, r := range resp.Data {
		if got, ok := want[r.Index]; !ok || got != r.RelevanceScore {
			t.Errorf("unexpected result index=%d score=%v", r.Index, r.RelevanceScore)
		}
	}
}

func TestVoyageRerankTopKDefaultsToLenDocuments(t *testing.T) {
	srv := newVoyageServer(t, "/v1/rerank", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["top_k"] != float64(4) {
			t.Errorf("top_k=%v want 4 (len(documents))", body["top_k"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]interface{}{}})
	})
	defer srv.Close()

	v := newVoyageForTest(srv.URL)
	apiKey := "test-key"
	model := "rerank-2"
	_, err := v.Rerank(&model, "x", []string{"a", "b", "c", "d"},
		&APIConfig{ApiKey: &apiKey}, &RerankConfig{})
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
}

func TestVoyageRerankEmptyDocuments(t *testing.T) {
	v := newVoyageForTest("http://unused")
	apiKey := "test-key"
	model := "rerank-2"
	resp, err := v.Rerank(&model, "x", nil,
		&APIConfig{ApiKey: &apiKey}, &RerankConfig{TopN: 0})
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected empty Data, got %d", len(resp.Data))
	}
}

func TestVoyageRerankRejectsOutOfRangeIndex(t *testing.T) {
	srv := newVoyageServer(t, "/v1/rerank", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"relevance_score": 0.9, "index": 7},
			},
		})
	})
	defer srv.Close()

	v := newVoyageForTest(srv.URL)
	apiKey := "test-key"
	model := "rerank-2"
	_, err := v.Rerank(&model, "x", []string{"a", "b"},
		&APIConfig{ApiKey: &apiKey}, &RerankConfig{TopN: 2})
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("expected out-of-range error, got %v", err)
	}
}

func TestVoyageRerankRejectsDuplicateIndex(t *testing.T) {
	// A duplicate index would silently overwrite an earlier slot, which
	// is the same failure mode Embed already guards against. Make sure
	// Rerank fails loudly too.
	srv := newVoyageServer(t, "/v1/rerank", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"relevance_score": 0.9, "index": 0},
				{"relevance_score": 0.8, "index": 0},
			},
		})
	})
	defer srv.Close()

	v := newVoyageForTest(srv.URL)
	apiKey := "test-key"
	model := "rerank-2"
	_, err := v.Rerank(&model, "x", []string{"a", "b"},
		&APIConfig{ApiKey: &apiKey}, &RerankConfig{TopN: 2})
	if err == nil || !strings.Contains(err.Error(), "duplicate rerank index 0") {
		t.Errorf("expected duplicate-index error, got %v", err)
	}
}

// TestVoyageEmbedTrimsTrailingSlashInBaseURL guards against a
// misconfigured baseURL ending in "/" producing a double-slash path
// (e.g. `.../v1//embeddings`). Rerank already trims, so Embed must
// trim too; CodeRabbit flagged the inconsistency.
func TestVoyageEmbedTrimsTrailingSlashInBaseURL(t *testing.T) {
	var sawPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{{"embedding": []float64{1}, "index": 0}},
		})
	}))
	defer srv.Close()

	v := NewVoyageModel(
		map[string]string{"default": srv.URL + "/"}, // trailing slash
		URLSuffix{Embedding: "v1/embeddings", Rerank: "v1/rerank"},
	)
	apiKey := "test-key"
	model := "voyage-3.5"
	if _, err := v.Embed(&model, []string{"x"}, &APIConfig{ApiKey: &apiKey}, nil); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if sawPath != "/v1/embeddings" {
		t.Errorf("path=%q want %q (no double slash)", sawPath, "/v1/embeddings")
	}
}
