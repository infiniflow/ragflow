package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newNovitaServer(t *testing.T, expectedPath string, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
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
		// Content-Type must declare JSON on BOTH POST chat (body is
		// JSON) and GET ListModels (Novita platform expects callers to
		// negotiate JSON content even though the body is empty —
		// maintainer review explicitly flagged the missing header).
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

func newNovitaForTest(baseURL string) *NovitaModel {
	return NewNovitaModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "openai/v1/chat/completions", Models: "openai/v1/models"},
	)
}

// newNovitaSSEServer asserts the SSE-chat wire contract (POST, path,
// Authorization, Content-Type) the same way newNovitaServer does for
// the JSON-chat path, then writes the supplied SSE payload. Closes
// the gap CodeRabbit flagged where streaming tests used
// httptest.NewServer directly and skipped the request-shape checks.
func newNovitaSSEServer(t *testing.T, expectedPath, ssePayload string) *httptest.Server {
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

// ---- think-tag split helpers ----

func TestSplitNovitaThinkPureText(t *testing.T) {
	v, r := splitNovitaThink("hello world")
	if v != "hello world" || r != "" {
		t.Errorf("got (%q,%q)", v, r)
	}
}

func TestSplitNovitaThinkSingleBlock(t *testing.T) {
	v, r := splitNovitaThink("<think>15% = 0.15. 0.15*80 = 12.</think>The answer is 12.")
	if v != "The answer is 12." {
		t.Errorf("visible=%q", v)
	}
	if r != "15% = 0.15. 0.15*80 = 12." {
		t.Errorf("reasoning=%q", r)
	}
}

func TestSplitNovitaThinkLeadingText(t *testing.T) {
	v, r := splitNovitaThink("intro <think>thought</think>tail")
	if v != "intro tail" {
		t.Errorf("visible=%q", v)
	}
	if r != "thought" {
		t.Errorf("reasoning=%q", r)
	}
}

func TestSplitNovitaThinkMultipleBlocks(t *testing.T) {
	v, r := splitNovitaThink("<think>A</think>part1<think>B</think>part2")
	if v != "part1part2" {
		t.Errorf("visible=%q", v)
	}
	if r != "AB" {
		t.Errorf("reasoning=%q", r)
	}
}

func TestSplitNovitaThinkUnclosedTag(t *testing.T) {
	// Unclosed <think> -> everything after the open tag is reasoning;
	// content stops at the open tag. This matches a real upstream that
	// got cut off mid-reasoning by max_tokens.
	v, r := splitNovitaThink("visible <think>still thinking when tokens ran out")
	if v != "visible " {
		t.Errorf("visible=%q", v)
	}
	if r != "still thinking when tokens ran out" {
		t.Errorf("reasoning=%q", r)
	}
}

// ---- streaming extractor ----

// Helper to push multiple chunks through and concatenate all output by
// kind. Each chunk goes into Feed; the output is what's safe to emit.
func feedAll(e *novitaThinkExtractor, chunks []string) (content, reasoning string) {
	var cb, rb strings.Builder
	for _, c := range chunks {
		for _, seg := range e.Feed(c) {
			cb.WriteString(seg.content)
			rb.WriteString(seg.reasoning)
		}
	}
	if seg := e.Flush(); seg != nil {
		cb.WriteString(seg.content)
		rb.WriteString(seg.reasoning)
	}
	return cb.String(), rb.String()
}

func TestNovitaThinkExtractorSingleChunk(t *testing.T) {
	e := &novitaThinkExtractor{}
	c, r := feedAll(e, []string{"hello <think>thought</think> world"})
	if c != "hello  world" {
		t.Errorf("content=%q", c)
	}
	if r != "thought" {
		t.Errorf("reasoning=%q", r)
	}
}

func TestNovitaThinkExtractorTagSpansChunks(t *testing.T) {
	// "<think>" split across two SSE deltas: "<thi" + "nk>"
	e := &novitaThinkExtractor{}
	c, r := feedAll(e, []string{"hello <thi", "nk>thought</think>tail"})
	if c != "hello tail" {
		t.Errorf("content=%q", c)
	}
	if r != "thought" {
		t.Errorf("reasoning=%q", r)
	}
}

func TestNovitaThinkExtractorClosingTagSpansChunks(t *testing.T) {
	// "</think>" split across two deltas
	e := &novitaThinkExtractor{}
	c, r := feedAll(e, []string{"<think>reasoning</thi", "nk>visible"})
	if c != "visible" {
		t.Errorf("content=%q", c)
	}
	if r != "reasoning" {
		t.Errorf("reasoning=%q", r)
	}
}

func TestNovitaThinkExtractorTokenBoundaries(t *testing.T) {
	// Simulate the kind of chunking we saw on the wire for qwen3 — many
	// small chunks, sometimes splitting tag bytes.
	e := &novitaThinkExtractor{}
	c, r := feedAll(e, []string{
		"<", "think>", "Ok", "ay, ", "compute. </", "think>", "12", "."})
	if c != "12." {
		t.Errorf("content=%q", c)
	}
	if r != "Okay, compute. " {
		t.Errorf("reasoning=%q", r)
	}
}

func TestNovitaThinkExtractorNoTags(t *testing.T) {
	e := &novitaThinkExtractor{}
	c, r := feedAll(e, []string{"plain ", "content ", "all ", "the way"})
	if c != "plain content all the way" {
		t.Errorf("content=%q", c)
	}
	if r != "" {
		t.Errorf("reasoning=%q", r)
	}
}

func TestNovitaThinkExtractorLessThanIsNotTagStart(t *testing.T) {
	// "<10" or "<a" in legitimate content must not be held back as
	// possible tag start. The extractor reserves trailing bytes only
	// when a "<" is present in the suffix.
	e := &novitaThinkExtractor{}
	c, r := feedAll(e, []string{"a < b and c < d"})
	if c != "a < b and c < d" {
		t.Errorf("content=%q", c)
	}
	if r != "" {
		t.Errorf("reasoning=%q", r)
	}
}

// ---- driver methods ----

func TestNovitaName(t *testing.T) {
	if got := newNovitaForTest("http://unused").Name(); got != "novita" {
		t.Errorf("Name()=%q", got)
	}
}

func TestNovitaChatPureText(t *testing.T) {
	srv := newNovitaServer(t, "/openai/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{"content": "pong"},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newNovitaForTest(srv.URL).ChatWithMessages(
		"meta-llama/llama-3.3-70b-instruct",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if *resp.Answer != "pong" || *resp.ReasonContent != "" {
		t.Errorf("(%q,%q)", *resp.Answer, *resp.ReasonContent)
	}
}

func TestNovitaChatExtractsThinkTags(t *testing.T) {
	// qwen3-style response: <think>...</think> embedded in content.
	// Driver must split it into Answer + ReasonContent.
	srv := newNovitaServer(t, "/openai/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "<think>15% = 0.15; 0.15 * 80 = 12.</think>The answer is 12.",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newNovitaForTest(srv.URL).ChatWithMessages(
		"qwen/qwen3-30b-a3b-fp8",
		[]Message{{Role: "user", Content: "15% of 80?"}},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if *resp.Answer != "The answer is 12." {
		t.Errorf("Answer=%q", *resp.Answer)
	}
	if *resp.ReasonContent != "15% = 0.15; 0.15 * 80 = 12." {
		t.Errorf("ReasonContent=%q", *resp.ReasonContent)
	}
}

// deepseek-v3.1 / glm-4.5 with enable_thinking=true return reasoning
// in a separate `reasoning_content` field on the message rather than
// inline as <think>...</think>. The driver must surface this field
// to ChatResponse.ReasonContent. Live-confirmed against
// api.novita.ai/openai/v1/chat/completions with deepseek/deepseek-v3.1.
func TestNovitaChatExtractsReasoningContentField(t *testing.T) {
	srv := newNovitaServer(t, "/openai/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"role":              "assistant",
					"content":           "4",
					"reasoning_content": "2+2 is straightforward arithmetic: the answer is 4.",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newNovitaForTest(srv.URL).ChatWithMessages(
		"deepseek/deepseek-v3.1",
		[]Message{{Role: "user", Content: "2+2?"}},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Answer == nil || resp.ReasonContent == nil {
		t.Fatalf("Answer/ReasonContent must be non-nil pointers")
	}
	if *resp.Answer != "4" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
	if *resp.ReasonContent != "2+2 is straightforward arithmetic: the answer is 4." {
		t.Errorf("ReasonContent=%q", *resp.ReasonContent)
	}
}

// Streaming deepseek-v3.1 with thinking on emits delta.reasoning_content
// (not delta.content with <think> tags). The driver must forward
// those chunks via the sender's second arg.
func TestNovitaStreamExtractsDeltaReasoningContent(t *testing.T) {
	srv := newNovitaSSEServer(t, "/openai/v1/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"step 1. "}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"reasoning_content":"step 2."}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"final answer"},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	var content, reasoning []string
	err := newNovitaForTest(srv.URL).ChatStreamlyWithSender(
		"deepseek/deepseek-v3.1",
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

// TestNovitaChatPropagatesEnableThinking pins the maintainer's
// requested behaviour: when ChatConfig.Thinking is set, the driver
// MUST forward it as Novita's documented `enable_thinking` body field
// so a tenant can switch a deepseek-v3.1 / glm-4.5 / qwen3 deployment
// out of its default thinking mode without prompt-level hacks.
func TestNovitaChatPropagatesEnableThinking(t *testing.T) {
	cases := []struct {
		name  string
		value bool
	}{
		{"enabled", true},
		{"disabled", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newNovitaServer(t, "/openai/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
				got, present := body["enable_thinking"]
				if !present {
					t.Errorf("enable_thinking missing from body, want %v", tc.value)
				}
				if got != tc.value {
					t.Errorf("enable_thinking=%v want %v", got, tc.value)
				}
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"choices": []map[string]interface{}{{
						"message": map[string]interface{}{"content": "ok"},
					}},
				})
			})
			defer srv.Close()

			apiKey := "test-key"
			thinking := tc.value
			_, err := newNovitaForTest(srv.URL).ChatWithMessages(
				"qwen/qwen3-30b-a3b-fp8",
				[]Message{{Role: "user", Content: "x"}},
				&APIConfig{ApiKey: &apiKey},
				&ChatConfig{Thinking: &thinking})
			if err != nil {
				t.Fatalf("Chat: %v", err)
			}
		})
	}
}

// Sending enable_thinking when the caller didn't ask for it would
// silently flip behavior for downstream proxies that distinguish
// "field absent" from "field present with default". Leave it out.
func TestNovitaChatOmitsEnableThinkingWhenUnset(t *testing.T) {
	srv := newNovitaServer(t, "/openai/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if _, present := body["enable_thinking"]; present {
			t.Errorf("enable_thinking must be absent when Thinking unset, got %v", body["enable_thinking"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{"content": "ok"},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newNovitaForTest(srv.URL).ChatWithMessages("m",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{}) // no Thinking
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

// TestNovitaStreamPropagatesEnableThinking mirrors the non-stream
// case for ChatStreamlyWithSender so callers get the same toggle
// regardless of streaming mode.
func TestNovitaStreamPropagatesEnableThinking(t *testing.T) {
	var seen map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &seen)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	}))
	defer srv.Close()

	apiKey := "test-key"
	thinking := false
	err := newNovitaForTest(srv.URL).ChatStreamlyWithSender(
		"deepseek/deepseek-v3.1",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Thinking: &thinking},
		func(*string, *string) error { return nil })
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got, ok := seen["enable_thinking"].(bool); !ok || got != false {
		t.Errorf("stream enable_thinking=%v want false", seen["enable_thinking"])
	}
}

func TestNovitaChatRequiresAPIKey(t *testing.T) {
	_, err := newNovitaForTest("http://unused").ChatWithMessages("m",
		[]Message{{Role: "user", Content: "x"}}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("got %v", err)
	}
}

func TestNovitaChatRequiresMessages(t *testing.T) {
	apiKey := "test-key"
	_, err := newNovitaForTest("http://unused").ChatWithMessages("m", nil,
		&APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("got %v", err)
	}
}

func TestNovitaChatRejectsHTTPError(t *testing.T) {
	srv := newNovitaServer(t, "/openai/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"unauthorized"}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newNovitaForTest(srv.URL).ChatWithMessages("m",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("got %v", err)
	}
}

// Streaming: an SSE stream that emits <think>...</think> inline in
// delta.content must surface reasoning chunks through the sender's
// second arg, and visible content through the first.
func TestNovitaStreamSplitsThinkTags(t *testing.T) {
	// Simulate the realistic case where tags span deltas — split
	// "<think>" across two chunks, and split "</think>" too.
	srv := newNovitaSSEServer(t, "/openai/v1/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"<"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"think>"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"Okay, "}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"compute. </"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"think>"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"12"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"."},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	var content, reasoning []string
	err := newNovitaForTest(srv.URL).ChatStreamlyWithSender(
		"qwen/qwen3-30b-a3b-fp8",
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
	gotContent := strings.Join(content, "")
	gotReason := strings.Join(reasoning, "")
	if gotContent != "12." {
		t.Errorf("content=%q want %q", gotContent, "12.")
	}
	if gotReason != "Okay, compute. " {
		t.Errorf("reasoning=%q", gotReason)
	}
}

// Streaming for a non-reasoning model that emits only content chunks
// must continue to work unchanged.
func TestNovitaStreamPureContent(t *testing.T) {
	srv := newNovitaSSEServer(t, "/openai/v1/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"Hello "}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"world"},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	var chunks []string
	var sawDone bool
	err := newNovitaForTest(srv.URL).ChatStreamlyWithSender("meta-llama/llama-3.3-70b-instruct",
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

func TestNovitaStreamRequiresSender(t *testing.T) {
	apiKey := "test-key"
	err := newNovitaForTest("http://unused").ChatStreamlyWithSender("m",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("got %v", err)
	}
}

func TestNovitaStreamRejectsExplicitFalse(t *testing.T) {
	apiKey := "test-key"
	stream := false
	err := newNovitaForTest("http://unused").ChatStreamlyWithSender("m",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Errorf("got %v", err)
	}
}

func TestNovitaListModelsHappyPath(t *testing.T) {
	srv := newNovitaServer(t, "/openai/v1/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "meta-llama/llama-3.3-70b-instruct"},
				{"id": "qwen/qwen3-30b-a3b-fp8"},
				{"id": "deepseek/deepseek-v4-pro"},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	ids, err := newNovitaForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(ids) != 3 {
		t.Errorf("ids=%v", ids)
	}
}

func TestNovitaCheckConnection(t *testing.T) {
	srv := newNovitaServer(t, "/openai/v1/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]interface{}{{"id": "x"}}})
	})
	defer srv.Close()

	apiKey := "test-key"
	if err := newNovitaForTest(srv.URL).CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Errorf("CheckConnection: %v", err)
	}
}

func TestNovitaEmbedReturnsNoSuchMethod(t *testing.T) {
	m := "x"
	_, err := newNovitaForTest("http://unused").Embed(&m, []string{"a"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("got %v", err)
	}
}

func TestNovitaRerankReturnsNoSuchMethod(t *testing.T) {
	m := "x"
	_, err := newNovitaForTest("http://unused").Rerank(&m, "q", []string{"a"}, &APIConfig{}, &RerankConfig{TopN: 1})
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("got %v", err)
	}
}

func TestNovitaBalanceReturnsNoSuchMethod(t *testing.T) {
	if _, err := newNovitaForTest("http://unused").Balance(&APIConfig{}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("got %v", err)
	}
}

func TestNovitaAudioOCRReturnNoSuchMethod(t *testing.T) {
	m := "x"
	v := newNovitaForTest("http://unused")
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

// TestNovitaBaseURLTrimsTrailingSlash pins the fix for a `//`-in-path
// bug a tenant could hit by configuring a baseURL like
// "https://api.novita.ai/v3/openai/". Every URL the driver builds via
// fmt.Sprintf("%s/%s", base, suffix) would then produce a double
// slash. baseURLForRegion now trims the trailing "/" so all three
// endpoint builders (Chat, Stream, ListModels) emit clean paths.
func TestNovitaBaseURLTrimsTrailingSlash(t *testing.T) {
	cases := []struct {
		name        string
		path        string
		method      string
		invoke      func(n *NovitaModel, apiKey string) error
		urlSuffix   URLSuffix
		respBody    string
		respHeaders map[string]string
	}{
		{
			name:      "Chat",
			path:      "/openai/v1/chat/completions",
			method:    http.MethodPost,
			urlSuffix: URLSuffix{Chat: "openai/v1/chat/completions"},
			invoke: func(n *NovitaModel, apiKey string) error {
				_, err := n.ChatWithMessages("m",
					[]Message{{Role: "user", Content: "x"}},
					&APIConfig{ApiKey: &apiKey}, nil)
				return err
			},
			respBody: `{"choices":[{"message":{"content":"ok"}}]}`,
		},
		{
			name:      "ListModels",
			path:      "/openai/v1/models",
			method:    http.MethodGet,
			urlSuffix: URLSuffix{Models: "openai/v1/models"},
			invoke: func(n *NovitaModel, apiKey string) error {
				_, err := n.ListModels(&APIConfig{ApiKey: &apiKey})
				return err
			},
			respBody: `{"data":[]}`,
		},
		{
			name:      "Stream",
			path:      "/openai/v1/chat/completions",
			method:    http.MethodPost,
			urlSuffix: URLSuffix{Chat: "openai/v1/chat/completions"},
			invoke: func(n *NovitaModel, apiKey string) error {
				return n.ChatStreamlyWithSender("m",
					[]Message{{Role: "user", Content: "x"}},
					&APIConfig{ApiKey: &apiKey}, nil,
					func(*string, *string) error { return nil })
			},
			respBody: `data: {"choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":"stop"}]}` + "\n" +
				`data: [DONE]` + "\n",
			respHeaders: map[string]string{"Content-Type": "text/event-stream"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// The load-bearing assertion: path is the clean
				// "/openai/v1/chat/completions" or "/openai/v1/models", never "//chat/...".
				if r.URL.Path != tc.path {
					t.Errorf("path=%q want %q (double-slash bug?)", r.URL.Path, tc.path)
					return
				}
				if r.Method != tc.method {
					t.Errorf("method=%s want %s", r.Method, tc.method)
					return
				}
				for k, v := range tc.respHeaders {
					w.Header().Set(k, v)
				}
				_, _ = io.WriteString(w, tc.respBody)
			}))
			defer srv.Close()

			// Configure baseURL WITH a trailing slash on purpose.
			n := NewNovitaModel(
				map[string]string{"default": srv.URL + "/"},
				tc.urlSuffix,
			)
			apiKey := "test-key"
			if err := tc.invoke(n, apiKey); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
		})
	}
}
