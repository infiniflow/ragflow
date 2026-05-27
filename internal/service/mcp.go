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
	"errors"
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// Sentinel errors mapped by the handler to the response codes used by the Python mcp_api.
var (
	// ErrMCPNotFound mirrors Python's "Cannot find MCP server ..." data error.
	ErrMCPNotFound = errors.New("cannot find MCP server")
	// ErrMCPInvalidType mirrors Python's "Unsupported MCP server type.".
	ErrMCPInvalidType = errors.New("unsupported MCP server type")
	// ErrMCPInvalidName mirrors Python's invalid-name/length error.
	ErrMCPInvalidName = errors.New("invalid MCP name")
	// ErrMCPInvalidURL mirrors Python's "Invalid url.".
	ErrMCPInvalidURL = errors.New("invalid url")
	// ErrMCPDuplicateName mirrors Python's "Duplicated MCP server name.".
	ErrMCPDuplicateName = errors.New("duplicated MCP server name")
	// ErrMCPTestUnsupported is returned by Test until the Go MCP client lands.
	ErrMCPTestUnsupported = errors.New("MCP connection test is not yet implemented in the Go server")
)

const mcpNameMaxBytes = 255

// MCPService MCP server service
type MCPService struct {
	mcpDAO *dao.MCPServerDAO
}

// NewMCPService create MCP server service
func NewMCPService() *MCPService {
	return &MCPService{
		mcpDAO: dao.NewMCPServerDAO(),
	}
}

// ListMCPServersResponse mirrors Python's {"mcp_servers": [...], "total": n}.
type ListMCPServersResponse struct {
	MCPServers []*entity.MCPServer `json:"mcp_servers"`
	Total      int                 `json:"total"`
}

// ListServers lists a tenant's MCP servers with keyword/order filters and applies
// in-memory pagination, mirroring Python's list_mcp.
func (s *MCPService) ListServers(tenantID string, idList []string, page, pageSize int, orderby string, desc bool, keywords string) (*ListMCPServersResponse, error) {
	servers, err := s.mcpDAO.GetServers(tenantID, idList, orderby, desc, keywords)
	if err != nil {
		return nil, err
	}
	total := len(servers)

	if page > 0 && pageSize > 0 {
		start := (page - 1) * pageSize
		end := page * pageSize
		if start >= total {
			servers = []*entity.MCPServer{}
		} else {
			if end > total {
				end = total
			}
			servers = servers[start:end]
		}
	}

	return &ListMCPServersResponse{MCPServers: servers, Total: total}, nil
}

// GetServer returns a tenant-scoped MCP server. Mirrors Python's detail.
func (s *MCPService) GetServer(mcpID, tenantID string) (*entity.MCPServer, error) {
	server, err := s.mcpDAO.GetByIDAndTenant(mcpID, tenantID)
	if err != nil {
		return nil, ErrMCPNotFound
	}
	return server, nil
}

// ExportServers returns the {"mcpServers": {...}} export envelope for the given ids,
// scoped to the tenant. Mirrors Python's _export_mcp_servers. Returns ErrMCPNotFound
// when no requested server is accessible.
func (s *MCPService) ExportServers(mcpIDs []string, tenantID string) (map[string]interface{}, error) {
	exported := map[string]interface{}{}
	for _, id := range mcpIDs {
		server, err := s.mcpDAO.GetByID(id)
		if err != nil || server.TenantID != tenantID {
			continue
		}
		token := ""
		var tools interface{} = map[string]interface{}{}
		if server.Variables != nil {
			if v, ok := server.Variables["authorization_token"].(string); ok {
				token = v
			}
			if v, ok := server.Variables["tools"]; ok {
				tools = v
			}
		}
		exported[server.Name] = map[string]interface{}{
			"type":                server.ServerType,
			"url":                 server.URL,
			"name":                server.Name,
			"authorization_token": token,
			"tools":               tools,
		}
	}
	if len(exported) == 0 {
		return nil, ErrMCPNotFound
	}
	return map[string]interface{}{"mcpServers": exported}, nil
}

// CreateMCPRequest holds the fields accepted when creating an MCP server.
type CreateMCPRequest struct {
	Name        string                 `json:"name"`
	URL         string                 `json:"url"`
	ServerType  string                 `json:"server_type"`
	Description *string                `json:"description,omitempty"`
	Variables   map[string]interface{} `json:"variables,omitempty"`
	Headers     map[string]interface{} `json:"headers,omitempty"`
}

// CreateServer creates a tenant-scoped MCP server. Mirrors Python's create, minus the
// live tool-discovery step (get_mcp_tools); variables.tools is not populated here.
func (s *MCPService) CreateServer(tenantID string, req *CreateMCPRequest) (*entity.MCPServer, error) {
	if !entity.IsValidMCPServerType(req.ServerType) {
		return nil, ErrMCPInvalidType
	}
	if req.Name == "" || len([]byte(req.Name)) > mcpNameMaxBytes {
		return nil, ErrMCPInvalidName
	}
	if req.URL == "" {
		return nil, ErrMCPInvalidURL
	}

	existing, err := s.mcpDAO.GetByNameAndTenant(req.Name, tenantID)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		return nil, ErrMCPDuplicateName
	}

	variables := entity.JSONMap{}
	for k, v := range req.Variables {
		variables[k] = v
	}
	// tools are populated by live discovery, which is not yet ported to Go.
	delete(variables, "tools")

	headers := entity.JSONMap{}
	for k, v := range req.Headers {
		headers[k] = v
	}

	server := &entity.MCPServer{
		ID:          common.GenerateUUID(),
		TenantID:    tenantID,
		Name:        req.Name,
		URL:         req.URL,
		ServerType:  req.ServerType,
		Description: req.Description,
		Variables:   variables,
		Headers:     headers,
	}
	if err := s.mcpDAO.Create(server); err != nil {
		return nil, fmt.Errorf("failed to create MCP server: %w", err)
	}
	return server, nil
}

// UpdateMCPRequest holds the mutable fields for an MCP server update. Pointer fields
// distinguish "not provided" (fall back to existing) from explicit values, matching
// Python's req.get(field, mcp_server.field) semantics.
type UpdateMCPRequest struct {
	Name        *string                 `json:"name,omitempty"`
	URL         *string                 `json:"url,omitempty"`
	ServerType  *string                 `json:"server_type,omitempty"`
	Description *string                 `json:"description,omitempty"`
	Variables   *map[string]interface{} `json:"variables,omitempty"`
	Headers     *map[string]interface{} `json:"headers,omitempty"`
}

// UpdateServer updates a tenant-scoped MCP server. Mirrors Python's update, minus the
// live tool-discovery step; variables.tools is not populated here.
func (s *MCPService) UpdateServer(mcpID, tenantID string, req *UpdateMCPRequest) (*entity.MCPServer, error) {
	current, err := s.mcpDAO.GetByIDAndTenant(mcpID, tenantID)
	if err != nil {
		return nil, ErrMCPNotFound
	}

	serverType := current.ServerType
	if req.ServerType != nil {
		serverType = *req.ServerType
	}
	if serverType != "" && !entity.IsValidMCPServerType(serverType) {
		return nil, ErrMCPInvalidType
	}

	name := current.Name
	if req.Name != nil {
		name = *req.Name
	}
	if name != "" && len([]byte(name)) > mcpNameMaxBytes {
		return nil, ErrMCPInvalidName
	}

	url := current.URL
	if req.URL != nil {
		url = *req.URL
	}
	if url == "" {
		return nil, ErrMCPInvalidURL
	}

	variables := entity.JSONMap{}
	if req.Variables != nil {
		for k, v := range *req.Variables {
			variables[k] = v
		}
	} else if current.Variables != nil {
		for k, v := range current.Variables {
			variables[k] = v
		}
	}
	delete(variables, "tools")

	headers := entity.JSONMap{}
	if req.Headers != nil {
		for k, v := range *req.Headers {
			headers[k] = v
		}
	} else if current.Headers != nil {
		for k, v := range current.Headers {
			headers[k] = v
		}
	}

	updates := map[string]interface{}{
		"name":        name,
		"url":         url,
		"server_type": serverType,
		"variables":   variables,
		"headers":     headers,
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}

	if err := s.mcpDAO.UpdateByIDAndTenant(mcpID, tenantID, updates); err != nil {
		return nil, fmt.Errorf("failed to update MCP server: %w", err)
	}

	updated, err := s.mcpDAO.GetByID(mcpID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated MCP server: %w", err)
	}
	return updated, nil
}

// DeleteServer deletes a tenant-scoped MCP server. Mirrors Python's rm.
func (s *MCPService) DeleteServer(mcpID, tenantID string) error {
	server, err := s.mcpDAO.GetByID(mcpID)
	if err != nil || server.TenantID != tenantID {
		return ErrMCPNotFound
	}
	return s.mcpDAO.DeleteByID(mcpID)
}

// ImportResult is a single per-server outcome in the bulk import response.
type ImportResult struct {
	Server  string `json:"server"`
	Success bool   `json:"success"`
	Action  string `json:"action,omitempty"`
	ID      string `json:"id,omitempty"`
	NewName string `json:"new_name,omitempty"`
	Message string `json:"message,omitempty"`
}

// ImportServers bulk-imports MCP servers from a {"mcpServers": {name: config}} map.
// Mirrors Python's import_multiple (name de-duplication with a "_N" suffix), minus
// the live tool-discovery step.
func (s *MCPService) ImportServers(tenantID string, servers map[string]map[string]interface{}) ([]ImportResult, error) {
	results := make([]ImportResult, 0, len(servers))

	for serverName, config := range servers {
		url, hasURL := config["url"].(string)
		stype, hasType := config["type"].(string)
		if !hasType || !hasURL {
			results = append(results, ImportResult{Server: serverName, Success: false, Message: "Missing required fields (type or url)"})
			continue
		}
		if serverName == "" || len([]byte(serverName)) > mcpNameMaxBytes {
			results = append(results, ImportResult{Server: serverName, Success: false, Message: fmt.Sprintf("Invalid MCP name or length is %d which is large than 255.", len(serverName))})
			continue
		}

		// De-duplicate the name with a "_N" suffix, mirroring Python's loop.
		baseName := serverName
		newName := baseName
		counter := 0
		for {
			existing, err := s.mcpDAO.GetByNameAndTenant(newName, tenantID)
			if err != nil {
				return nil, err
			}
			if len(existing) == 0 {
				break
			}
			newName = fmt.Sprintf("%s_%d", baseName, counter)
			counter++
		}

		variables := entity.JSONMap{}
		for k, v := range config {
			if k == "type" || k == "url" || k == "headers" {
				continue
			}
			variables[k] = v
		}
		delete(variables, "tools")

		headers := entity.JSONMap{}
		if token, ok := config["authorization_token"]; ok {
			headers["authorization_token"] = token
		}

		server := &entity.MCPServer{
			ID:         common.GenerateUUID(),
			TenantID:   tenantID,
			Name:       newName,
			URL:        url,
			ServerType: stype,
			Variables:  variables,
			Headers:    headers,
		}
		if err := s.mcpDAO.Create(server); err != nil {
			results = append(results, ImportResult{Server: serverName, Success: false, Message: "Failed to create MCP server."})
			continue
		}

		result := ImportResult{Server: serverName, Success: true, Action: "created", ID: server.ID, NewName: newName}
		if newName != baseName {
			result.Message = fmt.Sprintf("Renamed from '%s' to '%s' avoid duplication", baseName, newName)
		}
		results = append(results, result)
	}

	return results, nil
}

// TestServer would open a live MCP connection and enumerate the server's tools.
// The Go server has no MCP client yet (the Python path uses MCPToolCallSession /
// get_mcp_tools over SSE / streamable-HTTP), so this validates the request shape
// and returns ErrMCPTestUnsupported. Tracked as a follow-up per issue #15275.
func (s *MCPService) TestServer(url, serverType string) ([]map[string]interface{}, error) {
	if url == "" {
		return nil, ErrMCPInvalidURL
	}
	if !entity.IsValidMCPServerType(serverType) {
		return nil, ErrMCPInvalidType
	}
	return nil, ErrMCPTestUnsupported
}
