package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNvidiaListModelsUsesExactEndpointIDs(t *testing.T) {
	const apiKey = "nvapi-test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			t.Fatalf("Authorization = %q, want Bearer token", got)
		}
		_ = json.NewEncoder(w).Encode(ModelList{
			Object: "list",
			Models: []ModelListItem{
				{ID: "meta/llama-3.3-70b-instruct", Object: "model", OwnedBy: "meta"},
				{ID: " nvidia/nv-embed-v1 ", Object: "model", OwnedBy: "nvidia"},
				{ID: "meta/llama-3.3-70b-instruct", Object: "model", OwnedBy: "meta"},
				{ID: "   ", Object: "model", OwnedBy: "nvidia"},
			},
		})
	}))
	defer server.Close()

	driver := NewNvidiaModel(
		map[string]string{"default": server.URL + "/v1"},
		URLSuffix{Models: "models"},
	)
	region := "default"
	models, err := driver.ListModels(context.Background(), &APIConfig{ApiKey: ptr(apiKey), Region: &region})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}

	if got := joinModelNames(models, ","); got != "meta/llama-3.3-70b-instruct,nvidia/nv-embed-v1" {
		t.Fatalf("model names = %q", got)
	}
	if got := models[0].ModelTypes; len(got) != 1 || got[0] != "chat" {
		t.Fatalf("chat model types = %v, want [chat]", got)
	}
	if got := models[1].ModelTypes; len(got) != 1 || got[0] != "embedding" {
		t.Fatalf("embedding model types = %v, want [embedding]", got)
	}
}

func TestParseNvidiaModelListPrefersPresetMetadata(t *testing.T) {
	maxTokens := 131072
	provider := &Provider{Models: []*Model{
		{
			Name:       "nvidia/nemotron-3-super-120b-a12b",
			MaxTokens:  &maxTokens,
			ModelTypes: []string{"chat"},
			Thinking:   &ModelThinking{DefaultValue: true, ClearThinking: true},
		},
	}}

	models := parseNvidiaModelList(ModelList{Models: []ModelListItem{
		{ID: "nvidia/nemotron-3-super-120b-a12b", OwnedBy: "nvidia"},
	}}, provider)
	if len(models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(models))
	}
	if models[0].MaxTokens == nil || *models[0].MaxTokens != maxTokens {
		t.Fatalf("MaxTokens = %v, want %d", models[0].MaxTokens, maxTokens)
	}
	if models[0].Thinking == nil || !models[0].Thinking.DefaultValue {
		t.Fatalf("Thinking = %#v, want preset metadata", models[0].Thinking)
	}
}

func ptr[T any](value T) *T {
	return &value
}
