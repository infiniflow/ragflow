package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func newLocalAIForTest(baseURL string) *LocalAIModel {
	return NewLocalAIModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:      "chat/completions",
			Models:    "models",
			Embedding: "embeddings",
			Rerank:    "rerank",
		},
	)
}

// withLocalAIIdleTimeout swaps the package-level idle timeout for the
// duration of the test. Tests that exercise the stall watchdog use a
// sub-second value so they finish quickly.
func withLocalAIIdleTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	original := localAIStreamIdleTimeout
	localAIStreamIdleTimeout = d
	t.Cleanup(func() {
		localAIStreamIdleTimeout = original
	})
}

func TestLocalAIName(t *testing.T) {
	l := newLocalAIForTest("http://unused")
	if got := l.Name(); got != "localai" {
		t.Errorf("Name()=%q, want %q", got, "localai")
	}
}

func TestLocalAIStreamCancelsOnIdle(t *testing.T) {
	// The server emits one valid chunk and then stalls. Without the
	// watchdog, scanner.Scan() would hang forever. With the watchdog
	// at 200ms, it must return a clear "stream idle" error in well
	// under a second.
	withLocalAIIdleTimeout(t, 200*time.Millisecond)

	// hold blocks the handler until the test closes it. Register
	// close(hold) FIRST so it runs LAST (defers are LIFO) — wait,
	// that's the opposite. We want close(hold) to run BEFORE
	// srv.Close() so the handler can return. Use t.Cleanup, which
	// runs in reverse-registration order: register srv.Close first
	// so it runs last, then close(hold) so it runs first.
	hold := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"hi"}}]}`+"\n")
			f.Flush()
		}
		// Hang until either the client disconnects (watchdog cancels
		// the request, which causes r.Context() to fire) or the test
		// teardown signals via `hold`.
		select {
		case <-hold:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(hold) })

	l := newLocalAIForTest(srv.URL)
	var got []string
	var mu sync.Mutex
	start := time.Now()
	err := l.ChatStreamlyWithSender("gpt-4",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil,
		func(content *string, _ *string) error {
			if content == nil || *content == "" {
				return nil
			}
			mu.Lock()
			got = append(got, *content)
			mu.Unlock()
			return nil
		},
	)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an idle-timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "idle for more than") {
		t.Errorf("expected idle-timeout error, got %v", err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("watchdog did not fire promptly; elapsed=%v", elapsed)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) == 0 || got[0] != "hi" {
		t.Errorf("expected first chunk before stall, got %v", got)
	}
}

func TestLocalAIStreamCompletesWithoutTriggeringWatchdog(t *testing.T) {
	// Sanity check: a fast, complete stream should not trip the
	// watchdog even with a moderately tight idle window.
	withLocalAIIdleTimeout(t, 500*time.Millisecond)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"content":"a"}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":"b"}}]}`+"\n"+
				`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
		if f != nil {
			f.Flush()
		}
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	var chunks []string
	err := l.ChatStreamlyWithSender("gpt-4",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil,
		func(content *string, _ *string) error {
			if content != nil && *content != "" && *content != "[DONE]" {
				chunks = append(chunks, *content)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if strings.Join(chunks, "") != "ab" {
		t.Errorf("chunks=%v want [a b]", chunks)
	}
}

func TestLocalAIStreamRequiresSender(t *testing.T) {
	l := newLocalAIForTest("http://unused")
	err := l.ChatStreamlyWithSender("gpt-4",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("expected sender-required error, got %v", err)
	}
}

func TestLocalAIChatMissingBaseURLFailsClearly(t *testing.T) {
	// LocalAI has no public default; resolveBaseURL must fail with a
	// helpful message when neither the requested region nor "default"
	// is configured.
	l := NewLocalAIModel(map[string]string{}, URLSuffix{Chat: "chat/completions"})
	_, err := l.ChatWithMessages("gpt-4",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing base URL") {
		t.Errorf("expected missing-base-URL error, got %v", err)
	}
}

func TestLocalAIChatOmitsAuthHeaderWhenKeyEmpty(t *testing.T) {
	// Optional-auth contract: LocalAI accepts an empty key, so the
	// driver must NOT send a "Bearer " header in that case.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("expected no Authorization header, got %q", got)
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	resp, err := l.ChatWithMessages("gpt-4",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if *resp.Answer != "ok" {
		t.Errorf("answer=%q want ok", *resp.Answer)
	}
}

func TestLocalAIChatSendsAuthHeaderWhenKeyProvided(t *testing.T) {
	// And conversely: when a tenant has put LocalAI behind an auth
	// proxy with a token, the driver does send the Bearer header.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("expected Authorization=Bearer secret, got %q", got)
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	key := "secret"
	_, err := l.ChatWithMessages("gpt-4",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &key}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestLocalAIBalanceReturnsNoSuchMethod(t *testing.T) {
	l := newLocalAIForTest("http://unused")
	_, err := l.Balance(&APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance: expected 'no such method', got %v", err)
	}
}

func TestLocalAIEmbedHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"data":[
			{"embedding":[0.1,0.2],"index":0},
			{"embedding":[0.3,0.4],"index":1},
			{"embedding":[0.5,0.6],"index":2}]}`)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	model := "text-embedding-ada-002"
	vecs, err := l.Embed(&model, []string{"a", "b", "c"}, &APIConfig{}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 3 {
		t.Fatalf("len=%d want 3", len(vecs))
	}
	if vecs[1].Embedding[0] != 0.3 || vecs[1].Index != 1 {
		t.Errorf("vecs[1]=%+v", vecs[1])
	}
}

func TestLocalAIEmbedRejectsDuplicateIndex(t *testing.T) {
	// CodeRabbit caught that a response repeating data[*].index would
	// silently overwrite the earlier vector. Verify the driver fails
	// loudly instead.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[
			{"embedding":[1],"index":0},
			{"embedding":[2],"index":0}]}`)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	model := "text-embedding-ada-002"
	_, err := l.Embed(&model, []string{"a", "b"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "duplicate embedding index 0") {
		t.Errorf("expected duplicate-index error, got %v", err)
	}
}

func TestLocalAIEmbedRejectsOutOfRangeIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"embedding":[1],"index":7}]}`)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	model := "text-embedding-ada-002"
	_, err := l.Embed(&model, []string{"a", "b"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("expected out-of-range error, got %v", err)
	}
}

func TestLocalAIEmbedRejectsMissingSlot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"embedding":[1],"index":0}]}`)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	model := "text-embedding-ada-002"
	_, err := l.Embed(&model, []string{"a", "b"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing embedding for input index 1") {
		t.Errorf("expected missing-slot error, got %v", err)
	}
}

func TestLocalAIEmbedEmptyInputShortCircuits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("Embed([]) made an unexpected HTTP call")
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	model := "text-embedding-ada-002"
	vecs, err := l.Embed(&model, []string{}, &APIConfig{}, nil)
	if err != nil || len(vecs) != 0 {
		t.Errorf("Embed([])=(%v,%v) want ([],nil)", vecs, err)
	}
}

// ---------- reasoning extraction (multi-field) ----------

// Table-driven unit coverage of the helper. Mirrors the priority order
// reasoning_content > reasoning > thinking declared in
// localAIReasoningFields. New upstream field names can be added by
// extending that slice without touching call sites.
func TestExtractLocalAIReasoning(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]interface{}
		want string
	}{
		{"empty map", map[string]interface{}{}, ""},
		{"reasoning_content wins", map[string]interface{}{
			"reasoning_content": "rc",
			"reasoning":         "r",
			"thinking":          "t",
		}, "rc"},
		{"reasoning when no reasoning_content", map[string]interface{}{
			"reasoning": "r",
			"thinking":  "t",
		}, "r"},
		{"thinking when only that is set", map[string]interface{}{
			"thinking": "qwen3-thought",
		}, "qwen3-thought"},
		{"empty string treated as absent", map[string]interface{}{
			"reasoning_content": "",
			"reasoning":         "fallback",
		}, "fallback"},
		{"non-string ignored", map[string]interface{}{
			"reasoning_content": 42,
			"reasoning":         "fallback",
		}, "fallback"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractLocalAIReasoning(tc.in)
			if got != tc.want {
				t.Errorf("got=%q want=%q", got, tc.want)
			}
		})
	}
}

// Non-streaming chat against an upstream that puts the trace in
// message.reasoning_content (kimi-k2.6, OpenAI o-series, DeepSeek-R1
// when proxied through OpenAI-shim). The driver must surface it on
// ChatResponse.ReasonContent.
func TestLocalAIChatExtractsReasoningContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{
			"role":"assistant",
			"content":"The answer is 12.",
			"reasoning_content":"15% = 0.15; 0.15 * 80 = 12."
		}}]}`)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	resp, err := l.ChatWithMessages("kimi-k2.6",
		[]Message{{Role: "user", Content: "15% of 80?"}},
		&APIConfig{}, nil)
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

// Non-streaming chat that uses message.thinking (Qwen3 via Ollama-shim
// inside LocalAI). The driver must surface it on ReasonContent too.
func TestLocalAIChatExtractsThinking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{
			"role":"assistant",
			"content":"12",
			"thinking":"Compute 15/100 * 80"
		}}]}`)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	resp, err := l.ChatWithMessages("qwen3-32b",
		[]Message{{Role: "user", Content: "15% of 80?"}},
		&APIConfig{}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if *resp.ReasonContent != "Compute 15/100 * 80" {
		t.Errorf("ReasonContent=%q want %q", *resp.ReasonContent, "Compute 15/100 * 80")
	}
}

// Regression net: a response with no reasoning field at all (any
// non-reasoning model) must produce empty ReasonContent without
// crashing or erroring.
func TestLocalAIChatHandlesAbsentReasoning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{
			"role":"assistant","content":"hello"
		}}]}`)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	resp, err := l.ChatWithMessages("llama-3-8b-instruct",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if *resp.Answer != "hello" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
	if *resp.ReasonContent != "" {
		t.Errorf("ReasonContent=%q want empty", *resp.ReasonContent)
	}
}

// Streaming chat where the upstream interleaves delta.reasoning_content
// chunks and delta.content chunks (kimi-k2.6, o-series shape).
// Reasoning must reach the sender's 2nd arg, content the 1st.
func TestLocalAIStreamExtractsReasoningContentDelta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w,
			`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
				`data: {"choices":[{"index":0,"delta":{"reasoning_content":"step 1. "}}]}`+"\n"+
				`data: {"choices":[{"index":0,"delta":{"reasoning_content":"step 2."}}]}`+"\n"+
				`data: {"choices":[{"index":0,"delta":{"content":"Answer."},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	var content, reasoning []string
	err := l.ChatStreamlyWithSender("kimi-k2.6",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil,
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
		},
	)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if got := strings.Join(reasoning, ""); got != "step 1. step 2." {
		t.Errorf("reasoning joined=%q", got)
	}
	if got := strings.Join(content, ""); got != "Answer." {
		t.Errorf("content joined=%q", got)
	}
}

// Streaming chat where the upstream uses delta.thinking (Qwen3 shape).
// The same handler must work.
func TestLocalAIStreamExtractsThinkingDelta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w,
			`data: {"choices":[{"index":0,"delta":{"thinking":"qwen-trace"}}]}`+"\n"+
				`data: {"choices":[{"index":0,"delta":{"content":"final"},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	var got []string
	err := l.ChatStreamlyWithSender("qwen3-32b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil,
		func(c *string, r *string) error {
			if r != nil && *r != "" {
				got = append(got, "R:"+*r)
			}
			if c != nil && *c != "" && *c != "[DONE]" {
				got = append(got, "C:"+*c)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	want := []string{"R:qwen-trace", "C:final"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("seq=%v want %v", got, want)
	}
}

// Request-side: ChatConfig.Effort must flow into request body as
// reasoning_effort.
func TestLocalAIChatPropagatesReasoningEffort(t *testing.T) {
	var seen map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		if err := json.Unmarshal(raw, &seen); err != nil {
			t.Errorf("unmarshal request body: %v\nraw=%s", err, string(raw))
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	effort := "high"
	_, err := l.ChatWithMessages("kimi-k2.6",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, &ChatConfig{Effort: &effort})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if seen["reasoning_effort"] != "high" {
		t.Errorf("reasoning_effort=%v want high", seen["reasoning_effort"])
	}
	if _, present := seen["enable_thinking"]; present {
		t.Errorf("enable_thinking should be absent when Thinking nil")
	}
}

// Request-side: ChatConfig.Thinking must flow into request body as
// enable_thinking (Qwen3-style).
func TestLocalAIChatPropagatesEnableThinking(t *testing.T) {
	var seen map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		if err := json.Unmarshal(raw, &seen); err != nil {
			t.Errorf("unmarshal request body: %v\nraw=%s", err, string(raw))
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	think := true
	_, err := l.ChatWithMessages("qwen3-32b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, &ChatConfig{Thinking: &think})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if seen["enable_thinking"] != true {
		t.Errorf("enable_thinking=%v want true", seen["enable_thinking"])
	}
}

// Stream request also propagates the reasoning params.
func TestLocalAIStreamPropagatesReasoningParams(t *testing.T) {
	var seen map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		if err := json.Unmarshal(raw, &seen); err != nil {
			t.Errorf("unmarshal request body: %v\nraw=%s", err, string(raw))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"index":0,"delta":{"content":"x"},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	effort := "medium"
	think := true
	err := l.ChatStreamlyWithSender("kimi-k2.6",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, &ChatConfig{Effort: &effort, Thinking: &think},
		func(*string, *string) error { return nil },
	)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if seen["reasoning_effort"] != "medium" {
		t.Errorf("reasoning_effort=%v want medium", seen["reasoning_effort"])
	}
	if seen["enable_thinking"] != true {
		t.Errorf("enable_thinking=%v want true", seen["enable_thinking"])
	}
}
