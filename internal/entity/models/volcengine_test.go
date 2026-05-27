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

func newVolcEngineServer(t *testing.T, handler func(t *testing.T, r *http.Request, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(t, r, w)
	}))
}

func newVolcEngineForTest(baseURL string) *VolcEngine {
	return NewVolcEngine(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:      "chat/completions",
			Files:     "files",
			Embedding: "embeddings/multimodal",
			Models:    "/models",
		},
	)
}

func TestVolcEngineConfigDeclaresModelsSuffix(t *testing.T) {
	var provider struct {
		URLSuffix URLSuffix `json:"url_suffix"`
	}

	for _, candidate := range []string{
		filepath.Join("..", "..", "..", "conf", "models", "volcengine.json"),
		filepath.Join("conf", "models", "volcengine.json"),
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

	t.Fatal("could not locate conf/models/volcengine.json")
}

func TestVolcEngineListModelsHappyPath(t *testing.T) {
	srv := newVolcEngineServer(t, func(t *testing.T, r *http.Request, w http.ResponseWriter) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s", r.Method)
		}
		if r.URL.Path != "/models" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object": "list",
			"data": [
				{"id": "doubao-seed-2-0-pro-260215", "owned_by": "volcengine"},
				{"id": "doubao-embedding-vision-251215"}
			]
		}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	models, err := newVolcEngineForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if strings.Join(models, ",") != "doubao-seed-2-0-pro-260215@volcengine,doubao-embedding-vision-251215" {
		t.Errorf("models=%v", models)
	}
}

func TestVolcEngineListModelsRejectsProviderError(t *testing.T) {
	srv := newVolcEngineServer(t, func(t *testing.T, r *http.Request, w http.ResponseWriter) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newVolcEngineForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "401 Unauthorized") || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("expected provider error with status and body, got %v", err)
	}
}

func TestVolcEngineListModelsRequiresModelsSuffix(t *testing.T) {
	apiKey := "test-key"
	model := NewVolcEngine(map[string]string{"default": "http://unused"}, URLSuffix{})

	_, err := model.ListModels(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "models URL suffix is not configured") {
		t.Fatalf("expected missing models suffix error, got %v", err)
	}
}
