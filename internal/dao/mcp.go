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

// MCPServerDAO MCP server data access object.
type MCPServerDAO struct{}

// NewMCPServerDAO creates an MCP server DAO.
func NewMCPServerDAO() *MCPServerDAO {
	return &MCPServerDAO{}
}

// ExistsByNameAndTenant returns whether an MCP server name already exists for a tenant.
func (dao *MCPServerDAO) ExistsByNameAndTenant(name, tenantID string) (bool, error) {
	var count int64
	if err := DB.Model(&entity.MCPServer{}).
		Where("name = ? AND tenant_id = ?", name, tenantID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateMCPServer creates an MCP server.
func (dao *MCPServerDAO) CreateMCPServer(server *entity.MCPServer) error {
	return DB.Create(server).Error
}

// ListMCPServers returns MCP servers for a tenant with optional filtering.
func (dao *MCPServerDAO) ListMCPServers(tenantID string, ids []string, keywords string, page, pageSize int, orderby string, desc bool) ([]*entity.MCPServer, int64, error) {
	var servers []*entity.MCPServer
	var total int64

	query := DB.Model(&entity.MCPServer{}).Where("tenant_id = ?", tenantID)

	if len(ids) > 0 {
		query = query.Where("id IN ?", ids)
	}

	if keywords != "" {
		query = query.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	orderColumn := mcpServerOrderColumn(orderby)
	orderDirection := "ASC"
	if desc {
		orderDirection = "DESC"
	}
	query = query.Order(orderColumn + " " + orderDirection)

	if page > 0 && pageSize > 0 {
		query = query.Offset((page - 1) * pageSize).Limit(pageSize)
	}

	if err := query.
		Select("id", "name", "server_type", "url", "description", "variables", "create_date", "update_date").
		Find(&servers).Error; err != nil {
		return nil, 0, err
	}

	return servers, total, nil
}

// GetMCPServerByIDAndTenantID returns one MCP server scoped to a tenant.
func (dao *MCPServerDAO) GetMCPServerByIDAndTenantID(id, tenantID string) (*entity.MCPServer, error) {
	var server entity.MCPServer
	err := DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&server).Error
	if err != nil {
		return nil, err
	}
	return &server, nil
}

func mcpServerOrderColumn(orderby string) string {
	switch orderby {
	case "id":
		return "id"
	case "name":
		return "name"
	case "server_type":
		return "server_type"
	case "url":
		return "url"
	case "update_time":
		return "update_time"
	case "update_date":
		return "update_date"
	case "create_date":
		return "create_date"
	default:
		return "create_time"
	}
}
