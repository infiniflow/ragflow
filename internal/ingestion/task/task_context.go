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
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// TaskKind discriminates which execution path a queued TaskContext takes.
type TaskKind int

const (
	// TaskKindIngestion is an ingestion document task (IngestionTask set).
	TaskKindIngestion TaskKind = iota
	// TaskKindMemory is an async memory-extraction task (IngestionTask nil). It
	// shares the worker pool with ingestion tasks but
	// runs through executeMemoryTask instead of the ingestion state machine.
	TaskKindMemory
)

// TaskContext holds the execution inputs for an ingestion document task or a
// memory-extraction task. Ingestion tasks populate IngestionTask and the
// document/KB/tenant chain; memory tasks carry only the durable task id and
// leave IngestionTask nil.
type TaskContext struct {
	Ctx context.Context

	// Kind selects the execution path: TaskKindIngestion or TaskKindMemory.
	Kind TaskKind

	IngestionTask *entity.IngestionTask

	Doc    entity.Document
	KB     entity.Knowledgebase
	Tenant entity.Tenant

	PipelineID string
	File       any

	// taskID is the authoritative TaskMessage identity used for admission,
	// execution, settlement, and logging. It is unexported so it cannot change
	// while a worker owns the delivery. There is deliberately no fallback chain
	// in ID(): identity lives only here, never in task-specific data.
	taskID string

	// Handle is the message-queue ack handle for the task message that scheduled
	// this context. The scheduler sets it before queueing; the worker decides
	// the terminal Ack/Nack:
	//   - TaskKindIngestion: ack on a durably-persisted terminal status and
	//     nack otherwise (e.g. shutdown mid-task) so the message is redelivered
	//     and resumed after restart.
	//   - TaskKindMemory: ack when the durable task runner permits it; otherwise
	//     leave the delivery unsettled for broker or reconciler recovery.
	Handle common.TaskHandle
}

// NewMemoryTaskContextForScheduling creates a lightweight TaskContext for a
// memory-extraction task. taskID is the envelope TaskMessage.TaskID that the
// scheduler receives; it is the authoritative identity for the whole memory
// task lifecycle. Only scheduling fields are set, not full business data.
func NewMemoryTaskContextForScheduling(ctx context.Context, taskID string, handle common.TaskHandle) *TaskContext {
	return &TaskContext{
		Ctx:    ctx,
		Kind:   TaskKindMemory,
		taskID: taskID,
		Handle: handle,
	}
}

// NewTaskContextForScheduling creates a lightweight TaskContext for queue scheduling.
// This only sets the scheduling-related fields, not the full business data.
func NewTaskContextForScheduling(ctx context.Context, task *entity.IngestionTask) *TaskContext {
	return &TaskContext{
		Ctx:           ctx,
		Kind:          TaskKindIngestion,
		taskID:        task.ID,
		IngestionTask: task,
	}
}

// ID returns the task identifier for claim/release and logging. It is the
// envelope TaskMessage.TaskID captured at construction — there is deliberately
// no fallback to IngestionTask or broker payload, so identity is never
// re-derived from a source that could disagree with the claim key.
func (c *TaskContext) ID() string {
	if c == nil {
		return ""
	}
	return c.taskID
}

// LoadFromIngestionTask loads the full task context from an IngestionTask.
// It follows the FK chain: ingestion task -> document -> knowledgebase -> tenant.
func LoadFromIngestionTask(ctx context.Context, ingestionTask *entity.IngestionTask) (*TaskContext, error) {
	doc, err := dao.NewDocumentDAO().GetByID(ctx, dao.DB, ingestionTask.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("load document %s: %w", ingestionTask.DocumentID, err)
	}
	if doc == nil {
		return nil, fmt.Errorf("document %s not found", ingestionTask.DocumentID)
	}

	kb, err := dao.NewKnowledgebaseDAO().GetByID(ctx, dao.DB, doc.KbID)
	if err != nil || kb == nil {
		return nil, fmt.Errorf("error when load knowledgebase %s: %w", doc.KbID, err)
	}

	tenant, err := dao.NewTenantDAO().GetByID(ctx, dao.DB, kb.TenantID)
	if err != nil || tenant == nil {
		return nil, fmt.Errorf("error when load tenant %s: %w", kb.TenantID, err)
	}

	pipelineID := resolvePipelineID(doc)

	return &TaskContext{
		Ctx:           ctx,
		taskID:        ingestionTask.ID,
		IngestionTask: ingestionTask,
		PipelineID:    pipelineID,
		Doc:           *doc,
		KB:            *kb,
		Tenant:        *tenant,
	}, nil
}

// resolvePipelineID resolves the pipeline selected for a document.
func resolvePipelineID(doc *entity.Document) string {
	if doc != nil && doc.PipelineID != nil {
		return strings.TrimSpace(*doc.PipelineID)
	}
	return ""
}
