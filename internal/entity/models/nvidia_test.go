package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
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

func TestNvidiaListModelsFiltersHostedCatalog(t *testing.T) {
	const apiKey = "nvapi-test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(ModelList{Models: []ModelListItem{
				{ID: "aisingapore/sea-lion-7b-instruct"},
				{ID: "meta/llama-3.1-8b-instruct"},
				{ID: "nvidia/nv-embed-v1"},
				{ID: "future/preserved-model"},
			}})
		case "/catalog":
			_ = json.NewEncoder(w).Encode(nvidiaCatalogResponse{Results: []nvidiaCatalogGroup{{
				GroupValue: "ENDPOINT",
				TotalCount: 4,
				Resources: []nvidiaCatalogResource{
					{DisplayName: "sea-lion-7b-instruct", Labels: []nvidiaCatalogLabel{{Key: "publisher", UnresolvedValues: []string{"aisingapore"}}, {Key: "nimType", Values: []string{"Free Endpoint"}}}, Attributes: []nvidiaCatalogAttribute{{Key: "DEPRECATION", Value: "04/17/2026"}}},
					{DisplayName: "llama-3.1-8b-instruct", Labels: []nvidiaCatalogLabel{{Key: "publisher", UnresolvedValues: []string{"meta"}}, {Key: "nimType", Values: []string{"Free Endpoint"}}}},
					{DisplayName: "nv-embed-v1", Labels: []nvidiaCatalogLabel{{Key: "publisher", UnresolvedValues: []string{"nvidia"}}, {Key: "nimType", Values: []string{"Partner Endpoint"}}}},
					{DisplayName: "preserved-model", Labels: []nvidiaCatalogLabel{{Key: "publisher", UnresolvedValues: []string{"future"}}, {Key: "nimType", Values: []string{"Free Endpoint"}}}, Attributes: []nvidiaCatalogAttribute{{Key: "DEPRECATION", Value: "12/31/2099"}}},
				},
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	driver := NewNvidiaModel(map[string]string{"default": server.URL + "/v1"}, URLSuffix{Models: "models"})
	driver.catalogURL = server.URL + "/catalog"
	serverURL, _ := url.Parse(server.URL)
	driver.hostedAPIHost = serverURL.Hostname()
	region := "default"
	models, err := driver.ListModels(context.Background(), &APIConfig{ApiKey: ptr(apiKey), Region: &region})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}

	if got := joinModelNames(models, ","); got != "future/preserved-model,meta/llama-3.1-8b-instruct" {
		t.Fatalf("model names = %q, want active hosted models", got)
	}
}

func TestNvidiaListModelsRejectsPartialHostedCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_ = json.NewEncoder(w).Encode(ModelList{Models: []ModelListItem{{ID: "meta/llama-3.1-8b-instruct"}}})
			return
		}
		_ = json.NewEncoder(w).Encode(nvidiaCatalogResponse{Results: []nvidiaCatalogGroup{{
			GroupValue: "ENDPOINT", TotalCount: 2,
			Resources: []nvidiaCatalogResource{{DisplayName: "llama-3.1-8b-instruct"}},
		}}})
	}))
	defer server.Close()

	driver := NewNvidiaModel(map[string]string{"default": server.URL + "/v1"}, URLSuffix{Models: "models"})
	driver.catalogURL = server.URL + "/catalog"
	serverURL, _ := url.Parse(server.URL)
	driver.hostedAPIHost = serverURL.Hostname()
	region := "default"
	if _, err := driver.ListModels(context.Background(), &APIConfig{ApiKey: ptr("nvapi-test"), Region: &region}); err == nil {
		t.Fatal("ListModels() error = nil, want partial catalog rejection")
	}
}

func TestNvidiaCatalogResourceRejectsMalformedDeprecation(t *testing.T) {
	resource := nvidiaCatalogResource{
		Labels:     []nvidiaCatalogLabel{{Key: "nimType", Values: []string{"Free Endpoint"}}},
		Attributes: []nvidiaCatalogAttribute{{Key: "DEPRECATION", Value: "not-a-date"}},
	}

	if nvidiaCatalogResourceIsActive(resource, time.Now()) {
		t.Fatal("nvidiaCatalogResourceIsActive() = true, want false")
	}
}

func ptr[T any](value T) *T {
	return &value
}
