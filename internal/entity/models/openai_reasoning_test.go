package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestOpenAIChatReasoningEffort(t *testing.T) {
	withSSRFBypass(t)
	for _, streaming := range []bool{false, true} {
		path := "Generate"
		if streaming {
			path = "Stream"
		}
		t.Run(path, func(t *testing.T) {
			for _, effort := range []string{"none", "low", "medium", "high", "", "unset", "nil_config"} {
				caseName := effort
				if caseName == "" {
					caseName = "empty"
				}
				t.Run(caseName, func(t *testing.T) {
					var seen map[string]any
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path != "/chat/completions" {
							t.Errorf("path = %q, want /chat/completions", r.URL.Path)
						}
						if err := json.NewDecoder(r.Body).Decode(&seen); err != nil {
							t.Errorf("decode request: %v", err)
						}
						if seen["stream"] != streaming {
							t.Errorf("stream = %v, want %v", seen["stream"], streaming)
						}
						if streaming {
							w.Header().Set("Content-Type", "text/event-stream")
							io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
						} else {
							io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
						}
					}))
					defer srv.Close()

					cfg := &ChatConfig{}
					if effort == "nil_config" {
						cfg = nil
					} else if effort != "unset" {
						cfg.Effort = &effort
					}
					name, key := "gpt-6-luna", "test-key"
					model := NewEinoChatModel(NewChatModel(newOpenAIForTest(srv.URL), &name, &APIConfig{ApiKey: &key}), cfg)
					bound, err := model.WithTools([]*schema.ToolInfo{{Name: "retrieval", Desc: "Search documents"}})
					if err != nil {
						t.Fatal(err)
					}
					messages := []*schema.Message{schema.UserMessage("Find documents")}
					if streaming {
						messages = append(messages, &schema.Message{Role: schema.Tool, Content: "document found", ToolCallID: "call-1"})
						stream, err := bound.Stream(t.Context(), messages)
						if err != nil {
							t.Fatal(err)
						}
						defer stream.Close()
						for {
							_, err := stream.Recv()
							if err == io.EOF {
								break
							}
							if err != nil {
								t.Fatal(err)
							}
						}
					} else {
						response, err := bound.Generate(t.Context(), messages)
						if err != nil {
							t.Fatal(err)
						}
						if response.Content != "ok" {
							t.Errorf("content = %q, want ok", response.Content)
						}
					}
					if seen["tools"] == nil || seen["tool_choice"] != "auto" {
						t.Errorf("tool configuration = %#v", seen)
					}
					if effort == "unset" || effort == "nil_config" || effort == "" {
						if value, present := seen["reasoning_effort"]; present {
							t.Errorf("reasoning_effort = %v, want omitted", value)
						}
					} else if seen["reasoning_effort"] != effort {
						t.Errorf("reasoning_effort = %v, want %q", seen["reasoning_effort"], effort)
					}
				})
			}
		})
	}
}
