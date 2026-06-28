package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newJieKouAIServer(t *testing.T, handler func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization=Bearer test-key, got %q", got)
			return
		}

		var body map[string]interface{}
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
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("unmarshal: %v\nraw=%s", err, string(raw))
				return
			}
		} else {
			if r.ContentLength > 0 {
				t.Errorf("expected %s request without body, ContentLength=%d", r.Method, r.ContentLength)
				return
			}
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
				return
			}
			if len(raw) != 0 {
				t.Errorf("expected %s request without body, got %q", r.Method, string(raw))
				return
			}
		}

		handler(t, r, body, w)
	}))
}

func newJieKouAIForTest(baseURL string) *JieKouAIModel {
	return NewJieKouAIModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:      "openai/v1/chat/completions",
			Embedding: "openai/v1/embeddings",
			Rerank:    "openai/v1/rerank",
			Models:    "openai/v1/models",
		},
	)
}

func TestJieKouAIChatForcesNonStreaming(t *testing.T) {
	srv := newJieKouAIServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
		}
		if r.URL.Path != "/openai/v1/chat/completions" {
			t.Errorf("path=%s, want /openai/v1/chat/completions", r.URL.Path)
		}
		if body["stream"] != false {
			t.Errorf("stream=%v, want false", body["stream"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content":           "pong",
					"reasoning_content": "\nthought",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	stream := true
	thinking := true
	resp, err := newJieKouAIForTest(srv.URL).ChatWithMessages(
		" gpt-5 ",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream, Thinking: &thinking},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Errorf("Answer=%v, want pong", resp.Answer)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "thought" {
		t.Errorf("ReasonContent=%v, want thought", resp.ReasonContent)
	}
}

func TestJieKouAIStreamForcesStreaming(t *testing.T) {
	srv := newJieKouAIServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/openai/v1/chat/completions" {
			t.Errorf("path=%s, want /openai/v1/chat/completions", r.URL.Path)
		}
		if body["stream"] != true {
			t.Errorf("stream=%v, want true", body["stream"])
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("Accept=%q, want text/event-stream", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, strings.Join([]string{
			`data: {"choices":[{"delta":{"reasoning_content":"thinking"}}]}`,
			`data: {"choices":[{"delta":{"content":"hello"}}]}`,
			`data: [DONE]`,
			``,
		}, "\n"))
	})
	defer srv.Close()

	apiKey := "test-key"
	stream := false
	var content, reasoning []string
	err := newJieKouAIForTest(srv.URL).ChatStreamlyWithSender(
		"gpt-5",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		func(answer, reason *string) error {
			if answer != nil {
				content = append(content, *answer)
			}
			if reason != nil {
				reasoning = append(reasoning, *reason)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if got := strings.Join(content, ""); got != "hello[DONE]" {
		t.Errorf("content=%q, want hello[DONE]", got)
	}
	if got := strings.Join(reasoning, ""); got != "thinking" {
		t.Errorf("reasoning=%q, want thinking", got)
	}
}

func TestJieKouAIListModelsHappyPath(t *testing.T) {
	srv := newJieKouAIServer(t, func(t *testing.T, r *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		if r.URL.Path != "/openai/v1/models" {
			t.Errorf("path=%s, want /openai/v1/models", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{
				{"id": "gpt-5"},
				{"id": " text-embedding-3-large "},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	models, err := newJieKouAIForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if got := joinModelNames(models, ","); got != "gpt-5,text-embedding-3-large" {
		t.Errorf("models=%q", got)
	}
}

func TestJieKouAIListModelsRejectsMalformedResponse(t *testing.T) {
	apiKey := "test-key"
	for name, response := range map[string]interface{}{
		"missing data": map[string]interface{}{"object": "list"},
		"empty id":     map[string]interface{}{"data": []map[string]string{{"id": ""}}},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newJieKouAIServer(t, func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
				_ = json.NewEncoder(w).Encode(response)
			})
			defer srv.Close()

			if _, err := newJieKouAIForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey}); err == nil {
				t.Fatal("expected malformed response error")
			}
		})
	}
}

func TestJieKouAIEmbedSendsValidatedRequest(t *testing.T) {
	srv := newJieKouAIServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/openai/v1/embeddings" {
			t.Errorf("path=%s, want /openai/v1/embeddings", r.URL.Path)
		}
		if body["model"] != "text-embedding-3-large" {
			t.Errorf("model=%v, want text-embedding-3-large", body["model"])
		}
		inputs, ok := body["input"].([]interface{})
		if !ok || len(inputs) != 1 || inputs[0] != "hello" {
			t.Errorf("input=%v, want [hello]", body["input"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{{
				"embedding": []float64{0.1, 0.2},
				"index":     0,
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := " text-embedding-3-large "
	embeddings, err := newJieKouAIForTest(srv.URL).Embed(&model, []string{"hello"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(embeddings) != 1 || embeddings[0].Index != 0 || len(embeddings[0].Embedding) != 2 {
		t.Fatalf("embeddings=%v", embeddings)
	}
}

func TestJieKouAIRerankHandlesNilConfig(t *testing.T) {
	srv := newJieKouAIServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/openai/v1/rerank" {
			t.Errorf("path=%s, want /openai/v1/rerank", r.URL.Path)
		}
		if body["model"] != "baai/bge-reranker-v2-m3" {
			t.Errorf("model=%v, want baai/bge-reranker-v2-m3", body["model"])
		}
		if body["query"] != "question" {
			t.Errorf("query=%v, want question", body["query"])
		}
		if _, ok := body["top_n"]; ok {
			t.Errorf("top_n=%v, want omitted", body["top_n"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{{
				"index":           0,
				"relevance_score": 0.9,
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := " baai/bge-reranker-v2-m3 "
	resp, err := newJieKouAIForTest(srv.URL).Rerank(&model, " question ", []string{"doc"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if resp == nil || len(resp.Data) != 1 || resp.Data[0].Index != 0 || resp.Data[0].RelevanceScore != 0.9 {
		t.Fatalf("Rerank response=%v", resp)
	}
}

func TestJieKouAIValidatesInputs(t *testing.T) {
	apiKey := "test-key"
	emptyKey := "  "
	model := "gpt-5"
	send := func(*string, *string) error { return nil }

	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "chat api key",
			run: func() error {
				_, err := newJieKouAIForTest("http://unused").ChatWithMessages("gpt-5", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &emptyKey}, nil)
				return err
			},
			want: "api key is required",
		},
		{
			name: "chat model",
			run: func() error {
				_, err := newJieKouAIForTest("http://unused").ChatWithMessages("  ", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
				return err
			},
			want: "model name is required",
		},
		{
			name: "stream api key",
			run: func() error {
				return newJieKouAIForTest("http://unused").ChatStreamlyWithSender("gpt-5", []Message{{Role: "user", Content: "x"}}, nil, nil, send)
			},
			want: "api key is required",
		},
		{
			name: "stream sender",
			run: func() error {
				return newJieKouAIForTest("http://unused").ChatStreamlyWithSender("gpt-5", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, nil)
			},
			want: "sender is required",
		},
		{
			name: "embed model",
			run: func() error {
				_, err := newJieKouAIForTest("http://unused").Embed(nil, []string{"x"}, &APIConfig{ApiKey: &apiKey}, nil)
				return err
			},
			want: "model name is required",
		},
		{
			name: "embed api key",
			run: func() error {
				_, err := newJieKouAIForTest("http://unused").Embed(&model, []string{"x"}, nil, nil)
				return err
			},
			want: "api key is required",
		},
		{
			name: "rerank model",
			run: func() error {
				_, err := newJieKouAIForTest("http://unused").Rerank(nil, "q", []string{"doc"}, &APIConfig{ApiKey: &apiKey}, nil)
				return err
			},
			want: "model name is required",
		},
		{
			name: "rerank api key",
			run: func() error {
				_, err := newJieKouAIForTest("http://unused").Rerank(&model, "q", []string{"doc"}, &APIConfig{ApiKey: &emptyKey}, nil)
				return err
			},
			want: "api key is required",
		},
		{
			name: "rerank query",
			run: func() error {
				_, err := newJieKouAIForTest("http://unused").Rerank(&model, "  ", []string{"doc"}, &APIConfig{ApiKey: &apiKey}, nil)
				return err
			},
			want: "query is required",
		},
		{
			name: "models api key",
			run: func() error {
				_, err := newJieKouAIForTest("http://unused").ListModels(&APIConfig{})
				return err
			},
			want: "api key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}
