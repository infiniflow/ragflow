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
	"strings"
)

// TaskHandler dispatches document processing tasks by task_type.
// Mirrors Python task_handler.py:handle().
type TaskHandler struct {
	ctx                 *TaskContext
	newPipelineExecutor func(ctx *TaskContext, canvasID string) (*PipelineExecutor, error)
}

// NewTaskHandler creates a TaskHandler for the given task context.
func NewTaskHandler(ctx *TaskContext) *TaskHandler {
	return &TaskHandler{
		ctx: ctx,
		newPipelineExecutor: func(ctx *TaskContext, canvasID string) (*PipelineExecutor, error) {
			return NewPipelineExecutor(ctx, canvasID, 0)
		},
	}
}

func (h *TaskHandler) WithPipelineExecutorFactory(factory func(ctx *TaskContext, canvasID string) (*PipelineExecutor, error)) *TaskHandler {
	h.newPipelineExecutor = factory
	return h
}

// Handle routes the task by type and executes the appropriate handler.
func (h *TaskHandler) Handle() (*PipelineResult, error) {
	if h.ctx == nil {
		return nil, fmt.Errorf("task handler: nil context")
	}

	return h.handlePipeline()
}

func (h *TaskHandler) handlePipeline() (*PipelineResult, error) {
	svc, err := h.newPipelineExecutor(h.ctx, strings.TrimSpace(h.ctx.PipelineID))
	if err != nil {
		return nil, err
	}
	return svc.Execute(h.ctx.Ctx)
}
