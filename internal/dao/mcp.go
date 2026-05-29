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
	"errors"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// MCPServerDAO MCP server data access object.
type MCPServerDAO struct{}

// NewMCPServerDAO creates an MCP server DAO.
func NewMCPServerDAO() *MCPServerDAO {
	return &MCPServerDAO{}
}

// GetByID returns an MCP server by ID.
func (dao *MCPServerDAO) GetByID(id string) (*entity.MCPServer, error) {
	var server entity.MCPServer
	if err := DB.Where("id = ?", id).First(&server).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &server, nil
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

// DeleteMCPServer deletes an MCP server owned by a tenant.
func (dao *MCPServerDAO) DeleteMCPServer(id, tenantID string) (bool, error) {
	result := DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&entity.MCPServer{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}
