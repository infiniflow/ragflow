package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newGroqServer(t *testing.T, handler func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
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
		}
		handler(t, r, body, w)
	}))
}

func newGroqForTest(baseURL string) *GroqModel {
	return NewGroqModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestGroqName(t *testing.T) {
	if got := newGroqForTest("http://unused").Name(); got != "groq" {
		t.Errorf("Name()=%q", got)
	}
}

func TestGroqFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("Groq", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*GroqModel); !ok {
		t.Fatalf("driver type=%T, want *GroqModel", driver)
	}
}

type groqStubRoundTripper struct{}

func (groqStubRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, http.ErrUseLastResponse
}

func TestNewGroqModelHandlesCustomDefaultTransport(t *testing.T) {
	originalTransport := http.DefaultTransport
	http.DefaultTransport = groqStubRoundTripper{}
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	model := NewGroqModel(map[string]string{"default": "http://unused"}, URLSuffix{})
	if model == nil || model.httpClient == nil || model.httpClient.Transport == nil {
		t.Fatalf("NewGroqModel returned incomplete model: %#v", model)
	}
}

func TestGroqChatHappyPath(t *testing.T) {
	srv := newGroqServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if body["model"] != "llama-3.3-70b-versatile" {
			t.Errorf("model=%v", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v want false", body["stream"])
		}
		if body["max_tokens"] != float64(32) {
			t.Errorf("max_tokens=%v", body["max_tokens"])
		}
		if body["temperature"] != 0.3 {
			t.Errorf("temperature=%v", body["temperature"])
		}
		if body["top_p"] != 0.9 {
			t.Errorf("top_p=%v", body["top_p"])
		}
		stop, ok := body["stop"].([]interface{})
		if !ok || len(stop) != 1 || stop[0] != "END" {
			t.Errorf("stop=%v", body["stop"])
		}
		if _, ok := body["reasoning_effort"]; ok {
			t.Errorf("reasoning_effort should not be sent: %v", body["reasoning_effort"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content":           "pong",
					"reasoning_content": "thinking",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	maxTokens := 32
	temperature := 0.3
	topP := 0.9
	stop := []string{"END"}
	effort := "high"
	resp, err := newGroqForTest(srv.URL).ChatWithMessages(
		"llama-3.3-70b-versatile",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{MaxTokens: &maxTokens, Temperature: &temperature, TopP: &topP, Stop: &stop, Effort: &effort},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Fatalf("Answer=%v, want pong", resp.Answer)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "thinking" {
		t.Fatalf("ReasonContent=%v, want thinking", resp.ReasonContent)
	}
}

func TestGroqChatRequiresModelName(t *testing.T) {
	apiKey := "test-key"
	_, err := newGroqForTest("http://unused").ChatWithMessages("", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("expected model-name error, got %v", err)
	}
}

func TestGroqChatRequiresMessages(t *testing.T) {
	apiKey := "test-key"
	_, err := newGroqForTest("http://unused").ChatWithMessages("llama-3.3-70b-versatile", nil, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("expected messages-empty error, got %v", err)
	}
}

func TestGroqChatRejectsHTTPError(t *testing.T) {
	srv := newGroqServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"unauthorized"}}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newGroqForTest(srv.URL).ChatWithMessages(
		"llama-3.3-70b-versatile",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 propagated, got %v", err)
	}
}

func TestGroqStreamHappyPath(t *testing.T) {
	srv := newGroqServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if body["stream"] != true {
			t.Errorf("stream=%v want true", body["stream"])
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("Accept=%q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"reasoning_content":"think ","reasoning":"fallback "}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":"Hello"}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n",
		)
	})
	defer srv.Close()

	apiKey := "test-key"
	var content []string
	var reasoning []string
	var sawDone bool
	err := newGroqForTest(srv.URL).ChatStreamlyWithSender(
		"llama-3.3-70b-versatile",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		func(c *string, r *string) error {
			if c != nil {
				if *c == "[DONE]" {
					sawDone = true
					return nil
				}
				content = append(content, *c)
			}
			if r != nil {
				reasoning = append(reasoning, *r)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if strings.Join(content, "") != "Hello world" {
		t.Errorf("content=%q", strings.Join(content, ""))
	}
	if strings.Join(reasoning, "") != "think " {
		t.Errorf("reasoning=%q", strings.Join(reasoning, ""))
	}
	if !sawDone {
		t.Error("expected [DONE] sentinel")
	}
}

func TestGroqStreamRejectsExplicitFalse(t *testing.T) {
	apiKey := "test-key"
	stream := false
	err := newGroqForTest("http://unused").ChatStreamlyWithSender(
		"llama-3.3-70b-versatile",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Errorf("expected stream-true guard, got %v", err)
	}
}

func TestGroqStreamRequiresSender(t *testing.T) {
	apiKey := "test-key"
	err := newGroqForTest("http://unused").ChatStreamlyWithSender(
		"llama-3.3-70b-versatile",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("expected sender-required error, got %v", err)
	}
}

func TestGroqStreamRejectsUnterminatedStream(t *testing.T) {
	srv := newGroqServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"partial"}}]}`+"\n")
	})
	defer srv.Close()

	apiKey := "test-key"
	err := newGroqForTest(srv.URL).ChatStreamlyWithSender(
		"llama-3.3-70b-versatile",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "stream ended before") {
		t.Errorf("expected unterminated-stream error, got %v", err)
	}
}

func TestGroqListModelsAndCheckConnection(t *testing.T) {
	srv := newGroqServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s", r.Method)
		}
		if r.URL.Path != "/models" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "llama-3.3-70b-versatile"},
				{"id": "openai/gpt-oss-120b"},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := newGroqForTest(srv.URL)
	models, err := model.ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if strings.Join(models, ",") != "llama-3.3-70b-versatile,openai/gpt-oss-120b" {
		t.Errorf("models=%v", models)
	}
	if err := model.CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestGroqBaseURLTrimsTrailingSlash(t *testing.T) {
	srv := newGroqServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{"content": "ok"},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newGroqForTest(srv.URL+"/").ChatWithMessages(
		"llama-3.3-70b-versatile",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
}

func TestGroqUsesEmptyRegionCustomBaseURL(t *testing.T) {
	srv := newGroqServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{"content": "ok"},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	region := ""
	model := NewGroqModel(
		map[string]string{"": srv.URL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
	_, err := model.ChatWithMessages(
		"llama-3.3-70b-versatile",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey, Region: &region},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
}

func TestGroqUnsupportedMethods(t *testing.T) {
	m := newGroqForTest("http://unused")
	if _, err := m.Embed(nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Embed error=%v", err)
	}
	if _, err := m.Rerank(nil, "", nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank error=%v", err)
	}
	if _, err := m.Balance(nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance error=%v", err)
	}
	if _, err := m.ParseFile(nil, nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ParseFile error=%v", err)
	}
	if _, err := m.ListTasks(nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ListTasks error=%v", err)
	}
}
