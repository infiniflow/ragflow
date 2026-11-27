package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"ragflow/internal/common"
	"strings"
	"testing"
)

const scalewayTestProjectID = "11111111-2222-3333-4444-555555555555"

func newScalewayForTest(baseURL string) *ScalewayModel {
	return NewScalewayModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models", Embedding: "embeddings", ASR: "audio/transcriptions"},
	)
}

func TestScalewayName(t *testing.T) {
	if got := newScalewayForTest("http://unused").Name(); got != "scaleway" {
		t.Errorf("Name()=%q", got)
	}
}

func TestScalewayFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("scaleway", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*ScalewayModel); !ok {
		t.Fatalf("driver type=%T, want *ScalewayModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*ScalewayModel); !ok {
		t.Fatal("NewInstance did not return *ScalewayModel")
	}
}

func TestScalewayProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "scaleway.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("Scaleway")
	if provider == nil {
		t.Fatal("Scaleway provider not found")
	}
	if _, ok := provider.ModelDriver.(*ScalewayModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.ScalewayModel", provider.ModelDriver)
	}
}

func TestScalewayChatSendsAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-gw-test" {
			t.Errorf("Authorization=%q", got)
		}
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Model != "llama-3.3-70b-instruct" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-scaleway",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newScalewayForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"llama-3.3-70b-instruct",
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

// A user-supplied base URL with a Project ID must be used instead of the default.
func TestScalewayProjectScopedBaseURL(t *testing.T) {
	withSSRFBypass(t)
	wantPath := "/" + scalewayTestProjectID + "/v1/chat/completions"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Errorf("path=%s, want %s", r.URL.Path, wantPath)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-scaleway",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer srv.Close()

	projectURL := srv.URL + "/" + scalewayTestProjectID + "/v1"
	_, err := newScalewayForTest("http://unused").ChatWithMessages(
		t.Context(),
		"llama-3.3-70b-instruct",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &testAPIKey, BaseURL: &projectURL},
		nil,
		&common.ModelUsage{},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
}

func TestScalewayListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-gw-test" {
			t.Errorf("Authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "llama-3.3-70b-instruct"},
				{"id": "mistral-small-3.2-24b-instruct-2506"},
			},
		})
	}))
	defer srv.Close()

	model := newScalewayForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "llama-3.3-70b-instruct,mistral-small-3.2-24b-instruct-2506" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestScalewayEmbedUsesEmbeddingsEndpoint(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("path=%s, want /embeddings", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"index": 0, "embedding": []float64{0.1, 0.2}},
				{"index": 1, "embedding": []float64{0.3, 0.4}},
			},
			"usage": map[string]any{"prompt_tokens": 4, "total_tokens": 4},
		})
	}))
	defer srv.Close()

	name := "qwen3-embedding-8b"
	got, err := newScalewayForTest(srv.URL).Embed(t.Context(), &name, EmbedRequest{Texts: []string{"a", "b"}},
		&APIConfig{ApiKey: &testAPIKey}, &EmbeddingConfig{}, &common.ModelUsage{})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("embeddings=%v, want 2", got)
	}
}

func TestScalewayTranscribeAudioUsesTranscriptionsEndpoint(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" {
			t.Errorf("path=%s, want /audio/transcriptions", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("Content-Type=%q, want multipart/form-data", r.Header.Get("Content-Type"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"text": "hello"})
	}))
	defer srv.Close()

	file := filepath.Join(t.TempDir(), "clip.wav")
	if err := os.WriteFile(file, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	name := "whisper-large-v3"
	resp, err := newScalewayForTest(srv.URL).TranscribeAudio(t.Context(), &name, &file,
		&APIConfig{ApiKey: &testAPIKey}, nil, &common.ModelUsage{})
	if err != nil {
		t.Fatalf("TranscribeAudio: %v", err)
	}
	if resp.Text != "hello" {
		t.Errorf("Text=%q", resp.Text)
	}
}
