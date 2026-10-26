package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
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

func TestAnthropicToolOptionalFields(t *testing.T) {
	for _, tc := range []struct {
		name     string
		function map[string]any
		want     string
		wantErr  bool
	}{
		{"omitted parameters and description", map[string]any{"name": "clock"}, `{"input_schema":{"properties":{},"type":"object"},"name":"clock"}`, false},
		{"string description", map[string]any{"name": "clock", "parameters": map[string]any{"type": "object"}, "description": "Read clock"}, `{"description":"Read clock","input_schema":{"type":"object"},"name":"clock"}`, false},
		{"non-string description", map[string]any{"name": "clock", "parameters": map[string]any{"type": "object"}, "description": 42}, `{"input_schema":{"type":"object"},"name":"clock"}`, false},
		{"null description", map[string]any{"name": "clock", "parameters": map[string]any{"type": "object"}, "description": nil}, `{"input_schema":{"type":"object"},"name":"clock"}`, false},
		{"null parameters", map[string]any{"name": "clock", "parameters": nil}, "", true},
		{"non-object parameters", map[string]any{"name": "clock", "parameters": "invalid"}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := make(map[string]any)
			err := applyAnthropicChatConfig(body, &ChatConfig{Tools: []any{map[string]any{"type": "function", "function": tc.function}}})
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected invalid parameters to be rejected")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tools, ok := body["tools"].([]map[string]any)
			if !ok || len(tools) != 1 {
				t.Fatalf("tools = %#v, want one tool", body["tools"])
			}
			got, err := json.Marshal(tools[0])
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("tool = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestAnthropicChatHappyPath(t *testing.T) {
	withSSRFBypass(t)
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
	ctx := t.Context()

	apiKey := "test-key"
	resp, err := newAnthropicForTest(srv.URL).ChatWithMessages(
		ctx,
		"claude-sonnet-4-5-20250929",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{},
		&common.ModelUsage{},
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
	withSSRFBypass(t)
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
	ctx := t.Context()

	apiKey := "test-key"
	maxTokens := 64
	temperature := 0.25
	topP := 0.8
	stop := []string{"END"}
	_, err := newAnthropicForTest(srv.URL).ChatWithMessages(
		ctx,
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
		&common.ModelUsage{},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
}

func TestAnthropicChatMapsDataImageURL(t *testing.T) {
	withSSRFBypass(t)
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
	ctx := t.Context()

	apiKey := "test-key"
	_, err := newAnthropicForTest(srv.URL).ChatWithMessages(
		ctx,
		"claude-sonnet-4-5-20250929",
		[]Message{{Role: "user", Content: []interface{}{
			map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "data:image/png;base64,aGVsbG8="}},
		}}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{},
		&common.ModelUsage{},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
}

func TestAnthropicChatValidationErrors(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newAnthropicForTest("http://unused")
	apiKey := "test-key"
	if _, err := m.ChatWithMessages(ctx, "claude", []Message{{Role: "user", Content: "x"}}, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("nil api config: got %v", err)
	}
	if _, err := m.ChatWithMessages(ctx, "claude", nil, &APIConfig{ApiKey: &apiKey}, nil, nil); err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("empty messages: got %v", err)
	}
	if _, err := m.ChatWithMessages(ctx, "claude", []Message{{Role: "developer", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, nil); err == nil || !strings.Contains(err.Error(), "unsupported message role") {
		t.Errorf("bad role: got %v", err)
	}
	if _, err := m.ChatWithMessages(ctx, "claude", []Message{{Role: "user", Content: []interface{}{map[string]interface{}{"type": "video_url"}}}}, &APIConfig{ApiKey: &apiKey}, nil, nil); err == nil || !strings.Contains(err.Error(), "unsupported content block type") {
		t.Errorf("bad block: got %v", err)
	}
	if _, err := m.ChatWithMessages(ctx, "claude", []Message{{Role: "user", Content: []interface{}{map[string]interface{}{"type": "text", "text": 42}}}}, &APIConfig{ApiKey: &apiKey}, nil, nil); err == nil || !strings.Contains(err.Error(), "invalid text field") {
		t.Errorf("bad text block: got %v", err)
	}
	if _, err := m.ChatWithMessages(ctx, "claude", []Message{{Role: "user", Content: []interface{}{map[string]interface{}{"type": "image"}}}}, &APIConfig{ApiKey: &apiKey}, nil, nil); err == nil || !strings.Contains(err.Error(), "image block missing source") {
		t.Errorf("bad image block: got %v", err)
	}
}

func TestAnthropicChatRejectsHTTPError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newAnthropicServer(t, "/v1/messages", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newAnthropicForTest(srv.URL).ChatWithMessages(ctx, "claude", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "bad key") {
		t.Errorf("expected provider error, got %v", err)
	}
}

func TestAnthropicChatRejectsMalformedResponse(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newAnthropicServer(t, "/v1/messages", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"content": []map[string]interface{}{{"type": "tool_use"}}})
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newAnthropicForTest(srv.URL).ChatWithMessages(ctx, "claude", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid tool_use") {
		t.Errorf("expected invalid tool_use error, got %v", err)
	}
}

func TestAnthropicChatStreamlyWithSenderHappyPath(t *testing.T) {
	withSSRFBypass(t)
	srv := newAnthropicServer(t, "/v1/messages", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["stream"] != true {
			t.Errorf("stream=%v, want true", body["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`{"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":5,"output_tokens":0}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"pondering"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"pong"}}`,
			`{"type":"content_block_stop","index":1}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
			`{"type":"message_stop"}`,
		}
		for _, event := range events {
			_, _ = w.Write([]byte("data: " + event + "\n\n"))
		}
	})
	defer srv.Close()
	ctx := t.Context()

	apiKey := "test-key"
	var answer, reasoning strings.Builder
	sawDone := false
	modelUsage := &common.ModelUsage{}
	err := newAnthropicForTest(srv.URL).ChatStreamlyWithSender(
		ctx,
		"claude-sonnet-4-5-20250929",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{},
		modelUsage,
		func(text, reason *string) error {
			if text != nil {
				if *text == "[DONE]" {
					sawDone = true
				} else {
					answer.WriteString(*text)
				}
			}
			if reason != nil {
				reasoning.WriteString(*reason)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if answer.String() != "pong" {
		t.Errorf("answer=%q, want pong", answer.String())
	}
	if reasoning.String() != "pondering" {
		t.Errorf("reasoning=%q, want pondering", reasoning.String())
	}
	if !sawDone {
		t.Error("expected terminal [DONE] sender call")
	}
	if modelUsage.InputTokens != 5 {
		t.Errorf("InputTokens=%d, want 5", modelUsage.InputTokens)
	}
	if modelUsage.OutputTokens != 2 {
		t.Errorf("OutputTokens=%d, want 2", modelUsage.OutputTokens)
	}
	if modelUsage.TotalTokens != 7 {
		t.Errorf("TotalTokens=%d, want 7", modelUsage.TotalTokens)
	}
}

func TestAnthropicStreamToolCalls(t *testing.T) {
	withSSRFBypass(t)
	for _, tc := range []struct {
		name   string
		events []string
		want   []map[string]interface{}
	}{
		{
			name: "fragmented arguments and multiple tools",
			events: []string{
				`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call_1","name":"weather","input":{}}}`,
				`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"city\":\"San "}}`,
				`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"Francisco\",\"days\":2}"}}`,
				`{"type":"content_block_stop","index":1}`,
				`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"call_2","name":"clock","input":{}}}`,
				`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"zone\":\"UTC\"}"}}`,
				`{"type":"content_block_stop","index":2}`,
			},
			want: []map[string]interface{}{
				{"id": "call_1", "type": "function", "function": map[string]interface{}{"name": "weather", "arguments": `{"city":"San Francisco","days":2}`}},
				{"id": "call_2", "type": "function", "function": map[string]interface{}{"name": "clock", "arguments": `{"zone":"UTC"}`}},
			},
		},
		{
			name: "empty arguments",
			events: []string{
				`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call_1","name":"clock","input":{}}}`,
				`{"type":"content_block_stop","index":1}`,
			},
			want: []map[string]interface{}{
				{"id": "call_1", "type": "function", "function": map[string]interface{}{"name": "clock", "arguments": `{}`}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newAnthropicServer(t, "/v1/messages", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
				w.Header().Set("Content-Type", "text/event-stream")
				for _, event := range append(tc.events, `{"type":"message_delta","delta":{"stop_reason":"tool_use"}}`, `{"type":"message_stop"}`) {
					_, _ = io.WriteString(w, "data: "+event+"\n\n")
				}
			})
			defer srv.Close()
			apiKey := "test-key"
			config := &ChatConfig{}
			sawDone := false
			err := newAnthropicForTest(srv.URL).ChatStreamlyWithSender(t.Context(), "claude", []Message{{Role: "user", Content: "use tools"}}, &APIConfig{ApiKey: &apiKey}, config, nil, func(text, _ *string) error {
				if text != nil && *text == "[DONE]" {
					sawDone = true
					if config.ToolCallsResult == nil {
						t.Error("tool calls missing when stream completes")
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if !sawDone || config.ToolCallsResult == nil {
				t.Fatal("stream did not complete with tool calls")
			}
			got, _ := json.Marshal(*config.ToolCallsResult)
			want, _ := json.Marshal(tc.want)
			if string(got) != string(want) {
				t.Errorf("tool calls = %s, want %s", got, want)
			}
		})
	}
}

func TestAnthropicStreamRejectsInvalidToolInput(t *testing.T) {
	withSSRFBypass(t)
	for _, tc := range []struct {
		name    string
		partial string
		stop    bool
	}{
		{"malformed JSON", `{`, true},
		{"non-object JSON", `[]`, true},
		{"null JSON", `null`, true},
		{"unfinished tool block", `{}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newAnthropicServer(t, "/v1/messages", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: "+`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_1","name":"weather","input":{}}}`+"\n\n")
				fragment, _ := json.Marshal(map[string]interface{}{"type": "content_block_delta", "index": 0, "delta": map[string]interface{}{"type": "input_json_delta", "partial_json": tc.partial}})
				_, _ = io.WriteString(w, "data: "+string(fragment)+"\n\n")
				if tc.stop {
					_, _ = io.WriteString(w, "data: "+`{"type":"content_block_stop","index":0}`+"\n\n")
				}
				_, _ = io.WriteString(w, "data: "+`{"type":"message_stop"}`+"\n\n")
			})
			defer srv.Close()
			apiKey := "test-key"
			config := &ChatConfig{}
			sawDone := false
			err := newAnthropicForTest(srv.URL).ChatStreamlyWithSender(t.Context(), "claude", []Message{{Role: "user", Content: "use tools"}}, &APIConfig{ApiKey: &apiKey}, config, nil, func(text, _ *string) error {
				sawDone = text != nil && *text == "[DONE]"
				return nil
			})
			if err == nil || sawDone || config.ToolCallsResult != nil {
				t.Fatalf("invalid tool input: err=%v, done=%v, calls=%v", err, sawDone, config.ToolCallsResult)
			}
		})
	}
}

func TestAnthropicChatStreamlyWithSenderRejectsHTTPError(t *testing.T) {
	withSSRFBypass(t)
	srv := newAnthropicServer(t, "/v1/messages", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	})
	defer srv.Close()
	ctx := t.Context()

	apiKey := "test-key"
	err := newAnthropicForTest(srv.URL).ChatStreamlyWithSender(
		ctx,
		"claude",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		nil,
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "bad key") {
		t.Errorf("expected provider error, got %v", err)
	}
}

func TestAnthropicChatStreamlyWithSenderRejectsStreamError(t *testing.T) {
	withSSRFBypass(t)
	srv := newAnthropicServer(t, "/v1/messages", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"type":"error","error":{"message":"overloaded"}}` + "\n\n"))
	})
	defer srv.Close()
	ctx := t.Context()

	apiKey := "test-key"
	err := newAnthropicForTest(srv.URL).ChatStreamlyWithSender(
		ctx,
		"claude",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		nil,
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "overloaded") {
		t.Errorf("expected stream error, got %v", err)
	}
}

func TestAnthropicListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
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
	models, err := m.ListModels(ctx, &APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(models, ",") != "claude-sonnet-4-5-20250929,claude-haiku-4-5-20251001" {
		t.Errorf("models=%v", models)
	}
	if err = m.CheckConnection(ctx, &APIConfig{ApiKey: &apiKey}); err != nil {
		t.Errorf("CheckConnection: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls=%d, want 2", calls)
	}
}

func TestAnthropicListModelsRejectsProviderError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newAnthropicServer(t, "/v1/models", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newAnthropicForTest(srv.URL).ListModels(ctx, &APIConfig{ApiKey: &apiKey})
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
	withSSRFBypass(t)
	ctx := t.Context()
	m := newAnthropicForTest("http://unused")
	apiKey := "test-key"
	modelName := "claude"
	if _, err := m.Embed(ctx, &modelName, EmbedRequest{Texts: []string{"x"}}, &APIConfig{ApiKey: &apiKey}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Embed: got %v", err)
	}
	if _, err := m.Rerank(ctx, &modelName, RerankRequest{Query: "q", Documents: []string{"d"}}, &APIConfig{ApiKey: &apiKey}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank: got %v", err)
	}
	if _, err := m.Balance(ctx, &APIConfig{ApiKey: &apiKey}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance: got %v", err)
	}
	if _, err := m.TranscribeAudio(ctx, &modelName, &modelName, &APIConfig{ApiKey: &apiKey}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudio: got %v", err)
	}
	if err := m.TranscribeAudioWithSender(ctx, &modelName, &modelName, &APIConfig{ApiKey: &apiKey}, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudioWithSender: got %v", err)
	}
	if _, err := m.AudioSpeech(ctx, &modelName, &modelName, &APIConfig{ApiKey: &apiKey}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeech: got %v", err)
	}
	if err := m.AudioSpeechWithSender(ctx, &modelName, &modelName, &APIConfig{ApiKey: &apiKey}, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeechWithSender: got %v", err)
	}
	if _, err := m.OCRFile(ctx, &modelName, nil, &modelName, &APIConfig{ApiKey: &apiKey}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile: got %v", err)
	}
	if _, err := m.ParseFile(ctx, &modelName, nil, &modelName, &APIConfig{ApiKey: &apiKey}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ParseFile: got %v", err)
	}
	if _, err := m.ListTasks(ctx, &APIConfig{ApiKey: &apiKey}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ListTasks: got %v", err)
	}
	if _, err := m.ShowTask(ctx, "task-id", &APIConfig{ApiKey: &apiKey}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ShowTask: got %v", err)
	}
}
