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
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// TaskDAO task data access object
type TaskDAO struct{}

// NewTaskDAO create task DAO
func NewTaskDAO() *TaskDAO {
	return &TaskDAO{}
}

// Create creates a new task
func (dao *TaskDAO) Create(ctx context.Context, db *gorm.DB, task *entity.Task) error {
	return db.WithContext(ctx).Create(task).Error
}

// CreateMany creates multiple tasks in one batch.
func (dao *TaskDAO) CreateMany(ctx context.Context, db *gorm.DB, tasks []*entity.Task) error {
	if len(tasks) == 0 {
		return nil
	}
	return db.WithContext(ctx).Create(&tasks).Error
}

// GetByID gets task by ID
func (dao *TaskDAO) GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.Task, error) {
	var task entity.Task
	err := db.WithContext(ctx).Where("id = ?", id).First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// DeleteIngestionTasksByDocIDs deletes ingestion tasks by document IDs (hard delete)
func (dao *TaskDAO) DeleteIngestionTasksByDocIDs(ctx context.Context, db *gorm.DB, docIDs []string) (int64, error) {
	if len(docIDs) == 0 {
		return 0, nil
	}
	result := db.WithContext(ctx).Unscoped().Where("document_id IN ?", docIDs).Delete(&entity.IngestionTask{})
	return result.RowsAffected, result.Error
}

// DeleteByDocIDs deletes tasks by document IDs (hard delete)
func (dao *TaskDAO) DeleteByDocIDs(ctx context.Context, db *gorm.DB, docIDs []string) (int64, error) {
	if len(docIDs) == 0 {
		return 0, nil
	}
	result := db.WithContext(ctx).Unscoped().Where("doc_id IN ?", docIDs).Delete(&entity.Task{})
	return result.RowsAffected, result.Error
}

// DeleteByTenantID deletes all tasks by tenant ID (hard delete via document join)
func (dao *TaskDAO) DeleteByTenantID(ctx context.Context, db *gorm.DB, tenantID string) (int64, error) {
	result := db.WithContext(ctx).Unscoped().Where("doc_id IN (SELECT id FROM document WHERE tenant_id = ?)", tenantID).Delete(&entity.Task{})
	return result.RowsAffected, result.Error
}

// GetByDocID gets all tasks by document ID
func (dao *TaskDAO) GetByDocID(ctx context.Context, db *gorm.DB, docID string) ([]*entity.Task, error) {
	var tasks []*entity.Task
	err := db.WithContext(ctx).Where("doc_id = ?", docID).Order("from_page ASC, create_time ASC").Find(&tasks).Error
	return tasks, err
}

func (dao *TaskDAO) GetAllTasks(ctx context.Context, db *gorm.DB) ([]*entity.Task, error) {
	var tasks []*entity.Task
	err := db.WithContext(ctx).Find(&tasks).Error
	return tasks, err
}
