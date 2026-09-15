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
	"context"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/mcp"
	"ragflow/internal/service"
	dataset "ragflow/internal/service/dataset"
)

// MCPServerHandler handles MCP protocol requests (JSON-RPC over HTTP).
// It exposes RAGFlow capabilities as MCP tools to external AI clients.
type MCPServerHandler struct {
	listDatasetsFunc func(ctx context.Context, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error)
	listChatsFunc    func(ctx context.Context, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error)
	retrievalFunc    func(ctx context.Context, userID string, req mcp.RetrievalRequest) (string, error)
}

// NewStandaloneMCPHandler exposes the native SDK transports.
func NewStandaloneMCPHandler(
	resolveUser func(context.Context, string) (string, error),
	listDatasetsFunc func(context.Context, string, int, int, string, bool) ([]map[string]interface{}, int64, error),
	listChatsFunc func(context.Context, string, int, int, string, bool) ([]map[string]interface{}, int64, error),
	retrievalFunc func(context.Context, string, mcp.RetrievalRequest) (string, error),
	opts mcp.Options,
) *mcp.Handler {
	return mcp.NewHandler(resolveUser, func(userID string) mcp.Connector {
		return mcp.NewServiceConnector(userID, listDatasetsFunc, listChatsFunc, retrievalFunc)
	}, opts)
}

// NewMCPServerHandler creates a new MCPServerHandler.
// The service functions are passed as closures to avoid importing the service
// package directly from the handler layer.
func NewMCPServerHandler(
	listDatasetsFunc func(ctx context.Context, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error),
	listChatsFunc func(ctx context.Context, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error),
	retrievalFunc func(ctx context.Context, userID string, req mcp.RetrievalRequest) (string, error),
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
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	hdl := NewStandaloneMCPHandler(func(context.Context, string) (string, error) { return user.ID, nil }, h.listDatasetsFunc, h.listChatsFunc, h.retrievalFunc, mcp.Options{StreamableHTTP: true, JSONResponse: true})
	defer hdl.Close()
	request := c.Request.Clone(c.Request.Context())
	request.URL.Path = "/mcp"
	hdl.ServeHTTP(c.Writer, request)
}

// MCPListDatasets wraps DatasetService.ListDatasets for the MCP tool handler,
// filling in default values for parameters that the MCP tool does not expose.
func MCPListDatasets(ctx context.Context, ds *dataset.DatasetService, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error) {
	data, total, _, err := ds.ListDatasets(ctx,
		"", "", page, pageSize, orderby, desc,
		"", nil, "", userID, nil,
	)
	return data, total, err
}

// MCPListChats wraps ChatService.ListChats for the MCP tool handler,
// converting the typed response into a generic []map[string]interface{}.
func MCPListChats(ctx context.Context, chatService *service.ChatService, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error) {
	resp, err := chatService.ListChats(ctx, userID, "1", "", page, pageSize, orderby, desc, nil)
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
