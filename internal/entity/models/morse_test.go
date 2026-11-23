package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
)

func newMorseForTest(baseURL string) *MorseModel {
	return NewMorseModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestMorseName(t *testing.T) {
	if got := newMorseForTest("http://unused").Name(); got != "morse" {
		t.Errorf("Name()=%q", got)
	}
}

func TestMorseFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("morse", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*MorseModel); !ok {
		t.Fatalf("driver type=%T, want *MorseModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*MorseModel); !ok {
		t.Fatal("NewInstance did not return *MorseModel")
	}
}

func TestMorseProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "morse.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("Morse")
	if provider == nil {
		t.Fatal("Morse provider not found")
	}
	if _, ok := provider.ModelDriver.(*MorseModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.MorseModel", provider.ModelDriver)
	}
}

func TestMorseChatSendsAPIKey(t *testing.T) {
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
		if body.Model != "z-ai/glm-5.3-flash" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-morse",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newMorseForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"z-ai/glm-5.3-flash",
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

func TestMorseListModelsAndCheckConnection(t *testing.T) {
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
				{"id": "moonshotai/kimi-k3.1-pro"},
				{"id": "z-ai/glm-5.3-flash"},
			},
		})
	}))
	defer srv.Close()

	model := newMorseForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "moonshotai/kimi-k3.1-pro,z-ai/glm-5.3-flash" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}
