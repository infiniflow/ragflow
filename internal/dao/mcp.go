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
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// MCPServerDAO MCP server data access object.
type MCPServerDAO struct{}

// InvalidMCPServerOrderByError matches the Python list endpoint's error shape
// for unknown MCPServer ordering fields.
type InvalidMCPServerOrderByError struct {
	Field string
}

func (e *InvalidMCPServerOrderByError) Error() string {
	return fmt.Sprintf("AttributeError(%q)", fmt.Sprintf("type object 'MCPServer' has no attribute '%s'", e.Field))
}

type MCPServerListRow struct {
	entity.MCPServer
	Nickname *string `gorm:"column:nickname"`
}

// NewMCPServerDAO creates an MCP server DAO.
func NewMCPServerDAO() *MCPServerDAO {
	return &MCPServerDAO{}
}

// GetByID returns an MCP server by ID.
func (dao *MCPServerDAO) GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.MCPServer, error) {
	var server entity.MCPServer
	if err := db.WithContext(ctx).Take(&server, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &server, nil
}

// ExistsByNameAndTenant returns whether an MCP server name already exists for a tenant.
func (dao *MCPServerDAO) ExistsByNameAndTenant(ctx context.Context, db *gorm.DB, name, tenantID string) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&entity.MCPServer{}).
		Where("LOWER(name) = LOWER(?) AND tenant_id = ?", name, tenantID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateMCPServer creates an MCP server.
func (dao *MCPServerDAO) CreateMCPServer(ctx context.Context, db *gorm.DB, server *entity.MCPServer) error {
	return db.WithContext(ctx).Create(server).Error
}

// ListMCPServers returns the authorized MCP server page and the total number
// of authorized servers. Authorization is applied before counting and paging.
func (dao *MCPServerDAO) ListMCPServers(ctx context.Context, db *gorm.DB, tenantID string, ids []string, keywords, orderby string, desc bool, page, pageSize int, authorize func(*gorm.DB) *gorm.DB) ([]*MCPServerListRow, int64, error) {
	orderColumn, err := mcpServerOrderColumn(orderby)
	if err != nil {
		return nil, 0, err
	}
	if authorize == nil {
		return nil, 0, fmt.Errorf("MCP server list requires an authorization scope")
	}

	baseQuery := func() *gorm.DB {
		query := db.WithContext(ctx).Model(&entity.MCPServer{}).Where("tenant_id = ?", tenantID)
		if len(ids) > 0 {
			query = query.Where("mcp_server.id IN ?", ids)
		}
		if keywords != "" {
			query = query.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
		}
		return authorize(query)
	}

	var total int64
	if err := baseQuery().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query := baseQuery()
	orderDirection := "ASC"
	if desc {
		orderDirection = "DESC"
	}
	query = query.Order(orderColumn + " " + orderDirection)
	if orderColumn != "mcp_server.id" {
		query = query.Order("mcp_server.id ASC")
	}
	if page != 0 && pageSize != 0 {
		start, stop := mcpServerPageRange(total, page, pageSize)
		if stop <= start {
			return []*MCPServerListRow{}, total, nil
		}
		query = query.Offset(start).Limit(stop - start)
	}
	query = query.Joins("LEFT JOIN user AS owner ON mcp_server.user_id = owner.id").
		Select("mcp_server.id, mcp_server.user_id, mcp_server.tenant_id, mcp_server.name, mcp_server.server_type, mcp_server.url, mcp_server.description, mcp_server.variables, mcp_server.create_date, mcp_server.update_date, owner.nickname")
	var servers []*MCPServerListRow
	if err := query.Find(&servers).Error; err != nil {
		return nil, 0, err
	}
	return servers, total, nil
}

func mcpServerPageRange(total int64, page, pageSize int) (int, int) {
	pageValue := big.NewInt(int64(page))
	pageSizeValue := big.NewInt(int64(pageSize))
	startValue := new(big.Int).Sub(new(big.Int).Set(pageValue), big.NewInt(1))
	startValue.Mul(startValue, pageSizeValue)
	stopValue := new(big.Int).Mul(new(big.Int).Set(pageValue), pageSizeValue)
	totalValue := big.NewInt(total)

	clamp := func(value *big.Int) int {
		value = new(big.Int).Set(value)
		if value.Sign() < 0 {
			value.Add(value, totalValue)
		}
		if value.Sign() < 0 {
			return 0
		}
		if value.Cmp(totalValue) > 0 {
			return int(total)
		}
		return int(value.Int64())
	}
	return clamp(startValue), clamp(stopValue)
}

// DeleteMCPServer deletes an MCP server owned by a tenant.
func (dao *MCPServerDAO) DeleteMCPServer(ctx context.Context, db *gorm.DB, id, tenantID string) (bool, error) {
	result := db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&entity.MCPServer{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// UpdateMCPServer updates an MCP server owned by a tenant.
func (dao *MCPServerDAO) UpdateMCPServer(ctx context.Context, db *gorm.DB, id, tenantID string, updates map[string]interface{}) (bool, error) {
	result := db.WithContext(ctx).Model(&entity.MCPServer{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func mcpServerOrderColumn(orderby string) (string, error) {
	switch orderby {
	case "id":
		return "mcp_server.id", nil
	case "name":
		return "mcp_server.name", nil
	case "server_type":
		return "mcp_server.server_type", nil
	case "url":
		return "mcp_server.url", nil
	case "update_time", "update_date":
		return "mcp_server.update_date", nil
	case "create_time", "create_date":
		return "mcp_server.create_date", nil
	default:
		return "", &InvalidMCPServerOrderByError{Field: orderby}
	}
}
