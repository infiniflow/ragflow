package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
)

func newSarvamForTest(baseURL string) *SarvamModel {
	return NewSarvamModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestSarvamName(t *testing.T) {
	if got := newSarvamForTest("http://unused").Name(); got != "sarvam" {
		t.Errorf("Name()=%q", got)
	}
}

func TestSarvamFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("sarvam", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*SarvamModel); !ok {
		t.Fatalf("driver type=%T, want *SarvamModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*SarvamModel); !ok {
		t.Fatal("NewInstance did not return *SarvamModel")
	}
}

func TestSarvamProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "sarvam.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("Sarvam")
	if provider == nil {
		t.Fatal("Sarvam provider not found")
	}
	if _, ok := provider.ModelDriver.(*SarvamModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.SarvamModel", provider.ModelDriver)
	}
}

func TestSarvamV2Endpoint(t *testing.T) {
	model := newSarvamForTest("https://api.sarvam.ai/v2")

	url, err := model.baseModel.EndpointURL(&APIConfig{ApiKey: &testAPIKey}, "chat/completions")
	if err != nil {
		t.Fatalf("EndpointURL: %v", err)
	}
	if url != "https://api.sarvam.ai/v2/chat/completions" {
		t.Errorf("url=%q", url)
	}
}

func TestSarvamChatSendsAPIKey(t *testing.T) {
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
		if body.Model != "sarvam-105b" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-sarvam",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newSarvamForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"sarvam-105b",
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

func TestSarvamListModelsAndCheckConnection(t *testing.T) {
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
				{"id": "sarvam-105b"},
				{"id": "glm5.3"},
			},
		})
	}))
	defer srv.Close()

	model := newSarvamForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "sarvam-105b,glm5.3" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}
