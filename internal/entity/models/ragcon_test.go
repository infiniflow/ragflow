package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newRAGconServer asserts the JSON wire contract (path, Authorization,
// Content-Type) shared by the POST chat/embed paths and the GET
// ListModels path, then delegates the response to handler.
func newRAGconServer(t *testing.T, expectedPath string, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
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
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("expected Content-Type to start with application/json, got %q", got)
			return
		}
		if r.Method == http.MethodPost {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
				return
			}
			var body map[string]interface{}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("unmarshal: %v\nraw=%s", err, string(raw))
				return
			}
			handler(t, body, w)
			return
		}
		handler(t, nil, w)
	}))
}

// newRAGconSSEServer asserts the SSE-chat request shape, then writes the
// supplied SSE payload.
func newRAGconSSEServer(t *testing.T, expectedPath, ssePayload string) *httptest.Server {
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

func newRAGconForTest(baseURL string) *RAGconModel {
	return NewRAGconModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models", Embedding: "embeddings"},
	)
}

func TestRAGconName(t *testing.T) {
	if got := newRAGconForTest("http://unused").Name(); got != "ragcon" {
		t.Errorf("Name()=%q", got)
	}
}

func TestRAGconChatHappyPath(t *testing.T) {
	srv := newRAGconServer(t, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "gpt-4o-mini" {
			t.Errorf("model=%v", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v, want false", body["stream"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{"content": "pong"},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newRAGconForTest(srv.URL).ChatWithMessages(
		"gpt-4o-mini",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if *resp.Answer != "pong" || *resp.ReasonContent != "" {
		t.Errorf("(%q,%q)", *resp.Answer, *resp.ReasonContent)
	}
}

func TestRAGconChatExtractsReasoningContent(t *testing.T) {
	srv := newRAGconServer(t, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"role":              "assistant",
					"content":           "4",
					"reasoning_content": "2+2 is 4.",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newRAGconForTest(srv.URL).ChatWithMessages(
		"deepseek-r1",
		[]Message{{Role: "user", Content: "2+2?"}},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if *resp.Answer != "4" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
	if *resp.ReasonContent != "2+2 is 4." {
		t.Errorf("ReasonContent=%q", *resp.ReasonContent)
	}
}

func TestRAGconChatRequiresAPIKey(t *testing.T) {
	_, err := newRAGconForTest("http://unused").ChatWithMessages("m",
		[]Message{{Role: "user", Content: "x"}}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("got %v", err)
	}
}

func TestRAGconChatRequiresModelName(t *testing.T) {
	apiKey := "test-key"
	_, err := newRAGconForTest("http://unused").ChatWithMessages("  ",
		[]Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("got %v", err)
	}
}

func TestRAGconChatRequiresMessages(t *testing.T) {
	apiKey := "test-key"
	_, err := newRAGconForTest("http://unused").ChatWithMessages("m", nil,
		&APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("got %v", err)
	}
}

func TestRAGconChatRejectsHTTPError(t *testing.T) {
	srv := newRAGconServer(t, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newRAGconForTest(srv.URL).ChatWithMessages("m",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("got %v", err)
	}
}

func TestRAGconStreamHappyPath(t *testing.T) {
	srv := newRAGconSSEServer(t, "/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"Hello "}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"world"},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	var chunks []string
	var sawDone bool
	err := newRAGconForTest(srv.URL).ChatStreamlyWithSender("gpt-4o-mini",
		[]Message{{Role: "user", Content: "x"}},
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

func TestRAGconStreamForwardsReasoningContent(t *testing.T) {
	srv := newRAGconSSEServer(t, "/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"reasoning_content":"step 1. "}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"reasoning_content":"step 2."}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"answer"},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	var content, reasoning []string
	err := newRAGconForTest(srv.URL).ChatStreamlyWithSender("deepseek-r1",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(c *string, rc *string) error {
			if c != nil && rc != nil {
				t.Errorf("sender called with both args non-nil")
			}
			if rc != nil && *rc != "" {
				reasoning = append(reasoning, *rc)
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
	if got := strings.Join(content, ""); got != "answer" {
		t.Errorf("content=%q", got)
	}
}

func TestRAGconStreamRequiresSender(t *testing.T) {
	apiKey := "test-key"
	err := newRAGconForTest("http://unused").ChatStreamlyWithSender("m",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("got %v", err)
	}
}

func TestRAGconStreamRejectsExplicitFalse(t *testing.T) {
	apiKey := "test-key"
	stream := false
	err := newRAGconForTest("http://unused").ChatStreamlyWithSender("m",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Errorf("got %v", err)
	}
}

// A stream that closes without [DONE] or a finish_reason is a truncated
// response; the driver must surface an error rather than emit a synthetic
// terminal marker.
func TestRAGconStreamErrorsOnTruncation(t *testing.T) {
	srv := newRAGconSSEServer(t, "/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"content":"partial"}}]}`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	err := newRAGconForTest(srv.URL).ChatStreamlyWithSender("m",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream ended before") {
		t.Errorf("got %v", err)
	}
}

func TestRAGconListModelsHappyPath(t *testing.T) {
	srv := newRAGconServer(t, "/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "gpt-4o-mini"},
				{"id": "text-embedding-3-small"},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	ids, err := newRAGconForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("ids=%v", ids)
	}
}

func TestRAGconCheckConnection(t *testing.T) {
	srv := newRAGconServer(t, "/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]interface{}{{"id": "x"}}})
	})
	defer srv.Close()

	apiKey := "test-key"
	if err := newRAGconForTest(srv.URL).CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Errorf("CheckConnection: %v", err)
	}
}

func TestRAGconEmbedHappyPath(t *testing.T) {
	srv := newRAGconServer(t, "/embeddings", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "text-embedding-3-small" {
			t.Errorf("model=%v", body["model"])
		}
		// Return out of order to verify the driver re-orders by index.
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"index": 1, "embedding": []float64{0.3, 0.4}},
				{"index": 0, "embedding": []float64{0.1, 0.2}},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := "text-embedding-3-small"
	out, err := newRAGconForTest(srv.URL).Embed(&model, []string{"a", "b"},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0].Embedding[0] != 0.1 || out[1].Embedding[0] != 0.3 {
		t.Errorf("out of order: %v", out)
	}
}

func TestRAGconEmbedEmptyTexts(t *testing.T) {
	model := "text-embedding-3-small"
	out, err := newRAGconForTest("http://unused").Embed(&model, nil, &APIConfig{}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty, got %v", out)
	}
}

func TestRAGconEmbedRequiresAPIKey(t *testing.T) {
	model := "m"
	_, err := newRAGconForTest("http://unused").Embed(&model, []string{"a"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("got %v", err)
	}
}

func TestRAGconRerankReturnsNoSuchMethod(t *testing.T) {
	m := "x"
	_, err := newRAGconForTest("http://unused").Rerank(&m, "q", []string{"a"}, &APIConfig{}, &RerankConfig{TopN: 1})
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("got %v", err)
	}
}

func TestRAGconBalanceReturnsNoSuchMethod(t *testing.T) {
	if _, err := newRAGconForTest("http://unused").Balance(&APIConfig{}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("got %v", err)
	}
}

func TestRAGconAudioOCRReturnNoSuchMethod(t *testing.T) {
	m := "x"
	v := newRAGconForTest("http://unused")
	if _, err := v.TranscribeAudio(&m, &m, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudio: %v", err)
	}
	if _, err := v.AudioSpeech(&m, &m, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeech: %v", err)
	}
	if _, err := v.OCRFile(&m, nil, &m, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile: %v", err)
	}
}

// TestRAGconBaseURLTrimsTrailingSlash pins that a base URL configured
// with a trailing "/" still produces clean single-slash paths.
func TestRAGconBaseURLTrimsTrailingSlash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%q want /chat/completions (double-slash bug?)", r.URL.Path)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	m := NewRAGconModel(
		map[string]string{"default": srv.URL + "/"},
		URLSuffix{Chat: "chat/completions"},
	)
	apiKey := "test-key"
	if _, err := m.ChatWithMessages("gpt-4o-mini",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil); err != nil {
		t.Fatalf("Chat: %v", err)
	}
}
