package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newAnthropicServer(t *testing.T, expectedPath string, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectedPath {
			t.Errorf("expected path=%s, got %s", expectedPath, r.URL.Path)
			return
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Errorf("expected x-api-key=test-key, got %q", got)
			return
		}
		if got := r.Header.Get("anthropic-version"); got != anthropicVersion {
			t.Errorf("expected anthropic-version=%s, got %q", anthropicVersion, got)
			return
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("expected Content-Type to start with application/json, got %q", got)
			return
		}
		if r.Method == http.MethodPost {
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

func newAnthropicForTest(baseURL string) *AnthropicModel {
	return NewAnthropicModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "v1/messages", Models: "v1/models"},
	)
}

func TestAnthropicName(t *testing.T) {
	if got := newAnthropicForTest("http://unused").Name(); got != "anthropic" {
		t.Errorf("Name()=%q, want anthropic", got)
	}
}

func TestAnthropicChatHappyPath(t *testing.T) {
	srv := newAnthropicServer(t, "/v1/messages", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "claude-sonnet-4-5-20250929" {
			t.Errorf("model=%v", body["model"])
		}
		if body["max_tokens"] != float64(1024) {
			t.Errorf("max_tokens=%v want 1024", body["max_tokens"])
		}
		msgs, ok := body["messages"].([]interface{})
		if !ok || len(msgs) != 1 {
			t.Errorf("messages=%v, want one message", body["messages"])
			return
		}
		msg, ok := msgs[0].(map[string]interface{})
		if !ok || msg["role"] != "user" || msg["content"] != "ping" {
			t.Errorf("message=%v, want user ping", msgs[0])
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "thinking", "thinking": "reasoning"},
				{"type": "text", "text": "pong"},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newAnthropicForTest(srv.URL).ChatWithMessages(
		"claude-sonnet-4-5-20250929",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Errorf("answer=%v, want pong", resp.Answer)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "reasoning" {
		t.Errorf("reason=%v, want reasoning", resp.ReasonContent)
	}
}

func TestAnthropicChatMapsSystemConfigAndImages(t *testing.T) {
	srv := newAnthropicServer(t, "/v1/messages", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["system"] != "be concise" {
			t.Errorf("system=%v, want be concise", body["system"])
		}
		if body["max_tokens"] != float64(64) {
			t.Errorf("max_tokens=%v want 64", body["max_tokens"])
		}
		if body["temperature"] != 0.25 {
			t.Errorf("temperature=%v want 0.25", body["temperature"])
		}
		if body["top_p"] != 0.8 {
			t.Errorf("top_p=%v want 0.8", body["top_p"])
		}
		stop, ok := body["stop_sequences"].([]interface{})
		if !ok || len(stop) != 1 || stop[0] != "END" {
			t.Errorf("stop_sequences=%v want [END]", body["stop_sequences"])
		}
		msgs, ok := body["messages"].([]interface{})
		if !ok || len(msgs) == 0 {
			t.Errorf("messages=%v, want non-empty array", body["messages"])
			return
		}
		first, ok := msgs[0].(map[string]interface{})
		if !ok {
			t.Errorf("first message=%v, want object", msgs[0])
			return
		}
		content, ok := first["content"].([]interface{})
		if !ok || len(content) < 2 {
			t.Errorf("content=%v, want at least 2 blocks", first["content"])
			return
		}
		image, ok := content[1].(map[string]interface{})
		if !ok {
			t.Errorf("image block=%v, want object", content[1])
			return
		}
		source, ok := image["source"].(map[string]interface{})
		if !ok {
			t.Errorf("image source=%v, want object", image["source"])
			return
		}
		if image["type"] != "image" || source["type"] != "url" || source["url"] != "https://example.com/cat.png" {
			t.Errorf("image block=%v", image)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{{"type": "text", "text": "ok"}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	maxTokens := 64
	temperature := 0.25
	topP := 0.8
	stop := []string{"END"}
	_, err := newAnthropicForTest(srv.URL).ChatWithMessages(
		"claude-opus-4-5-20251101",
		[]Message{
			{Role: "system", Content: "be concise"},
			{Role: "user", Content: []interface{}{
				map[string]interface{}{"type": "text", "text": "what is this?"},
				map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "https://example.com/cat.png"}},
			}},
		},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{MaxTokens: &maxTokens, Temperature: &temperature, TopP: &topP, Stop: &stop},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
}

func TestAnthropicChatMapsDataImageURL(t *testing.T) {
	srv := newAnthropicServer(t, "/v1/messages", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		msgs, ok := body["messages"].([]interface{})
		if !ok || len(msgs) == 0 {
			t.Errorf("messages=%v, want non-empty array", body["messages"])
			return
		}
		first, ok := msgs[0].(map[string]interface{})
		if !ok {
			t.Errorf("first message=%v, want object", msgs[0])
			return
		}
		content, ok := first["content"].([]interface{})
		if !ok || len(content) == 0 {
			t.Errorf("content=%v, want non-empty array", first["content"])
			return
		}
		image, ok := content[0].(map[string]interface{})
		if !ok {
			t.Errorf("image block=%v, want object", content[0])
			return
		}
		source, ok := image["source"].(map[string]interface{})
		if !ok {
			t.Errorf("source=%v, want object", image["source"])
			return
		}
		if source["type"] != "base64" || source["media_type"] != "image/png" || source["data"] != "aGVsbG8=" {
			t.Errorf("source=%v", source)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{{"type": "text", "text": "ok"}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newAnthropicForTest(srv.URL).ChatWithMessages(
		"claude-sonnet-4-5-20250929",
		[]Message{{Role: "user", Content: []interface{}{
			map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "data:image/png;base64,aGVsbG8="}},
		}}},
		&APIConfig{ApiKey: &apiKey},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
}

func TestAnthropicChatValidationErrors(t *testing.T) {
	m := newAnthropicForTest("http://unused")
	apiKey := "test-key"
	if _, err := m.ChatWithMessages("claude", []Message{{Role: "user", Content: "x"}}, nil, nil); err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("nil api config: got %v", err)
	}
	if _, err := m.ChatWithMessages("claude", nil, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("empty messages: got %v", err)
	}
	if _, err := m.ChatWithMessages("claude", []Message{{Role: "tool", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "unsupported message role") {
		t.Errorf("bad role: got %v", err)
	}
	if _, err := m.ChatWithMessages("claude", []Message{{Role: "user", Content: []interface{}{map[string]interface{}{"type": "video_url"}}}}, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "unsupported content block type") {
		t.Errorf("bad block: got %v", err)
	}
	if _, err := m.ChatWithMessages("claude", []Message{{Role: "user", Content: []interface{}{map[string]interface{}{"type": "text", "text": 42}}}}, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "invalid text field") {
		t.Errorf("bad text block: got %v", err)
	}
	if _, err := m.ChatWithMessages("claude", []Message{{Role: "user", Content: []interface{}{map[string]interface{}{"type": "image"}}}}, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "image block missing source") {
		t.Errorf("bad image block: got %v", err)
	}
}

func TestAnthropicChatRejectsHTTPError(t *testing.T) {
	srv := newAnthropicServer(t, "/v1/messages", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newAnthropicForTest(srv.URL).ChatWithMessages("claude", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "bad key") {
		t.Errorf("expected provider error, got %v", err)
	}
}

func TestAnthropicChatRejectsMalformedResponse(t *testing.T) {
	srv := newAnthropicServer(t, "/v1/messages", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"content": []map[string]interface{}{{"type": "tool_use"}}})
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newAnthropicForTest(srv.URL).ChatWithMessages("claude", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "no text content") {
		t.Errorf("expected no-text error, got %v", err)
	}
}

func TestAnthropicListModelsAndCheckConnection(t *testing.T) {
	var calls int
	srv := newAnthropicServer(t, "/v1/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		calls++
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "claude-sonnet-4-5-20250929"},
				{"id": "claude-haiku-4-5-20251001"},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	m := newAnthropicForTest(srv.URL)
	models, err := m.ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if strings.Join(models, ",") != "claude-sonnet-4-5-20250929,claude-haiku-4-5-20251001" {
		t.Errorf("models=%v", models)
	}
	if err := m.CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Errorf("CheckConnection: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls=%d, want 2", calls)
	}
}

func TestAnthropicListModelsRejectsProviderError(t *testing.T) {
	srv := newAnthropicServer(t, "/v1/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newAnthropicForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("expected 403 error, got %v", err)
	}
}

func TestAnthropicFactoryRegistration(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("Anthropic", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*AnthropicModel); !ok {
		t.Fatalf("driver type=%T, want *AnthropicModel", driver)
	}
}

func TestAnthropicUnsupportedMethods(t *testing.T) {
	m := newAnthropicForTest("http://unused")
	apiKey := "test-key"
	modelName := "claude"
	checks := []struct {
		name string
		err  error
	}{
		{"stream", m.ChatStreamlyWithSender(modelName, []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, func(*string, *string) error { return nil })},
	}
	for _, check := range checks {
		if check.err == nil || !strings.Contains(check.err.Error(), "no such method") {
			t.Errorf("%s: want no such method, got %v", check.name, check.err)
		}
	}
	if _, err := m.Embed(&modelName, []string{"x"}, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Embed: got %v", err)
	}
	if _, err := m.Rerank(&modelName, "q", []string{"d"}, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank: got %v", err)
	}
	if _, err := m.Balance(&APIConfig{ApiKey: &apiKey}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance: got %v", err)
	}
	if _, err := m.TranscribeAudio(&modelName, &modelName, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudio: got %v", err)
	}
	if err := m.TranscribeAudioWithSender(&modelName, &modelName, &APIConfig{ApiKey: &apiKey}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudioWithSender: got %v", err)
	}
	if _, err := m.AudioSpeech(&modelName, &modelName, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeech: got %v", err)
	}
	if err := m.AudioSpeechWithSender(&modelName, &modelName, &APIConfig{ApiKey: &apiKey}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeechWithSender: got %v", err)
	}
	if _, err := m.OCRFile(&modelName, nil, &modelName, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile: got %v", err)
	}
	if _, err := m.ParseFile(&modelName, nil, &modelName, &APIConfig{ApiKey: &apiKey}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ParseFile: got %v", err)
	}
	if _, err := m.ListTasks(&APIConfig{ApiKey: &apiKey}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ListTasks: got %v", err)
	}
	if _, err := m.ShowTask("task-id", &APIConfig{ApiKey: &apiKey}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ShowTask: got %v", err)
	}
}
