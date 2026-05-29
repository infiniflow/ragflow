package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newXAIForTest(baseURL string) *XAIModel {
	return NewXAIModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "/models"},
	)
}

func TestXAIConfigDeclaresModelsSuffix(t *testing.T) {
	var provider struct {
		URLSuffix URLSuffix `json:"url_suffix"`
	}

	for _, candidate := range []string{
		filepath.Join("..", "..", "..", "conf", "models", "xai.json"),
		filepath.Join("conf", "models", "xai.json"),
	} {
		data, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(data, &provider); err != nil {
			t.Fatalf("unmarshal %s: %v", candidate, err)
		}
		if provider.URLSuffix.Models != "models" {
			t.Fatalf("models suffix=%q, want models", provider.URLSuffix.Models)
		}
		return
	}

	t.Fatal("could not locate conf/models/xai.json")
}

func TestXAIListModelsHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		if r.URL.Path != "/models" {
			t.Errorf("path=%s, want /models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q, want Bearer test-key", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type=%q, want application/json", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object": "list",
			"data": [
				{"id": "grok-4"},
				{"id": "grok-3-mini"}
			]
		}`))
	}))
	defer srv.Close()

	apiKey := "test-key"
	models, err := newXAIForTest(srv.URL + "/").ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if strings.Join(models, ",") != "grok-4,grok-3-mini" {
		t.Fatalf("models=%v", models)
	}
}

func TestXAIListModelsRequiresAPIKey(t *testing.T) {
	_, err := newXAIForTest("http://unused").ListModels(&APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Fatalf("expected api key error, got %v", err)
	}
}

func TestXAIListModelsRejectsProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	defer srv.Close()

	apiKey := "test-key"
	_, err := newXAIForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("expected provider error with status and body, got %v", err)
	}
}

func TestXAICheckConnectionDelegatesToListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path=%s, want /models", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"grok-4"}]}`))
	}))
	defer srv.Close()

	apiKey := "test-key"
	if err := newXAIForTest(srv.URL).CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestXAIListModelsRequiresModelsSuffix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("ListModels should reject a missing models suffix before sending a request")
	}))
	defer srv.Close()

	apiKey := "test-key"
	model := NewXAIModel(map[string]string{"default": srv.URL}, URLSuffix{})

	_, err := model.ListModels(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "models URL suffix is not configured") {
		t.Fatalf("expected missing models suffix error, got %v", err)
	}
}
