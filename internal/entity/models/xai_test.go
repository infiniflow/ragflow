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

func newXAIForTest(baseURL string) *XAIModel {
	return NewXAIModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "/models"},
	)
}

func TestXAIConfigDeclaresModelsSuffix(t *testing.T) {
	var provider struct {
		URLSuffix URLSuffix `json:"url_suffix"`
	}

	for _, candidate := range []string{
		filepath.Join("..", "..", "..", "conf", "models", "xai.json"),
		filepath.Join("conf", "models", "xai.json"),
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

	t.Fatal("could not locate conf/models/xai.json")
}

func TestXAIListModelsHappyPath(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		if r.URL.Path != "/models" {
			t.Errorf("path=%s, want /models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q, want Bearer test-key", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type=%q, want application/json", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object": "list",
			"data": [
				{"id": "grok-4"},
				{"id": "grok-3-mini"}
			]
		}`))
	}))
	defer srv.Close()

	apiKey := "test-key"
	models, err := newXAIForTest(srv.URL+"/").ListModels(ctx, &APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(models, ",") != "grok-4,grok-3-mini" {
		t.Fatalf("models=%v", models)
	}
}

func TestXAIListModelsRequiresAPIKey(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	_, err := newXAIForTest("http://unused").ListModels(ctx, &APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Fatalf("expected api key error, got %v", err)
	}
}

func TestXAIListModelsRejectsProviderError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	defer srv.Close()

	apiKey := "test-key"
	_, err := newXAIForTest(srv.URL).ListModels(ctx, &APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("expected provider error with status and body, got %v", err)
	}
}

func TestXAICheckConnectionDelegatesToListModels(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path=%s, want /models", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"grok-4"}]}`))
	}))
	defer srv.Close()

	apiKey := "test-key"
	if err := newXAIForTest(srv.URL).CheckConnection(ctx, &APIConfig{ApiKey: &apiKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestXAIListModelsRequiresModelsSuffix(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("ListModels should reject a missing models suffix before sending a request")
	}))
	defer srv.Close()

	apiKey := "test-key"
	model := NewXAIModel(map[string]string{"default": srv.URL}, URLSuffix{})

	_, err := model.ListModels(ctx, &APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "models URL suffix is not configured") {
		t.Fatalf("expected missing models suffix error, got %v", err)
	}
}

func TestXAIChatHappyPath(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q, want Bearer test-key", got)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if body["stream"] != false {
			t.Errorf("stream=%v, want false", body["stream"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content":           "pong",
					"reasoning_content": "\nthought",
				},
			}},
		})
	}))
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newXAIForTest(srv.URL).ChatWithMessages(
		ctx,
		"grok-3-mini",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Errorf("Answer=%v, want pong", resp.Answer)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "thought" {
		t.Errorf("ReasonContent=%v, want thought", resp.ReasonContent)
	}
}

func TestXAIStreamHappyPath(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("Accept=%q, want text/event-stream", got)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		streamOptions, ok := body["stream_options"].(map[string]interface{})
		if !ok || streamOptions["include_usage"] != true {
			t.Errorf("stream_options=%#v, want include_usage=true", body["stream_options"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, strings.Join([]string{
			`data: {"choices":[{"delta":{"reasoning_content":"step "}}]}`,
			`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
			`data: {"choices":[{"delta":{"content":" world"},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
			``,
		}, "\n"))
	}))
	defer srv.Close()

	apiKey := "test-key"
	var content, reasoning []string
	err := newXAIForTest(srv.URL).ChatStreamlyWithSender(
		ctx,
		"grok-3-mini",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		nil,
		func(answer, reason *string) error {
			if answer != nil {
				content = append(content, *answer)
			}
			if reason != nil {
				reasoning = append(reasoning, *reason)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if strings.Join(reasoning, "") != "step " {
		t.Errorf("reasoning=%q", strings.Join(reasoning, ""))
	}
	if got := strings.Join(content, ""); got != "Hello world[DONE]" {
		t.Errorf("content=%q, want Hello world[DONE]", got)
	}
}
