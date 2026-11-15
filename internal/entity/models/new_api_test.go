package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
)

var newAPITestAPIKey = "sk-newapi-test"

func newNewAPIForTest(baseURL string) *NewAPIModel {
	return NewNewAPIModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models", Embedding: "embeddings", Rerank: "rerank"},
	)
}

func TestNewAPIName(t *testing.T) {
	if got := newNewAPIForTest("http://unused").Name(); got != "New API" {
		t.Errorf("Name()=%q", got)
	}
}

func TestNewAPIFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("new api", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*NewAPIModel); !ok {
		t.Fatalf("driver type=%T, want *NewAPIModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*NewAPIModel); !ok {
		t.Fatal("NewInstance did not return *NewAPIModel")
	}
}

// New API is an aggregation gateway, so it must receive the tenant's key as a
// bearer token on every call.
func TestNewAPIChatSendsAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-newapi-test" {
			t.Errorf("Authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-newapi",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newNewAPIForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"gpt-4o",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &newAPITestAPIKey},
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

// New API exposes whatever models its upstreams relay, so RAGFlow discovers them
// dynamically through /models rather than from a shipped catalogue.
func TestNewAPIListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-newapi-test" {
			t.Errorf("Authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-4o"},
				{"id": "text-embedding-3-large"},
			},
		})
	}))
	defer srv.Close()

	model := newNewAPIForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &newAPITestAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "gpt-4o,text-embedding-3-large" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &newAPITestAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

// The shipped config must resolve to the New API driver and expose the OpenAI
// endpoint suffixes for every model type the gateway relays. Unlike a hosted
// provider with a fixed catalogue, New API ships no static models and advertises
// a url_hint so the operator points it at their own gateway.
func TestNewAPIProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "new-api.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("New API")
	if provider == nil {
		t.Fatal("New API provider not found")
	}
	if _, ok := provider.ModelDriver.(*NewAPIModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.NewAPIModel", provider.ModelDriver)
	}
	if provider.URLSuffix.Chat != "chat/completions" ||
		provider.URLSuffix.Models != "models" ||
		provider.URLSuffix.Embedding != "embeddings" ||
		provider.URLSuffix.Rerank != "rerank" {
		t.Errorf("URLSuffix=%+v", provider.URLSuffix)
	}
	if provider.URLSuffix.TTS != "audio/speech" || provider.URLSuffix.ASR != "audio/transcriptions" {
		t.Errorf("audio URLSuffix=%+v", provider.URLSuffix)
	}
	if provider.URLHint == "" {
		t.Error("New API should advertise a url_hint for the operator's gateway")
	}
	if len(provider.Models) != 0 {
		t.Errorf("New API ships %d static models, want 0 (dynamic discovery)", len(provider.Models))
	}
}
