package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
)

func newDaoxeForTest(baseURL string) *DaoxeModel {
	return NewDaoxeModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models", Embedding: "embeddings"},
	)
}

func TestDaoxeName(t *testing.T) {
	if got := newDaoxeForTest("http://unused").Name(); got != "DaoXE" {
		t.Errorf("Name()=%q", got)
	}
}

func TestDaoxeFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("daoxe", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*DaoxeModel); !ok {
		t.Fatalf("driver type=%T, want *DaoxeModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*DaoxeModel); !ok {
		t.Fatal("NewInstance did not return *DaoxeModel")
	}
}

// DaoXE is a hosted gateway, so unlike a local server it must receive the
// tenant's key as a bearer token on every call.
func TestDaoxeChatSendsAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-daoxe-test" {
			t.Errorf("Authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-daoxe",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newDaoxeForTest(srv.URL).ChatWithMessages(
		t.Context(), "gpt-4o", []Message{{Role: "user", Content: "ping"}},
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
