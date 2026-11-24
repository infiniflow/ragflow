//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package component

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/tokenizer"
)

func TestAnySearchAgentFitsActualModelRequest(t *testing.T) {
	setupAllowAnyHost(t, true)
	if err := models.InitProviderManager("../../../conf/models"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, toolName string
		streaming      bool
	}{
		{"search", "anysearch_search", false},
		{"search stream", "anysearch_search", true},
		{"extract", "anysearch_extract", false},
		{"extract stream", "anysearch_extract", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const window = 1024
			var received bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				received = true
				encoded, _ := json.Marshal(request["messages"])
				toolDefinitions, _ := json.Marshal(request["tools"])
				if tokenizer.NumTokensFromString(string(encoded))+tokenizer.NumTokensFromString(string(toolDefinitions))+128 > window {
					w.WriteHeader(400)
					fmt.Fprint(w, `{"error":{"message":"context length exceeded","type":"invalid_request_error"}}`)
					return
				}
				if request["stream"] == true {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"bounded\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
					return
				}
				fmt.Fprint(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"bounded"},"finish_reason":"stop"}]}`)
			}))
			defer server.Close()
			outputTokens := 128
			inner, err := buildAgentChatModel(t.Context(), AgentParam{ModelID: "small-model", Driver: "OpenAI-API-Compatible", APIKey: "synthetic", BaseURL: server.URL, MaxTokens: &outputTokens})
			if err != nil {
				t.Fatal(err)
			}
			wrapper := &roundLimitedChatModel{ToolCallingChatModel: inner, remaining: 2, contextLength: window, outputTokens: 128}
			bound, err := wrapper.WithTools([]*schema.ToolInfo{{Name: tc.toolName, Desc: strings.Repeat("Read the web. ", 20)}})
			if err != nil {
				t.Fatal(err)
			}
			wrapper = bound.(*roundLimitedChatModel)
			original := strings.Repeat("evidence ", 5000)
			input := []*schema.Message{schema.UserMessage("Find evidence"), {Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: tc.toolName, Arguments: `{}`}}}}, {Role: schema.Tool, ToolCallID: "call-1", ToolName: tc.toolName, Content: original}}
			if tc.streaming {
				stream, err := wrapper.Stream(t.Context(), input)
				if err != nil {
					t.Fatal(err)
				}
				defer stream.Close()
				for {
					_, err = stream.Recv()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
				}
			} else {
				if _, err := wrapper.Generate(t.Context(), input); err != nil {
					t.Fatal(err)
				}
			}
			if !received {
				t.Fatal("no actual HTTP model request")
			}
			if input[2].Content != original {
				t.Fatal("original tool evidence was mutated")
			}
		})
	}
}

func TestAnySearchAgentBudgetPreservesImageAndEvidence(t *testing.T) {
	setupAllowAnyHost(t, true)
	if err := models.InitProviderManager("../../../conf/models"); err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	_, _ = rand.New(rand.NewSource(1)).Read(img.Pix)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
	const evidence = `{"queries":[{"results":[{"content":"small usable result"}]}]}`
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprint(streaming), func(t *testing.T) {
			var received atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Messages []struct {
						Role    string          `json:"role"`
						Content json.RawMessage `json:"content"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				received.Store(true)
				if len(request.Messages) != 3 {
					t.Errorf("messages=%d, want user, tool call and tool result", len(request.Messages))
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				var parts []struct {
					Type     string `json:"type"`
					Text     string `json:"text"`
					ImageURL *struct {
						URL string `json:"url"`
					} `json:"image_url"`
				}
				if err := json.Unmarshal(request.Messages[0].Content, &parts); err != nil {
					t.Error(err)
				}
				if len(parts) != 2 || parts[0].Text != "Compare the image with web evidence" || parts[1].ImageURL == nil || parts[1].ImageURL.URL != uri {
					t.Error("model request lost or changed user text/image")
				}
				var result string
				if err := json.Unmarshal(request.Messages[2].Content, &result); err != nil || result != evidence {
					t.Errorf("successful AnySearch evidence changed: %s", request.Messages[2].Content)
				}
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"compared\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
					return
				}
				fmt.Fprint(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"compared"},"finish_reason":"stop"}]}`)
			}))
			defer server.Close()
			outputTokens := 128
			inner, err := buildAgentChatModel(t.Context(), AgentParam{ModelID: "small-model", Driver: "OpenAI-API-Compatible", APIKey: "synthetic", BaseURL: server.URL, MaxTokens: &outputTokens})
			if err != nil {
				t.Fatal(err)
			}
			wrapper := &roundLimitedChatModel{ToolCallingChatModel: inner, remaining: 2, contextLength: 1024, outputTokens: outputTokens}
			bound, err := wrapper.WithTools([]*schema.ToolInfo{{Name: "anysearch_search", Desc: "Search the web"}})
			if err != nil {
				t.Fatal(err)
			}
			wrapper = bound.(*roundLimitedChatModel)
			input := buildAgentInputMessages(t.Context(), AgentParam{UserPrompt: "Compare the image with web evidence", visualFiles: []string{uri}})
			input = append(input, &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "anysearch_search", Arguments: `{"query":"q"}`}}}}, &schema.Message{Role: schema.Tool, ToolCallID: "call-1", ToolName: "anysearch_search", Content: evidence})
			if streaming {
				stream, err := wrapper.Stream(t.Context(), input)
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
			} else if _, err := wrapper.Generate(t.Context(), input); err != nil {
				t.Fatal(err)
			}
			if !received.Load() {
				t.Fatal("no actual HTTP model request")
			}
			if *input[0].UserInputMultiContent[1].Image.URL != uri || input[2].Content != evidence {
				t.Fatal("original image or tool evidence was mutated")
			}
		})
	}
}

func TestAnySearchAgentBudgetRejectsOversizedMultimodalText(t *testing.T) {
	for _, plainContent := range []string{"", "Plain content alongside multipart text"} {
		t.Run(plainContent, func(t *testing.T) {
			user := userMessageWithImages("Short first part", []string{"https://example.com/image.png"})
			user.Content = plainContent
			user.UserInputMultiContent = append(user.UserInputMultiContent, schema.MessageInputPart{Type: schema.ChatMessagePartTypeText, Text: strings.Repeat("oversized user text ", 5000)})
			model := &roundLimitedChatModel{contextLength: 1024, outputTokens: 128}
			input := []*schema.Message{&user, {Role: schema.Tool, ToolName: "anysearch_search", Content: "small usable result"}}
			if _, err := model.fitAnySearchInput(input); err == nil {
				t.Fatal("oversized multipart user text was accepted after omitting AnySearch output")
			}
		})
	}
}

func TestAnySearchAgentBudgetPreservesOtherEvidence(t *testing.T) {
	model := &roundLimitedChatModel{contextLength: 2048, outputTokens: 256}
	input := []*schema.Message{schema.UserMessage("Find evidence"), {Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "large-call", Function: schema.FunctionCall{Name: "anysearch_search"}}}}, {Role: schema.Tool, ToolCallID: "large-call", Content: strings.Repeat("large ", 5000)}, {Role: schema.Tool, ToolName: "anysearch_search", ToolCallID: "small-call", Content: `{"queries":[{"results":[{"content":"small usable result"}]}]}`}}
	fitted, err := model.fitAnySearchInput(input)
	if err != nil {
		t.Fatal(err)
	}
	if fitted[2].ToolCallID != "large-call" || !json.Valid([]byte(fitted[2].Content)) || !strings.Contains(fitted[2].Content, "_ERROR") {
		t.Fatalf("invalid replacement: %+v", fitted[2])
	}
	if fitted[3].Content != input[3].Content {
		t.Fatal("smaller independent result was lost")
	}
	model.toolSchemaTokens = 2048
	if _, err = model.fitAnySearchInput(input); err == nil {
		t.Fatal("unfittable schema budget was accepted")
	}
}

func TestAnySearchAgentUsesTenantContextOverride(t *testing.T) {
	db := setupComponentTestDB(t)
	for _, row := range []any{
		&entity.TenantModelProvider{ID: "budget-provider", UserID: "tenant-1", TenantID: "tenant-1", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "budget-instance", ProviderID: "budget-provider", InstanceName: "default", APIKey: "synthetic", Status: "active"},
		&entity.TenantModel{ID: "0123456789abcdef0123456789abcdef", ProviderID: "budget-provider", InstanceID: "budget-instance", ModelName: "gpt-4o", ModelType: int(entity.ModelTypeChat), Status: "active", Extra: `{"max_tokens":2048}`},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	previous := agentRunner
	t.Cleanup(func() { agentRunner = previous })
	var contextLength int
	agentRunner = func(_ context.Context, p AgentParam) (*schema.Message, error) {
		contextLength = p.contextLength
		return schema.AssistantMessage("ok", nil), nil
	}
	component := NewAgentComponent(AgentParam{ModelID: "0123456789abcdef0123456789abcdef", UserPrompt: "Find evidence"})
	if _, err := component.Invoke(stateWithTenant("tenant-1"), db, nil); err != nil {
		t.Fatal(err)
	}
	if contextLength != 2048 {
		t.Fatalf("context length=%d, want tenant override 2048", contextLength)
	}
}
