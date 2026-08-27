package models

import (
	"encoding/json"
	"io"
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
	withSSRFBypass(t)
	ctx := t.Context()
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
	models, err := newVolcEngineForTest(srv.URL).ListModels(ctx, &APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(models, ",") != "doubao-seed-2-0-pro-260215,doubao-embedding-vision-251215" {
		t.Errorf("models=%v", models)
	}
}

func TestVolcEngineListModelsRejectsProviderError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newVolcEngineServer(t, func(t *testing.T, r *http.Request, w http.ResponseWriter) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newVolcEngineForTest(srv.URL).ListModels(ctx, &APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "401 Unauthorized") || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("expected provider error with status and body, got %v", err)
	}
}

func TestVolcEngineListModelsRequiresModelsSuffix(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	apiKey := "test-key"
	model := NewVolcEngine(map[string]string{"default": "http://unused"}, URLSuffix{})

	_, err := model.ListModels(ctx, &APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "models URL suffix is not configured") {
		t.Fatalf("expected missing models suffix error, got %v", err)
	}
}

func TestVolcEngineChatStreamSupportsMaxEffortAndUsage(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newVolcEngineServer(t, func(t *testing.T, r *http.Request, w http.ResponseWriter) {
		// The streaming endpoint must honor URLSuffix.Chat, not a hardcoded
		// "chat/completions" path.
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%s, want /v1/chat/completions", r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["reasoning_effort"] != "max" {
			t.Errorf("reasoning_effort=%v, want max", body["reasoning_effort"])
		}
		streamOptions, ok := body["stream_options"].(map[string]interface{})
		if !ok || streamOptions["include_usage"] != true {
			t.Errorf("stream_options=%#v, want include_usage=true", body["stream_options"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":5,\"total_tokens\":8}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	defer srv.Close()

	apiKey := "test-key"
	thinking := true
	effort := "max"
	config := &ChatConfig{Thinking: &thinking, Effort: &effort}
	driver := NewVolcEngine(
		map[string]string{"default": srv.URL},
		URLSuffix{Chat: "v1/chat/completions", Models: "models"},
	)
	if err := driver.ChatStreamlyWithSender(
		ctx,
		"doubao-seed-2-0-pro-260215",
		[]Message{{Role: "user", Content: "hello"}},
		&APIConfig{ApiKey: &apiKey},
		config,
		nil,
		func(*string, *string) error { return nil },
	); err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if config.UsageResult == nil || config.UsageResult.TotalTokens != 8 {
		t.Fatalf("UsageResult=%#v, want total tokens 8", config.UsageResult)
	}
}

func TestVolcEngineChatStreamRejectsTruncatedResponse(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newVolcEngineServer(t, func(t *testing.T, _ *http.Request, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
	})
	defer srv.Close()

	apiKey := "test-key"
	err := newVolcEngineForTest(srv.URL).ChatStreamlyWithSender(
		ctx,
		"doubao-seed-2-0-pro-260215",
		[]Message{{Role: "user", Content: "hello"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{},
		nil,
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "stream ended before [DONE] or finish_reason") {
		t.Fatalf("error=%v, want truncated stream error", err)
	}
}
