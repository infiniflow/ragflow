package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newGPUStackServer(t *testing.T, expectedPath string, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
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

func newGPUStackSSEServer(t *testing.T, expectedPath, ssePayload string) *httptest.Server {
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
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("expected Accept=text/event-stream, got %q", got)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, ssePayload)
	}))
}

func newGPUStackForTest(baseURL string) *GPUStackModel {
	return NewGPUStackModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "v1/chat/completions", Models: "v1/models", Embedding: "v1-openai/embeddings"},
	)
}

const gpustackEmbeddingsPath = "/v1-openai/embeddings"

func TestGPUStackName(t *testing.T) {
	if got := newGPUStackForTest("http://unused").Name(); got != "gpustack" {
		t.Errorf("Name()=%q, want %q", got, "gpustack")
	}
}

func TestGPUStackFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("GPUStack", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*GPUStackModel); !ok {
		t.Fatalf("driver type=%T, want *GPUStackModel", driver)
	}
}

func TestGPUStackChatHappyPath(t *testing.T) {
	srv := newGPUStackServer(t, "/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "qwen3-8b" {
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

	apiKey := "test-key"
	resp, err := newGPUStackForTest(srv.URL).ChatWithMessages(
		"qwen3-8b",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey}, nil,
	)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Errorf("Answer=%v", resp.Answer)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "" {
		t.Errorf("ReasonContent=%v want empty", resp.ReasonContent)
	}
}

func TestGPUStackChatExtractsReasoningContent(t *testing.T) {
	srv := newGPUStackServer(t, "/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"role":              "assistant",
					"content":           "12",
					"reasoning_content": "0.15 * 80 = 12",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newGPUStackForTest(srv.URL).ChatWithMessages(
		"qwen3-32b",
		[]Message{{Role: "user", Content: "15% of 80?"}},
		&APIConfig{ApiKey: &apiKey}, nil,
	)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if *resp.Answer != "12" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
	if *resp.ReasonContent != "0.15 * 80 = 12" {
		t.Errorf("ReasonContent=%q", *resp.ReasonContent)
	}
}

func TestGPUStackChatForwardsDocumentedFields(t *testing.T) {
	srv := newGPUStackServer(t, "/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		for _, k := range []string{"model", "messages", "stream", "max_tokens", "temperature", "top_p", "stop"} {
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

	apiKey := "test-key"
	mt := 64
	temp := 0.5
	topP := 0.95
	stop := []string{"END"}
	_, err := newGPUStackForTest(srv.URL).ChatWithMessages(
		"qwen3-8b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{MaxTokens: &mt, Temperature: &temp, TopP: &topP, Stop: &stop},
	)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestGPUStackChatRequiresAPIKey(t *testing.T) {
	_, err := newGPUStackForTest("http://unused").ChatWithMessages(
		"qwen3-8b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestGPUStackChatRequiresModelName(t *testing.T) {
	apiKey := "test-key"
	_, err := newGPUStackForTest("http://unused").ChatWithMessages(
		"",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("expected model-name error, got %v", err)
	}
}

func TestGPUStackChatRequiresMessages(t *testing.T) {
	apiKey := "test-key"
	_, err := newGPUStackForTest("http://unused").ChatWithMessages(
		"qwen3-8b", nil, &APIConfig{ApiKey: &apiKey}, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("expected messages-empty error, got %v", err)
	}
}

func TestGPUStackChatRejectsHTTPError(t *testing.T) {
	srv := newGPUStackServer(t, "/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newGPUStackForTest(srv.URL).ChatWithMessages(
		"qwen3-8b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 propagated, got %v", err)
	}
}

func TestGPUStackChatRequiresBaseURL(t *testing.T) {
	model := NewGPUStackModel(map[string]string{}, URLSuffix{Chat: "v1/chat/completions"})
	apiKey := "test-key"
	_, err := model.ChatWithMessages(
		"qwen3-8b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "no base URL configured") {
		t.Errorf("expected base-URL error, got %v", err)
	}
}

func TestGPUStackStreamHappyPath(t *testing.T) {
	srv := newGPUStackSSEServer(t, "/v1/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"Hello"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	var chunks []string
	var sawDone bool
	err := newGPUStackForTest(srv.URL).ChatStreamlyWithSender(
		"qwen3-8b",
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
		},
	)
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

func TestGPUStackStreamExtractsReasoningContent(t *testing.T) {
	srv := newGPUStackSSEServer(t, "/v1/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"reasoning_content":"think "}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"answer"},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	var content, reasoning []string
	err := newGPUStackForTest(srv.URL).ChatStreamlyWithSender(
		"qwen3-32b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(c *string, r *string) error {
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
	if strings.Join(reasoning, "") != "think " {
		t.Errorf("reasoning=%q", strings.Join(reasoning, ""))
	}
	if strings.Join(content, "") != "answer" {
		t.Errorf("content=%q", strings.Join(content, ""))
	}
}

func TestGPUStackStreamRejectsExplicitFalse(t *testing.T) {
	apiKey := "test-key"
	stream := false
	err := newGPUStackForTest("http://unused").ChatStreamlyWithSender(
		"qwen3-8b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Errorf("expected stream-true guard, got %v", err)
	}
}

func TestGPUStackStreamRequiresSender(t *testing.T) {
	apiKey := "test-key"
	err := newGPUStackForTest("http://unused").ChatStreamlyWithSender(
		"qwen3-8b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("expected sender-required error, got %v", err)
	}
}

func TestGPUStackStreamFailsWithoutTerminal(t *testing.T) {
	srv := newGPUStackSSEServer(t, "/v1/chat/completions",
		`data: {"choices":[{"delta":{"content":"half"}}]}`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	err := newGPUStackForTest(srv.URL).ChatStreamlyWithSender(
		"qwen3-8b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "stream ended before") {
		t.Errorf("expected truncation error, got %v", err)
	}
}

func TestGPUStackStreamRejectsMalformedFrame(t *testing.T) {
	srv := newGPUStackSSEServer(t, "/v1/chat/completions",
		`data: {"choices":[{"delta":{"content":"ok"}}]}`+"\n"+
			`data: {this is not valid json}`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	err := newGPUStackForTest(srv.URL).ChatStreamlyWithSender(
		"qwen3-8b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "invalid SSE event") {
		t.Errorf("expected invalid-SSE error, got %v", err)
	}
}

func TestGPUStackStreamSurfacesUpstreamError(t *testing.T) {
	srv := newGPUStackSSEServer(t, "/v1/chat/completions",
		`data: {"choices":[{"delta":{"content":"partial "}}]}`+"\n"+
			`data: {"error":{"message":"oom","type":"runtime_error"}}`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	err := newGPUStackForTest(srv.URL).ChatStreamlyWithSender(
		"qwen3-8b",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "upstream stream error") {
		t.Errorf("expected upstream-error surfacing, got %v", err)
	}
}

func TestGPUStackListModelsHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s want GET", r.Method)
		}
		if r.URL.Path != "/v1/models" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "qwen3-8b"},
				{"id": "qwen3-32b"},
			},
		})
	}))
	defer srv.Close()

	apiKey := "test-key"
	model := newGPUStackForTest(srv.URL)
	models, err := model.ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if strings.Join(models, ",") != "qwen3-8b,qwen3-32b" {
		t.Errorf("models=%v", models)
	}
	if err := model.CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestGPUStackListModelsRequiresAPIKey(t *testing.T) {
	_, err := newGPUStackForTest("http://unused").ListModels(&APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

// TestGPUStackEmbedHappyPath verifies request shape and dimensions on v1-openai/embeddings.
func TestGPUStackEmbedHappyPath(t *testing.T) {
	srv := newGPUStackServer(t, gpustackEmbeddingsPath, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "bge-m3" {
			t.Errorf("model=%v", body["model"])
		}
		if body["dimensions"] != float64(512) {
			t.Errorf("dimensions=%v, want 512", body["dimensions"])
		}
		inputs, ok := body["input"].([]interface{})
		if !ok || len(inputs) != 2 {
			t.Errorf("input=%v, want 2-element array", body["input"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{0.2, 0.2}, "index": 1},
				{"embedding": []float64{0.1, 0.2}, "index": 0},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := "bge-m3"
	vecs, err := newGPUStackForTest(srv.URL).Embed(
		&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, &EmbeddingConfig{Dimension: 512})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("len(vecs)=%d, want 2", len(vecs))
	}
	if vecs[0].Index != 0 || vecs[0].Embedding[0] != 0.1 || vecs[1].Index != 1 || vecs[1].Embedding[0] != 0.2 {
		t.Errorf("vecs=%+v", vecs)
	}
}

// TestGPUStackEmbedReordersByIndex verifies out-of-order response indices are mapped correctly.
func TestGPUStackEmbedReordersByIndex(t *testing.T) {
	srv := newGPUStackServer(t, gpustackEmbeddingsPath, func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{2}, "index": 2},
				{"embedding": []float64{0}, "index": 0},
				{"embedding": []float64{1}, "index": 1},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := "bge-m3"
	vecs, err := newGPUStackForTest(srv.URL).Embed(
		&model, []string{"a", "b", "c"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	for i, v := range vecs {
		if v.Index != i || v.Embedding[0] != float64(i) {
			t.Errorf("slot %d = %+v, want Embedding=[%d] Index=%d", i, v, i, i)
		}
	}
}

// TestGPUStackEmbedEmptyInputShortCircuits avoids HTTP when texts is empty.
func TestGPUStackEmbedEmptyInputShortCircuits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("Embed([]) made an unexpected HTTP call")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	apiKey := "test-key"
	model := "bge-m3"
	vecs, err := newGPUStackForTest(srv.URL).Embed(&model, []string{}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Embed([]): %v", err)
	}
	if len(vecs) != 0 {
		t.Errorf("len(vecs)=%d, want 0", len(vecs))
	}
}

// TestGPUStackEmbedRequiresAPIKey rejects requests without an API key.
func TestGPUStackEmbedRequiresAPIKey(t *testing.T) {
	model := "bge-m3"
	_, err := newGPUStackForTest("http://unused").Embed(&model, []string{"a"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

// TestGPUStackEmbedRejectsDuplicateIndex errors on duplicate response indices.
func TestGPUStackEmbedRejectsDuplicateIndex(t *testing.T) {
	srv := newGPUStackServer(t, gpustackEmbeddingsPath, func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{0.1}, "index": 0},
				{"embedding": []float64{0.2}, "index": 0},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := "bge-m3"
	_, err := newGPUStackForTest(srv.URL).Embed(&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected duplicate-index error, got %v", err)
	}
}

// TestGPUStackEmbedRejectsOutOfRangeIndex errors when index exceeds input length.
func TestGPUStackEmbedRejectsOutOfRangeIndex(t *testing.T) {
	srv := newGPUStackServer(t, gpustackEmbeddingsPath, func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{0.1}, "index": 2},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := "bge-m3"
	_, err := newGPUStackForTest(srv.URL).Embed(&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("expected out-of-range error, got %v", err)
	}
}

// TestGPUStackEmbedRejectsMissingIndex errors when index is omitted from response.
func TestGPUStackEmbedRejectsMissingIndex(t *testing.T) {
	srv := newGPUStackServer(t, gpustackEmbeddingsPath, func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{0.1}},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := "bge-m3"
	_, err := newGPUStackForTest(srv.URL).Embed(&model, []string{"a"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing embedding index") {
		t.Errorf("expected missing-index error, got %v", err)
	}
}

// TestGPUStackEmbedRejectsEmptyVector errors when the API returns a zero-length vector.
func TestGPUStackEmbedRejectsEmptyVector(t *testing.T) {
	srv := newGPUStackServer(t, gpustackEmbeddingsPath, func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{}, "index": 0},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := "bge-m3"
	_, err := newGPUStackForTest(srv.URL).Embed(&model, []string{"a"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "empty embedding vector") {
		t.Errorf("expected empty-vector error, got %v", err)
	}
}

// TestGPUStackEmbedRejectsMissingSlot errors when a response index is never returned.
func TestGPUStackEmbedRejectsMissingSlot(t *testing.T) {
	srv := newGPUStackServer(t, gpustackEmbeddingsPath, func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{0.1}, "index": 0},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := "bge-m3"
	_, err := newGPUStackForTest(srv.URL).Embed(&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing embedding for input index") {
		t.Errorf("expected missing-slot error, got %v", err)
	}
}

func TestGPUStackUnsupportedMethods(t *testing.T) {
	m := newGPUStackForTest("http://unused")
	model := "x"
	if _, err := m.Rerank(&model, "q", []string{"a"}, &APIConfig{}, &RerankConfig{TopN: 1}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank: %v", err)
	}
	if _, err := m.Balance(&APIConfig{}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance: %v", err)
	}
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
