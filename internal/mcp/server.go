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

package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Connector provides the data-access operations the MCP tools need.
// It abstracts the RAGFlow backend so that tool implementations can use
// either in-process service calls or out-of-process HTTP calls.
type Connector interface {
	// ListDatasets returns newline-delimited JSON lines, each containing
	// at minimum {"id": "...", "description": "..."}.
	ListDatasets(page, pageSize int, orderby string, desc bool) (string, error)

	// ListChats returns newline-delimited JSON lines, each containing
	// at minimum {"id": "...", "name": "...", "description": "..."}.
	ListChats(page, pageSize int, orderby string, desc bool) (string, error)

	// Retrieval executes a retrieval request and returns the result as
	// a JSON string.
	Retrieval(req RetrievalRequest) (string, error)
}

// RetrievalRequest carries all parameters for a retrieval query.
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

// Server handles MCP JSON-RPC requests.
type Server struct {
	connector Connector
	version   string
}

// NewServer creates a new MCP Server.
func NewServer(connector Connector) *Server {
	return &Server{
		connector: connector,
		version:   "1.0.0",
	}
}

// HandleRequest dispatches a raw JSON-RPC request body and returns the
// serialized JSON-RPC response. Returns nil if the request is a
// notification (no id) and requires no response.
func (s *Server) HandleRequest(body []byte) ([]byte, bool, error) {
	// Try to decode as a request (with an id) first.
	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		resp := NewParseError()
		data, _ := json.Marshal(resp)
		return data, true, nil
	}

	// Notifications have no id — do not send a response.
	if req.ID == nil || string(req.ID) == "null" {
		return nil, false, nil
	}

	// Validate jsonrpc field.
	if req.JSONRPC != JSONRPCVersion {
		resp := NewInvalidRequestError(req.ID, "jsonrpc must be \"2.0\"")
		data, _ := json.Marshal(resp)
		return data, true, nil
	}

	var resp JSONRPCResponse

	switch req.Method {
	case "initialize":
		resp = s.handleInitialize(req.ID)
	case "tools/list":
		resp = s.handleListTools(req.ID)
	case "tools/call":
		resp = s.handleCallTool(req.ID, req.Params)
	case "ping":
		resp = s.handlePing(req.ID)
	default:
		resp = NewErrorResponse(req.ID, ErrCodeMethodNotFound,
			fmt.Sprintf("Method not found: %s", req.Method))
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return nil, true, fmt.Errorf("failed to marshal response: %w", err)
	}
	return data, true, nil
}

func (s *Server) handleInitialize(id json.RawMessage) JSONRPCResponse {
	result := InitializeResult{
		ProtocolVersion: MCPProtocolVersion,
		Capabilities: Capabilities{
			Tools: &ToolsCapability{ListChanged: false},
		},
		ServerInfo: ServerInfo{
			Name:    ServerName,
			Version: s.version,
		},
	}
	return NewSuccessResponse(id, result)
}

func (s *Server) handlePing(id json.RawMessage) JSONRPCResponse {
	return NewSuccessResponse(id, struct{}{})
}

func (s *Server) handleListTools(id json.RawMessage) JSONRPCResponse {
	// Fetch dataset and chat descriptions for embedding into tool descriptions,
	// matching the Python MCP server behavior.
	datasetDescription, err := s.connector.ListDatasets(1, 100, "create_time", true)
	if err != nil {
		datasetDescription = ""
	}
	chatDescription, err := s.connector.ListChats(1, 30, "create_time", true)
	if err != nil {
		chatDescription = ""
	}

	tools := []Tool{
		{
			Name:        "ragflow_retrieval",
			Description: "Retrieve relevant chunks from the RAGFlow retrieve interface based on the question. You can optionally specify dataset_ids to search only specific datasets, or omit dataset_ids entirely to search across ALL available datasets. You can also optionally specify document_ids to search within specific documents. When dataset_ids is not provided or is empty, the system will automatically search across all available datasets. Below is the list of all available datasets, including their descriptions and IDs:" + datasetDescription,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"dataset_ids": {
						Type:        "array",
						Description: "Optional array of dataset IDs to search. If not provided or empty, all datasets will be searched.",
						Items:       &Items{Type: "string"},
					},
					"document_ids": {
						Type:        "array",
						Description: "Optional array of document IDs to search within.",
						Items:       &Items{Type: "string"},
					},
					"question": {
						Type:        "string",
						Description: "The question or query to search for.",
					},
					"page": {
						Type:        "integer",
						Description: "Page number for pagination",
						Default:     1,
						Minimum:     float64Ptr(1),
					},
					"page_size": {
						Type:        "integer",
						Description: "Number of results to return per page (default: 10, max recommended: 50 to avoid token limits)",
						Default:     10,
						Minimum:     float64Ptr(1),
						Maximum:     float64Ptr(100),
					},
					"similarity_threshold": {
						Type:        "number",
						Description: "Minimum similarity threshold for results",
						Default:     0.2,
						Minimum:     float64Ptr(0),
						Maximum:     float64Ptr(1),
					},
					"vector_similarity_weight": {
						Type:        "number",
						Description: "Weight for vector similarity vs term similarity",
						Default:     0.3,
						Minimum:     float64Ptr(0),
						Maximum:     float64Ptr(1),
					},
					"keyword": {
						Type:        "boolean",
						Description: "Enable keyword-based search",
						Default:     false,
					},
					"top_k": {
						Type:        "integer",
						Description: "Maximum results to consider before ranking",
						Default:     1024,
						Minimum:     float64Ptr(1),
						Maximum:     float64Ptr(1024),
					},
					"rerank_id": {
						Type:        "string",
						Description: "Optional reranking model identifier",
					},
					"force_refresh": {
						Type:        "boolean",
						Description: "Set to true only if fresh dataset and document metadata is explicitly required. Otherwise, cached metadata is used (default: false).",
						Default:     false,
					},
				},
				Required: []string{"question"},
			},
		},
		{
			Name:        "ragflow_list_datasets",
			Description: "List all accessible datasets (knowledge bases) in RAGFlow. Returns dataset IDs, names, and descriptions. Use this tool to discover which datasets are available before performing retrieval." + datasetDescription,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"page": {
						Type:        "integer",
						Description: "Page number",
						Default:     1,
						Minimum:     float64Ptr(1),
					},
					"page_size": {
						Type:        "integer",
						Description: "Results per page",
						Default:     100,
						Minimum:     float64Ptr(1),
						Maximum:     float64Ptr(1000),
					},
				},
			},
		},
		{
			Name:        "ragflow_list_chats",
			Description: "List all accessible chat assistants in RAGFlow. Returns chat assistant IDs, names, and descriptions. Use this tool to discover available chat assistants that can be used for conversations." + chatDescription,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"page": {
						Type:        "integer",
						Description: "Page number",
						Default:     1,
						Minimum:     float64Ptr(1),
					},
					"page_size": {
						Type:        "integer",
						Description: "Results per page",
						Default:     30,
						Minimum:     float64Ptr(1),
						Maximum:     float64Ptr(100),
					},
				},
			},
		},
	}

	return NewSuccessResponse(id, ListToolsResult{Tools: tools})
}

func (s *Server) handleCallTool(id json.RawMessage, rawParams json.RawMessage) JSONRPCResponse {
	var params CallToolParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return NewErrorResponse(id, ErrCodeInvalidParams,
			fmt.Sprintf("Invalid params: %s", err.Error()))
	}

	if params.Arguments == nil {
		params.Arguments = make(map[string]interface{})
	}

	switch params.Name {
	case "ragflow_retrieval":
		return s.callRagflowRetrieval(id, params.Arguments)
	case "ragflow_list_datasets":
		return s.callListDatasets(id, params.Arguments)
	case "ragflow_list_chats":
		return s.callListChats(id, params.Arguments)
	default:
		return NewErrorResponse(id, ErrCodeMethodNotFound,
			fmt.Sprintf("Tool not found: %s", params.Name))
	}
}

func (s *Server) callRagflowRetrieval(id json.RawMessage, args map[string]interface{}) JSONRPCResponse {
	req := RetrievalRequest{
		Page:                   getBoundedIntArg(args, "page", 1, 1, 1_000_000),
		PageSize:               getBoundedIntArg(args, "page_size", 10, 1, 100),
		SimilarityThreshold:    getBoundedFloat64Arg(args, "similarity_threshold", 0.2, 0, 1),
		VectorSimilarityWeight: getBoundedFloat64Arg(args, "vector_similarity_weight", 0.3, 0, 1),
		Keyword:                getBoolArg(args, "keyword", false),
		TopK:                   getBoundedIntArg(args, "top_k", 1024, 1, 1024),
		ForceRefresh:           getBoolArg(args, "force_refresh", false),
		Question:               getStringArg(args, "question", ""),
		RerankID:               getStringArg(args, "rerank_id", ""),
	}

	if v, ok := args["dataset_ids"]; ok {
		req.DatasetIDs = toStringSlice(v)
	}
	if v, ok := args["document_ids"]; ok {
		req.DocumentIDs = toStringSlice(v)
	}

	if strings.TrimSpace(req.Question) == "" {
		return NewSuccessResponse(id, NewErrorResult("question is required"))
	}

	result, err := s.connector.Retrieval(req)
	if err != nil {
		return NewSuccessResponse(id, NewErrorResult(err.Error()))
	}
	return NewSuccessResponse(id, NewTextResult(result))
}

func (s *Server) callListDatasets(id json.RawMessage, args map[string]interface{}) JSONRPCResponse {
	page := getBoundedIntArg(args, "page", 1, 1, 1_000_000)
	pageSize := getBoundedIntArg(args, "page_size", 100, 1, 1000)

	result, err := s.connector.ListDatasets(page, pageSize, "create_time", true)
	if err != nil {
		return NewSuccessResponse(id, NewErrorResult(err.Error()))
	}
	return NewSuccessResponse(id, NewTextResult(result))
}

func (s *Server) callListChats(id json.RawMessage, args map[string]interface{}) JSONRPCResponse {
	page := getBoundedIntArg(args, "page", 1, 1, 1_000_000)
	pageSize := getBoundedIntArg(args, "page_size", 30, 1, 100)

	result, err := s.connector.ListChats(page, pageSize, "create_time", true)
	if err != nil {
		return NewSuccessResponse(id, NewErrorResult(err.Error()))
	}
	return NewSuccessResponse(id, NewTextResult(result))
}

// --- argument extraction helpers ---

func getStringArg(args map[string]interface{}, key, defaultVal string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultVal
}

func getIntArg(args map[string]interface{}, key string, defaultVal int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case int64:
			return int(n)
		case json.Number:
			if i, err := n.Int64(); err == nil {
				return int(i)
			}
		}
	}
	return defaultVal
}

func getBoundedIntArg(args map[string]interface{}, key string, defaultVal, minVal, maxVal int) int {
	v := getIntArg(args, key, defaultVal)
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

func getFloat64Arg(args map[string]interface{}, key string, defaultVal float64) float64 {
	if v, ok := args[key]; ok {
		switch f := v.(type) {
		case float64:
			return f
		case int:
			return float64(f)
		case int64:
			return float64(f)
		case json.Number:
			if fl, err := f.Float64(); err == nil {
				return fl
			}
		}
	}
	return defaultVal
}

func getBoundedFloat64Arg(args map[string]interface{}, key string, defaultVal, minVal, maxVal float64) float64 {
	v := getFloat64Arg(args, key, defaultVal)
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

func getBoolArg(args map[string]interface{}, key string, defaultVal bool) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
		if s, ok := v.(string); ok {
			switch strings.ToLower(s) {
			case "true", "1", "yes":
				return true
			case "false", "0", "no":
				return false
			}
		}
	}
	return defaultVal
}

// toStringSlice converts an interface{} value (expected to be a JSON array)
// into a []string. Values are converted via fmt.Sprintf.
func toStringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		result = append(result, fmt.Sprintf("%v", item))
	}
	return result
}
