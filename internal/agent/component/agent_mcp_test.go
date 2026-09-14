package component

import (
	"encoding/json"
	"fmt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"io"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/utility"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAgentMCPRequiresRuntimeServer(t *testing.T) {
	p := mergeAgentParam(AgentParam{}, map[string]any{"mcp": []any{map[string]any{"mcp_id": "missing", "tools": map[string]any{"route": map[string]any{"name": "route"}}}}})
	if _, err := buildAgentTools(t.Context(), p); err == nil {
		t.Fatal("configured MCP tools silently discarded")
	}
}

func TestAgentMCPModelDispatch(t *testing.T) {
	db := setupComponentTestDB(t)
	if err := db.AutoMigrate(&entity.MCPServer{}); err != nil {
		t.Fatal(err)
	}
	pushComponentDB(t, db)
	lookup := utility.LookupHost
	utility.LookupHost = func(string) ([]string, error) { return []string{"8.8.8.8"}, nil }
	t.Cleanup(func() { utility.LookupHost = lookup })
	var dispatched atomic.Bool
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			w.WriteHeader(204)
			return
		}
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string            `json:"name"`
				Arguments map[string]string `json:"arguments"`
			} `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"protocolVersion":"2025-03-26","capabilities":{}}}`, req.ID)
		case "notifications/initialized":
			w.WriteHeader(202)
		case "tools/call":
			dispatched.Store(true)
			if req.Params.Name != "route" || req.Params.Arguments["origin"] != "大连路站" {
				t.Errorf("unexpected dispatch: %+v", req)
			}
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"content":[{"type":"text","text":"metro route"}]}}`, req.ID)
		}
	}))
	defer mcp.Close()
	pinned := utility.PinnedHTTPClient
	utility.PinnedHTTPClient = func(string, string, time.Duration) *http.Client { return mcp.Client() }
	t.Cleanup(func() { utility.PinnedHTTPClient = pinned })
	if err := db.Create(&entity.MCPServer{ID: "mcp", TenantID: "tenant", Name: "route server", URL: mcp.URL, ServerType: utility.TransportStreamableHTTP}).Error; err != nil {
		t.Fatal(err)
	}
	state := runtime.NewCanvasState("run", "task")
	state.Sys["tenant_id"] = "tenant"
	ctx := withStateForTest(t.Context(), state)
	p := mergeAgentParam(AgentParam{}, map[string]any{"mcp": []any{map[string]any{"mcp_id": "mcp", "tools": map[string]any{"route": map[string]any{"name": "route", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"origin": map[string]any{"type": "string"}}, "required": []string{"origin"}}}}}}})
	tools, err := buildAgentTools(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Tools []struct {
				Function struct {
					Name       string         `json:"name"`
					Parameters map[string]any `json:"parameters"`
				} `json:"function"`
			} `json:"tools"`
			ToolChoice string           `json:"tool_choice"`
			Messages   []models.Message `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Tools) != 1 || req.Tools[0].Function.Name != "route" || req.ToolChoice != "auto" {
			t.Errorf("tools missing from model request: %+v", req)
		}
		if len(req.Tools) == 1 {
			properties, _ := req.Tools[0].Function.Parameters["properties"].(map[string]any)
			if _, ok := properties["origin"]; !ok {
				t.Error("MCP argument schema missing from model request")
			}
		}
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"call1","type":"function","function":{"name":"route","arguments":"{\"origin\":\"大连路站\"}"}}]}}]}`)
		} else {
			raw, _ := json.Marshal(req.Messages)
			if !strings.Contains(string(raw), "metro route") {
				t.Error("MCP result missing from follow-up model request")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"metro answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer llm.Close()
	driver := models.NewOpenAIModel(map[string]string{"default": llm.URL}, models.URLSuffix{Chat: "chat/completions"})
	modelName, key := "test", "test"
	chat := models.NewEinoChatModel(models.NewChatModel(driver, &modelName, &models.APIConfig{ApiKey: &key}), nil)
	a, err := react.NewAgent(ctx, &react.AgentConfig{ToolCallingModel: chat, ToolsConfig: compose.ToolsNodeConfig{Tools: tools}, StreamToolCallChecker: scanAllStreamForToolCall, MaxStep: 3})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := a.Stream(ctx, []*schema.Message{schema.UserMessage("how to go from 大连路站 to 水清三村公寓 by metro")})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var content string
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		content += msg.Content
	}
	if !dispatched.Load() || content != "metro answer" {
		t.Fatalf("dispatch=%v content=%q", dispatched.Load(), content)
	}
	state.Sys["tenant_id"] = "other-tenant"
	if _, err := buildAgentTools(ctx, p); err == nil {
		t.Fatal("cross-tenant MCP access allowed")
	}
}
