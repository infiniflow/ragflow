package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newXinferenceForTest(baseURL string) *XinferenceModel {
	return NewXinferenceModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:      "v1/chat/completions",
			Embedding: "v1/embeddings",
			Models:    "v1/models",
			Rerank:    "v1/rerank",
		},
	)
}

func withXinferenceIdleTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	original := xinferenceStreamIdleTimeout
	xinferenceStreamIdleTimeout = d
	t.Cleanup(func() {
		xinferenceStreamIdleTimeout = original
	})
}

func TestXinferenceName(t *testing.T) {
	x := newXinferenceForTest("http://unused")
	if got := x.Name(); got != "xinference" {
		t.Errorf("Name()=%q, want %q", got, "xinference")
	}
}

func TestNormalizeXinferenceBaseURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"http://127.0.0.1:9997", "http://127.0.0.1:9997"},
		{"http://127.0.0.1:9997/", "http://127.0.0.1:9997"},
		{"http://127.0.0.1:9997/v1", "http://127.0.0.1:9997"},
		{" http://127.0.0.1:9997/v1/ ", "http://127.0.0.1:9997"},
	}
	for _, tc := range cases {
		if got := normalizeXinferenceBaseURL(tc.in); got != tc.want {
			t.Errorf("normalizeXinferenceBaseURL(%q)=%q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestXinferenceFactoryRoute(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("xinference", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if driver.Name() != "xinference" {
		t.Errorf("driver.Name()=%q, want xinference", driver.Name())
	}
}

func TestXinferenceChatHappyPathNormalizesBaseURLAndOmitsEmptyAuth(t *testing.T) {
	var seen map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%s, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("expected no Authorization header, got %q", got)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		if err := json.Unmarshal(raw, &seen); err != nil {
			t.Errorf("unmarshal request: %v", err)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"pong"}}]}`)
	}))
	defer srv.Close()

	x := newXinferenceForTest(srv.URL)
	maxTokens := 32
	temp := 0.2
	resp, err := x.ChatWithMessages("qwen2.5-instruct",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{},
		&ChatConfig{MaxTokens: &maxTokens, Temperature: &temp})
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Fatalf("Answer=%v, want pong", resp.Answer)
	}
	if seen["stream"] != false {
		t.Errorf("stream=%v, want false", seen["stream"])
	}
	if seen["max_tokens"] != float64(32) {
		t.Errorf("max_tokens=%v, want 32", seen["max_tokens"])
	}
	if seen["temperature"] != 0.2 {
		t.Errorf("temperature=%v, want 0.2", seen["temperature"])
	}
}

func TestXinferenceChatSendsAuthHeaderWhenKeyProvided(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("Authorization=%q, want Bearer sk-test", got)
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	x := newXinferenceForTest(srv.URL + "/v1")
	key := "sk-test"
	_, err := x.ChatWithMessages("qwen2.5-instruct",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &key}, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
}

func TestXinferenceChatExtractsReasoningFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{
			"content":"12",
			"reasoning_content":"0.15 * 80 = 12"
		}}]}`)
	}))
	defer srv.Close()

	x := newXinferenceForTest(srv.URL)
	resp, err := x.ChatWithMessages("qwen3",
		[]Message{{Role: "user", Content: "15% of 80?"}},
		&APIConfig{}, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "0.15 * 80 = 12" {
		t.Errorf("ReasonContent=%v", resp.ReasonContent)
	}
}

func TestXinferenceStreamHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		var seen map[string]interface{}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &seen)
		if seen["stream"] != true {
			t.Errorf("stream=%v, want true", seen["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"reasoning_content":"step. "}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":"Hello"}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	}))
	defer srv.Close()

	x := newXinferenceForTest(srv.URL)
	var content []string
	var reasoning []string
	var sawDone bool
	err := x.ChatStreamlyWithSender("qwen2.5-instruct",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{}, nil,
		func(c *string, r *string) error {
			if r != nil && *r != "" {
				reasoning = append(reasoning, *r)
			}
			if c != nil && *c == "[DONE]" {
				sawDone = true
			}
			if c != nil && *c != "" && *c != "[DONE]" {
				content = append(content, *c)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if strings.Join(reasoning, "") != "step. " {
		t.Errorf("reasoning=%q", strings.Join(reasoning, ""))
	}
	if strings.Join(content, "") != "Hello world" {
		t.Errorf("content=%q", strings.Join(content, ""))
	}
	if !sawDone {
		t.Error("expected [DONE] callback")
	}
}

func TestXinferenceStreamRejectsFalseStreamConfig(t *testing.T) {
	x := newXinferenceForTest("http://unused")
	stream := false
	err := x.ChatStreamlyWithSender("qwen2.5-instruct",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{},
		&ChatConfig{Stream: &stream},
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Errorf("expected stream-must-be-true error, got %v", err)
	}
}

func TestXinferenceStreamCancelsOnIdle(t *testing.T) {
	withXinferenceIdleTimeout(t, 200*time.Millisecond)

	hold := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"hi"}}]}`+"\n")
			f.Flush()
		}
		select {
		case <-hold:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(hold) })

	x := newXinferenceForTest(srv.URL)
	err := x.ChatStreamlyWithSender("qwen2.5-instruct",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream idle") {
		t.Errorf("expected stream-idle error, got %v", err)
	}
}

func TestXinferenceListModelsAndCheckConnection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path=%s, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("Authorization=%q, want Bearer sk-test", got)
		}
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"qwen2.5-instruct"},{"id":"custom-chat"}]}`)
	}))
	defer srv.Close()

	x := newXinferenceForTest(srv.URL)
	key := "sk-test"
	apiConfig := &APIConfig{ApiKey: &key}
	models, err := x.ListModels(apiConfig)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(models, ",") != "qwen2.5-instruct,custom-chat" {
		t.Errorf("models=%v", models)
	}
	if err := x.CheckConnection(apiConfig); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func newXinferenceEmbedServer(t *testing.T, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
			return
		}
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("path=%s, want /v1/embeddings", r.URL.Path)
			return
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization=%q, want empty when no API key configured", got)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		var body map[string]interface{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("unmarshal request: %v\n%s", err, string(raw))
			return
		}
		handler(t, body, w)
	}))
}

func TestXinferenceEmbedHappyPathAndOmitsEmptyAuth(t *testing.T) {
	srv := newXinferenceEmbedServer(t, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "bge-m3" {
			t.Errorf("model=%v, want bge-m3", body["model"])
		}
		inputs, ok := body["input"].([]interface{})
		if !ok || len(inputs) != 2 || inputs[0] != "hello" || inputs[1] != "world" {
			t.Errorf("input=%v, want [hello world]", body["input"])
		}
		// API key is empty — Authorization must not be set on a no-auth Xinference deployment.
		// Return data out of input order to verify the driver reorders by Index.
		_, _ = io.WriteString(w, `{"data":[{"index":1,"embedding":[0.4,0.5]},{"index":0,"embedding":[0.1,0.2]}]}`)
	})
	defer srv.Close()

	x := newXinferenceForTest(srv.URL)
	model := "bge-m3"
	got, err := x.Embed(&model, []string{"hello", "world"}, &APIConfig{}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got)=%d, want 2", len(got))
	}
	if got[0].Index != 0 || got[0].Embedding[0] != 0.1 || got[0].Embedding[1] != 0.2 {
		t.Errorf("got[0]=%+v, want Index=0 Embedding=[0.1 0.2]", got[0])
	}
	if got[1].Index != 1 || got[1].Embedding[0] != 0.4 || got[1].Embedding[1] != 0.5 {
		t.Errorf("got[1]=%+v, want Index=1 Embedding=[0.4 0.5]", got[1])
	}
}

func TestXinferenceEmbedSendsAuthWhenKeyConfigured(t *testing.T) {
	gotAuth := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"data":[{"index":0,"embedding":[0.1]}]}`)
	}))
	defer srv.Close()

	x := newXinferenceForTest(srv.URL)
	key := "sk-test"
	model := "bge-m3"
	if _, err := x.Embed(&model, []string{"x"}, &APIConfig{ApiKey: &key}, nil); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("Authorization=%q, want Bearer sk-test", gotAuth)
	}
}

func TestXinferenceEmbedNormalizesBaseURLWithV1Suffix(t *testing.T) {
	srv := newXinferenceEmbedServer(t, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_, _ = io.WriteString(w, `{"data":[{"index":0,"embedding":[0.1]}]}`)
	})
	defer srv.Close()

	x := NewXinferenceModel(
		map[string]string{"default": srv.URL + "/v1"}, // tenant supplied /v1 suffix
		URLSuffix{Chat: "v1/chat/completions", Embedding: "v1/embeddings", Models: "v1/models"},
	)
	model := "bge-m3"
	if _, err := x.Embed(&model, []string{"x"}, &APIConfig{}, nil); err != nil {
		t.Fatalf("Embed: %v", err)
	}
}

func TestXinferenceEmbedForwardsDimension(t *testing.T) {
	srv := newXinferenceEmbedServer(t, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["dimensions"] != float64(384) {
			t.Errorf("dimensions=%v, want 384", body["dimensions"])
		}
		_, _ = io.WriteString(w, `{"data":[{"index":0,"embedding":[0.1]}]}`)
	})
	defer srv.Close()

	x := newXinferenceForTest(srv.URL)
	model := "bge-m3"
	if _, err := x.Embed(&model, []string{"x"}, &APIConfig{}, &EmbeddingConfig{Dimension: 384}); err != nil {
		t.Fatalf("Embed: %v", err)
	}
}

func TestXinferenceEmbedRejectsDuplicateIndex(t *testing.T) {
	srv := newXinferenceEmbedServer(t, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_, _ = io.WriteString(w, `{"data":[{"index":0,"embedding":[0.1]},{"index":0,"embedding":[0.2]}]}`)
	})
	defer srv.Close()

	x := newXinferenceForTest(srv.URL)
	model := "bge-m3"
	_, err := x.Embed(&model, []string{"a", "b"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "duplicate embedding index") {
		t.Errorf("expected duplicate-index error, got %v", err)
	}
}

func TestXinferenceEmbedRejectsOutOfRangeIndex(t *testing.T) {
	srv := newXinferenceEmbedServer(t, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_, _ = io.WriteString(w, `{"data":[{"index":5,"embedding":[0.1]}]}`)
	})
	defer srv.Close()

	x := newXinferenceForTest(srv.URL)
	model := "bge-m3"
	_, err := x.Embed(&model, []string{"a", "b"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("expected out-of-range error, got %v", err)
	}
}

func TestXinferenceEmbedRejectsMissingIndex(t *testing.T) {
	srv := newXinferenceEmbedServer(t, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		// Two inputs requested but only one returned — index 1 is missing.
		_, _ = io.WriteString(w, `{"data":[{"index":0,"embedding":[0.1]}]}`)
	})
	defer srv.Close()

	x := newXinferenceForTest(srv.URL)
	model := "bge-m3"
	_, err := x.Embed(&model, []string{"a", "b"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing embedding") {
		t.Errorf("expected missing-embedding error, got %v", err)
	}
}

func TestXinferenceEmbedEmptyTextsShortCircuits(t *testing.T) {
	x := newXinferenceForTest("http://unused")
	model := "bge-m3"
	got, err := x.Embed(&model, nil, &APIConfig{}, nil)
	if err != nil {
		t.Fatalf("expected nil error for empty inputs, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got)=%d, want 0", len(got))
	}
}

func TestXinferenceEmbedRequiresModelName(t *testing.T) {
	x := newXinferenceForTest("http://unused")
	_, err := x.Embed(nil, []string{"x"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("expected model-name error, got %v", err)
	}
}

func TestXinferenceEmbedSurfacesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"model not loaded"}`)
	}))
	defer srv.Close()

	x := newXinferenceForTest(srv.URL)
	model := "bge-m3"
	_, err := x.Embed(&model, []string{"x"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "Xinference embeddings API error") {
		t.Errorf("expected API error, got %v", err)
	}
}

func TestXinferenceEmbedRejectsMissingEmbeddingSuffix(t *testing.T) {
	x := NewXinferenceModel(
		map[string]string{"default": "http://unused"},
		URLSuffix{Chat: "v1/chat/completions"}, // no Embedding suffix
	)
	model := "bge-m3"
	_, err := x.Embed(&model, []string{"x"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "no embedding URL suffix configured") {
		t.Errorf("expected missing-suffix error, got %v", err)
	}
}

func TestXinferenceMissingBaseURLFailsClearly(t *testing.T) {
	x := NewXinferenceModel(map[string]string{}, URLSuffix{Chat: "v1/chat/completions"})
	_, err := x.ChatWithMessages("qwen2.5-instruct",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "base URL") {
		t.Errorf("expected missing-base-URL error, got %v", err)
	}
}

func TestXinferenceUnsupportedMethodsReturnNoSuchMethod(t *testing.T) {
	x := newXinferenceForTest("http://unused")
	model := "qwen2.5-instruct"

	if _, err := x.Balance(&APIConfig{}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance: expected no such method, got %v", err)
	}
	// TranscribeAudio IS implemented; it validates inputs after APIConfigCheck.
	// The test passes nil file, which should yield an input-validation error,
	// not an api-key error.
	if _, err := x.TranscribeAudio(&model, nil, &APIConfig{}, nil); err == nil || strings.Contains(err.Error(), "api key is required") {
		t.Errorf("TranscribeAudio: expected input validation error (file missing), got %v", err)
	}
	if err := x.TranscribeAudioWithSender(&model, nil, &APIConfig{}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudioWithSender: expected no such method, got %v", err)
	}
	// AudioSpeech IS implemented; it validates inputs after APIConfigCheck.
	if _, err := x.AudioSpeech(&model, nil, &APIConfig{}, nil); err == nil || strings.Contains(err.Error(), "api key is required") {
		t.Errorf("AudioSpeech: expected input validation error (text missing), got %v", err)
	}
	if err := x.AudioSpeechWithSender(&model, nil, &APIConfig{}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeechWithSender: expected no such method, got %v", err)
	}
	if _, err := x.OCRFile(&model, nil, nil, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile: expected no such method, got %v", err)
	}
}

func newXinferenceRerankServer(t *testing.T, expectedAuth string, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rerank" {
			t.Errorf("path=%s want /v1/rerank", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method=%s want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != expectedAuth {
			t.Errorf("Authorization=%q want %q", got, expectedAuth)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("Content-Type=%q", got)
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
	}))
}

func TestXinferenceRerankHappyPathReordersByIndex(t *testing.T) {
	srv := newXinferenceRerankServer(t, "", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "bge-reranker-v2-m3" {
			t.Errorf("model=%v", body["model"])
		}
		if body["query"] != "capital of France" {
			t.Errorf("query=%v", body["query"])
		}
		if got := body["top_n"].(float64); got != 3 {
			t.Errorf("top_n=%v want 3", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{"index": 2, "relevance_score": 0.91},
				{"index": 0, "relevance_score": 0.88},
				{"index": 1, "relevance_score": 0.42},
			},
		})
	})
	defer srv.Close()

	x := newXinferenceForTest(srv.URL)
	model := "bge-reranker-v2-m3"
	resp, err := x.Rerank(&model, "capital of France",
		[]string{"Paris is the capital of France.", "Eiffel Tower.", "Berlin is the capital of Germany."},
		&APIConfig{}, nil,
	)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(resp.Data) != 3 {
		t.Fatalf("Data len=%d", len(resp.Data))
	}
	if resp.Data[0].Index != 2 || resp.Data[1].Index != 0 || resp.Data[2].Index != 1 {
		t.Errorf("order=%v %v %v", resp.Data[0].Index, resp.Data[1].Index, resp.Data[2].Index)
	}
	if resp.Data[0].RelevanceScore != 0.91 {
		t.Errorf("top score=%v", resp.Data[0].RelevanceScore)
	}
}

func TestXinferenceRerankNormalizesV1BaseURL(t *testing.T) {
	srv := newXinferenceRerankServer(t, "Bearer test-key", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []map[string]interface{}{}})
	})
	defer srv.Close()

	x := NewXinferenceModel(
		map[string]string{"default": srv.URL + "/v1"},
		URLSuffix{Rerank: "v1/rerank"},
	)
	apiKey := "test-key"
	model := "bge-reranker-v2-m3"
	_, err := x.Rerank(&model, "q", []string{"a"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
}

func TestXinferenceRerankRespectsTopNConfig(t *testing.T) {
	srv := newXinferenceRerankServer(t, "", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if got := body["top_n"].(float64); got != 2 {
			t.Errorf("top_n=%v want 2", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []map[string]interface{}{}})
	})
	defer srv.Close()

	x := newXinferenceForTest(srv.URL)
	model := "bge-reranker-v2-m3"
	_, err := x.Rerank(&model, "q", []string{"a", "b", "c", "d"}, &APIConfig{}, &RerankConfig{TopN: 2})
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
}

func TestXinferenceRerankEmptyDocumentsShortCircuits(t *testing.T) {
	x := newXinferenceForTest("http://unused")
	model := "bge-reranker-v2-m3"
	resp, err := x.Rerank(&model, "q", nil, &APIConfig{}, nil)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("Data len=%d want 0", len(resp.Data))
	}
}

func TestXinferenceRerankRequiresModelName(t *testing.T) {
	x := newXinferenceForTest("http://unused")
	_, err := x.Rerank(nil, "q", []string{"a"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("err=%v", err)
	}
}

func TestXinferenceRerankRejectsOutOfRangeIndex(t *testing.T) {
	srv := newXinferenceRerankServer(t, "", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{{"index": 5, "relevance_score": 0.1}},
		})
	})
	defer srv.Close()

	x := newXinferenceForTest(srv.URL)
	model := "bge-reranker-v2-m3"
	_, err := x.Rerank(&model, "q", []string{"a", "b"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("err=%v", err)
	}
}

func TestXinferenceRerankRejectsDuplicateIndex(t *testing.T) {
	srv := newXinferenceRerankServer(t, "", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{"index": 0, "relevance_score": 0.9},
				{"index": 0, "relevance_score": 0.8},
			},
		})
	})
	defer srv.Close()

	x := newXinferenceForTest(srv.URL)
	model := "bge-reranker-v2-m3"
	_, err := x.Rerank(&model, "q", []string{"a", "b"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("err=%v", err)
	}
}

func TestXinferenceRerankSurfacesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"model not loaded"}`))
	}))
	defer srv.Close()

	x := newXinferenceForTest(srv.URL)
	model := "bge-reranker-v2-m3"
	_, err := x.Rerank(&model, "q", []string{"a"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "Xinference rerank API error") {
		t.Errorf("err=%v", err)
	}
}

func TestXinferenceRerankRejectsMissingRerankSuffix(t *testing.T) {
	x := NewXinferenceModel(
		map[string]string{"default": "http://unused"},
		URLSuffix{Chat: "v1/chat/completions"},
	)
	model := "bge-reranker-v2-m3"
	_, err := x.Rerank(&model, "q", []string{"a"}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "no rerank URL suffix configured") {
		t.Errorf("err=%v", err)
	}
}
