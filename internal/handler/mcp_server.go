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

package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/mcp"
	"ragflow/internal/service"
)

// MCPRetrievalService abstracts the dataset retrieval operations needed
// by the MCP server handler.
type MCPRetrievalService interface {
	SearchDatasets(req *service.SearchDatasetsRequest, userID string) (*service.SearchDatasetsResponse, error)
	ListDatasets(id, name string, page, pageSize int, orderby string, desc bool, keywords string, ownerIDs []string, parserID, userID string) ([]map[string]interface{}, int64, common.ErrorCode, error)
}

// MCPServerHandler handles MCP protocol requests (JSON-RPC over HTTP).
// It exposes RAGFlow capabilities as MCP tools to external AI clients.
type MCPServerHandler struct {
	listDatasetsFunc func(userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error)
	listChatsFunc    func(userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error)
	retrievalFunc    func(userID string, req mcp.RetrievalRequest) (string, error)
}

// NewMCPServerHandler creates a new MCPServerHandler.
// The service functions are passed as closures to avoid importing the service
// package directly from the handler layer.
func NewMCPServerHandler(
	listDatasetsFunc func(userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error),
	listChatsFunc func(userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error),
	retrievalFunc func(userID string, req mcp.RetrievalRequest) (string, error),
) *MCPServerHandler {
	return &MCPServerHandler{
		listDatasetsFunc: listDatasetsFunc,
		listChatsFunc:    listChatsFunc,
		retrievalFunc:    retrievalFunc,
	}
}

// HandleMCP is the Gin handler for the MCP endpoint. It reads the JSON-RPC
// request body, creates a connector for the authenticated user, and returns
// the JSON-RPC response. The endpoint is placed behind BetaAuthMiddleware
// so the user is already resolved from the Authorization header.
//
// @Summary MCP Endpoint (JSON-RPC over HTTP)
// @Tags mcp
// @Accept json
// @Produce json
// @Router /api/v1/mcp [post]
func (h *MCPServerHandler) HandleMCP(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	const maxMCPBodyBytes = 1 << 20 // 1 MiB
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxMCPBodyBytes)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Failed to read request body: "+err.Error())
		return
	}

	// Create a connector for this user. Each request gets its own connector
	// so that user context is always correct.
	connector := mcp.NewServiceConnector(
		user.ID,
		h.listDatasetsFunc,
		h.listChatsFunc,
		h.retrievalFunc,
	)

	server := mcp.NewServer(connector)
	respBody, hasResponse, err := server.HandleRequest(body)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, common.CodeBadRequest, nil, "MCP server error: "+err.Error())
		return
	}

	if !hasResponse {
		// Notification — no response per JSON-RPC spec.
		c.Status(http.StatusAccepted)
		return
	}

	// The MCP protocol uses application/json with JSON-RPC responses.
	c.Data(http.StatusOK, "application/json", respBody)
}

// MCPListDatasets wraps DatasetService.ListDatasets for the MCP tool handler,
// filling in default values for parameters that the MCP tool does not expose.
func MCPListDatasets(ds *service.DatasetService, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error) {
	data, total, _, err := ds.ListDatasets(
		"", "", page, pageSize, orderby, desc,
		"", nil, "", userID,
	)
	return data, total, err
}

// MCPListChats wraps ChatService.ListChats for the MCP tool handler,
// converting the typed response into a generic []map[string]interface{}.
func MCPListChats(cs *service.ChatService, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error) {
	resp, err := cs.ListChats(userID, "1", "", page, pageSize, orderby, desc)
	if err != nil {
		return nil, 0, err
	}
	var chatList []map[string]interface{}
	for _, chat := range resp.Chats {
		chatList = append(chatList, map[string]interface{}{
			"id":          chat.ID,
			"name":        chat.Name,
			"description": chat.Description,
		})
	}
	return chatList, resp.Total, nil
}

// MCPRetrieval executes a retrieval request on behalf of the MCP tool handler.
// It translates the mcp.RetrievalRequest into a service.SearchDatasetsRequest
// and calls DatasetService.SearchDatasets. The result is serialized as JSON.
func MCPRetrieval(ds *service.DatasetService, userID string, req mcp.RetrievalRequest) (string, error) {
	// Resolve dataset IDs: if none provided, fetch ALL accessible datasets
	// across all pages (matching Python _fetch_all_datasets behaviour).
	datasetIDs := req.DatasetIDs
	if len(datasetIDs) == 0 {
		const maxPageSize = 100
		page := 1
		for {
			data, _, _, err := ds.ListDatasets(
				"", "", page, maxPageSize, "create_time", true,
				"", nil, "", userID,
			)
			if err != nil {
				return "", fmt.Errorf("cannot resolve accessible datasets: %w", err)
			}
			if len(data) == 0 {
				break
			}
			for _, d := range data {
				if id, ok := d["id"].(string); ok && id != "" {
					datasetIDs = append(datasetIDs, id)
				}
			}
			// A page smaller than maxPageSize is the last page.
			if len(data) < maxPageSize {
				break
			}
			page++
		}
		if len(datasetIDs) == 0 {
			return "", fmt.Errorf("no accessible datasets found")
		}
	}

	searchReq := &service.SearchDatasetsRequest{
		DatasetIDs:   datasetIDs,
		Question:     req.Question,
		DocIDs:       req.DocumentIDs,
		ForceRefresh: req.ForceRefresh,
	}

	if req.Page > 0 {
		v := req.Page
		searchReq.Page = &v
	}
	if req.PageSize > 0 {
		v := req.PageSize
		searchReq.Size = &v
	}
	if req.TopK > 0 {
		v := req.TopK
		searchReq.TopK = &v
	}
	{
		v := req.SimilarityThreshold
		searchReq.SimilarityThreshold = &v
	}
	{
		v := req.VectorSimilarityWeight
		searchReq.VectorSimilarityWeight = &v
	}
	if req.RerankID != "" {
		v := req.RerankID
		searchReq.RerankID = &v
	}
	{
		v := req.Keyword
		searchReq.Keyword = &v
	}

	resp, err := ds.SearchDatasets(searchReq, userID)
	if err != nil {
		return "", err
	}

	result, err := json.Marshal(resp)
	if err != nil {
		return "", fmt.Errorf("failed to serialize retrieval result: %w", err)
	}
	return string(result), nil
}
