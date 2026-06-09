package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newAstraflowServer(t *testing.T, expectedPath string, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
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

func newAstraflowSSEServer(t *testing.T, expectedPath, ssePayload string) *httptest.Server {
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

func newAstraflowForTest(baseURL string) *AstraflowModel {
	return NewAstraflowModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestAstraflowName(t *testing.T) {
	if got := newAstraflowForTest("http://unused").Name(); got != "astraflow" {
		t.Errorf("Name()=%q, want %q", got, "astraflow")
	}
}

func TestAstraflowFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("Astraflow", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*AstraflowModel); !ok {
		t.Fatalf("driver type=%T, want *AstraflowModel", driver)
	}
}

func TestAstraflowChatHappyPath(t *testing.T) {
	srv := newAstraflowServer(t, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "claude-opus-4-7" {
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
	resp, err := newAstraflowForTest(srv.URL).ChatWithMessages(
		"claude-opus-4-7",
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

func TestAstraflowChatNoReasoning(t *testing.T) {
	srv := newAstraflowServer(t, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{"content": "hi"},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newAstraflowForTest(srv.URL).ChatWithMessages(
		"gpt-4o-mini",
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

func TestAstraflowChatRequiresAPIKey(t *testing.T) {
	_, err := newAstraflowForTest("http://unused").ChatWithMessages(
		"claude-opus-4-7",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestAstraflowChatRequiresMessages(t *testing.T) {
	apiKey := "test-key"
	_, err := newAstraflowForTest("http://unused").ChatWithMessages(
		"claude-opus-4-7", nil, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("expected messages-empty error, got %v", err)
	}
}

func TestAstraflowChatPropagatesHTTPError(t *testing.T) {
	srv := newAstraflowServer(t, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newAstraflowForTest(srv.URL).ChatWithMessages(
		"claude-opus-4-7",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 propagated, got %v", err)
	}
}

func TestAstraflowStreamHappyPath(t *testing.T) {
	srv := newAstraflowSSEServer(t, "/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"Hello"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	var chunks []string
	var sawDone bool
	err := newAstraflowForTest(srv.URL).ChatStreamlyWithSender(
		"claude-opus-4-7",
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

func TestAstraflowStreamSplitsReasoning(t *testing.T) {
	srv := newAstraflowSSEServer(t, "/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"reasoning_content":"step 1. "}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"reasoning_content":"step 2."}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"final"},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	var content, reasoning []string
	err := newAstraflowForTest(srv.URL).ChatStreamlyWithSender(
		"kimi-k2.6",
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

func TestAstraflowStreamRejectsExplicitFalse(t *testing.T) {
	apiKey := "test-key"
	stream := false
	err := newAstraflowForTest("http://unused").ChatStreamlyWithSender(
		"claude-opus-4-7",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Errorf("expected stream-true guard, got %v", err)
	}
}

func TestAstraflowStreamRequiresSender(t *testing.T) {
	apiKey := "test-key"
	err := newAstraflowForTest("http://unused").ChatStreamlyWithSender(
		"claude-opus-4-7",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("expected sender-required error, got %v", err)
	}
}

func TestAstraflowStreamFailsWithoutTerminal(t *testing.T) {
	srv := newAstraflowSSEServer(t, "/chat/completions",
		`data: {"choices":[{"delta":{"content":"half"}}]}`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	err := newAstraflowForTest(srv.URL).ChatStreamlyWithSender(
		"claude-opus-4-7",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream ended before") {
		t.Errorf("expected truncation error, got %v", err)
	}
}

func TestAstraflowStreamRejectsMalformedFrame(t *testing.T) {
	srv := newAstraflowSSEServer(t, "/chat/completions",
		`data: {"choices":[{"delta":{"content":"ok"}}]}`+"\n"+
			`data: {oops not json}`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	err := newAstraflowForTest(srv.URL).ChatStreamlyWithSender(
		"claude-opus-4-7",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "invalid SSE event") {
		t.Errorf("expected invalid-SSE error, got %v", err)
	}
}

func TestAstraflowStreamSurfacesUpstreamError(t *testing.T) {
	srv := newAstraflowSSEServer(t, "/chat/completions",
		`data: {"choices":[{"delta":{"content":"partial "}}]}`+"\n"+
			`data: {"error":{"message":"rate limit","type":"rate_limit_error"}}`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	err := newAstraflowForTest(srv.URL).ChatStreamlyWithSender(
		"claude-opus-4-7",
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

func TestAstraflowListModelsHappyPath(t *testing.T) {
	srv := newAstraflowServer(t, "/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "claude-opus-4-7"},
				{"id": "gpt-5.4"},
				{"id": "Qwen/Qwen3-Max"},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	models, err := newAstraflowForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	want := []string{"claude-opus-4-7", "gpt-5.4", "Qwen/Qwen3-Max"}
	if joinModelNames(models, ",") != strings.Join(want, ",") {
		t.Errorf("models=%v, want %v", models, want)
	}
}

func TestAstraflowListModelsRequiresAPIKey(t *testing.T) {
	_, err := newAstraflowForTest("http://unused").ListModels(&APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestAstraflowCheckConnectionDelegatesToListModels(t *testing.T) {
	srv := newAstraflowServer(t, "/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{{"id": "claude-opus-4-7"}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	if err := newAstraflowForTest(srv.URL).CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Errorf("CheckConnection: %v", err)
	}
}

func TestAstraflowCheckConnectionPropagatesError(t *testing.T) {
	srv := newAstraflowServer(t, "/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	err := newAstraflowForTest(srv.URL).CheckConnection(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 propagated, got %v", err)
	}
}

func TestAstraflowBaseURLForRegionUnknown(t *testing.T) {
	m := newAstraflowForTest("http://unused")
	apiKey := "test-key"
	region := "missing"
	_, err := m.ListModels(&APIConfig{ApiKey: &apiKey, Region: &region})
	if err == nil || !strings.Contains(err.Error(), "no base URL configured") {
		t.Errorf("expected base-URL error, got %v", err)
	}
}

func TestAstraflowEmbedReturnsNoSuchMethod(t *testing.T) {
	// Embed IS implemented (not a stub). It should NOT be blocked by APIConfigCheck.
	// With empty input texts it short-circuits to empty result (no error).
	apiKey := "test-key"
	embeddings, err := newAstraflowForTest("http://unused").Embed(nil, nil, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil || len(embeddings) != 0 {
		t.Errorf("Embed: want empty result (no error), got embeddings=%v err=%v", embeddings, err)
	}
}

func TestAstraflowAudioOCRReturnNoSuchMethod(t *testing.T) {
	m := newAstraflowForTest("http://unused")
	model := "x"
	apiKey := "test-key"
	// TranscribeAudio is a stub → "no such method"
	if _, err := m.TranscribeAudio(&model, &model, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudio: %v", err)
	}
	// AudioSpeech IS implemented; pass nil content to hit input validation,
	// not api-key check (which would mean APIConfigCheck still blocks it).
	if _, err := m.AudioSpeech(&model, nil, &APIConfig{ApiKey: &apiKey}, nil); err == nil || strings.Contains(err.Error(), "api key is required") {
		t.Errorf("AudioSpeech: expected non-api-key error, got %v", err)
	}
	// OCRFile is a stub → "no such method"
	if _, err := m.OCRFile(&model, nil, &model, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile: %v", err)
	}
}
