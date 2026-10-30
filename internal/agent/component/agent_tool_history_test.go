package component

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/entity/models"
)

// Drive actual nested runners and HTTP adapters. The outer Eino future also
// observes nested model callbacks, so holder identity alone is not sufficient.
func TestAgentToolHistoryNestedAndConcurrent(t *testing.T) {
	setupAllowAnyHost(t, true)
	if err := models.InitProviderManager("../../../conf/models"); err != nil {
		t.Fatal(err)
	}
	previous := getDefaultChatInvoker()
	SetDefaultChatInvoker(nil)
	t.Cleanup(func() { SetDefaultChatInvoker(previous) })
	var mu sync.Mutex
	summaries := map[string][]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		if len(req.Messages) > 0 && strings.Contains(req.Messages[0].Content, "memory summarizer") {
			mu.Lock()
			summaries[req.Model] = append(summaries[req.Model], req.Messages[1].Content)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":"%s summary"},"finish_reason":"stop"}]}`, req.Model)
			return
		}
		if !req.Stream {
			name := "child"
			if req.Model == "child-model" {
				name = "leaf"
			}
			w.Header().Set("Content-Type", "application/json")
			call := map[string]any{"id": req.Model + "-call", "type": "function", "function": map[string]any{"name": name, "arguments": `{"user_prompt":"nested task"}`}}
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "tool_calls": []any{call}}, "finish_reason": "tool_calls"}}})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"%s answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", req.Model)
	}))
	defer server.Close()
	param := func(name string) AgentParam {
		return AgentParam{ModelID: name + "-model", Driver: "OpenAI-API-Compatible", APIKey: "synthetic", BaseURL: server.URL, UserPrompt: "task", MaxRounds: 2}
	}
	leaf := param("leaf")
	child := param("child")
	child.SubAgents = []SubAgentTool{{Name: "leaf", Param: leaf}}
	parent := param("parent")
	parent.SubAgents = []SubAgentTool{{Name: "child", Param: child}}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	state := runtime.NewCanvasState("nested-history", "tenant")
	out, err := NewAgentComponent(parent).Invoke(runtime.WithState(ctx, state), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	calls, _ := out["tool_calls"].([]map[string]any)
	if len(calls) != 1 || calls[0]["name"] != "child" || calls[0]["id"] != "parent-model-call" {
		t.Fatalf("parent history contains nested calls: %#v", calls)
	}
	mu.Lock()
	if len(summaries["parent-model"]) != 1 || summaries["parent-model"][0] != `Tool calls: child({"user_prompt":"nested task"})` {
		t.Errorf("parent summaries: %#v", summaries["parent-model"])
	}
	if len(summaries["child-model"]) != 1 || summaries["child-model"][0] != `Tool calls: leaf({"user_prompt":"nested task"})` {
		t.Errorf("child summaries: %#v", summaries["child-model"])
	}
	mu.Unlock()
	if len(state.SnapshotMemory()) != 2 {
		t.Errorf("nested memory: %#v", state.SnapshotMemory())
	}
	// Both roots inherit the same context. Neither may pick up the other's calls.
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			msg, err := runEinoReActAgent(ctx, parent)
			if err != nil {
				t.Error(err)
				return
			}
			if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "child" {
				t.Errorf("concurrent root history: %#v", msg.ToolCalls)
			}
		})
	}
	wg.Wait()
}

func TestAgentToolHistoryMergesFragmentsPerRound(t *testing.T) {
	collector := &toolCallCollector{}
	ctx := context.WithValue(t.Context(), toolCallCollectorKey{}, collector)
	index := 0
	for _, id := range []string{"first", "second"} {
		stream := schema.StreamReaderFromArray([]*schema.Message{
			{Role: schema.Assistant, Content: "checking"},
			{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{Index: &index, ID: id, Type: "function", Function: schema.FunctionCall{Name: "lookup", Arguments: `{"q":`}}}},
			{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{Index: &index, Function: schema.FunctionCall{Arguments: `"manual"}`}}}},
		})
		if found, err := scanAllStreamForToolCall(ctx, stream); err != nil || !found {
			t.Fatalf("checker: %v, %v", found, err)
		}
	}
	if len(collector.calls) != 2 {
		t.Fatalf("calls: %#v", collector.calls)
	}
	for i, id := range []string{"first", "second"} {
		if call := collector.calls[i]; call.ID != id || call.Function.Arguments != `{"q":"manual"}` {
			t.Errorf("round %d: %#v", i, call)
		}
	}
}
