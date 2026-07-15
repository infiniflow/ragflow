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

// TaskContext holds the execution inputs for an ingestion document task.
type TaskContext struct {
	Ctx context.Context

	IngestionTask *entity.IngestionTask

	Doc    entity.Document
	KB     entity.Knowledgebase
	Tenant entity.Tenant

	PipelineID string
	File       any

	// Handle is the message-queue ack handle for the task message that scheduled
	// this context. The scheduler sets it before queueing; the worker acks on a
	// durably-persisted terminal status and nacks otherwise (e.g. shutdown
	// mid-task) so the message is redelivered and resumed after restart.
	Handle common.TaskHandle
}

// NewTaskContextForScheduling creates a lightweight TaskContext for queue scheduling.
// This only sets the scheduling-related fields, not the full business data.
func NewTaskContextForScheduling(ctx context.Context, task *entity.IngestionTask) *TaskContext {
	return &TaskContext{
		Ctx:           ctx,
		IngestionTask: task,
	}
}

// LoadFromIngestionTask loads the full task context from an IngestionTask.
// It follows the FK chain: ingestion task -> document -> knowledgebase -> tenant.
func LoadFromIngestionTask(ingestionTask *entity.IngestionTask) (*TaskContext, error) {
	doc, err := dao.NewDocumentDAO().GetByID(ingestionTask.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("load document %s: %w", ingestionTask.DocumentID, err)
	}
	if doc == nil {
		return nil, fmt.Errorf("document %s not found", ingestionTask.DocumentID)
	}

	kb, err := dao.NewKnowledgebaseDAO().GetByID(doc.KbID)
	if err != nil || kb == nil {
		return nil, fmt.Errorf("error when load knowledgebase %s: %w", doc.KbID, err)
	}

	tenant, err := dao.NewTenantDAO().GetByID(kb.TenantID)
	if err != nil || tenant == nil {
		return nil, fmt.Errorf("error when load tenant %s: %w", kb.TenantID, err)
	}

	pipelineID := resolvePipelineID(doc, kb)

	return &TaskContext{
		IngestionTask: ingestionTask,
		PipelineID:    pipelineID,
		Doc:           *doc,
		KB:            *kb,
		Tenant:        *tenant,
	}, nil
}

func resolvePipelineID(doc *entity.Document, kb *entity.Knowledgebase) string {
	if doc != nil && doc.PipelineID != nil {
		if pipelineID := strings.TrimSpace(*doc.PipelineID); pipelineID != "" {
			return pipelineID
		}
	}
	if kb != nil && kb.PipelineID != nil {
		if pipelineID := strings.TrimSpace(*kb.PipelineID); pipelineID != "" {
			return pipelineID
		}
	}
	return ""
}
