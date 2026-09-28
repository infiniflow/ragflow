package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
)

func newVoxellForgeForTest(baseURL string) *VoxellForgeModel {
	return NewVoxellForgeModel(
		map[string]string{"default": baseURL},
		URLSuffix{Models: "models", Embedding: "embeddings"},
	)
}

func TestVoxellForgeName(t *testing.T) {
	if got := newVoxellForgeForTest("http://unused").Name(); got != "voxellforge" {
		t.Errorf("Name()=%q", got)
	}
}

func TestVoxellForgeFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("Voxell Forge", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*VoxellForgeModel); !ok {
		t.Fatalf("driver type=%T, want *VoxellForgeModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*VoxellForgeModel); !ok {
		t.Fatal("NewInstance did not return *VoxellForgeModel")
	}
}

// Forge is a hosted API, so every embeddings call must carry the tenant's key
// as a bearer token, a requested dimension must reach the request body, and
// input_type must say whether the texts are queries or documents.
func TestVoxellForgeEmbed(t *testing.T) {
	withSSRFBypass(t)
	for _, tc := range []struct {
		name      string
		query     bool
		inputType string
	}{
		{"document", false, "document"},
		{"query", true, "query"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/embeddings" {
					t.Errorf("%s %s", r.Method, r.URL.Path)
				}
				if got, want := r.Header.Get("Authorization"), "Bearer "+testAPIKey; got != want {
					t.Errorf("Authorization=%q, want %q", got, want)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				if body["model"] != "turbo" {
					t.Errorf("model=%v", body["model"])
				}
				if body["dimensions"] != float64(256) {
					t.Errorf("dimensions=%v", body["dimensions"])
				}
				if body["input_type"] != tc.inputType {
					t.Errorf("input_type=%v, want %q", body["input_type"], tc.inputType)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"object": "list",
					"model":  "turbo",
					"data": []map[string]any{
						{"object": "embedding", "embedding": []float64{0.1, 0.2}, "index": 0},
						{"object": "embedding", "embedding": []float64{0.3, 0.4}, "index": 1},
					},
					"usage": map[string]any{"prompt_tokens": 7, "total_tokens": 7},
				})
			}))
			defer srv.Close()

			modelName := "turbo"
			embeddings, err := newVoxellForgeForTest(srv.URL).Embed(
				t.Context(),
				&modelName,
				EmbedRequest{Texts: []string{"first text", "second text"}, Query: tc.query},
				&APIConfig{ApiKey: &testAPIKey},
				&EmbeddingConfig{Dimension: 256},
				&common.ModelUsage{},
			)
			if err != nil {
				t.Fatalf("Embed: %v", err)
			}
			if len(embeddings) != 2 || embeddings[1].Index != 1 || len(embeddings[1].Embedding) != 2 {
				t.Fatalf("embeddings=%#v", embeddings)
			}
		})
	}
}

// Forge model ids carry no embedding hint, so the live listing must be tagged
// as embedding explicitly rather than falling through to chat.
func TestVoxellForgeListModelsAreEmbedding(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "forge-turbo", "object": "model"},
				{"id": "forge-pro", "object": "model"},
			},
		})
	}))
	defer srv.Close()

	list, err := newVoxellForgeForTest(srv.URL).ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "forge-turbo,forge-pro" {
		t.Fatalf("models=%v", list)
	}
	for _, m := range list {
		if len(m.ModelTypes) != 1 || m.ModelTypes[0] != "embedding" {
			t.Errorf("%s ModelTypes=%v, want [embedding]", m.Name, m.ModelTypes)
		}
	}
}
