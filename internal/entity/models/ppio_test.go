package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// roundTripperFunc is the package-wide test helper for stubbing
// http.RoundTripper. It lives here (the first provider test to need it) and is
// shared by the other provider tests in this package; do not redeclare it.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func newPPIOServer(t *testing.T, handler func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization=Bearer test-key, got %q", got)
			return
		}
		if got := r.Header.Get("Content-Type"); r.Method != http.MethodGet && !strings.HasPrefix(got, "application/json") {
			t.Errorf("expected Content-Type to start with application/json, got %q", got)
			return
		}

		var body map[string]interface{}
		if r.Method == http.MethodPost {
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

func newPPIOForTest(baseURL string) *PPIOModel {
	return NewPPIOModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestPPIOName(t *testing.T) {
	if got := newPPIOForTest("http://unused").Name(); got != "ppio" {
		t.Errorf("Name()=%q", got)
	}
}

func TestPPIOFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("PPIO", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*PPIOModel); !ok {
		t.Fatalf("driver type=%T, want *PPIOModel", driver)
	}
}

func TestPPIONewModelWithCustomDefaultTransport(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = original
	})

	if model := NewPPIOModel(map[string]string{"default": "http://unused"}, URLSuffix{}); model == nil {
		t.Fatal("NewPPIOModel returned nil")
	}
}

func TestPPIOChatHappyPath(t *testing.T) {
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if body["model"] != "deepseek/deepseek-r1" {
			t.Errorf("model=%v", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v want false", body["stream"])
		}
		if _, ok := body["reasoning_effort"]; ok {
			t.Errorf("reasoning_effort should not be sent: %v", body["reasoning_effort"])
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
			t.Errorf("stop=%#v", body["stop"])
		}
		messages, ok := body["messages"].([]interface{})
		if !ok || len(messages) != 1 {
			t.Fatalf("messages=%#v", body["messages"])
		}
		first, ok := messages[0].(map[string]interface{})
		if !ok {
			t.Fatalf("message type=%T", messages[0])
		}
		if first["role"] != "user" || first["content"] != "ping" {
			t.Errorf("message=%#v", first)
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
	mt := 32
	temp := 0.3
	topP := 0.9
	stop := []string{"END"}
	effort := "high"
	resp, err := newPPIOForTest(srv.URL).ChatWithMessages(
		"deepseek/deepseek-r1",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{MaxTokens: &mt, Temperature: &temp, TopP: &topP, Stop: &stop, Effort: &effort},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if *resp.Answer != "pong" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
	if *resp.ReasonContent != "thinking" {
		t.Errorf("ReasonContent=%q", *resp.ReasonContent)
	}
}

func TestPPIOChatUsesReasoningFallback(t *testing.T) {
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content":   "pong",
					"reasoning": "fallback reasoning",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newPPIOForTest(srv.URL).ChatWithMessages(
		"deepseek/deepseek-r1",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if *resp.ReasonContent != "fallback reasoning" {
		t.Errorf("ReasonContent=%q", *resp.ReasonContent)
	}
}

func TestPPIOChatRequiresModelName(t *testing.T) {
	apiKey := "test-key"
	_, err := newPPIOForTest("http://unused").ChatWithMessages("", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("expected model-name error, got %v", err)
	}
}

func TestPPIOChatRequiresMessages(t *testing.T) {
	apiKey := "test-key"
	_, err := newPPIOForTest("http://unused").ChatWithMessages("deepseek/deepseek-r1", nil, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("expected messages error, got %v", err)
	}
}

func TestPPIOChatSurfacesHTTPError(t *testing.T) {
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newPPIOForTest(srv.URL).ChatWithMessages("deepseek/deepseek-r1", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Errorf("expected HTTP status error, got %v", err)
	}
}

func TestPPIOChatRejectsProviderError(t *testing.T) {
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{"message": "invalid model"},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newPPIOForTest(srv.URL).ChatWithMessages("deepseek/deepseek-r1", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "upstream error") {
		t.Errorf("expected upstream error, got %v", err)
	}
}

func TestPPIOStreamHappyPath(t *testing.T) {
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
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
			`data: {"choices":[{"delta":{"reasoning_content":"think "}}]}`+"\n"+
				`data: {"choices":[{"delta":{"reasoning":"fallback "}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":"Hello"}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n",
		)
	})
	defer srv.Close()

	apiKey := "test-key"
	var content []string
	var reasoning []string
	err := newPPIOForTest(srv.URL).ChatStreamlyWithSender(
		"deepseek/deepseek-r1",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(c *string, r *string) error {
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
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if strings.Join(content, "") != "Hello world[DONE]" {
		t.Errorf("content=%q", strings.Join(content, ""))
	}
	if strings.Join(reasoning, "") != "think fallback " {
		t.Errorf("reasoning=%q", strings.Join(reasoning, ""))
	}
	if len(content) == 0 || content[len(content)-1] != "[DONE]" {
		t.Errorf("final content sentinel missing: %#v", content)
	}
}

func TestPPIOStreamSurfacesHTTPError(t *testing.T) {
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	})
	defer srv.Close()

	apiKey := "test-key"
	err := newPPIOForTest(srv.URL).ChatStreamlyWithSender(
		"deepseek/deepseek-r1",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Errorf("expected HTTP status error, got %v", err)
	}
}

func TestPPIOStreamStopsOnSenderError(t *testing.T) {
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"partial"}}]}`+"\n")
	})
	defer srv.Close()

	apiKey := "test-key"
	err := newPPIOForTest(srv.URL).ChatStreamlyWithSender(
		"deepseek/deepseek-r1",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return io.ErrUnexpectedEOF },
	)
	if err == nil || !strings.Contains(err.Error(), "unexpected EOF") {
		t.Errorf("expected sender error, got %v", err)
	}
}

func TestPPIOStreamRejectsExplicitFalse(t *testing.T) {
	apiKey := "test-key"
	stream := false
	err := newPPIOForTest("http://unused").ChatStreamlyWithSender(
		"deepseek/deepseek-r1",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Errorf("expected stream guard, got %v", err)
	}
}

func TestPPIOStreamRequiresSender(t *testing.T) {
	apiKey := "test-key"
	err := newPPIOForTest("http://unused").ChatStreamlyWithSender(
		"deepseek/deepseek-r1",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("expected sender error, got %v", err)
	}
}

func TestPPIOStreamRequiresTerminalEvent(t *testing.T) {
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"partial"}}]}`+"\n")
	})
	defer srv.Close()

	apiKey := "test-key"
	err := newPPIOForTest(srv.URL).ChatStreamlyWithSender(
		"deepseek/deepseek-r1",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "stream ended before") {
		t.Errorf("expected unterminated stream error, got %v", err)
	}
}

func TestPPIOListModelsAndCheckConnection(t *testing.T) {
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s", r.Method)
		}
		if r.URL.Path != "/models" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "deepseek/deepseek-r1"},
				{"id": "qwen/qwen-2.5-72b-instruct"},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := newPPIOForTest(srv.URL)
	models, err := model.ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if strings.Join(models, ",") != "deepseek/deepseek-r1,qwen/qwen-2.5-72b-instruct" {
		t.Errorf("models=%v", models)
	}
	if err := model.CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestPPIOListModelsRequiresAPIKey(t *testing.T) {
	_, err := newPPIOForTest("http://unused").ListModels(&APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestPPIOListModelsRejectsProviderError(t *testing.T) {
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{"message": "unauthorized"},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newPPIOForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "upstream error") {
		t.Errorf("expected upstream error, got %v", err)
	}
}

func TestPPIOEndpointTrimsTrailingSlash(t *testing.T) {
	model := NewPPIOModel(map[string]string{"default": "https://example.test/base/"}, URLSuffix{Chat: "/chat/completions"})
	apiKey := "test-key"
	endpoint, err := model.endpoint(&APIConfig{ApiKey: &apiKey}, model.URLSuffix.Chat)
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	if endpoint != "https://example.test/base/chat/completions" {
		t.Errorf("endpoint=%q", endpoint)
	}
}

func TestPPIODefaultEndpointUsesPPIOAPI(t *testing.T) {
	model := NewPPIOModel(map[string]string{"default": "https://api.ppio.com/openai/v1"}, URLSuffix{Chat: "chat/completions"})
	apiKey := "test-key"
	endpoint, err := model.endpoint(&APIConfig{ApiKey: &apiKey}, model.URLSuffix.Chat)
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	if endpoint != "https://api.ppio.com/openai/v1/chat/completions" {
		t.Errorf("endpoint=%q", endpoint)
	}
}

func TestPPIOEmptyRegionCustomBaseURL(t *testing.T) {
	model := NewPPIOModel(map[string]string{"": "https://custom.example/openai/v1"}, URLSuffix{Models: "models"})
	apiKey := "test-key"
	region := ""
	endpoint, err := model.endpoint(&APIConfig{ApiKey: &apiKey, Region: &region}, model.URLSuffix.Models)
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	if endpoint != "https://custom.example/openai/v1/models" {
		t.Errorf("endpoint=%q", endpoint)
	}
}

func TestPPIONamedRegionBaseURL(t *testing.T) {
	model := NewPPIOModel(map[string]string{
		"default": "https://api.ppio.com/openai/v1",
		"us":      "https://api.ppinfra.com/v3/openai",
	}, URLSuffix{Chat: "chat/completions"})
	apiKey := "test-key"
	region := "us"
	endpoint, err := model.endpoint(&APIConfig{ApiKey: &apiKey, Region: &region}, model.URLSuffix.Chat)
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	if endpoint != "https://api.ppinfra.com/v3/openai/chat/completions" {
		t.Errorf("endpoint=%q", endpoint)
	}
}

func TestPPIOMissingRegionBaseURL(t *testing.T) {
	model := NewPPIOModel(map[string]string{"default": "https://api.ppinfra.com/v3/openai"}, URLSuffix{Models: "models"})
	apiKey := "test-key"
	region := "missing"
	_, err := model.endpoint(&APIConfig{ApiKey: &apiKey, Region: &region}, model.URLSuffix.Models)
	if err == nil || !strings.Contains(err.Error(), "no base URL configured") {
		t.Errorf("expected base URL error, got %v", err)
	}
}

func TestPPIOUnsupportedMethods(t *testing.T) {
	m := newPPIOForTest("http://unused")
	if _, err := m.Embed(nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Embed error=%v", err)
	}
	if _, err := m.Rerank(nil, "", nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank error=%v", err)
	}
	if _, err := m.Balance(nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance error=%v", err)
	}
	if _, err := m.TranscribeAudio(nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudio error=%v", err)
	}
	if err := m.TranscribeAudioWithSender(nil, nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudioWithSender error=%v", err)
	}
	if _, err := m.AudioSpeech(nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeech error=%v", err)
	}
	if err := m.AudioSpeechWithSender(nil, nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeechWithSender error=%v", err)
	}
	if _, err := m.OCRFile(nil, nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile error=%v", err)
	}
	if _, err := m.ParseFile(nil, nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ParseFile error=%v", err)
	}
	if _, err := m.ListTasks(nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ListTasks error=%v", err)
	}
	if _, err := m.ShowTask("", nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ShowTask error=%v", err)
	}
}
