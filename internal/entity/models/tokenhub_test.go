package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTokenHubServer(t *testing.T, expectedMethod, expectedPath string, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != expectedMethod {
			t.Errorf("expected method=%s, got %s", expectedMethod, r.Method)
			return
		}
		if r.URL.Path != expectedPath {
			t.Errorf("expected path=%s, got %s", expectedPath, r.URL.Path)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization=Bearer test-key, got %q", got)
			return
		}

		if r.Method == http.MethodPost {
			if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
				t.Errorf("expected Content-Type to start with application/json, got %q", got)
				return
			}
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
				http.Error(w, "read error", http.StatusBadRequest)
				return
			}
			var body map[string]interface{}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("unmarshal: %v\nraw=%s", err, string(raw))
				http.Error(w, "unmarshal error", http.StatusBadRequest)
				return
			}
			handler(t, body, w)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		if len(raw) != 0 {
			t.Errorf("expected no request body for %s, got %q", r.Method, string(raw))
			http.Error(w, "unexpected body", http.StatusBadRequest)
			return
		}
		handler(t, nil, w)
	}))
}

func newTokenHubSSEServer(t *testing.T, expectedPath, ssePayload string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
			return
		}
		if r.URL.Path != expectedPath {
			t.Errorf("expected path=%s, got %s", expectedPath, r.URL.Path)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization=Bearer test-key, got %q", got)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, ssePayload)
	}))
}

func newTokenHubForTest(baseURL string) *TokenHubModel {
	return NewTokenHubModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Embedding: "embeddings", Models: "models"},
	)
}

func TestTokenHubFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("TokenHub", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*TokenHubModel); !ok {
		t.Fatalf("driver type=%T, want *TokenHubModel", driver)
	}
}

func TestTokenHubChatWithMessagesForcesNonStreaming(t *testing.T) {
	srv := newTokenHubServer(t, http.MethodPost, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
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
	})
	defer srv.Close()

	apiKey := "test-key"
	stream := true
	resp, err := newTokenHubForTest(srv.URL).ChatWithMessages(
		"gpt-4o-mini",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
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

func TestTokenHubChatRequiresAPIKey(t *testing.T) {
	_, err := newTokenHubForTest("http://unused").ChatWithMessages("gpt-4o-mini", []Message{{Role: "user", Content: "x"}}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Fatalf("expected api-key error, got %v", err)
	}
}

func TestTokenHubChatRequiresModelName(t *testing.T) {
	apiKey := "test-key"
	_, err := newTokenHubForTest("http://unused").ChatWithMessages(" ", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Fatalf("expected model-name error, got %v", err)
	}
}

func TestTokenHubStreamHappyPath(t *testing.T) {
	srv := newTokenHubSSEServer(t, "/chat/completions", strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"thinking"}}]}`,
		`data: {"choices":[{"delta":{"content":"hello"}}]}`,
		`data: [DONE]`,
		``,
	}, "\n"))
	defer srv.Close()

	apiKey := "test-key"
	var content []string
	var reasoning []string
	err := newTokenHubForTest(srv.URL).ChatStreamlyWithSender(
		"gpt-4o-mini",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		func(c *string, r *string) error {
			if c != nil {
				content = append(content, *c)
			}
			if r != nil {
				reasoning = append(reasoning, *r)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if strings.Join(reasoning, "") != "thinking" {
		t.Errorf("reasoning=%v", reasoning)
	}
	if strings.Join(content, "") != "hello[DONE]" {
		t.Errorf("content=%v", content)
	}
}

func TestTokenHubStreamRejectsFalseStreamConfig(t *testing.T) {
	apiKey := "test-key"
	stream := false
	err := newTokenHubForTest("http://unused").ChatStreamlyWithSender(
		"gpt-4o-mini",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Fatalf("expected stream error, got %v", err)
	}
}

func TestTokenHubStreamRequiresSender(t *testing.T) {
	apiKey := "test-key"
	err := newTokenHubForTest("http://unused").ChatStreamlyWithSender(
		"gpt-4o-mini",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Fatalf("expected sender error, got %v", err)
	}
}

func TestTokenHubStreamRequiresAPIKey(t *testing.T) {
	err := newTokenHubForTest("http://unused").ChatStreamlyWithSender(
		"gpt-4o-mini",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{},
		nil,
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Fatalf("expected api-key error, got %v", err)
	}
}

func TestTokenHubStreamRequiresModelName(t *testing.T) {
	apiKey := "test-key"
	err := newTokenHubForTest("http://unused").ChatStreamlyWithSender(
		" ",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Fatalf("expected model-name error, got %v", err)
	}
}

func TestTokenHubEmbedHappyPath(t *testing.T) {
	srv := newTokenHubServer(t, http.MethodPost, "/embeddings", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "text-embedding-3-small" {
			t.Errorf("model=%v", body["model"])
		}
		inputs, ok := body["input"].([]interface{})
		if !ok || len(inputs) != 2 {
			t.Errorf("input=%#v", body["input"])
			http.Error(w, "invalid input", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{0.1, 0.2}, "index": 0},
				{"embedding": []float64{0.3, 0.4}, "index": 1},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := "text-embedding-3-small"
	embeddings, err := newTokenHubForTest(srv.URL).Embed(&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(embeddings) != 2 || embeddings[1].Index != 1 || embeddings[1].Embedding[0] != 0.3 {
		t.Fatalf("embeddings=%#v", embeddings)
	}
}

func TestTokenHubEmbedValidatesInputs(t *testing.T) {
	apiKey := "test-key"
	if embeddings, err := newTokenHubForTest("http://unused").Embed(nil, nil, &APIConfig{ApiKey: &apiKey}, nil); err != nil || len(embeddings) != 0 {
		t.Fatalf("empty input should return empty embeddings, got %#v err=%v", embeddings, err)
	}
	if _, err := newTokenHubForTest("http://unused").Embed(nil, []string{"x"}, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Fatalf("expected model-name error, got %v", err)
	}
	model := "text-embedding-3-small"
	if _, err := newTokenHubForTest("http://unused").Embed(&model, []string{"x"}, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Fatalf("expected api-key error, got %v", err)
	}
}

func TestTokenHubListModelsHappyPathSkipsMalformedItems(t *testing.T) {
	srv := newTokenHubServer(t, http.MethodGet, "/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []interface{}{
				map[string]interface{}{"id": "gpt-4o-mini"},
				map[string]interface{}{"name": "missing-id"},
				"not-an-object",
				map[string]interface{}{"id": "gpt-4o"},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	models, err := newTokenHubForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	want := []string{"gpt-4o-mini", "gpt-4o"}
	if joinModelNames(models, ",") != strings.Join(want, ",") {
		t.Fatalf("models=%v, want %v", models, want)
	}
}

func TestTokenHubListModelsValidatesResponseAndAPIKey(t *testing.T) {
	if _, err := newTokenHubForTest("http://unused").ListModels(&APIConfig{}); err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Fatalf("expected api-key error, got %v", err)
	}

	srv := newTokenHubServer(t, http.MethodGet, "/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{"id": "wrong"}})
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newTokenHubForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "invalid models list format") {
		t.Fatalf("expected invalid-format error, got %v", err)
	}
}
