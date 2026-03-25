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
	"ragflow/internal/model"
)

// TaskDAO task data access object
type TaskDAO struct{}

// NewTaskDAO create task DAO
func NewTaskDAO() *TaskDAO {
	return &TaskDAO{}
}

// Create creates a new task
func (dao *TaskDAO) Create(task *model.Task) error {
	return DB.Create(task).Error
}

// GetByID gets task by ID
func (dao *TaskDAO) GetByID(id string) (*model.Task, error) {
	var task model.Task
	err := DB.Where("id = ?", id).First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// DeleteByDocIDs deletes tasks by document IDs (hard delete)
func (dao *TaskDAO) DeleteByDocIDs(docIDs []string) (int64, error) {
	if len(docIDs) == 0 {
		return 0, nil
	}
	result := DB.Unscoped().Where("doc_id IN ?", docIDs).Delete(&model.Task{})
	return result.RowsAffected, result.Error
}

// DeleteByTenantID deletes all tasks by tenant ID (hard delete via document join)
func (dao *TaskDAO) DeleteByTenantID(tenantID string) (int64, error) {
	result := DB.Unscoped().Where("doc_id IN (SELECT id FROM document WHERE tenant_id = ?)", tenantID).Delete(&model.Task{})
	return result.RowsAffected, result.Error
}
