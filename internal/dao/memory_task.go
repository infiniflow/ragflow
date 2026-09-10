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
	"time"

	"ragflow/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var activeMemoryTaskStates = []entity.MemoryTaskState{
	entity.MemoryTaskStatePending,
	entity.MemoryTaskStateExtracted,
	entity.MemoryTaskStateStored,
}

// MemoryTaskDAO persists durable memory extraction checkpoints and leases.
type MemoryTaskDAO struct{}

// NewMemoryTaskDAO creates a memory task DAO.
func NewMemoryTaskDAO() *MemoryTaskDAO { return &MemoryTaskDAO{} }

// CreateWithTask creates the UI task and its durable execution record in one
// transaction so workers never observe only one side of the pair.
func (d *MemoryTaskDAO) CreateWithTask(ctx context.Context, db *gorm.DB, task *entity.Task, memoryTask *entity.MemoryTask) error {
	if db == nil {
		return errors.New("memory task: nil database")
	}
	if task == nil || memoryTask == nil {
		return errors.New("memory task: task and execution record are required")
	}
	if task.ID == "" || memoryTask.TaskID == "" || task.ID != memoryTask.TaskID {
		return errors.New("memory task: task ids must be non-empty and equal")
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		return tx.Create(memoryTask).Error
	})
}

// GetByID returns one durable memory task.
func (d *MemoryTaskDAO) GetByID(ctx context.Context, db *gorm.DB, taskID string) (*entity.MemoryTask, error) {
	if db == nil {
		return nil, errors.New("memory task: nil database")
	}
	var task entity.MemoryTask
	if err := db.WithContext(ctx).Where("task_id = ?", taskID).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

// Claim acquires an expired or unowned due task and increments its attempt
// count. It returns the current row with acquired=false when the task is not
// due, is terminal, or already has a live lease.
func (d *MemoryTaskDAO) Claim(ctx context.Context, db *gorm.DB, taskID, owner string, now time.Time, leaseTTL time.Duration) (*entity.MemoryTask, bool, error) {
	if db == nil {
		return nil, false, errors.New("memory task: nil database")
	}
	if taskID == "" || owner == "" {
		return nil, false, errors.New("memory task: task id and lease owner are required")
	}
	if leaseTTL <= 0 {
		return nil, false, errors.New("memory task: lease TTL must be positive")
	}

	var (
		task     entity.MemoryTask
		acquired bool
	)
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.WithContext(ctx)
		if tx.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.Where("task_id = ?", taskID).First(&task).Error; err != nil {
			return err
		}
		if !isActiveMemoryTaskState(task.State) ||
			(task.NextRetryAt != nil && task.NextRetryAt.After(now)) ||
			(task.LeaseExpiresAt != nil && task.LeaseExpiresAt.After(now)) {
			return nil
		}

		expiresAt := now.Add(leaseTTL)
		result := tx.WithContext(ctx).Model(&entity.MemoryTask{}).
			Where("task_id = ?", taskID).
			Updates(map[string]any{
				"attempt_count":    gorm.Expr("attempt_count + 1"),
				"lease_owner":      owner,
				"lease_expires_at": expiresAt,
				"next_retry_at":    nil,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		acquired = true
		return tx.WithContext(ctx).Where("task_id = ?", taskID).First(&task).Error
	})
	if err != nil {
		return nil, false, err
	}
	return &task, acquired, err
}

// RenewLease extends a live lease held by owner.
func (d *MemoryTaskDAO) RenewLease(ctx context.Context, db *gorm.DB, taskID, owner string, now time.Time, leaseTTL time.Duration) (bool, error) {
	if db == nil {
		return false, errors.New("memory task: nil database")
	}
	if taskID == "" || owner == "" || leaseTTL <= 0 {
		return false, errors.New("memory task: task id, lease owner, and positive TTL are required")
	}

	result := db.WithContext(ctx).Model(&entity.MemoryTask{}).
		Where("task_id = ? AND state IN ? AND lease_owner = ? AND lease_expires_at > ?", taskID, activeMemoryTaskStates, owner, now).
		Update("lease_expires_at", now.Add(leaseTTL))
	return result.RowsAffected == 1, result.Error
}

// PersistExtraction advances a leased task from pending to extracted and
// stores the materialized LLM result used by later retries.
func (d *MemoryTaskDAO) PersistExtraction(ctx context.Context, db *gorm.DB, taskID, owner string, now time.Time, extraction entity.JSONSlice) (bool, error) {
	return d.transition(ctx, db, taskID, owner, now, entity.MemoryTaskStatePending, entity.MemoryTaskStateExtracted, map[string]any{
		"extraction": extraction,
		"last_error": "",
	})
}

// MarkStored advances a leased task after its extracted chunks are stored.
func (d *MemoryTaskDAO) MarkStored(ctx context.Context, db *gorm.DB, taskID, owner string, now time.Time) (bool, error) {
	return d.transition(ctx, db, taskID, owner, now, entity.MemoryTaskStateExtracted, entity.MemoryTaskStateStored, map[string]any{
		"last_error": "",
	})
}

// ScheduleRetry records the next execution time and releases the current
// lease. The task remains at its last durable checkpoint.
func (d *MemoryTaskDAO) ScheduleRetry(ctx context.Context, db *gorm.DB, taskID, owner string, nextRetryAt time.Time, lastError string) (bool, error) {
	if db == nil {
		return false, errors.New("memory task: nil database")
	}
	if taskID == "" || owner == "" {
		return false, errors.New("memory task: task id and lease owner are required")
	}
	result := db.WithContext(ctx).Model(&entity.MemoryTask{}).
		Where("task_id = ? AND state IN ? AND lease_owner = ?", taskID, activeMemoryTaskStates, owner).
		Updates(map[string]any{
			"next_retry_at":    nextRetryAt,
			"lease_owner":      "",
			"lease_expires_at": nil,
			"last_error":       lastError,
		})
	return result.RowsAffected == 1, result.Error
}

// MarkFailed records a terminal failure and projects it onto the generic task
// row when that row still exists.
func (d *MemoryTaskDAO) MarkFailed(ctx context.Context, db *gorm.DB, taskID, owner, progressMsg string) (bool, error) {
	if db == nil {
		return false, errors.New("memory task: nil database")
	}
	if taskID == "" || owner == "" {
		return false, errors.New("memory task: task id and lease owner are required")
	}
	var updated bool
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.WithContext(ctx).Model(&entity.MemoryTask{}).
			Where("task_id = ? AND state IN ? AND lease_owner = ?", taskID, activeMemoryTaskStates, owner).
			Updates(map[string]any{
				"state":            entity.MemoryTaskStateFailed,
				"next_retry_at":    nil,
				"lease_owner":      "",
				"lease_expires_at": nil,
				"last_error":       progressMsg,
			})
		if result.Error != nil || result.RowsAffected != 1 {
			return result.Error
		}
		updated = true
		return tx.WithContext(ctx).Model(&entity.Task{}).Where("id = ?", taskID).Updates(map[string]any{
			"progress":     -1,
			"progress_msg": progressMsg,
		}).Error
	})
	if err != nil {
		return false, err
	}
	return updated, nil
}

// Complete atomically advances a leased stored task to completed and updates
// the generic task progress projection when it still exists.
func (d *MemoryTaskDAO) Complete(ctx context.Context, db *gorm.DB, taskID, owner, progressMsg string, now time.Time) (bool, error) {
	if db == nil {
		return false, errors.New("memory task: nil database")
	}
	if taskID == "" || owner == "" {
		return false, errors.New("memory task: task id and lease owner are required")
	}
	var completed bool
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.WithContext(ctx).Model(&entity.MemoryTask{}).
			Where("task_id = ? AND state = ? AND lease_owner = ? AND lease_expires_at > ?", taskID, entity.MemoryTaskStateStored, owner, now).
			Updates(map[string]any{
				"state":            entity.MemoryTaskStateCompleted,
				"next_retry_at":    nil,
				"lease_owner":      "",
				"lease_expires_at": nil,
				"last_error":       "",
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}

		progress := tx.WithContext(ctx).Model(&entity.Task{}).Where("id = ?", taskID).Updates(map[string]any{
			"progress":     1.0,
			"progress_msg": progressMsg,
		})
		if progress.Error != nil {
			return progress.Error
		}
		completed = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return completed, nil
}

// ListDue returns non-terminal tasks whose retry time and lease allow another
// wake-up to be published.
func (d *MemoryTaskDAO) ListDue(ctx context.Context, db *gorm.DB, now time.Time, limit int) ([]*entity.MemoryTask, error) {
	if db == nil {
		return nil, errors.New("memory task: nil database")
	}
	if limit <= 0 {
		return nil, errors.New("memory task: positive list limit is required")
	}

	var tasks []*entity.MemoryTask
	err := db.WithContext(ctx).
		Where("state IN ?", activeMemoryTaskStates).
		Where("next_retry_at IS NULL OR next_retry_at <= ?", now).
		Where("lease_expires_at IS NULL OR lease_expires_at <= ?", now).
		Order("next_retry_at ASC, create_time ASC, task_id ASC").
		Limit(limit).
		Find(&tasks).Error
	return tasks, err
}

// transition applies a lease-guarded checkpoint change.
func (d *MemoryTaskDAO) transition(ctx context.Context, db *gorm.DB, taskID, owner string, now time.Time, from, to entity.MemoryTaskState, updates map[string]any) (bool, error) {
	if db == nil {
		return false, errors.New("memory task: nil database")
	}
	if taskID == "" || owner == "" {
		return false, errors.New("memory task: task id and lease owner are required")
	}
	updates["state"] = to
	result := db.WithContext(ctx).Model(&entity.MemoryTask{}).
		Where("task_id = ? AND state = ? AND lease_owner = ? AND lease_expires_at > ?", taskID, from, owner, now).
		Updates(updates)
	return result.RowsAffected == 1, result.Error
}

// isActiveMemoryTaskState reports whether a task can still be executed.
func isActiveMemoryTaskState(state entity.MemoryTaskState) bool {
	return state == entity.MemoryTaskStatePending ||
		state == entity.MemoryTaskStateExtracted ||
		state == entity.MemoryTaskStateStored
}
