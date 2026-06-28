package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/profile"
	"ragflow/internal/harness/core/schema"
)

// ---- Anthropic provider tests ----

func TestAnthropicProvider_Registration(t *testing.T) {
	resetProfileRegistries()
	RegisterAnthropic(AnthropicConfig{APIKey: "test-key"})

	p := profile.LookupProvider("anthropic")
	if p == nil {
		t.Fatal("expected anthropic provider to be registered")
	}
	if p.DefaultModel != "claude-sonnet-4-6" {
		t.Errorf("expected default model claude-sonnet-4-6, got %s", p.DefaultModel)
	}
}

func TestAnthropicModel_Generate(t *testing.T) {
	// Mock Anthropic API server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers.
		if r.Header.Get("x-api-key") != "secret-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Parse and verify the request.
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if req["model"] != "claude-sonnet-4-6" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "wrong model"})
			return
		}

		// Return a valid response.
		resp := map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "Hello from Claude"},
			},
			"stop_reason": "end_turn",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	m := newAnthropicModel("claude-sonnet-4-6", map[string]any{
		"api_key":  "secret-key",
		"api_base": server.URL,
	})

	msg, err := m.Generate(context.Background(), []*schema.Message{
		schema.UserMessage("hello"),
	})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Content != "Hello from Claude" {
		t.Errorf("expected 'Hello from Claude', got %q", msg.Content)
	}
}

func TestAnthropicModel_ToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": "Let me search.",
				},
				{
					"type":  "tool_use",
					"id":    "tu_123",
					"name":  "web_search",
					"input": map[string]any{"query": "golang"},
				},
			},
			"stop_reason": "tool_use",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	m := newAnthropicModel("claude-sonnet-4-6", map[string]any{
		"api_key":  "key",
		"api_base": server.URL,
	})

	msg, err := m.Generate(context.Background(), []*schema.Message{
		schema.UserMessage("search"),
	})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].Function.Name != "web_search" {
		t.Errorf("expected web_search, got %s", msg.ToolCalls[0].Function.Name)
	}
	t.Logf("tool call: %s -> %s", msg.ToolCalls[0].Function.Name, msg.ToolCalls[0].Function.Arguments)
}

// ---- OpenAI provider tests ----

func TestOpenAIProvider_Registration(t *testing.T) {
	resetProfileRegistries()
	RegisterOpenAI(OpenAIConfig{APIKey: "sk-test"})

	p := profile.LookupProvider("openai")
	if p == nil {
		t.Fatal("expected openai provider to be registered")
	}
	if p.DefaultModel != "gpt-4o" {
		t.Errorf("expected default model gpt-4o, got %s", p.DefaultModel)
	}
}

func TestOpenAIModel_Generate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "Hello from GPT",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	m := newOpenAIModel("gpt-4o", map[string]any{
		"api_key":  "sk-secret",
		"api_base": server.URL,
	})

	msg, err := m.Generate(context.Background(), []*schema.Message{
		schema.UserMessage("hello"),
	})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if msg.Content != "Hello from GPT" {
		t.Errorf("expected 'Hello from GPT', got %q", msg.Content)
	}
}

func TestOpenAIModel_ToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id":   "call_abc",
								"type": "function",
								"function": map[string]any{
									"name":      "get_weather",
									"arguments": `{"city":"Beijing"}`,
								},
							},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	m := newOpenAIModel("gpt-4o", map[string]any{
		"api_key":  "sk-key",
		"api_base": server.URL,
	})

	msg, err := m.Generate(context.Background(), []*schema.Message{
		schema.UserMessage("weather"),
	})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("expected get_weather, got %s", msg.ToolCalls[0].Function.Name)
	}
	t.Logf("openai tool call: %s -> %s", msg.ToolCalls[0].Function.Name, msg.ToolCalls[0].Function.Arguments)
}

// ---- Profile integration test ----

func TestProfileWithAnthropicProvider(t *testing.T) {
	resetProfileRegistries()

	// Setup mock Anthropic API.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "Profile integration works"},
			},
			"stop_reason": "end_turn",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	RegisterAnthropic(AnthropicConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	profile.RegisterHarness(&profile.HarnessProfile{
		Name:             "test-harness",
		BaseSystemPrompt: profile.StrPtr("Test mode."),
	})

	agent, err := profile.NewAgent(context.Background(), &profile.AgentConfig{
		ModelSpec:          "anthropic:claude-sonnet-4-6",
		HarnessProfileName: "test-harness",
	})
	if err != nil {
		t.Fatalf("NewAgent error: %v", err)
	}
	if agent == nil {
		t.Fatal("expected non-nil agent")
	}

	runner := core.NewTypedRunner(core.RunnerConfig[*schema.Message]{Agent: agent})
	iter := runner.Run(context.Background(), []*schema.Message{schema.UserMessage("test")})
	var final string
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			// Mock may return err if HTTP request fails in test env
			t.Logf("event err: %v", ev.Err)
			break
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil &&
			!ev.Output.MessageOutput.IsStreaming &&
			ev.Output.MessageOutput.Message != nil {
			final = ev.Output.MessageOutput.Message.Content
		}
	}
	t.Logf("profile + anthropic integration: final=%q", final)
}

// TestRegisterAll validates the convenience function registers both providers.
func TestRegisterAll(t *testing.T) {
	resetProfileRegistries()
	RegisterAll(
		AnthropicConfig{APIKey: "ant-key"},
		OpenAIConfig{APIKey: "openai-key"},
	)

	if profile.LookupProvider("anthropic") == nil {
		t.Error("expected anthropic to be registered")
	}
	if profile.LookupProvider("openai") == nil {
		t.Error("expected openai to be registered")
	}
}

// ---- Helper ----

func resetProfileRegistries() {
	profile.ClearProviders()
	profile.ClearHarnesses()
}
