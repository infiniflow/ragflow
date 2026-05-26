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
	"ragflow/internal/entity"
)

type IngestionDAO struct{}

func NewIngestionDAO() *IngestionDAO {
	return &IngestionDAO{}
}

func (dao *IngestionDAO) Create(ingestionTask *entity.IngestionTask) error {

	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// create ingestion task
	if err := DB.Create(ingestionTask).Error; err != nil {
		tx.Rollback()
		return err
	}

	taskLog := &entity.IngestionTaskLog{
		TaskID: ingestionTask.ID,
		Stage:  0,
	}

	// create task log
	if err := DB.Create(taskLog).Error; err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit().Error; err != nil {
		return err
	}

	return nil
}

func (dao *IngestionDAO) GetAllTasks() ([]*entity.IngestionTask, error) {
	var tasks []*entity.IngestionTask
	err := DB.Find(&tasks).Error
	return tasks, err
}

func (dao *IngestionDAO) ListByUserID(userID string) ([]*entity.IngestionTask, error) {
	var tasks []*entity.IngestionTask
	err := DB.Where("user_id = ?", userID).Find(&tasks).Error
	return tasks, err
}

func (dao *IngestionDAO) GetByID(id string) (*entity.IngestionTask, error) {
	var task *entity.IngestionTask
	err := DB.Where("id = ?", id).First(&task).Error
	return task, err
}

type IngestionLogDAO struct{}

func NewIngestionLogDAO() *IngestionLogDAO {
	return &IngestionLogDAO{}
}

func (dao *IngestionLogDAO) Create(ingestionLog *entity.IngestionTaskLog) error {
	return DB.Create(ingestionLog).Error
}

func (dao *IngestionDAO) ListLogsByTaskID(taskID string) ([]*entity.IngestionTaskLog, error) {
	var tasks []*entity.IngestionTaskLog
	err := DB.Where("task_id = ?", taskID).Find(&tasks).Error
	return tasks, err
}

func (dao *IngestionDAO) GetLogByLogID(logID string) (*entity.IngestionTaskLog, error) {
	var task *entity.IngestionTaskLog
	err := DB.Where("id = ?", logID).First(&task).Error
	return task, err
}

func (dao *IngestionDAO) DeleteByTaskID(taskID string) (int64, error) {
	result := DB.Unscoped().Where("task_id = ?", taskID).Delete(&entity.IngestionTaskLog{})
	return result.RowsAffected, result.Error
}
