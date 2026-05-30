package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newHunyuanServer(t *testing.T, expectedMethod, expectedPath string, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
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
		handler(t, nil, w)
	}))
}

func newHunyuanSSEServer(t *testing.T, expectedPath, ssePayload string) *httptest.Server {
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

func newHunyuanForTest(baseURL string) *HunyuanModel {
	return NewHunyuanModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Embedding: "embeddings", Models: "models"},
	)
}

func TestHunyuanName(t *testing.T) {
	if got := newHunyuanForTest("http://unused").Name(); got != "hunyuan" {
		t.Errorf("Name()=%q, want %q", got, "hunyuan")
	}
}

func TestHunyuanFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("Hunyuan", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*HunyuanModel); !ok {
		t.Fatalf("driver type=%T, want *HunyuanModel", driver)
	}
}

func TestHunyuanChatHappyPath(t *testing.T) {
	srv := newHunyuanServer(t, http.MethodPost, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "hunyuan-pro" {
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
	resp, err := newHunyuanForTest(srv.URL).ChatWithMessages(
		"hunyuan-pro",
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

func TestHunyuanChatNoReasoning(t *testing.T) {
	srv := newHunyuanServer(t, http.MethodPost, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{"content": "hi"},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newHunyuanForTest(srv.URL).ChatWithMessages(
		"hunyuan-lite",
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

func TestHunyuanChatRequiresAPIKey(t *testing.T) {
	_, err := newHunyuanForTest("http://unused").ChatWithMessages(
		"hunyuan-pro",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestHunyuanChatRequiresMessages(t *testing.T) {
	apiKey := "test-key"
	_, err := newHunyuanForTest("http://unused").ChatWithMessages(
		"hunyuan-pro", nil, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("expected messages-empty error, got %v", err)
	}
}

func TestHunyuanChatPropagatesHTTPError(t *testing.T) {
	srv := newHunyuanServer(t, http.MethodPost, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newHunyuanForTest(srv.URL).ChatWithMessages(
		"hunyuan-pro",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 propagated, got %v", err)
	}
}

func TestHunyuanStreamHappyPath(t *testing.T) {
	srv := newHunyuanSSEServer(t, "/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"Hello"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	var chunks []string
	var sawDone bool
	err := newHunyuanForTest(srv.URL).ChatStreamlyWithSender(
		"hunyuan-pro",
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

func TestHunyuanStreamSplitsReasoning(t *testing.T) {
	srv := newHunyuanSSEServer(t, "/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"reasoning_content":"step 1. "}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"reasoning_content":"step 2."}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"final"},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	var content, reasoning []string
	err := newHunyuanForTest(srv.URL).ChatStreamlyWithSender(
		"hunyuan-standard-256K",
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

func TestHunyuanStreamRejectsExplicitFalse(t *testing.T) {
	apiKey := "test-key"
	stream := false
	err := newHunyuanForTest("http://unused").ChatStreamlyWithSender(
		"hunyuan-pro",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Errorf("expected stream-true guard, got %v", err)
	}
}

func TestHunyuanStreamRequiresSender(t *testing.T) {
	apiKey := "test-key"
	err := newHunyuanForTest("http://unused").ChatStreamlyWithSender(
		"hunyuan-pro",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("expected sender-required error, got %v", err)
	}
}

func TestHunyuanStreamFailsWithoutTerminal(t *testing.T) {
	srv := newHunyuanSSEServer(t, "/chat/completions",
		`data: {"choices":[{"delta":{"content":"half"}}]}`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	err := newHunyuanForTest(srv.URL).ChatStreamlyWithSender(
		"hunyuan-pro",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream ended before") {
		t.Errorf("expected truncation error, got %v", err)
	}
}

func TestHunyuanStreamRejectsMalformedFrame(t *testing.T) {
	srv := newHunyuanSSEServer(t, "/chat/completions",
		`data: {"choices":[{"delta":{"content":"ok"}}]}`+"\n"+
			`data: {oops not json}`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	err := newHunyuanForTest(srv.URL).ChatStreamlyWithSender(
		"hunyuan-pro",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "invalid SSE event") {
		t.Errorf("expected invalid-SSE error, got %v", err)
	}
}

func TestHunyuanStreamSurfacesUpstreamError(t *testing.T) {
	srv := newHunyuanSSEServer(t, "/chat/completions",
		`data: {"choices":[{"delta":{"content":"partial "}}]}`+"\n"+
			`data: {"error":{"message":"rate limit","type":"rate_limit_error"}}`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	err := newHunyuanForTest(srv.URL).ChatStreamlyWithSender(
		"hunyuan-pro",
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

func TestHunyuanListModelsHappyPath(t *testing.T) {
	srv := newHunyuanServer(t, http.MethodGet, "/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "hunyuan-pro"},
				{"id": "hunyuan-standard"},
				{"id": "hunyuan-standard-256K"},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	models, err := newHunyuanForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	want := []string{"hunyuan-pro", "hunyuan-standard", "hunyuan-standard-256K"}
	if strings.Join(models, ",") != strings.Join(want, ",") {
		t.Errorf("models=%v, want %v", models, want)
	}
}

func TestHunyuanListModelsRequiresAPIKey(t *testing.T) {
	_, err := newHunyuanForTest("http://unused").ListModels(&APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestHunyuanCheckConnectionDelegatesToListModels(t *testing.T) {
	srv := newHunyuanServer(t, http.MethodGet, "/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{{"id": "hunyuan-pro"}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	if err := newHunyuanForTest(srv.URL).CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Errorf("CheckConnection: %v", err)
	}
}

func TestHunyuanCheckConnectionPropagatesError(t *testing.T) {
	srv := newHunyuanServer(t, http.MethodGet, "/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	err := newHunyuanForTest(srv.URL).CheckConnection(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 propagated, got %v", err)
	}
}

func TestHunyuanBaseURLForRegionUnknown(t *testing.T) {
	m := newHunyuanForTest("http://unused")
	apiKey := "test-key"
	region := "missing"
	_, err := m.ListModels(&APIConfig{ApiKey: &apiKey, Region: &region})
	if err == nil || !strings.Contains(err.Error(), "no base URL configured") {
		t.Errorf("expected base-URL error, got %v", err)
	}
}

func TestHunyuanEmbedHappyPath(t *testing.T) {
	srv := newHunyuanServer(t, http.MethodPost, "/embeddings", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "hunyuan-embedding" {
			t.Errorf("model=%v", body["model"])
		}
		inputs, ok := body["input"].([]interface{})
		if !ok || len(inputs) != 2 {
			t.Errorf("input=%#v", body["input"])
			http.Error(w, "bad input", http.StatusBadRequest)
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
	model := "hunyuan-embedding"
	embeddings, err := newHunyuanForTest(srv.URL).Embed(&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(embeddings) != 2 || embeddings[1].Index != 1 || embeddings[1].Embedding[0] != 0.3 {
		t.Errorf("embeddings=%+v", embeddings)
	}
}

func TestHunyuanEmbedValidatesInputs(t *testing.T) {
	apiKey := "test-key"
	model := "hunyuan-embedding"

	if embeddings, err := newHunyuanForTest("http://unused").Embed(nil, nil, nil, nil); err != nil || len(embeddings) != 0 {
		t.Errorf("empty input: embeddings=%+v err=%v", embeddings, err)
	}
	if _, err := newHunyuanForTest("http://unused").Embed(nil, []string{"x"}, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("nil model: %v", err)
	}
	emptyModel := ""
	if _, err := newHunyuanForTest("http://unused").Embed(&emptyModel, []string{"x"}, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("empty model: %v", err)
	}
	if _, err := newHunyuanForTest("http://unused").Embed(&model, []string{"x"}, nil, nil); err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("nil api config: %v", err)
	}
	if _, err := newHunyuanForTest("http://unused").Embed(&model, []string{"x"}, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("missing api key: %v", err)
	}
	emptyKey := ""
	if _, err := newHunyuanForTest("http://unused").Embed(&model, []string{"x"}, &APIConfig{ApiKey: &emptyKey}, nil); err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("empty api key: %v", err)
	}
}

func TestHunyuanAudioOCRReturnNoSuchMethod(t *testing.T) {
	m := newHunyuanForTest("http://unused")
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
