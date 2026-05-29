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
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/service"
)

// MCPHandler handles MCP server requests.
type MCPHandler struct {
	mcpService *service.MCPService
}

// NewMCPHandler creates an MCP handler.
func NewMCPHandler(mcpService *service.MCPService) *MCPHandler {
	return &MCPHandler{
		mcpService: mcpService,
	}
}

// CreateMCPServer creates an MCP server for the current user.
func (h *MCPHandler) CreateMCPServer(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req service.CreateMCPServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	result, code, err := h.mcpService.CreateMCPServer(user.ID, req)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    result,
	})
}

// mcpErrorResponse maps the import / test sentinel errors to the response
// codes Python's mcp_api emits.
func mcpErrorResponse(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, service.ErrMCPInvalidType),
		errors.Is(err, service.ErrMCPInvalidName),
		errors.Is(err, service.ErrMCPInvalidURL):
		c.JSON(http.StatusOK, gin.H{"code": common.CodeDataError, "data": nil, "message": mcpErrorMessage(err)})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"code": common.CodeServerError, "data": nil, "message": err.Error()})
	}
	return true
}

func mcpErrorMessage(err error) string {
	switch {
	case errors.Is(err, service.ErrMCPInvalidType):
		return "Unsupported MCP server type."
	case errors.Is(err, service.ErrMCPInvalidURL):
		return "Invalid url."
	default:
		return err.Error()
	}
}

// ImportMCPRequest is the body for the bulk-import endpoint.
type ImportMCPRequest struct {
	MCPServers map[string]map[string]interface{} `json:"mcpServers"`
	Timeout    float64                           `json:"timeout,omitempty"`
}

// ImportMCPServers bulk-imports MCP servers from a JSON config, fetching the
// remote tool list for each entry and persisting it under variables.tools.
// Mirrors Python's import_multiple.
//
// @Summary Import MCP Servers
// @Tags mcp
// @Accept json
// @Produce json
// @Param request body handler.ImportMCPRequest true "import config"
// @Router /api/v1/mcp/servers/import [post]
func (h *MCPHandler) ImportMCPServers(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req ImportMCPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeBadRequest, "data": nil, "message": "Invalid request body: " + err.Error()})
		return
	}
	if len(req.MCPServers) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": common.CodeDataError, "data": nil, "message": "No MCP servers provided."})
		return
	}

	results, err := h.mcpService.ImportServers(user.ID, req.MCPServers, req.Timeout)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": common.CodeServerError, "data": nil, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": gin.H{"results": results}, "message": "success"})
}

// TestMCPServer opens a live MCP session and returns the tools the server
// advertises. The mcp_id path parameter identifies the stored record the
// user is trying to validate; the actual connection uses the request body
// so the user can preview unsaved edits — matching Python's test_mcp.
//
// @Summary Test MCP Server
// @Tags mcp
// @Accept json
// @Produce json
// @Param mcp_id path string true "MCP server ID"
// @Param request body service.TestServerRequest true "test parameters"
// @Router /api/v1/mcp/servers/{mcp_id}/test [post]
func (h *MCPHandler) TestMCPServer(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	mcpID := c.Param("mcp_id")
	if mcpID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeBadRequest, "data": nil, "message": "mcp_id is required"})
		return
	}

	var req service.TestServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeBadRequest, "data": nil, "message": "Invalid request body: " + err.Error()})
		return
	}

	tools, err := h.mcpService.TestServer(mcpID, &req)
	if mcpErrorResponse(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": tools, "message": "success"})
}
