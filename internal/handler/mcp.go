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
	"net/http"
	"strconv"
	"strings"

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

// ListMCPServers lists MCP servers for the current user.
func (h *MCPHandler) ListMCPServers(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	page := parseMCPPositiveInt(c.Query("page"))
	pageSize := parseMCPPositiveInt(c.Query("page_size"))
	orderby := c.DefaultQuery("orderby", "create_time")
	desc := strings.ToLower(c.DefaultQuery("desc", "true")) != "false"
	keywords := c.Query("keywords")
	mcpIDs := getMCPIDsFromQuery(c)

	result, err := h.mcpService.ListMCPServers(user.ID, mcpIDs, keywords, page, pageSize, orderby, desc)
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    result,
	})
}

// GetMCPServer gets one MCP server for the current user.
func (h *MCPHandler) GetMCPServer(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	mcpID := c.Param("mcp_id")
	if c.Query("mode") == "download" {
		h.downloadMCPServer(c, user.ID, mcpID)
		return
	}

	result, found, err := h.mcpService.GetMCPServer(user.ID, mcpID)
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}
	if !found {
		mcpNotFound(c, mcpID, user.ID)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    result,
	})
}

func (h *MCPHandler) downloadMCPServer(c *gin.Context, tenantID, mcpID string) {
	result, found, err := h.mcpService.ExportMCPServer(tenantID, mcpID)
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}
	if !found {
		mcpNotFound(c, mcpID, tenantID)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    result,
	})
}

func mcpNotFound(c *gin.Context, mcpID, tenantID string) {
	jsonError(c, common.CodeDataError, "Cannot find MCP server "+mcpID+" for user "+tenantID)
}

func parseMCPPositiveInt(value string) int {
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func getMCPIDsFromQuery(c *gin.Context) []string {
	rawValues := c.QueryArray("mcp_ids")
	if len(rawValues) == 0 {
		rawValues = []string{c.Query("mcp_id")}
	}

	ids := make([]string, 0)
	for _, rawValue := range rawValues {
		for _, item := range strings.Split(rawValue, ",") {
			id := strings.TrimSpace(item)
			if id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids
}
