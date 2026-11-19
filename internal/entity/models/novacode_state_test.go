package models

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// Exercise provider state through the Eino adapter and a serialized checkpoint,
// rather than injecting state directly into the provider request builder.
func TestNovaCodeEinoReasoningRoundTrip(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			withSSRFBypass(t)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if r.URL.Path != "/v1/responses" {
					t.Errorf("path = %s", r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer test-secret" {
					t.Error("unexpected authorization")
				}
				if calls.Add(1) == 1 {
					output := []any{
						map[string]any{"type": "reasoning", "id": "rs_1", "encrypted_content": "opaque-cipher", "summary": []any{}},
						map[string]any{"type": "function_call", "id": "fc_1", "call_id": "call_1", "name": "lookup", "arguments": "{}", "status": "completed"},
					}
					response := map[string]any{"id": "resp_1", "status": "completed", "output": output}
					if !stream {
						_ = json.NewEncoder(w).Encode(response)
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					for index, item := range output {
						writeNovaCodeStateEvent(w, map[string]any{"type": "response.output_item.done", "output_index": index, "item": item})
					}
					writeNovaCodeStateEvent(w, map[string]any{"type": "response.completed", "response": response})
					return
				}
				input, ok := body["input"].([]any)
				if !ok || len(input) != 4 {
					t.Errorf("continuation input = %#v; want user + original reasoning + function call + tool output", body["input"])
					w.WriteHeader(400)
					return
				}
				reasoning, _ := input[1].(map[string]any)
				if reasoning["id"] != "rs_1" || reasoning["type"] != "reasoning" || reasoning["encrypted_content"] != "opaque-cipher" {
					t.Errorf("reasoning was not preserved: %#v", reasoning)
				}
				call, _ := input[2].(map[string]any)
				if call["id"] != "fc_1" || call["call_id"] != "call_1" || call["arguments"] != "{}" {
					t.Errorf("original function call was not preserved: %#v", call)
				}
				result, _ := input[3].(map[string]any)
				if result["type"] != "function_call_output" || result["call_id"] != "call_1" || result["output"] != "found" {
					t.Errorf("tool output = %#v", result)
				}
				response := map[string]any{"id": "resp_2", "status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "done"}}}}}
				if !stream {
					_ = json.NewEncoder(w).Encode(response)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				writeNovaCodeStateEvent(w, map[string]any{"type": "response.output_text.delta", "delta": "done"})
				writeNovaCodeStateEvent(w, map[string]any{"type": "response.completed", "response": response})
			}))
			defer server.Close()
			driver := NewNovaCodeModel(map[string]string{"default": server.URL + "/v1"}, URLSuffix{})
			chat := NewEinoChatModel(NewChatModel(driver, new("gpt-6.1-sol"), novaCodeTestConfig(t, "responses", "medium")), nil)
			invoke := func(messages []*schema.Message) *schema.Message {
				t.Helper()
				if !stream {
					response, err := chat.Generate(t.Context(), messages)
					if err != nil {
						t.Fatal(err)
					}
					return response
				}
				reader, err := chat.Stream(t.Context(), messages)
				if err != nil {
					t.Fatal(err)
				}
				defer reader.Close()
				var chunks []*schema.Message
				for {
					chunk, err := reader.Recv()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					chunks = append(chunks, chunk)
				}
				response, err := schema.ConcatMessages(chunks)
				if err != nil {
					t.Fatal(err)
				}
				return response
			}
			history := []*schema.Message{schema.UserMessage("look it up")}
			first := invoke(history)
			if len(first.ToolCalls) != 1 || first.ToolCalls[0].ID != "call_1" {
				t.Fatalf("first tool calls = %#v", first.ToolCalls)
			}
			checkpoint, err := json.Marshal(first)
			if err != nil {
				t.Fatal(err)
			}
			var restored schema.Message
			if err := json.Unmarshal(checkpoint, &restored); err != nil {
				t.Fatal(err)
			}
			history = append(history, &restored, &schema.Message{Role: schema.Tool, ToolCallID: "call_1", Content: "found"})
			second := invoke(history)
			if second.Content != "done" {
				t.Fatalf("answer = %q", second.Content)
			}
			if calls.Load() != 2 {
				t.Fatalf("requests = %d", calls.Load())
			}
		})
	}
}

func writeNovaCodeStateEvent(w http.ResponseWriter, event map[string]any) {
	raw, _ := json.Marshal(event)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
}
