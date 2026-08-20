//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
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

const (
	// TaskTypeSync is the Python-compatible SYNC task type.
	TaskTypeSync = "sync"
	// TaskTypePrune is the Python-compatible PRUNE task type.
	TaskTypePrune = "prune"
	// SyncStatusSchedule is the Python TaskStatus.SCHEDULE value for sync_logs.
	SyncStatusSchedule = "5"
	// SyncStatusRunning is the Python TaskStatus.RUNNING value for sync_logs.
	SyncStatusRunning = "1"
	// SyncStatusCancel is the Python TaskStatus.CANCEL value for sync_logs.
	SyncStatusCancel = "2"
	// SyncStatusDone is the Python TaskStatus.DONE value for sync_logs.
	SyncStatusDone = "3"
	// SyncStatusFail is the Python TaskStatus.FAIL value for sync_logs.
	SyncStatusFail = "4"
)

// SyncTask is the Python-compatible sync task row.
type SyncTask struct {
	entity.SyncLogs `gorm:"embedded"`
}

// TableName returns the Python-compatible sync task table.
func (SyncTask) TableName() string {
	return "sync_logs"
}

// SyncTaskContext contains every database row required to execute one task.
type SyncTaskContext struct {
	Task          entity.SyncLogs
	Connector     entity.Connector
	Connector2Kb  entity.Connector2Kb
	Knowledgebase entity.Knowledgebase
}

// SyncTaskDAO reads and updates sync_logs tasks.
type SyncTaskDAO struct {
	db *gorm.DB
}

// NewSyncTaskDAO creates a syncer task DAO.
func NewSyncTaskDAO(db *gorm.DB) *SyncTaskDAO {
	if db == nil {
		db = GetDB()
	}
	return &SyncTaskDAO{db: db}
}

// DB returns the DAO database handle.
func (d *SyncTaskDAO) DB() *gorm.DB {
	return d.db
}

type dueSyncTaskRow struct {
	entity.SyncLogs
	ConnectorRefreshFreq int64          `gorm:"column:connector_refresh_freq"`
	ConnectorPruneFreq   int64          `gorm:"column:connector_prune_freq"`
	ConnectorConfig      entity.JSONMap `gorm:"column:connector_config"`
}

// ScheduledSyncTask contains one scheduled task and its connector scheduling settings.
type ScheduledSyncTask struct {
	entity.SyncLogs
	ConnectorRefreshFreq int64
	ConnectorPruneFreq   int64
	ConnectorConfig      entity.JSONMap
}

// ScheduledSyncTaskCursor identifies the last row from a scheduled task page.
type ScheduledSyncTaskCursor struct {
	UpdateTime int64
	ID         string
}

// Cursor returns the keyset cursor for the task.
func (t ScheduledSyncTask) Cursor() ScheduledSyncTaskCursor {
	updateTime := int64(0)
	if t.UpdateTime != nil {
		updateTime = *t.UpdateTime
	}
	return ScheduledSyncTaskCursor{UpdateTime: updateTime, ID: t.ID}
}

// ListScheduledTasks returns one page of scheduled tasks with connector scheduling settings.
func (d *SyncTaskDAO) ListScheduledTasks(ctx context.Context, limit int, cursor *ScheduledSyncTaskCursor) ([]ScheduledSyncTask, error) {
	var rows []dueSyncTaskRow
	query := d.db.WithContext(ctx).
		Model(&entity.SyncLogs{}).
		Select("sync_logs.*, connector.refresh_freq AS connector_refresh_freq, connector.prune_freq AS connector_prune_freq, connector.config AS connector_config").
		Joins("JOIN connector ON sync_logs.connector_id = connector.id").
		Joins("JOIN connector2kb ON sync_logs.connector_id = connector2kb.connector_id AND sync_logs.kb_id = connector2kb.kb_id").
		Joins("JOIN knowledgebase ON sync_logs.kb_id = knowledgebase.id").
		Where("sync_logs.status = ? AND connector.status = ? AND sync_logs.task_type IN ?", SyncStatusSchedule, SyncStatusSchedule, []string{TaskTypeSync, TaskTypePrune})
	if cursor != nil {
		query = query.Where("COALESCE(sync_logs.update_time, 0) < ? OR (COALESCE(sync_logs.update_time, 0) = ? AND sync_logs.id < ?)", cursor.UpdateTime, cursor.UpdateTime, cursor.ID)
	}
	if err := query.
		Order("COALESCE(sync_logs.update_time, 0) DESC, sync_logs.id DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	tasks := make([]ScheduledSyncTask, 0, len(rows))
	for _, row := range rows {
		tasks = append(tasks, ScheduledSyncTask{
			SyncLogs:             row.SyncLogs,
			ConnectorRefreshFreq: row.ConnectorRefreshFreq,
			ConnectorPruneFreq:   row.ConnectorPruneFreq,
			ConnectorConfig:      row.ConnectorConfig,
		})
	}
	return tasks, nil
}

// GetScheduledTask returns one scheduled task with connector scheduling settings.
func (d *SyncTaskDAO) GetScheduledTask(ctx context.Context, taskID string) (ScheduledSyncTask, error) {
	var row dueSyncTaskRow
	if err := d.db.WithContext(ctx).
		Model(&entity.SyncLogs{}).
		Select("sync_logs.*, connector.refresh_freq AS connector_refresh_freq, connector.prune_freq AS connector_prune_freq, connector.config AS connector_config").
		Joins("JOIN connector ON sync_logs.connector_id = connector.id").
		Joins("JOIN connector2kb ON sync_logs.connector_id = connector2kb.connector_id AND sync_logs.kb_id = connector2kb.kb_id").
		Joins("JOIN knowledgebase ON sync_logs.kb_id = knowledgebase.id").
		Where("sync_logs.id = ? AND sync_logs.status = ? AND connector.status = ? AND sync_logs.task_type IN ?", taskID, SyncStatusSchedule, SyncStatusSchedule, []string{TaskTypeSync, TaskTypePrune}).
		First(&row).Error; err != nil {
		return ScheduledSyncTask{}, err
	}
	return ScheduledSyncTask{
		SyncLogs:             row.SyncLogs,
		ConnectorRefreshFreq: row.ConnectorRefreshFreq,
		ConnectorPruneFreq:   row.ConnectorPruneFreq,
		ConnectorConfig:      row.ConnectorConfig,
	}, nil
}

// ClaimTask conditionally marks a scheduled task as running.
func (d *SyncTaskDAO) ClaimTask(ctx context.Context, taskID string, now time.Time) (bool, error) {
	var claimed bool
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task entity.SyncLogs
		query := tx.WithContext(ctx)
		if tx.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.Where("id = ?", taskID).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if task.Status != SyncStatusSchedule {
			return nil
		}

		var mapping entity.Connector2Kb
		lockQuery := tx.WithContext(ctx)
		if tx.Dialector.Name() != "sqlite" {
			lockQuery = lockQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := lockQuery.
			Where("connector_id = ? AND kb_id = ?", task.ConnectorID, task.KbID).
			First(&mapping).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var running int64
		if err := tx.WithContext(ctx).Model(&entity.SyncLogs{}).
			Where("id <> ? AND connector_id = ? AND kb_id = ? AND status = ? AND task_type IN ?", taskID, task.ConnectorID, task.KbID, SyncStatusRunning, []string{TaskTypeSync, TaskTypePrune}).
			Count(&running).Error; err != nil {
			return err
		}
		if running > 0 {
			return nil
		}

		result := tx.Model(&entity.SyncLogs{}).
			Where("id = ? AND status = ?", taskID, SyncStatusSchedule).
			Updates(map[string]any{"status": SyncStatusRunning, "time_started": now})
		if result.Error != nil {
			return result.Error
		}
		claimed = result.RowsAffected == 1
		return nil
	})
	return claimed, err
}

// GetTaskContext loads a task with connector, mapping, and knowledgebase rows.
func (d *SyncTaskDAO) GetTaskContext(ctx context.Context, taskID string) (SyncTaskContext, error) {
	var task entity.SyncLogs
	if err := d.db.WithContext(ctx).Where("id = ?", taskID).First(&task).Error; err != nil {
		return SyncTaskContext{}, err
	}
	// get connector
	var connector entity.Connector
	if err := d.db.WithContext(ctx).Where("id = ?", task.ConnectorID).First(&connector).Error; err != nil {
		return SyncTaskContext{}, err
	}
	// get relation
	var connector2Kb entity.Connector2Kb
	if err := d.db.WithContext(ctx).Where("connector_id = ? AND kb_id = ?", task.ConnectorID, task.KbID).First(&connector2Kb).Error; err != nil {
		return SyncTaskContext{}, err
	}
	// get KB
	var kb entity.Knowledgebase
	if err := d.db.WithContext(ctx).Where("id = ?", task.KbID).First(&kb).Error; err != nil {
		return SyncTaskContext{}, err
	}

	return SyncTaskContext{Task: task, Connector: connector, Connector2Kb: connector2Kb, Knowledgebase: kb}, nil
}

// IsTaskCanceled reports whether a sync_logs task has been canceled.
func (d *SyncTaskDAO) IsTaskCanceled(ctx context.Context, taskID string) (bool, error) {
	var task entity.SyncLogs
	if err := d.db.WithContext(ctx).Select("status").Where("id = ?", taskID).First(&task).Error; err != nil {
		return false, err
	}
	return task.Status == SyncStatusCancel, nil
}

// MarkConnectorRunning marks a connector running.
func (d *SyncTaskDAO) MarkConnectorRunning(ctx context.Context, connectorID string) error {
	return d.db.WithContext(ctx).Model(&entity.Connector{}).Where("id = ?", connectorID).Update("status", SyncStatusRunning).Error
}

// RescheduleClaimed puts a claimed task back into schedule state.
func (d *SyncTaskDAO) RescheduleClaimed(ctx context.Context, taskID string) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task entity.SyncLogs
		if err := tx.WithContext(ctx).Where("id = ? AND status = ?", taskID, SyncStatusRunning).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if err := tx.Model(&entity.SyncLogs{}).
			Where("id = ? AND status = ?", taskID, SyncStatusRunning).
			Update("status", SyncStatusSchedule).Error; err != nil {
			return err
		}
		return tx.Model(&entity.Connector{}).Where("id = ? AND status = ?", task.ConnectorID, SyncStatusRunning).Update("status", SyncStatusSchedule).Error
	})
}

// FailTask marks a task failed without advancing its poll waterline.
func (d *SyncTaskDAO) FailTask(ctx context.Context, taskID, connectorID, message string, errorCount int64) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&entity.SyncLogs{}).Where("id = ? AND status <> ?", taskID, SyncStatusCancel).Updates(map[string]any{
			"status":      SyncStatusFail,
			"error_msg":   message,
			"error_count": errorCount,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if connectorID == "" {
			return nil
		}
		return tx.Model(&entity.Connector{}).Where("id = ?", connectorID).Update("status", SyncStatusFail).Error
	})
}

// HandleTransientFailure retries a running task until maxRetries is reached.
func (d *SyncTaskDAO) HandleTransientFailure(ctx context.Context, taskID, connectorID, message string, maxRetries int64) (int64, bool, error) {
	var attempts int64
	var failed bool
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task entity.SyncLogs
		query := tx.WithContext(ctx)
		if tx.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.Where("id = ? AND status = ?", taskID, SyncStatusRunning).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		attempts = task.ErrorCount + 1
		status := SyncStatusSchedule
		connectorStatus := SyncStatusSchedule
		errorMsg := message
		if attempts >= maxRetries {
			failed = true
			status = SyncStatusFail
			connectorStatus = SyncStatusFail
			errorMsg = fmt.Sprintf("sync task failed after %d transient retries: %s", maxRetries, message)
		}

		if err := tx.Model(&entity.SyncLogs{}).
			Where("id = ? AND status = ?", taskID, SyncStatusRunning).
			Updates(map[string]any{
				"status":      status,
				"error_msg":   errorMsg,
				"error_count": attempts,
			}).Error; err != nil {
			return err
		}
		if connectorID == "" {
			return nil
		}
		return tx.Model(&entity.Connector{}).Where("id = ?", connectorID).Update("status", connectorStatus).Error
	})
	return attempts, failed, err
}

// CompleteSyncTask marks SYNC done and creates the next schedule task.
func (d *SyncTaskDAO) CompleteSyncTask(ctx context.Context, taskContext SyncTaskContext, pollRangeEnd time.Time, newDocs, totalDocs, errorCount int64, errorMsg string) (string, error) {
	var nextTaskID string
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&entity.SyncLogs{}).Where("id = ? AND status = ?", taskContext.Task.ID, SyncStatusRunning).Updates(map[string]any{
			"status":             SyncStatusDone,
			"poll_range_end":     entity.FlexibleTime(pollRangeEnd),
			"new_docs_indexed":   newDocs,
			"total_docs_indexed": totalDocs,
			"error_msg":          errorMsg,
			"error_count":        errorCount,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if err := tx.Model(&entity.Connector{}).Where("id = ?", taskContext.Connector.ID).Update("status", SyncStatusDone).Error; err != nil {
			return err
		}

		var err error
		nextTaskID, err = createScheduledTask(ctx, tx, taskContext.Connector.ID, taskContext.Knowledgebase.ID, TaskTypeSync, false, &pollRangeEnd, taskContext.Task.TotalDocsIndexed+totalDocs)
		return err
	})
	return nextTaskID, err
}

// CompletePruneTask marks PRUNE done and creates the next schedule task.
func (d *SyncTaskDAO) CompletePruneTask(ctx context.Context, taskContext SyncTaskContext, removed int64) (string, error) {
	var nextTaskID string
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&entity.SyncLogs{}).Where("id = ? AND status = ?", taskContext.Task.ID, SyncStatusRunning).Updates(map[string]any{
			"status":                  SyncStatusDone,
			"docs_removed_from_index": gorm.Expr("docs_removed_from_index + ?", removed),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if err := tx.Model(&entity.Connector{}).Where("id = ?", taskContext.Connector.ID).Update("status", SyncStatusDone).Error; err != nil {
			return err
		}
		if !utility.ConfigBool(taskContext.Connector.Config, "sync_deleted_files") {
			return nil
		}
		var err error
		nextTaskID, err = createScheduledTask(ctx, tx, taskContext.Connector.ID, taskContext.Knowledgebase.ID, TaskTypePrune, false, nil, taskContext.Task.TotalDocsIndexed)
		return err
	})
	return nextTaskID, err
}

// RecoverRunning restores running sync tasks during syncer startup.
func (d *SyncTaskDAO) RecoverRunning(ctx context.Context) (int64, error) {
	type runningTaskRow struct {
		ID          string `gorm:"column:id"`
		ConnectorID string `gorm:"column:connector_id"`
	}

	var rows []runningTaskRow
	if err := d.db.WithContext(ctx).
		Model(&entity.SyncLogs{}).
		Select("id, connector_id").
		Where("status = ? AND task_type IN ?", SyncStatusRunning, []string{TaskTypeSync, TaskTypePrune}).
		Scan(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}

	connectorIDs := map[string]struct{}{}
	return int64(len(rows)), d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		dueAt := time.Unix(0, 0).Local()
		for _, row := range rows {
			if err := tx.Model(&entity.SyncLogs{}).
				Where("id = ? AND status = ?", row.ID, SyncStatusRunning).
				Updates(map[string]any{
					"status":      SyncStatusSchedule,
					"update_time": dueAt.UnixMilli(),
					"update_date": dueAt,
				}).Error; err != nil {
				return err
			}
			connectorIDs[row.ConnectorID] = struct{}{}
		}

		ids := make([]string, 0, len(connectorIDs))
		for connectorID := range connectorIDs {
			ids = append(ids, connectorID)
		}
		return tx.Model(&entity.Connector{}).Where("id IN ? AND status = ?", ids, SyncStatusRunning).Update("status", SyncStatusSchedule).Error
	})
}

// createScheduledTask creates the next Python-compatible scheduled task.
func createScheduledTask(ctx context.Context, tx *gorm.DB, connectorID, kbID, taskType string, fromBeginning bool, pollRangeStart *time.Time, totalDocsIndexed int64) (string, error) {
	var lockRow entity.Connector2Kb
	query := tx.WithContext(ctx)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.
		Where("connector_id = ? AND kb_id = ?", connectorID, kbID).
		First(&lockRow).Error; err != nil {
		return "", err
	}

	var existing entity.SyncLogs
	err := tx.WithContext(ctx).
		Where("connector_id = ? AND kb_id = ? AND task_type = ? AND status = ?", connectorID, kbID, taskType, SyncStatusSchedule).
		Order("update_time DESC").
		First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	if err == nil {
		return existing.ID, nil
	}

	reindex := "0"
	if fromBeginning {
		reindex = "1"
	}

	now := time.Now().Local()
	if err := tx.WithContext(ctx).Model(&entity.Connector{}).
		Where("id = ?", connectorID).
		Update("status", SyncStatusSchedule).Error; err != nil {
		return "", err
	}
	taskID := utility.GenerateToken()
	return taskID, tx.WithContext(ctx).Create(&entity.SyncLogs{
		ID:               taskID,
		ConnectorID:      connectorID,
		KbID:             kbID,
		TaskType:         taskType,
		Status:           SyncStatusSchedule,
		FromBeginning:    &reindex,
		PollRangeStart:   entity.NewFlexibleTime(pollRangeStart),
		TimeStarted:      &now,
		ErrorMsg:         "",
		TotalDocsIndexed: totalDocsIndexed,
	}).Error
}
