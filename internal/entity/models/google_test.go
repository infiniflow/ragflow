package models

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"google.golang.org/genai"
)

var googleListModelsMu sync.Mutex
var googleVertexListModelsMu sync.Mutex

func withGoogleListModelsStub(t *testing.T, fn func(context.Context, string) ([]string, error)) {
	t.Helper()

	googleListModelsMu.Lock()
	original := googleListModels
	googleListModels = fn
	t.Cleanup(func() {
		googleListModels = original
		googleListModelsMu.Unlock()
	})
}

func withGoogleVertexListModelsStub(t *testing.T, fn func(context.Context, *APIConfig, map[string]string) ([]string, error)) {
	t.Helper()

	googleVertexListModelsMu.Lock()
	original := googleVertexListModels
	googleVertexListModels = fn
	t.Cleanup(func() {
		googleVertexListModels = original
		googleVertexListModelsMu.Unlock()
	})
}

func TestGoogleModelListModelsRequiresAPIKey(t *testing.T) {
	model := &GoogleModel{}
	cases := []struct {
		name      string
		apiConfig *APIConfig
	}{
		{
			name:      "nil config",
			apiConfig: nil,
		},
		{
			name:      "nil api key",
			apiConfig: &APIConfig{},
		},
		{
			name: "empty api key",
			apiConfig: &APIConfig{
				ApiKey: stringPtr(""),
			},
		},
		{
			name: "blank api key",
			apiConfig: &APIConfig{
				ApiKey: stringPtr("  \t\n  "),
			},
		},
	}

	calls := 0
	withGoogleListModelsStub(t, func(context.Context, string) ([]string, error) {
		calls++
		return nil, nil
	})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			models, err := model.ListModels(tc.apiConfig)
			if err == nil {
				t.Fatal("expected an API key error")
			}
			if !strings.Contains(err.Error(), "api key is required") {
				t.Fatalf("expected API key error, got %v", err)
			}
			if models != nil {
				t.Fatalf("expected no models, got %v", models)
			}
		})
	}

	if calls != 0 {
		t.Fatalf("expected no ListModels calls without an API key, got %d", calls)
	}
}

func TestGoogleModelListModelsReturnsModelNames(t *testing.T) {
	model := &GoogleModel{}
	apiKey := "test-api-key"
	expected := []string{"models/gemini-2.5-flash", "models/gemini-2.5-pro"}

	withGoogleListModelsStub(t, func(_ context.Context, gotAPIKey string) ([]string, error) {
		if gotAPIKey != apiKey {
			t.Fatalf("expected API key %q, got %q", apiKey, gotAPIKey)
		}
		return expected, nil
	})

	models, err := model.ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !reflect.DeepEqual(models, expected) {
		t.Fatalf("expected models %v, got %v", expected, models)
	}
}

func TestGoogleModelCheckConnectionUsesListModels(t *testing.T) {
	model := &GoogleModel{}
	apiKey := "test-api-key"
	calls := 0

	withGoogleListModelsStub(t, func(_ context.Context, gotAPIKey string) ([]string, error) {
		calls++
		if gotAPIKey != apiKey {
			t.Fatalf("expected API key %q, got %q", apiKey, gotAPIKey)
		}
		return []string{"models/gemini-2.5-flash"}, nil
	})

	if err := model.CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one ListModels call, got %d", calls)
	}
}

func TestGoogleModelCheckConnectionRequiresAPIKey(t *testing.T) {
	model := &GoogleModel{}
	calls := 0

	withGoogleListModelsStub(t, func(context.Context, string) ([]string, error) {
		calls++
		return nil, nil
	})

	cases := []struct {
		name      string
		apiConfig *APIConfig
	}{
		{
			name:      "nil config",
			apiConfig: nil,
		},
		{
			name:      "nil api key",
			apiConfig: &APIConfig{},
		},
		{
			name: "empty api key",
			apiConfig: &APIConfig{
				ApiKey: stringPtr(""),
			},
		},
		{
			name: "blank api key",
			apiConfig: &APIConfig{
				ApiKey: stringPtr("  \t\n  "),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := model.CheckConnection(tc.apiConfig)
			if err == nil {
				t.Fatal("expected an API key error")
			}
			if !strings.Contains(err.Error(), "api key is required") {
				t.Fatalf("expected API key error, got %v", err)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("expected no ListModels calls without an API key, got %d", calls)
	}
}

func TestGoogleModelCheckConnectionReturnsListModelsError(t *testing.T) {
	model := &GoogleModel{}
	apiKey := "test-api-key"
	listErr := errors.New("list models failed")

	withGoogleListModelsStub(t, func(context.Context, string) ([]string, error) {
		return nil, listErr
	})

	err := model.CheckConnection(&APIConfig{ApiKey: &apiKey})
	if !errors.Is(err, listErr) {
		t.Fatalf("expected ListModels error %v, got %v", listErr, err)
	}
}

func TestCollectGoogleModelNamesPaginates(t *testing.T) {
	pages := []googleModelPage{
		{items: []string{"models/gemini-2.5-flash"}, nextPageToken: "page-2"},
		{items: []string{"models/gemini-2.5-pro"}, nextPageToken: ""},
	}
	var pageTokens []string

	models, err := collectGoogleModelNames(context.Background(), func(_ context.Context, pageToken string) (googleModelPage, error) {
		pageTokens = append(pageTokens, pageToken)
		if len(pageTokens) > len(pages) {
			t.Fatalf("unexpected extra page request with token %q", pageToken)
		}
		return pages[len(pageTokens)-1], nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expectedModels := []string{"models/gemini-2.5-flash", "models/gemini-2.5-pro"}
	if !reflect.DeepEqual(models, expectedModels) {
		t.Fatalf("expected models %v, got %v", expectedModels, models)
	}
	expectedPageTokens := []string{"", "page-2"}
	if !reflect.DeepEqual(pageTokens, expectedPageTokens) {
		t.Fatalf("expected page tokens %v, got %v", expectedPageTokens, pageTokens)
	}
}

func TestCollectGoogleModelNamesPreservesEmptyResult(t *testing.T) {
	models, err := collectGoogleModelNames(context.Background(), func(context.Context, string) (googleModelPage, error) {
		return googleModelPage{}, nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if models != nil {
		t.Fatalf("expected nil models, got %v", models)
	}
}

func TestCollectGoogleModelNamesReturnsPageError(t *testing.T) {
	pageErr := errors.New("next page failed")
	calls := 0

	models, err := collectGoogleModelNames(context.Background(), func(context.Context, string) (googleModelPage, error) {
		calls++
		if calls == 1 {
			return googleModelPage{items: []string{"models/gemini-2.5-flash"}, nextPageToken: "page-2"}, nil
		}
		return googleModelPage{}, pageErr
	})
	if !errors.Is(err, pageErr) {
		t.Fatalf("expected page error %v, got %v", pageErr, err)
	}
	if models != nil {
		t.Fatalf("expected no models on error, got %v", models)
	}
}

func TestGoogleVertexFactoryRoutesAliases(t *testing.T) {
	for _, providerName := range []string{"Google Vertex", "Google Cloud"} {
		t.Run(providerName, func(t *testing.T) {
			driver, err := NewModelFactory().CreateModelDriver(providerName, nil, URLSuffix{})
			if err != nil {
				t.Fatalf("CreateModelDriver: %v", err)
			}
			model, ok := driver.(*GoogleModel)
			if !ok {
				t.Fatalf("driver=%T, want *GoogleModel", driver)
			}
			if model.Backend != genai.BackendVertexAI {
				t.Fatalf("Backend=%v, want %v", model.Backend, genai.BackendVertexAI)
			}
			if model.Name() != "google vertex" {
				t.Fatalf("Name()=%q, want google vertex", model.Name())
			}
		})
	}
}

func TestGoogleVertexListModelsUsesVertexLister(t *testing.T) {
	baseURL := map[string]string{"default": "https://vertex.example"}
	model := NewGoogleVertexModel(baseURL, URLSuffix{})
	apiKey := `{"google_project_id":"test-project","google_region":"us-central1"}`
	expected := []string{"publishers/google/models/gemini-2.5-flash"}

	withGoogleVertexListModelsStub(t, func(_ context.Context, gotConfig *APIConfig, gotBaseURL map[string]string) ([]string, error) {
		if gotConfig == nil || gotConfig.ApiKey == nil || *gotConfig.ApiKey != apiKey {
			t.Fatalf("apiConfig.ApiKey=%v, want %q", gotConfig, apiKey)
		}
		if !reflect.DeepEqual(gotBaseURL, baseURL) {
			t.Fatalf("baseURL=%v, want %v", gotBaseURL, baseURL)
		}
		return expected, nil
	})

	models, err := model.ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if !reflect.DeepEqual(models, expected) {
		t.Fatalf("models=%v, want %v", models, expected)
	}
}

func TestGoogleVertexClientConfigPlainAPIKey(t *testing.T) {
	apiKey := "vertex-api-key"
	region := "us-central1"

	cfg, err := googleVertexClientConfig(&APIConfig{ApiKey: &apiKey, Region: &region}, nil)
	if err != nil {
		t.Fatalf("googleVertexClientConfig: %v", err)
	}
	if cfg.Backend != genai.BackendVertexAI {
		t.Fatalf("Backend=%v, want %v", cfg.Backend, genai.BackendVertexAI)
	}
	if cfg.APIKey != apiKey {
		t.Fatalf("APIKey=%q, want %q", cfg.APIKey, apiKey)
	}
	if cfg.Project != "" || cfg.Location != "" {
		t.Fatalf("Project=%q Location=%q, want empty in API key mode", cfg.Project, cfg.Location)
	}
}

func TestGoogleVertexClientConfigProjectAndRegion(t *testing.T) {
	key := `{"google_project_id":"test-project","google_region":"europe-west4"}`

	cfg, err := googleVertexClientConfig(&APIConfig{ApiKey: &key}, nil)
	if err != nil {
		t.Fatalf("googleVertexClientConfig: %v", err)
	}
	if cfg.Backend != genai.BackendVertexAI {
		t.Fatalf("Backend=%v, want %v", cfg.Backend, genai.BackendVertexAI)
	}
	if cfg.Project != "test-project" {
		t.Fatalf("Project=%q, want test-project", cfg.Project)
	}
	if cfg.Location != "europe-west4" {
		t.Fatalf("Location=%q, want europe-west4", cfg.Location)
	}
	if cfg.APIKey != "" {
		t.Fatalf("APIKey=%q, want empty", cfg.APIKey)
	}
}

func TestGoogleVertexClientConfigUsesRegionalBaseURL(t *testing.T) {
	region := "europe-west4"

	cfg, err := googleVertexClientConfig(&APIConfig{Region: &region}, map[string]string{
		"default":      "https://default.example/",
		"europe-west4": "https://region.example/",
	})
	if err != nil {
		t.Fatalf("googleVertexClientConfig: %v", err)
	}
	if cfg.HTTPOptions.BaseURL != "https://region.example/" {
		t.Fatalf("BaseURL=%q, want regional URL", cfg.HTTPOptions.BaseURL)
	}
}

func stringPtr(value string) *string {
	return &value
}
