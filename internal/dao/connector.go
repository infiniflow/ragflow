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
	"gorm.io/gorm/clause"
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

// ConnectorDatasetListItem represents a connector linked to a dataset.
type ConnectorDatasetListItem struct {
	ID        string `json:"id" gorm:"column:id"`
	Source    string `json:"source" gorm:"column:source"`
	Name      string `json:"name" gorm:"column:name"`
	AutoParse string `json:"auto_parse" gorm:"column:auto_parse"`
	Status    string `json:"status" gorm:"column:status"`
}

// ListByTenantID list connectors by tenant ID
// Only selects id, name, source, status fields (matching Python implementation)
func (dao *ConnectorDAO) ListByTenantID(tenantID string) ([]*ConnectorListItem, error) {
	var connectors []*ConnectorListItem

	err := DB.Model(&entity.Connector{}).
		Select("id", "name", "source", "status").
		Where("tenant_id = ?", tenantID).
		Find(&connectors).Error

	if err != nil {
		return nil, err
	}

	return connectors, nil
}

// ListByDatasetID lists connectors linked to a dataset.
func (dao *ConnectorDAO) ListByDatasetID(datasetID string) ([]*ConnectorDatasetListItem, error) {
	var connectors []*ConnectorDatasetListItem

	err := DB.Model(&entity.Connector2Kb{}).
		Select("connector.id, connector.source, connector.name, connector2kb.auto_parse, connector.status").
		Joins("JOIN connector ON connector2kb.connector_id = connector.id").
		Where("connector2kb.kb_id = ?", datasetID).
		Scan(&connectors).Error

	if err != nil {
		return nil, err
	}

	return connectors, nil
}

// GetByID get connector by ID
func (dao *ConnectorDAO) GetByID(id string) (*entity.Connector, error) {
	var connector entity.Connector
	err := DB.Where("id = ?", id).First(&connector).Error
	if err != nil {
		return nil, err
	}
	return &connector, nil
}

// Create create a new connector
func (dao *ConnectorDAO) Create(connector *entity.Connector) error {
	return DB.Create(connector).Error
}

// UpdateByID update connector by ID
func (dao *ConnectorDAO) UpdateByID(id string, updates map[string]interface{}) error {
	return DB.Model(&entity.Connector{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteByID delete connector by ID
func (dao *ConnectorDAO) DeleteByID(id string) error {
	return DB.Where("id = ?", id).Delete(&entity.Connector{}).Error
}

// ListMappingsByConnectorID lists dataset links for a connector.
func (dao *ConnectorDAO) ListMappingsByConnectorID(connectorID string) ([]*entity.Connector2Kb, error) {
	var mappings []*entity.Connector2Kb
	err := DB.Where("connector_id = ?", connectorID).Find(&mappings).Error
	return mappings, err
}

// CancelRunningOrScheduledLogs marks active sync logs as canceled for a connector.
func (dao *ConnectorDAO) CancelRunningOrScheduledLogs(connectorID string) error {
	return DB.Model(&entity.SyncLogs{}).
		Where("connector_id = ? AND status IN ?", connectorID, []string{string(entity.TaskStatusSchedule), string(entity.TaskStatusRunning)}).
		Update("status", string(entity.TaskStatusCancel)).Error
}

// GetLatestDoneSyncLog gets the latest completed sync log for a connector/dataset/task type.
func (dao *ConnectorDAO) GetLatestDoneSyncLog(connectorID, datasetID, taskType string) (*entity.SyncLogs, error) {
	var log entity.SyncLogs
	err := DB.Where("connector_id = ? AND kb_id = ? AND task_type = ? AND status = ?", connectorID, datasetID, taskType, string(entity.TaskStatusDone)).
		Order("update_time DESC").
		First(&log).Error
	if err != nil {
		return nil, err
	}
	return &log, nil
}

// HasScheduledSyncLog returns whether a scheduled sync log already exists.
func (dao *ConnectorDAO) HasScheduledSyncLog(connectorID, datasetID, taskType string) (bool, error) {
	var count int64
	err := DB.Model(&entity.SyncLogs{}).
		Where("connector_id = ? AND kb_id = ? AND task_type = ? AND status = ?", connectorID, datasetID, taskType, string(entity.TaskStatusSchedule)).
		Count(&count).Error
	return count > 0, err
}

// CreateSyncLog creates a connector sync log.
func (dao *ConnectorDAO) CreateSyncLog(log *entity.SyncLogs) error {
	return DB.Create(log).Error
}

// ScheduleSyncLogIfAbsent creates a scheduled sync log and marks the connector scheduled atomically.
func (dao *ConnectorDAO) ScheduleSyncLogIfAbsent(log *entity.SyncLogs) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var connector entity.Connector
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", log.ConnectorID).
			First(&connector).Error; err != nil {
			return err
		}

		var count int64
		if err := tx.Model(&entity.SyncLogs{}).
			Where("connector_id = ? AND kb_id = ? AND task_type = ? AND status = ?", log.ConnectorID, log.KbID, log.TaskType, string(entity.TaskStatusSchedule)).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}

		if err := tx.Model(&entity.Connector{}).
			Where("id = ?", log.ConnectorID).
			Update("status", string(entity.TaskStatusSchedule)).Error; err != nil {
			return err
		}
		return tx.Create(log).Error
	})
}

// IsRecordNotFound reports whether err is GORM's record-not-found sentinel.
func IsRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
