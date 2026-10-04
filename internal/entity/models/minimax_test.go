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

func newMinimaxServer(t *testing.T, handler func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
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

func newMinimaxForTest(baseURL string) *MinimaxModel {
	return NewMinimaxModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:   "v1/text/chatcompletion_v2",
			Models: "v1/models",
			Files:  "v1/files/list",
		},
	)
}

func TestMinimaxNewInstancePreservesConfig(t *testing.T) {
	model := NewMinimaxModel(
		map[string]string{"default": "http://old.example"},
		URLSuffix{Chat: "chat", Models: "models"},
	)

	instance, ok := model.NewInstance(map[string]string{"default": "http://new.example"}).(*MinimaxModel)
	if !ok {
		t.Fatalf("NewInstance type=%T, want *MinimaxModel", instance)
	}
	if instance.baseModel.BaseURL["default"] != "http://new.example" {
		t.Errorf("BaseURL=%q", instance.baseModel.BaseURL["default"])
	}
	if instance.baseModel.URLSuffix.Chat != "chat" || instance.baseModel.URLSuffix.Models != "models" {
		t.Errorf("URLSuffix=%+v", instance.baseModel.URLSuffix)
	}
	if instance.baseModel.httpClient == nil {
		t.Error("httpClient is nil")
	}
}

func TestMinimaxChatForcesNonStreaming(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newMinimaxServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/text/chatcompletion_v2" {
			t.Errorf("path=%s, want /v1/text/chatcompletion_v2", r.URL.Path)
		}
		if body["model"] != "MiniMax-M3" {
			t.Errorf("model=%v, want MiniMax-M3", body["model"])
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

	apiKey := " test-key "
	stream := true
	thinking := true
	resp, err := newMinimaxForTest(srv.URL).ChatWithMessages(
		ctx,
		" MiniMax-M3 ",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream, Thinking: &thinking},
		nil,
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

func TestMinimaxChatRejectsEmptyChoices(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newMinimaxServer(t, func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"choices": []map[string]interface{}{}})
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newMinimaxForTest(srv.URL).ChatWithMessages(
		ctx,
		"MiniMax-M3",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "no choices in response") {
		t.Fatalf("expected choices error, got %v", err)
	}
}

func TestMinimaxChatSurfacesBaseRespError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newMinimaxServer(t, func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		// MiniMax rate-limit: HTTP 200 with base_resp, no choices.
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"base_resp": map[string]interface{}{
				"status_code": 1027,
				"status_msg":  "frequency limit reached",
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newMinimaxForTest(srv.URL).ChatWithMessages(
		ctx,
		"MiniMax-M3",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "frequency limit reached") {
		t.Fatalf("expected rate-limit message, got %v", err)
	}
}

func TestMinimaxChatSurfacesOpenAIError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newMinimaxServer(t, func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": "rate limit exceeded",
				"type":    "rate_limit_error",
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newMinimaxForTest(srv.URL).ChatWithMessages(
		ctx,
		"MiniMax-M3",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Fatalf("expected rate-limit message, got %v", err)
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("expected status code 429 in error, got %v", err)
	}
}

func TestMinimaxStreamSurfacesBaseRespError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newMinimaxServer(t, func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Error event before any choices — rate limit.
		_, _ = io.WriteString(w, `data: {"base_resp":{"status_code":1027,"status_msg":"frequency limit reached"}}`+"\n")
	})
	defer srv.Close()

	apiKey := "test-key"
	err := newMinimaxForTest(srv.URL).ChatStreamlyWithSender(
		ctx,
		"MiniMax-M3",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		nil,
		func(*string, *string) error { return nil },
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "frequency limit reached") {
		t.Fatalf("expected rate-limit message, got %v", err)
	}
}

func TestMinimaxStreamForcesStreaming(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newMinimaxServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/text/chatcompletion_v2" {
			t.Errorf("path=%s, want /v1/text/chatcompletion_v2", r.URL.Path)
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
	err := newMinimaxForTest(srv.URL).ChatStreamlyWithSender(
		ctx,
		"MiniMax-M3",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		nil,
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

// TestMinimaxStreamReturnsWhenServerHoldsConnectionOpen pins that a completed
// stream is reported as soon as its terminal event arrives.
//
// MiniMax ends an SSE response by sending `data: [DONE]` but leaves the HTTP
// connection open instead of closing it. The base_resp sniffing goroutine reads
// the body until EOF, so with the connection held open it stays parked in
// scanner.Scan(). The stream used to wait for that goroutine after
// HandleStreamingResponse had already returned, so a finished turn blocked for
// the whole streamCallTimeout (20 minutes) and the chat never emitted its
// terminal `final: true` event — the UI sat on "thinking" with the complete
// answer already rendered.
//
// Every other MiniMax stream test closes the server connection, so none of them
// could see this. Holding the connection open here is what MiniMax actually
// does.
func TestMinimaxStreamReturnsWhenServerHoldsConnectionOpen(t *testing.T) {
	withSSRFBypass(t)

	released := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(released) }) }

	srv := newMinimaxServer(t, func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, strings.Join([]string{
			`data: {"choices":[{"delta":{"content":"hello"}}]}`,
			`data: [DONE]`,
			``,
		}, "\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Never return: the response body stays open, so the reader sees no
		// EOF. This is what the live provider does after [DONE].
		<-released
	})
	// Release the handler BEFORE closing the server: srv.Close() blocks until
	// outstanding requests finish, so closing in the other order deadlocks the
	// test binary instead of failing the assertion.
	defer func() {
		release()
		srv.Close()
	}()

	apiKey := "test-key"
	var content, reasoning []string
	done := make(chan error, 1)
	go func() {
		done <- newMinimaxForTest(srv.URL).ChatStreamlyWithSender(
			t.Context(),
			"MiniMax-M3",
			[]Message{{Role: "user", Content: "ping"}},
			&APIConfig{ApiKey: &apiKey},
			nil,
			nil,
			func(answer *string, reason *string) error {
				if answer != nil && *answer != "" {
					content = append(content, *answer)
				}
				if reason != nil && *reason != "" {
					reasoning = append(reasoning, *reason)
				}
				return nil
			},
		)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ChatStreamlyWithSender returned %v, want nil once [DONE] was seen", err)
		}
		if got := strings.Join(content, ""); !strings.HasPrefix(got, "hello") {
			t.Errorf("content=%q, want it to start with the streamed answer", got)
		}
		_ = reasoning
	case <-time.After(15 * time.Second):
		t.Fatal("ChatStreamlyWithSender did not return after [DONE]: it is waiting on a " +
			"scanner EOF the server never sends, so the chat never emits its final event")
	}
}

// TestMinimaxStreamSurfacesBodyReadError pins that a mid-stream transport
// failure still reaches the caller as an error. Routing the scanner's error
// through the pipe is what preserves this now that the caller no longer waits
// on the scanner goroutine.
func TestMinimaxStreamSurfacesBodyReadError(t *testing.T) {
	withSSRFBypass(t)

	srv := newMinimaxServer(t, func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Announce a body longer than what is written, then end the handler:
		// the client sees a truncated chunked body and Scan fails.
		w.Header().Set("Content-Length", "4096")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"partial"}}]}`+"\n")
	})
	defer srv.Close()

	apiKey := "test-key"
	err := newMinimaxForTest(srv.URL).ChatStreamlyWithSender(
		t.Context(),
		"MiniMax-M3",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		nil,
		func(*string, *string) error { return nil },
	)
	if err == nil {
		t.Fatal("expected an error for a truncated stream, got nil")
	}
}

func TestMinimaxStreamAcceptsNilConfig(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newMinimaxServer(t, func(t *testing.T, _ *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if body["stream"] != true {
			t.Errorf("stream=%v, want true", body["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: [DONE]\n")
	})
	defer srv.Close()

	apiKey := "test-key"
	err := newMinimaxForTest(srv.URL).ChatStreamlyWithSender(
		ctx,
		"MiniMax-M3",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		nil,
		func(*string, *string) error { return nil },
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
}

func TestMinimaxListModelsUsesBodylessGet(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newMinimaxServer(t, func(t *testing.T, r *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/models" {
			t.Errorf("path=%s, want /v1/models", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{
				{"id": "MiniMax-M3"},
				{"id": " minimax-m2.7 "},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	models, err := newMinimaxForTest(srv.URL).ListModels(ctx, &APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if got := joinModelNames(models, ","); got != "MiniMax-M3,minimax-m2.7" {
		t.Errorf("models=%q", got)
	}
}

func TestMinimaxListModelsRejectsMalformedResponse(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	apiKey := "test-key"
	for name, response := range map[string]interface{}{
		"missing data": map[string]interface{}{"object": "list"},
		"empty id":     map[string]interface{}{"data": []map[string]string{{"id": ""}}},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newMinimaxServer(t, func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
				_ = json.NewEncoder(w).Encode(response)
			})
			defer srv.Close()

			if _, err := newMinimaxForTest(srv.URL).ListModels(ctx, &APIConfig{ApiKey: &apiKey}); err == nil {
				t.Fatal("expected malformed response error")
			}
		})
	}
}

func TestMinimaxCheckConnectionUsesListModels(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newMinimaxServer(t, func(t *testing.T, r *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/models" {
			t.Errorf("path=%s, want /v1/models", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{{"id": "MiniMax-M3"}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	if err := newMinimaxForTest(srv.URL).CheckConnection(ctx, &APIConfig{ApiKey: &apiKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestMinimaxValidatesInputs(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	apiKey := "test-key"
	emptyKey := " "
	send := func(*string, *string) error { return nil }

	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "chat api key",
			run: func() error {
				_, err := newMinimaxForTest("http://unused").ChatWithMessages(ctx, "MiniMax-M3", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &emptyKey}, nil, nil)
				return err
			},
			want: "api key is required",
		},
		{
			name: "chat model",
			run: func() error {
				_, err := newMinimaxForTest("http://unused").ChatWithMessages(ctx, " ", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, nil)
				return err
			},
			want: "model name is required",
		},
		{
			name: "stream api key",
			run: func() error {
				return newMinimaxForTest("http://unused").ChatStreamlyWithSender(ctx, "MiniMax-M3", []Message{{Role: "user", Content: "x"}}, nil, nil, nil, send)
			},
			want: "api key is required",
		},
		{
			name: "stream model",
			run: func() error {
				return newMinimaxForTest("http://unused").ChatStreamlyWithSender(ctx, " ", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, nil, send)
			},
			want: "model name is required",
		},
		{
			name: "stream sender",
			run: func() error {
				return newMinimaxForTest("http://unused").ChatStreamlyWithSender(ctx, "MiniMax-M3", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, nil, nil)
			},
			want: "sender is required",
		},
		{
			name: "models api key",
			run: func() error {
				_, err := newMinimaxForTest("http://unused").ListModels(ctx, &APIConfig{})
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
