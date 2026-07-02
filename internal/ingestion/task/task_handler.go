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
	"strings"
)

// TaskHandler dispatches document processing tasks by task_type.
// Mirrors Python task_handler.py:handle().
type TaskHandler struct {
	ctx *TaskContext
}

// NewTaskHandler creates a TaskHandler for the given task context.
func NewTaskHandler(ctx *TaskContext) *TaskHandler {
	return &TaskHandler{ctx: ctx}
}

// Handle routes the task by type and executes the appropriate handler.
func (h *TaskHandler) Handle() error {
	taskType := h.ctx.Task.TaskType

	switch {
	case taskType == "memory":
		return h.handleMemory()
	case taskType == "dataflow" && h.ctx.Doc.ID == CANVAS_DEBUG_DOC_ID:
		return h.handleDataflow()
	case strings.HasPrefix(taskType, "dataflow"):
		return h.handleDataflow()
	case taskType == "raptor":
		return h.handleRaptor()
	case taskType == "graphrag":
		return h.handleGraphRAG()
	case taskType == "mindmap":
		return h.handleStub("mindmap")
	case taskType == "evaluation":
		return h.handleStub("evaluation")
	case taskType == "reembedding":
		return h.handleStub("reembedding")
	case taskType == "clone":
		return h.handleStub("clone")
	default:
		return h.handleStandard()
	}
}

func (h *TaskHandler) handleMemory() error {
	return nil // stub
}

func (h *TaskHandler) handleDataflow() error {

	return nil // stub
}

func (h *TaskHandler) handleRaptor() error {
	return nil // stub
}

func (h *TaskHandler) handleGraphRAG() error {
	return nil // stub
}

func (h *TaskHandler) handleStub(name string) error {
	return nil
}

func (h *TaskHandler) handleStandard() error {
	return nil // stub: parser not yet ready
}
