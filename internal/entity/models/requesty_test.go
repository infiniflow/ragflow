package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
)

func newRequestyForTest(baseURL string) *RequestyModel {
	return NewRequestyModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestRequestyName(t *testing.T) {
	if got := newRequestyForTest("http://unused").Name(); got != "Requesty" {
		t.Errorf("Name()=%q", got)
	}
}

func TestRequestyFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("Requesty", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*RequestyModel); !ok {
		t.Fatalf("driver type=%T, want *RequestyModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*RequestyModel); !ok {
		t.Fatal("NewInstance did not return *RequestyModel")
	}
}

func TestRequestyChatSendsAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer "+testAPIKey; got != want {
			t.Errorf("Authorization=%q, want %q", got, want)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-requesty",
			"model":   "gpt-4o-mini-2024-07-18",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8, "cost": 0.0001},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newRequestyForTest(srv.URL).ChatWithMessages(
		t.Context(), "openai/gpt-4o-mini", []Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &testAPIKey}, nil, usage,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if *resp.Answer != "pong" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
	assertModelUsage(t, usage, 3, 5, 8)
}

// ListModels puts managed models first, merges the full catalog without
// duplicates, keeps only chat entries and drops IDs with control characters.
func TestRequestyListModelsMergesManagedAndCatalog(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer "+testAPIKey; got != want {
			t.Errorf("Authorization=%q, want %q", got, want)
		}
		switch r.URL.Path {
		case "/models/managed":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"id": "claude-sonnet-4-5", "api": "chat", "context_window": 200000, "max_output_tokens": 64000, "supports_vision": true},
			}})
		case "/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"id": "claude-sonnet-4-5", "api": "chat"},
				{"id": "openai/gpt-4o-mini", "api": "chat", "context_window": 128000, "max_output_tokens": 16384},
				{"id": "openai/text-embedding-3-small", "api": "embedding"},
				{"id": "bad/\x1b[31mred", "api": "chat"},
				{"id": " ", "api": "chat"},
			}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	models, err := newRequestyForTest(srv.URL).ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2: %+v", len(models), models)
	}
	first := models[0]
	if first.Name != "claude-sonnet-4-5" || first.ContextLength == nil || *first.ContextLength != 200000 ||
		first.MaxOutput == nil || *first.MaxOutput != 64000 {
		t.Errorf("first=%+v", first)
	}
	if len(first.ModelTypes) != 2 || first.ModelTypes[0] != "chat" || first.ModelTypes[1] != "vision" {
		t.Errorf("first.ModelTypes=%v", first.ModelTypes)
	}
	if second := models[1]; second.Name != "openai/gpt-4o-mini" || len(second.ModelTypes) != 1 || second.ModelTypes[0] != "chat" {
		t.Errorf("second=%+v", second)
	}
}

// One failing list must not hide the other, and the public catalog is
// fetched without an Authorization header when no key is set.
func TestRequestyListModelsKeylessWithOneEndpointFailing(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization=%q, want empty", got)
		}
		if r.URL.Path == "/models/managed" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"id": "openai/gpt-4o-mini", "api": "chat"},
		}})
	}))
	defer srv.Close()

	models, err := newRequestyForTest(srv.URL).ListModels(t.Context(), &APIConfig{})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 || models[0].Name != "openai/gpt-4o-mini" {
		t.Fatalf("models=%+v", models)
	}
}

func TestRequestyListModelsBothEndpointsFailing(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	if _, err := newRequestyForTest(srv.URL).ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err == nil {
		t.Fatal("ListModels succeeded, want error")
	}
}
