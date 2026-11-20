package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
)

func TestCheckConnectionOllamaModelTypes(t *testing.T) {
	t.Setenv(common.EnvAllowAnyHost, "true")
	for _, tc := range []struct {
		name       string
		model      string
		modelTypes []string
		path       string
		status     int
		wantCode   common.ErrorCode
		wantError  string
	}{
		{name: "unsupported decision", model: "llama3.2", modelTypes: []string{"decision"}, wantCode: common.CodeBadRequest, wantError: "does not support decision models"},
		{name: "unsupported decision with chat", model: "llama3.2", modelTypes: []string{"chat", "decision"}, wantCode: common.CodeBadRequest, wantError: "does not support decision models"},
		{name: "chat", model: "llama3.2", modelTypes: []string{"chat"}, path: "/api/chat", wantCode: common.CodeSuccess},
		{name: "inferred chat", model: "llama3.2", path: "/api/chat", wantCode: common.CodeSuccess},
		{name: "vision", model: "llava", modelTypes: []string{"vision"}, path: "/api/chat", wantCode: common.CodeSuccess},
		{name: "embedding", model: "nomic-embed-text", modelTypes: []string{"embedding"}, path: "/api/embed", wantCode: common.CodeSuccess},
		{name: "chat endpoint failure", model: "llama3.2", modelTypes: []string{"chat"}, path: "/api/chat", status: http.StatusInternalServerError, wantCode: common.CodeServerError, wantError: "model unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPost || r.URL.Path != tc.path {
					t.Errorf("request = %s %s, want POST %s", r.Method, r.URL.Path, tc.path)
				}
				var body struct {
					Model string `json:"model"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				if body.Model != tc.model {
					t.Errorf("model = %q, want %q", body.Model, tc.model)
				}
				w.Header().Set("Content-Type", "application/json")
				if tc.status != 0 {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(`{"error":"model unavailable"}`))
				} else if tc.path == "/api/embed" {
					_, _ = w.Write([]byte(`{"embeddings":[[0.1,0.2]]}`))
				} else {
					_, _ = w.Write([]byte(`{"message":{"content":"Hello"},"done":true}`))
				}
			}))
			defer srv.Close()

			code, err := NewModelProviderService().CheckConnection(t.Context(), "Ollama", CheckConnectionRequest{
				BaseURL: srv.URL,
				ModelInfo: []CheckConnectionModelInfo{{
					ModelName: tc.model, ModelTypes: tc.modelTypes,
				}},
			})
			if code != tc.wantCode {
				t.Fatalf("CheckConnection code = %v, want %v; error = %v", code, tc.wantCode, err)
			}
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("CheckConnection: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("CheckConnection error = %v, want %q", err, tc.wantError)
			}
			wantCalls := 1
			if tc.path == "" {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Fatalf("endpoint calls = %d, want %d", calls, wantCalls)
			}
		})
	}
}

func TestCheckConnectionSystemOneDecision(t *testing.T) {
	t.Setenv(common.EnvAllowAnyHost, "true")
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/systemone" {
			t.Errorf("request = %s %s, want POST /v1/systemone", r.Method, r.URL.Path)
		}
		var body struct {
			Model     string                     `json:"model"`
			Questions map[string]json.RawMessage `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Model != "decision-model-preview" || len(body.Questions) != 1 {
			t.Errorf("decision request = %+v, want decision-model-preview with one question", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answers":{"verification":{"type":"noul","noul":0.5}}}`))
	}))
	defer srv.Close()

	code, err := NewModelProviderService().CheckConnection(t.Context(), "SystemOne", CheckConnectionRequest{
		BaseURL: srv.URL + "/v1",
		APIKey:  json.RawMessage(`"sk-test"`),
		ModelInfo: []CheckConnectionModelInfo{{
			ModelName: "decision-model-preview", ModelTypes: []string{"decision"},
		}},
	})
	if code != common.CodeSuccess || err != nil {
		t.Fatalf("CheckConnection = (%v, %v), want success", code, err)
	}
	if calls != 1 {
		t.Fatalf("decision endpoint calls = %d, want 1", calls)
	}
}
