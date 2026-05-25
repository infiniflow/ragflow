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

type IngestionTaskDAO struct{}

func NewIngestionTaskDAO() *IngestionTaskDAO {
	return &IngestionTaskDAO{}
}

func (dao *IngestionTaskDAO) Create(ingestionTask *entity.IngestionTask) error {
	return DB.Create(ingestionTask).Error
}

func (dao *IngestionTaskDAO) GetAllTasks() ([]*entity.IngestionTask, error) {
	var tasks []*entity.IngestionTask
	err := DB.Find(&tasks).Error
	return tasks, err
}

func (dao *IngestionTaskDAO) ListByUserID(userID string) ([]*entity.IngestionTask, error) {
	var tasks []*entity.IngestionTask
	err := DB.Where("user_id = ?", userID).Find(&tasks).Error
	return tasks, err
}

func (dao *IngestionTaskDAO) GetByID(id string) (*entity.IngestionTask, error) {
	var task *entity.IngestionTask
	err := DB.Where("id = ?", id).First(&task).Error
	return task, err
}

type IngestionTaskLogDAO struct{}

func NewIngestionTaskLogDAO() *IngestionTaskLogDAO {
	return &IngestionTaskLogDAO{}
}

func (dao *IngestionTaskLogDAO) Create(ingestionLog *entity.IngestionTaskLog) error {
	return DB.Create(ingestionLog).Error
}

func (dao *IngestionTaskLogDAO) ListLogsByTaskID(taskID string) ([]*entity.IngestionTaskLog, error) {
	var tasks []*entity.IngestionTaskLog
	err := DB.Where("task_id = ?", taskID).Find(&tasks).Error
	return tasks, err
}

func (dao *IngestionTaskLogDAO) GetLogByLogID(logID string) (*entity.IngestionTaskLog, error) {
	var task *entity.IngestionTaskLog
	err := DB.Where("id = ?", logID).First(&task).Error
	return task, err
}

func (dao *IngestionTaskLogDAO) DeleteByTaskID(taskID string) (int64, error) {
	result := DB.Unscoped().Where("task_id = ?", taskID).Delete(&entity.IngestionTaskLog{})
	return result.RowsAffected, result.Error
}

type IngestionTaskletDAO struct{}

func NewIngestionTaskletDAO() *IngestionTaskletDAO {
	return &IngestionTaskletDAO{}
}

func (dao *IngestionTaskletDAO) Create(ingestionTasklet *entity.IngestionTasklet) error {
	return DB.Create(ingestionTasklet).Error
}

func (dao *IngestionTaskletDAO) GetAllTasklets() ([]*entity.IngestionTasklet, error) {
	var tasks []*entity.IngestionTasklet
	err := DB.Find(&tasks).Error
	return tasks, err
}

func (dao *IngestionTaskletDAO) ListByUserID(userID string) ([]*entity.IngestionTasklet, error) {
	var tasks []*entity.IngestionTasklet
	err := DB.Where("user_id = ?", userID).Find(&tasks).Error
	return tasks, err
}

func (dao *IngestionTaskletDAO) GetByID(id string) (*entity.IngestionTasklet, error) {
	var task *entity.IngestionTasklet
	err := DB.Where("id = ?", id).First(&task).Error
	return task, err
}

type IngestionTaskletLogDAO struct{}

func NewIngestionTaskletLogDAO() *IngestionTaskletLogDAO {
	return &IngestionTaskletLogDAO{}
}

func (dao *IngestionTaskletLogDAO) Create(ingestionLog *entity.IngestionTaskletLog) error {
	return DB.Create(ingestionLog).Error
}

func (dao *IngestionTaskletLogDAO) ListLogsByTaskletID(taskID string) ([]*entity.IngestionTaskletLog, error) {
	var tasks []*entity.IngestionTaskletLog
	err := DB.Where("task_id = ?", taskID).Find(&tasks).Error
	return tasks, err
}

func (dao *IngestionTaskletLogDAO) GetLogByLogID(logID string) (*entity.IngestionTaskletLog, error) {
	var task *entity.IngestionTaskletLog
	err := DB.Where("id = ?", logID).First(&task).Error
	return task, err
}

func (dao *IngestionTaskletLogDAO) DeleteByTaskletID(taskID string) (int64, error) {
	result := DB.Unscoped().Where("task_id = ?", taskID).Delete(&entity.IngestionTaskletLog{})
	return result.RowsAffected, result.Error
}
