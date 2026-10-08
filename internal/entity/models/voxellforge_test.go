package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"strings"
	"testing"
	"unicode/utf16"
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
					t.Errorf("decode request: %v", err)
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
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

// Forge's edge rejects any input over 32,000 characters and any request over
// 256,000, so Embed must cap each input and split the texts across requests,
// returning indexes that refer to the caller's slice.
func TestVoxellForgeEmbedRespectsSizeLimits(t *testing.T) {
	withSSRFBypass(t)
	var requests [][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		total := 0
		for _, text := range body.Input {
			n := len(utf16.Encode([]rune(text)))
			if n > voxellForgeMaxInputChars {
				t.Errorf("input of %d chars exceeds %d", n, voxellForgeMaxInputChars)
			}
			total += n
		}
		if total > voxellForgeMaxRequestChars {
			t.Errorf("request of %d chars exceeds %d", total, voxellForgeMaxRequestChars)
		}
		requests = append(requests, body.Input)
		data := make([]map[string]any, len(body.Input))
		for i := range body.Input {
			data[i] = map[string]any{"object": "embedding", "embedding": []float64{0.1}, "index": i}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data})
	}))
	defer srv.Close()

	// Twelve inputs of 40,000 characters: each is capped to 32,000, and eight
	// capped inputs fill one 256,000-character request, so two requests go out.
	// The emoji is two UTF-16 code units, the unit the edge counts in.
	texts := make([]string, 12)
	for i := range texts {
		texts[i] = strings.Repeat("\U0001F600", 20000)
	}
	modelName := "turbo"
	embeddings, err := newVoxellForgeForTest(srv.URL).Embed(
		t.Context(),
		&modelName,
		EmbedRequest{Texts: texts},
		&APIConfig{ApiKey: &testAPIKey},
		nil,
		&common.ModelUsage{},
	)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(requests) != 2 || len(requests[0]) != 8 || len(requests[1]) != 4 {
		sizes := make([]int, len(requests))
		for i, r := range requests {
			sizes[i] = len(r)
		}
		t.Fatalf("request sizes=%v, want [8 4]", sizes)
	}
	if len(embeddings) != len(texts) {
		t.Fatalf("got %d embeddings, want %d", len(embeddings), len(texts))
	}
	for i, e := range embeddings {
		if e.Index != i {
			t.Errorf("embeddings[%d].Index=%d", i, e.Index)
		}
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
