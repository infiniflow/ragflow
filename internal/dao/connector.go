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
	"ragflow/internal/entity"
	"ragflow/internal/utility"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ConnectorDAO connector data access object
type ConnectorDAO struct{}

var errConnectorNotAccessible = errors.New("connector is not accessible for this tenant")

// IsConnectorNotAccessibleErr reports connector ownership mismatches.
func IsConnectorNotAccessibleErr(err error) bool {
	return errors.Is(err, errConnectorNotAccessible)
}

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
func (dao *ConnectorDAO) ListByTenantID(ctx context.Context, db *gorm.DB, tenantID string) ([]*ConnectorListItem, error) {
	var connectors []*ConnectorListItem

	err := db.WithContext(ctx).Model(&entity.Connector{}).
		Select("id", "name", "source", "status").
		Where("tenant_id = ?", tenantID).
		Find(&connectors).Error

	if err != nil {
		return nil, err
	}

	return connectors, nil
}

// ListByDatasetID lists connectors linked to a dataset.
func (dao *ConnectorDAO) ListByDatasetID(ctx context.Context, db *gorm.DB, datasetID string) ([]*ConnectorDatasetListItem, error) {
	return dao.ListByDatasetIDTx(ctx, db, datasetID)
}

// ListByDatasetIDTx lists connectors linked to a dataset using the caller's DB handle.
func (dao *ConnectorDAO) ListByDatasetIDTx(ctx context.Context, db *gorm.DB, datasetID string) ([]*ConnectorDatasetListItem, error) {
	var connectors []*ConnectorDatasetListItem

	err := db.WithContext(ctx).Model(&entity.Connector2Kb{}).
		Select("connector.id, connector.source, connector.name, connector2kb.auto_parse, connector.status").
		Joins("JOIN connector ON connector2kb.connector_id = connector.id").
		Where("connector2kb.kb_id = ?", datasetID).
		Scan(&connectors).Error

	if err != nil {
		return nil, err
	}

	return connectors, nil
}

// DatasetConnectorLink is the connector relation payload accepted by dataset update.
type DatasetConnectorLink struct {
	ID        string
	AutoParse string
}

// LinkDatasetConnectors syncs connector2kb rows for a dataset.
func (dao *ConnectorDAO) LinkDatasetConnectors(ctx context.Context, db *gorm.DB, kbID string, connectors []DatasetConnectorLink) ([]string, error) {
	var taskIDs []string
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var kb entity.Knowledgebase
		if err := tx.WithContext(ctx).Select("tenant_id").Where("id = ? AND status = ?", kbID, string(entity.StatusValid)).First(&kb).Error; err != nil {
			return err
		}
		var err error
		taskIDs, err = dao.LinkDatasetConnectorsTx(ctx, tx, kbID, kb.TenantID, connectors)
		return err
	})
	return taskIDs, err
}

// LinkDatasetConnectorsTx syncs connector2kb rows using the caller's transaction.
func (dao *ConnectorDAO) LinkDatasetConnectorsTx(ctx context.Context, tx *gorm.DB, kbID, tenantID string, connectors []DatasetConnectorLink) ([]string, error) {
	var taskIDs []string
	var existing []entity.Connector2Kb
	if err := tx.WithContext(ctx).Where("kb_id = ?", kbID).Find(&existing).Error; err != nil {
		return nil, err
	}

	oldConnectorIDs := make(map[string]entity.Connector2Kb, len(existing))
	for _, row := range existing {
		oldConnectorIDs[row.ConnectorID] = row
	}

	nextConnectorIDs := make(map[string]struct{}, len(connectors))
	for _, connector := range connectors {
		nextConnectorIDs[connector.ID] = struct{}{}
		autoParse := connector.AutoParse
		if autoParse == "" {
			autoParse = "1"
		}

		var fullConnector entity.Connector
		if err := tx.WithContext(ctx).Where("id = ? AND tenant_id = ?", connector.ID, tenantID).First(&fullConnector).Error; err != nil {
			if IsNotFoundErr(err) {
				return nil, fmt.Errorf("%w: %s", errConnectorNotAccessible, connector.ID)
			}
			return nil, err
		}

		if _, ok := oldConnectorIDs[connector.ID]; ok {
			if err := tx.WithContext(ctx).Model(&entity.Connector2Kb{}).
				Where("connector_id = ? AND kb_id = ?", connector.ID, kbID).
				Update("auto_parse", autoParse).Error; err != nil {
				return nil, err
			}
			continue
		}

		if err := tx.WithContext(ctx).Create(&entity.Connector2Kb{
			ID:          utility.GenerateUUID(),
			ConnectorID: connector.ID,
			KbID:        kbID,
			AutoParse:   autoParse,
		}).Error; err != nil {
			return nil, err
		}

		taskID, err := scheduleConnectorTask(ctx, tx, connector.ID, kbID, connectorTaskTypeSync, true)
		if err != nil {
			return nil, err
		}
		taskIDs = append(taskIDs, taskID)

		if connectorConfigBool(fullConnector.Config, "sync_deleted_files") {
			taskID, err = scheduleConnectorTask(ctx, tx, connector.ID, kbID, connectorTaskTypePrune, false)
			if err != nil {
				return nil, err
			}
			taskIDs = append(taskIDs, taskID)
		}
	}

	for connectorID := range oldConnectorIDs {
		if _, ok := nextConnectorIDs[connectorID]; ok {
			continue
		}
		if err := tx.WithContext(ctx).Where("kb_id = ? AND connector_id = ?", kbID, connectorID).
			Delete(&entity.Connector2Kb{}).Error; err != nil {
			return nil, err
		}
		if err := tx.WithContext(ctx).Model(&entity.SyncLogs{}).
			Where("connector_id = ? AND kb_id = ? AND status IN ?", connectorID, kbID, []string{string(entity.TaskStatusSchedule), string(entity.TaskStatusRunning)}).
			Update("status", string(entity.TaskStatusCancel)).Error; err != nil {
			return nil, err
		}
	}

	return taskIDs, nil
}

// GetByID get connector by ID
func (dao *ConnectorDAO) GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.Connector, error) {
	var connector entity.Connector
	err := db.WithContext(ctx).Where("id = ?", id).First(&connector).Error
	if err != nil {
		return nil, err
	}
	return &connector, nil
}

// Create a new connector
func (dao *ConnectorDAO) Create(ctx context.Context, db *gorm.DB, connector *entity.Connector) error {
	return db.WithContext(ctx).Create(connector).Error
}

// UpdateByID update connector by ID
func (dao *ConnectorDAO) UpdateByID(ctx context.Context, db *gorm.DB, id string, updates map[string]interface{}) error {
	return db.WithContext(ctx).Model(&entity.Connector{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteByID delete connector by ID
func (dao *ConnectorDAO) DeleteByID(ctx context.Context, db *gorm.DB, id string) error {
	return db.WithContext(ctx).Where("id = ?", id).Delete(&entity.Connector{}).Error
}

// CancelRunningOrScheduledLogs marks active sync logs as canceled for a connector.
func (dao *ConnectorDAO) CancelRunningOrScheduledLogs(ctx context.Context, db *gorm.DB, connectorID string) error {
	return db.WithContext(ctx).Model(&entity.SyncLogs{}).
		Where("connector_id = ? AND status IN ?", connectorID, []string{string(entity.TaskStatusSchedule), string(entity.TaskStatusRunning)}).
		Update("status", string(entity.TaskStatusCancel)).Error
}

// ScheduleConnectorTasks schedules sync and optional prune tasks for a connector.
func (dao *ConnectorDAO) ScheduleConnectorTasks(ctx context.Context, db *gorm.DB, connectorID string) ([]string, error) {
	taskIDs := []string{}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var connector entity.Connector
		if err := tx.WithContext(ctx).Where("id = ?", connectorID).First(&connector).Error; err != nil {
			return err
		}

		var mappings []entity.Connector2Kb
		if err := tx.WithContext(ctx).Where("connector_id = ?", connectorID).Find(&mappings).Error; err != nil {
			return err
		}

		for _, mapping := range mappings {
			taskID, err := scheduleConnectorTask(ctx, tx, connectorID, mapping.KbID, connectorTaskTypeSync, false)
			if err != nil {
				return err
			}
			if taskID != "" {
				taskIDs = append(taskIDs, taskID)
			}
			if connectorConfigBool(connector.Config, "sync_deleted_files") {
				taskID, err = scheduleConnectorTask(ctx, tx, connectorID, mapping.KbID, connectorTaskTypePrune, false)
				if err != nil {
					return err
				}
				if taskID != "" {
					taskIDs = append(taskIDs, taskID)
				}
			}
		}

		return tx.WithContext(ctx).Model(&entity.Connector{}).
			Where("id = ?", connectorID).
			Update("status", string(entity.TaskStatusSchedule)).Error
	})
	return taskIDs, err
}

// Connector2KBExists reports whether the connector is bound to the knowledge base.
func (dao *ConnectorDAO) Connector2KBExists(ctx context.Context, db *gorm.DB, connectorID, kbID string) (bool, error) {
	var count int64
	err := db.WithContext(ctx).Model(&entity.Connector2Kb{}).
		Where("connector_id = ? AND kb_id = ?", connectorID, kbID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ListDocumentsByKBAndSourceType lists connector documents in a dataset.
func (dao *ConnectorDAO) ListDocumentsByKBAndSourceType(ctx context.Context, db *gorm.DB, kbID, sourceType string) ([]*entity.Document, error) {
	var documents []*entity.Document
	err := db.WithContext(ctx).Where("kb_id = ? AND source_type = ?", kbID, sourceType).Find(&documents).Error
	return documents, err
}

// RebuildConnector replaces old connector documents with scheduled sync tasks.
func (dao *ConnectorDAO) RebuildConnector(ctx context.Context, db *gorm.DB, connector *entity.Connector, kbID string, documents []*entity.Document) ([]string, []string, error) {
	var taskIDs []string
	var oldSyncTaskIDs []string
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Model(&entity.SyncLogs{}).
			Where("connector_id = ? AND kb_id = ? AND task_type = ?", connector.ID, kbID, connectorTaskTypeSync).
			Pluck("id", &oldSyncTaskIDs).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Where("connector_id = ? AND kb_id = ?", connector.ID, kbID).Delete(&entity.SyncLogs{}).Error; err != nil {
			return err
		}

		if len(documents) > 0 {
			docIDs := make([]string, 0, len(documents))
			var tokenNum int64
			var chunkNum int64
			for _, document := range documents {
				docIDs = append(docIDs, document.ID)
				tokenNum += document.TokenNum
				chunkNum += document.ChunkNum
			}

			var mappings []entity.File2Document
			if err := tx.WithContext(ctx).Where("document_id IN ?", docIDs).Find(&mappings).Error; err != nil {
				return err
			}
			fileIDs := make([]string, 0, len(mappings))
			seenFileIDs := make(map[string]struct{}, len(mappings))
			for _, mapping := range mappings {
				if mapping.FileID == nil || *mapping.FileID == "" {
					continue
				}
				if _, ok := seenFileIDs[*mapping.FileID]; ok {
					continue
				}
				seenFileIDs[*mapping.FileID] = struct{}{}
				fileIDs = append(fileIDs, *mapping.FileID)
			}

			if err := tx.WithContext(ctx).Where("doc_id IN ?", docIDs).Delete(&entity.Task{}).Error; err != nil {
				return err
			}
			if err := tx.WithContext(ctx).Where("document_id IN ?", docIDs).Delete(&entity.File2Document{}).Error; err != nil {
				return err
			}
			if len(fileIDs) > 0 {
				if err := tx.WithContext(ctx).Unscoped().
					Where("id IN ? AND source_type = ?", fileIDs, string(entity.FileSourceKnowledgebase)).
					Delete(&entity.File{}).Error; err != nil {
					return err
				}
			}
			if err := tx.WithContext(ctx).Where("id IN ?", docIDs).Delete(&entity.Document{}).Error; err != nil {
				return err
			}
			if err := tx.WithContext(ctx).Model(&entity.Knowledgebase{}).
				Where("id = ?", kbID).
				Updates(map[string]interface{}{
					"doc_num":   gorm.Expr("doc_num - ?", len(docIDs)),
					"token_num": gorm.Expr("token_num - ?", tokenNum),
					"chunk_num": gorm.Expr("chunk_num - ?", chunkNum),
				}).Error; err != nil {
				return err
			}
		}

		if err := tx.WithContext(ctx).Model(&entity.Connector{}).
			Where("id = ?", connector.ID).
			Update("status", string(entity.TaskStatusSchedule)).Error; err != nil {
			return err
		}

		taskID, err := createRebuildSyncLog(ctx, tx, connector.ID, kbID, connectorTaskTypeSync, true)
		if err != nil {
			return err
		}
		taskIDs = append(taskIDs, taskID)
		if syncDeletedFiles, _ := connector.Config["sync_deleted_files"].(bool); syncDeletedFiles {
			taskID, err = createRebuildSyncLog(ctx, tx, connector.ID, kbID, connectorTaskTypePrune, false)
			if err != nil {
				return err
			}
			taskIDs = append(taskIDs, taskID)
		}
		return nil
	})
	return taskIDs, oldSyncTaskIDs, err
}

// ResumeFailedSyncTask schedules one failed sync task for checkpoint resume.
func (dao *ConnectorDAO) ResumeFailedSyncTask(ctx context.Context, db *gorm.DB, connectorID, kbID, taskID string) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task entity.SyncLogs
		query := tx.WithContext(ctx)
		if tx.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.
			Where("id = ? AND connector_id = ? AND kb_id = ? AND task_type = ? AND status = ?", taskID, connectorID, kbID, connectorTaskTypeSync, string(entity.TaskStatusFail)).
			First(&task).Error; err != nil {
			return err
		}

		now := time.Now().Local()
		if err := tx.WithContext(ctx).Model(&entity.SyncLogs{}).
			Where("id = ? AND status = ?", taskID, string(entity.TaskStatusFail)).
			Updates(map[string]any{
				"status":       string(entity.TaskStatusSchedule),
				"error_msg":    "",
				"error_count":  int64(0),
				"time_started": &now,
			}).Error; err != nil {
			return err
		}
		return tx.WithContext(ctx).Model(&entity.Connector{}).
			Where("id = ?", connectorID).
			Update("status", string(entity.TaskStatusSchedule)).Error
	})
}

const (
	connectorTaskTypeSync  = "sync"
	connectorTaskTypePrune = "prune"
)

func createRebuildSyncLog(ctx context.Context, tx *gorm.DB, connectorID, kbID, taskType string, reindex bool) (string, error) {
	fromBeginning := "0"
	if reindex {
		fromBeginning = "1"
	}
	now := time.Now().Local()
	taskID := utility.GenerateToken()
	return taskID, tx.WithContext(ctx).Create(&entity.SyncLogs{
		ID:               taskID,
		ConnectorID:      connectorID,
		KbID:             kbID,
		TaskType:         taskType,
		Status:           string(entity.TaskStatusSchedule),
		FromBeginning:    &fromBeginning,
		TimeStarted:      &now,
		ErrorMsg:         "",
		TotalDocsIndexed: 0,
	}).Error
}

func scheduleConnectorTask(ctx context.Context, tx *gorm.DB, connectorID, kbID, taskType string, reindex bool) (string, error) {
	var scheduled entity.SyncLogs
	err := tx.WithContext(ctx).
		Where("connector_id = ? AND kb_id = ? AND task_type = ? AND status = ?", connectorID, kbID, taskType, string(entity.TaskStatusSchedule)).
		Order("update_time DESC").
		First(&scheduled).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	if err == nil {
		return scheduled.ID, nil
	}

	var running int64
	if err := tx.WithContext(ctx).Model(&entity.SyncLogs{}).
		Where("connector_id = ? AND kb_id = ? AND task_type = ? AND status = ?", connectorID, kbID, taskType, string(entity.TaskStatusRunning)).
		Count(&running).Error; err != nil {
		return "", err
	}
	if running > 0 {
		return "", nil
	}

	var pollRangeStart *time.Time
	var totalDocsIndexed int64
	if taskType == connectorTaskTypeSync {
		var latest entity.SyncLogs
		err := tx.WithContext(ctx).Where("connector_id = ? AND kb_id = ? AND task_type = ? AND status = ?", connectorID, kbID, taskType, string(entity.TaskStatusDone)).
			Order("update_time DESC").
			First(&latest).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
		if err == nil {
			totalDocsIndexed = latest.TotalDocsIndexed
			if latest.PollRangeEnd != nil {
				pollRangeEnd := latest.PollRangeEnd.Time()
				pollRangeStart = &pollRangeEnd
			}
		}
	}

	fromBeginning := "0"
	if reindex {
		fromBeginning = "1"
	}
	now := time.Now().Local()
	taskID := utility.GenerateToken()
	return taskID, tx.WithContext(ctx).Create(&entity.SyncLogs{
		ID:               taskID,
		ConnectorID:      connectorID,
		KbID:             kbID,
		TaskType:         taskType,
		Status:           string(entity.TaskStatusSchedule),
		FromBeginning:    &fromBeginning,
		PollRangeStart:   entity.NewFlexibleTime(pollRangeStart),
		TimeStarted:      &now,
		ErrorMsg:         "",
		TotalDocsIndexed: totalDocsIndexed,
	}).Error
}

func connectorConfigBool(config map[string]interface{}, key string) bool {
	value, ok := config[key]
	if !ok {
		return false
	}

	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed == "1" || typed == "true" || typed == "TRUE"
	default:
		return false
	}
}

// ListLogsByConnectorID lists sync logs for one connector with pagination.
func (dao *ConnectorDAO) ListLogsByConnectorID(ctx context.Context, db *gorm.DB, connectorID string, offset, limit int) ([]*entity.ConnectorSyncLog, int64, error) {
	baseQuery := db.WithContext(ctx).Model(&entity.SyncLogs{}).
		Joins("JOIN connector ON sync_logs.connector_id = connector.id").
		Joins("JOIN connector2kb ON sync_logs.connector_id = connector2kb.connector_id AND sync_logs.kb_id = connector2kb.kb_id").
		Joins("JOIN knowledgebase ON sync_logs.kb_id = knowledgebase.id").
		Where("sync_logs.connector_id = ?", connectorID)

	var total int64
	if err := baseQuery.Distinct("sync_logs.id").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var logs []*entity.ConnectorSyncLog
	err := baseQuery.
		Select(
			"sync_logs.id",
			"sync_logs.connector_id",
			"sync_logs.task_type",
			"sync_logs.kb_id",
			"sync_logs.update_date",
			"sync_logs.new_docs_indexed",
			"sync_logs.total_docs_indexed",
			"sync_logs.docs_removed_from_index",
			"sync_logs.error_msg",
			"sync_logs.error_count",
			"sync_logs.time_started",
			"connector.refresh_freq AS refresh_freq",
			"connector.prune_freq AS prune_freq",
			"knowledgebase.name AS kb_name",
			"sync_logs.status",
		).
		Distinct().
		Order("sync_logs.update_date DESC").
		Offset(offset).
		Limit(limit).
		Scan(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// ListLogs lists sync logs for the given tenant IDs with pagination.
// When datasetID is non-empty, only logs of that dataset are returned.
func (dao *ConnectorDAO) ListLogs(ctx context.Context, db *gorm.DB, tenantIDs []string, datasetID string, offset, limit int) ([]*entity.ConnectorSyncLog, int64, error) {
	baseQuery := db.WithContext(ctx).Model(&entity.SyncLogs{}).
		Joins("JOIN connector ON sync_logs.connector_id = connector.id").
		Joins("JOIN connector2kb ON sync_logs.connector_id = connector2kb.connector_id AND sync_logs.kb_id = connector2kb.kb_id").
		Joins("JOIN knowledgebase ON sync_logs.kb_id = knowledgebase.id").
		Where("connector.tenant_id IN ?", tenantIDs)

	if datasetID != "" {
		baseQuery = baseQuery.Where("sync_logs.kb_id = ?", datasetID)
	}

	var total int64
	if err := baseQuery.Distinct("sync_logs.id").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var logs []*entity.ConnectorSyncLog
	query := baseQuery.
		Select(
			"sync_logs.id",
			"sync_logs.connector_id",
			"sync_logs.task_type",
			"sync_logs.kb_id",
			"sync_logs.update_date",
			"sync_logs.new_docs_indexed",
			"sync_logs.total_docs_indexed",
			"sync_logs.docs_removed_from_index",
			"sync_logs.error_msg",
			"sync_logs.error_count",
			"sync_logs.time_started",
			"connector.refresh_freq AS refresh_freq",
			"connector.prune_freq AS prune_freq",
			"knowledgebase.name AS kb_name",
			"sync_logs.status",
		).
		Distinct().
		Order("sync_logs.update_date DESC")
	if limit > 0 {
		query = query.Offset(offset).Limit(limit)
	}
	err := query.Scan(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}
