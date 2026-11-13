package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
)

var anonRouterTestAPIKey = "sk-anon-test"

func newAnonRouterForTest(baseURL string) *AnonRouterModel {
	return NewAnonRouterModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestAnonRouterName(t *testing.T) {
	if got := newAnonRouterForTest("http://unused").Name(); got != "anonrouter" {
		t.Errorf("Name()=%q", got)
	}
}

func TestAnonRouterFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("anonrouter", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*AnonRouterModel); !ok {
		t.Fatalf("driver type=%T, want *AnonRouterModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*AnonRouterModel); !ok {
		t.Fatal("NewInstance did not return *AnonRouterModel")
	}
}

// AnonRouter is a hosted gateway, so unlike a local server it must receive the
// tenant's key as a bearer token on every call.
func TestAnonRouterChatSendsAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-anon-test" {
			t.Errorf("Authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-anonrouter",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newAnonRouterForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"anthropic/claude-sonnet-5",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &anonRouterTestAPIKey},
		nil,
		usage,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if *resp.Answer != "pong" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
	assertModelUsage(t, usage, 3, 5, 8)
}

func TestAnonRouterListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-anon-test" {
			t.Errorf("Authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "anthropic/claude-sonnet-5"},
				{"id": "openai/gpt-6-astra"},
			},
		})
	}))
	defer srv.Close()

	model := newAnonRouterForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &anonRouterTestAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "anthropic/claude-sonnet-5,openai/gpt-6-astra" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &anonRouterTestAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

// The shipped config must resolve to the AnonRouter driver and the pinned
// gateway endpoint.
func TestAnonRouterProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "anonrouter.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("AnonRouter")
	if provider == nil {
		t.Fatal("AnonRouter provider not found")
	}
	if _, ok := provider.ModelDriver.(*AnonRouterModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.AnonRouterModel", provider.ModelDriver)
	}
	if got := provider.URL["default"]; got != "https://api.anonrouter.ai/v1" {
		t.Errorf("default URL=%q", got)
	}
	if provider.URLSuffix.Chat != "chat/completions" || provider.URLSuffix.Models != "models" {
		t.Errorf("URLSuffix=%+v", provider.URLSuffix)
	}
	if len(provider.Models) == 0 {
		t.Fatal("no models declared")
	}

	// AnonRouter relays text-generation models only; the multimodal ones carry
	// the extra "vision" type, and every entry must declare both token limits
	// and tool support.
	vision := map[string]bool{}
	for _, model := range provider.Models {
		switch {
		case len(model.ModelTypes) == 1 && model.ModelTypes[0] == "chat":
		case len(model.ModelTypes) == 2 && model.ModelTypes[0] == "chat" && model.ModelTypes[1] == "vision":
			vision[model.Name] = true
		default:
			t.Errorf("%s model_types=%v, want [chat] or [chat vision]", model.Name, model.ModelTypes)
		}
		if model.ContextLength == nil || model.MaxOutput == nil {
			t.Errorf("%s missing context_length/max_output", model.Name)
		} else if *model.MaxOutput > *model.ContextLength {
			t.Errorf("%s max_output %d exceeds context_length %d", model.Name, *model.MaxOutput, *model.ContextLength)
		}
		if model.Tools == nil || !model.Tools.Support {
			t.Errorf("%s does not declare tool support", model.Name)
		}
		if model.Thinking != nil {
			t.Errorf("%s declares thinking metadata; AnonRouter reasoning is not on the thinking wire", model.Name)
		}
	}

	// Guard both sides of the split against a catalogue edit that drops the
	// vision type wholesale or applies it to every entry.
	for _, name := range []string{"anthropic/claude-sonnet-5", "google/gemini-3.8-flash", "openai/gpt-6-astra"} {
		if !vision[name] {
			t.Errorf("%s should declare vision", name)
		}
	}
	for _, name := range []string{"deepseek/deepseek-v4-pro", "openai/gpt-oss-120b", "tencent/hy3"} {
		if vision[name] {
			t.Errorf("%s should not declare vision", name)
		}
	}
}
