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
	"errors"
	"fmt"
	"io"
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req service.CreateMCPServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	result, code, err := h.mcpService.CreateMCPServer(user.ID, req)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

// ListMCPServers lists MCP servers for the current user.
func (h *MCPHandler) ListMCPServers(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	page, err := parseMCPServerPage(c.Query("page"))
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}
	pageSize, err := parseMCPServerPageSize(c.Query("page_size"))
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	orderby := c.DefaultQuery("orderby", "create_time")
	desc := strings.ToLower(c.DefaultQuery("desc", "true")) != "false"
	keywords := c.Query("keywords")
	mcpIDs := getMCPIDsFromQuery(c)

	result, code, err := h.mcpService.ListMCPServers(user.ID, mcpIDs, keywords, page, pageSize, orderby, desc)
	if err != nil {
		if code == common.CodeServerError {
			common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, code, nil, err.Error())
			return
		}
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

func (h *MCPHandler) GetMCPServer(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	mcpID := c.Param("mcp_id")
	if c.Query("mode") == "download" {
		result, code, err := h.mcpService.ExportMCPServer(user.ID, mcpID)
		if err != nil {
			mcpDetailError(c, code, err)
			return
		}
		common.SuccessWithData(c, result, "success")
		return
	}

	result, code, err := h.mcpService.GetMCPServer(user.ID, mcpID)
	if err != nil {
		mcpDetailError(c, code, err)
		return
	}
	common.SuccessWithData(c, newMCPServerResponse(result), "success")
}

func mcpDetailError(c *gin.Context, code common.ErrorCode, err error) {
	if code == common.CodeDataError {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}
	common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
}

// UpdateMCPServer updates an MCP server for the current user.
func (h *MCPHandler) UpdateMCPServer(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	mcpID := c.Param("mcp_id")
	var req service.UpdateMCPServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	result, code, err := h.mcpService.UpdateMCPServer(user.ID, mcpID, req)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithData(c, newMCPServerResponse(result), "success")
}

// DeleteMCPServer deletes an MCP server for the current user.
func (h *MCPHandler) DeleteMCPServer(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	result, code, err := h.mcpService.DeleteMCPServer(user.ID, c.Param("mcp_id"))
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
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
		errors.Is(err, service.ErrMCPInvalidURL),
		errors.Is(err, service.ErrMCPTestFailed):
		common.ResponseWithCodeData(c, common.CodeDataError, nil, mcpErrorMessage(err))
	default:
		common.ResponseWithCodeData(c, common.CodeServerError, nil, err.Error())
	}
	return true
}

func mcpErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	// service wraps its sentinels as "<sentinel>: <detail>" via
	// fmt.Errorf("%w: ...", err). Surface the detail when present so the
	// SSRF guard's per-failure message (e.g. "URL resolves to a non-public
	// address (...).") reaches the caller verbatim, matching what Python's
	// _assert_mcp_url_is_safe returns.
	switch {
	case errors.Is(err, service.ErrMCPInvalidURL):
		if detail := unwrapDetail(err, service.ErrMCPInvalidURL); detail != "" {
			return detail
		}
		return "Invalid url."
	case errors.Is(err, service.ErrMCPInvalidType):
		return "Unsupported MCP server type."
	case errors.Is(err, service.ErrMCPTestFailed):
		if detail := unwrapDetail(err, service.ErrMCPTestFailed); detail != "" {
			return detail
		}
		return "Test MCP error."
	default:
		return err.Error()
	}
}

// unwrapDetail pulls the "<sentinel>: <detail>" suffix off a wrapped error
// and returns the detail. Returns "" when the error is the bare sentinel
// (no wrapped message) so the caller can fall back to a default.
func unwrapDetail(err, sentinel error) string {
	if err == nil || sentinel == nil {
		return ""
	}
	prefix := sentinel.Error() + ": "
	msg := err.Error()
	if !strings.HasPrefix(msg, prefix) {
		return ""
	}
	return strings.TrimPrefix(msg, prefix)
}

// ImportMCPRequest is the body for the bulk-import endpoint.
type ImportMCPRequest struct {
	MCPServers map[string]map[string]interface{} `json:"mcpServers"`
	Timeout    float64                           `json:"timeout,omitempty"`
}

// ImportMCPServers bulk-imports MCP servers from a JSON config
// @Summary Import MCP Servers
// @Tags mcp
// @Accept json
// @Produce json
// @Param request body handler.ImportMCPRequest true "import config"
// @Router /api/v1/mcp/servers/import [post]
func (h *MCPHandler) ImportMCPServers(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Invalid request body: "+err.Error())
		return
	}
	var raw map[string]json.RawMessage
	if len(body) > 0 {
		if err = json.Unmarshal(body, &raw); err != nil {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Invalid request body: "+err.Error())
			return
		}
	}

	rawServers, hasServers := raw["mcpServers"]
	if !hasServers {
		// Match Python validate_request: code 101, message includes the
		// trailing "; " separator the Python decorator emits.
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "required argument are missing: mcpServers; ")
		return
	}

	var servers map[string]map[string]interface{}
	if err = json.Unmarshal(rawServers, &servers); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Invalid request body: "+err.Error())
		return
	}
	if len(servers) == 0 {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "No MCP servers provided.")
		return
	}

	var timeout float64
	if rawTimeout, ok := raw["timeout"]; ok {
		// Ignore parse errors for timeout to match Python's get_float
		// default-on-failure behavior; the service applies its own
		// 10 s fallback when timeout <= 0.
		_ = json.Unmarshal(rawTimeout, &timeout)
	}

	results, err := h.mcpService.ImportServers(user.ID, servers, timeout)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, common.CodeBadRequest, nil, err.Error())
		return
	}

	common.SuccessWithData(c, gin.H{"results": results}, "success")
}

// TestMCPServer opens a live MCP session and returns the tools the server advertises.
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	mcpID := c.Param("mcp_id")
	if mcpID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "mcp_id is required")
		return
	}

	var req service.TestServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Invalid request body: "+err.Error())
		return
	}

	var missingFields []string
	if req.URL == "" {
		missingFields = append(missingFields, "url")
	}
	if req.ServerType == "" {
		missingFields = append(missingFields, "server_type")
	}
	if len(missingFields) > 0 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "required argument are missing: "+strings.Join(missingFields, ", ")+"; ")
		return
	}

	tools, err := h.mcpService.TestServer(mcpID, &req)
	if mcpErrorResponse(c, err) {
		return
	}
	common.SuccessWithData(c, tools, "success")
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
