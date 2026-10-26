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
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

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
			var err error
			page.HasMoreBefore, err = dao.hasEventBeforeOrEqual(ctx, db, pipelineLogID, *afterID)
			if err != nil {
				return nil, err
			}
		case beforeID != nil:
			var err error
			page.HasMoreAfter, err = dao.hasEventAfterOrEqual(ctx, db, pipelineLogID, *beforeID)
			if err != nil {
				return nil, err
			}
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
