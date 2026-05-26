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
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/dao"
)

// MCPService handles MCP server operations.
type MCPService struct {
	mcpServerDAO *dao.MCPServerDAO
}

// NewMCPService creates an MCP service.
func NewMCPService() *MCPService {
	return &MCPService{
		mcpServerDAO: dao.NewMCPServerDAO(),
	}
}

// DeleteMCPServer deletes an MCP server owned by a tenant.
func (s *MCPService) DeleteMCPServer(tenantID, mcpID string) (bool, common.ErrorCode, error) {
	server, err := s.mcpServerDAO.GetByID(mcpID)
	if err != nil || server.TenantID != tenantID {
		return false, common.CodeDataError, fmt.Errorf("Cannot find MCP server %s for user %s", mcpID, tenantID)
	}

	deleted, err := s.mcpServerDAO.DeleteMCPServer(mcpID, tenantID)
	if err != nil {
		return false, common.CodeServerError, err
	}
	if !deleted {
		return false, common.CodeDataError, fmt.Errorf("Failed to delete MCP servers ['%s']", mcpID)
	}

	return true, common.CodeSuccess, nil
}
