package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/component"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	mcpclient "ragflow/internal/mcp/client"
	"ragflow/internal/utility"
)

// Keep the real service, canvas, AgentComponent, ReAct loop, model adapter,
// MCP transport and session persistence. Only external HTTP services are fixtures.
func TestRunAgent_ToolHistoryPersists(t *testing.T) {
	for _, tc := range []struct {
		name                                                     string
		deferred, multi, parallel, direct, empty, fail, canceled bool
	}{
		{name: "terminal_agent"},
		{name: "downstream_message", deferred: true},
		{name: "multiple_streamed_rounds", multi: true},
		{name: "parallel_calls", parallel: true},
		{name: "direct_answer", direct: true},
		{name: "empty_final_stream", empty: true},
		{name: "tool_failure", fail: true},
		{name: "canceled_tool", canceled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deferred := tc.deferred
			wantCalls := int32(1)
			if tc.multi || tc.parallel {
				wantCalls = 2
			} else if tc.direct {
				wantCalls = 0
			}
			db := setupServiceTestDB(t)
			if err := db.AutoMigrate(&entity.UserCanvasVersion{}, &entity.APIToken{},
				&entity.TenantModelProvider{}, &entity.TenantModelInstance{},
				&entity.TenantModel{}, &entity.MCPServer{}); err != nil {
				t.Fatal(err)
			}
			pushServiceDB(t, db)
			previousInvoker := component.GetDefaultChatInvokerForTest()
			component.SetDefaultChatInvoker(nil)
			t.Cleanup(func() { component.SetDefaultChatInvoker(previousInvoker) })
			previousAllow := common.AllowAnyHostForTest
			common.AllowAnyHostForTest = true
			t.Cleanup(func() { common.AllowAnyHostForTest = previousAllow })
			if err := models.InitProviderManager("../../conf/models"); err != nil {
				t.Fatal(err)
			}

			var toolCalls, toolResults, summaryCalls atomic.Int32
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				var req struct {
					ID     json.RawMessage `json:"id"`
					Method string          `json:"method"`
					Params struct {
						Name      string            `json:"name"`
						Arguments map[string]string `json:"arguments"`
					} `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch req.Method {
				case "initialize":
					fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-03-26","capabilities":{}}}`, req.ID)
				case "notifications/initialized":
					w.WriteHeader(http.StatusAccepted)
				case "tools/call":
					if req.Params.Name != "lookup" || req.Params.Arguments["query"] != "manual" {
						t.Errorf("unexpected MCP call: %+v", req.Params)
					}
					toolCalls.Add(1)
					if tc.fail {
						http.Error(w, "synthetic tool failure", http.StatusInternalServerError)
						return
					}
					if tc.canceled {
						cancel()
						return
					}
					fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"content":[{"type":"text","text":"synthetic manual result"}]}}`, req.ID)
				default:
					t.Errorf("unexpected MCP method %q", req.Method)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer mcp.Close()
			previousPinned := utility.PinnedHTTPClient
			utility.PinnedHTTPClient = func(string, string, time.Duration) *http.Client { return mcp.Client() }
			t.Cleanup(func() { utility.PinnedHTTPClient = previousPinned })

			const answer = "The synthetic manual contains the answer."
			const summary = "Looked up the synthetic manual."
			llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Stream   bool `json:"stream"`
					Messages []struct {
						Role       string `json:"role"`
						Content    any    `json:"content"`
						ToolCallID string `json:"tool_call_id"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if len(req.Messages) > 0 && strings.Contains(fmt.Sprint(req.Messages[0].Content), "memory summarizer") {
					summaryCalls.Add(1)
					raw, _ := json.Marshal(req.Messages)
					if strings.Count(fmt.Sprint(req.Messages[1].Content), "lookup_0(") != int(wantCalls) || !strings.Contains(string(raw), "manual") {
						t.Errorf("summary lost tool metadata: %s", raw)
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
						"message": map[string]any{"role": "assistant", "content": summary}, "finish_reason": "stop",
					}}})
					return
				}
				results := 0
				for _, message := range req.Messages {
					if message.Role == "user" && strings.Contains(fmt.Sprint(message.Content), "direct on next turn") {
						tcDirectAnswer(w, answer)
						return
					}
					if message.Role == "tool" {
						if (message.ToolCallID != "call_lookup" && message.ToolCallID != "call_lookup_2") || !strings.Contains(fmt.Sprint(message.Content), "synthetic manual result") {
							t.Errorf("tool result not delivered to model: %+v", message)
						}
						results++
					}
				}
				toolResults.Store(int32(results))
				if tc.direct {
					tcDirectAnswer(w, answer)
					return
				}
				if !req.Stream {
					w.Header().Set("Content-Type", "application/json")
					calls := []any{map[string]any{"id": "call_lookup", "type": "function", "function": map[string]any{"name": "lookup_0", "arguments": `{"query":"manual"}`}}}
					if tc.parallel {
						calls = append(calls, map[string]any{"id": "call_lookup_2", "type": "function", "function": map[string]any{"name": "lookup_0", "arguments": `{"query":"manual"}`}})
					}
					json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"index": 0, "finish_reason": "tool_calls", "message": map[string]any{"role": "assistant", "tool_calls": calls}}}})
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				send := func(delta map[string]any, finish any) {
					raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
						"index": 0, "delta": delta, "finish_reason": finish,
					}}})
					fmt.Fprintf(w, "data: %s\n\n", raw)
				}
				if !tc.multi || results == 2 {
					if !tc.empty {
						send(map[string]any{"role": "assistant", "content": answer}, "stop")
					} else {
						send(map[string]any{}, "stop")
					}
					fmt.Fprint(w, "data: [DONE]\n\n")
					return
				}
				// After one result, request another tool through the actual SSE parser.
				send(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{
					"index": 0, "id": "call_lookup_2", "type": "function",
					"function": map[string]any{"name": "lookup_0", "arguments": "{\"query\":"},
				}}}, nil)
				send(map[string]any{"tool_calls": []any{map[string]any{
					"index": 0, "function": map[string]any{"arguments": "\"manual\"}"},
				}}}, "tool_calls")
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer llm.Close()

			canvasID, sessionID := "canvas-tool-history", "session-tool-history"
			params := map[string]any{
				"llm_id": "fixture-model", "driver": "OpenAI-API-Compatible",
				"api_key": "synthetic-key", "base_url": llm.URL,
				"prompts":    []any{map[string]any{"role": "user", "content": "{sys.query}"}},
				"cite":       false,
				"max_rounds": 3,
				"mcp": []any{map[string]any{"mcp_id": "mcp-tool-history", "tools": map[string]any{
					"lookup": map[string]any{"name": "lookup", "inputSchema": map[string]any{
						"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}},
						"required": []string{"query"},
					}},
				}}},
			}
			agentNode := map[string]any{
				"obj":      map[string]any{"component_name": "Agent", "params": params},
				"upstream": []any{"begin_0"},
			}
			nodes := map[string]any{
				"begin_0": map[string]any{
					"obj":        map[string]any{"component_name": "Begin", "params": map[string]any{}},
					"downstream": []any{"Agent:worker"},
				},
				"Agent:worker": agentNode,
			}
			if deferred {
				agentNode["downstream"] = []any{"Message:answer"}
				nodes["Message:answer"] = map[string]any{
					"obj": map[string]any{"component_name": "Message",
						"params": map[string]any{"content": []any{"{Agent:worker@content}\nCalls: {Agent:worker@tool_calls}"}}},
					"upstream": []any{"Agent:worker"},
				}
			}
			dsl := map[string]any{"components": nodes, "path": []any{}}
			makeCanvasWithDSL(t, canvasID, "user-1", "tenant-1", "version-tool-history", dsl)
			modelExtra, _ := json.Marshal(map[string]string{"base_url": llm.URL})
			for _, row := range []any{
				&entity.TenantModelProvider{ID: "provider-history", UserID: "user-1", TenantID: "tenant-1", ProviderName: "OpenAI-API-Compatible"},
				&entity.TenantModelInstance{ID: "instance-history", ProviderID: "provider-history", InstanceName: "synthetic", APIKey: "synthetic-key", Status: "active", Extra: string(modelExtra)},
				&entity.TenantModel{ID: "fixture-model", ModelName: "fixture-model", ProviderID: "provider-history", InstanceID: "instance-history", ModelType: int(entity.ModelTypeChat), Status: "active", Extra: "{}"},
			} {
				if err := db.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Create(&entity.MCPServer{ID: "mcp-tool-history", TenantID: "tenant-1",
				Name: "synthetic lookup", URL: mcp.URL, ServerType: mcpclient.TransportStreamableHTTP}).Error; err != nil {
				t.Fatal(err)
			}
			tracker, _ := newAgentCancelTracker(t)
			svc := NewAgentServiceWithOptions(nil, nil, tracker)
			events, err := svc.RunAgent(WithAgentSessionID(ctx, sessionID), "user-1", canvasID, sessionID, "", "Find the manual", nil)
			if err != nil {
				t.Fatal(err)
			}
			var visible strings.Builder
			var exportedCalls []any
			var nodeAnswer string
			var runErrors int
			deadline := time.After(15 * time.Second)
			for {
				select {
				case event, ok := <-events:
					if !ok {
						goto finished
					}
					switch event.Type {
					case "error":
						runErrors++
						if !tc.fail && !tc.canceled {
							t.Errorf("Agent error event: %s", event.Data)
						}
					case "message":
						var message canvas.MessageEvent
						if err := json.Unmarshal([]byte(event.Data), &message); err != nil {
							t.Fatal(err)
						}
						visible.WriteString(message.Content)
					case "node_finished":
						var node canvas.NodeFinishedData
						if err := json.Unmarshal([]byte(event.Data), &node); err != nil {
							t.Fatal(err)
						}
						if node.ComponentID == "Agent:worker" {
							outputs, _ := node.Outputs.(map[string]any)
							exportedCalls, _ = outputs["tool_calls"].([]any)
							nodeAnswer, _ = outputs["content"].(string)
						}
					}
				case <-deadline:
					t.Fatal("Agent execution did not finish")
				}
			}
		finished:
			if tc.fail || tc.canceled {
				if toolCalls.Load() != 1 || summaryCalls.Load() != 0 || (tc.fail && runErrors == 0) {
					t.Fatalf("failed/canceled run: attempted=%d summaries=%d errors=%d", toolCalls.Load(), summaryCalls.Load(), runErrors)
				}
				if memory := getAPIConversationForTest(t, db, sessionID).DSL["memory"]; memory != nil {
					if raw, _ := json.Marshal(memory); string(raw) != "[]" && string(raw) != "null" {
						t.Errorf("failed run persisted memory: %s", raw)
					}
				}
				return
			}
			if toolCalls.Load() != wantCalls || toolResults.Load() != wantCalls || (!tc.empty && !strings.Contains(visible.String(), answer) && nodeAnswer != answer) {
				t.Fatalf("real Agent prerequisites failed: tool executions=%d model tool results=%d streamed=%q node=%q", toolCalls.Load(), toolResults.Load(), visible.String(), nodeAnswer)
			}
			t.Logf("real chain: MCP executed=%d, model consumed results=%d, empty final=%v, answer contract verified", toolCalls.Load(), toolResults.Load(), tc.empty)
			if deferred {
				// Validate deferred output through the actual downstream consumer.
				_, payload, found := strings.Cut(visible.String(), "\nCalls: ")
				if !found {
					t.Fatalf("downstream tool_calls output missing: %s", visible.String())
				}
				if err := json.Unmarshal([]byte(payload), &exportedCalls); err != nil {
					t.Fatalf("downstream tool_calls is not JSON: %s: %v", payload, err)
				}
			}
			if len(exportedCalls) != int(wantCalls) {
				t.Errorf("Agent tool_calls=%v, want the executed lookup call", exportedCalls)
			}
			for i, raw := range exportedCalls {
				call, _ := raw.(map[string]any)
				id := "call_lookup"
				if i == 1 {
					id = "call_lookup_2"
				}
				if call["id"] != id || call["type"] != "function" || call["name"] != "lookup_0" || call["arguments"] != `{"query":"manual"}` {
					t.Errorf("exported tool metadata = %#v", call)
				}
			}
			session := getAPIConversationForTest(t, db, sessionID)
			memory, _ := session.DSL["memory"].([]any)
			wantMemory := 1
			if tc.direct {
				wantMemory = 0
			}
			if summaryCalls.Load() != int32(wantMemory) || len(memory) != wantMemory {
				t.Fatalf("executed tool lost from persisted memory: summary calls=%d memory=%#v", summaryCalls.Load(), session.DSL["memory"])
			}
			raw, _ := json.Marshal(memory)
			if wantMemory > 0 && !strings.Contains(string(raw), summary) {
				t.Errorf("persisted memory missing summary: %s", raw)
			}
			// A fresh direct-answer turn reuses persisted DSL but must not re-summarize old calls.
			events, err = svc.RunAgent(WithAgentSessionID(ctx, sessionID), "user-1", canvasID, sessionID, "", "direct on next turn", nil)
			if err != nil {
				t.Fatal(err)
			}
			messages, _, errs, _, _ := drainAgentEvents(t, events)
			if len(errs) != 0 || (deferred && len(messages) == 0) {
				t.Fatalf("next turn failed: %+v", errs)
			}
			nextMemory, _ := json.Marshal(getAPIConversationForTest(t, db, sessionID).DSL["memory"])
			if string(nextMemory) != string(raw) || summaryCalls.Load() != int32(wantMemory) || toolCalls.Load() != wantCalls {
				t.Fatalf("next direct turn reused old calls: memory=%s summaries=%d tools=%d", nextMemory, summaryCalls.Load(), toolCalls.Load())
			}
		})
	}
}

func tcDirectAnswer(w http.ResponseWriter, answer string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": answer}}}})
}
