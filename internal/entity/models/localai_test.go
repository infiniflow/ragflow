package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestLocalAIName(t *testing.T) {
	l := newLocalAIForTest("http://unused")
	if got := l.Name(); got != "LocalAI" {
		t.Errorf("Name()=%q, want %q", got, "LocalAI")
	}
}

func TestLocalAIStreamRequiresSender(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	l := newLocalAIForTest("http://unused")
	err := l.ChatStreamlyWithSender(ctx, "gpt-4",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("expected sender-required error, got %v", err)
	}
}

func TestLocalAIChatMissingBaseURLFailsClearly(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	// LocalAI has no public default; resolveBaseURL must fail with a
	// helpful message when neither the requested region nor "default"
	// is configured.
	l := NewLocalAIModel(map[string]string{}, URLSuffix{Chat: "chat/completions"})
	_, err := l.ChatWithMessages(ctx, "gpt-4",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "base URL") {
		t.Errorf("expected missing-base-URL error, got %v", err)
	}
}

func TestLocalAIChatOmitsAuthHeaderWhenKeyEmpty(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
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
	resp, err := l.ChatWithMessages(ctx, "gpt-4",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil, nil,
	)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if *resp.Answer != "ok" {
		t.Errorf("answer=%q want ok", *resp.Answer)
	}
}

func TestLocalAIChatSendsAuthHeaderWhenKeyProvided(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
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
	_, err := l.ChatWithMessages(ctx, "gpt-4",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &key}, nil, nil,
	)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestLocalAIBalanceReturnsNoSuchMethod(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	l := newLocalAIForTest("http://unused")
	_, err := l.Balance(ctx, &APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance: expected 'no such method', got %v", err)
	}
}

func TestLocalAIEmbedHappyPath(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
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
	vecs, err := l.Embed(ctx, &model, EmbedRequest{Texts: []string{"a", "b", "c"}}, &APIConfig{}, nil, nil)
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
	withSSRFBypass(t)
	ctx := t.Context()
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
	_, err := l.Embed(ctx, &model, EmbedRequest{Texts: []string{"a", "b"}}, &APIConfig{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "duplicate embedding index 0") {
		t.Errorf("expected duplicate-index error, got %v", err)
	}
}

func TestLocalAIEmbedRejectsOutOfRangeIndex(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"embedding":[1],"index":7}]}`)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	model := "text-embedding-ada-002"
	_, err := l.Embed(ctx, &model, EmbedRequest{Texts: []string{"a", "b"}}, &APIConfig{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("expected out-of-range error, got %v", err)
	}
}

func TestLocalAIEmbedRejectsMissingSlot(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"embedding":[1],"index":0}]}`)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	model := "text-embedding-ada-002"
	_, err := l.Embed(ctx, &model, EmbedRequest{Texts: []string{"a", "b"}}, &APIConfig{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "missing embedding for input index 1") {
		t.Errorf("expected missing-slot error, got %v", err)
	}
}

func TestLocalAIEmbedEmptyInputShortCircuits(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("Embed([]) made an unexpected HTTP call")
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	model := "text-embedding-ada-002"
	vecs, err := l.Embed(ctx, &model, EmbedRequest{Texts: []string{}}, &APIConfig{}, nil, nil)
	if err != nil || len(vecs) != 0 {
		t.Errorf("Embed([])=(%v,%v) want ([],nil)", vecs, err)
	}
}

// ---------- reasoning content (message.reasoning_content) ----------
func TestLocalAIChatExtractsReasoningContent(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{
			"role":"assistant",
			"content":"The answer is 12.",
			"reasoning_content":"15% = 0.15; 0.15 * 80 = 12."
		}}]}`)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	resp, err := l.ChatWithMessages(ctx, "kimi-k2.6",
		[]Message{{Role: "user", Content: "15% of 80?"}},
		&APIConfig{}, nil, nil,
	)
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

// Regression net: a response with no reasoning field at all (any
// non-reasoning model) must produce empty ReasonContent without
// crashing or erroring.
func TestLocalAIChatHandlesAbsentReasoning(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{
			"role":"assistant","content":"hello"
		}}]}`)
	}))
	defer srv.Close()

	l := newLocalAIForTest(srv.URL)
	resp, err := l.ChatWithMessages(ctx, "llama-3-8b-instruct",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{}, nil, nil,
	)
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
	withSSRFBypass(t)
	ctx := t.Context()
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
	err := l.ChatStreamlyWithSender(ctx, "kimi-k2.6",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil, nil,
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

// Request-side: ChatConfig.Effort must flow into request body as
// reasoning_effort.
func TestLocalAIChatPropagatesReasoningEffort(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
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
	_, err := l.ChatWithMessages(ctx, "kimi-k2.6",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, &ChatConfig{Effort: &effort}, nil)
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
	withSSRFBypass(t)
	ctx := t.Context()
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
	_, err := l.ChatWithMessages(ctx, "qwen3-32b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, &ChatConfig{Thinking: &think}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if seen["enable_thinking"] != true {
		t.Errorf("enable_thinking=%v want true", seen["enable_thinking"])
	}
}

// Stream request also propagates the reasoning params.
func TestLocalAIStreamPropagatesReasoningParams(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
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
	err := l.ChatStreamlyWithSender(ctx, "kimi-k2.6",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, &ChatConfig{Effort: &effort, Thinking: &think}, nil,
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
