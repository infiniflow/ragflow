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
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("expected Content-Type=application/json, got %q", got)
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
		URLSuffix{Chat: "v1/chat/completions"},
	)
}

func TestLongCatName(t *testing.T) {
	if got := newLongCatForTest("http://unused").Name(); got != "longcat" {
		t.Errorf("Name()=%q, want %q", got, "longcat")
	}
}

func TestLongCatChatHappyPath(t *testing.T) {
	srv := newLongCatServer(t, "/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
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
	srv := newLongCatServer(t, "/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
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
	srv := newLongCatServer(t, "/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
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
	srv := newLongCatServer(t, "/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
				`data: {"choices":[{"index":0,"delta":{"content":"Hello"}}]}`+"\n"+
				`data: {"choices":[{"index":0,"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	}))
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
				`data: {"choices":[{"index":0,"delta":{"reasoning_content":"step 1. "}}]}`+"\n"+
				`data: {"choices":[{"index":0,"delta":{"reasoning_content":"step 2."}}]}`+"\n"+
				`data: {"choices":[{"index":0,"delta":{"content":"final answer"},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	}))
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"half"}}]}`+"\n")
	}))
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

// LongCat does not document a /models endpoint, so ListModels returns
// a shipped catalog rather than hitting the network. This test pins the
// known names so accidental edits to longcatKnownModels are caught.
func TestLongCatListModelsReturnsKnownCatalog(t *testing.T) {
	apiKey := "test-key"
	ids, err := newLongCatForTest("http://unused").ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	want := map[string]bool{
		"LongCat-Flash-Chat":          true,
		"LongCat-Flash-Lite":          true,
		"LongCat-Flash-Thinking-2601": true,
		"LongCat-Flash-Omni-2603":     true,
		"LongCat-2.0-Preview":         true,
	}
	if len(ids) != len(want) {
		t.Errorf("len(ids)=%d want %d (%v)", len(ids), len(want), ids)
	}
	for _, id := range ids {
		if !want[id] {
			t.Errorf("unexpected model id %q", id)
		}
	}
}

func TestLongCatListModelsRequiresAPIKey(t *testing.T) {
	m := newLongCatForTest("http://unused")
	if _, err := m.ListModels(&APIConfig{}); err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

// CheckConnection must hit the documented chat endpoint, not /models,
// since LongCat does not expose /models. A 1-token chat ping against
// LongCat-Flash-Lite is what the maintainer asked for.
func TestLongCatCheckConnectionChatPings(t *testing.T) {
	var (
		sawChat   bool
		sawModel  string
		sawTokens interface{}
	)
	srv := newLongCatServer(t, "/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		sawChat = true
		if m, ok := body["model"].(string); ok {
			sawModel = m
		}
		sawTokens = body["max_tokens"]
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{"content": "ok"},
			}},
		})
	})
	defer srv.Close()
	apiKey := "test-key"
	if err := newLongCatForTest(srv.URL).CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Errorf("CheckConnection: %v", err)
	}
	if !sawChat {
		t.Error("CheckConnection did not hit /v1/chat/completions")
	}
	if sawModel != "LongCat-Flash-Lite" {
		t.Errorf("CheckConnection ping model=%q want LongCat-Flash-Lite", sawModel)
	}
	if mt, ok := sawTokens.(float64); !ok || mt != 1 {
		t.Errorf("CheckConnection max_tokens=%v want 1", sawTokens)
	}
}

// On 401 (bad key), CheckConnection must surface the upstream error
// instead of silently returning nil — credentials are exactly what
// this method exists to validate.
func TestLongCatCheckConnectionPropagatesAuthError(t *testing.T) {
	srv := newLongCatServer(t, "/v1/chat/completions", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})
	defer srv.Close()
	apiKey := "test-key"
	err := newLongCatForTest(srv.URL).CheckConnection(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 propagated, got %v", err)
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
	if _, err := m.OCRFile(&model, &model, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile: want 'no such method', got %v", err)
	}
}
