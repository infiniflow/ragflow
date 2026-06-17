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

// Package tool — MCP (Model Context Protocol) wrapper.
//
// Wraps a single MCP-server-discovered tool (utility/mcpclient.Tool) as
// an eino BaseTool so it can be invoked from inside the Agent's
// ReAct loop. The MCP tool list is fetched via utility/mcpclient
// (which currently only implements tools/list discovery; tools/call
// invocation is the next step on the MCP client).
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	mcpclient "ragflow/internal/utility"
)

// MCPToolAdapter wraps a single MCP-discovered tool descriptor as an
// eino InvokableTool. The wire format matches what eino's react.Agent
// expects: a ToolInfo with name/description/params, and an
// InvokableRun that accepts a JSON arguments string and returns a
// string result.
//
// InvokableRun dispatches through mcpclient.CallTool
// (streamable-HTTP transport). The MCP server URL + headers
// are captured on construction so the adapter has everything it
// needs to call back into the server. Adapters built without
// a URL (legacy callers) fall back to the "not yet wired"
// sentinel so existing call sites don't break.
type MCPToolAdapter struct {
	mcpTool    mcpclient.Tool
	serverURL  string
	headers    map[string]string
	timeout    time.Duration
	httpClient *http.Client
}

// NewMCPToolAdapter constructs a wrapper for a single MCP tool.
// The returned adapter has no MCP server URL and so cannot be
// invoked — use NewMCPToolAdapterWithServer for adapters that
// need to call back into the server.
func NewMCPToolAdapter(t mcpclient.Tool) *MCPToolAdapter {
	return &MCPToolAdapter{mcpTool: t}
}

// NewMCPToolAdapterWithServer constructs a wrapper that knows
// the MCP server URL + transport headers. InvokableRun uses this
// to route InvokableRun into mcpclient.CallTool.
func NewMCPToolAdapterWithServer(t mcpclient.Tool, serverURL string, headers map[string]string, timeout time.Duration) *MCPToolAdapter {
	return &MCPToolAdapter{
		mcpTool:   t,
		serverURL: serverURL,
		headers:   headers,
		timeout:   timeout,
	}
}

// NewMCPToolAdapterFull is the most-configurable constructor;
// callers can also pass an *http.Client (e.g. an httptest server's
// Client, or a custom transport with mTLS) so the underlying
// CallTool call doesn't have to fall back to a pinned client.
func NewMCPToolAdapterFull(t mcpclient.Tool, serverURL string, headers map[string]string, timeout time.Duration, client *http.Client) *MCPToolAdapter {
	return &MCPToolAdapter{
		mcpTool:    t,
		serverURL:  serverURL,
		headers:    headers,
		timeout:    timeout,
		httpClient: client,
	}
}

// Name returns the underlying MCP tool name.
func (m *MCPToolAdapter) Name() string { return m.mcpTool.Name }

// Info returns eino-compatible tool metadata. InputSchema is
// translated from the MCP tool's JSON Schema.
func (m *MCPToolAdapter) Info(_ context.Context) (*schema.ToolInfo, error) {
	// eino's schema.ParameterInfo shape: name → description.
	// We translate the MCP tool's inputSchema.properties into a
	// best-effort ParameterInfo map. For tools without a JSON schema
	// the params map is empty — eino falls back to free-form args.
	params := make(map[string]*schema.ParameterInfo, len(m.mcpTool.InputSchema))
	for name := range m.mcpTool.InputSchema {
		params[name] = &schema.ParameterInfo{
			Type:     schema.String, // conservative default
			Desc:     fmt.Sprintf("MCP tool parameter: %s", name),
			Required: false, // MCP doesn't surface required; we err permissive
		}
	}
	return &schema.ToolInfo{
		Name:        m.mcpTool.Name,
		Desc:        m.mcpTool.Description,
		ParamsOneOf: schema.NewParamsOneOfByParams(params),
	}, nil
}

// InvokableRun is the eino entry point. When the adapter was
// built with a server URL, dispatch through mcpclient.CallTool.
// Legacy adapters (no URL) keep the "not yet wired" sentinel
// so existing tests that pin the error message don't break.
func (m *MCPToolAdapter) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	if m.serverURL == "" {
		return "", fmt.Errorf("mcp tool %q: tools/call not yet implemented in mcpclient; arguments were: %s",
			m.mcpTool.Name, argumentsInJSON)
	}
	argsJSON, mErr := marshalArguments(argumentsInJSON)
	if mErr != nil {
		return "", mErr
	}
	res, err := mcpclient.CallTool(ctx, mcpclient.CallOptions{
		URL:        m.serverURL,
		ServerType: mcpclient.TransportStreamableHTTP,
		Headers:    m.headers,
		ToolName:   m.mcpTool.Name,
		Arguments:  argsJSON,
		Timeout:    m.timeout,
		HTTPClient: m.httpClient,
	})
	if err != nil {
		return "", err
	}
	if res == nil {
		return "", nil
	}
	if res.IsError {
		// Surface the structured tool error under a known prefix
		// so the ReAct loop can route it as a tool-level error
		// rather than a transport failure.
		return "", fmt.Errorf("mcp tool %q returned isError: %s", m.mcpTool.Name, res.Text)
	}
	return res.Text, nil
}

// BuildMCPToolAdapters wraps a slice of mcpclient.Tool descriptors as
// eino InvokableTool. Returned slice is suitable for handing to
// agenttool.NewRetrieverTool / NewMCPToolAdapter paths or directly to
// the Agent's tool list.
func BuildMCPToolAdapters(tools []mcpclient.Tool) []tool.InvokableTool {
	out := make([]tool.InvokableTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, NewMCPToolAdapter(t))
	}
	return out
}

// marshalArguments is a helper for the future tools/call
// implementation. The argumentsInJSON string from eino is
// round-tripped through json.RawMessage before being passed to the
// MCP server so the server's expected payload structure is preserved.
func marshalArguments(argumentsInJSON string) (json.RawMessage, error) {
	if argumentsInJSON == "" || argumentsInJSON == "{}" {
		return json.RawMessage("{}"), nil
	}
	if !json.Valid([]byte(argumentsInJSON)) {
		return nil, fmt.Errorf("mcp tool: arguments are not valid JSON: %q", argumentsInJSON)
	}
	return json.RawMessage(argumentsInJSON), nil
}
