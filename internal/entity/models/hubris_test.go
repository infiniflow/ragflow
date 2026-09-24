package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
)

var testAPIKey = "sk-gw-test"

func newHubrisForTest(baseURL string) *HubrisModel {
	return NewHubrisModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models", Embedding: "embeddings"},
	)
}

func TestHubrisName(t *testing.T) {
	if got := newHubrisForTest("http://unused").Name(); got != "hubris" {
		t.Errorf("Name()=%q", got)
	}
}

func TestHubrisFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("hubris", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*HubrisModel); !ok {
		t.Fatalf("driver type=%T, want *HubrisModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*HubrisModel); !ok {
		t.Fatal("NewInstance did not return *HubrisModel")
	}
}

// Hubris is a hosted gateway, so unlike a local server it must receive the
// tenant's key as a bearer token on every call.
func TestHubrisChatSendsAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-gw-test" {
			t.Errorf("Authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-hubris",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newHubrisForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"anthropic/claude-sonnet-5",
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

func TestHubrisEmbed(t *testing.T) {
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

	modelName := "openai/text-embedding-3-small"
	embeddings, err := newHubrisForTest(srv.URL).Embed(
		t.Context(),
		&modelName,
		EmbedRequest{Texts: []string{"document"}},
		&APIConfig{ApiKey: &testAPIKey},
		nil,
		&common.ModelUsage{},
	)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(embeddings) != 1 || len(embeddings[0].Embedding) != 2 {
		t.Fatalf("embeddings=%#v", embeddings)
	}
}

func TestHubrisListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "anthropic/claude-sonnet-5"},
				{"id": "openai/text-embedding-3-small"},
			},
		})
	}))
	defer srv.Close()

	model := newHubrisForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "anthropic/claude-sonnet-5,openai/text-embedding-3-small" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}
