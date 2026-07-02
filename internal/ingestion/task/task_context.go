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
	"fmt"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// TaskContext holds the full task context loaded from DB, equivalent to Python TaskService.get_task().
// Entity types are embedded directly to avoid manual field mapping.
type TaskContext struct {
	Task   entity.Task
	Doc    entity.Document
	KB     entity.Knowledgebase
	Tenant entity.Tenant
}

// LoadTaskContext loads the full task context following the FK chain: task → document → knowledgebase → tenant.
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

	return &TaskContext{
		Task:   *task,
		Doc:    *doc,
		KB:     *kb,
		Tenant: *tenant,
	}, nil
}
