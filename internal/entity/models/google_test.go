package models

import (
	"context"
	"encoding/json"
	"errors"
	"math"
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
	withSSRFBypass(t)
	ctx := t.Context()
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
			models, err := model.ListModels(ctx, tc.apiConfig)
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
	withSSRFBypass(t)
	ctx := t.Context()
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

	models, err := model.ListModels(ctx, &APIConfig{ApiKey: &configuredAPIKey})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !reflect.DeepEqual(models, expected) {
		t.Fatalf("expected models %v, got %v", expected, models)
	}
}

func TestGoogleModelCheckConnectionUsesListModels(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
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

	if err := model.CheckConnection(ctx, &APIConfig{ApiKey: &apiKey}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one ListModels call, got %d", calls)
	}
}

func TestGoogleModelCheckConnectionRequiresAPIKey(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
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
			err := model.CheckConnection(ctx, tc.apiConfig)
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
	withSSRFBypass(t)
	ctx := t.Context()
	model := &GoogleModel{}
	apiKey := "test-api-key"
	listErr := errors.New("list models failed")

	withGoogleListModelsStub(t, func(context.Context, *genai.ClientConfig) ([]ListModelResponse, error) {
		return nil, listErr
	})

	err := model.CheckConnection(ctx, &APIConfig{ApiKey: &apiKey})
	if !errors.Is(err, listErr) {
		t.Fatalf("expected ListModels error %v, got %v", listErr, err)
	}
}

func TestGoogleModelChatStreamlyRequiresAPIKey(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
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
			err := model.ChatStreamlyWithSender(ctx, "gemini-2.5-flash", messages, tc.apiConfig, nil, nil, func(*string, *string) error {
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
	withSSRFBypass(t)
	ctx := t.Context()
	model := &GoogleModel{}
	apiKey := "test-api-key"
	messages := []Message{{Role: "user", Content: "hello"}}

	response, err := model.ChatWithMessages(ctx, "", messages, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil {
		t.Fatal("expected a model name error")
	}
	if !strings.Contains(err.Error(), "model name is empty") {
		t.Fatalf("expected model name error, got %v", err)
	}
	if response != nil {
		t.Fatalf("expected no response, got %v", response)
	}

	err = model.ChatStreamlyWithSender(ctx, "", messages, &APIConfig{ApiKey: &apiKey}, nil, nil, func(*string, *string) error {
		t.Errorf("sender should not be called without a model name")
		return nil
	})
	if err == nil {
		t.Fatal("expected a model name error")
	}
	if !strings.Contains(err.Error(), "model name is empty") {
		t.Fatalf("expected model name error, got %v", err)
	}

	err = model.ChatStreamlyWithSender(ctx, "gemini-2.5-flash", messages, &APIConfig{ApiKey: &apiKey}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected a sender error")
	}
	if !strings.Contains(err.Error(), "sender is nil") {
		t.Fatalf("expected sender error, got %v", err)
	}
}

func TestGoogleModelChatRequiresConversationalMessage(t *testing.T) {
	ctx := t.Context()
	model := &GoogleModel{}
	apiKey := "test-api-key"
	messages := []Message{{Role: "system", Content: "You are a helpful assistant."}}

	response, err := model.ChatWithMessages(ctx, "gemini-2.5-flash", messages, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil {
		t.Fatal("expected an error for system-only messages")
	}
	if !strings.Contains(err.Error(), "no conversational message") {
		t.Fatalf("expected no-conversational-message error, got %v", err)
	}
	if response != nil {
		t.Fatalf("expected no response, got %v", response)
	}

	err = model.ChatStreamlyWithSender(ctx, "gemini-2.5-flash", messages, &APIConfig{ApiKey: &apiKey}, nil, nil, func(*string, *string) error {
		t.Errorf("sender should not be called for system-only messages")
		return nil
	})
	if err == nil {
		t.Fatal("expected an error for system-only messages")
	}
	if !strings.Contains(err.Error(), "no conversational message") {
		t.Fatalf("expected no-conversational-message error, got %v", err)
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
	withSSRFBypass(t)
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
			ctx := t.Context()
			model := NewGoogleModel(tc.baseURL, URLSuffix{})
			withGoogleListModelsStub(t, func(_ context.Context, config *genai.ClientConfig) ([]ListModelResponse, error) {
				if config.HTTPOptions.BaseURL != tc.expectedBaseURL {
					t.Fatalf("expected base URL %q, got %q", tc.expectedBaseURL, config.HTTPOptions.BaseURL)
				}
				return []ListModelResponse{{Name: "models/gemini-2.5-flash"}}, nil
			})

			if _, err := model.ListModels(ctx, &APIConfig{ApiKey: &apiKey, Region: tc.region}); err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestCollectGoogleModelNamesPaginates(t *testing.T) {
	pages := []googleModelPage{
		{items: []ModelListItem{{ID: "Gemini 2.5 Flash", OwnedBy: "Google"}}, nextPageToken: "page-2"},
		{items: []ModelListItem{{ID: "Gemini 2.5 Pro", OwnedBy: "Google"}}, nextPageToken: ""},
	}
	var pageTokens []string

	models, err := collectGoogleModelNames(t.Context(), func(_ context.Context, pageToken string) (googleModelPage, error) {
		pageTokens = append(pageTokens, pageToken)
		if len(pageTokens) > len(pages) {
			t.Fatalf("unexpected extra page request with token %q", pageToken)
		}
		return pages[len(pageTokens)-1], nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expectedModels := []ListModelResponse{
		{Name: "Gemini 2.5 Flash", ModelTypes: []string{"chat"}},
		{Name: "Gemini 2.5 Pro", ModelTypes: []string{"chat"}},
	}
	if !reflect.DeepEqual(models, expectedModels) {
		t.Fatalf("expected models %v, got %v", expectedModels, models)
	}
	expectedPageTokens := []string{"", "page-2"}
	if !reflect.DeepEqual(pageTokens, expectedPageTokens) {
		t.Fatalf("expected page tokens %v, got %v", expectedPageTokens, pageTokens)
	}
}

func TestCollectGoogleModelNamesPreservesEmptyResult(t *testing.T) {
	models, err := collectGoogleModelNames(t.Context(), func(context.Context, string) (googleModelPage, error) {
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

	_, err := collectGoogleModelNames(t.Context(), func(context.Context, string) (googleModelPage, error) {
		calls++
		if calls == 1 {
			return googleModelPage{items: []ModelListItem{{ID: "Gemini 2.5 Flash", OwnedBy: "Google"}}, nextPageToken: "page-2"}, nil
		}
		return googleModelPage{}, pageErr
	})
	if !errors.Is(err, pageErr) {
		t.Fatalf("expected page error %v, got %v", pageErr, err)
	}
}

func TestFinalizeGoogleModelListFiltersUnknownModelTypes(t *testing.T) {
	list := []ListModelResponse{
		{Name: "gemini-2.5-pro"},                                    // not in catalog: inferred
		{Name: "gemini-embedding-001"},                              // not in catalog: inferred
		{Name: "custom", ModelTypes: []string{"chat", "image-gen"}}, // unsupported value stripped
		{Name: "broken", ModelTypes: []string{"image-gen"}},         // no supported type: dropped
	}

	got := finalizeGoogleModelList(list)

	expected := []ListModelResponse{
		{Name: "gemini-2.5-pro", ModelTypes: []string{"chat"}},
		{Name: "gemini-embedding-001", ModelTypes: []string{"embedding"}},
		{Name: "custom", ModelTypes: []string{"chat"}},
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("expected models %v, got %v", expected, got)
	}
}

func TestFinalizeGoogleModelListPreservesNil(t *testing.T) {
	if got := finalizeGoogleModelList(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestGoogleSupportsUsableAction(t *testing.T) {
	usable := [][]string{
		{"generateContent", "countTokens"},
		{"embedContent"},
		{"batchEmbedContents"},
	}
	for _, actions := range usable {
		if !googleSupportsUsableAction(actions) {
			t.Fatalf("expected actions %v to be usable", actions)
		}
	}
	unusable := [][]string{
		nil,
		{"predict"},             // imagen-style image generation
		{"predictLongRunning"},  // veo-style video generation
		{"generateAnswer"},      // aqa-style question answering
		{"createCachedContent"}, // cache-only entry
	}
	for _, actions := range unusable {
		if googleSupportsUsableAction(actions) {
			t.Fatalf("expected actions %v to be filtered out", actions)
		}
	}
}

func TestGoogleGenerateContentConfigConvertsTools(t *testing.T) {
	toolChoice := "required"
	cfg, err := googleGenerateContentConfig(&ChatConfig{
		Tools: []map[string]interface{}{{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "search_my_dataset",
				"description": "Search dataset.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{"type": "string"},
					},
					"required": []string{"query"},
				},
			},
		}},
		ToolChoice: &toolChoice,
	}, nil)
	if err != nil {
		t.Fatalf("googleGenerateContentConfig error = %v", err)
	}
	if cfg == nil || len(cfg.Tools) != 1 || len(cfg.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("tools = %#v, want one function declaration", cfg)
	}
	declaration := cfg.Tools[0].FunctionDeclarations[0]
	if declaration.Name != "search_my_dataset" || declaration.Description != "Search dataset." {
		t.Fatalf("declaration = %#v", declaration)
	}
	if declaration.ParametersJsonSchema == nil {
		t.Fatal("ParametersJsonSchema is nil")
	}
	if cfg.ToolConfig == nil || cfg.ToolConfig.FunctionCallingConfig == nil {
		t.Fatalf("ToolConfig = %#v", cfg.ToolConfig)
	}
	if cfg.ToolConfig.FunctionCallingConfig.Mode != genai.FunctionCallingConfigModeAny {
		t.Fatalf("mode = %s, want ANY", cfg.ToolConfig.FunctionCallingConfig.Mode)
	}
}

func TestGoogleGenerateContentConfigRejectsMaxTokensOverflow(t *testing.T) {
	overflow := int(math.MaxInt32) + 1
	cfg, err := googleGenerateContentConfig(&ChatConfig{MaxTokens: &overflow}, nil)
	if err == nil {
		t.Fatalf("expected an error for max_tokens overflowing int32, got cfg = %#v", cfg)
	}
	if cfg != nil {
		t.Fatalf("cfg = %#v, want nil on error", cfg)
	}

	maxInt32 := math.MaxInt32
	cfg, err = googleGenerateContentConfig(&ChatConfig{MaxTokens: &maxInt32}, nil)
	if err != nil {
		t.Fatalf("googleGenerateContentConfig error = %v", err)
	}
	if cfg == nil || cfg.MaxOutputTokens != math.MaxInt32 {
		t.Fatalf("cfg.MaxOutputTokens = %#v, want %d", cfg, int32(math.MaxInt32))
	}
}

func TestGoogleChatContentsConvertsToolHistory(t *testing.T) {
	contents := googleChatContents([]Message{
		{
			Role:    "assistant",
			Content: nil,
			ToolCalls: []map[string]interface{}{{
				"id":   "call-1",
				"type": "function",
				"function": map[string]interface{}{
					"name":      "search_my_dataset",
					"arguments": `{"query":"marigold"}`,
				},
			}},
		},
		{Role: "tool", ToolCallID: "call-1", Content: "flower result"},
	})
	if len(contents) != 2 {
		t.Fatalf("contents len = %d, want 2", len(contents))
	}
	functionCall := contents[0].Parts[0].FunctionCall
	if functionCall == nil || functionCall.ID != "call-1" || functionCall.Name != "search_my_dataset" {
		t.Fatalf("function call = %#v", functionCall)
	}
	if functionCall.Args["query"] != "marigold" {
		t.Fatalf("args = %#v", functionCall.Args)
	}
	functionResponse := contents[1].Parts[0].FunctionResponse
	if functionResponse == nil || functionResponse.ID != "call-1" || functionResponse.Name != "search_my_dataset" {
		t.Fatalf("function response = %#v", functionResponse)
	}
	if functionResponse.Response["output"] != "flower result" {
		t.Fatalf("response = %#v", functionResponse.Response)
	}
}

func TestGoogleSystemInstructionExtractedFromMessages(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hello"},
		{Role: "system", Content: "Always answer in pirate speak."},
	}

	contents := googleChatContents(messages)
	if len(contents) != 1 {
		t.Fatalf("contents len = %d, want 1 (both system messages must be excluded)", len(contents))
	}
	if contents[0].Role != genai.RoleUser {
		t.Fatalf("contents[0].Role = %s, want user", contents[0].Role)
	}

	systemInstruction, err := googleSystemInstruction(messages)
	if err != nil {
		t.Fatalf("googleSystemInstruction error = %v", err)
	}
	if systemInstruction == nil || len(systemInstruction.Parts) != 2 {
		t.Fatalf("systemInstruction = %#v, want two parts", systemInstruction)
	}
	if systemInstruction.Parts[0].Text != "You are a helpful assistant." {
		t.Fatalf("systemInstruction first part text = %q", systemInstruction.Parts[0].Text)
	}
	if systemInstruction.Parts[1].Text != "Always answer in pirate speak." {
		t.Fatalf("systemInstruction second part text = %q", systemInstruction.Parts[1].Text)
	}

	cfg, err := googleGenerateContentConfig(nil, systemInstruction)
	if err != nil {
		t.Fatalf("googleGenerateContentConfig error = %v", err)
	}
	if cfg == nil || cfg.SystemInstruction != systemInstruction {
		t.Fatalf("cfg.SystemInstruction = %#v, want %#v", cfg, systemInstruction)
	}
}

func TestGoogleSystemInstructionNilWhenNoSystemMessage(t *testing.T) {
	got, err := googleSystemInstruction([]Message{{Role: "user", Content: "Hello"}})
	if err != nil {
		t.Fatalf("googleSystemInstruction error = %v", err)
	}
	if got != nil {
		t.Fatalf("systemInstruction = %#v, want nil", got)
	}
	cfg, err := googleGenerateContentConfig(nil, nil)
	if err != nil {
		t.Fatalf("googleGenerateContentConfig error = %v", err)
	}
	if cfg != nil {
		t.Fatalf("cfg = %#v, want nil", cfg)
	}
}

func TestGoogleSystemInstructionRejectsImageContent(t *testing.T) {
	messages := []Message{
		{
			Role: "system",
			Content: []interface{}{
				map[string]interface{}{
					"type":      "image_url",
					"image_url": map[string]interface{}{"url": "https://example.com/cat.png"},
				},
			},
		},
		{Role: "user", Content: "Hello"},
	}

	systemInstruction, err := googleSystemInstruction(messages)
	if err == nil {
		t.Fatalf("googleSystemInstruction error = nil, want error for image content in system message")
	}
	if systemInstruction != nil {
		t.Fatalf("systemInstruction = %#v, want nil on error", systemInstruction)
	}
}

func TestGoogleToolCallsConvertsFunctionCalls(t *testing.T) {
	toolCalls := googleToolCalls([]*genai.FunctionCall{{
		ID:   "call-1",
		Name: "search_my_dataset",
		Args: map[string]any{"query": "marigold"},
	}})
	if len(toolCalls) != 1 {
		t.Fatalf("tool calls len = %d, want 1", len(toolCalls))
	}
	if toolCalls[0]["id"] != "call-1" || toolCalls[0]["type"] != "function" {
		t.Fatalf("tool call = %#v", toolCalls[0])
	}
	function, _ := toolCalls[0]["function"].(map[string]interface{})
	if function["name"] != "search_my_dataset" {
		t.Fatalf("function = %#v", function)
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(function["arguments"].(string)), &args); err != nil {
		t.Fatalf("arguments JSON: %v", err)
	}
	if args["query"] != "marigold" {
		t.Fatalf("arguments = %#v", args)
	}
}

// TestGoogleUsageFromMetadataIncludesToolUsePromptTokens verifies that
// ToolUsePromptTokenCount from the genai SDK is folded into
// PromptTokens, that it is treated as non-zero by the presence check
// (so the helper does not return nil), and that TotalTokenCount is
// used as the authoritative total when it is present.
func TestGoogleUsageFromMetadataIncludesToolUsePromptTokens(t *testing.T) {
	m := &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:        10,
		CandidatesTokenCount:    4,
		ToolUsePromptTokenCount: 5,
		ThoughtsTokenCount:      1,
		TotalTokenCount:         20,
	}
	got := googleUsageFromMetadata(m)
	if got == nil {
		t.Fatal("googleUsageFromMetadata returned nil, want populated TokenUsage")
	}
	if got.PromptTokens != 15 {
		t.Errorf("PromptTokens=%d, want 15 (10 prompt + 5 tool-use prompt)", got.PromptTokens)
	}
	if got.CompletionTokens != 5 {
		t.Errorf("CompletionTokens=%d, want 5 (4 candidates + 1 thoughts)", got.CompletionTokens)
	}
	if got.TotalTokens != 20 {
		t.Errorf("TotalTokens=%d, want 20 (SDK authoritative total)", got.TotalTokens)
	}

	// When TotalTokenCount is absent, the helper must fall back to
	// prompt + completion so callers still get a consistent total.
	m.TotalTokenCount = 0
	got = googleUsageFromMetadata(m)
	if got == nil {
		t.Fatal("googleUsageFromMetadata returned nil for non-zero counts")
	}
	if got.TotalTokens != 20 {
		t.Errorf("TotalTokens=%d, want 20 (15 prompt + 5 completion)", got.TotalTokens)
	}
}

func stringPtr(value string) *string {
	return &value
}
