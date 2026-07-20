package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
