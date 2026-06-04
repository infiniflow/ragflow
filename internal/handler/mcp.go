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
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

const (
	defaultMCPServerPage     = 0
	defaultMCPServerPageSize = 0
	maxMCPServerPageSize     = 100
	mcpServerDateFormat      = "2006-01-02T15:04:05"
)

// MCPHandler handles MCP server requests.
type MCPHandler struct {
	mcpService *service.MCPService
}

type mcpServerResponse struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	TenantID    string                 `json:"tenant_id"`
	URL         string                 `json:"url"`
	ServerType  string                 `json:"server_type"`
	Description *string                `json:"description"`
	Variables   map[string]interface{} `json:"variables"`
	Headers     map[string]interface{} `json:"headers"`
	CreateTime  *int64                 `json:"create_time"`
	CreateDate  string                 `json:"create_date"`
	UpdateTime  *int64                 `json:"update_time"`
	UpdateDate  string                 `json:"update_date"`
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

	page, err := parseMCPServerPage(c.Query("page"))
	if err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}
	pageSize, err := parseMCPServerPageSize(c.Query("page_size"))
	if err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	orderby := c.DefaultQuery("orderby", "create_time")
	desc := strings.ToLower(c.DefaultQuery("desc", "true")) != "false"
	keywords := c.Query("keywords")
	mcpIDs := getMCPIDsFromQuery(c)

	result, code, err := h.mcpService.ListMCPServers(user.ID, mcpIDs, keywords, page, pageSize, orderby, desc)
	if err != nil {
		if code == common.CodeServerError {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    code,
				"message": err.Error(),
				"data":    nil,
			})
			return
		}
		jsonError(c, code, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    result,
	})
}

// UpdateMCPServer updates an MCP server for the current user.
func (h *MCPHandler) UpdateMCPServer(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	mcpID := c.Param("mcp_id")
	var req service.UpdateMCPServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	result, code, err := h.mcpService.UpdateMCPServer(user.ID, mcpID, req)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    newMCPServerResponse(result),
	})
}

// DeleteMCPServer deletes an MCP server for the current user.
func (h *MCPHandler) DeleteMCPServer(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	result, code, err := h.mcpService.DeleteMCPServer(user.ID, c.Param("mcp_id"))
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

func newMCPServerResponse(server *entity.MCPServer) *mcpServerResponse {
	if server == nil {
		return nil
	}

	return &mcpServerResponse{
		ID:          server.ID,
		Name:        server.Name,
		TenantID:    server.TenantID,
		URL:         server.URL,
		ServerType:  server.ServerType,
		Description: server.Description,
		Variables:   map[string]interface{}(server.Variables),
		Headers:     map[string]interface{}(server.Headers),
		CreateTime:  server.CreateTime,
		CreateDate:  formatMCPServerDate(server.CreateDate),
		UpdateTime:  server.UpdateTime,
		UpdateDate:  formatMCPServerDate(server.UpdateDate),
	}
}

func formatMCPServerDate(date *time.Time) string {
	if date == nil {
		return ""
	}
	return date.Format(mcpServerDateFormat)
}

func parseMCPServerPage(value string) (int, error) {
	if value == "" {
		return defaultMCPServerPage, nil
	}
	page, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("page must be an integer")
	}
	return page, nil
}

func parseMCPServerPageSize(value string) (int, error) {
	if value == "" {
		return defaultMCPServerPageSize, nil
	}
	pageSize, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("page_size must be an integer")
	}
	if pageSize > maxMCPServerPageSize {
		return 0, fmt.Errorf("page_size must be less than or equal to %d", maxMCPServerPageSize)
	}
	return pageSize, nil
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
