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
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/mcp"
	"ragflow/internal/server/config"
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

// NewMCPServerHandler shares the service callbacks between API and standalone transports.
func NewMCPServerHandler(ds *dataset.DatasetService, chats *service.ChatService) *MCPServerHandler {
	return &MCPServerHandler{
		listDatasetsFunc: func(ctx context.Context, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error) {
			return mcpListDatasets(ctx, ds, userID, page, pageSize, orderby, desc)
		},
		listChatsFunc: func(ctx context.Context, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error) {
			return mcpListChats(ctx, chats, userID, page, pageSize, orderby, desc)
		},
		retrievalFunc: func(ctx context.Context, userID string, req mcp.RetrievalRequest) (string, error) {
			return mcpRetrieval(ctx, ds, userID, req)
		},
	}
}

func (h *MCPServerHandler) newTransport(resolveUser func(context.Context, string) (string, error), opts mcp.Options) *mcp.Handler {
	return mcp.NewHandler(resolveUser, func(userID string) mcp.Connector {
		return mcp.NewServiceConnector(userID, h.listDatasetsFunc, h.listChatsFunc, h.retrievalFunc)
	}, opts)
}

// NewStandalone validates the configured identity before creating transport sessions.
// Self-host mode intentionally gives trusted clients the configured user's identity;
// host mode resolves each request's credentials independently.
func (h *MCPServerHandler) NewStandalone(ctx context.Context, auth *AuthHandler, cfg config.MCPConfig) (*mcp.Handler, error) {
	resolveUser := func(ctx context.Context, authorization string) (string, error) {
		if cfg.LaunchMode == "self-host" {
			authorization = cfg.HostAPIKey
		}
		user, err := auth.ResolveMCPUser(ctx, authorization)
		if err != nil {
			return "", err
		}
		return user.ID, nil
	}
	if cfg.LaunchMode == "self-host" {
		authCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		_, err := resolveUser(authCtx, "")
		cancel()
		if err != nil {
			return nil, errors.New("invalid configured MCP API key")
		}
	}
	return h.newTransport(resolveUser, mcp.Options{SSE: cfg.SSE, StreamableHTTP: cfg.StreamableHTTP, JSONResponse: cfg.JSONResponse}), nil
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

	hdl := h.newTransport(func(context.Context, string) (string, error) { return user.ID, nil }, mcp.Options{StreamableHTTP: true, JSONResponse: true})
	defer hdl.Close()
	request := c.Request.Clone(c.Request.Context())
	request.URL.Path = "/mcp"
	// Preserve the legacy REST contract: callers historically sent ordinary
	// JSON without Streamable HTTP's dual Accept header.
	if accept := request.Header.Get("Accept"); accept == "" || accept == "application/json" {
		request.Header.Set("Accept", "application/json, text/event-stream")
	}
	hdl.ServeHTTP(c.Writer, request)
}

// mcpListDatasets wraps DatasetService.ListDatasets for the MCP tool handler,
// filling in default values for parameters that the MCP tool does not expose.
func mcpListDatasets(ctx context.Context, ds *dataset.DatasetService, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error) {
	data, total, _, err := ds.ListDatasets(ctx,
		"", "", page, pageSize, []dao.OrderTerm{{Column: orderby, Desc: desc}},
		"", nil, "", userID, nil,
	)
	return data, total, err
}

// mcpListChats wraps ChatService.ListChats for the MCP tool handler,
// converting the typed response into a generic []map[string]interface{}.
func mcpListChats(ctx context.Context, chatService *service.ChatService, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error) {
	resp, err := chatService.ListChats(ctx, userID, "1", "", "", "", page, pageSize, []dao.OrderTerm{{Column: orderby, Desc: desc}}, nil)
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
