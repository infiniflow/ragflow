package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
)

func newStraitlyForTest(baseURL string) *StraitlyModel {
	return NewStraitlyModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestStraitlyName(t *testing.T) {
	if got := newStraitlyForTest("http://unused").Name(); got != "straitly" {
		t.Errorf("Name()=%q", got)
	}
}

func TestStraitlyFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("straitly", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*StraitlyModel); !ok {
		t.Fatalf("driver type=%T, want *StraitlyModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*StraitlyModel); !ok {
		t.Fatal("NewInstance did not return *StraitlyModel")
	}
}

func TestStraitlyProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "straitly.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("Straitly")
	if provider == nil {
		t.Fatal("Straitly provider not found")
	}
	if _, ok := provider.ModelDriver.(*StraitlyModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.StraitlyModel", provider.ModelDriver)
	}
}

func TestStraitlyChatSendsAPIKey(t *testing.T) {
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
		if body.Model != "claude-sonnet-5-5" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-straitly",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newStraitlyForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"claude-sonnet-5-5",
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

func TestStraitlyListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-gw-test" {
			t.Errorf("Authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "claude-sonnet-5-5"},
				{"id": "gpt-6.2-astra"},
			},
		})
	}))
	defer srv.Close()

	model := newStraitlyForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "claude-sonnet-5-5,gpt-6.2-astra" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}
