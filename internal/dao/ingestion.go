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
	"gorm.io/gorm/clause"
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

// UpdatePipelineLogID binds the task to the pipeline_operation_log row its
// current run owns. The terminal writer updates exactly that row, so a
// superseded run whose row was deleted or replaced cannot adopt the
// replacement run's row.
func (dao *IngestionTaskDAO) UpdatePipelineLogID(ctx context.Context, db *gorm.DB, taskID, logID string) error {
	return db.WithContext(ctx).Model(&entity.IngestionTask{}).Where("id = ?", taskID).Update("pipeline_log_id", logID).Error
}

// ClearPipelineLogID detaches a terminal task from its completed run before a
// user-initiated retry receives a new immutable run identity.
func (dao *IngestionTaskDAO) ClearPipelineLogID(ctx context.Context, db *gorm.DB, taskID string) error {
	return db.WithContext(ctx).Model(&entity.IngestionTask{}).Where("id = ?", taskID).Update("pipeline_log_id", nil).Error
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

// GetByIDForUpdate fetches and locks a task for a short ownership-establishment
// transaction. Callers must pass a transaction and keep metadata lookups and
// message publishing outside the lock.
func (dao *IngestionTaskDAO) GetByIDForUpdate(ctx context.Context, db *gorm.DB, id string) (*entity.IngestionTask, error) {
	var task *entity.IngestionTask
	err := db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", id).
		First(&task).Error
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

// GetLatestByDocumentIDs returns a map of documentID -> latest IngestionTask.
func (dao *IngestionTaskDAO) GetLatestByDocumentIDs(ctx context.Context, db *gorm.DB, documentIDs []string) (map[string]*entity.IngestionTask, error) {
	if len(documentIDs) == 0 {
		return map[string]*entity.IngestionTask{}, nil
	}
	var tasks []*entity.IngestionTask
	err := db.WithContext(ctx).
		Where("document_id IN ?", documentIDs).
		Order("COALESCE(create_time, 0) DESC").
		Order("id DESC").
		Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]*entity.IngestionTask, len(documentIDs))
	for _, task := range tasks {
		if _, exists := result[task.DocumentID]; !exists {
			result[task.DocumentID] = task
		}
	}
	return result, nil
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
			)`, datasetID, common.ActiveTaskStatuses).
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

// IngestionEventPage is one keyset-paginated segment of an immutable run's
// event stream. Events are always returned in ascending ID order so callers
// can append or prepend them without re-sorting.
type IngestionEventPage struct {
	Events        []*entity.IngestionTaskLog
	HasMoreBefore bool
	HasMoreAfter  bool
}

// Event types stored in ingestion_task_log.event_type. Only lifecycle events
// participate in component progress aggregation; the remaining kinds are the
// immutable run event stream rendered by the UI.
const (
	EventTypeLifecycle = iota
	EventTypeMessage
	EventTypeTerminal
	EventTypeSystem
	EventTypeLegacy
)

func NewIngestionTaskLogDAO() *IngestionTaskLogDAO {
	return &IngestionTaskLogDAO{}
}

func (dao *IngestionTaskLogDAO) Create(ctx context.Context, db *gorm.DB, ingestionLog *entity.IngestionTaskLog) error {
	return db.WithContext(ctx).Create(ingestionLog).Error
}

func (dao *IngestionTaskLogDAO) Update(ctx context.Context, db *gorm.DB, ingestionLog *entity.IngestionTaskLog) error {
	return db.WithContext(ctx).Save(ingestionLog).Error
}

// ListLogsByPipelineLogID returns one run's events in chronological write
// order. The pipeline log id is the immutable run identity; task ids are
// reusable across retries and must not be used to reconstruct a run.
func (dao *IngestionTaskLogDAO) ListLogsByPipelineLogID(ctx context.Context, db *gorm.DB, pipelineLogID string) ([]*entity.IngestionTaskLog, error) {
	var tasks []*entity.IngestionTaskLog
	err := db.WithContext(ctx).Where("pipeline_log_id = ?", pipelineLogID).Order("id ASC").Find(&tasks).Error
	return tasks, err
}

// LatestEventsByPipelineLogIDs returns each requested run's latest persisted
// event in one query. It never falls back to task_id because a task can be
// reused by a later run.
func (dao *IngestionTaskLogDAO) LatestEventsByPipelineLogIDs(ctx context.Context, db *gorm.DB, pipelineLogIDs []string) (map[string]*entity.IngestionTaskLog, error) {
	if len(pipelineLogIDs) == 0 {
		return map[string]*entity.IngestionTaskLog{}, nil
	}
	latestIDs := db.WithContext(ctx).Model(&entity.IngestionTaskLog{}).
		Select("MAX(id)").
		Where("pipeline_log_id IN ?", pipelineLogIDs).
		Group("pipeline_log_id")
	var events []*entity.IngestionTaskLog
	if err := db.WithContext(ctx).Where("id IN (?)", latestIDs).Find(&events).Error; err != nil {
		return nil, err
	}
	result := make(map[string]*entity.IngestionTaskLog, len(events))
	for _, event := range events {
		if event != nil && event.PipelineLogID != nil && *event.PipelineLogID != "" {
			result[*event.PipelineLogID] = event
		}
	}
	return result, nil
}

// ListEventsPageByPipelineLogID returns one page for a run's immutable event
// stream. afterID and beforeID are mutually exclusive keyset cursors; callers
// validate public request parameters before invoking this DAO method.
func (dao *IngestionTaskLogDAO) ListEventsPageByPipelineLogID(ctx context.Context, db *gorm.DB, pipelineLogID string, limit int, afterID, beforeID *int) (*IngestionEventPage, error) {
	if limit <= 0 {
		return nil, errors.New("ingestion event page limit must be positive")
	}
	if afterID != nil && beforeID != nil {
		return nil, errors.New("ingestion event page cursors are mutually exclusive")
	}

	query := db.WithContext(ctx).Where("pipeline_log_id = ?", pipelineLogID)
	descending := false
	switch {
	case afterID != nil:
		query = query.Where("id > ?", *afterID).Order("id ASC")
	case beforeID != nil:
		query = query.Where("id < ?", *beforeID).Order("id DESC")
		descending = true
	default:
		query = query.Order("id DESC")
		descending = true
	}

	var events []*entity.IngestionTaskLog
	if err := query.Limit(limit + 1).Find(&events).Error; err != nil {
		return nil, err
	}
	page := &IngestionEventPage{}
	if len(events) > limit {
		if descending {
			page.HasMoreBefore = true
		} else {
			page.HasMoreAfter = true
		}
		events = events[:limit]
	}
	if descending {
		for left, right := 0, len(events)-1; left < right; left, right = left+1, right-1 {
			events[left], events[right] = events[right], events[left]
		}
	}
	page.Events = events

	if len(events) == 0 {
		switch {
		case afterID != nil:
			page.HasMoreBefore, _ = dao.hasEventBeforeOrEqual(ctx, db, pipelineLogID, *afterID)
		case beforeID != nil:
			page.HasMoreAfter, _ = dao.hasEventAfterOrEqual(ctx, db, pipelineLogID, *beforeID)
		}
		return page, nil
	}

	oldestID := events[0].ID
	newestID := events[len(events)-1].ID
	if !page.HasMoreBefore {
		var err error
		page.HasMoreBefore, err = dao.hasEventBefore(ctx, db, pipelineLogID, oldestID)
		if err != nil {
			return nil, err
		}
	}
	if !page.HasMoreAfter {
		var err error
		page.HasMoreAfter, err = dao.hasEventAfter(ctx, db, pipelineLogID, newestID)
		if err != nil {
			return nil, err
		}
	}
	return page, nil
}

func (dao *IngestionTaskLogDAO) hasEventBefore(ctx context.Context, db *gorm.DB, pipelineLogID string, id int) (bool, error) {
	return dao.hasEvent(ctx, db, pipelineLogID, "id < ?", id)
}

func (dao *IngestionTaskLogDAO) hasEventAfter(ctx context.Context, db *gorm.DB, pipelineLogID string, id int) (bool, error) {
	return dao.hasEvent(ctx, db, pipelineLogID, "id > ?", id)
}

func (dao *IngestionTaskLogDAO) hasEventBeforeOrEqual(ctx context.Context, db *gorm.DB, pipelineLogID string, id int) (bool, error) {
	return dao.hasEvent(ctx, db, pipelineLogID, "id <= ?", id)
}

func (dao *IngestionTaskLogDAO) hasEventAfterOrEqual(ctx context.Context, db *gorm.DB, pipelineLogID string, id int) (bool, error) {
	return dao.hasEvent(ctx, db, pipelineLogID, "id >= ?", id)
}

func (dao *IngestionTaskLogDAO) hasEvent(ctx context.Context, db *gorm.DB, pipelineLogID, condition string, id int) (bool, error) {
	var count int64
	err := db.WithContext(ctx).Model(&entity.IngestionTaskLog{}).
		Where("pipeline_log_id = ? AND "+condition, pipelineLogID, id).
		Limit(1).Count(&count).Error
	return count > 0, err
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

// AggregateProgressByPipelineLogID computes component progress for one run.
// Detailed messages and terminal/system events deliberately do not affect a
// component's latest lifecycle state.
func (dao *IngestionTaskLogDAO) AggregateProgressByPipelineLogID(ctx context.Context, db *gorm.DB, pipelineLogID string, total int) (*TaskProgress, error) {
	latestIDs := db.WithContext(ctx).Model(&entity.IngestionTaskLog{}).
		Select("MAX(id)").
		Where("pipeline_log_id = ? AND event_type = ? AND component <> ?", pipelineLogID, EventTypeLifecycle, "").
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
	for _, row := range rows {
		switch {
		case row.Phase == 1:
			progress.Done++
		case row.Phase < 0 || row.Phase == 2:
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

func (dao *IngestionTaskLogDAO) GetLogByLogID(ctx context.Context, db *gorm.DB, logID string) (*entity.IngestionTaskLog, error) {
	var task *entity.IngestionTaskLog
	err := db.WithContext(ctx).Where("id = ?", logID).First(&task).Error
	return task, err
}
