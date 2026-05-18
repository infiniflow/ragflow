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

// newCometAPIServer stands up an httptest server that asserts the
// request shape and lets the caller decide what to return.
func newCometAPIServer(t *testing.T, expectedPath string, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectedPath {
			t.Errorf("expected path=%s, got %s", expectedPath, r.URL.Path)
			return
		}
		if r.Method != http.MethodGet && r.Header.Get("Authorization") != "Bearer test-key" {
			got := r.Header.Get("Authorization")
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

func newCometAPIForTest(baseURL string) *CometAPIModel {
	return NewCometAPIModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:      "v1/chat/completions",
			Models:    "api/models",
			Embedding: "v1/embeddings",
			Balance:   "user/quota",
		},
	)
}

func TestCometAPIName(t *testing.T) {
	m := newCometAPIForTest("http://unused")
	if got := m.Name(); got != "cometapi" {
		t.Errorf("Name()=%q, want %q", got, "cometapi")
	}
}

func TestCometAPIFactoryRoute(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("cometapi", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*CometAPIModel); !ok {
		t.Fatalf("driver type=%T, want *CometAPIModel", driver)
	}
}

func TestCometAPIChatHappyPath(t *testing.T) {
	srv := newCometAPIServer(t, "/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "gpt-5" {
			t.Errorf("expected model=gpt-5, got %v", body["model"])
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

	m := newCometAPIForTest(srv.URL)
	apiKey := "test-key"
	resp, err := m.ChatWithMessages("gpt-5", []Message{
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

func TestCometAPIChatPropagatesConfig(t *testing.T) {
	srv := newCometAPIServer(t, "/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
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

	m := newCometAPIForTest(srv.URL)
	apiKey := "test-key"
	mt := 64
	temp := 0.3
	topP := 0.9
	stop := []string{"END"}
	_, err := m.ChatWithMessages("gpt-5", []Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{MaxTokens: &mt, Temperature: &temp, TopP: &topP, Stop: &stop},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
}

func TestCometAPIChatReturnsReasoningContent(t *testing.T) {
	srv := newCometAPIServer(t, "/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "answer", "reasoning_content": "\nreason"}},
			},
		})
	})
	defer srv.Close()

	m := newCometAPIForTest(srv.URL)
	apiKey := "test-key"
	resp, err := m.ChatWithMessages("gpt-5", []Message{{Role: "user", Content: "ping"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "reason" {
		t.Errorf("reason=%v want reason", resp.ReasonContent)
	}
}

func TestCometAPIChatRequiresAPIKey(t *testing.T) {
	m := newCometAPIForTest("http://unused")
	_, err := m.ChatWithMessages("gpt-5", []Message{{Role: "user", Content: "x"}}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
	emptyKey := ""
	_, err = m.ChatWithMessages("gpt-5", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &emptyKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("empty key: expected api-key error, got %v", err)
	}
}

func TestCometAPIChatRequiresModelName(t *testing.T) {
	m := newCometAPIForTest("http://unused")
	apiKey := "test-key"
	_, err := m.ChatWithMessages("", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("expected model-name error, got %v", err)
	}
	err = m.ChatStreamlyWithSender(" ", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("stream: expected model-name error, got %v", err)
	}
}

func TestCometAPIChatRequiresMessages(t *testing.T) {
	m := newCometAPIForTest("http://unused")
	apiKey := "test-key"
	_, err := m.ChatWithMessages("gpt-5", nil, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("expected messages-empty error, got %v", err)
	}
}

func TestCometAPIChatRejectsHTTPError(t *testing.T) {
	srv := newCometAPIServer(t, "/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})
	defer srv.Close()

	m := newCometAPIForTest(srv.URL)
	apiKey := "test-key"
	_, err := m.ChatWithMessages("gpt-5", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 propagated, got %v", err)
	}
}

func TestCometAPIChatFallsBackToDefaultOnEmptyRegion(t *testing.T) {
	// Empty *Region pointer must fall back to the "default" entry, not
	// be treated as an explicit "" region (which would miss the lookup).
	srv := newCometAPIServer(t, "/v1/chat/completions", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]interface{}{"content": "ok"}}},
		})
	})
	defer srv.Close()

	m := newCometAPIForTest(srv.URL)
	apiKey := "test-key"
	emptyRegion := ""
	_, err := m.ChatWithMessages("gpt-5",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey, Region: &emptyRegion}, nil)
	if err != nil {
		t.Errorf("empty Region: expected fallback to default, got %v", err)
	}
}

func TestCometAPIListModelsFallsBackToDefaultOnEmptyRegion(t *testing.T) {
	srv := newCometAPIServer(t, "/api/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]interface{}{{"id": "x"}}})
	})
	defer srv.Close()

	m := newCometAPIForTest(srv.URL)
	apiKey := "test-key"
	emptyRegion := ""
	if _, err := m.ListModels(&APIConfig{ApiKey: &apiKey, Region: &emptyRegion}); err != nil {
		t.Errorf("empty Region: expected fallback to default, got %v", err)
	}
}

func TestCometAPIStreamRequiresSender(t *testing.T) {
	m := newCometAPIForTest("http://unused")
	apiKey := "test-key"
	err := m.ChatStreamlyWithSender("gpt-5",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("expected sender-required error, got %v", err)
	}
}

func TestCometAPIChatRejectsUnknownRegion(t *testing.T) {
	m := newCometAPIForTest("http://unused")
	apiKey := "test-key"
	region := "eu"
	_, err := m.ChatWithMessages("gpt-5", []Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey, Region: &region}, nil)
	if err == nil || !strings.Contains(err.Error(), "no base URL configured for region") {
		t.Errorf("expected region error, got %v", err)
	}
}

func TestCometAPIBaseURLNormalizesSlashes(t *testing.T) {
	tests := []struct {
		name string
		path string
		run  func(*CometAPIModel, *APIConfig) error
	}{
		{
			name: "Chat",
			path: "/v1/chat/completions",
			run: func(m *CometAPIModel, apiConfig *APIConfig) error {
				_, err := m.ChatWithMessages("gpt-5", []Message{{Role: "user", Content: "x"}}, apiConfig, nil)
				return err
			},
		},
		{
			name: "Stream",
			path: "/v1/chat/completions",
			run: func(m *CometAPIModel, apiConfig *APIConfig) error {
				return m.ChatStreamlyWithSender("gpt-5", []Message{{Role: "user", Content: "x"}}, apiConfig, nil, func(*string, *string) error { return nil })
			},
		},
		{
			name: "Embed",
			path: "/v1/embeddings",
			run: func(m *CometAPIModel, apiConfig *APIConfig) error {
				model := "text-embedding-3-small"
				_, err := m.Embed(&model, []string{"x"}, apiConfig, nil)
				return err
			},
		},
		{
			name: "ListModels",
			path: "/api/models",
			run: func(m *CometAPIModel, apiConfig *APIConfig) error {
				_, err := m.ListModels(apiConfig)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newCometAPIServer(t, tt.path, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
				switch tt.name {
				case "Chat":
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"choices": []map[string]interface{}{{"message": map[string]interface{}{"content": "ok"}}}})
				case "Stream":
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`+"\n")
				case "Embed":
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]interface{}{{"embedding": []float64{1}, "index": 0}}})
				case "ListModels":
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]interface{}{{"id": "gpt-5"}}})
				}
			})
			defer srv.Close()

			m := newCometAPIForTest(srv.URL + "/")
			m.URLSuffix.Chat = "/v1/chat/completions"
			m.URLSuffix.Models = "/api/models"
			m.URLSuffix.Embedding = "/v1/embeddings"
			apiKey := "test-key"
			if err := tt.run(m, &APIConfig{ApiKey: &apiKey}); err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
		})
	}
}

func TestCometAPIStreamHappyPath(t *testing.T) {
	srv := newCometAPIServer(t, "/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
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
	})
	defer srv.Close()

	m := newCometAPIForTest(srv.URL)
	apiKey := "test-key"
	var chunks []string
	var sawDone int32
	err := m.ChatStreamlyWithSender("gpt-5",
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

func TestCometAPIStreamRejectsExplicitFalse(t *testing.T) {
	m := newCometAPIForTest("http://unused")
	apiKey := "test-key"
	stream := false
	err := m.ChatStreamlyWithSender("gpt-5",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Errorf("expected stream-true guard, got %v", err)
	}
}

func TestCometAPIStreamFailsWithoutTerminal(t *testing.T) {
	// Body closes before [DONE] or a finish_reason -> driver must complain
	// instead of pretending the stream finished cleanly.
	srv := newCometAPIServer(t, "/v1/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"half"}}]}`+"\n")
	})
	defer srv.Close()

	m := newCometAPIForTest(srv.URL)
	apiKey := "test-key"
	err := m.ChatStreamlyWithSender("gpt-5",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "stream ended before") {
		t.Errorf("expected stream-truncation error, got %v", err)
	}
}

func TestCometAPIListModelsHappyPath(t *testing.T) {
	srv := newCometAPIServer(t, "/api/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "gpt-5"},
				{"id": "gpt-4o-mini"},
				{"id": "text-embedding-3-small"},
			},
		})
	})
	defer srv.Close()

	m := newCometAPIForTest(srv.URL)
	apiKey := "test-key"
	ids, err := m.ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(ids) != 3 || ids[0] != "gpt-5" || ids[2] != "text-embedding-3-small" {
		t.Errorf("ids=%v, want [gpt-5 gpt-4o-mini text-embedding-3-small]", ids)
	}
}

func TestCometAPIListModelsAllowsNilAPIConfig(t *testing.T) {
	srv := newCometAPIServer(t, "/api/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]interface{}{{"id": "gpt-5"}}})
	})
	defer srv.Close()

	m := newCometAPIForTest(srv.URL)
	ids, err := m.ListModels(nil)
	if err != nil {
		t.Fatalf("ListModels(nil): %v", err)
	}
	if len(ids) != 1 || ids[0] != "gpt-5" {
		t.Errorf("ids=%v want [gpt-5]", ids)
	}
}

func TestCometAPICheckConnectionDelegatesToBalance(t *testing.T) {
	// 200 -> CheckConnection succeeds; 401 -> CheckConnection propagates.
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/quota" {
			t.Errorf("path=%s want /user/quota", r.URL.Path)
		}
		if got := r.URL.Query().Get("key"); got != "test-key" {
			t.Errorf("key query=%q want test-key", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"total_quota": 10.0})
	}))
	defer okSrv.Close()
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer failSrv.Close()

	apiKey := "test-key"
	mOK := newCometAPIForTest(okSrv.URL)
	mOK.URLSuffix.Balance = okSrv.URL + "/user/quota"
	if err := mOK.CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Errorf("CheckConnection(ok): %v", err)
	}
	mFail := newCometAPIForTest(failSrv.URL)
	mFail.URLSuffix.Balance = failSrv.URL + "/user/quota"
	if err := mFail.CheckConnection(&APIConfig{ApiKey: &apiKey}); err == nil {
		t.Error("CheckConnection(fail): expected error, got nil")
	}
}

func TestCometAPIBalanceHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/quota" {
			t.Errorf("path=%s want /user/quota", r.URL.Path)
		}
		if got := r.URL.Query().Get("key"); got != "test-key" {
			t.Errorf("key query=%q want test-key", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization=%q want empty", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"username":         "tester",
			"total_quota":      20.5,
			"total_used_quota": 1.25,
			"request_count":    7,
		})
	}))
	defer srv.Close()

	m := newCometAPIForTest("http://unused")
	m.URLSuffix.Balance = srv.URL + "/user/quota"
	apiKey := "test-key"
	balance, err := m.Balance(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if balance["username"] != "tester" || balance["total_quota"] != 20.5 {
		t.Errorf("balance=%v", balance)
	}
}

func TestCometAPIBalanceRequiresAPIKey(t *testing.T) {
	m := newCometAPIForTest("http://unused")
	_, err := m.Balance(&APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("Balance: expected api-key error, got %v", err)
	}
}

func TestCometAPIBalanceRequiresConfiguredURL(t *testing.T) {
	m := newCometAPIForTest("http://unused")
	m.URLSuffix.Balance = ""
	apiKey := "test-key"
	_, err := m.Balance(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "balance URL is required") {
		t.Errorf("Balance: expected balance URL error, got %v", err)
	}
}

func TestCometAPIRerankReturnsNoSuchMethod(t *testing.T) {
	m := newCometAPIForTest("http://unused")
	q := "gpt-5"
	_, err := m.Rerank(&q, "what is rag?", []string{"a", "b"}, &APIConfig{}, &RerankConfig{TopN: 2})
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank: expected 'no such method', got %v", err)
	}
}

func TestCometAPIEmbedHappyPath(t *testing.T) {
	srv := newCometAPIServer(t, "/v1/embeddings", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "text-embedding-3-small" {
			t.Errorf("model=%v want text-embedding-3-small", body["model"])
		}
		if body["dimensions"] != float64(256) {
			t.Errorf("dimensions=%v want 256", body["dimensions"])
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

	m := newCometAPIForTest(srv.URL)
	apiKey := "test-key"
	model := "text-embedding-3-small"
	vecs, err := m.Embed(&model, []string{"a", "b", "c"}, &APIConfig{ApiKey: &apiKey}, &EmbeddingConfig{Dimension: 256})
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

func TestCometAPIEmbedReordersByIndex(t *testing.T) {
	// Upstream returns the three vectors in shuffled order. The driver
	// must reorder them so the slot at position i corresponds to input i.
	srv := newCometAPIServer(t, "/v1/embeddings", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{2}, "index": 2},
				{"embedding": []float64{0}, "index": 0},
				{"embedding": []float64{1}, "index": 1},
			},
		})
	})
	defer srv.Close()

	m := newCometAPIForTest(srv.URL)
	apiKey := "test-key"
	model := "text-embedding-3-small"
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

func TestCometAPIEmbedEmptyInputShortCircuits(t *testing.T) {
	// Empty input must NOT make an HTTP call; the test fails the request
	// rather than the assertion if it does.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("Embed([]) made an unexpected HTTP call")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := newCometAPIForTest(srv.URL)
	apiKey := "test-key"
	model := "text-embedding-3-small"
	vecs, err := m.Embed(&model, []string{}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Embed([]): %v", err)
	}
	if len(vecs) != 0 {
		t.Errorf("len(vecs)=%d want 0", len(vecs))
	}
}

func TestCometAPIEmbedRequiresAPIKey(t *testing.T) {
	m := newCometAPIForTest("http://unused")
	model := "text-embedding-3-small"
	_, err := m.Embed(&model, []string{"a"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestCometAPIEmbedRequiresModelName(t *testing.T) {
	m := newCometAPIForTest("http://unused")
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

func TestCometAPIEmbedRejectsDuplicateIndex(t *testing.T) {
	// A malformed upstream that repeats data[*].index would silently
	// overwrite the earlier vector; the driver must fail loudly instead.
	srv := newCometAPIServer(t, "/v1/embeddings", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{1}, "index": 0},
				{"embedding": []float64{2}, "index": 0},
			},
		})
	})
	defer srv.Close()

	m := newCometAPIForTest(srv.URL)
	apiKey := "test-key"
	model := "text-embedding-3-small"
	_, err := m.Embed(&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "duplicate embedding index 0") {
		t.Errorf("expected duplicate-index error, got %v", err)
	}
}

func TestCometAPIEmbedRejectsOutOfRangeIndex(t *testing.T) {
	srv := newCometAPIServer(t, "/v1/embeddings", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{1}, "index": 7}, // out of range for 2-input request
			},
		})
	})
	defer srv.Close()

	m := newCometAPIForTest(srv.URL)
	apiKey := "test-key"
	model := "text-embedding-3-small"
	_, err := m.Embed(&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("expected out-of-range error, got %v", err)
	}
}

func TestCometAPIEmbedRejectsMissingSlot(t *testing.T) {
	// Upstream returns only one of the two requested embeddings.
	srv := newCometAPIServer(t, "/v1/embeddings", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{1}, "index": 0},
			},
		})
	})
	defer srv.Close()

	m := newCometAPIForTest(srv.URL)
	apiKey := "test-key"
	model := "text-embedding-3-small"
	_, err := m.Embed(&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing embedding for input index 1") {
		t.Errorf("expected missing-embedding error for slot 1, got %v", err)
	}
}

func TestCometAPIEmbedRejectsHTTPError(t *testing.T) {
	srv := newCometAPIServer(t, "/v1/embeddings", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})
	defer srv.Close()

	m := newCometAPIForTest(srv.URL)
	apiKey := "test-key"
	model := "text-embedding-3-small"
	_, err := m.Embed(&model, []string{"a"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "CometAPI embeddings API error") {
		t.Errorf("expected CometAPI embeddings API error, got %v", err)
	}
}
