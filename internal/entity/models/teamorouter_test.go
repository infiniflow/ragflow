package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
)

func newTeamoRouterForTest(baseURL string) *TeamoRouterModel {
	return NewTeamoRouterModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestTeamoRouterName(t *testing.T) {
	if got := newTeamoRouterForTest("http://unused").Name(); got != "teamorouter" {
		t.Errorf("Name()=%q", got)
	}
}

func TestTeamoRouterFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("teamorouter", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*TeamoRouterModel); !ok {
		t.Fatalf("driver type=%T, want *TeamoRouterModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*TeamoRouterModel); !ok {
		t.Fatal("NewInstance did not return *TeamoRouterModel")
	}
}

func TestTeamoRouterProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "teamorouter.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("TeamoRouter")
	if provider == nil {
		t.Fatal("TeamoRouter provider not found")
	}
	if _, ok := provider.ModelDriver.(*TeamoRouterModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.TeamoRouterModel", provider.ModelDriver)
	}
}

func TestTeamoRouterChatSendsAPIKey(t *testing.T) {
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
		if body.Model != "glm-5.3-flash" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-teamorouter",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newTeamoRouterForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"glm-5.3-flash",
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

func TestTeamoRouterListModelsAndCheckConnection(t *testing.T) {
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
				{"id": "kimi-k3.1-pro"},
				{"id": "glm-5.3-flash"},
			},
		})
	}))
	defer srv.Close()

	model := newTeamoRouterForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "kimi-k3.1-pro,glm-5.3-flash" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}
