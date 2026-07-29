package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newLongCatServer(t *testing.T, expectedPath string, handler func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
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
			handler(t, r, body, w)
			return
		}
		handler(t, r, nil, w)
	}))
}

func newLongCatForTest(baseURL string) *LongCatModel {
	return NewLongCatModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "openai/v1/chat/completions", Models: "openai/v1/models"},
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

func TestLongCatNewModelWithCustomDefaultTransport(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = original
	})

	if model := NewLongCatModel(map[string]string{"default": "http://unused"}, URLSuffix{}); model == nil {
		t.Fatal("NewLongCatModel returned nil")
	}
}

func TestLongCatChatHappyPath(t *testing.T) {
	ctx := t.Context()
	srv := newLongCatServer(t, "/openai/v1/chat/completions", func(t *testing.T, _ *http.Request, body map[string]interface{}, w http.ResponseWriter) {
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
	resp, err := m.ChatWithMessages(ctx, "LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
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
	ctx := t.Context()
	// LongCat-Flash-Thinking returns the chain-of-thought in
	// message.reasoning_content (OpenAI o-series shape). Live-probed
	// against api.longcat.chat; the fixture mimics the actual response
	// shape captured there.
	srv := newLongCatServer(t, "/openai/v1/chat/completions", func(t *testing.T, _ *http.Request, body map[string]interface{}, w http.ResponseWriter) {
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
	resp, err := m.ChatWithMessages(ctx, "LongCat-Flash-Thinking",
		[]Message{{Role: "user", Content: "15% of 80?"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
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

// TestLongCatChatParsesUsage verifies that the typed LongCat response
// parser extracts the OpenAI-compatible usage block into ChatResponse.Usage.
// LongCat always returns usage on success, so the field must be populated.
func TestLongCatChatParsesUsage(t *testing.T) {
	ctx := t.Context()
	srv := newLongCatServer(t, "/openai/v1/chat/completions", func(t *testing.T, _ *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chatcmpl-abc123",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "LongCat-2.0",
			"choices": []map[string]interface{}{{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "Hello!",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]interface{}{
				"prompt_tokens":     20,
				"completion_tokens": 15,
				"total_tokens":      35,
			},
		})
	})
	defer srv.Close()

	m := newLongCatForTest(srv.URL)
	apiKey := "test-key"
	resp, err := m.ChatWithMessages(ctx, "LongCat-2.0",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Usage == nil {
		t.Fatal("Usage must be non-nil")
	}
	if resp.Usage.PromptTokens != 20 {
		t.Errorf("PromptTokens=%d, want 20", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 15 {
		t.Errorf("CompletionTokens=%d, want 15", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 35 {
		t.Errorf("TotalTokens=%d, want 35", resp.Usage.TotalTokens)
	}
}

// TestLongCatChatAcceptsReasoningOnlyResponse verifies that a response with
// null content but non-empty reasoning_content is accepted. LongCat's
// thinking model can emit all output as reasoning_content, leaving content
// null — that is a valid response, not an error.
func TestLongCatChatAcceptsReasoningOnlyResponse(t *testing.T) {
	ctx := t.Context()
	srv := newLongCatServer(t, "/openai/v1/chat/completions", func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "cmpl_reasoning_only",
			"object": "chat.completion",
			"model":  "LongCat-2.0",
			"choices": []map[string]interface{}{{
				"index": 0,
				"message": map[string]interface{}{
					"role":              "assistant",
					"content":           nil,
					"reasoning_content": "\nThe answer is 4.",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]interface{}{
				"prompt_tokens":     5,
				"completion_tokens": 10,
				"total_tokens":      15,
			},
		})
	})
	defer srv.Close()

	m := newLongCatForTest(srv.URL)
	apiKey := "test-key"
	resp, err := m.ChatWithMessages(ctx, "LongCat-2.0",
		[]Message{{Role: "user", Content: "what is 2+2"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v (should not error on reasoning-only response)", err)
	}
	if resp.Answer == nil {
		t.Error("Answer must be non-nil")
	} else if *resp.Answer != "" {
		t.Errorf("Answer=%q, want empty string", *resp.Answer)
	}
	if resp.ReasonContent == nil {
		t.Error("ReasonContent must be non-nil")
	} else if *resp.ReasonContent != "The answer is 4." {
		t.Errorf("ReasonContent=%q, want 'The answer is 4.'", *resp.ReasonContent)
	}
	if resp.Usage == nil {
		t.Error("Usage must be non-nil")
	} else if resp.Usage.TotalTokens != 15 {
		t.Errorf("Usage=%#v, want total=15", resp.Usage)
	}
}

// TestLongCatStreamRequestsIncludeUsage verifies that the streaming driver
// asks LongCat for aggregate token usage via stream_options.include_usage,
// and that the usage event populates chatConfig.UsageResult.
func TestLongCatStreamRequestsIncludeUsage(t *testing.T) {
	ctx := t.Context()
	srv := newLongCatServer(t, "/openai/v1/chat/completions", func(t *testing.T, _ *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		streamOpts, ok := body["stream_options"].(map[string]interface{})
		if !ok {
			t.Errorf("stream_options missing: %v", body)
			return
		}
		if inc, _ := streamOpts["include_usage"].(bool); !inc {
			t.Errorf("stream_options.include_usage=%v, want true", streamOpts["include_usage"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"index":0,"delta":{"content":"hi"}}]}`+"\n"+
				`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`+"\n"+
				`data: [DONE]`+"\n")
	})
	defer srv.Close()

	m := newLongCatForTest(srv.URL)
	apiKey := "test-key"
	chatConfig := &ChatConfig{}
	err := m.ChatStreamlyWithSender(ctx, "LongCat-2.0",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, chatConfig, nil,
		func(*string, *string) error { return nil })
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if chatConfig.UsageResult == nil {
		t.Fatal("UsageResult must be non-nil after stream with usage event")
	}
	if chatConfig.UsageResult.PromptTokens != 10 || chatConfig.UsageResult.CompletionTokens != 2 || chatConfig.UsageResult.TotalTokens != 12 {
		t.Errorf("UsageResult=%#v, want prompt=10 completion=2 total=12", chatConfig.UsageResult)
	}
}

// TestLongCatChatDropsUndocumentedFields guards against re-introducing
// stop / reasoning_effort / response_format / tools etc. The LongCat
// docs only list model, messages, stream, max_tokens, temperature,
// top_p — anything else is undocumented and must not be sent, since
// the maintainer specifically flagged this on PR #14809.
func TestLongCatChatDropsUndocumentedFields(t *testing.T) {
	ctx := t.Context()
	srv := newLongCatServer(t, "/openai/v1/chat/completions", func(t *testing.T, _ *http.Request, body map[string]interface{}, w http.ResponseWriter) {
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
	_, err := m.ChatWithMessages(ctx, "LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		// Deliberately pass Stop/Effort to prove they are filtered out.
		&ChatConfig{MaxTokens: &mt, Temperature: &temp, TopP: &topP, Stop: &stop, Effort: &effort},
		nil,
	)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestLongCatChatRequiresAPIKey(t *testing.T) {
	ctx := t.Context()
	m := newLongCatForTest("http://unused")
	_, err := m.ChatWithMessages(ctx, "LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestLongCatChatRequiresMessages(t *testing.T) {
	ctx := t.Context()
	m := newLongCatForTest("http://unused")
	apiKey := "test-key"
	_, err := m.ChatWithMessages(ctx, "LongCat-Flash-Chat", nil, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("expected messages-empty error, got %v", err)
	}
}

func TestLongCatChatRejectsHTTPError(t *testing.T) {
	ctx := t.Context()
	srv := newLongCatServer(t, "/openai/v1/chat/completions", func(t *testing.T, _ *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})
	defer srv.Close()

	m := newLongCatForTest(srv.URL)
	apiKey := "test-key"
	_, err := m.ChatWithMessages(ctx, "LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 propagated, got %v", err)
	}
}

func TestLongCatStreamHappyPath(t *testing.T) {
	ctx := t.Context()
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
	err := m.ChatStreamlyWithSender(ctx, "LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
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
	ctx := t.Context()
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
	err := m.ChatStreamlyWithSender(ctx, "LongCat-Flash-Thinking",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
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
	ctx := t.Context()
	m := newLongCatForTest("http://unused")
	apiKey := "test-key"
	stream := false
	err := m.ChatStreamlyWithSender(ctx, "LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Errorf("expected stream-true guard, got %v", err)
	}
}

func TestLongCatStreamRequiresSender(t *testing.T) {
	ctx := t.Context()
	m := newLongCatForTest("http://unused")
	apiKey := "test-key"
	err := m.ChatStreamlyWithSender(ctx, "LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("expected sender-required error, got %v", err)
	}
}

func TestLongCatStreamFailsWithoutTerminal(t *testing.T) {
	ctx := t.Context()
	srv := newLongCatSSEServer(t, "/openai/v1/chat/completions",
		`data: {"choices":[{"delta":{"content":"half"}}]}`+"\n",
	)
	defer srv.Close()

	m := newLongCatForTest(srv.URL)
	apiKey := "test-key"
	err := m.ChatStreamlyWithSender(ctx, "LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream ended before") {
		t.Errorf("expected truncation error, got %v", err)
	}
}

// A malformed SSE frame (invalid JSON) used to be silently skipped,
// which masked truncated or corrupted streams. The driver must now
// fail hard with a "longcat: invalid SSE event" wrapper.
func TestLongCatStreamRejectsMalformedFrame(t *testing.T) {
	ctx := t.Context()
	srv := newLongCatSSEServer(t, "/openai/v1/chat/completions",
		`data: {"choices":[{"delta":{"content":"ok"}}]}`+"\n"+
			`data: {this is not valid json}`+"\n",
	)
	defer srv.Close()

	m := newLongCatForTest(srv.URL)
	apiKey := "test-key"
	err := m.ChatStreamlyWithSender(ctx, "LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "invalid SSE event") {
		t.Errorf("expected invalid-SSE error, got %v", err)
	}
}

// An upstream {"error": ...} frame mid-stream used to fall through to
// the "no choices" continue and leave the caller with a generic
// truncation error. The driver must surface the upstream error verbatim.
func TestLongCatStreamSurfacesUpstreamError(t *testing.T) {
	ctx := t.Context()
	srv := newLongCatSSEServer(t, "/openai/v1/chat/completions",
		`data: {"choices":[{"delta":{"content":"partial "}}]}`+"\n"+
			`data: {"error":{"message":"rate limit exceeded","type":"rate_limit_error"}}`+"\n",
	)
	defer srv.Close()

	m := newLongCatForTest(srv.URL)
	apiKey := "test-key"
	err := m.ChatStreamlyWithSender(ctx, "LongCat-Flash-Chat",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "upstream stream error") {
		t.Errorf("expected upstream-error surfacing, got %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("expected upstream message included, got %v", err)
	}
}

func TestLongCatListModelsAndCheckConnection(t *testing.T) {
	ctx := t.Context()
	var requests int
	srv := newLongCatServer(t, "/openai/v1/models", func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		requests++
		if body != nil {
			t.Errorf("GET /models should not send a JSON body: %v", body)
		}
		if r.ContentLength > 0 {
			t.Errorf("GET /models should not send request body with ContentLength=%d", r.ContentLength)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read GET body: %v", err)
			return
		}
		if len(raw) > 0 {
			t.Errorf("GET /models should not send request body: %q", string(raw))
		}
		if requests == 1 {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"object": "list",
				"data": []map[string]interface{}{
					{"id": "LongCat-Flash-Chat", "object": "model"},
					{"id": "LongCat-Flash-Thinking-2601", "object": "model"},
					{"id": "", "object": "model"},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"object": "list",
			"data": []map[string]interface{}{
				{"id": "LongCat-Flash-Chat", "object": "model"},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	models, err := newLongCatForTest(srv.URL).ListModels(ctx, &APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if got := joinModelNames(models, ","); got != "LongCat-Flash-Chat,LongCat-Flash-Thinking-2601" {
		t.Errorf("models=%q", got)
	}
	if err := newLongCatForTest(srv.URL).CheckConnection(ctx, &APIConfig{ApiKey: &apiKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestLongCatListModelsRejectsInvalidResponses(t *testing.T) {
	ctx := t.Context()
	for name, response := range map[string]string{
		"missing data": `{}`,
		"null data":    `{"data":null}`,
		"too large":    strings.Repeat(" ", longCatMaxListModelsResponseBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			srv := newLongCatServer(t, "/openai/v1/models", func(t *testing.T, _ *http.Request, body map[string]interface{}, w http.ResponseWriter) {
				_, _ = io.WriteString(w, response)
			})
			defer srv.Close()

			apiKey := "test-key"
			_, err := newLongCatForTest(srv.URL).ListModels(ctx, &APIConfig{ApiKey: &apiKey})
			if err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestLongCatListModelsRequiresAPIKey(t *testing.T) {
	ctx := t.Context()
	for name, cfg := range map[string]*APIConfig{
		"nil config": nil,
		"nil key":    {},
		"empty key":  {ApiKey: new(string)},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := newLongCatForTest("http://unused").ListModels(ctx, cfg)
			if err == nil || !strings.Contains(err.Error(), "api key is required") {
				t.Errorf("expected api-key error, got %v", err)
			}
			err = newLongCatForTest("http://unused").CheckConnection(ctx, cfg)
			if err == nil || !strings.Contains(err.Error(), "api key is required") {
				t.Errorf("CheckConnection expected api-key error, got %v", err)
			}
		})
	}
}

func TestLongCatEmbedReturnsNoSuchMethod(t *testing.T) {
	ctx := t.Context()
	m := newLongCatForTest("http://unused")
	model := "x"
	_, err := m.Embed(ctx, &model, []string{"a"}, &APIConfig{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Embed: want 'no such method', got %v", err)
	}
}

func TestLongCatRerankReturnsNoSuchMethod(t *testing.T) {
	ctx := t.Context()
	m := newLongCatForTest("http://unused")
	model := "x"
	_, err := m.Rerank(ctx, &model, "q", []string{"a"}, &APIConfig{}, &RerankConfig{TopN: 1}, nil)
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank: want 'no such method', got %v", err)
	}
}

func TestLongCatBalanceReturnsNoSuchMethod(t *testing.T) {
	ctx := t.Context()
	m := newLongCatForTest("http://unused")
	_, err := m.Balance(ctx, &APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance: want 'no such method', got %v", err)
	}
}

func TestLongCatAudioOCRReturnNoSuchMethod(t *testing.T) {
	ctx := t.Context()
	m := newLongCatForTest("http://unused")
	model := "x"
	if _, err := m.TranscribeAudio(ctx, &model, &model, &APIConfig{}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudio: want 'no such method', got %v", err)
	}
	if _, err := m.AudioSpeech(ctx, &model, &model, &APIConfig{}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeech: want 'no such method', got %v", err)
	}
	if _, err := m.OCRFile(ctx, &model, nil, &model, &APIConfig{}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile: want 'no such method', got %v", err)
	}
}
