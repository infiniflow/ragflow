//go:build integration

package api

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"maps"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRESTOpenAIValidation(t *testing.T) {
	id := chatID(t, createRESTChat(t, nil))
	valid := apiObject{"model": "model", "messages": []any{apiObject{"role": "user", "content": "hello"}}}
	for _, tc := range []struct {
		name    string
		change  apiObject
		code    int
		message string
	}{
		{"null_messages", apiObject{"messages": nil}, 102, "You have to provide messages."},
		{"empty_messages", apiObject{"messages": []any{}}, 102, "You have to provide messages."},
		{"assistant_last", apiObject{"messages": []any{apiObject{"role": "assistant", "content": "hello"}}}, 102, "The last content of this conversation is not from user."},
		{"extra_body", apiObject{"extra_body": "invalid"}, 101, "extra_body must be an object."},
		{"reference_metadata", apiObject{"extra_body": apiObject{"reference_metadata": "invalid"}}, 102, "reference_metadata must be an object."},
		{"fields_string", apiObject{"extra_body": apiObject{"reference_metadata": apiObject{"fields": "author"}}}, 101, "reference_metadata.fields must be an array."},
		{"fields_element", apiObject{"extra_body": apiObject{"reference_metadata": apiObject{"fields": []any{"author", 1}}}}, 101, "reference_metadata.fields must be an array."},
		{"metadata_condition", apiObject{"extra_body": apiObject{"metadata_condition": "invalid"}}, 101, "metadata_condition must be an object."},
		{"content_type", apiObject{"messages": []any{apiObject{"role": "user", "content": 42}}}, 102, "messages[].content must be a string or an array of content parts."},
		{"zero_max_tokens", apiObject{"max_tokens": 0}, 101, "`max_tokens` must be greater than 0."},
		{"negative_max_tokens", apiObject{"max_tokens": -1}, 101, "`max_tokens` must be greater than 0."},
		{"stream_type", apiObject{"stream": "true"}, 101, "cannot unmarshal"},
		{"messages_type", apiObject{"messages": "hello"}, 101, "cannot unmarshal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := maps.Clone(valid)
			maps.Copy(body, tc.change)
			result := restCall(t, TestConfig, http.MethodPost, "/openai/"+id+"/chat/completions", body, nil)
			restCode(t, result, 200, tc.code)
			message, ok := result.JSON["message"].(string)
			if !ok || !strings.Contains(message, tc.message) {
				t.Fatalf("validation message=%q want containing %q", message, tc.message)
			}
		})
	}
	restCode(t, restCall(t, TestConfig, http.MethodPost, "/openai/"+id+"/chat/completions", apiObject{"model": "model"}, nil), 200, 102)
	for _, missing := range []string{"invalid_chat_id", uniqueName("missing_chat")} {
		restCode(t, restCall(t, TestConfig, http.MethodPost, "/openai/"+missing+"/chat/completions", valid, nil), 200, 404)
	}
	for _, key := range []string{"", "invalid-api-key"} {
		c := *TestConfig
		c.APIKey = key
		restCode(t, restCall(t, &c, http.MethodPost, "/openai/"+id+"/chat/completions", valid, nil), 401, 401)
	}
}

func TestRESTOpenAICompletions(t *testing.T) {
	chat := createRESTChat(t, nil)
	model, ok := chat["llm_id"].(string)
	if !ok || model == "" {
		t.Skip("tenant has no configured default chat model")
	}
	id := chatID(t, chat)
	for _, tc := range []struct {
		name      string
		options   apiObject
		reference bool
	}{
		{"nonstream", apiObject{"stream": false}, false},
		{"default_nonstream", nil, false},
		{"reference_metadata", apiObject{"stream": false, "extra_body": apiObject{"reference": true, "reference_metadata": apiObject{"include": true, "fields": []string{"author", "year"}}}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := apiObject{"model": "model", "messages": []any{apiObject{"role": "user", "content": "Say hello briefly."}}, "max_tokens": 64}
			maps.Copy(body, tc.options)
			ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
			defer cancel()
			result := restCallContext(t, ctx, TestConfig, http.MethodPost, "/openai/"+id+"/chat/completions", body, nil)
			if result.Status != 200 || !strings.Contains(result.Header.Get("Content-Type"), "application/json") {
				t.Fatal("expected JSON completion (see diagnostic)")
			}
			assertOpenAIEnvelope(t, result.JSON, model, "chat.completion")
			choices := array(t, result.JSON["choices"])
			if len(choices) != 1 {
				t.Fatalf("choices count=%d want 1", len(choices))
			}
			choice := object(t, choices[0])
			message := object(t, choice["message"])
			if choice["index"] != float64(0) || choice["finish_reason"] != "stop" || message["role"] != "assistant" {
				t.Fatal("invalid completion choice")
			}
			assertOpenAIContent(t, message["content"])
			assertRESTAgentUsage(t, result.JSON["usage"])
			if tc.reference {
				if len(array(t, message["reference"])) != 0 {
					t.Fatal("dataset-free chat returned references")
				}
			} else if _, exists := message["reference"]; exists {
				t.Fatal("unrequested reference field returned")
			}
		})
	}
	for _, reference := range []bool{false, true} {
		name := "stream"
		if reference {
			name = "stream_reference"
		}
		t.Run(name, func(t *testing.T) {
			body := apiObject{"model": "model", "messages": []any{apiObject{"role": "user", "content": "Say hello briefly."}}, "stream": true, "max_tokens": 64, "extra_body": apiObject{"reference": reference}}
			ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
			defer cancel()
			result := restCallContext(t, ctx, TestConfig, http.MethodPost, "/openai/"+id+"/chat/completions", body, nil)
			if result.Status != 200 || !strings.Contains(result.Header.Get("Content-Type"), "text/event-stream") {
				t.Fatal("expected SSE completion (see diagnostic)")
			}
			events, err := parseSSE(bytes.NewReader(result.Body))
			if err != nil {
				t.Fatal(err)
			}
			if len(events) < 2 || events[len(events)-1].Data != "[DONE]" {
				t.Fatal("stream must terminate with [DONE] after JSON events")
			}
			var content strings.Builder
			var streamID string
			for index, event := range events[:len(events)-1] {
				var payload apiObject
				if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
					t.Fatalf("event %d: %v", index, err)
				}
				assertOpenAIEnvelope(t, payload, model, "chat.completion.chunk")
				if index == 0 {
					streamID = payload["id"].(string)
				} else if payload["id"] != streamID {
					t.Fatalf("completion id changed at event %d", index)
				}
				choices := array(t, payload["choices"])
				if len(choices) != 1 {
					t.Fatalf("event %d has %d choices", index, len(choices))
				}
				choice := object(t, choices[0])
				delta := object(t, choice["delta"])
				if choice["index"] != float64(0) || delta["role"] != "assistant" {
					t.Fatalf("invalid choice at event %d", index)
				}
				if value, ok := delta["content"].(string); ok {
					content.WriteString(value)
				} else if delta["content"] != nil {
					t.Fatalf("invalid delta content at event %d", index)
				}
				if value := delta["reasoning_content"]; value != nil {
					if _, ok := value.(string); !ok {
						t.Fatalf("invalid reasoning content at event %d", index)
					}
				}
				final := index == len(events)-2
				if final {
					if choice["finish_reason"] != "stop" {
						t.Fatal("last JSON event must finish with stop")
					}
					assertRESTAgentUsage(t, payload["usage"])
					if reference {
						if len(array(t, delta["reference"])) != 0 {
							t.Fatal("dataset-free stream returned references")
						}
						assertOpenAIContent(t, delta["final_content"])
						if delta["final_content"] != strings.TrimSpace(content.String()) {
							t.Fatal("final content differs from assembled stream")
						}
					} else if _, exists := delta["reference"]; exists {
						t.Fatal("unrequested streaming reference field returned")
					}
				} else if choice["finish_reason"] != nil || payload["usage"] != nil {
					t.Fatalf("premature terminal fields at event %d", index)
				}
			}
			assertOpenAIContent(t, content.String())
		})
	}
}

func assertOpenAIEnvelope(t *testing.T, payload apiObject, model, kind string) {
	t.Helper()
	identifier, ok := payload["id"].(string)
	if !ok || strings.TrimSpace(identifier) == "" || payload["object"] != kind || payload["model"] != model {
		t.Fatalf("invalid completion identity or type (see diagnostic)")
	}
	created, ok := payload["created"].(float64)
	if !ok || created <= 0 || math.Trunc(created) != created {
		t.Fatalf("invalid creation timestamp=%v", payload["created"])
	}
}

func assertOpenAIContent(t *testing.T, value any) {
	t.Helper()
	content, ok := value.(string)
	if !ok || strings.TrimSpace(content) == "" || strings.Contains(content, "**ERROR**") {
		t.Fatal("completion is empty or contains a model error (see diagnostic)")
	}
}
