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

package task

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/semaphore"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// TaskLog represents a task execution log entry.
type TaskLog struct {
	StartTime   time.Time              `json:"start_time"`
	EndTime     time.Time              `json:"end_time"`
	Description string                 `json:"description"`
	Details     map[string]interface{} `json:"details"`
}

// TaskContext holds the full task context loaded from DB, equivalent to Python TaskService.get_task().
// Entity types are embedded directly to avoid manual field mapping.
type TaskContext struct {
	// Context and execution control (from service package)
	Ctx        context.Context
	TaskHandle common.TaskHandle

	// IngestionTask is the ingestion task entity (from service package)
	IngestionTask *entity.IngestionTask

	// Progress and status tracking (from service package)
	Logs         []*TaskLog
	Progress     int32
	ErrorMessage string

	// Task type for handling dispatch
	TaskType string

	// Business data (original task package fields)
	Doc    entity.Document
	KB     entity.Knowledgebase
	Tenant entity.Tenant

	// pipeline used to parse the document
	PipelineID string

	// File is an optional file object for dataflow pipeline debugging.
	// Mirrors Python: task["file"] — passed through to Pipeline.run().
	File any

	// EmbedLimiter limits embedding API concurrency.
	// If nil, no concurrency limit is applied.
	// Mirrors Python: ctx.embed_limiter (asyncio.Semaphore)
	EmbedLimiter *semaphore.Weighted

	// ProgressFunc updates the task-level progress state.
	// If nil, a no-op callback is used.
	ProgressFunc ProgressFunc
}

// NewTaskContextForScheduling creates a lightweight TaskContext for queue scheduling.
// This only sets the scheduling-related fields, not the full business data.
func NewTaskContextForScheduling(ctx context.Context, task *entity.IngestionTask, handle common.TaskHandle) *TaskContext {
	return &TaskContext{
		Ctx:           ctx,
		IngestionTask: task,
		TaskHandle:    handle,
	}
}

// LoadFromIngestionTask loads the full task context from an IngestionTask.
// It follows the FK chain: ingestion task → document → knowledgebase → tenant.
func LoadFromIngestionTask(ingestionTask *entity.IngestionTask) (*TaskContext, error) {
	doc, err := dao.NewDocumentDAO().GetByID(ingestionTask.DocumentID)
	if err != nil || doc == nil {
		return nil, fmt.Errorf("error when load document %s : %w", ingestionTask.DocumentID, err)
	}

	kb, err := dao.NewKnowledgebaseDAO().GetByID(doc.KbID)
	if err != nil || kb == nil {
		return nil, fmt.Errorf("error when load knowledgebase %s: %w", doc.KbID, err)
	}

	tenant, err := dao.NewTenantDAO().GetByID(kb.TenantID)
	if err != nil || tenant == nil {
		return nil, fmt.Errorf("error when load tenant %s: %w", kb.TenantID, err)
	}

	pipelineID := ""
	if doc.PipelineID != nil {
		pipelineID = *doc.PipelineID
	}

	return &TaskContext{
		IngestionTask: ingestionTask,
		TaskType:      "dataflow",
		PipelineID:    pipelineID,
		Doc:           *doc,
		KB:            *kb,
		Tenant:        *tenant,
	}, nil
}

// LoadTaskContext loads the full task context following the FK chain: task → document → knowledgebase → tenant.
// Kept for backward compatibility.
func LoadTaskContext(taskID string) (*TaskContext, error) {
	task, err := dao.NewTaskDAO().GetByID(taskID)
	if err != nil {
		return nil, fmt.Errorf("task %s: %w", taskID, err)
	}

	doc, err := dao.NewDocumentDAO().GetByID(task.DocID)
	if err != nil || doc == nil {
		return nil, fmt.Errorf("document %s for task %s: %w", task.DocID, taskID, err)
	}

	kb, err := dao.NewKnowledgebaseDAO().GetByID(doc.KbID)
	if err != nil || kb == nil {
		return nil, fmt.Errorf("knowledgebase %s for doc %s: %w", doc.KbID, doc.ID, err)
	}

	tenant, err := dao.NewTenantDAO().GetByID(kb.TenantID)
	if err != nil || tenant == nil {
		return nil, fmt.Errorf("tenant %s for kb %s: %w", kb.TenantID, kb.ID, err)
	}

	pipelineID := ""
	if doc.PipelineID != nil {
		pipelineID = *doc.PipelineID
	}

	return &TaskContext{
		TaskType:     task.TaskType,
		PipelineID:   pipelineID,
		Doc:          *doc,
		KB:           *kb,
		Tenant:       *tenant,
		ProgressFunc: func(prog float64, msg string) {},
	}, nil
}
