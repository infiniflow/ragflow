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

package dao

import (
	"strings"

	"ragflow/internal/entity"
)

// MCPServerDAO MCP server data access object
type MCPServerDAO struct{}

// NewMCPServerDAO create MCP server DAO
func NewMCPServerDAO() *MCPServerDAO {
	return &MCPServerDAO{}
}

// mcpOrderColumns whitelists the columns that may be used for ordering, guarding
// against SQL injection through the caller-supplied orderby value. Mirrors the
// columns exposed by Python's MCPServerService.get_servers.
var mcpOrderColumns = map[string]string{
	"create_time": "create_time",
	"create_date": "create_date",
	"update_time": "update_time",
	"update_date": "update_date",
	"name":        "name",
}

// GetServers lists a tenant's MCP servers, optionally filtered by an id list and
// keyword, ordered by the given column. Mirrors Python's MCPServerService.get_servers
// (without DB-level pagination; the caller paginates the slice, matching list_mcp).
func (dao *MCPServerDAO) GetServers(tenantID string, idList []string, orderby string, desc bool, keywords string) ([]*entity.MCPServer, error) {
	query := DB.Model(&entity.MCPServer{}).Where("tenant_id = ?", tenantID)

	if len(idList) > 0 {
		query = query.Where("id IN ?", idList)
	}
	if keywords != "" {
		query = query.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}

	column, ok := mcpOrderColumns[orderby]
	if !ok {
		column = "create_time"
	}
	direction := "ASC"
	if desc {
		direction = "DESC"
	}
	query = query.Order(column + " " + direction)

	var servers []*entity.MCPServer
	if err := query.Find(&servers).Error; err != nil {
		return nil, err
	}
	return servers, nil
}

// GetByID gets an MCP server by ID.
func (dao *MCPServerDAO) GetByID(id string) (*entity.MCPServer, error) {
	var server entity.MCPServer
	if err := DB.Where("id = ?", id).First(&server).Error; err != nil {
		return nil, err
	}
	return &server, nil
}

// GetByIDAndTenant gets an MCP server scoped to a tenant.
func (dao *MCPServerDAO) GetByIDAndTenant(id, tenantID string) (*entity.MCPServer, error) {
	var server entity.MCPServer
	if err := DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&server).Error; err != nil {
		return nil, err
	}
	return &server, nil
}

// GetByNameAndTenant returns the MCP servers matching a name within a tenant.
func (dao *MCPServerDAO) GetByNameAndTenant(name, tenantID string) ([]*entity.MCPServer, error) {
	var servers []*entity.MCPServer
	err := DB.Where("name = ? AND tenant_id = ?", name, tenantID).Find(&servers).Error
	return servers, err
}

// Create inserts a new MCP server.
func (dao *MCPServerDAO) Create(server *entity.MCPServer) error {
	return DB.Create(server).Error
}

// UpdateByIDAndTenant updates an MCP server scoped to a tenant.
func (dao *MCPServerDAO) UpdateByIDAndTenant(id, tenantID string, updates map[string]interface{}) error {
	return DB.Model(&entity.MCPServer{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Updates(updates).Error
}

// DeleteByID deletes an MCP server by ID.
func (dao *MCPServerDAO) DeleteByID(id string) error {
	return DB.Where("id = ?", id).Delete(&entity.MCPServer{}).Error
}
