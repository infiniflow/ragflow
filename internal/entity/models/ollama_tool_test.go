package models

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestOllamaToolRoundTrip(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			withSSRFBypass(t)
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				var body struct {
					Messages []map[string]any `json:"messages"`
					Tools    []any            `json:"tools"`
					Stream   bool             `json:"stream"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if len(body.Tools) != 1 {
					t.Errorf("tools=%v", body.Tools)
				}
				if requests == 1 {
					if body.Stream {
						io.WriteString(w, "{\"message\":{\"thinking\":\"searching\"},\"done\":false}\n")
						io.WriteString(w, "{\"message\":{\"tool_calls\":[{\"function\":{\"name\":\"search\",\"arguments\":{\"query\":\"合同\"}}}]},\"done\":false}\n")
						io.WriteString(w, "{\"message\":{\"tool_calls\":[{\"id\":\"upstream-id\",\"function\":{\"name\":\"search\",\"arguments\":{\"query\":\"质保\"}}}]},\"done\":false}\n{\"done\":true}\n")
					} else {
						io.WriteString(w, `{"message":{"content":"","tool_calls":[{"function":{"name":"search","arguments":{"query":"合同"}}},{"id":"upstream-id","function":{"name":"search","arguments":{"query":"质保"}}}]},"done":true}`)
					}
					return
				}
				if len(body.Messages) != 4 {
					t.Errorf("messages=%v", body.Messages)
					return
				}
				calls := body.Messages[1]["tool_calls"].([]any)
				for i, c := range calls {
					fn := c.(map[string]any)["function"].(map[string]any)
					if _, ok := fn["arguments"].(map[string]any); !ok {
						t.Errorf("arguments must be an object: %v", fn)
					}
					result := body.Messages[i+2]
					if result["tool_name"] != "search" || result["content"] != "合同原文" {
						t.Errorf("tool result=%v", result)
					}
				}
				if body.Stream {
					io.WriteString(w, "{\"message\":{\"content\":\"基于合同原文回答\"},\"done\":true}\n")
				} else {
					io.WriteString(w, `{"message":{"content":"基于合同原文回答"},"done":true}`)
				}
			}))
			defer srv.Close()
			name := "qwen3.6:35b-a3b"
			model := NewEinoChatModel(NewChatModel(newOllamaForChatTest(srv.URL), &name, &APIConfig{}), nil)
			bound, err := model.WithTools([]*schema.ToolInfo{{Name: "search", Desc: "Search contracts"}})
			if err != nil {
				t.Fatal(err)
			}
			invoke := func(messages []*schema.Message) *schema.Message {
				t.Helper()
				if !stream {
					m, err := bound.Generate(t.Context(), messages)
					if err != nil {
						t.Fatal(err)
					}
					return m
				}
				reader, err := bound.Stream(t.Context(), messages)
				if err != nil {
					t.Fatal(err)
				}
				defer reader.Close()
				var parts []*schema.Message
				for {
					m, err := reader.Recv()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					parts = append(parts, m)
				}
				m, err := schema.ConcatMessages(parts)
				if err != nil {
					t.Fatal(err)
				}
				return m
			}
			messages := []*schema.Message{schema.UserMessage("摘录合同")}
			response := invoke(messages)
			if len(response.ToolCalls) != 2 {
				t.Fatalf("tool calls=%+v", response.ToolCalls)
			}
			if response.ToolCalls[0].ID == "" || response.ToolCalls[0].ID == response.ToolCalls[1].ID || response.ToolCalls[1].ID != "upstream-id" {
				t.Fatalf("call IDs=%+v", response.ToolCalls)
			}
			messages = append(messages, response)
			for _, call := range response.ToolCalls {
				var args map[string]any
				if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
					t.Fatal(err)
				}
				messages = append(messages, &schema.Message{Role: schema.Tool, ToolCallID: call.ID, Content: "合同原文"})
			}
			if final := invoke(messages); final.Content != "基于合同原文回答" {
				t.Fatalf("answer=%+v", final)
			}
			if requests != 2 {
				t.Fatalf("requests=%d", requests)
			}
		})
	}
}

func TestOllamaRejectsMalformedToolHistory(t *testing.T) {
	_, err := buildOllamaMessages([]Message{{Role: "assistant", ToolCalls: []map[string]interface{}{{"function": map[string]interface{}{"name": "search", "arguments": "broken"}}}}})
	if err == nil {
		t.Fatal("expected invalid arguments error")
	}
}

func TestOllamaIncompleteStreamDoesNotPublishTools(t *testing.T) {
	cfg := &ChatConfig{}
	err := handleOllamaStreamingResponse(strings.NewReader(`{"message":{"tool_calls":[{"function":{"name":"search","arguments":{}}}]}}`), nil, cfg, func(*string, *string) error { return nil })
	if err == nil || cfg.ToolCallsResult != nil {
		t.Fatalf("err=%v calls=%v", err, cfg.ToolCallsResult)
	}
}

func TestOllamaStreamingPreservesCompleteParallelCalls(t *testing.T) {
	cfg := &ChatConfig{}
	body := `{"message":{"thinking":"searching"},"done":false}
{"message":{"tool_calls":[{"function":{"name":"search","arguments":{"queries":["甲方"]}}}]},"done":false}
{"message":{"tool_calls":[{"id":"second","function":{"name":"search","arguments":{"queries":["质保"]}}}]},"done":false}
{"done":true,"prompt_eval_count":10,"eval_count":5}
`
	var thinking string
	err := handleOllamaStreamingResponse(strings.NewReader(body), nil, cfg, func(content, reason *string) error {
		if reason != nil {
			thinking += *reason
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if thinking != "searching" || cfg.ToolCallsResult == nil || len(*cfg.ToolCallsResult) != 2 {
		t.Fatalf("thinking=%q calls=%v", thinking, cfg.ToolCallsResult)
	}
	calls := toolCallsFromInternal(*cfg.ToolCallsResult)
	if calls[0].ID == "" || calls[1].ID != "second" || calls[0].ID == calls[1].ID {
		t.Fatalf("calls=%+v", calls)
	}
	if calls[0].Function.Arguments != `{"queries":["甲方"]}` || calls[1].Function.Arguments != `{"queries":["质保"]}` {
		t.Fatalf("arguments=%+v", calls)
	}
	if cfg.UsageResult == nil || cfg.UsageResult.TotalTokens != 15 {
		t.Fatalf("usage=%+v", cfg.UsageResult)
	}
}
