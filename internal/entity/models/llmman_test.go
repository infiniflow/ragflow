package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
)

func newLlmmanForTest(baseURL string) *LlmmanModel {
	return NewLlmmanModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models", Embedding: "embeddings"},
	)
}

func TestLlmmanName(t *testing.T) {
	if got := newLlmmanForTest("http://unused").Name(); got != "llmman" {
		t.Errorf("Name()=%q", got)
	}
}

func TestLlmmanFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("llmman", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*LlmmanModel); !ok {
		t.Fatalf("driver type=%T, want *LlmmanModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*LlmmanModel); !ok {
		t.Fatal("NewInstance did not return *LlmmanModel")
	}
}

// A local server needs no credential, so calls must work with an empty APIConfig.
func TestLlmmanChatWithoutAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization=%q, want empty", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-llmman",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newLlmmanForTest(srv.URL).ChatWithMessages(
		t.Context(), "qwen3", []Message{{Role: "user", Content: "ping"}}, &APIConfig{}, nil, usage,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if *resp.Answer != "pong" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
	assertModelUsage(t, usage, 3, 5, 8)
}

func TestLlmmanEmbed(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":  []map[string]any{{"embedding": []float64{0.1, 0.2}, "index": 0}},
			"usage": map[string]any{"prompt_tokens": 7, "total_tokens": 7},
		})
	}))
	defer srv.Close()

	modelName := "nomic-embed-text"
	embeddings, err := newLlmmanForTest(srv.URL).Embed(
		t.Context(), &modelName, EmbedRequest{Texts: []string{"document"}}, &APIConfig{}, nil, &common.ModelUsage{},
	)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(embeddings) != 1 || len(embeddings[0].Embedding) != 2 {
		t.Fatalf("embeddings=%#v", embeddings)
	}
}

func TestLlmmanListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "qwen3"}, {"id": "nomic-embed-text"}},
		})
	}))
	defer srv.Close()

	model := newLlmmanForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "qwen3,nomic-embed-text" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}
