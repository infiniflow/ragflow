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
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/utility"

	"gorm.io/gorm"
)

type IngestionTaskDAO struct{}

func NewIngestionTaskDAO() *IngestionTaskDAO {
	return &IngestionTaskDAO{}
}

func (dao *IngestionTaskDAO) Create(ctx context.Context, db *gorm.DB, ingestionTask *entity.IngestionTask) (*entity.IngestionTask, error) {
	existing, err := dao.GetByDocumentID(ctx, db, ingestionTask.DocumentID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("document id %s already exists, status: %s, task id: %s", ingestionTask.DocumentID, existing.Status, existing.ID)
	}
	if ingestionTask.ID == "" {
		ingestionTask.ID = utility.GenerateUUID()
	}
	if err = db.WithContext(ctx).Create(ingestionTask).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			existing, getErr := dao.GetByDocumentID(ctx, db, ingestionTask.DocumentID)
			if getErr != nil {
				return nil, getErr
			}
			if existing != nil {
				return nil, fmt.Errorf("document id %s already exists, status: %s, task id: %s", ingestionTask.DocumentID, existing.Status, existing.ID)
			}
		}
		return nil, err
	}
	return ingestionTask, nil
}

func (dao *IngestionTaskDAO) UpdateStatusIfCurrent(ctx context.Context, db *gorm.DB, taskID, fromStatus, toStatus string) (bool, error) {
	result := db.WithContext(ctx).Model(&entity.IngestionTask{}).
		Where("id = ? AND status = ?", taskID, fromStatus).
		Update("status", toStatus)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// UpdateComponentTotal records the number of components in the task's DSL
// graph. It is the authoritative denominator for progress percentage.
func (dao *IngestionTaskDAO) UpdateComponentTotal(ctx context.Context, db *gorm.DB, taskID string, total int) error {
	return db.WithContext(ctx).Model(&entity.IngestionTask{}).Where("id = ?", taskID).Update("component_total", total).Error
}

type TaskInfo struct {
	TaskID        string   `json:"task_id"`
	FilesToDelete []string `json:"files_to_delete"`
}

func (dao *IngestionTaskDAO) Delete(ctx context.Context, db *gorm.DB, taskID string, userID *string) (*TaskInfo, error) {
	tx := db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	var committed bool

	defer func() {
		if committed {
			tx.Commit()
		} else {
			tx.Rollback()
			if r := recover(); r != nil {
				panic(r)
			}
		}
	}()

	var tasks []*entity.IngestionTask
	err := tx.Where("id = ?", taskID).Find(&tasks).Error
	if err != nil {
		return nil, err
	}

	if len(tasks) == 0 {
		return nil, fmt.Errorf("task %s not found", taskID)
	}

	if len(tasks) != 1 {
		return nil, fmt.Errorf("task %s has multiple records", taskID)
	}

	if userID != nil {
		if tasks[0].UserID != *userID {
			return nil, errors.New("task does not belong to the user")
		}
	}

	taskStatus := tasks[0].Status
	switch taskStatus {
	case common.CREATED, common.SCHEDULED, common.STOPPED, common.COMPLETED, common.FAILED:
		// ingestion_task_log no longer carries file references (the old
		// checkpoint JSON column was dropped in favor of typed columns), so
		// there are no task-level files to delete here.
		var filesToDelete []string

		result := tx.Model(&entity.IngestionTask{}).
			Where("id = ? AND status IN ?", taskID, []string{common.CREATED, common.SCHEDULED, common.STOPPED, common.COMPLETED, common.FAILED}).
			Delete(&entity.IngestionTask{})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, fmt.Errorf("task %s status changed, cannot be removed", taskID)
		}

		taskInfo := &TaskInfo{
			TaskID:        taskID,
			FilesToDelete: filesToDelete,
		}
		committed = true
		return taskInfo, nil
	default:
		return nil, fmt.Errorf("task %s is executing, cannot be removed", taskID)
	}
}

func (dao *IngestionTaskDAO) GetAllTasks(ctx context.Context, db *gorm.DB, page, pageSize int) ([]*entity.IngestionTask, error) {
	var tasks []*entity.IngestionTask
	query := db.WithContext(ctx)
	var err error
	if pageSize == 0 {
		err = query.Find(&tasks).Error
	} else {
		err = query.Order("create_time DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&tasks).Error
	}
	return tasks, err
}

func (dao *IngestionTaskDAO) ListByUserID(ctx context.Context, db *gorm.DB, userID string, page, pageSize int) ([]*entity.IngestionTask, error) {
	var tasks []*entity.IngestionTask
	query := db.WithContext(ctx).Where("user_id = ?", userID)
	var err error
	if pageSize == 0 {
		err = query.Order("create_time DESC").Find(&tasks).Error
	} else {
		err = query.Order("create_time DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&tasks).Error
	}

	return tasks, err
}

func (dao *IngestionTaskDAO) ListByUserIDAndDatasetID(ctx context.Context, db *gorm.DB, userID, datasetID string, page, pageSize int) ([]*entity.IngestionTask, error) {
	var tasks []*entity.IngestionTask
	query := db.WithContext(ctx).Where("user_id = ? AND dataset_id = ?", userID, datasetID)
	var err error
	if pageSize == 0 {
		err = query.Order("create_time DESC").Find(&tasks).Error
	} else {
		err = query.Order("create_time DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&tasks).Error
	}

	return tasks, err
}

func (dao *IngestionTaskDAO) ListByStatus(ctx context.Context, db *gorm.DB, status string) ([]*entity.IngestionTask, error) {
	var tasks []*entity.IngestionTask
	err := db.WithContext(ctx).Where("status = ?", status).Order("create_time ASC").Find(&tasks).Error
	return tasks, err
}

func (dao *IngestionTaskDAO) GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.IngestionTask, error) {
	var task *entity.IngestionTask
	err := db.WithContext(ctx).Where("id = ?", id).First(&task).Error
	return task, err
}

// GetByDocumentID returns the latest ingestion task for a document. Historical
// retries are ordered by create_time and then ID to match document-list state.
func (dao *IngestionTaskDAO) GetByDocumentID(ctx context.Context, db *gorm.DB, documentId string) (*entity.IngestionTask, error) {
	var tasks []*entity.IngestionTask
	err := db.WithContext(ctx).
		Where("document_id = ?", documentId).
		Order("COALESCE(create_time, 0) DESC").
		Order("id DESC").
		Limit(1).
		Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, nil
	}
	return tasks[0], nil
}

// CountActiveByDatasetID returns the number of ingestion tasks for the
// dataset whose latest task is non-terminal (CREATED/SCHEDULED/RUNNING/STOPPING).
// It uses the same create_time/ID ordering as document-list state so historical
// retries cannot keep polling alive after a newer task becomes terminal.
func (dao *IngestionTaskDAO) CountActiveByDatasetID(ctx context.Context, db *gorm.DB, datasetID string) (int64, error) {
	var count int64
	err := db.WithContext(ctx).Model(&entity.IngestionTask{}).
		Where(`ingestion_task.dataset_id = ?
			AND ingestion_task.status IN ?
			AND NOT EXISTS (
				SELECT 1
				FROM ingestion_task AS newer_ingestion_task
				WHERE newer_ingestion_task.document_id = ingestion_task.document_id
				  AND (
					COALESCE(newer_ingestion_task.create_time, 0) > COALESCE(ingestion_task.create_time, 0)
					OR (
						COALESCE(newer_ingestion_task.create_time, 0) = COALESCE(ingestion_task.create_time, 0)
						AND newer_ingestion_task.id > ingestion_task.id
					)
				  )
			)`, datasetID, []string{common.CREATED, common.SCHEDULED, common.RUNNING, common.STOPPING}).
		Count(&count).Error
	return count, err
}

// DeleteIfTerminal deletes ingestion tasks for a document that are in a
// terminal state (COMPLETED, STOPPED, FAILED), or not yet running
// (CREATED, SCHEDULED).
// RUNNING and STOPPING tasks are NOT deleted because an in-flight worker
// would keep writing chunks and corrupt a new run's results.
// Returns the number of rows deleted.
func (dao *IngestionTaskDAO) DeleteIfTerminal(ctx context.Context, db *gorm.DB, documentID string) (int64, error) {
	result := db.WithContext(ctx).Where("document_id = ? AND status NOT IN (?, ?)",
		documentID, common.RUNNING, common.STOPPING).
		Delete(&entity.IngestionTask{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

type IngestionTaskLogDAO struct{}

func NewIngestionTaskLogDAO() *IngestionTaskLogDAO {
	return &IngestionTaskLogDAO{}
}

func (dao *IngestionTaskLogDAO) Create(ctx context.Context, db *gorm.DB, ingestionLog *entity.IngestionTaskLog) error {
	return db.WithContext(ctx).Create(ingestionLog).Error
}

func (dao *IngestionTaskLogDAO) Update(ctx context.Context, db *gorm.DB, ingestionLog *entity.IngestionTaskLog) error {
	return db.WithContext(ctx).Save(ingestionLog).Error
}

// ListLogsByTaskID returns the task's logs in chronological (write) order.
// Ordering is by auto-increment `id ASC` (NOT `create_time`) because
// create_time has only second-level resolution and would tie-break
// arbitrarily; `id` is monotonic and always reflects write order. This
// feeds the frontend log stream (GET .../logs), which renders each row by
// phase (0 started / 1 done / -1 failed).
func (dao *IngestionTaskLogDAO) ListLogsByTaskID(ctx context.Context, db *gorm.DB, taskID string) ([]*entity.IngestionTaskLog, error) {
	var tasks []*entity.IngestionTaskLog
	err := db.WithContext(ctx).Where("task_id = ?", taskID).Order("id ASC").Find(&tasks).Error
	return tasks, err
}

// TaskProgress is the server-side aggregate of a task's component progress,
// served by GET /api/v1/ingestion_task/{task_id}/progress so the frontend
// can render a progress bar without pulling the full log stream.
type TaskProgress struct {
	Total   int     `json:"total"`
	Done    int     `json:"done"`
	Failed  int     `json:"failed"`
	Running int     `json:"running"`
	Percent float64 `json:"percent"`
}

// AggregateProgress computes {total, done, failed, running, percent} for a
// task purely in SQL. It takes each component's latest row (max id per
// component) and classifies by its phase:
//
//	done    = latest phase is exit/success   (1)
//	failed  = latest phase is error/failure  (-1 legacy, or 2 after 1c)
//	running = anything else (started, 0)
//
// `total` is the authoritative denominator from ingestion_task.component_total.
// The classification is forward-compatible with the §5.1 ProgressPhase
// renumbering (exit=1 stays; error moves -1 -> 2).
func (dao *IngestionTaskLogDAO) AggregateProgress(ctx context.Context, db *gorm.DB, taskID string, total int) (*TaskProgress, error) {
	// Latest row id per component for this task.
	latestIDs := db.WithContext(ctx).Model(&entity.IngestionTaskLog{}).
		Select("MAX(id)").
		Where("task_id = ?", taskID).
		Group("component")

	type phaseRow struct {
		Phase int
	}
	var rows []phaseRow
	err := db.WithContext(ctx).Model(&entity.IngestionTaskLog{}).
		Select("phase").
		Where("id IN (?)", latestIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	progress := &TaskProgress{Total: total}
	for _, r := range rows {
		switch {
		case r.Phase == 1:
			progress.Done++
		case r.Phase < 0 || r.Phase == 2:
			progress.Failed++
		default:
			progress.Running++
		}
	}
	if total > 0 {
		progress.Percent = float64(progress.Done) / float64(total) * 100
	}
	return progress, nil
}

func (dao *IngestionTaskLogDAO) LatestLogByTaskID(ctx context.Context, db *gorm.DB, taskID string) (*entity.IngestionTaskLog, error) {
	var task *entity.IngestionTaskLog
	err := db.WithContext(ctx).Where("task_id = ?", taskID).Order("create_time DESC").First(&task).Error
	return task, err
}

func (dao *IngestionTaskLogDAO) GetLogByLogID(ctx context.Context, db *gorm.DB, logID string) (*entity.IngestionTaskLog, error) {
	var task *entity.IngestionTaskLog
	err := db.WithContext(ctx).Where("id = ?", logID).First(&task).Error
	return task, err
}

func (dao *IngestionTaskLogDAO) DeleteByTaskID(ctx context.Context, db *gorm.DB, taskID string) (int64, error) {
	result := db.WithContext(ctx).Unscoped().Where("task_id = ?", taskID).Delete(&entity.IngestionTaskLog{})
	return result.RowsAffected, result.Error
}

// DeleteComponentLogsByTaskID removes component lifecycle rows from a new run
// while preserving checkpoint rows such as run_count for task history.
func (dao *IngestionTaskLogDAO) DeleteComponentLogsByTaskID(ctx context.Context, db *gorm.DB, taskID string) (int64, error) {
	var logs []*entity.IngestionTaskLog
	if err := db.WithContext(ctx).Where("task_id = ?", taskID).Find(&logs).Error; err != nil {
		return 0, err
	}
	ids := make([]int, 0, len(logs))
	for _, log := range logs {
		if log != nil && len(log.Checkpoint) == 0 {
			ids = append(ids, log.ID)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := db.WithContext(ctx).Unscoped().Where("id IN ?", ids).Delete(&entity.IngestionTaskLog{})
	return result.RowsAffected, result.Error
}
