package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newAI302Server(t *testing.T, handler func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
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

func newAI302ForTest(baseURL string) *AI302Model {
	return NewAI302Model(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:          "v1/chat/completions",
			Embedding:     "jina/v1/embeddings",
			Rerank:        "jina/v1/rerank",
			Models:        "v1/models",
			ASR:           "v1/audio/transcriptions",
			OCR:           "mistral/v1/ocr",
			DocumentParse: "mineru/api/v4/extract/task",
		},
	)
}

func TestAI302ChatForcesNonStreaming(t *testing.T) {
	srv := newAI302Server(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%s, want /v1/chat/completions", r.URL.Path)
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
	resp, err := newAI302ForTest(srv.URL).ChatWithMessages(
		"gpt-5",
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

func TestAI302StreamForcesStreaming(t *testing.T) {
	srv := newAI302Server(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%s, want /v1/chat/completions", r.URL.Path)
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
	err := newAI302ForTest(srv.URL).ChatStreamlyWithSender(
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

func TestAI302ListModelsHappyPath(t *testing.T) {
	srv := newAI302Server(t, func(t *testing.T, r *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/models" {
			t.Errorf("path=%s, want /v1/models", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{
				{"id": "gpt-5"},
				{"id": " jina-embeddings-v3 "},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	models, err := newAI302ForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if got := strings.Join(models, ","); got != "gpt-5,jina-embeddings-v3" {
		t.Errorf("models=%q", got)
	}
}

func TestAI302ListModelsRejectsMalformedResponse(t *testing.T) {
	apiKey := "test-key"
	for name, response := range map[string]interface{}{
		"missing data": map[string]interface{}{"object": "list"},
		"empty id":     map[string]interface{}{"data": []map[string]string{{"id": ""}}},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newAI302Server(t, func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
				_ = json.NewEncoder(w).Encode(response)
			})
			defer srv.Close()

			if _, err := newAI302ForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey}); err == nil {
				t.Fatal("expected malformed response error")
			}
		})
	}
}

func TestAI302ShowTaskEscapesTaskID(t *testing.T) {
	srv := newAI302Server(t, func(t *testing.T, r *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		want := "/mineru/api/v4/extract/task/task%2Fwith%3Fquery%23fragment"
		if r.RequestURI != want {
			t.Errorf("RequestURI=%q, want %q", r.RequestURI, want)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"state":        "done",
				"full_zip_url": "https://example.com/result.zip",
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newAI302ForTest(srv.URL).ShowTask(" task/with?query#fragment ", &APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ShowTask: %v", err)
	}
	if len(resp.Segments) != 1 || resp.Segments[0].Content != "https://example.com/result.zip" {
		t.Fatalf("Segments=%v", resp.Segments)
	}
}

func TestAI302ValidatesInputs(t *testing.T) {
	apiKey := "test-key"
	emptyKey := "  "
	model := "gpt-5"
	file := "  "
	blankURL := "  "
	docURL := "https://example.com/doc.pdf"
	invalidURL := "ftp://example.com/doc.pdf"
	send := func(*string, *string) error { return nil }

	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "chat api key",
			run: func() error {
				_, err := newAI302ForTest("http://unused").ChatWithMessages("gpt-5", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &emptyKey}, nil)
				return err
			},
			want: "api key is required",
		},
		{
			name: "chat model",
			run: func() error {
				_, err := newAI302ForTest("http://unused").ChatWithMessages("  ", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
				return err
			},
			want: "model name is required",
		},
		{
			name: "stream api key",
			run: func() error {
				return newAI302ForTest("http://unused").ChatStreamlyWithSender("gpt-5", []Message{{Role: "user", Content: "x"}}, nil, nil, send)
			},
			want: "api key is required",
		},
		{
			name: "stream sender",
			run: func() error {
				return newAI302ForTest("http://unused").ChatStreamlyWithSender("gpt-5", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, nil)
			},
			want: "sender is required",
		},
		{
			name: "embed model",
			run: func() error {
				_, err := newAI302ForTest("http://unused").Embed(nil, []string{"x"}, &APIConfig{ApiKey: &apiKey}, nil)
				return err
			},
			want: "model name is required",
		},
		{
			name: "rerank api key",
			run: func() error {
				_, err := newAI302ForTest("http://unused").Rerank(&model, "q", []string{"doc"}, nil, nil)
				return err
			},
			want: "api key is required",
		},
		{
			name: "rerank query",
			run: func() error {
				_, err := newAI302ForTest("http://unused").Rerank(&model, "  ", []string{"doc"}, &APIConfig{ApiKey: &apiKey}, nil)
				return err
			},
			want: "query is required",
		},
		{
			name: "asr model",
			run: func() error {
				_, err := newAI302ForTest("http://unused").TranscribeAudio(nil, &docURL, &APIConfig{ApiKey: &apiKey}, nil)
				return err
			},
			want: "model name is required",
		},
		{
			name: "asr file",
			run: func() error {
				_, err := newAI302ForTest("http://unused").TranscribeAudio(&model, &file, &APIConfig{ApiKey: &apiKey}, nil)
				return err
			},
			want: "file is missing",
		},
		{
			name: "ocr api key",
			run: func() error {
				_, err := newAI302ForTest("http://unused").OCRFile(&model, nil, &docURL, nil, nil)
				return err
			},
			want: "api key is required",
		},
		{
			name: "ocr input",
			run: func() error {
				_, err := newAI302ForTest("http://unused").OCRFile(&model, nil, &blankURL, &APIConfig{ApiKey: &apiKey}, nil)
				return err
			},
			want: "file url or content is required",
		},
		{
			name: "ocr invalid url",
			run: func() error {
				_, err := newAI302ForTest("http://unused").OCRFile(&model, nil, &invalidURL, &APIConfig{ApiKey: &apiKey}, nil)
				return err
			},
			want: "invalid document URL",
		},
		{
			name: "parse file url",
			run: func() error {
				_, err := newAI302ForTest("http://unused").ParseFile(&model, nil, &blankURL, &APIConfig{ApiKey: &apiKey}, nil)
				return err
			},
			want: "valid public document URL",
		},
		{
			name: "parse file invalid url",
			run: func() error {
				_, err := newAI302ForTest("http://unused").ParseFile(&model, nil, &invalidURL, &APIConfig{ApiKey: &apiKey}, nil)
				return err
			},
			want: "invalid document URL",
		},
		{
			name: "models api key",
			run: func() error {
				_, err := newAI302ForTest("http://unused").ListModels(&APIConfig{})
				return err
			},
			want: "api key is required",
		},
		{
			name: "show task id",
			run: func() error {
				_, err := newAI302ForTest("http://unused").ShowTask("  ", &APIConfig{ApiKey: &apiKey})
				return err
			},
			want: "task id is required",
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
