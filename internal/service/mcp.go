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

package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

const (
	mcpServerTypeSSE            = "sse"
	mcpServerTypeStreamableHTTP = "streamable-http"
	mcpServerNameLimit          = 255
	mcpServerDateFormat         = "2006-01-02T15:04:05"
)

// MCPService handles MCP server operations.
type MCPService struct {
	mcpServerDAO *dao.MCPServerDAO
	tenantDAO    *dao.TenantDAO
}

// NewMCPService creates an MCP service.
func NewMCPService() *MCPService {
	return &MCPService{
		mcpServerDAO: dao.NewMCPServerDAO(),
		tenantDAO:    dao.NewTenantDAO(),
	}
}

// CreateMCPServerRequest is the request payload for creating an MCP server.
type CreateMCPServerRequest struct {
	Name        string          `json:"name"`
	URL         string          `json:"url"`
	ServerType  string          `json:"server_type"`
	Description *string         `json:"description,omitempty"`
	Variables   json.RawMessage `json:"variables,omitempty"`
	Headers     json.RawMessage `json:"headers,omitempty"`
}

// CreateMCPServerResponse is the response payload for creating an MCP server.
type CreateMCPServerResponse struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	Name        string         `json:"name"`
	URL         string         `json:"url"`
	ServerType  string         `json:"server_type"`
	Description *string        `json:"description,omitempty"`
	Variables   entity.JSONMap `json:"variables"`
	Headers     entity.JSONMap `json:"headers"`
}

// UpdateMCPServerRequest is the raw request payload for updating an MCP server.
type UpdateMCPServerRequest map[string]json.RawMessage

// MCPServerListItem is an MCP server item in the list response.
type MCPServerListItem struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	ServerType  string         `json:"server_type"`
	URL         string         `json:"url"`
	Description *string        `json:"description"`
	Variables   entity.JSONMap `json:"variables"`
	CreateDate  *string        `json:"create_date"`
	UpdateDate  *string        `json:"update_date"`
}

// ListMCPServersResponse is the response payload for listing MCP servers.
type ListMCPServersResponse struct {
	MCPServers []*MCPServerListItem `json:"mcp_servers"`
	Total      int64                `json:"total"`
}

// CreateMCPServer creates an MCP server owned by a tenant.
func (s *MCPService) CreateMCPServer(tenantID string, req CreateMCPServerRequest) (*CreateMCPServerResponse, common.ErrorCode, error) {
	if !isValidMCPServerType(req.ServerType) {
		return nil, common.CodeDataError, errors.New("Unsupported MCP server type.")
	}

	if req.Name == "" || len([]byte(req.Name)) > mcpServerNameLimit {
		return nil, common.CodeDataError, fmt.Errorf("Invalid MCP name or length is %d which is large than 255.", len([]byte(req.Name)))
	}

	exists, err := s.mcpServerDAO.ExistsByNameAndTenant(req.Name, tenantID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if exists {
		return nil, common.CodeDataError, errors.New("Duplicated MCP server name.")
	}

	if req.URL == "" {
		return nil, common.CodeDataError, errors.New("Invalid url.")
	}

	if _, err := s.tenantDAO.GetByID(tenantID); err != nil {
		return nil, common.CodeDataError, errors.New("Tenant not found.")
	}

	headers := safeJSONMap(req.Headers)
	variables := safeJSONMap(req.Variables)
	delete(variables, "tools")
	variables["tools"] = map[string]interface{}{}

	server := &entity.MCPServer{
		ID:          common.GenerateUUID(),
		Name:        req.Name,
		TenantID:    tenantID,
		URL:         req.URL,
		ServerType:  req.ServerType,
		Description: req.Description,
		Variables:   variables,
		Headers:     headers,
	}

	if err := s.mcpServerDAO.CreateMCPServer(server); err != nil {
		return nil, common.CodeDataError, errors.New("Failed to create MCP server.")
	}

	return &CreateMCPServerResponse{
		ID:          server.ID,
		TenantID:    server.TenantID,
		Name:        server.Name,
		URL:         server.URL,
		ServerType:  server.ServerType,
		Description: server.Description,
		Variables:   server.Variables,
		Headers:     server.Headers,
	}, common.CodeSuccess, nil
}

// UpdateMCPServer updates an MCP server owned by a tenant.
func (s *MCPService) UpdateMCPServer(tenantID, mcpID string, req UpdateMCPServerRequest) (*entity.MCPServer, common.ErrorCode, error) {
	server, err := s.mcpServerDAO.GetByIDAndTenant(mcpID, tenantID)
	if err != nil {
		if isMCPServerNotFound(err) {
			return nil, common.CodeDataError, mcpServerNotFoundError(mcpID, tenantID)
		}
		return nil, common.CodeServerError, fmt.Errorf("failed to get MCP server %s: %w", mcpID, err)
	}
	if server == nil {
		return nil, common.CodeDataError, mcpServerNotFoundError(mcpID, tenantID)
	}

	serverType := server.ServerType
	serverTypeProvided := false
	if value, ok, err := optionalString(req, "server_type"); err != nil {
		return nil, common.CodeDataError, err
	} else if ok {
		serverType = value
		serverTypeProvided = true
	}
	if serverTypeProvided && !isValidMCPServerType(serverType) {
		return nil, common.CodeDataError, errors.New("Unsupported MCP server type.")
	}

	serverName := server.Name
	serverNameProvided := false
	if value, ok, err := optionalString(req, "name"); err != nil {
		return nil, common.CodeDataError, err
	} else if ok {
		serverName = value
		serverNameProvided = true
	}
	if serverName != "" && len([]byte(serverName)) > mcpServerNameLimit {
		return nil, common.CodeDataError, fmt.Errorf("Invalid MCP name or length is %d which is large than 255.", len([]byte(serverName)))
	}

	serverURL := server.URL
	serverURLProvided := false
	if value, ok, err := optionalString(req, "url"); err != nil {
		return nil, common.CodeDataError, err
	} else if ok {
		serverURL = strings.TrimSpace(value)
		if serverURL == "" {
			return nil, common.CodeDataError, errors.New("Invalid url.")
		}
		serverURLProvided = true
	}
	if serverURL == "" {
		return nil, common.CodeDataError, errors.New("Invalid url.")
	}

	headers := server.Headers
	if raw, ok := req["headers"]; ok {
		headers = safeJSONMap(raw)
	}
	if headers == nil {
		headers = entity.JSONMap{}
	}

	variables := server.Variables
	if raw, ok := req["variables"]; ok {
		variables = safeJSONMap(raw)
	}
	if variables == nil {
		variables = entity.JSONMap{}
	}
	delete(variables, "tools")
	variables["tools"] = map[string]interface{}{}

	updates := map[string]interface{}{
		"id":        mcpID,
		"tenant_id": tenantID,
		"headers":   headers,
		"variables": variables,
	}
	if serverNameProvided {
		updates["name"] = serverName
	}
	if serverURLProvided {
		updates["url"] = serverURL
	}
	if serverTypeProvided {
		updates["server_type"] = serverType
	}
	if raw, ok := req["description"]; ok {
		description, err := optionalNullableString(raw, "description")
		if err != nil {
			return nil, common.CodeDataError, err
		}
		updates["description"] = description
	}

	if _, err := s.mcpServerDAO.UpdateMCPServer(mcpID, tenantID, updates); err != nil {
		return nil, common.CodeServerError, err
	}

	updatedServer, err := s.mcpServerDAO.GetByIDAndTenant(mcpID, tenantID)
	if err != nil {
		if isMCPServerNotFound(err) {
			return nil, common.CodeDataError, mcpServerNotFoundError(mcpID, tenantID)
		}
		return nil, common.CodeServerError, fmt.Errorf("failed to fetch updated MCP server %s: %w", mcpID, err)
	}
	if updatedServer == nil {
		return nil, common.CodeDataError, mcpServerNotFoundError(mcpID, tenantID)
	}
	return updatedServer, common.CodeSuccess, nil
}

func isMCPServerNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// ListMCPServers lists MCP servers owned by a tenant.
func (s *MCPService) ListMCPServers(tenantID string, ids []string, keywords string, page, pageSize int, orderby string, desc bool) (*ListMCPServersResponse, common.ErrorCode, error) {
	servers, total, err := s.mcpServerDAO.ListMCPServers(tenantID, ids, keywords, orderby, desc)
	if err != nil {
		var orderbyErr *dao.InvalidMCPServerOrderByError
		if errors.As(err, &orderbyErr) {
			return nil, common.CodeExceptionError, err
		}
		return nil, common.CodeServerError, err
	}
	if servers == nil {
		servers = []*entity.MCPServer{}
	}
	servers = paginateMCPServers(servers, page, pageSize)

	items := make([]*MCPServerListItem, 0, len(servers))
	for _, server := range servers {
		variables := server.Variables
		if variables == nil {
			variables = entity.JSONMap{}
		}
		items = append(items, &MCPServerListItem{
			ID:          server.ID,
			Name:        server.Name,
			ServerType:  server.ServerType,
			URL:         server.URL,
			Description: server.Description,
			Variables:   variables,
			CreateDate:  formatMCPServerDate(server.CreateDate),
			UpdateDate:  formatMCPServerDate(server.UpdateDate),
		})
	}

	return &ListMCPServersResponse{
		MCPServers: items,
		Total:      total,
	}, common.CodeSuccess, nil
}

// DeleteMCPServer deletes an MCP server owned by a tenant.
func (s *MCPService) DeleteMCPServer(tenantID, mcpID string) (bool, common.ErrorCode, error) {
	server, err := s.mcpServerDAO.GetByID(mcpID)
	if err != nil {
		return false, common.CodeServerError, fmt.Errorf("failed to get MCP server %s: %w", mcpID, err)
	}
	if server == nil || server.TenantID != tenantID {
		return false, common.CodeDataError, mcpServerNotFoundError(mcpID, tenantID)
	}

	deleted, err := s.mcpServerDAO.DeleteMCPServer(mcpID, tenantID)
	if err != nil {
		return false, common.CodeServerError, err
	}
	if !deleted {
		return false, common.CodeDataError, mcpServerNotFoundError(mcpID, tenantID)
	}

	return true, common.CodeSuccess, nil
}

func mcpServerNotFoundError(mcpID, tenantID string) error {
	return fmt.Errorf("Cannot find MCP server %s for user %s", mcpID, tenantID)
}

func isValidMCPServerType(serverType string) bool {
	return serverType == mcpServerTypeSSE || serverType == mcpServerTypeStreamableHTTP
}

func optionalString(req UpdateMCPServerRequest, key string) (string, bool, error) {
	raw, ok := req[key]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", false, nil
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", true, fmt.Errorf("%s must be a string", key)
	}
	return value, true, nil
}

func optionalNullableString(raw json.RawMessage, key string) (*string, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("%s must be a string", key)
	}
	return &value, nil
}

func safeJSONMap(raw json.RawMessage) entity.JSONMap {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return entity.JSONMap{}
	}

	var value map[string]interface{}
	if err := json.Unmarshal(raw, &value); err == nil && value != nil {
		return entity.JSONMap(value)
	}

	var textValue string
	if err := json.Unmarshal(raw, &textValue); err != nil || textValue == "" {
		return entity.JSONMap{}
	}

	if err := json.Unmarshal([]byte(textValue), &value); err != nil || value == nil {
		return entity.JSONMap{}
	}
	return entity.JSONMap(value)
}

func formatMCPServerDate(date *time.Time) *string {
	if date == nil {
		return nil
	}
	formatted := date.Format(mcpServerDateFormat)
	return &formatted
}

func paginateMCPServers(servers []*entity.MCPServer, page, pageSize int) []*entity.MCPServer {
	if page == 0 || pageSize == 0 {
		return servers
	}

	start := (page - 1) * pageSize
	stop := page * pageSize
	return sliceMCPServers(servers, start, stop)
}

func sliceMCPServers(servers []*entity.MCPServer, start, stop int) []*entity.MCPServer {
	length := len(servers)
	start = normalizeMCPServerSliceIndex(start, length)
	stop = normalizeMCPServerSliceIndex(stop, length)
	if stop < start {
		return []*entity.MCPServer{}
	}
	return servers[start:stop]
}

func normalizeMCPServerSliceIndex(index, length int) int {
	if index < 0 {
		index += length
	}
	if index < 0 {
		return 0
	}
	if index > length {
		return length
	}
	return index
}
