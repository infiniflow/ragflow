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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprint(streaming), func(t *testing.T) {
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
			bound, err := wrapper.WithTools([]*schema.ToolInfo{{Name: "anysearch_search", Desc: strings.Repeat("Search the web. ", 20)}})
			if err != nil {
				t.Fatal(err)
			}
			wrapper = bound.(*roundLimitedChatModel)
			original := strings.Repeat("evidence ", 5000)
			input := []*schema.Message{schema.UserMessage("Find evidence"), {Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "anysearch_search", Arguments: `{"query":"q"}`}}}}, {Role: schema.Tool, ToolCallID: "call-1", ToolName: "anysearch_search", Content: original}}
			if streaming {
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
