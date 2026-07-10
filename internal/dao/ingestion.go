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
	"ragflow/internal/utility"
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
		ingestionTask.ID = utility.GenerateUUID()
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

// UpdateComponentTotal records the number of components in the task's DSL
// graph. It is the authoritative denominator for progress percentage.
func (dao *IngestionTaskDAO) UpdateComponentTotal(taskID string, total int) error {
	return DB.Model(&entity.IngestionTask{}).Where("id = ?", taskID).Update("component_total", total).Error
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

type TaskInfo struct {
	TaskID        string   `json:"task_id"`
	FilesToDelete []string `json:"files_to_delete"`
}

func (dao *IngestionTaskDAO) Delete(taskID string, userID *string) (*TaskInfo, error) {
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
		// ingestion_task_log no longer carries file references (the old
		// checkpoint JSON column was dropped in favor of typed columns), so
		// there are no task-level files to delete here.
		var filesToDelete []string

		err = tx.Model(&entity.IngestionTask{}).Where("id = ?", taskID).Delete(&entity.IngestionTask{}).Error
		if err != nil {
			return nil, err
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
	var tasks []*entity.IngestionTask
	err := DB.Where("document_id = ?", documentId).Limit(1).Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, nil
	}
	return tasks[0], nil
}

type IngestionTaskLogDAO struct{}

func NewIngestionTaskLogDAO() *IngestionTaskLogDAO {
	return &IngestionTaskLogDAO{}
}

func (dao *IngestionTaskLogDAO) Create(ingestionLog *entity.IngestionTaskLog) error {
	return DB.Create(ingestionLog).Error
}

func (dao *IngestionTaskLogDAO) Update(ingestionLog *entity.IngestionTaskLog) error {
	return DB.Save(ingestionLog).Error
}

// ListLogsByTaskID returns the task's logs in chronological (write) order.
// Ordering is by auto-increment `id ASC` (NOT `create_time`) because
// create_time has only second-level resolution and would tie-break
// arbitrarily; `id` is monotonic and always reflects write order. This
// feeds the frontend log stream (GET .../logs), which renders each row by
// phase (0 started / 1 done / -1 failed).
func (dao *IngestionTaskLogDAO) ListLogsByTaskID(taskID string) ([]*entity.IngestionTaskLog, error) {
	var tasks []*entity.IngestionTaskLog
	err := DB.Where("task_id = ?", taskID).Order("id ASC").Find(&tasks).Error
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
func (dao *IngestionTaskLogDAO) AggregateProgress(taskID string, total int) (*TaskProgress, error) {
	// Latest row id per component for this task.
	latestIDs := DB.Model(&entity.IngestionTaskLog{}).
		Select("MAX(id)").
		Where("task_id = ?", taskID).
		Group("component")

	type phaseRow struct {
		Phase int
	}
	var rows []phaseRow
	err := DB.Model(&entity.IngestionTaskLog{}).
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
