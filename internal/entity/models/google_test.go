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

func withGoogleListModelsStub(t *testing.T, fn func(context.Context, *genai.ClientConfig) ([]ListModelResponse, error)) {
	t.Helper()

	googleListModelsMu.Lock()
	original := googleListModels
	googleListModels = fn
	t.Cleanup(func() {
		googleListModels = original
		googleListModelsMu.Unlock()
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
	withGoogleListModelsStub(t, func(context.Context, *genai.ClientConfig) ([]ListModelResponse, error) {
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
	configuredAPIKey := "  " + apiKey + "  "
	expected := []ListModelResponse{{Name: "models/gemini-2.5-flash"}, {Name: "models/gemini-2.5-pro"}}

	withGoogleListModelsStub(t, func(_ context.Context, config *genai.ClientConfig) ([]ListModelResponse, error) {
		if config.APIKey != apiKey {
			t.Fatalf("expected API key %q, got %q", apiKey, config.APIKey)
		}
		return expected, nil
	})

	models, err := model.ListModels(&APIConfig{ApiKey: &configuredAPIKey})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !reflect.DeepEqual(models, expected) {
		t.Fatalf("expected models %v, got %v", expected, models)
	}
}

func TestGoogleModelCheckConnectionUsesListModels(t *testing.T) {
	customBaseURL := "https://check-connection.example.test/google"
	model := NewGoogleModel(map[string]string{"default": customBaseURL}, URLSuffix{})
	apiKey := "test-api-key"
	calls := 0

	withGoogleListModelsStub(t, func(_ context.Context, config *genai.ClientConfig) ([]ListModelResponse, error) {
		calls++
		if config.APIKey != apiKey {
			t.Fatalf("expected API key %q, got %q", apiKey, config.APIKey)
		}
		if config.HTTPOptions.BaseURL != customBaseURL {
			t.Fatalf("expected base URL %q, got %q", customBaseURL, config.HTTPOptions.BaseURL)
		}
		return []ListModelResponse{{Name: "models/gemini-2.5-flash"}}, nil
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

	withGoogleListModelsStub(t, func(context.Context, *genai.ClientConfig) ([]ListModelResponse, error) {
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

	withGoogleListModelsStub(t, func(context.Context, *genai.ClientConfig) ([]ListModelResponse, error) {
		return nil, listErr
	})

	err := model.CheckConnection(&APIConfig{ApiKey: &apiKey})
	if !errors.Is(err, listErr) {
		t.Fatalf("expected ListModels error %v, got %v", listErr, err)
	}
}

func TestGoogleModelChatStreamlyRequiresAPIKey(t *testing.T) {
	model := &GoogleModel{}
	messages := []Message{{Role: "user", Content: "hello"}}
	cases := []struct {
		name      string
		apiConfig *APIConfig
	}{
		{name: "nil config"},
		{name: "nil api key", apiConfig: &APIConfig{}},
		{name: "empty api key", apiConfig: &APIConfig{ApiKey: stringPtr("")}},
		{name: "blank api key", apiConfig: &APIConfig{ApiKey: stringPtr("  \t\n  ")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := model.ChatStreamlyWithSender("gemini-2.5-flash", messages, tc.apiConfig, nil, func(*string, *string) error {
				t.Errorf("sender should not be called without an API key")
				return nil
			})
			if err == nil {
				t.Fatal("expected an API key error")
			}
			if !strings.Contains(err.Error(), "api key is required") {
				t.Fatalf("expected API key error, got %v", err)
			}
		})
	}
}

func TestGoogleModelChatRequiresModelName(t *testing.T) {
	model := &GoogleModel{}
	apiKey := "test-api-key"
	messages := []Message{{Role: "user", Content: "hello"}}

	response, err := model.ChatWithMessages("", messages, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil {
		t.Fatal("expected a model name error")
	}
	if !strings.Contains(err.Error(), "model name is empty") {
		t.Fatalf("expected model name error, got %v", err)
	}
	if response != nil {
		t.Fatalf("expected no response, got %v", response)
	}

	err = model.ChatStreamlyWithSender("", messages, &APIConfig{ApiKey: &apiKey}, nil, func(*string, *string) error {
		t.Errorf("sender should not be called without a model name")
		return nil
	})
	if err == nil {
		t.Fatal("expected a model name error")
	}
	if !strings.Contains(err.Error(), "model name is empty") {
		t.Fatalf("expected model name error, got %v", err)
	}

	err = model.ChatStreamlyWithSender("gemini-2.5-flash", messages, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil {
		t.Fatal("expected a sender error")
	}
	if !strings.Contains(err.Error(), "sender is nil") {
		t.Fatalf("expected sender error, got %v", err)
	}
}

func TestGoogleModelNewInstancePreservesCustomBaseURL(t *testing.T) {
	model := NewGoogleModel(map[string]string{"default": "https://generativelanguage.googleapis.com"}, URLSuffix{Models: "v1beta/models"})
	customBaseURL := map[string]string{"default": "https://example.test/google"}

	instance := model.NewInstance(customBaseURL)
	google, ok := instance.(*GoogleModel)
	if !ok {
		t.Fatalf("expected *GoogleModel, got %T", instance)
	}
	if google.baseModel.BaseURL["default"] != customBaseURL["default"] {
		t.Fatalf("expected base URL %q, got %q", customBaseURL["default"], google.baseModel.BaseURL["default"])
	}
	if google.baseModel.URLSuffix != model.baseModel.URLSuffix {
		t.Fatalf("expected URL suffix %v, got %v", model.baseModel.URLSuffix, google.baseModel.URLSuffix)
	}
}

func TestGoogleModelListModelsPassesBaseURL(t *testing.T) {
	apiKey := "test-api-key"
	cases := []struct {
		name            string
		baseURL         map[string]string
		region          *string
		expectedBaseURL string
	}{
		{
			name:            "default custom base URL",
			baseURL:         map[string]string{"default": "https://example.test/google"},
			expectedBaseURL: "https://example.test/google",
		},
		{
			name:            "regional custom base URL",
			baseURL:         map[string]string{"east": "https://east.example.test/google", "default": "https://default.example.test/google"},
			region:          stringPtr("east"),
			expectedBaseURL: "https://east.example.test/google",
		},
		{
			name:            "empty region custom base URL",
			baseURL:         map[string]string{"": "https://empty-region.example.test/google"},
			region:          stringPtr(""),
			expectedBaseURL: "https://empty-region.example.test/google",
		},
		{
			name:            "missing region falls back to default base URL",
			baseURL:         map[string]string{"default": "https://default.example.test/google"},
			region:          stringPtr("missing"),
			expectedBaseURL: "https://default.example.test/google",
		},
		{
			name: "SDK default base URL",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := NewGoogleModel(tc.baseURL, URLSuffix{})
			withGoogleListModelsStub(t, func(_ context.Context, config *genai.ClientConfig) ([]ListModelResponse, error) {
				if config.HTTPOptions.BaseURL != tc.expectedBaseURL {
					t.Fatalf("expected base URL %q, got %q", tc.expectedBaseURL, config.HTTPOptions.BaseURL)
				}
				return []ListModelResponse{{Name: "models/gemini-2.5-flash"}}, nil
			})

			if _, err := model.ListModels(&APIConfig{ApiKey: &apiKey, Region: tc.region}); err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestCollectGoogleModelNamesPaginates(t *testing.T) {
	pages := []googleModelPage{
		{items: []DSModel{{ID: "Gemini 2.5 Flash", OwnedBy: "Google"}}, nextPageToken: "page-2"},
		{items: []DSModel{{ID: "Gemini 2.5 Pro", OwnedBy: "Google"}}, nextPageToken: ""},
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

	expectedModels := []ListModelResponse{{Name: "Gemini 2.5 Flash@Google"}, {Name: "Gemini 2.5 Pro@Google"}}
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
			return googleModelPage{items: []DSModel{{ID: "Gemini 2.5 Flash", OwnedBy: "Google"}}, nextPageToken: "page-2"}, nil
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

func stringPtr(value string) *string {
	return &value
}
