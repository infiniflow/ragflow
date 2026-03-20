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

package service

import (
	"ragflow/internal/dao"
	"ragflow/internal/model"
)

// TaskService task service for managing task operations
type TaskService struct {
	taskDAO *dao.TaskDAO
}

// NewTaskService creates a new task service instance
//
// Returns:
//   - *TaskService: a new TaskService instance
//
// Example:
//
//	taskService := NewTaskService()
//	tasks, err := taskService.GetTasksProgressByDocIDs([]string{"doc1", "doc2"})
func NewTaskService() *TaskService {
	return &TaskService{
		taskDAO: dao.NewTaskDAO(),
	}
}

// TaskProgressResponse represents the task progress information
// This struct contains the fields returned by GetTasksProgressByDocIDs
type TaskProgressResponse struct {
	ID          string  `json:"id"`
	DocID       string  `json:"doc_id"`
	FromPage    int64   `json:"from_page"`
	Progress    float64 `json:"progress"`
	ProgressMsg *string `json:"progress_msg,omitempty"`
	Digest      *string `json:"digest,omitempty"`
	ChunkIDs    *string `json:"chunk_ids,omitempty"`
	CreateTime  *int64  `json:"create_time,omitempty"`
}

// GetTasksProgressByDocIDs retrieves all tasks associated with specific documents.
// This method fetches all processing tasks for given document IDs, ordered by
// creation time in descending order. It includes task progress and chunk information.
//
// This is the Go implementation of Python's get_tasks_progress_by_doc_ids method
// in api/db/services/task_service.py (lines 177-207).
//
// Parameters:
//   - docIDs: slice of document IDs to query tasks for
//
// Returns:
//   - []TaskProgressResponse: slice of task progress information
//   - error: error if query fails, nil otherwise
//
// Example:
//
//	taskService := NewTaskService()
//	docIDs := []string{"doc_id_1", "doc_id_2"}
//	tasks, err := taskService.GetTasksProgressByDocIDs(docIDs)
//	if err != nil {
//	    log.Printf("Failed to get tasks: %v", err)
//	    return
//	}
//	for _, task := range tasks {
//	    fmt.Printf("Task ID: %s, Progress: %.2f%%\n", task.ID, task.Progress)
//	}
func (s *TaskService) GetTasksProgressByDocIDs(docIDs []string) ([]TaskProgressResponse, error) {
	// Return empty slice if no docIDs provided
	if len(docIDs) == 0 {
		return nil, nil
	}

	// Query tasks from database
	var tasks []model.Task
	err := dao.DB.Model(&model.Task{}).
		Select("id, doc_id, from_page, progress, progress_msg, digest, chunk_ids, create_time").
		Where("doc_id IN ?", docIDs).
		Order("create_time DESC").
		Find(&tasks).Error

	if err != nil {
		return nil, err
	}

	// Return nil if no tasks found (matching Python behavior)
	if len(tasks) == 0 {
		return nil, nil
	}

	// Convert model.Task slice to TaskProgressResponse slice
	responses := make([]TaskProgressResponse, len(tasks))
	for i, task := range tasks {
		responses[i] = TaskProgressResponse{
			ID:          task.ID,
			DocID:       task.DocID,
			FromPage:    task.FromPage,
			Progress:    task.Progress,
			ProgressMsg: task.ProgressMsg,
			Digest:      task.Digest,
			ChunkIDs:    task.ChunkIDs,
			CreateTime:  task.CreateTime,
		}
	}

	return responses, nil
}
