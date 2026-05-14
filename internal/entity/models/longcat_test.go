package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newLongCatServer(t *testing.T, expectedPath string, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
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
			// Accept "application/json" with or without a parameter
			// suffix like "; charset=utf-8" — both are valid JSON.
			if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
				t.Errorf("expected Content-Type to start with application/json, got %q", got)
				return
			}
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

func newLongCatForTest(baseURL string) *LongCatModel {
	return NewLongCatModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "openai/v1/chat/completions"},
	)
}

// newLongCatSSEServer returns an httptest.Server that asserts the
// request contract (POST + path + Authorization + Content-Type prefix)
// before writing the supplied SSE payload. Used by the streaming tests
// so a regression in the wire shape can't slip through unnoticed.
func newLongCatSSEServer(t *testing.T, expectedPath, ssePayload string) *httptest.Server {
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

func TestLongCatName(t *testing.T) {
	if got := newLongCatForTest("http://unused").Name(); got != "longcat" {
		t.Errorf("Name()=%q, want %q", got, "longcat")
	}
}

func TestLongCatChatHappyPath(t *testing.T) {
	srv := newLongCatServer(t, "/openai/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "LongCat-Flash-Chat" {
			t.Errorf("model=%v", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v want false", body["stream"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{"content": "pong"},
			}},
		})
	})
	defer srv.Close()

	m := newLongCatForTest(srv.URL)
	apiKey := "test-key"
	resp, err := m.ChatWithMessages("LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Answer == nil || resp.ReasonContent == nil {
		t.Fatalf("Answer/ReasonContent must be non-nil pointers, got Answer=%v ReasonContent=%v", resp.Answer, resp.ReasonContent)
	}
	if *resp.Answer != "pong" {
		t.Errorf("answer=%q want pong", *resp.Answer)
	}
	if *resp.ReasonContent != "" {
		t.Errorf("ReasonContent=%q want empty", *resp.ReasonContent)
	}
}

func TestLongCatChatExtractsReasoningContent(t *testing.T) {
	// LongCat-Flash-Thinking returns the chain-of-thought in
	// message.reasoning_content (OpenAI o-series shape). Live-probed
	// against api.longcat.chat; the fixture mimics the actual response
	// shape captured there.
	srv := newLongCatServer(t, "/openai/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "LongCat-Flash-Thinking" {
			t.Errorf("model=%v", body["model"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"role":              "assistant",
					"content":           "15% of 80 is 12.",
					"reasoning_content": "We need to compute 15% of 80. 0.15 * 80 = 12.",
				},
			}},
		})
	})
	defer srv.Close()

	m := newLongCatForTest(srv.URL)
	apiKey := "test-key"
	resp, err := m.ChatWithMessages("LongCat-Flash-Thinking",
		[]Message{{Role: "user", Content: "15% of 80?"}},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Answer == nil || resp.ReasonContent == nil {
		t.Fatalf("Answer/ReasonContent must be non-nil pointers, got Answer=%v ReasonContent=%v", resp.Answer, resp.ReasonContent)
	}
	if *resp.Answer != "15% of 80 is 12." {
		t.Errorf("Answer=%q", *resp.Answer)
	}
	if *resp.ReasonContent != "We need to compute 15% of 80. 0.15 * 80 = 12." {
		t.Errorf("ReasonContent=%q", *resp.ReasonContent)
	}
}

// TestLongCatChatDropsUndocumentedFields guards against re-introducing
// stop / reasoning_effort / response_format / tools etc. The LongCat
// docs only list model, messages, stream, max_tokens, temperature,
// top_p — anything else is undocumented and must not be sent, since
// the maintainer specifically flagged this on PR #14809.
func TestLongCatChatDropsUndocumentedFields(t *testing.T) {
	srv := newLongCatServer(t, "/openai/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		for _, k := range []string{"stop", "reasoning_effort", "response_format", "tools", "tool_choice", "presence_penalty", "frequency_penalty", "n", "logprobs"} {
			if _, present := body[k]; present {
				t.Errorf("undocumented field %q must not be sent: %v", k, body[k])
			}
		}
		// Documented fields, on the other hand, MUST be forwarded when set.
		for _, k := range []string{"model", "messages", "stream", "max_tokens", "temperature", "top_p"} {
			if _, present := body[k]; !present {
				t.Errorf("documented field %q missing from request body", k)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{"content": "ok"},
			}},
		})
	})
	defer srv.Close()

	m := newLongCatForTest(srv.URL)
	apiKey := "test-key"
	mt := 32
	temp := 0.7
	topP := 0.9
	stop := []string{"END"}
	effort := "high"
	_, err := m.ChatWithMessages("LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		// Deliberately pass Stop/Effort to prove they are filtered out.
		&ChatConfig{MaxTokens: &mt, Temperature: &temp, TopP: &topP, Stop: &stop, Effort: &effort})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestLongCatChatRequiresAPIKey(t *testing.T) {
	m := newLongCatForTest("http://unused")
	_, err := m.ChatWithMessages("LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestLongCatChatRequiresMessages(t *testing.T) {
	m := newLongCatForTest("http://unused")
	apiKey := "test-key"
	_, err := m.ChatWithMessages("LongCat-Flash-Chat", nil, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("expected messages-empty error, got %v", err)
	}
}

func TestLongCatChatRejectsHTTPError(t *testing.T) {
	srv := newLongCatServer(t, "/openai/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})
	defer srv.Close()

	m := newLongCatForTest(srv.URL)
	apiKey := "test-key"
	_, err := m.ChatWithMessages("LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 propagated, got %v", err)
	}
}

func TestLongCatStreamHappyPath(t *testing.T) {
	srv := newLongCatSSEServer(t, "/openai/v1/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"Hello"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	m := newLongCatForTest(srv.URL)
	apiKey := "test-key"
	var chunks []string
	var sawDone bool
	err := m.ChatStreamlyWithSender("LongCat-Flash-Chat",
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

func TestLongCatStreamExtractsReasoningContent(t *testing.T) {
	// Fixture matches the shape captured live from
	// LongCat-Flash-Thinking against api.longcat.chat: deltas
	// interleave reasoning_content and content within the stream.
	srv := newLongCatSSEServer(t, "/openai/v1/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"reasoning_content":"step 1. "}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"reasoning_content":"step 2."}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"final answer"},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	m := newLongCatForTest(srv.URL)
	apiKey := "test-key"
	var content, reasoning []string
	err := m.ChatStreamlyWithSender("LongCat-Flash-Thinking",
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
	if got := strings.Join(content, ""); got != "final answer" {
		t.Errorf("content=%q", got)
	}
}

func TestLongCatStreamRejectsExplicitFalse(t *testing.T) {
	m := newLongCatForTest("http://unused")
	apiKey := "test-key"
	stream := false
	err := m.ChatStreamlyWithSender("LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Errorf("expected stream-true guard, got %v", err)
	}
}

func TestLongCatStreamRequiresSender(t *testing.T) {
	m := newLongCatForTest("http://unused")
	apiKey := "test-key"
	err := m.ChatStreamlyWithSender("LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("expected sender-required error, got %v", err)
	}
}

func TestLongCatStreamFailsWithoutTerminal(t *testing.T) {
	srv := newLongCatSSEServer(t, "/openai/v1/chat/completions",
		`data: {"choices":[{"delta":{"content":"half"}}]}`+"\n",
	)
	defer srv.Close()

	m := newLongCatForTest(srv.URL)
	apiKey := "test-key"
	err := m.ChatStreamlyWithSender("LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream ended before") {
		t.Errorf("expected truncation error, got %v", err)
	}
}

// A malformed SSE frame (invalid JSON) used to be silently skipped,
// which masked truncated or corrupted streams. The driver must now
// fail hard with a "longcat: invalid SSE event" wrapper.
func TestLongCatStreamRejectsMalformedFrame(t *testing.T) {
	srv := newLongCatSSEServer(t, "/openai/v1/chat/completions",
		`data: {"choices":[{"delta":{"content":"ok"}}]}`+"\n"+
			`data: {this is not valid json}`+"\n",
	)
	defer srv.Close()

	m := newLongCatForTest(srv.URL)
	apiKey := "test-key"
	err := m.ChatStreamlyWithSender("LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "invalid SSE event") {
		t.Errorf("expected invalid-SSE error, got %v", err)
	}
}

// An upstream {"error": ...} frame mid-stream used to fall through to
// the "no choices" continue and leave the caller with a generic
// truncation error. The driver must surface the upstream error verbatim.
func TestLongCatStreamSurfacesUpstreamError(t *testing.T) {
	srv := newLongCatSSEServer(t, "/openai/v1/chat/completions",
		`data: {"choices":[{"delta":{"content":"partial "}}]}`+"\n"+
			`data: {"error":{"message":"rate limit exceeded","type":"rate_limit_error"}}`+"\n",
	)
	defer srv.Close()

	m := newLongCatForTest(srv.URL)
	apiKey := "test-key"
	err := m.ChatStreamlyWithSender("LongCat-Flash-Chat",
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

// LongCat does not document /models or /health endpoints, so per
// maintainer guidance ListModels and CheckConnection both return the
// "no such method" sentinel rather than inventing fake catalogs or
// burning chat completions for connection checks.
func TestLongCatListModelsReturnsNoSuchMethod(t *testing.T) {
	apiKey := "test-key"
	_, err := newLongCatForTest("http://unused").ListModels(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ListModels: want 'no such method', got %v", err)
	}
}

func TestLongCatCheckConnectionReturnsNoSuchMethod(t *testing.T) {
	apiKey := "test-key"
	err := newLongCatForTest("http://unused").CheckConnection(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("CheckConnection: want 'no such method', got %v", err)
	}
}

func TestLongCatEmbedReturnsNoSuchMethod(t *testing.T) {
	m := newLongCatForTest("http://unused")
	model := "x"
	_, err := m.Embed(&model, []string{"a"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Embed: want 'no such method', got %v", err)
	}
}

func TestLongCatRerankReturnsNoSuchMethod(t *testing.T) {
	m := newLongCatForTest("http://unused")
	model := "x"
	_, err := m.Rerank(&model, "q", []string{"a"}, &APIConfig{}, &RerankConfig{TopN: 1})
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank: want 'no such method', got %v", err)
	}
}

func TestLongCatBalanceReturnsNoSuchMethod(t *testing.T) {
	m := newLongCatForTest("http://unused")
	_, err := m.Balance(&APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance: want 'no such method', got %v", err)
	}
}

func TestLongCatAudioOCRReturnNoSuchMethod(t *testing.T) {
	m := newLongCatForTest("http://unused")
	model := "x"
	if _, err := m.TranscribeAudio(&model, &model, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudio: want 'no such method', got %v", err)
	}
	if _, err := m.AudioSpeech(&model, &model, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeech: want 'no such method', got %v", err)
	}
	if _, err := m.OCRFile(&model, nil, &model, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile: want 'no such method', got %v", err)
	}
}
