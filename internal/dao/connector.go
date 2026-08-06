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
func (dao *ConnectorDAO) LinkDatasetConnectors(ctx context.Context, db *gorm.DB, kbID string, connectors []DatasetConnectorLink) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var kb entity.Knowledgebase
		if err := db.WithContext(ctx).Select("tenant_id").Where("id = ? AND status = ?", kbID, string(entity.StatusValid)).First(&kb).Error; err != nil {
			return err
		}
		return dao.LinkDatasetConnectorsTx(ctx, tx, kbID, kb.TenantID, connectors)
	})
}

// LinkDatasetConnectorsTx syncs connector2kb rows using the caller's transaction.
func (dao *ConnectorDAO) LinkDatasetConnectorsTx(ctx context.Context, tx *gorm.DB, kbID, tenantID string, connectors []DatasetConnectorLink) error {
	var existing []entity.Connector2Kb
	if err := tx.WithContext(ctx).Where("kb_id = ?", kbID).Find(&existing).Error; err != nil {
		return err
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
				return fmt.Errorf("%w: %s", errConnectorNotAccessible, connector.ID)
			}
			return err
		}

		if _, ok := oldConnectorIDs[connector.ID]; ok {
			if err := tx.WithContext(ctx).Model(&entity.Connector2Kb{}).
				Where("connector_id = ? AND kb_id = ?", connector.ID, kbID).
				Update("auto_parse", autoParse).Error; err != nil {
				return err
			}
			continue
		}

		if err := tx.WithContext(ctx).Create(&entity.Connector2Kb{
			ID:          utility.GenerateUUID(),
			ConnectorID: connector.ID,
			KbID:        kbID,
			AutoParse:   autoParse,
		}).Error; err != nil {
			return err
		}

		if err := scheduleConnectorTask(ctx, tx, connector.ID, kbID, connectorTaskTypeSync, true); err != nil {
			return err
		}

		if connectorConfigBool(fullConnector.Config, "sync_deleted_files") {
			if err := scheduleConnectorTask(ctx, tx, connector.ID, kbID, connectorTaskTypePrune, false); err != nil {
				return err
			}
		}
	}

	for connectorID := range oldConnectorIDs {
		if _, ok := nextConnectorIDs[connectorID]; ok {
			continue
		}
		if err := tx.WithContext(ctx).Where("kb_id = ? AND connector_id = ?", kbID, connectorID).
			Delete(&entity.Connector2Kb{}).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Model(&entity.SyncLogs{}).
			Where("connector_id = ? AND kb_id = ? AND status IN ?", connectorID, kbID, []string{string(entity.TaskStatusSchedule), string(entity.TaskStatusRunning)}).
			Update("status", string(entity.TaskStatusCancel)).Error; err != nil {
			return err
		}
	}

	return nil
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
func (dao *ConnectorDAO) ScheduleConnectorTasks(ctx context.Context, db *gorm.DB, connectorID string) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var connector entity.Connector
		if err := tx.WithContext(ctx).Where("id = ?", connectorID).First(&connector).Error; err != nil {
			return err
		}

		var mappings []entity.Connector2Kb
		if err := tx.WithContext(ctx).Where("connector_id = ?", connectorID).Find(&mappings).Error; err != nil {
			return err
		}

		for _, mapping := range mappings {
			if err := scheduleConnectorTask(ctx, tx, connectorID, mapping.KbID, connectorTaskTypeSync, false); err != nil {
				return err
			}
			if connectorConfigBool(connector.Config, "sync_deleted_files") {
				if err := scheduleConnectorTask(ctx, tx, connectorID, mapping.KbID, connectorTaskTypePrune, false); err != nil {
					return err
				}
			}
		}

		return tx.WithContext(ctx).Model(&entity.Connector{}).
			Where("id = ?", connectorID).
			Update("status", string(entity.TaskStatusSchedule)).Error
	})
}

// ListDocumentsByKBAndSourceType lists connector documents in a dataset.
func (dao *ConnectorDAO) ListDocumentsByKBAndSourceType(ctx context.Context, db *gorm.DB, kbID, sourceType string) ([]*entity.Document, error) {
	var documents []*entity.Document
	err := db.WithContext(ctx).Where("kb_id = ? AND source_type = ?", kbID, sourceType).Find(&documents).Error
	return documents, err
}

// RebuildConnector replaces old connector documents with scheduled sync tasks.
func (dao *ConnectorDAO) RebuildConnector(ctx context.Context, db *gorm.DB, connector *entity.Connector, kbID string, documents []*entity.Document) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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

		if err := createRebuildSyncLog(ctx, tx, connector.ID, kbID, connectorTaskTypeSync, true); err != nil {
			return err
		}
		if syncDeletedFiles, _ := connector.Config["sync_deleted_files"].(bool); syncDeletedFiles {
			if err := createRebuildSyncLog(ctx, tx, connector.ID, kbID, connectorTaskTypePrune, false); err != nil {
				return err
			}
		}
		return nil
	})
}

const (
	connectorTaskTypeSync  = "sync"
	connectorTaskTypePrune = "prune"
)

// DueSyncTask is the joined projection of one due sync_logs row with its
// connector, connector2kb link and knowledgebase, mirroring the task dicts
// produced by Python SyncLogsService._list_due_tasks_for_freq.
type DueSyncTask struct {
	ID               string         `gorm:"column:id"`
	ConnectorID      string         `gorm:"column:connector_id"`
	TaskType         string         `gorm:"column:task_type"`
	KbID             string         `gorm:"column:kb_id"`
	PollRangeStart   *time.Time     `gorm:"column:poll_range_start"`
	PollRangeEnd     *time.Time     `gorm:"column:poll_range_end"`
	NewDocsIndexed   int64          `gorm:"column:new_docs_indexed"`
	TotalDocsIndexed int64          `gorm:"column:total_docs_indexed"`
	FromBeginning    *string        `gorm:"column:from_beginning"`
	ConnectorName    string         `gorm:"column:connector_name"`
	Source           string         `gorm:"column:source"`
	TenantID         string         `gorm:"column:tenant_id"`
	TimeoutSecs      int64          `gorm:"column:timeout_secs"`
	Config           entity.JSONMap `gorm:"column:config"`
	RefreshFreq      int64          `gorm:"column:refresh_freq"`
	PruneFreq        int64          `gorm:"column:prune_freq"`
	KbName           string         `gorm:"column:kb_name"`
	AutoParse        string         `gorm:"column:auto_parse"`
}

// ListDueTasks returns scheduled sync/prune tasks whose connector is scheduled
// and whose last update is older than the given frequency (minutes), mirroring
// Python SyncLogsService._list_due_tasks_for_freq (MySQL dialect).
func (dao *ConnectorDAO) ListDueTasks(ctx context.Context, db *gorm.DB, taskType, freqField string) ([]*DueSyncTask, error) {
	var tasks []*DueSyncTask
	err := db.WithContext(ctx).Model(&entity.SyncLogs{}).
		Select(
			"sync_logs.id",
			"sync_logs.connector_id",
			"sync_logs.task_type",
			"sync_logs.kb_id",
			"sync_logs.poll_range_start",
			"sync_logs.poll_range_end",
			"sync_logs.new_docs_indexed",
			"sync_logs.total_docs_indexed",
			"sync_logs.from_beginning",
			"connector.name AS connector_name",
			"connector.source",
			"connector.tenant_id",
			"connector.timeout_secs",
			"connector.config",
			"connector.refresh_freq",
			"connector.prune_freq",
			"knowledgebase.name AS kb_name",
			"connector2kb.auto_parse",
		).
		Joins("JOIN connector ON sync_logs.connector_id = connector.id").
		Joins("JOIN connector2kb ON sync_logs.connector_id = connector2kb.connector_id AND sync_logs.kb_id = connector2kb.kb_id").
		Joins("JOIN knowledgebase ON sync_logs.kb_id = knowledgebase.id").
		Where("connector.input_type = ?", "poll").
		Where("connector.status = ?", string(entity.TaskStatusSchedule)).
		Where("sync_logs.status = ?", string(entity.TaskStatusSchedule)).
		Where("sync_logs.task_type = ?", taskType).
		Where("sync_logs.update_date < NOW() - INTERVAL connector." + freqField + " MINUTE").
		Distinct().
		Order("sync_logs.update_time DESC").
		Scan(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// ClaimTask atomically transitions a scheduled task to running and mirrors the
// status onto the connector, mirroring Python SyncLogsService.start. It
// returns false when another worker claimed the task first.
func (dao *ConnectorDAO) ClaimTask(ctx context.Context, db *gorm.DB, taskID, connectorID string) (bool, error) {
	now := time.Now().Local()
	result := db.WithContext(ctx).Model(&entity.SyncLogs{}).
		Where("id = ? AND status = ?", taskID, string(entity.TaskStatusSchedule)).
		Updates(map[string]interface{}{
			"status":       string(entity.TaskStatusRunning),
			"time_started": now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	if err := db.WithContext(ctx).Model(&entity.Connector{}).
		Where("id = ?", connectorID).
		Update("status", string(entity.TaskStatusRunning)).Error; err != nil {
		return false, err
	}
	return true, nil
}

// IncreaseDocs accumulates per-batch sync progress, keeping the poll window
// monotonic, mirroring Python SyncLogsService.increase_docs.
func (dao *ConnectorDAO) IncreaseDocs(ctx context.Context, db *gorm.DB, taskID string, maxUpdate time.Time, docNum int64, errMsg string, errCount int64) error {
	return db.WithContext(ctx).Model(&entity.SyncLogs{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"new_docs_indexed":   gorm.Expr("new_docs_indexed + ?", docNum),
			"total_docs_indexed": gorm.Expr("total_docs_indexed + ?", docNum),
			"poll_range_start":   gorm.Expr("COALESCE(GREATEST(poll_range_start, ?), ?)", maxUpdate, maxUpdate),
			"poll_range_end":     gorm.Expr("COALESCE(GREATEST(poll_range_end, ?), ?)", maxUpdate, maxUpdate),
			"error_msg":          gorm.Expr("CONCAT(error_msg, ?)", errMsg),
			"error_count":        gorm.Expr("error_count + ?", errCount),
		}).Error
}

// IncreaseRemovedDocs accumulates per-task prune progress, mirroring Python
// SyncLogsService.increase_removed_docs.
func (dao *ConnectorDAO) IncreaseRemovedDocs(ctx context.Context, db *gorm.DB, taskID string, removedCount int64, errMsg string, errCount int64) error {
	return db.WithContext(ctx).Model(&entity.SyncLogs{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"docs_removed_from_index": gorm.Expr("docs_removed_from_index + ?", removedCount),
			"error_msg":               gorm.Expr("CONCAT(error_msg, ?)", errMsg),
			"error_count":             gorm.Expr("error_count + ?", errCount),
		}).Error
}

// FinishTask marks a task (and, on success, the connector) with a terminal
// status, mirroring Python SyncLogsService.done / the failure updates in
// rag/svr/sync_data_source.py.
func (dao *ConnectorDAO) FinishTask(ctx context.Context, db *gorm.DB, taskID, connectorID string, status entity.TaskStatus, errMsg, trace string) error {
	updates := map[string]interface{}{
		"status": string(status),
	}
	if errMsg != "" {
		updates["error_msg"] = errMsg
	}
	if trace != "" {
		updates["full_exception_trace"] = trace
	}
	if err := db.WithContext(ctx).Model(&entity.SyncLogs{}).
		Where("id = ?", taskID).
		Updates(updates).Error; err != nil {
		return err
	}
	if status == entity.TaskStatusDone {
		return db.WithContext(ctx).Model(&entity.Connector{}).
			Where("id = ?", connectorID).
			Update("status", string(entity.TaskStatusDone)).Error
	}
	return nil
}

// RescheduleTask inserts the next scheduled task for a connector/kb pair
// (carrying over the poll window from the latest done task) and marks the
// connector scheduled, mirroring Python SyncLogsService.schedule.
func (dao *ConnectorDAO) RescheduleTask(ctx context.Context, db *gorm.DB, connectorID, kbID, taskType string) error {
	if err := dao.pruneOldSyncLogs(ctx, db, connectorID, kbID); err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := scheduleConnectorTask(ctx, tx, connectorID, kbID, taskType, false); err != nil {
			return err
		}
		return tx.WithContext(ctx).Model(&entity.Connector{}).
			Where("id = ?", connectorID).
			Update("status", string(entity.TaskStatusSchedule)).Error
	})
}

// pruneOldSyncLogs caps the log history per connector/kb pair at 100 rows by
// deleting the oldest 70, mirroring Python SyncLogsService.schedule.
func (dao *ConnectorDAO) pruneOldSyncLogs(ctx context.Context, db *gorm.DB, connectorID, kbID string) error {
	var total int64
	if err := db.WithContext(ctx).Model(&entity.SyncLogs{}).
		Where("connector_id = ? AND kb_id = ?", connectorID, kbID).
		Count(&total).Error; err != nil {
		return err
	}
	if total <= 100 {
		return nil
	}
	var ids []string
	if err := db.WithContext(ctx).Model(&entity.SyncLogs{}).
		Where("connector_id = ? AND kb_id = ?", connectorID, kbID).
		Order("update_time ASC").
		Limit(70).
		Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	return db.WithContext(ctx).Where("id IN ?", ids).Delete(&entity.SyncLogs{}).Error
}

func createRebuildSyncLog(ctx context.Context, tx *gorm.DB, connectorID, kbID, taskType string, reindex bool) error {
	fromBeginning := "0"
	if reindex {
		fromBeginning = "1"
	}
	now := time.Now().Local()
	row := &entity.SyncLogs{
		ID:               utility.GenerateToken(),
		ConnectorID:      connectorID,
		KbID:             kbID,
		TaskType:         taskType,
		Status:           string(entity.TaskStatusSchedule),
		FromBeginning:    &fromBeginning,
		TimeStarted:      &now,
		ErrorMsg:         "",
		TotalDocsIndexed: 0,
	}
	if reindex {
		// A from-beginning sync is due immediately (Python run_immediately=True).
		row.UpdateTime = new(int64)
		epoch := time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local)
		row.UpdateDate = &epoch
	}
	return tx.WithContext(ctx).Create(row).Error
}

func scheduleConnectorTask(ctx context.Context, tx *gorm.DB, connectorID, kbID, taskType string, reindex bool) error {
	var existing int64
	if err := tx.WithContext(ctx).Model(&entity.SyncLogs{}).
		Where("connector_id = ? AND kb_id = ? AND task_type = ? AND status = ?", connectorID, kbID, taskType, string(entity.TaskStatusSchedule)).
		Count(&existing).Error; err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}

	var pollRangeStart *time.Time
	var totalDocsIndexed int64
	if taskType == connectorTaskTypeSync {
		var latest entity.SyncLogs
		err := tx.WithContext(ctx).Where("connector_id = ? AND kb_id = ? AND task_type = ? AND status = ?", connectorID, kbID, taskType, string(entity.TaskStatusDone)).
			Order("update_time DESC").
			First(&latest).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			pollRangeStart = latest.PollRangeEnd
			totalDocsIndexed = latest.TotalDocsIndexed
		}
	}

	fromBeginning := "0"
	if reindex {
		fromBeginning = "1"
	}
	now := time.Now().Local()
	row := &entity.SyncLogs{
		ID:               utility.GenerateToken(),
		ConnectorID:      connectorID,
		KbID:             kbID,
		TaskType:         taskType,
		Status:           string(entity.TaskStatusSchedule),
		FromBeginning:    &fromBeginning,
		PollRangeStart:   pollRangeStart,
		TimeStarted:      &now,
		ErrorMsg:         "",
		TotalDocsIndexed: totalDocsIndexed,
	}
	if reindex {
		// A from-beginning sync is due immediately (Python run_immediately=True).
		row.UpdateTime = new(int64)
		epoch := time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local)
		row.UpdateDate = &epoch
	}
	return tx.WithContext(ctx).Create(row).Error
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
