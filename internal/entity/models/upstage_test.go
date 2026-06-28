package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newUpstageForTest(baseURL string) *UpstageModel {
	return NewUpstageModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:      "chat/completions",
			Models:    "models",
			Embedding: "embeddings",
		},
	)
}

// ---------- reasoning_effort / reasoning field ----------

func TestUpstageChatPropagatesReasoningEffort(t *testing.T) {
	// Per https://console.upstage.ai/api/docs/for-agents/raw, Upstage
	// Solar models accept `reasoning_effort: minimal|low|medium|high`.
	// ChatConfig.Effort is the canonical carrier; this test asserts it
	// flows into the wire body verbatim.
	var seen map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &seen)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	u := newUpstageForTest(srv.URL)
	apiKey := "test-key"
	effort := "high"
	_, err := u.ChatWithMessages("solar-pro2",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Effort: &effort})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got, ok := seen["reasoning_effort"].(string); !ok || got != "high" {
		t.Errorf("reasoning_effort=%v want \"high\"", seen["reasoning_effort"])
	}
}

func TestUpstageChatOmitsReasoningEffortWhenUnset(t *testing.T) {
	// If the caller does not opt in, the field must NOT be sent. Sending
	// "minimal" by default would silently change behavior for downstream
	// proxies that treat a present field differently from an absent one.
	var seen map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &seen)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	u := newUpstageForTest(srv.URL)
	apiKey := "test-key"
	_, err := u.ChatWithMessages("solar-pro2",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{}, // no Effort
	)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if _, present := seen["reasoning_effort"]; present {
		t.Errorf("reasoning_effort should be absent when Effort is unset, got %v", seen["reasoning_effort"])
	}
}

func TestUpstageStreamPropagatesReasoningEffort(t *testing.T) {
	var seen map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &seen)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"content":"hi"},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	}))
	defer srv.Close()

	u := newUpstageForTest(srv.URL)
	apiKey := "test-key"
	effort := "medium"
	err := u.ChatStreamlyWithSender("solar-pro2",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Effort: &effort},
		func(*string, *string) error { return nil },
	)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got, ok := seen["reasoning_effort"].(string); !ok || got != "medium" {
		t.Errorf("stream reasoning_effort=%v want \"medium\"", seen["reasoning_effort"])
	}
}

func TestUpstageChatExtractsReasoningField(t *testing.T) {
	// Per the Upstage docs: when reasoning_effort is high|medium for
	// solar-pro3 (or high for solar-pro2), the response's
	// choices[0].message includes a `reasoning` field. The driver must
	// pass it through as ChatResponse.ReasonContent.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{
			"content":"15% of 80 is **12**.",
			"reasoning":"15/100 = 0.15; 0.15 * 80 = 12"
		}}]}`)
	}))
	defer srv.Close()

	u := newUpstageForTest(srv.URL)
	apiKey := "test-key"
	resp, err := u.ChatWithMessages("solar-pro3",
		[]Message{{Role: "user", Content: "What is 15% of 80?"}},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "15/100 = 0.15; 0.15 * 80 = 12" {
		t.Errorf("ReasonContent=%v want the reasoning trace", resp.ReasonContent)
	}
	if resp.Answer == nil || *resp.Answer != "15% of 80 is **12**." {
		t.Errorf("Answer=%v", resp.Answer)
	}
}

func TestUpstageChatHandlesAbsentReasoning(t *testing.T) {
	// Models without reasoning (solar-mini, syn-pro) or low-effort
	// requests return no `reasoning` field. The driver must leave
	// ReasonContent empty without crashing.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	u := newUpstageForTest(srv.URL)
	apiKey := "test-key"
	resp, err := u.ChatWithMessages("solar-mini",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "" {
		t.Errorf("ReasonContent=%v want empty string for no-reasoning response", resp.ReasonContent)
	}
	if resp.Answer == nil || *resp.Answer != "ok" {
		t.Errorf("Answer=%v want ok", resp.Answer)
	}
}

// Ensure the same JSON shape used by the maintainer's docs (per
// https://console.upstage.ai/api/chat) round-trips through the request
// body for both streaming and non-streaming paths.
func TestUpstageRequestBodyMatchesSolarAPIShape(t *testing.T) {
	var seen map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &seen)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	u := newUpstageForTest(srv.URL)
	apiKey := "test-key"
	mt := 256
	temp := 0.7
	topP := 0.9
	stop := []string{"END"}
	effort := "high"
	_, err := u.ChatWithMessages("solar-pro2",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{MaxTokens: &mt, Temperature: &temp, TopP: &topP, Stop: &stop, Effort: &effort})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	want := map[string]interface{}{
		"model":            "solar-pro2",
		"stream":           false,
		"max_tokens":       float64(256),
		"temperature":      0.7,
		"top_p":            0.9,
		"reasoning_effort": "high",
	}
	for k, v := range want {
		if got, ok := seen[k]; !ok {
			t.Errorf("missing key %q in body", k)
		} else if !strings.HasPrefix(k, "stop") && got != v {
			t.Errorf("body[%q]=%v want %v", k, got, v)
		}
	}
	if stopArr, ok := seen["stop"].([]interface{}); !ok || len(stopArr) != 1 || stopArr[0] != "END" {
		t.Errorf("body[stop]=%v want [END]", seen["stop"])
	}
	if _, ok := seen["messages"].([]interface{}); !ok {
		t.Errorf("body[messages] missing or wrong type")
	}
}

// ---------- Embed: duplicate / out-of-range / reorder ----------

func TestUpstageEmbedRejectsDuplicateIndex(t *testing.T) {
	// A malformed upstream that repeats data[*].index would silently
	// overwrite the earlier vector; the driver must fail loudly instead.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[
			{"embedding":[1],"index":0},
			{"embedding":[2],"index":0}]}`)
	}))
	defer srv.Close()

	u := newUpstageForTest(srv.URL)
	apiKey := "test-key"
	model := "solar-embedding-1-large-passage"
	_, err := u.Embed(&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "duplicate embedding index 0") {
		t.Errorf("expected duplicate-index error, got %v", err)
	}
}

func TestUpstageEmbedRejectsOutOfRangeIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"embedding":[1],"index":7}]}`)
	}))
	defer srv.Close()

	u := newUpstageForTest(srv.URL)
	apiKey := "test-key"
	model := "solar-embedding-1-large-passage"
	_, err := u.Embed(&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("expected out-of-range error, got %v", err)
	}
}

func TestUpstageEmbedHappyPathReordersByIndex(t *testing.T) {
	// Upstream returns vectors in shuffled order; driver must realign.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[
			{"embedding":[2],"index":2},
			{"embedding":[0],"index":0},
			{"embedding":[1],"index":1}]}`)
	}))
	defer srv.Close()

	u := newUpstageForTest(srv.URL)
	apiKey := "test-key"
	model := "solar-embedding-1-large-passage"
	vecs, err := u.Embed(&model, []string{"a", "b", "c"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	for i, v := range vecs {
		if v.Index != i || v.Embedding[0] != float64(i) {
			t.Errorf("slot %d = %+v, want index=%d embedding=[%d]", i, v, i, i)
		}
	}
}

// ---------- streaming reasoning delta extraction ----------

// TestUpstageStreamExtractsReasoningDelta verifies that the SSE parser
// forwards `delta.reasoning` chunks via the sender's second arg (the
// reasonContent channel) and `delta.content` chunks via the first arg.
// Fixture matches the shape captured live from solar-pro3 with
// reasoning_effort=high — both fields appear, sometimes in the same
// chunk and sometimes separately.
func TestUpstageStreamExtractsReasoningDelta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w,
			`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
				`data: {"choices":[{"index":0,"delta":{"reasoning":"We need "}}]}`+"\n"+
				`data: {"choices":[{"index":0,"delta":{"reasoning":"to compute. "}}]}`+"\n"+
				`data: {"choices":[{"index":0,"delta":{"reasoning":"15% = 0.15."}}]}`+"\n"+
				`data: {"choices":[{"index":0,"delta":{"content":"15% of 80 "}}]}`+"\n"+
				`data: {"choices":[{"index":0,"delta":{"content":"is 12."},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	}))
	defer srv.Close()

	u := newUpstageForTest(srv.URL)
	apiKey := "test-key"
	var contentChunks, reasoningChunks []string
	err := u.ChatStreamlyWithSender("solar-pro3",
		[]Message{{Role: "user", Content: "What is 15% of 80?"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(content *string, reason *string) error {
			// At most one of (content, reason) is set per call: callers
			// need this contract to route to the right UI channel.
			if content != nil && reason != nil {
				t.Errorf("sender called with both content and reason non-nil")
			}
			if content != nil && *content != "" && *content != "[DONE]" {
				contentChunks = append(contentChunks, *content)
			}
			if reason != nil && *reason != "" {
				reasoningChunks = append(reasoningChunks, *reason)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	wantReason := "We need to compute. 15% = 0.15."
	wantContent := "15% of 80 is 12."
	if got := strings.Join(reasoningChunks, ""); got != wantReason {
		t.Errorf("joined reasoning=%q want %q", got, wantReason)
	}
	if got := strings.Join(contentChunks, ""); got != wantContent {
		t.Errorf("joined content=%q want %q", got, wantContent)
	}
	if len(reasoningChunks) != 3 {
		t.Errorf("expected 3 reasoning chunks, got %d", len(reasoningChunks))
	}
}

// TestUpstageStreamReasoningChunksArriveBeforeContent verifies the
// ordering contract inside a single SSE event when both fields are
// present: reasoning is forwarded first so a UI consuming both can
// render the chain-of-thought before the answer for that token.
func TestUpstageStreamReasoningChunksArriveBeforeContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w,
			// One SSE event carries BOTH reasoning and content in the
			// same delta. The driver must forward reasoning first.
			`data: {"choices":[{"index":0,"delta":{"reasoning":"R1","content":"C1"}}]}`+"\n"+
				`data: {"choices":[{"index":0,"delta":{"content":"C2"},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	}))
	defer srv.Close()

	u := newUpstageForTest(srv.URL)
	apiKey := "test-key"
	var seq []string
	err := u.ChatStreamlyWithSender("solar-pro3",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(content *string, reason *string) error {
			if reason != nil && *reason != "" {
				seq = append(seq, "R:"+*reason)
			}
			if content != nil && *content != "" && *content != "[DONE]" {
				seq = append(seq, "C:"+*content)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	wantSeq := []string{"R:R1", "C:C1", "C:C2"}
	if len(seq) != len(wantSeq) {
		t.Fatalf("seq=%v want %v", seq, wantSeq)
	}
	for i, want := range wantSeq {
		if seq[i] != want {
			t.Errorf("seq[%d]=%q want %q (full=%v)", i, seq[i], want, seq)
		}
	}
}

// TestUpstageStreamWithoutReasoningStillWorks is the regression net:
// non-reasoning models (solar-mini, solar-pro2 with no reasoning_effort)
// emit only delta.content. The driver must not regress on them.
func TestUpstageStreamWithoutReasoningStillWorks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w,
			`data: {"choices":[{"index":0,"delta":{"content":"Hello "}}]}`+"\n"+
				`data: {"choices":[{"index":0,"delta":{"content":"world"},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	}))
	defer srv.Close()

	u := newUpstageForTest(srv.URL)
	apiKey := "test-key"
	var content []string
	var reasonCalled bool
	err := u.ChatStreamlyWithSender("solar-mini",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(c *string, r *string) error {
			if r != nil && *r != "" {
				reasonCalled = true
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
	if reasonCalled {
		t.Errorf("reasoning callback fired despite no delta.reasoning in fixture")
	}
	if got := strings.Join(content, ""); got != "Hello world" {
		t.Errorf("content=%q want %q", got, "Hello world")
	}
}
