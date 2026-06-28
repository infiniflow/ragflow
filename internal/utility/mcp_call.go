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

// Phase 3.7: MCP tools/call implementation. The mcp_client.go
// file handles tools/list discovery; this file adds the
// tools/call invocation path so the MCPToolAdapter can return
// real results instead of "not yet implemented" errors.
//
// The implementation focuses on the streamable-HTTP transport
// (spec 2025-03-26) because that is the dominant transport for
// modern MCP servers. The legacy SSE transport's session
// lifecycle is more complex; deferring it matches the rest of
// the package's "loud-fail with a clear error" pattern.

package utility

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// CallOptions controls a single tools/call invocation. The same
// URL safety / DNS pinning guarantees that FetchTools applies
// (AssertURLSafe + PinnedHTTPClient) are reused so this path
// cannot be coerced into SSRF via a malicious MCP server.
type CallOptions struct {
	URL        string
	ServerType string
	Headers    map[string]string
	Variables  map[string]string
	ToolName   string
	Arguments  json.RawMessage // JSON-encoded argument object
	Timeout    time.Duration
	HTTPClient *http.Client
}

// CallResult is the parsed tools/call response. The MCP spec
// (2025-03-26 §4.3) defines the result envelope as
// { "content": [ ... ], "isError": bool } where each content
// entry is one of {type: "text", text: "..."} or {type:
// "image"|"audio"|"resource", ...}.
//
// For the Phase 3.7 milestone the Go side surfaces Text
// (concatenated text content) and the structured content list
// so callers can branch on type when they care. IsError is
// surfaced so a tool that returns a structured error message
// (rather than a JSON-RPC error) is still distinguishable.
type CallResult struct {
	Text    string           `json:"text"`
	Content []map[string]any `json:"content"`
	IsError bool             `json:"is_error"`
}

// CallTool invokes an MCP tool by name. URL safety is enforced
// the same way FetchTools does it; the same protocol constants
// (protocolVersion, clientName, etc.) apply. The session is
// per-call (initialize + notifications/initialized +
// tools/call) — a future optimization can pool sessions.
func CallTool(ctx context.Context, opts CallOptions) (*CallResult, error) {
	if opts.URL == "" {
		return nil, errors.New("Invalid url.")
	}
	if opts.ToolName == "" {
		return nil, errors.New("MCP tool name is required")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	hostname, resolvedIP, err := AssertURLSafe(opts.URL)
	if err != nil {
		return nil, err
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = PinnedHTTPClient(hostname, resolvedIP, opts.Timeout)
	}
	headers, headerErr := renderHeaders(opts.Headers, opts.Variables)
	if headerErr != nil {
		return nil, headerErr
	}
	connectCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	switch opts.ServerType {
	case TransportStreamableHTTP, "":
		// Empty ServerType is treated as streamable-http because
		// that is the default per the spec. Servers explicitly
		// declaring the legacy SSE transport get the legacy path.
		return callToolStreamableHTTP(connectCtx, opts.URL, headers, opts.HTTPClient, opts.ToolName, opts.Arguments)
	case TransportSSE:
		return nil, errors.New("MCP tools/call on legacy SSE transport is not yet implemented in Go (Phase 3.7 deferred; use streamable-http)")
	default:
		return nil, fmt.Errorf("Unsupported MCP server type.")
	}
}

// callToolStreamableHTTP drives the streamable-HTTP session:
// initialize → notifications/initialized → tools/call. The
// session is torn down at the end (the server is free to
// garbage-collect the session id; future calls re-initialize).
func callToolStreamableHTTP(ctx context.Context, endpoint string, headers map[string]string, client *http.Client, toolName string, args json.RawMessage) (*CallResult, error) {
	sessionID, initRes, err := streamableSend(ctx, client, endpoint, "", headers, jsonRPCRequest{
		JSONRPC: jsonRPCVersion,
		ID:      0,
		Method:  "initialize",
		Params:  initializeParams(),
	}, true)
	if err != nil {
		return nil, err
	}
	if initRes.Error != nil {
		return nil, formatMCPError("initialize", initRes.Error)
	}

	if _, _, err := streamableSend(ctx, client, endpoint, sessionID, headers, jsonRPCRequest{
		JSONRPC: jsonRPCVersion,
		Method:  "notifications/initialized",
	}, false); err != nil {
		return nil, err
	}

	var argsAny any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &argsAny); err != nil {
			return nil, fmt.Errorf("mcp tools/call: arguments are not valid JSON: %w", err)
		}
	}
	_, callRes, err := streamableSend(ctx, client, endpoint, sessionID, headers, jsonRPCRequest{
		JSONRPC: jsonRPCVersion,
		ID:      2,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      toolName,
			"arguments": argsAny,
		},
	}, true)
	if err != nil {
		return nil, err
	}
	if callRes.Error != nil {
		return nil, formatMCPError("tools/call", callRes.Error)
	}
	return parseCallResult(callRes.Result)
}

// parseCallResult decodes the tools/call response envelope.
// The result is { "content": [ {type, ...}, ...], "isError":
// bool }. Text content blocks are concatenated into Result.Text
// (most agents consume a single string); the full Content
// slice is preserved for callers that need to distinguish
// text / image / resource blocks.
func parseCallResult(raw json.RawMessage) (*CallResult, error) {
	if len(raw) == 0 {
		return &CallResult{}, nil
	}
	var envelope struct {
		Content []map[string]any `json:"content"`
		IsError bool             `json:"isError"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("parse tools/call result: %w", err)
	}
	out := &CallResult{
		Content: envelope.Content,
		IsError: envelope.IsError,
	}
	for _, block := range envelope.Content {
		t, _ := block["type"].(string)
		if t != "text" {
			continue
		}
		if s, ok := block["text"].(string); ok {
			if out.Text != "" {
				out.Text += "\n"
			}
			out.Text += s
		}
	}
	return out, nil
}
