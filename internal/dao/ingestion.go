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
	"errors"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/entity"
)

type IngestionTaskDAO struct{}

func NewIngestionTaskDAO() *IngestionTaskDAO {
	return &IngestionTaskDAO{}
}

// Use by api server to create task
// created → running : After the ingestor component assigns the task, it changes the status to running
// running → completed : Task executes successfully
// running → failed : Error occurs during execution
// created → canceling : User cancels before the task is picked up by the ingestor
// running → canceling : User cancels during execution
// completed → canceling : User cancels a completed task (e.g., for cleanup/rollback)
// canceling → canceled : Cancellation completes
// failed → created : Retry (back to start)
// canceled → created : Retry/re-execute (back to start)
func (dao *IngestionTaskDAO) CheckAndCreate(ingestionTask *entity.IngestionTask) (*entity.IngestionTask, error) {

	tx := DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// Check if the task is created
	var taskRecord *entity.IngestionTask
	err := tx.Where("document_id = ?", ingestionTask.DocumentID).First(&taskRecord).Error
	if err == nil {
		// found
		if taskRecord.Status == common.FAILED || taskRecord.Status == common.STOPPED {
			// restart the task
			err = tx.Model(&entity.IngestionTask{}).Where("id = ?", taskRecord.ID).Update("status", common.CREATED).Error
			if err != nil {
				tx.Rollback()
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("document id %s already exists, status: %s, task id: %s", ingestionTask.DocumentID, taskRecord.Status, taskRecord.ID)
		}
	} else {
		// create ingestion task
		ingestionTask.ID = common.GenerateUUID()
		if err = tx.Create(ingestionTask).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
		taskRecord = ingestionTask
	}

	if err = tx.Commit().Error; err != nil {
		return nil, err
	}

	return taskRecord, nil
}

// UpdateStatus Update ingestion task status
func (dao *IngestionTaskDAO) UpdateStatus(taskID, status string) error {
	return DB.Model(&entity.IngestionTask{}).Where("id = ?", taskID).Update("status", status).Error
}

// CheckAnd called by ingestor
// if task status is RUNNING, COMPLETED, STOPPED, FAILED, just return without error
// if task status is CREATE, update to RUNNING
// if task status is STOPPING, update to STOPPED
func (dao *IngestionTaskDAO) SetRunningByIngestor(taskID string) (*entity.IngestionTask, error) {

	tx := DB.Begin()
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
		return nil, common.ErrTaskNotFound
	}

	if len(tasks) != 1 {
		return nil, fmt.Errorf("task %s has multiple records", taskID)
	}

	taskStatus := tasks[0].Status
	switch taskStatus {
	case common.CREATED:
		tasks[0].Status = common.RUNNING
		err = tx.Model(&entity.IngestionTask{}).Where("id = ?", taskID).Update("status", common.RUNNING).Error
		if err != nil {
			return nil, err
		}
		committed = true
		return tasks[0], nil
	case common.STOPPING:
		tasks[0].Status = common.STOPPED
		err = tx.Model(&entity.IngestionTask{}).Where("id = ?", taskID).Update("status", common.STOPPED).Error
		if err != nil {
			return nil, err
		}
		committed = true
		return tasks[0], nil
	case common.RUNNING:
		// this task was executing before, just return without error
		committed = true
		return tasks[0], nil
	default:
		return tasks[0], nil
	}
}

func (dao *IngestionTaskDAO) SetStoppingByAPIServer(taskID string) (*entity.IngestionTask, error) {

	tx := DB.Begin()
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

	taskStatus := tasks[0].Status
	switch taskStatus {
	case common.CREATED:
		tasks[0].Status = common.STOPPED
		err = tx.Model(&entity.IngestionTask{}).Where("id = ?", taskID).Update("status", common.STOPPED).Error
		if err != nil {
			return nil, err
		}
		committed = true
		return tasks[0], nil
	case common.RUNNING:
		tasks[0].Status = common.STOPPING
		err = tx.Model(&entity.IngestionTask{}).Where("id = ?", taskID).Update("status", common.STOPPING).Error
		if err != nil {
			return nil, err
		}
		committed = true
		return tasks[0], nil
	default:
		return tasks[0], nil
	}
}

type TaskletInfo struct {
	TaskletID     string   `json:"tasklet_id"`
	FilesToDelete []string `json:"files_to_delete"`
}

type TaskInfo struct {
	TaskID        string        `json:"task_id"`
	FilesToDelete []string      `json:"files_to_delete"`
	Tasklets      []TaskletInfo `json:"tasklets"`
}

func (dao *IngestionTaskDAO) RemoveByAPIServerOrAdminServer(taskID string, userID *string) (*TaskInfo, error) {

	tx := DB.Begin()
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
	case common.CREATED, common.STOPPED, common.COMPLETED, common.FAILED:
		// get all ingestion tasklets
		var tasklets []*entity.IngestionTasklet
		err = tx.Where("task_id = ?", taskID).Find(&tasklets).Error
		if err != nil {
			return nil, err
		}
		var TaskletInfos []TaskletInfo
		for _, tasklet := range tasklets {
			// get all ingestion tasklet log
			var taskletLogs []*entity.IngestionTaskletLog
			err = tx.Where("tasklet_id = ?", tasklet.ID).Find(&taskletLogs).Error

			fileMap := make(map[string]bool)
			for _, taskletLog := range taskletLogs {
				files, ok := taskletLog.Checkpoint["files"].([]string)
				if ok {
					for _, file := range files {
						fileMap[file] = true
					}
				}
			}
			var filesToDelete []string
			for file := range fileMap {
				filesToDelete = append(filesToDelete, file)
			}
			TaskletInfos = append(TaskletInfos, TaskletInfo{
				TaskletID:     tasklet.ID,
				FilesToDelete: filesToDelete,
			})
		}

		// get all ingestion task log
		var taskLogs []*entity.IngestionTaskLog
		err = tx.Where("task_id = ?", taskID).Find(&taskLogs).Error
		if err != nil {
			return nil, err
		}

		fileMap := make(map[string]bool)
		for _, taskLog := range taskLogs {
			files, ok := taskLog.Checkpoint["files"].([]string)
			if ok {
				for _, file := range files {
					fileMap[file] = true
				}
			}
		}
		var filesToDelete []string
		for file := range fileMap {
			filesToDelete = append(filesToDelete, file)
		}

		err = tx.Model(&entity.IngestionTask{}).Where("id = ?", taskID).Delete(&entity.IngestionTask{}).Error
		if err != nil {
			return nil, err
		}

		taskInfo := &TaskInfo{
			TaskID:        taskID,
			FilesToDelete: filesToDelete,
			Tasklets:      TaskletInfos,
		}
		committed = true
		return taskInfo, nil
	default:
		return nil, fmt.Errorf("task %s is executing, cannot be removed", taskID)
	}
}

func (dao *IngestionTaskDAO) GetAllTasks(page, pageSize int) ([]*entity.IngestionTask, error) {
	var tasks []*entity.IngestionTask
	var err error
	if pageSize == 0 {
		err = DB.Find(&tasks).Error
	} else {
		err = DB.Order("create_time DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&tasks).Error
	}
	return tasks, err
}

func (dao *IngestionTaskDAO) ListByUserID(userID string, page, pageSize int) ([]*entity.IngestionTask, error) {
	var tasks []*entity.IngestionTask
	var err error
	if pageSize == 0 {
		err = DB.Where("user_id = ?", userID).Order("create_time DESC").Find(&tasks).Error
	} else {
		err = DB.Where("user_id = ?", userID).Order("create_time DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&tasks).Error
	}

	return tasks, err
}

func (dao *IngestionTaskDAO) ListByUserIDAndDatasetID(userID, datasetID string, page, pageSize int) ([]*entity.IngestionTask, error) {
	var tasks []*entity.IngestionTask
	var err error
	if pageSize == 0 {
		err = DB.Where("user_id = ? AND dataset_id = ?", userID, datasetID).Order("create_time DESC").Find(&tasks).Error
	} else {
		err = DB.Where("user_id = ? AND dataset_id = ?", userID, datasetID).Order("create_time DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&tasks).Error
	}

	return tasks, err
}

func (dao *IngestionTaskDAO) GetByID(id string) (*entity.IngestionTask, error) {
	var task *entity.IngestionTask
	err := DB.Where("id = ?", id).First(&task).Error
	return task, err
}

func (dao *IngestionTaskDAO) GetByDocumentID(documentId string) (*entity.IngestionTask, error) {
	var task *entity.IngestionTask
	err := DB.Where("document_id = ?", documentId).First(&task).Error
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
	err := DB.Where("task_id = ?", taskID).Order("create_time DESC").Find(&tasks).Error
	return tasks, err
}

func (dao *IngestionTaskLogDAO) LatestLogByTaskID(taskID string) (*entity.IngestionTaskLog, error) {
	var task *entity.IngestionTaskLog
	err := DB.Where("task_id = ?", taskID).Order("create_time DESC").First(&task).Error
	return task, err
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

func (dao *IngestionTaskletDAO) UpdateStatus(taskletID, status string) error {
	return DB.Model(&entity.IngestionTasklet{}).Where("id = ?", taskletID).Update("status", status).Error
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

func (dao *IngestionTaskletLogDAO) LatestLogByTaskletID(taskletID string) (*entity.IngestionTaskletLog, error) {
	var tasklet *entity.IngestionTaskletLog
	err := DB.Where("tasklet_id = ?", taskletID).Order("create_time DESC").First(&tasklet).Error
	return tasklet, err
}

func (dao *IngestionTaskletLogDAO) DeleteByTaskletID(taskID string) (int64, error) {
	result := DB.Unscoped().Where("task_id = ?", taskID).Delete(&entity.IngestionTaskletLog{})
	return result.RowsAffected, result.Error
}
