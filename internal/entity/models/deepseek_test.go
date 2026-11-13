package models

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newDeepSeekForTest(baseURL string) *DeepSeekModel {
	return NewDeepSeekModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:   "chat/completions",
			Models: "models",
		},
	)
}

func TestDeepSeekChatWithMessagesSupportsToolCalls(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	var requestBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s, want /chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q, want Bearer test-key", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": nil,
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call-1",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "search_my_dateset",
									"arguments": `{"query":"marigold"}`,
								},
							},
						},
					},
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     1,
				"completion_tokens": 2,
				"total_tokens":      3,
			},
		})
	}))
	defer srv.Close()

	apiKey := "test-key"
	toolChoice := "auto"
	resp, err := newDeepSeekForTest(srv.URL).ChatWithMessages(
		ctx,
		"deepseek-chat",
		[]Message{
			{Role: "user", Content: "what is marigold"},
		},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{
			Tools: []map[string]interface{}{
				{
					"type": "function",
					"function": map[string]interface{}{
						"name":        "search_my_dateset",
						"description": "Search datasets.",
					},
				},
			},
			ToolChoice: &toolChoice,
		},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if requestBody["tool_choice"] != "auto" {
		t.Fatalf("tool_choice=%#v, want auto", requestBody["tool_choice"])
	}
	if _, ok := requestBody["tools"].([]interface{}); !ok {
		t.Fatalf("tools missing or wrong type: %#v", requestBody["tools"])
	}
	if resp.Answer == nil || *resp.Answer != "" {
		t.Fatalf("Answer=%#v, want empty string for tool-call response", resp.Answer)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len=%d, want 1", len(resp.ToolCalls))
	}
	fn, _ := resp.ToolCalls[0]["function"].(map[string]interface{})
	if fn["name"] != "search_my_dateset" {
		t.Fatalf("tool call function=%#v, want search_my_dateset", fn)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 3 {
		t.Fatalf("Usage=%#v, want total tokens 3", resp.Usage)
	}
}

func TestDeepSeekChatWithMessagesForwardsToolHistory(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	var requestBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"role": "assistant", "content": "answer"}},
			},
		})
	}))
	defer srv.Close()

	apiKey := "test-key"
	_, err := newDeepSeekForTest(srv.URL).ChatWithMessages(
		ctx,
		"deepseek-chat",
		[]Message{
			{
				Role:    "assistant",
				Content: nil,
				ToolCalls: []map[string]interface{}{
					{
						"id":   "call-1",
						"type": "function",
						"function": map[string]interface{}{
							"name":      "search_my_dateset",
							"arguments": `{"query":"marigold"}`,
						},
					},
				},
			},
			{Role: "tool", ToolCallID: "call-1", Content: `{"formalized_content":"marigold"}`},
		},
		&APIConfig{ApiKey: &apiKey},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	messages, ok := requestBody["messages"].([]interface{})
	if !ok || len(messages) != 2 {
		t.Fatalf("messages=%#v, want 2 messages", requestBody["messages"])
	}
	assistant, _ := messages[0].(map[string]interface{})
	if _, ok := assistant["tool_calls"].([]interface{}); !ok {
		t.Fatalf("assistant tool_calls missing: %#v", assistant)
	}
	toolMsg, _ := messages[1].(map[string]interface{})
	if toolMsg["tool_call_id"] != "call-1" {
		t.Fatalf("tool_call_id=%#v, want call-1", toolMsg["tool_call_id"])
	}
}

func TestDeepSeekThinkingEffortPayload(t *testing.T) {
	thinkingConfig := func(effort string) *ChatConfig {
		return &ChatConfig{Thinking: boolPointer(true), Effort: &effort}
	}
	unsetThinkingConfig := thinkingConfig("low")
	unsetThinkingConfig.Thinking = nil
	disabledConfig := thinkingConfig("unknown")
	disabledConfig.Thinking = boolPointer(false)
	cases := []struct {
		name         string
		config       *ChatConfig
		wantThinking string
		wantEffort   string
		wantError    bool
	}{
		{name: "nil config", wantThinking: "disabled"},
		{name: "thinking unset", config: unsetThinkingConfig, wantThinking: "disabled"},
		{name: "thinking disabled", config: &ChatConfig{Thinking: boolPointer(false)}, wantThinking: "disabled"},
		{name: "disabled ignores effort", config: disabledConfig, wantThinking: "disabled"},
		{name: "effort unset", config: &ChatConfig{Thinking: boolPointer(true)}, wantThinking: "enabled", wantEffort: "high"},
		{name: "default", config: thinkingConfig("default"), wantThinking: "enabled", wantEffort: "high"},
		{name: "none", config: thinkingConfig("none"), wantThinking: "disabled"},
		{name: "minimal", config: thinkingConfig("minimal"), wantThinking: "enabled", wantEffort: "low"},
		{name: "low", config: thinkingConfig("low"), wantThinking: "enabled", wantEffort: "low"},
		{name: "medium", config: thinkingConfig("medium"), wantThinking: "enabled", wantEffort: "high"},
		{name: "high", config: thinkingConfig("high"), wantThinking: "enabled", wantEffort: "high"},
		{name: "xhigh", config: thinkingConfig("xhigh"), wantThinking: "enabled", wantEffort: "high"},
		{name: "max", config: thinkingConfig("max"), wantThinking: "enabled", wantEffort: "max"},
		{name: "ultra", config: thinkingConfig("ultra"), wantThinking: "enabled", wantEffort: "max"},
		{name: "empty effort", config: thinkingConfig(""), wantError: true},
		{name: "unknown effort", config: thinkingConfig("unknown"), wantError: true},
	}

	for _, stream := range []bool{false, true} {
		for _, testCase := range cases {
			t.Run(fmt.Sprintf("stream=%t/%s", stream, testCase.name), func(t *testing.T) {
				requests := make(chan map[string]interface{}, 1)
				server := newThinkingPayloadServer(t, requests)
				defer server.Close()
				driver := newDeepSeekForTest(server.URL)
				driver.baseModel.httpClient = server.Client()

				apiKey := "test-key"
				apiConfig := &APIConfig{ApiKey: &apiKey}
				messages := []Message{{Role: "user", Content: "hello"}}
				var err error
				if stream {
					err = driver.ChatStreamlyWithSender(t.Context(), "deepseek-v4-pro", messages, apiConfig, testCase.config, nil, func(*string, *string) error { return nil })
				} else {
					var response *ChatResponse
					response, err = driver.ChatWithMessages(t.Context(), "deepseek-v4-pro", messages, apiConfig, testCase.config, nil)
					if testCase.wantError && response != nil {
						t.Errorf("response=%+v, want nil for invalid effort", response)
					}
				}

				if testCase.wantError {
					if err == nil || !strings.Contains(err.Error(), "invalid effort level") {
						t.Errorf("error=%v, want invalid effort level", err)
					}
					if len(requests) != 0 {
						t.Error("invalid effort sent an HTTP request")
					}
					return
				}
				if err != nil {
					t.Fatalf("chat: %v", err)
				}

				select {
				case body := <-requests:
					thinking, ok := body["thinking"].(map[string]interface{})
					if !ok || thinking["type"] != testCase.wantThinking {
						t.Errorf("thinking=%v, want type=%s", body["thinking"], testCase.wantThinking)
					}
					if testCase.wantEffort == "" {
						if _, ok := body["reasoning_effort"]; ok {
							t.Errorf("reasoning_effort=%v, want omitted", body["reasoning_effort"])
						}
					} else if body["reasoning_effort"] != testCase.wantEffort {
						t.Errorf("reasoning_effort=%v, want %s", body["reasoning_effort"], testCase.wantEffort)
					}
					if body["stream"] != stream {
						t.Errorf("stream=%v, want %t", body["stream"], stream)
					}
					if stream {
						options, ok := body["stream_options"].(map[string]interface{})
						if !ok || options["include_usage"] != true {
							t.Errorf("stream_options=%v, want include_usage=true", body["stream_options"])
						}
					}
				default:
					t.Fatal("chat did not send an HTTP request")
				}
			})
		}
	}
}
