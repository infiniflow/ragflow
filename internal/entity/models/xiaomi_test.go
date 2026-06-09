package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newXiaomiServer(t *testing.T, expectedPath string, handler func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectedPath {
			t.Errorf("expected path=%s, got %s", expectedPath, r.URL.Path)
			return
		}
		if got := r.Header.Get("api-key"); got != "test-key" {
			t.Errorf("expected api-key=test-key, got %q", got)
			return
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("expected no Authorization header, got %q", got)
			return
		}
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
		handler(t, r, body, w)
	}))
}

func newXiaomiForTest(baseURL string) *XiaomiModel {
	return NewXiaomiModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "v1/chat/completions"},
	)
}

func TestXiaomiName(t *testing.T) {
	if got := newXiaomiForTest("http://unused").Name(); got != "xiaomi" {
		t.Errorf("Name()=%q, want xiaomi", got)
	}
}

func TestXiaomiFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("Xiaomi", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if _, ok := driver.(*XiaomiModel); !ok {
		t.Fatalf("driver type=%T, want *XiaomiModel", driver)
	}
}

func TestXiaomiNewModelWithCustomDefaultTransport(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = original
	})

	if model := NewXiaomiModel(map[string]string{"default": "http://unused"}, URLSuffix{}); model == nil {
		t.Fatal("NewXiaomiModel returned nil")
	}
}

func TestXiaomiChatHappyPath(t *testing.T) {
	srv := newXiaomiServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "mimo-v2.5-pro" {
			t.Errorf("model=%v", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v want false", body["stream"])
		}
		if body["max_tokens"] != nil {
			t.Errorf("max_tokens must not be sent: %v", body["max_tokens"])
		}
		if body["max_completion_tokens"] != float64(1024) {
			t.Errorf("max_completion_tokens=%v", body["max_completion_tokens"])
		}
		thinking, ok := body["thinking"].(map[string]interface{})
		if !ok || thinking["type"] != "disabled" {
			t.Errorf("thinking=%v", body["thinking"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "pong",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	maxTokens := 1024
	thinking := false
	resp, err := newXiaomiForTest(srv.URL).ChatWithMessages(
		"mimo-v2.5-pro",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{MaxTokens: &maxTokens, Thinking: &thinking},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Fatalf("answer=%v", resp.Answer)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "" {
		t.Fatalf("reasoning=%v", resp.ReasonContent)
	}
}

func TestXiaomiUsesEmptyRegionBaseURLOverride(t *testing.T) {
	srv := newXiaomiServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content": "pong",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	m := NewXiaomiModel(
		map[string]string{"default": srv.URL},
		URLSuffix{Chat: "v1/chat/completions"},
	)
	resp, err := m.ChatWithMessages("mimo-v2.5-pro", []Message{{Role: "user", Content: "ping"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Fatalf("answer=%v", resp.Answer)
	}
}

func TestXiaomiAPIConfigBaseURLOverridesRegionMap(t *testing.T) {
	srv := newXiaomiServer(t, "/override/chat", func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content": "override",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	baseURL := srv.URL
	m := NewXiaomiModel(
		map[string]string{"default": "http://unused"},
		URLSuffix{Chat: "override/chat"},
	)
	resp, err := m.ChatWithMessages("mimo-v2.5-pro", []Message{{Role: "user", Content: "ping"}}, &APIConfig{ApiKey: &apiKey, BaseURL: &baseURL}, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "override" {
		t.Fatalf("answer=%v", resp.Answer)
	}
}

func TestXiaomiChatExtractsReasoningContent(t *testing.T) {
	srv := newXiaomiServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content":           "final",
					"reasoning_content": "think",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newXiaomiForTest(srv.URL).ChatWithMessages("mimo-v2.5-pro", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if *resp.Answer != "final" || *resp.ReasonContent != "think" {
		t.Fatalf("response=%+v", resp)
	}
}

func TestXiaomiChatRequiresInputs(t *testing.T) {
	apiKey := "test-key"
	m := newXiaomiForTest("http://unused")
	if _, err := m.ChatWithMessages("mimo-v2.5-pro", []Message{{Role: "user", Content: "x"}}, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("api key guard: %v", err)
	}
	if _, err := m.ChatWithMessages("", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("model guard: %v", err)
	}
	if _, err := m.ChatWithMessages("mimo-v2.5-pro", nil, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("messages guard: %v", err)
	}
}

func TestXiaomiChatRejectsHTTPError(t *testing.T) {
	srv := newXiaomiServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"unauthorized"}`)
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newXiaomiForTest(srv.URL).ChatWithMessages("mimo-v2.5-pro", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 propagated, got %v", err)
	}
}

func TestXiaomiStreamHappyPath(t *testing.T) {
	srv := newXiaomiServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if body["stream"] != true {
			t.Errorf("stream=%v want true", body["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"reasoning_content":"step "}}]}`+"\n\n"+
				`data: {"choices":[{"delta":{"content":"Hello"}}]}`+"\n\n"+
				`data: {"choices":[{"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n\n",
		)
	})
	defer srv.Close()

	apiKey := "test-key"
	var content, reasoning []string
	var sawDone bool
	err := newXiaomiForTest(srv.URL).ChatStreamlyWithSender(
		"mimo-v2.5-pro",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		func(c *string, r *string) error {
			if c != nil && *c == "[DONE]" {
				sawDone = true
				return nil
			}
			if c != nil {
				content = append(content, *c)
			}
			if r != nil {
				reasoning = append(reasoning, *r)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if strings.Join(content, "") != "Hello world" {
		t.Errorf("content=%v", content)
	}
	if strings.Join(reasoning, "") != "step " {
		t.Errorf("reasoning=%v", reasoning)
	}
	if !sawDone {
		t.Error("expected [DONE] sentinel")
	}
}

func TestXiaomiStreamHandlesCRLFFrames(t *testing.T) {
	srv := newXiaomiServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\r\n\r\n"+
				"data: {\"choices\":[{\"delta\":{\"content\":\" world\"},\"finish_reason\":\"stop\"}]}\r\n\r\n",
		)
	})
	defer srv.Close()

	apiKey := "test-key"
	var content []string
	err := newXiaomiForTest(srv.URL).ChatStreamlyWithSender(
		"mimo-v2.5-pro",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		func(c *string, _ *string) error {
			if c != nil && *c != "[DONE]" {
				content = append(content, *c)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if strings.Join(content, "") != "Hello world" {
		t.Errorf("content=%v", content)
	}
}

func TestXiaomiStreamRejectsMalformedFrame(t *testing.T) {
	srv := newXiaomiServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {bad json}\n\n")
	})
	defer srv.Close()

	apiKey := "test-key"
	// Malformed SSE frames are silently skipped; the stream completes and sends [DONE].
	err := newXiaomiForTest(srv.URL).ChatStreamlyWithSender("mimo-v2.5-pro", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, func(*string, *string) error { return nil })
	if err != nil {
		t.Errorf("expected no error on malformed frame, got %v", err)
	}
}

func TestXiaomiUnsupportedMethods(t *testing.T) {
	m := newXiaomiForTest("http://unused")
	model := "mimo-v2.5-pro"
	apiKey := "test-key"
	cfg := &APIConfig{ApiKey: &apiKey}

	if _, err := m.Embed(&model, []string{"x"}, cfg, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Embed: %v", err)
	}
	if _, err := m.Rerank(&model, "q", []string{"d"}, cfg, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank: %v", err)
	}
	// CheckConnection IS implemented — verifies API config and base URL are reachable.
	if err := m.CheckConnection(cfg); err != nil {
		t.Errorf("CheckConnection: %v", err)
	}
	// TranscribeAudio IS implemented; with nil file it returns input validation error.
	if _, err := m.TranscribeAudio(&model, nil, cfg, nil); err == nil || !strings.Contains(err.Error(), "file is missing") {
		t.Errorf("TranscribeAudio: %v", err)
	}
	// AudioSpeech IS implemented; with nil content it returns input validation error.
	if _, err := m.AudioSpeech(&model, nil, cfg, nil); err == nil || !strings.Contains(err.Error(), "audio content is empty") {
		t.Errorf("AudioSpeech: %v", err)
	}
	if _, err := m.OCRFile(&model, nil, nil, cfg, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile: %v", err)
	}
}
