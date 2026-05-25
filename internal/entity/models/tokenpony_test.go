package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTokenPonyServer(t *testing.T, expectedPath string, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		handler(t, nil, w)
	}))
}

func newTokenPonySSEServer(t *testing.T, expectedPath, ssePayload string) *httptest.Server {
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
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("expected Content-Type to start with application/json, got %q", got)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, ssePayload)
	}))
}

func newTokenPonyForTest(baseURL string) *TokenPonyModel {
	return NewTokenPonyModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestTokenPonyName(t *testing.T) {
	if got := newTokenPonyForTest("http://unused").Name(); got != "tokenpony" {
		t.Errorf("Name()=%q, want %q", got, "tokenpony")
	}
}

func TestTokenPonyFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("TokenPony", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*TokenPonyModel); !ok {
		t.Fatalf("driver type=%T, want *TokenPonyModel", driver)
	}
}

func TestTokenPonyChatHappyPath(t *testing.T) {
	srv := newTokenPonyServer(t, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "qwen3-32b" {
			t.Errorf("model=%v", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v, want false", body["stream"])
		}
		if body["max_tokens"] != float64(64) {
			t.Errorf("max_tokens=%v, want 64", body["max_tokens"])
		}
		if body["temperature"] != 0.3 {
			t.Errorf("temperature=%v, want 0.3", body["temperature"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content":           "pong",
					"reasoning_content": "thought about it",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	mt := 64
	temp := 0.3
	resp, err := newTokenPonyForTest(srv.URL).ChatWithMessages(
		"qwen3-32b",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{MaxTokens: &mt, Temperature: &temp},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Errorf("Answer=%v, want pong", resp.Answer)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "thought about it" {
		t.Errorf("ReasonContent=%v, want 'thought about it'", resp.ReasonContent)
	}
}

func TestTokenPonyChatNoReasoning(t *testing.T) {
	srv := newTokenPonyServer(t, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{"content": "hi"},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newTokenPonyForTest(srv.URL).ChatWithMessages(
		"qwen3-8b",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "hi" {
		t.Errorf("Answer=%v, want 'hi'", resp.Answer)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "" {
		t.Errorf("ReasonContent=%v, want empty", resp.ReasonContent)
	}
}

func TestTokenPonyChatRequiresAPIKey(t *testing.T) {
	_, err := newTokenPonyForTest("http://unused").ChatWithMessages(
		"qwen3-32b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestTokenPonyChatRequiresMessages(t *testing.T) {
	apiKey := "test-key"
	_, err := newTokenPonyForTest("http://unused").ChatWithMessages(
		"qwen3-32b", nil, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("expected messages-empty error, got %v", err)
	}
}

func TestTokenPonyChatPropagatesHTTPError(t *testing.T) {
	srv := newTokenPonyServer(t, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newTokenPonyForTest(srv.URL).ChatWithMessages(
		"qwen3-32b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 propagated, got %v", err)
	}
}

func TestTokenPonyStreamHappyPath(t *testing.T) {
	srv := newTokenPonySSEServer(t, "/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"Hello"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	var chunks []string
	var sawDone bool
	err := newTokenPonyForTest(srv.URL).ChatStreamlyWithSender(
		"qwen3-32b",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(c *string, _ *string) error {
			if c == nil {
				return nil
			}
			if *c == "[DONE]" {
				sawDone = true
				return nil
			}
			chunks = append(chunks, *c)
			return nil
		})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if strings.Join(chunks, "") != "Hello world" {
		t.Errorf("content=%v", chunks)
	}
	if !sawDone {
		t.Error("expected [DONE] sentinel")
	}
}

func TestTokenPonyStreamSplitsReasoning(t *testing.T) {
	srv := newTokenPonySSEServer(t, "/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"reasoning_content":"step 1. "}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"reasoning_content":"step 2."}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"final"},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	var content, reasoning []string
	err := newTokenPonyForTest(srv.URL).ChatStreamlyWithSender(
		"deepseek-r1-0528",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(c *string, r *string) error {
			if c != nil && r != nil {
				t.Errorf("sender called with both args non-nil")
			}
			if r != nil && *r != "" {
				reasoning = append(reasoning, *r)
			}
			if c != nil && *c != "" && *c != "[DONE]" {
				content = append(content, *c)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if got := strings.Join(reasoning, ""); got != "step 1. step 2." {
		t.Errorf("reasoning=%q", got)
	}
	if got := strings.Join(content, ""); got != "final" {
		t.Errorf("content=%q", got)
	}
}

func TestTokenPonyStreamRejectsExplicitFalse(t *testing.T) {
	apiKey := "test-key"
	stream := false
	err := newTokenPonyForTest("http://unused").ChatStreamlyWithSender(
		"qwen3-32b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Errorf("expected stream-true guard, got %v", err)
	}
}

func TestTokenPonyStreamRequiresSender(t *testing.T) {
	apiKey := "test-key"
	err := newTokenPonyForTest("http://unused").ChatStreamlyWithSender(
		"qwen3-32b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("expected sender-required error, got %v", err)
	}
}

func TestTokenPonyStreamFailsWithoutTerminal(t *testing.T) {
	srv := newTokenPonySSEServer(t, "/chat/completions",
		`data: {"choices":[{"delta":{"content":"half"}}]}`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	err := newTokenPonyForTest(srv.URL).ChatStreamlyWithSender(
		"qwen3-32b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream ended before") {
		t.Errorf("expected truncation error, got %v", err)
	}
}

func TestTokenPonyStreamRejectsMalformedFrame(t *testing.T) {
	srv := newTokenPonySSEServer(t, "/chat/completions",
		`data: {"choices":[{"delta":{"content":"ok"}}]}`+"\n"+
			`data: {oops not json}`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	err := newTokenPonyForTest(srv.URL).ChatStreamlyWithSender(
		"qwen3-32b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "invalid SSE event") {
		t.Errorf("expected invalid-SSE error, got %v", err)
	}
}

func TestTokenPonyStreamSurfacesUpstreamError(t *testing.T) {
	srv := newTokenPonySSEServer(t, "/chat/completions",
		`data: {"choices":[{"delta":{"content":"partial "}}]}`+"\n"+
			`data: {"error":{"message":"rate limit","type":"rate_limit_error"}}`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	err := newTokenPonyForTest(srv.URL).ChatStreamlyWithSender(
		"qwen3-32b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "upstream stream error") {
		t.Errorf("expected upstream-error surfacing, got %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("expected upstream message included, got %v", err)
	}
}

func TestTokenPonyListModelsHappyPath(t *testing.T) {
	srv := newTokenPonyServer(t, "/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "qwen3-32b"},
				{"id": "deepseek-v3-0324"},
				{"id": "qwen3-coder-480b"},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	models, err := newTokenPonyForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	want := []string{"qwen3-32b", "deepseek-v3-0324", "qwen3-coder-480b"}
	if strings.Join(models, ",") != strings.Join(want, ",") {
		t.Errorf("models=%v, want %v", models, want)
	}
}

func TestTokenPonyListModelsRequiresAPIKey(t *testing.T) {
	_, err := newTokenPonyForTest("http://unused").ListModels(&APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestTokenPonyCheckConnectionDelegatesToListModels(t *testing.T) {
	srv := newTokenPonyServer(t, "/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{{"id": "qwen3-32b"}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	if err := newTokenPonyForTest(srv.URL).CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Errorf("CheckConnection: %v", err)
	}
}

func TestTokenPonyCheckConnectionPropagatesError(t *testing.T) {
	srv := newTokenPonyServer(t, "/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	err := newTokenPonyForTest(srv.URL).CheckConnection(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 propagated, got %v", err)
	}
}

func TestTokenPonyBaseURLForRegionUnknown(t *testing.T) {
	m := newTokenPonyForTest("http://unused")
	apiKey := "test-key"
	region := "missing"
	_, err := m.ListModels(&APIConfig{ApiKey: &apiKey, Region: &region})
	if err == nil || !strings.Contains(err.Error(), "no base URL configured") {
		t.Errorf("expected base-URL error, got %v", err)
	}
}

func TestTokenPonyEmbedReturnsNoSuchMethod(t *testing.T) {
	model := "x"
	_, err := newTokenPonyForTest("http://unused").Embed(&model, []string{"a"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Embed: want 'no such method', got %v", err)
	}
}

func TestTokenPonyAudioOCRReturnNoSuchMethod(t *testing.T) {
	m := newTokenPonyForTest("http://unused")
	model := "x"
	if _, err := m.TranscribeAudio(&model, &model, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudio: %v", err)
	}
	if _, err := m.AudioSpeech(&model, &model, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeech: %v", err)
	}
	if _, err := m.OCRFile(&model, nil, &model, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile: %v", err)
	}
}
