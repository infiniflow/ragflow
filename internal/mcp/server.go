// Package mcp exposes native RAGFlow services through the official MCP SDK.
package mcp

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Connector is the service boundary shared by both MCP transports.
type Connector interface {
	ListDatasets(context.Context, int, int, string, bool) (string, error)
	ListChats(context.Context, int, int, string, bool) (string, error)
	Retrieval(context.Context, RetrievalRequest) (string, error)
}

type RetrievalRequest struct {
	DatasetIDs             []string `json:"dataset_ids"`
	DocumentIDs            []string `json:"document_ids"`
	Question               string   `json:"question"`
	Page                   int      `json:"page"`
	PageSize               int      `json:"page_size"`
	SimilarityThreshold    float64  `json:"similarity_threshold"`
	VectorSimilarityWeight float64  `json:"vector_similarity_weight"`
	TopK                   int      `json:"top_k"`
	RerankID               string   `json:"rerank_id,omitempty"`
	Keyword                bool     `json:"keyword"`
	ForceRefresh           bool     `json:"force_refresh"`
}

// toolsJSON contains the schemas and base descriptions from mcp/server/server.py.
//
//go:embed tools.json
var toolsJSON []byte

func textResult(text string) *sdk.CallToolResult {
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: text}}}
}

func newServer(ctx context.Context, connector Connector) *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{Name: "ragflow-mcp-server", Version: "1.0.0"}, &sdk.ServerOptions{
		Capabilities:              &sdk.ServerCapabilities{Tools: &sdk.ToolCapabilities{}},
		SupportedProtocolVersions: []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"},
	})
	var tools []*sdk.Tool
	if err := json.Unmarshal(toolsJSON, &tools); err != nil {
		panic(err)
	}
	sdk.AddTool(server, tools[0], func(ctx context.Context, _ *sdk.CallToolRequest, input RetrievalRequest) (*sdk.CallToolResult, any, error) {
		text, err := connector.Retrieval(ctx, input)
		return textResult(text), nil, err
	})
	type pageInput struct {
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
	}
	sdk.AddTool(server, tools[1], func(ctx context.Context, _ *sdk.CallToolRequest, input pageInput) (*sdk.CallToolResult, any, error) {
		text, err := connector.ListDatasets(ctx, input.Page, input.PageSize, "create_time", true)
		return textResult(text), nil, err
	})
	sdk.AddTool(server, tools[2], func(ctx context.Context, _ *sdk.CallToolRequest, input pageInput) (*sdk.CallToolResult, any, error) {
		text, err := connector.ListChats(ctx, input.Page, input.PageSize, "create_time", true)
		return textResult(text), nil, err
	})
	requestContext := ctx
	server.AddReceivingMiddleware(func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
			ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			stop := context.AfterFunc(requestContext, cancel)
			defer stop()
			if method == "tools/list" {
				datasets, err := connector.ListDatasets(ctx, 1, -1, "create_time", true)
				if err != nil {
					return nil, err
				}
				chats, err := connector.ListChats(ctx, 1, 30, "create_time", true)
				if err != nil {
					return nil, err
				}
				// Descriptions are built for this request, never stored on a shared server.
				result := &sdk.ListToolsResult{Tools: make([]*sdk.Tool, len(tools))}
				for i, tool := range tools {
					copy := *tool
					if i == 2 {
						copy.Description += chats
					} else {
						copy.Description += datasets
					}
					result.Tools[i] = &copy
				}
				return result, nil
			}
			if method == "tools/call" {
				call := req.(*sdk.CallToolRequest)
				switch call.Params.Name {
				case "ragflow_retrieval", "ragflow_list_datasets", "ragflow_list_chats":
				default:
					result := textResult(fmt.Sprintf("Tool not found: %s", call.Params.Name))
					result.IsError = true
					return result, nil
				}
			}
			return next(ctx, method, req)
		}
	})
	return server
}
