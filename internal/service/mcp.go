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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/utility"

	"gorm.io/gorm"
)

const (
	mcpServerTypeSSE            = "sse"
	mcpServerTypeStreamableHTTP = "streamable-http"
	mcpServerNameLimit          = 255
	defaultMCPFetchTimeoutSec   = 10
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

// ---------- import + test (this PR's additions) ----------

// Sentinel errors mapped by the handler to Python's response codes for the
// import / test endpoints. Per-server CRUD errors stay inside CreateMCPServer.
var (
	// ErrMCPInvalidType mirrors Python's "Unsupported MCP server type.".
	ErrMCPInvalidType = errors.New("unsupported MCP server type")
	// ErrMCPInvalidName mirrors Python's invalid-name/length error.
	ErrMCPInvalidName = errors.New("invalid MCP name")
	// ErrMCPInvalidURL mirrors Python's "Invalid url.".
	ErrMCPInvalidURL = errors.New("invalid url")
	// ErrMCPTestFailed is returned by TestServer when the live connection or
	// tool-list fetch fails. The handler maps this to code 102 (DATA_ERROR),
	// matching Python's test_mcp which never returns HTTP 500 for fetch errors.
	ErrMCPTestFailed = errors.New("MCP test failed")
)

// ImportResult is a single per-server outcome in the bulk import response,
// matching the shape returned by Python's import_multiple.
type ImportResult struct {
	Server  string `json:"server"`
	Success bool   `json:"success"`
	Action  string `json:"action,omitempty"`
	ID      string `json:"id,omitempty"`
	NewName string `json:"new_name,omitempty"`
	Message string `json:"message,omitempty"`
}

// ImportServers bulk-imports MCP servers from a {"mcpServers": {name: config}}
// map. For each entry: validate type and URL, de-duplicate the name with a
// "_N" suffix, fetch the remote tool list via mcpclient (SSRF-guarded), and
// persist the server with tools stored under variables.tools. Mirrors
// Python's import_multiple.
//
// timeoutSeconds controls how long each tool-fetch call waits; <=0 falls back
// to the Python default of 10 s.
func (s *MCPService) ImportServers(tenantID string, servers map[string]map[string]interface{}, timeoutSeconds float64) ([]ImportResult, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultMCPFetchTimeoutSec
	}
	timeout := time.Duration(timeoutSeconds * float64(time.Second))

	results := make([]ImportResult, 0, len(servers))
	for serverName, config := range servers {
		url, hasURL := config["url"].(string)
		stype, hasType := config["type"].(string)
		if !hasType || !hasURL {
			results = append(results, ImportResult{Server: serverName, Success: false, Message: "Missing required fields (type or url)"})
			continue
		}
		if serverName == "" || len([]byte(serverName)) > mcpServerNameLimit {
			results = append(results, ImportResult{Server: serverName, Success: false, Message: fmt.Sprintf("Invalid MCP name or length is %d which is large than 255.", len(serverName))})
			continue
		}
		if !isValidMCPServerType(stype) {
			results = append(results, ImportResult{Server: serverName, Success: false, Message: "Unsupported MCP server type."})
			continue
		}

		baseName := serverName
		newName, err := s.nextAvailableMCPName(baseName, tenantID)
		if err != nil {
			return nil, err
		}

		variables := map[string]interface{}{}
		stringVars := map[string]string{}
		for k, v := range config {
			if k == "type" || k == "url" || k == "headers" {
				continue
			}
			variables[k] = v
			if sv, ok := v.(string); ok {
				stringVars[k] = sv
			}
		}
		delete(variables, "tools")
		delete(stringVars, "tools")

		// Headers can be provided either as a top-level "headers" map
		// (preferred — matches the Python import shape) or as a flat
		// "authorization_token" string at the entry root. Both go to the
		// MCP client for tool discovery and to the persisted record so
		// configs that depend on custom auth headers survive the round
		// trip.
		headers := map[string]string{}
		headerVals := map[string]interface{}{}
		if rawHeaders, ok := config["headers"].(map[string]interface{}); ok {
			for k, v := range rawHeaders {
				if sv, ok := v.(string); ok {
					headers[k] = sv
				}
				headerVals[k] = v
			}
		}
		if token, ok := config["authorization_token"].(string); ok {
			if _, exists := headers["authorization_token"]; !exists {
				headers["authorization_token"] = token
			}
			if _, exists := headerVals["authorization_token"]; !exists {
				headerVals["authorization_token"] = token
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		tools, fetchErr := utility.FetchTools(ctx, utility.FetchOptions{
			URL:        url,
			ServerType: stype,
			Headers:    headers,
			Variables:  stringVars,
			Timeout:    timeout,
		})
		cancel()
		if fetchErr != nil {
			results = append(results, ImportResult{Server: baseName, Success: false, Message: fetchErr.Error()})
			continue
		}
		variables["tools"] = toolsAsMap(tools)

		server := &entity.MCPServer{
			ID:         common.GenerateUUID(),
			TenantID:   tenantID,
			Name:       newName,
			URL:        url,
			ServerType: stype,
			Variables:  entity.JSONMap(variables),
			Headers:    entity.JSONMap(headerVals),
		}
		if err := s.mcpServerDAO.CreateMCPServer(server); err != nil {
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

func (s *MCPService) nextAvailableMCPName(base, tenantID string) (string, error) {
	name := base
	counter := 0
	for {
		exists, err := s.mcpServerDAO.ExistsByNameAndTenant(name, tenantID)
		if err != nil {
			return "", err
		}
		if !exists {
			return name, nil
		}
		name = fmt.Sprintf("%s_%d", base, counter)
		counter++
	}
}

// TestServerRequest is the body of POST /mcp/servers/:mcp_id/test. The mcp_id
// from the URL path is threaded through to the connect call for log
// correlation; the connection itself is opened from the request body so the
// user can preview unsaved edits — matching Python's test_mcp.
type TestServerRequest struct {
	URL        string                 `json:"url"`
	ServerType string                 `json:"server_type"`
	Headers    map[string]interface{} `json:"headers,omitempty"`
	Variables  map[string]interface{} `json:"variables,omitempty"`
	Timeout    float64                `json:"timeout,omitempty"`
}

// TestServer opens a live MCP session and returns the tools the server
// advertises. Mirrors Python's test_mcp. mcpID is used for log correlation
// only.
func (s *MCPService) TestServer(mcpID string, req *TestServerRequest) ([]map[string]interface{}, error) {
	if req == nil || req.URL == "" {
		return nil, fmt.Errorf("%w: Invalid MCP url.", ErrMCPInvalidURL)
	}
	if !isValidMCPServerType(req.ServerType) {
		return nil, ErrMCPInvalidType
	}

	// Run the SSRF guard up front so URL-shape failures (disallowed
	// scheme, missing host, non-public address) surface as
	// ErrMCPInvalidURL data errors instead of being swallowed inside the
	// generic FetchTools error and re-classified by the handler as a 500.
	// FetchTools repeats the check internally; the second call is cheap.
	if _, _, err := utility.AssertURLSafe(req.URL); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrMCPInvalidURL, err.Error())
	}

	timeoutSec := req.Timeout
	if timeoutSec <= 0 {
		timeoutSec = defaultMCPFetchTimeoutSec
	}
	timeout := time.Duration(timeoutSec * float64(time.Second))

	headers := map[string]string{}
	for k, v := range req.Headers {
		if sv, ok := v.(string); ok {
			headers[k] = sv
		}
	}
	vars := map[string]string{}
	for k, v := range req.Variables {
		if sv, ok := v.(string); ok {
			vars[k] = sv
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	tools, err := utility.FetchTools(ctx, utility.FetchOptions{
		URL:        req.URL,
		ServerType: req.ServerType,
		Headers:    headers,
		Variables:  vars,
		Timeout:    timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: Test MCP error (id=%s): %v", ErrMCPTestFailed, mcpID, err)
	}

	out := make([]map[string]interface{}, 0, len(tools))
	for _, t := range tools {
		raw := t.Raw
		if raw == nil {
			raw = map[string]interface{}{"name": t.Name}
			if t.Description != "" {
				raw["description"] = t.Description
			}
			if t.InputSchema != nil {
				raw["inputSchema"] = t.InputSchema
			}
		}
		raw["enabled"] = true
		out = append(out, raw)
	}
	return out, nil
}

// toolsAsMap mirrors Python's `{tool["name"]: tool ...}` shape used when
// persisting variables.tools.
func toolsAsMap(tools []utility.Tool) map[string]interface{} {
	m := map[string]interface{}{}
	for _, t := range tools {
		if t.Raw != nil {
			m[t.Name] = t.Raw
			continue
		}
		entry := map[string]interface{}{"name": t.Name}
		if t.Description != "" {
			entry["description"] = t.Description
		}
		if t.InputSchema != nil {
			entry["inputSchema"] = t.InputSchema
		}
		m[t.Name] = entry
	}
	return m
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
