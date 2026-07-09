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

	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/service"
)

// TaskHandler dispatches document processing tasks by task_type.
// Mirrors Python task_handler.py:handle().
type TaskHandler struct {
	ctx                *TaskContext
	newDataflowService func(ctx *TaskContext, dataflowID string) (*PipelineExecutor, error)
}

// NewTaskHandler creates a TaskHandler for the given task context.
func NewTaskHandler(ctx *TaskContext) *TaskHandler {
	return &TaskHandler{
		ctx: ctx,
		newDataflowService: func(ctx *TaskContext, dataflowID string) (*PipelineExecutor, error) {
			return NewDataflowService(ctx, dataflowID, 0, 0)
		},
	}
}

func (h *TaskHandler) WithDataflowServiceFactory(factory func(ctx *TaskContext, dataflowID string) (*PipelineExecutor, error)) *TaskHandler {
	h.newDataflowService = factory
	return h
}

// Handle routes the task by type and executes the appropriate handler.
func (h *TaskHandler) Handle() error {
	if h.ctx == nil {
		return fmt.Errorf("task handler: nil context")
	}
	taskType := h.ctx.TaskType
	if taskType == "" {
		// Determine task type - use PipelineID presence as indicator for dataflow
		taskType = "dataflow" // Default to dataflow for now
		if h.ctx.PipelineID == "" {
			taskType = "standard"
		}
	}

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
	dataflowID := ""
	if strings.Trim(h.ctx.PipelineID, " ") != "" {
		dataflowID = h.ctx.PipelineID
	}
	svc, err := h.newDataflowService(h.ctx, dataflowID)
	if err != nil {
		return err
	}
	return svc.Run(context.Background())
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
	return nil // stub
}

// BindEmbeddingModel creates an embedding model for the task's tenant.
// Returns the model and its vector dimension.
// Mirrors Python: _bind_embedding_model (task_handler.py:204)
func (h *TaskHandler) BindEmbeddingModel() (*models.EmbeddingModel, int, error) {
	var model *models.EmbeddingModel
	var err error
	if embdID := h.ctx.KB.EmbdID; embdID != "" {
		modelSvc := service.NewModelProviderService()
		model, err = modelSvc.GetEmbeddingModel(h.ctx.Tenant.ID, embdID)
	} else {
		// Use tenant's default embedding model
		model, err = defaultBindEmbeddingModel(h.ctx.Tenant.ID)
	}

	if err != nil {
		return nil, 0, fmt.Errorf("bind embedding model: %w", err)
	}

	dim, err := getEmbeddingDimension(model)
	if err != nil {
		return nil, 0, fmt.Errorf("bind embedding model: %w", err)
	}
	return model, dim, err
}

// defaultBindEmbeddingModel returns the tenant's default embedding model and its vector dimension.
func defaultBindEmbeddingModel(tenantID string) (*models.EmbeddingModel, error) {
	modelSvc := service.NewModelProviderService()
	driver, modelName, apiConfig, maxTokens, err := modelSvc.GetTenantDefaultModelByType(tenantID, entity.ModelTypeEmbedding)
	if err != nil {
		return nil, fmt.Errorf("bind default embedding model: %w", err)
	}
	model := models.NewEmbeddingModel(driver, &modelName, apiConfig, maxTokens)
	return model, nil
}

// getEmbeddingDimension encodes a test string to determine vector dimension.
func getEmbeddingDimension(model *models.EmbeddingModel) (int, error) {
	embeds, err := model.ModelDriver.Embed(model.ModelName, []string{"ok"}, model.APIConfig, &models.EmbeddingConfig{Dimension: 0})
	if err != nil {
		return 0, fmt.Errorf("test encode failed: %w", err)
	}
	if len(embeds) == 0 {
		return 0, fmt.Errorf("test encode returned no embeddings")
	}
	return len(embeds[0].Embedding), nil
}
