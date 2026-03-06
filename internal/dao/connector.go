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
	"ragflow/internal/model"
)

// ConnectorDAO connector data access object
type ConnectorDAO struct{}

// NewConnectorDAO create connector DAO
func NewConnectorDAO() *ConnectorDAO {
	return &ConnectorDAO{}
}

// ConnectorListItem connector list item (subset of fields)
type ConnectorListItem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"`
	Status string `json:"status"`
}

// ListByTenantID list connectors by tenant ID
// Only selects id, name, source, status fields (matching Python implementation)
func (dao *ConnectorDAO) ListByTenantID(tenantID string) ([]*ConnectorListItem, error) {
	var connectors []*ConnectorListItem

	err := DB.Model(&model.Connector{}).
		Select("id", "name", "source", "status").
		Where("tenant_id = ?", tenantID).
		Find(&connectors).Error

	if err != nil {
		return nil, err
	}

	return connectors, nil
}

// GetByID get connector by ID
func (dao *ConnectorDAO) GetByID(id string) (*model.Connector, error) {
	var connector model.Connector
	err := DB.Where("id = ?", id).First(&connector).Error
	if err != nil {
		return nil, err
	}
	return &connector, nil
}

// Create create a new connector
func (dao *ConnectorDAO) Create(connector *model.Connector) error {
	return DB.Create(connector).Error
}

// UpdateByID update connector by ID
func (dao *ConnectorDAO) UpdateByID(id string, updates map[string]interface{}) error {
	return DB.Model(&model.Connector{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteByID delete connector by ID
func (dao *ConnectorDAO) DeleteByID(id string) error {
	return DB.Where("id = ?", id).Delete(&model.Connector{}).Error
}
