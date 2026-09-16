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

package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/common"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
)

// builtinPipelineRegistry is the registry surface PipelineHandler needs.
// Defined as an interface so handler tests can inject a fake without
// touching the embedded template FS.
type builtinPipelineRegistry interface {
	List() *pipelinepkg.BuiltinPipelineListResponse
	Get(ref string) (*pipelinepkg.BuiltinPipeline, bool)
}

// PipelineHandler handles pipeline endpoints.
// Pipelines are the top-level resource for ingestion pipeline templates.
// Built-in templates are shipped with the binary; user-defined templates
// (canvas DSL) may be added later.
type PipelineHandler struct {
	registry builtinPipelineRegistry
}

// NewPipelineHandler builds a PipelineHandler backed by the embedded
// default registry. If the registry fails to load (malformed template),
// the handler is returned with a nil registry and ListPipelines serves
// an empty list rather than crashing the route.
func NewPipelineHandler() *PipelineHandler {
	registry, err := pipelinepkg.DefaultRegistry()
	if err != nil {
		common.Warn("failed to load builtin pipeline registry", zap.Error(err))
		return &PipelineHandler{}
	}
	return &PipelineHandler{registry: registry}
}

// ListPipelines GET /api/v1/pipelines
// Returns available pipeline templates. When type=builtin (or when no
// type is specified), returns the built-in pipeline catalog shipped with
// the binary. Support for user-defined pipelines may be added later.
// This is public static data, so no auth is required.
func (h *PipelineHandler) ListPipelines(c *gin.Context) {
	if h == nil || h.registry == nil {
		common.SuccessWithData(c, &pipelinepkg.BuiltinPipelineListResponse{
			BuiltinPipelines: []*pipelinepkg.BuiltinPipelineMeta{},
			Total:            0,
		}, "success")
		return
	}

	pipelineType := c.Query("type")
	// For now only builtin pipelines exist; user-defined may be added later.
	// When type is empty or "builtin", return the builtin catalog.
	_ = pipelineType

	common.SuccessWithData(c, h.registry.List(), "success")
}

// GetPipeline GET /api/v1/pipelines/:id
// Returns a single built-in pipeline template with its DSL.
// This is public static data, so no auth is required.
func (h *PipelineHandler) GetPipeline(c *gin.Context) {
	if h == nil || h.registry == nil {
		common.ErrorWithCode(c, common.CodeDataError, "pipeline not found")
		return
	}

	id := c.Param("id")
	tpl, ok := h.registry.Get(id)
	if !ok || tpl == nil {
		common.ErrorWithCode(c, common.CodeDataError, "pipeline not found")
		return
	}

	common.SuccessWithData(c, map[string]any{"dsl": tpl.DSL}, "success")
}
