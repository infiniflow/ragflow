package models

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestNvidiaListModelsUsesExactEndpointIDs(t *testing.T) {
	withSSRFBypass(t)
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
	models, err := driver.ListModels(t.Context(), &APIConfig{ApiKey: ptr(apiKey), Region: &region})
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
			MaxOutput:  &maxTokens,
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
	if models[0].MaxOutput == nil || *models[0].MaxOutput != maxTokens {
		t.Fatalf("MaxOutput = %v, want %d", models[0].MaxOutput, maxTokens)
	}
	if models[0].Thinking == nil || !models[0].Thinking.DefaultValue {
		t.Fatalf("Thinking = %#v, want preset metadata", models[0].Thinking)
	}
}

func TestParseNvidiaModelListInfersTypesForPresetWithoutTypes(t *testing.T) {
	preset := &Model{Name: "nvidia/nv-embed-v1"}
	models := parseNvidiaModelList(ModelList{Models: []ModelListItem{
		{ID: "nvidia/nv-embed-v1", OwnedBy: "nvidia"},
	}}, &Provider{Models: []*Model{preset}})

	if len(models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(models))
	}
	if got := models[0].ModelTypes; len(got) != 1 || got[0] != "embedding" {
		t.Fatalf("ModelTypes = %v, want [embedding]", got)
	}
	if preset.ModelTypes != nil {
		t.Fatalf("preset ModelTypes mutated to %v", preset.ModelTypes)
	}
}

func TestNvidiaListModelsFiltersHostedCatalog(t *testing.T) {
	withSSRFBypass(t)
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
	models, err := driver.ListModels(t.Context(), &APIConfig{ApiKey: ptr(apiKey), Region: &region})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}

	if got := joinModelNames(models, ","); got != "future/preserved-model,meta/llama-3.1-8b-instruct" {
		t.Fatalf("model names = %q, want active hosted models", got)
	}
}

func TestNvidiaFetchHostedCatalogPaginates(t *testing.T) {
	withSSRFBypass(t)
	resources := make([]nvidiaCatalogResource, nvidiaCatalogPageSize+1)
	for i := range resources {
		resources[i] = nvidiaCatalogResource{DisplayName: fmt.Sprintf("model-%d", i)}
	}

	requestedPages := make([]int, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var query struct {
			Page     int `json:"page"`
			PageSize int `json:"pageSize"`
		}
		if err := json.Unmarshal([]byte(r.URL.Query().Get("q")), &query); err != nil {
			t.Errorf("decode catalog query: %v", err)
			return
		}
		requestedPages = append(requestedPages, query.Page)
		if query.PageSize != nvidiaCatalogPageSize {
			t.Errorf("pageSize = %d, want %d", query.PageSize, nvidiaCatalogPageSize)
			return
		}

		start := query.Page * query.PageSize
		end := min(start+query.PageSize, len(resources))
		var pageResources []nvidiaCatalogResource
		if start < len(resources) {
			pageResources = resources[start:end]
		}
		_ = json.NewEncoder(w).Encode(nvidiaCatalogResponse{Results: []nvidiaCatalogGroup{{
			GroupValue: "ENDPOINT",
			TotalCount: len(resources),
			Resources:  pageResources,
		}}})
	}))
	defer server.Close()

	driver := NewNvidiaModel(nil, URLSuffix{})
	driver.catalogURL = server.URL
	catalog, err := driver.fetchHostedCatalog(t.Context())
	if err != nil {
		t.Fatalf("fetchHostedCatalog() error = %v", err)
	}
	if got := requestedPages; len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("requested pages = %v, want [0 1]", got)
	}
	if got := len(catalog.Results[0].Resources); got != len(resources) {
		t.Fatalf("resources = %d, want %d", got, len(resources))
	}
}

func TestNvidiaListModelsRejectsPartialHostedCatalog(t *testing.T) {
	withSSRFBypass(t)
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
	if _, err := driver.ListModels(t.Context(), &APIConfig{ApiKey: ptr("nvapi-test"), Region: &region}); err == nil {
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

func withNvidiaProviderManager(t *testing.T, models []*Model) {
	t.Helper()
	saved := providerManager
	providerManager = &ProviderManager{
		Providers: []Provider{{
			Name:   "NVIDIA",
			URL:    map[string]string{"default": "https://integrate.api.nvidia.com/v1"},
			Models: models,
		}},
	}
	t.Cleanup(func() { providerManager = saved })
}

func TestNvidiaResolveEndpointUsesModelURL(t *testing.T) {
	const modelURL = "https://integrate.api.nvidia.com/v1/meta/llama-3.2-11b-vision-instruct"
	withNvidiaProviderManager(t, []*Model{{
		Name: "meta/llama-3.2-11b-vision-instruct",
		URL:  modelURL,
	}})

	driver := NewNvidiaModel(
		map[string]string{"default": "https://integrate.api.nvidia.com/v1"},
		URLSuffix{Chat: "chat/completions"},
	)
	got, err := driver.resolveEndpoint("meta/llama-3.2-11b-vision-instruct", &APIConfig{}, "chat/completions")
	if err != nil {
		t.Fatalf("resolveEndpoint() error = %v", err)
	}
	if got != modelURL {
		t.Fatalf("resolveEndpoint() = %q, want %q", got, modelURL)
	}
}

func TestNvidiaResolveEndpointFallsBackToAssembly(t *testing.T) {
	withNvidiaProviderManager(t, []*Model{{Name: "meta/llama-3.1-8b-instruct"}})

	driver := NewNvidiaModel(
		map[string]string{"default": "https://integrate.api.nvidia.com/v1/"},
		URLSuffix{Chat: "chat/completions", Embedding: "embeddings"},
	)
	got, err := driver.resolveEndpoint("meta/llama-3.1-8b-instruct", &APIConfig{}, "chat/completions")
	if err != nil {
		t.Fatalf("resolveEndpoint() error = %v", err)
	}
	if want := "https://integrate.api.nvidia.com/v1/chat/completions"; got != want {
		t.Fatalf("resolveEndpoint() = %q, want %q", got, want)
	}

	got, err = driver.resolveEndpoint("meta/llama-3.1-8b-instruct", &APIConfig{}, "embeddings")
	if err != nil {
		t.Fatalf("resolveEndpoint() error = %v", err)
	}
	if want := "https://integrate.api.nvidia.com/v1/embeddings"; got != want {
		t.Fatalf("resolveEndpoint() = %q, want %q", got, want)
	}
}

func TestNvidiaResolveEndpointIgnoresManagerWhenNil(t *testing.T) {
	saved := providerManager
	providerManager = nil
	defer func() { providerManager = saved }()

	driver := NewNvidiaModel(
		map[string]string{"default": "https://integrate.api.nvidia.com/v1"},
		URLSuffix{},
	)
	got, err := driver.resolveEndpoint("meta/llama-3.2-11b-vision-instruct", &APIConfig{}, "chat/completions")
	if err != nil {
		t.Fatalf("resolveEndpoint() error = %v", err)
	}
	if want := "https://integrate.api.nvidia.com/v1/chat/completions"; got != want {
		t.Fatalf("resolveEndpoint() = %q, want %q", got, want)
	}
}

func TestNvidiaChatUsesModelSpecificURL(t *testing.T) {
	withSSRFBypass(t)
	const apiKey = "nvapi-test"
	const modelName = "meta/llama-3.2-11b-vision-instruct"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/meta/llama-3.2-11b-vision-instruct" {
			t.Fatalf("path = %s, want model-specific chat endpoint", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message":       map[string]interface{}{"role": "assistant", "content": "pong"},
				"finish_reason": "stop",
			}},
			"usage": map[string]interface{}{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer server.Close()

	withNvidiaProviderManager(t, []*Model{{
		Name: modelName,
		URL:  server.URL + "/v1/meta/llama-3.2-11b-vision-instruct",
	}})
	driver := NewNvidiaModel(
		map[string]string{"default": server.URL + "/v1"},
		URLSuffix{Chat: "chat/completions"},
	)
	resp, err := driver.ChatWithMessages(
		t.Context(),
		modelName,
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: ptr(apiKey)},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages() error = %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Fatalf("answer = %v, want pong", resp.Answer)
	}
}

func TestNvidiaEmbedUsesModelSpecificURL(t *testing.T) {
	withSSRFBypass(t)
	const apiKey = "nvapi-test"
	const modelName = "nvidia/nv-embed-v1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings/nvidia/nv-embed-v1" {
			t.Fatalf("path = %s, want model-specific embed endpoint", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{{"index": 0, "embedding": []float64{0.1, 0.2}}},
		})
	}))
	defer server.Close()

	withNvidiaProviderManager(t, []*Model{{
		Name: modelName,
		URL:  server.URL + "/v1/embeddings/nvidia/nv-embed-v1",
	}})
	driver := NewNvidiaModel(
		map[string]string{"default": server.URL + "/v1"},
		URLSuffix{Embedding: "embeddings"},
	)
	namePtr := modelName
	got, err := driver.Embed(
		t.Context(),
		&namePtr,
		EmbedRequest{Texts: []string{"hello"}},
		&APIConfig{ApiKey: ptr(apiKey)},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(got) != 1 || len(got[0].Embedding) != 2 {
		t.Fatalf("embedding = %#v, want 1 vector of length 2", got)
	}
}

func ptr[T any](value T) *T {
	return &value
}
