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
	"strconv"
	"strings"

	"ragflow/internal/common"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

// MCPHandler MCP server handler
type MCPHandler struct {
	mcpService  *service.MCPService
	userService *service.UserService
}

// NewMCPHandler create MCP server handler
func NewMCPHandler(mcpService *service.MCPService, userService *service.UserService) *MCPHandler {
	return &MCPHandler{
		mcpService:  mcpService,
		userService: userService,
	}
}

// mcpErrorResponse maps service sentinel errors to the response codes used by the
// Python mcp_api, and writes the JSON response. Returns true when handled.
func mcpErrorResponse(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, service.ErrMCPNotFound),
		errors.Is(err, service.ErrMCPInvalidType),
		errors.Is(err, service.ErrMCPInvalidName),
		errors.Is(err, service.ErrMCPInvalidURL),
		errors.Is(err, service.ErrMCPDuplicateName),
		errors.Is(err, service.ErrMCPTestUnsupported):
		// Python returns these as data errors (RetCode.DATA_ERROR).
		c.JSON(http.StatusOK, gin.H{"code": common.CodeDataError, "data": nil, "message": mcpErrorMessage(err)})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"code": common.CodeServerError, "data": nil, "message": err.Error()})
	}
	return true
}

// mcpErrorMessage capitalizes sentinel messages to match the Python wording.
func mcpErrorMessage(err error) string {
	switch {
	case errors.Is(err, service.ErrMCPInvalidType):
		return "Unsupported MCP server type."
	case errors.Is(err, service.ErrMCPInvalidURL):
		return "Invalid url."
	case errors.Is(err, service.ErrMCPDuplicateName):
		return "Duplicated MCP server name."
	default:
		return err.Error()
	}
}

// parseMCPIDs reads mcp_ids / mcp_id query params, splitting comma-separated values.
// Mirrors Python's _get_mcp_ids_from_args.
func parseMCPIDs(c *gin.Context) []string {
	var ids []string
	for _, item := range c.QueryArray("mcp_ids") {
		for _, id := range strings.Split(item, ",") {
			if id != "" {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) > 0 {
		return ids
	}
	for _, id := range strings.Split(c.Query("mcp_id"), ",") {
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// ListMCPServers lists MCP servers for the tenant.
// @Summary List MCP Servers
// @Tags mcp
// @Produce json
// @Router /api/v1/mcp/servers [get]
func (h *MCPHandler) ListMCPServers(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	keywords := c.Query("keywords")
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	orderby := c.DefaultQuery("orderby", "create_time")
	desc := strings.ToLower(c.DefaultQuery("desc", "true")) != "false"
	ids := parseMCPIDs(c)

	result, err := h.mcpService.ListServers(user.ID, ids, page, pageSize, orderby, desc, keywords)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": common.CodeServerError, "data": nil, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": result, "message": "success"})
}

// GetMCPServer returns one MCP server (or its export when mode=download).
// @Summary Get MCP Server
// @Tags mcp
// @Produce json
// @Param mcp_id path string true "MCP server ID"
// @Router /api/v1/mcp/servers/{mcp_id} [get]
func (h *MCPHandler) GetMCPServer(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	mcpID := c.Param("mcp_id")
	if mcpID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeBadRequest, "data": nil, "message": "mcp_id is required"})
		return
	}

	if c.Query("mode") == "download" {
		exported, err := h.mcpService.ExportServers([]string{mcpID}, user.ID)
		if mcpErrorResponse(c, err) {
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": exported, "message": "success"})
		return
	}

	server, err := h.mcpService.GetServer(mcpID, user.ID)
	if mcpErrorResponse(c, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": server, "message": "success"})
}

// CreateMCPServer creates an MCP server for the tenant.
// @Summary Create MCP Server
// @Tags mcp
// @Accept json
// @Produce json
// @Param request body service.CreateMCPRequest true "MCP server creation parameters"
// @Router /api/v1/mcp/servers [post]
func (h *MCPHandler) CreateMCPServer(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req service.CreateMCPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeBadRequest, "data": nil, "message": "Invalid request body: " + err.Error()})
		return
	}

	server, err := h.mcpService.CreateServer(user.ID, &req)
	if mcpErrorResponse(c, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": server, "message": "success"})
}

// UpdateMCPServer updates an MCP server for the tenant.
// @Summary Update MCP Server
// @Tags mcp
// @Accept json
// @Produce json
// @Param mcp_id path string true "MCP server ID"
// @Param request body service.UpdateMCPRequest true "MCP server update parameters"
// @Router /api/v1/mcp/servers/{mcp_id} [put]
func (h *MCPHandler) UpdateMCPServer(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	mcpID := c.Param("mcp_id")
	if mcpID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeBadRequest, "data": nil, "message": "mcp_id is required"})
		return
	}

	var req service.UpdateMCPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeBadRequest, "data": nil, "message": "Invalid request body: " + err.Error()})
		return
	}

	server, err := h.mcpService.UpdateServer(mcpID, user.ID, &req)
	if mcpErrorResponse(c, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": server, "message": "success"})
}

// DeleteMCPServer deletes an MCP server for the tenant.
// @Summary Delete MCP Server
// @Tags mcp
// @Produce json
// @Param mcp_id path string true "MCP server ID"
// @Router /api/v1/mcp/servers/{mcp_id} [delete]
func (h *MCPHandler) DeleteMCPServer(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	mcpID := c.Param("mcp_id")
	if mcpID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeBadRequest, "data": nil, "message": "mcp_id is required"})
		return
	}

	err := h.mcpService.DeleteServer(mcpID, user.ID)
	if mcpErrorResponse(c, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": true, "message": "success"})
}

// ImportMCPRequest is the body for the bulk import endpoint.
type ImportMCPRequest struct {
	MCPServers map[string]map[string]interface{} `json:"mcpServers"`
}

// ImportMCPServers bulk-imports MCP servers from a JSON config.
// @Summary Import MCP Servers
// @Tags mcp
// @Accept json
// @Produce json
// @Param request body handler.ImportMCPRequest true "import config"
// @Router /api/v1/mcp/import [post]
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

	results, err := h.mcpService.ImportServers(user.ID, req.MCPServers)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": common.CodeServerError, "data": nil, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": gin.H{"results": results}, "message": "success"})
}

// TestMCPRequest is the body for the test endpoint.
type TestMCPRequest struct {
	URL        string `json:"url"`
	ServerType string `json:"server_type"`
}

// TestMCPServer connects to an MCP server and lists its tools.
// Note: the live MCP client is not yet ported to Go; this returns a data error
// (see service.TestServer). Tracked as a follow-up per issue #15275.
// @Summary Test MCP Server
// @Tags mcp
// @Accept json
// @Produce json
// @Param mcp_id path string true "MCP server ID"
// @Param request body handler.TestMCPRequest true "test parameters"
// @Router /api/v1/mcp/servers/{mcp_id}/test [post]
func (h *MCPHandler) TestMCPServer(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req TestMCPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeBadRequest, "data": nil, "message": "Invalid request body: " + err.Error()})
		return
	}

	tools, err := h.mcpService.TestServer(req.URL, req.ServerType)
	if mcpErrorResponse(c, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": tools, "message": "success"})
}
