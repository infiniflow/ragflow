package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
)

func newCheaperInferenceForTest(baseURL string) *CheaperInferenceModel {
	return NewCheaperInferenceModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestCheaperInferenceName(t *testing.T) {
	if got := newCheaperInferenceForTest("http://unused").Name(); got != "Cheaper Inference" {
		t.Errorf("Name()=%q", got)
	}
}

func TestCheaperInferenceFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("Cheaper Inference", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*CheaperInferenceModel); !ok {
		t.Fatalf("driver type=%T, want *CheaperInferenceModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*CheaperInferenceModel); !ok {
		t.Fatal("NewInstance did not return *CheaperInferenceModel")
	}
}

// Cheaper Inference is a hosted gateway, so unlike a local server it must
// receive the tenant's key as a bearer token on every call.
func TestCheaperInferenceChatSendsAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer "+testAPIKey; got != want {
			t.Errorf("Authorization=%q, want %q", got, want)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-cheaperinference",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newCheaperInferenceForTest(srv.URL).ChatWithMessages(
		t.Context(), "claude-opus-5", []Message{{Role: "user", Content: "ping"}},
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

// Responses carry extra members the decoder does not know. They must not disturb
// answer or usage extraction.
func TestCheaperInferenceChatAcceptsUnknownResponseMembers(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "chat-cheaperinference-extra-members",
			"model":        "claude-opus-5",
			"provider":     "upstream-provider-name",
			"service_tier": "default",
			"choices":      []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":        map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newCheaperInferenceForTest(srv.URL).ChatWithMessages(
		t.Context(), "claude-opus-5", []Message{{Role: "user", Content: "ping"}},
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
