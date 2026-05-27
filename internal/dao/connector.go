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
	"time"

	"ragflow/internal/entity"

	"gorm.io/gorm"
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

// SyncLogItem is a flattened sync-log row joined with connector and knowledge base info.
// Mirrors the field list selected by Python's SyncLogsService.list_sync_tasks.
type SyncLogItem struct {
	ID                   string     `json:"id" gorm:"column:id"`
	ConnectorID          string     `json:"connector_id" gorm:"column:connector_id"`
	TaskType             string     `json:"task_type" gorm:"column:task_type"`
	KbID                 string     `json:"kb_id" gorm:"column:kb_id"`
	UpdateDate           *time.Time `json:"update_date" gorm:"column:update_date"`
	NewDocsIndexed       int64      `json:"new_docs_indexed" gorm:"column:new_docs_indexed"`
	TotalDocsIndexed     int64      `json:"total_docs_indexed" gorm:"column:total_docs_indexed"`
	DocsRemovedFromIndex int64      `json:"docs_removed_from_index" gorm:"column:docs_removed_from_index"`
	ErrorMsg             string     `json:"error_msg" gorm:"column:error_msg"`
	ErrorCount           int64      `json:"error_count" gorm:"column:error_count"`
	TimeStarted          *time.Time `json:"time_started" gorm:"column:time_started"`
	RefreshFreq          int64      `json:"refresh_freq" gorm:"column:refresh_freq"`
	PruneFreq            int64      `json:"prune_freq" gorm:"column:prune_freq"`
	KbName               string     `json:"kb_name" gorm:"column:kb_name"`
	Status               string     `json:"status" gorm:"column:status"`
}

// SyncLogsDAO sync logs data access object
type SyncLogsDAO struct{}

// NewSyncLogsDAO create sync logs DAO
func NewSyncLogsDAO() *SyncLogsDAO {
	return &SyncLogsDAO{}
}

// ListByConnectorID lists sync logs for a connector with pagination, mirroring
// Python's SyncLogsService.list_sync_tasks(connector_id, page, page_size).
// Returns the page of rows and the total count.
func (dao *SyncLogsDAO) ListByConnectorID(connectorID string, page, pageSize int) ([]*SyncLogItem, int64, error) {
	// withJoins builds the shared join + filter used by both the count and the page query.
	withJoins := func() *gorm.DB {
		return DB.Model(&entity.SyncLogs{}).
			Joins("JOIN connector ON sync_logs.connector_id = connector.id").
			Joins("JOIN connector2kb ON sync_logs.kb_id = connector2kb.kb_id").
			Joins("JOIN knowledgebase ON sync_logs.kb_id = knowledgebase.id").
			Where("sync_logs.connector_id = ?", connectorID)
	}

	var total int64
	if err := withJoins().Distinct("sync_logs.id").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query := withJoins().
		Select("sync_logs.id, sync_logs.connector_id, sync_logs.task_type, sync_logs.kb_id, "+
			"sync_logs.update_date, sync_logs.new_docs_indexed, sync_logs.total_docs_indexed, "+
			"sync_logs.docs_removed_from_index, sync_logs.error_msg, sync_logs.error_count, "+
			"sync_logs.time_started, connector.refresh_freq, connector.prune_freq, "+
			"knowledgebase.name AS kb_name, sync_logs.status").
		Distinct().
		Order("sync_logs.update_time DESC")
	if page > 0 {
		if pageSize <= 0 {
			pageSize = 15
		}
		query = query.Offset((page - 1) * pageSize).Limit(pageSize)
	}

	var logs []*SyncLogItem
	if err := query.Scan(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// HasScheduled reports whether a SCHEDULE-status task of the given type already
// exists for the connector/kb pair. Mirrors the guard in Python's SyncLogsService.schedule.
func (dao *SyncLogsDAO) HasScheduled(connectorID, kbID, taskType, scheduleStatus string) (bool, error) {
	var count int64
	err := DB.Model(&entity.SyncLogs{}).
		Where("connector_id = ? AND kb_id = ? AND status = ? AND task_type = ?",
			connectorID, kbID, scheduleStatus, taskType).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Create inserts a new sync log row.
func (dao *SyncLogsDAO) Create(log *entity.SyncLogs) error {
	return DB.Create(log).Error
}

// UpdateStatusByConnectorAndKb sets the status for the connector/kb tasks currently
// in any of fromStatuses. Mirrors the filter_update in Python's cancel_tasks.
func (dao *SyncLogsDAO) UpdateStatusByConnectorAndKb(connectorID, kbID string, fromStatuses []string, toStatus string) error {
	return DB.Model(&entity.SyncLogs{}).
		Where("connector_id = ? AND kb_id = ? AND status IN ?", connectorID, kbID, fromStatuses).
		Update("status", toStatus).Error
}

// UpdateStatusByConnector sets the status for all connector tasks currently in fromStatuses.
func (dao *SyncLogsDAO) UpdateStatusByConnector(connectorID string, fromStatuses []string, toStatus string) error {
	return DB.Model(&entity.SyncLogs{}).
		Where("connector_id = ? AND status IN ?", connectorID, fromStatuses).
		Update("status", toStatus).Error
}

// DeleteByConnectorAndKb removes all sync logs for the connector/kb pair.
func (dao *SyncLogsDAO) DeleteByConnectorAndKb(connectorID, kbID string) error {
	return DB.Where("connector_id = ? AND kb_id = ?", connectorID, kbID).
		Delete(&entity.SyncLogs{}).Error
}

// Connector2KbDAO connector-to-knowledgebase mapping data access object
type Connector2KbDAO struct{}

// NewConnector2KbDAO create connector2kb DAO
func NewConnector2KbDAO() *Connector2KbDAO {
	return &Connector2KbDAO{}
}

// ListByConnectorID returns the connector2kb mappings for a connector.
func (dao *Connector2KbDAO) ListByConnectorID(connectorID string) ([]*entity.Connector2Kb, error) {
	var mappings []*entity.Connector2Kb
	err := DB.Where("connector_id = ?", connectorID).Find(&mappings).Error
	if err != nil {
		return nil, err
	}
	return mappings, nil
}
