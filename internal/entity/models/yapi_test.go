package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
)

func newYAPIForTest(baseURL string) *YAPIModel {
	return NewYAPIModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestYAPIName(t *testing.T) {
	if got := newYAPIForTest("http://unused").Name(); got != "Y-API" {
		t.Errorf("Name()=%q", got)
	}
}

func TestYAPIFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("y-api", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*YAPIModel); !ok {
		t.Fatalf("driver type=%T, want *YAPIModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*YAPIModel); !ok {
		t.Fatal("NewInstance did not return *YAPIModel")
	}
}

// Y-API is a hosted gateway, so unlike a local server it must receive the
// tenant's key as a bearer token on every call.
func TestYAPIChatSendsAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer "+testAPIKey; got != want {
			t.Errorf("Authorization=%q, want %q", got, want)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-y-api",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newYAPIForTest(srv.URL).ChatWithMessages(
		t.Context(), "deepseek/deepseek-v4-flash", []Message{{Role: "user", Content: "ping"}},
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
