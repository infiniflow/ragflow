package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
)

func newAIGROWForTest(baseURL string) *AIGROWModel {
	return NewAIGROWModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestAIGROWName(t *testing.T) {
	if got := newAIGROWForTest("http://unused").Name(); got != "aigrow" {
		t.Errorf("Name()=%q", got)
	}
}

func TestAIGROWFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("aigrow", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*AIGROWModel); !ok {
		t.Fatalf("driver type=%T, want *AIGROWModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*AIGROWModel); !ok {
		t.Fatal("NewInstance did not return *AIGROWModel")
	}
}

func TestAIGROWProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "aigrow.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("AIGROW")
	if provider == nil {
		t.Fatal("AIGROW provider not found")
	}
	if _, ok := provider.ModelDriver.(*AIGROWModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.AIGROWModel", provider.ModelDriver)
	}
}

func TestAIGROWChatSendsAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-gw-test" {
			t.Errorf("Authorization=%q", got)
		}
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Model != "claude-haiku-5.5" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-aigrow",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newAIGROWForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"claude-haiku-5.5",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &testAPIKey},
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

// The ids are the endpoint's own, unprefixed: each is listed as a chat model.
func TestAIGROWListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-gw-test" {
			t.Errorf("Authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "claude-opus-5.5", "object": "model", "owned_by": "aigrow"},
				{"id": "gpt-6.2-sol", "object": "model", "owned_by": "aigrow"},
				{"id": "gemini-4-neon", "object": "model", "owned_by": "aigrow"},
				{"id": "qwen3.8-max", "object": "model", "owned_by": "aigrow"},
			},
		})
	}))
	defer srv.Close()

	model := newAIGROWForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if got := joinModelNames(list, ","); got != "claude-opus-5.5,gpt-6.2-sol,gemini-4-neon,qwen3.8-max" {
		t.Errorf("models=%q", got)
	}
	for _, m := range list {
		if len(m.ModelTypes) == 0 || m.ModelTypes[0] != modelTypeChat {
			t.Errorf("%s types=%v, want chat", m.Name, m.ModelTypes)
		}
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}
