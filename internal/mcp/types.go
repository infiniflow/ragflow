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

// Package mcp implements the MCP (Model Context Protocol) server embedded in
// the RAGFlow Go backend. It exposes RAGFlow capabilities (retrieval, dataset
// listing, chat listing) as MCP tools that external AI clients can discover
// and invoke via JSON-RPC over HTTP.
package mcp

import (
	"encoding/json"
	"fmt"
)

// JSONRPCVersion is the protocol version string.
const JSONRPCVersion = "2.0"

// MCPProtocolVersion is the MCP protocol version this server implements.
const MCPProtocolVersion = "2024-11-05"

// ServerName identifies this MCP server instance.
const ServerName = "ragflow-mcp-server"

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCNotification represents a JSON-RPC 2.0 notification
// (a request without an id, which requires no response).
type JSONRPCNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Predefined JSON-RPC error codes.
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
)

// InitializeResult is the response payload for the "initialize" method.
type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
}

// Capabilities describes the set of MCP capabilities this server supports.
type Capabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

// ToolsCapability indicates that the server supports tools and optionally
// whether tool list changes are notified.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ServerInfo provides identifying information about the MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Tool represents an MCP tool definition.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema is the JSON Schema for a tool's input parameters.
type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

// Property describes a single parameter in a tool's InputSchema.
type Property struct {
	Type        string      `json:"type"`
	Description string      `json:"description,omitempty"`
	Default     interface{} `json:"default,omitempty"`
	Minimum     *float64    `json:"minimum,omitempty"`
	Maximum     *float64    `json:"maximum,omitempty"`
	Items       *Items      `json:"items,omitempty"`
}

// Items describes the expected element type for array-typed properties.
type Items struct {
	Type string `json:"type"`
}

// ListToolsResult is the result of the "tools/list" method.
type ListToolsResult struct {
	Tools []Tool `json:"tools"`
}

// CallToolParams is the params payload for the "tools/call" method.
type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// CallToolResult is the result of the "tools/call" method.
type CallToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ContentItem is a single item in a CallToolResult's content array.
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Data string `json:"data,omitempty"`
	// MIME Type of the data, if present.
	MIMEType string `json:"mimeType,omitempty"`
}

// NewErrorResponse creates a JSONRPCResponse with an error.
func NewErrorResponse(id json.RawMessage, code int, message string) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
	}
}

// NewSuccessResponse creates a JSONRPCResponse with a result.
func NewSuccessResponse(id json.RawMessage, result interface{}) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  result,
	}
}

// NewTextContent creates a ContentItem of type "text".
func NewTextContent(text string) ContentItem {
	return ContentItem{Type: "text", Text: text}
}

// NewTextResult creates a CallToolResult with a single text content item.
func NewTextResult(text string) *CallToolResult {
	return &CallToolResult{
		Content: []ContentItem{NewTextContent(text)},
	}
}

// NewErrorResult creates a CallToolResult indicating a tool execution error.
func NewErrorResult(errMsg string) *CallToolResult {
	return &CallToolResult{
		Content: []ContentItem{NewTextContent(errMsg)},
		IsError: true,
	}
}

// NewParseError creates a standard Parse Error response (id is set to null).
func NewParseError() JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      nil,
		Error: &JSONRPCError{
			Code:    ErrCodeParseError,
			Message: "Parse error",
		},
	}
}

// NewInvalidRequestError creates a standard Invalid Request response.
func NewInvalidRequestError(id json.RawMessage, msg string) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error: &JSONRPCError{
			Code:    ErrCodeInvalidRequest,
			Message: fmt.Sprintf("Invalid Request: %s", msg),
		},
	}
}

// float64Ptr returns a pointer to a float64 value, used for Property
// minimum/maximum defaults.
func float64Ptr(v float64) *float64 {
	return &v
}
