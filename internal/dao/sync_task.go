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

// ConnectorSource returns the connector source name.
func (c SyncTaskContext) ConnectorSource() string {
	return c.Connector.Source
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

// ListDueTasks returns due schedule tasks across SYNC and PRUNE.
func (d *SyncTaskDAO) ListDueTasks(ctx context.Context, now time.Time, limit int) ([]entity.SyncLogs, error) {
	var rows []dueSyncTaskRow
	if err := d.db.WithContext(ctx).
		Model(&entity.SyncLogs{}).
		Select("sync_logs.*, connector.refresh_freq AS connector_refresh_freq, connector.prune_freq AS connector_prune_freq, connector.config AS connector_config").
		Joins("JOIN connector ON sync_logs.connector_id = connector.id").
		Joins("JOIN connector2kb ON sync_logs.connector_id = connector2kb.connector_id AND sync_logs.kb_id = connector2kb.kb_id").
		Joins("JOIN knowledgebase ON sync_logs.kb_id = knowledgebase.id").
		Where("sync_logs.status = ? AND connector.status = ? AND sync_logs.task_type IN ?", SyncStatusSchedule, SyncStatusSchedule, []string{TaskTypeSync, TaskTypePrune}).
		Order("sync_logs.update_time DESC").
		Limit(limit * 4).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	tasks := make([]entity.SyncLogs, 0, limit)
	for _, row := range rows {
		if !isDue(row.SyncLogs, row.ConnectorRefreshFreq, row.ConnectorPruneFreq, row.ConnectorConfig, now) {
			continue
		}
		tasks = append(tasks, row.SyncLogs)
		if len(tasks) >= limit {
			break
		}
	}
	return tasks, nil
}

// ClaimTask conditionally marks a scheduled task as running.
func (d *SyncTaskDAO) ClaimTask(ctx context.Context, taskID string, now time.Time) (bool, error) {
	result := d.db.WithContext(ctx).Model(&entity.SyncLogs{}).
		Where("id = ? AND status = ?", taskID, SyncStatusSchedule).
		Updates(map[string]any{"status": SyncStatusRunning, "time_started": now})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
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
		if err := tx.Model(&entity.SyncLogs{}).Where("id = ?", taskID).Updates(map[string]any{
			"status":      SyncStatusFail,
			"error_msg":   message,
			"error_count": errorCount,
		}).Error; err != nil {
			return err
		}
		if connectorID == "" {
			return nil
		}
		return tx.Model(&entity.Connector{}).Where("id = ?", connectorID).Update("status", SyncStatusFail).Error
	})
}

// CompleteSyncTask marks SYNC done and creates the next schedule task.
func (d *SyncTaskDAO) CompleteSyncTask(ctx context.Context, taskContext SyncTaskContext, pollRangeEnd time.Time, newDocs, totalDocs, errorCount int64, errorMsg string) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&entity.SyncLogs{}).Where("id = ?", taskContext.Task.ID).Updates(map[string]any{
			"status":             SyncStatusDone,
			"poll_range_end":     pollRangeEnd,
			"new_docs_indexed":   newDocs,
			"total_docs_indexed": gorm.Expr("total_docs_indexed + ?", totalDocs),
			"error_msg":          errorMsg,
			"error_count":        errorCount,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&entity.Connector{}).Where("id = ?", taskContext.Connector.ID).Update("status", SyncStatusDone).Error; err != nil {
			return err
		}

		return createScheduledTask(ctx, tx, taskContext.Connector.ID, taskContext.Knowledgebase.ID, TaskTypeSync, false, &pollRangeEnd, taskContext.Task.TotalDocsIndexed+totalDocs)
	})
}

// CompletePruneTask marks PRUNE done and creates the next schedule task.
func (d *SyncTaskDAO) CompletePruneTask(ctx context.Context, taskContext SyncTaskContext, removed int64) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&entity.SyncLogs{}).Where("id = ?", taskContext.Task.ID).Updates(map[string]any{
			"status":                  SyncStatusDone,
			"docs_removed_from_index": gorm.Expr("docs_removed_from_index + ?", removed),
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&entity.Connector{}).Where("id = ?", taskContext.Connector.ID).Update("status", SyncStatusDone).Error; err != nil {
			return err
		}
		if !syncConnectorConfigBool(taskContext.Connector.Config, "sync_deleted_files") {
			return nil
		}
		return createScheduledTask(ctx, tx, taskContext.Connector.ID, taskContext.Knowledgebase.ID, TaskTypePrune, false, nil, taskContext.Task.TotalDocsIndexed)
	})
}

// RecoverStaleRunning restores timed-out running tasks to schedule.
func (d *SyncTaskDAO) RecoverStaleRunning(ctx context.Context, now time.Time) (int64, error) {
	type staleRunningTaskRow struct {
		ID                   string     `gorm:"column:id"`
		ConnectorID          string     `gorm:"column:connector_id"`
		TimeStarted          *time.Time `gorm:"column:time_started"`
		ConnectorTimeoutSecs int64      `gorm:"column:connector_timeout_secs"`
	}
	var rows []staleRunningTaskRow
	if err := d.db.WithContext(ctx).
		Model(&entity.SyncLogs{}).
		Select("sync_logs.id, sync_logs.connector_id, sync_logs.time_started, connector.timeout_secs AS connector_timeout_secs").
		Joins("JOIN connector ON sync_logs.connector_id = connector.id").
		Where("sync_logs.status = ?", SyncStatusRunning).
		Scan(&rows).Error; err != nil {
		return 0, err
	}
	var recovered int64
	connectorIDs := map[string]struct{}{}

	for _, row := range rows {
		if row.TimeStarted == nil {
			continue
		}
		timeout := time.Duration(row.ConnectorTimeoutSecs) * time.Second
		if timeout <= 0 {
			timeout = time.Hour
		}
		if row.TimeStarted.Add(timeout).After(now) {
			continue
		}
		if err := d.RescheduleClaimed(ctx, row.ID); err != nil {
			return recovered, err
		}
		recovered++
		connectorIDs[row.ConnectorID] = struct{}{}
	}

	if recovered == 0 {
		return 0, nil
	}
	ids := make([]string, 0, len(connectorIDs))

	for connectorID := range connectorIDs {
		ids = append(ids, connectorID)
	}

	return recovered, d.db.WithContext(ctx).Model(&entity.Connector{}).Where("id IN ? AND status = ?", ids, SyncStatusRunning).Update("status", SyncStatusSchedule).Error
}

// createScheduledTask creates the next Python-compatible scheduled task.
func createScheduledTask(ctx context.Context, tx *gorm.DB, connectorID, kbID, taskType string, fromBeginning bool, pollRangeStart *time.Time, totalDocsIndexed int64) error {
	var lockRow entity.Connector2Kb
	query := tx.WithContext(ctx)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.
		Where("connector_id = ? AND kb_id = ?", connectorID, kbID).
		First(&lockRow).Error; err != nil {
		return err
	}

	var existing int64
	if err := tx.WithContext(ctx).Model(&entity.SyncLogs{}).
		Where("connector_id = ? AND kb_id = ? AND task_type = ? AND status = ?", connectorID, kbID, taskType, SyncStatusSchedule).
		Count(&existing).Error; err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}

	reindex := "0"
	if fromBeginning {
		reindex = "1"
	}

	now := time.Now().Local()
	return tx.WithContext(ctx).Create(&entity.SyncLogs{
		ID:               utility.GenerateToken(),
		ConnectorID:      connectorID,
		KbID:             kbID,
		TaskType:         taskType,
		Status:           SyncStatusSchedule,
		FromBeginning:    &reindex,
		PollRangeStart:   pollRangeStart,
		TimeStarted:      &now,
		ErrorMsg:         "",
		TotalDocsIndexed: totalDocsIndexed,
	}).Error
}

// isDue applies refresh_freq and prune_freq scheduling semantics.
func isDue(task entity.SyncLogs, refreshFreq, pruneFreq int64, config map[string]any, now time.Time) bool {
	if task.UpdateDate == nil {
		return true
	}
	var freqMinutes int64
	switch task.TaskType {
	case TaskTypeSync:
		freqMinutes = refreshFreq
	case TaskTypePrune:
		if !syncConnectorConfigBool(config, "sync_deleted_files") {
			return false
		}
		freqMinutes = pruneFreq
	default:
		return false
	}
	if freqMinutes <= 0 {
		return true
	}
	return task.UpdateDate.Before(now.Add(-time.Duration(freqMinutes) * time.Minute))
}

// syncConnectorConfigBool reads a Python JSON bool/string flag.
func syncConnectorConfigBool(config map[string]any, key string) bool {
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

// IsNotFound reports whether an error is a gorm not found error.
func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
