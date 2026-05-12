package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// newMistralServer stands up an httptest server that asserts the
// request shape and lets the caller decide what to return.
func newMistralServer(t *testing.T, expectedPath string, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
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
				t.Errorf("failed to read body: %v", err)
				return
			}
			var body map[string]interface{}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("invalid JSON body: %v\n%s", err, string(raw))
				return
			}
			handler(t, body, w)
			return
		}
		// GET path: no body
		handler(t, nil, w)
	}))
}

func newMistralForTest(baseURL string) *MistralModel {
	return NewMistralModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:      "chat/completions",
			Models:    "models",
			Embedding: "embeddings",
		},
	)
}

func TestMistralName(t *testing.T) {
	m := newMistralForTest("http://unused")
	if got := m.Name(); got != "mistral" {
		t.Errorf("Name()=%q, want %q", got, "mistral")
	}
}

func TestMistralChatHappyPath(t *testing.T) {
	srv := newMistralServer(t, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "mistral-large-latest" {
			t.Errorf("expected model=mistral-large-latest, got %v", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("expected stream=false, got %v", body["stream"])
		}
		msgs, ok := body["messages"].([]interface{})
		if !ok || len(msgs) != 1 {
			t.Errorf("expected 1 message, got %v", body["messages"])
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "pong"}},
			},
		})
	})
	defer srv.Close()

	m := newMistralForTest(srv.URL)
	apiKey := "test-key"
	resp, err := m.ChatWithMessages("mistral-large-latest", []Message{
		{Role: "user", Content: "ping"},
	}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Errorf("answer=%v, want pong", resp.Answer)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "" {
		t.Errorf("expected empty reason content, got %v", resp.ReasonContent)
	}
}

func TestMistralChatPropagatesConfig(t *testing.T) {
	srv := newMistralServer(t, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["max_tokens"] != float64(64) {
			t.Errorf("max_tokens=%v want 64", body["max_tokens"])
		}
		if body["temperature"] != 0.3 {
			t.Errorf("temperature=%v want 0.3", body["temperature"])
		}
		if body["top_p"] != 0.9 {
			t.Errorf("top_p=%v want 0.9", body["top_p"])
		}
		stop, ok := body["stop"].([]interface{})
		if !ok || len(stop) != 1 || stop[0] != "END" {
			t.Errorf("stop=%v want [END]", body["stop"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]interface{}{"content": "ok"}}},
		})
	})
	defer srv.Close()

	m := newMistralForTest(srv.URL)
	apiKey := "test-key"
	mt := 64
	temp := 0.3
	topP := 0.9
	stop := []string{"END"}
	_, err := m.ChatWithMessages("mistral-large-latest", []Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{MaxTokens: &mt, Temperature: &temp, TopP: &topP, Stop: &stop},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
}

func TestMistralChatRequiresAPIKey(t *testing.T) {
	m := newMistralForTest("http://unused")
	_, err := m.ChatWithMessages("mistral-large-latest", []Message{{Role: "user", Content: "x"}}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
	emptyKey := ""
	_, err = m.ChatWithMessages("mistral-large-latest", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &emptyKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("empty key: expected api-key error, got %v", err)
	}
}

func TestMistralChatRequiresMessages(t *testing.T) {
	m := newMistralForTest("http://unused")
	apiKey := "test-key"
	_, err := m.ChatWithMessages("mistral-large-latest", nil, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("expected messages-empty error, got %v", err)
	}
}

func TestMistralChatRejectsHTTPError(t *testing.T) {
	srv := newMistralServer(t, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})
	defer srv.Close()

	m := newMistralForTest(srv.URL)
	apiKey := "test-key"
	_, err := m.ChatWithMessages("mistral-large-latest", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 propagated, got %v", err)
	}
}

func TestMistralChatFallsBackToDefaultOnEmptyRegion(t *testing.T) {
	// Empty *Region pointer must fall back to the "default" entry, not
	// be treated as an explicit "" region (which would miss the lookup).
	srv := newMistralServer(t, "/chat/completions", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]interface{}{"content": "ok"}}},
		})
	})
	defer srv.Close()

	m := newMistralForTest(srv.URL)
	apiKey := "test-key"
	emptyRegion := ""
	_, err := m.ChatWithMessages("mistral-large-latest",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey, Region: &emptyRegion}, nil)
	if err != nil {
		t.Errorf("empty Region: expected fallback to default, got %v", err)
	}
}

func TestMistralListModelsFallsBackToDefaultOnEmptyRegion(t *testing.T) {
	srv := newMistralServer(t, "/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]interface{}{{"id": "x"}}})
	})
	defer srv.Close()

	m := newMistralForTest(srv.URL)
	apiKey := "test-key"
	emptyRegion := ""
	if _, err := m.ListModels(&APIConfig{ApiKey: &apiKey, Region: &emptyRegion}); err != nil {
		t.Errorf("empty Region: expected fallback to default, got %v", err)
	}
}

func TestMistralStreamRequiresSender(t *testing.T) {
	m := newMistralForTest("http://unused")
	apiKey := "test-key"
	err := m.ChatStreamlyWithSender("mistral-large-latest",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("expected sender-required error, got %v", err)
	}
}

func TestMistralChatRejectsUnknownRegion(t *testing.T) {
	m := newMistralForTest("http://unused")
	apiKey := "test-key"
	region := "eu"
	_, err := m.ChatWithMessages("mistral-large-latest", []Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey, Region: &region}, nil)
	if err == nil || !strings.Contains(err.Error(), "no base URL configured for region") {
		t.Errorf("expected region error, got %v", err)
	}
}

func TestMistralStreamHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]interface{}
		_ = json.Unmarshal(raw, &body)
		if body["stream"] != true {
			t.Errorf("expected stream=true, got %v", body["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Two content chunks then finish_reason terminator, then [DONE].
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"content":"Hello "}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":"world"}}]}`+"\n"+
				`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	}))
	defer srv.Close()

	m := newMistralForTest(srv.URL)
	apiKey := "test-key"
	var chunks []string
	var sawDone int32
	err := m.ChatStreamlyWithSender("mistral-large-latest",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(content *string, _ *string) error {
			if content == nil {
				return nil
			}
			if *content == "[DONE]" {
				atomic.StoreInt32(&sawDone, 1)
				return nil
			}
			chunks = append(chunks, *content)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if strings.Join(chunks, "") != "Hello world" {
		t.Errorf("chunks=%v want [\"Hello \" \"world\"]", chunks)
	}
	if atomic.LoadInt32(&sawDone) != 1 {
		t.Error("expected sender to receive [DONE] sentinel")
	}
}

func TestMistralStreamRejectsExplicitFalse(t *testing.T) {
	m := newMistralForTest("http://unused")
	apiKey := "test-key"
	stream := false
	err := m.ChatStreamlyWithSender("mistral-large-latest",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Errorf("expected stream-true guard, got %v", err)
	}
}

func TestMistralStreamFailsWithoutTerminal(t *testing.T) {
	// Body closes before [DONE] or a finish_reason -> driver must complain
	// instead of pretending the stream finished cleanly.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"half"}}]}`+"\n")
	}))
	defer srv.Close()

	m := newMistralForTest(srv.URL)
	apiKey := "test-key"
	err := m.ChatStreamlyWithSender("mistral-large-latest",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "stream ended before") {
		t.Errorf("expected stream-truncation error, got %v", err)
	}
}

func TestMistralListModelsHappyPath(t *testing.T) {
	srv := newMistralServer(t, "/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "mistral-large-latest"},
				{"id": "mistral-small-latest"},
				{"id": "mistral-embed"},
			},
		})
	})
	defer srv.Close()

	m := newMistralForTest(srv.URL)
	apiKey := "test-key"
	ids, err := m.ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(ids) != 3 || ids[0] != "mistral-large-latest" || ids[2] != "mistral-embed" {
		t.Errorf("ids=%v, want [mistral-large-latest mistral-small-latest mistral-embed]", ids)
	}
}

func TestMistralListModelsRequiresAPIKey(t *testing.T) {
	m := newMistralForTest("http://unused")
	if _, err := m.ListModels(&APIConfig{}); err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestMistralCheckConnectionDelegatesToListModels(t *testing.T) {
	// 200 -> CheckConnection succeeds; 401 -> CheckConnection propagates.
	okSrv := newMistralServer(t, "/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]interface{}{{"id": "x"}}})
	})
	defer okSrv.Close()
	failSrv := newMistralServer(t, "/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	defer failSrv.Close()

	apiKey := "test-key"
	mOK := newMistralForTest(okSrv.URL)
	if err := mOK.CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Errorf("CheckConnection(ok): %v", err)
	}
	mFail := newMistralForTest(failSrv.URL)
	if err := mFail.CheckConnection(&APIConfig{ApiKey: &apiKey}); err == nil {
		t.Error("CheckConnection(fail): expected error, got nil")
	}
}

func TestMistralBalanceReturnsNoSuchMethod(t *testing.T) {
	m := newMistralForTest("http://unused")
	_, err := m.Balance(&APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance: expected 'no such method', got %v", err)
	}
}

func TestMistralRerankReturnsNoSuchMethod(t *testing.T) {
	m := newMistralForTest("http://unused")
	q := "mistral-large-latest"
	_, err := m.Rerank(&q, "what is rag?", []string{"a", "b"}, &APIConfig{}, &RerankConfig{TopN: 2})
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank: expected 'no such method', got %v", err)
	}
}

func TestMistralEmbedHappyPath(t *testing.T) {
	srv := newMistralServer(t, "/embeddings", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "mistral-embed" {
			t.Errorf("model=%v want mistral-embed", body["model"])
		}
		inputs, ok := body["input"].([]interface{})
		if !ok || len(inputs) != 3 {
			t.Errorf("input=%v want 3-element array", body["input"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{0.1, 0.2}, "index": 0},
				{"embedding": []float64{0.3, 0.4}, "index": 1},
				{"embedding": []float64{0.5, 0.6}, "index": 2},
			},
		})
	})
	defer srv.Close()

	m := newMistralForTest(srv.URL)
	apiKey := "test-key"
	model := "mistral-embed"
	vecs, err := m.Embed(&model, []string{"a", "b", "c"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 3 {
		t.Fatalf("len(vecs)=%d want 3", len(vecs))
	}
	if vecs[1].Embedding[0] != 0.3 || vecs[1].Index != 1 {
		t.Errorf("vecs[1]=%+v want {Embedding:[0.3 0.4] Index:1}", vecs[1])
	}
}

func TestMistralEmbedReordersByIndex(t *testing.T) {
	// Upstream returns the three vectors in shuffled order. The driver
	// must reorder them so the slot at position i corresponds to input i.
	srv := newMistralServer(t, "/embeddings", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{2}, "index": 2},
				{"embedding": []float64{0}, "index": 0},
				{"embedding": []float64{1}, "index": 1},
			},
		})
	})
	defer srv.Close()

	m := newMistralForTest(srv.URL)
	apiKey := "test-key"
	model := "mistral-embed"
	vecs, err := m.Embed(&model, []string{"a", "b", "c"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	for i, v := range vecs {
		if v.Index != i || v.Embedding[0] != float64(i) {
			t.Errorf("slot %d = %+v, want Embedding=[%d] Index=%d", i, v, i, i)
		}
	}
}

func TestMistralEmbedEmptyInputShortCircuits(t *testing.T) {
	// Empty input must NOT make an HTTP call; the test fails the request
	// rather than the assertion if it does.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("Embed([]) made an unexpected HTTP call")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := newMistralForTest(srv.URL)
	apiKey := "test-key"
	model := "mistral-embed"
	vecs, err := m.Embed(&model, []string{}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Embed([]): %v", err)
	}
	if len(vecs) != 0 {
		t.Errorf("len(vecs)=%d want 0", len(vecs))
	}
}

func TestMistralEmbedRequiresAPIKey(t *testing.T) {
	m := newMistralForTest("http://unused")
	model := "mistral-embed"
	_, err := m.Embed(&model, []string{"a"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestMistralEmbedRequiresModelName(t *testing.T) {
	m := newMistralForTest("http://unused")
	apiKey := "test-key"
	_, err := m.Embed(nil, []string{"a"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("expected model-name error, got %v", err)
	}
	empty := ""
	_, err = m.Embed(&empty, []string{"a"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("empty model: expected model-name error, got %v", err)
	}
}

func TestMistralEmbedRejectsDuplicateIndex(t *testing.T) {
	// A malformed upstream that repeats data[*].index would silently
	// overwrite the earlier vector; the driver must fail loudly instead.
	srv := newMistralServer(t, "/embeddings", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{1}, "index": 0},
				{"embedding": []float64{2}, "index": 0},
			},
		})
	})
	defer srv.Close()

	m := newMistralForTest(srv.URL)
	apiKey := "test-key"
	model := "mistral-embed"
	_, err := m.Embed(&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "duplicate embedding index 0") {
		t.Errorf("expected duplicate-index error, got %v", err)
	}
}

func TestMistralEmbedRejectsOutOfRangeIndex(t *testing.T) {
	srv := newMistralServer(t, "/embeddings", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{1}, "index": 7}, // out of range for 2-input request
			},
		})
	})
	defer srv.Close()

	m := newMistralForTest(srv.URL)
	apiKey := "test-key"
	model := "mistral-embed"
	_, err := m.Embed(&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("expected out-of-range error, got %v", err)
	}
}

func TestMistralEmbedRejectsMissingSlot(t *testing.T) {
	// Upstream returns only one of the two requested embeddings.
	srv := newMistralServer(t, "/embeddings", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{1}, "index": 0},
			},
		})
	})
	defer srv.Close()

	m := newMistralForTest(srv.URL)
	apiKey := "test-key"
	model := "mistral-embed"
	_, err := m.Embed(&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing embedding for input index 1") {
		t.Errorf("expected missing-embedding error for slot 1, got %v", err)
	}
}

func TestMistralEmbedRejectsHTTPError(t *testing.T) {
	srv := newMistralServer(t, "/embeddings", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})
	defer srv.Close()

	m := newMistralForTest(srv.URL)
	apiKey := "test-key"
	model := "mistral-embed"
	_, err := m.Embed(&model, []string{"a"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "Mistral embeddings API error") {
		t.Errorf("expected Mistral embeddings API error, got %v", err)
	}
}
