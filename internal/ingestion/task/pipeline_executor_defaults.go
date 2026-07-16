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
	"encoding/json"
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
)

func (s *PipelineExecutor) defaultLoadDSL(ctx context.Context, canvasID string) (string, string, error) {
	if s == nil || s.taskCtx == nil {
		return "", "", fmt.Errorf("pipeline executor: nil task context")
	}
	if canvasID == "" {
		return "", "", fmt.Errorf("pipeline executor: empty canvas id")
	}
	canvas, err := dao.NewUserCanvasDAO().GetByID(canvasID)
	if err != nil {
		return "", "", fmt.Errorf("load canvas %s: %w", canvasID, err)
	}

	canvasTitle := ""
	if canvas.Title != nil {
		canvasTitle = *canvas.Title
	}
	common.Info(fmt.Sprintf("load canvas %s, name %s", canvasID, canvasTitle))

	raw, err := json.Marshal(canvas.DSL)
	if err != nil {
		return "", "", fmt.Errorf("marshal canvas dsl %s: %w", canvasID, err)
	}
	return string(raw), canvasID, nil
}

func (s *PipelineExecutor) defaultRunPipeline(ctx context.Context, dsl string) (map[string]any, string, error) {
	if s == nil || s.taskCtx == nil {
		return nil, dsl, fmt.Errorf("pipeline executor: nil task context")
	}

	// Use doc ID as pipeline ID if available, otherwise a placeholder
	pipelineID := "pipeline_" + s.taskCtx.Doc.ID
	if s.taskCtx.IngestionTask != nil && s.taskCtx.IngestionTask.ID != "" {
		pipelineID = s.taskCtx.IngestionTask.ID
	}
	opts := []pipelinepkg.PipelineOption{
		pipelinepkg.WithProgressSink(s.progressSink),
		pipelinepkg.WithDocumentID(s.taskCtx.Doc.ID),
	}
	if s.requireResume {
		opts = append(opts, pipelinepkg.WithRequireResume())
	}
	pipe, err := pipelinepkg.NewPipelineFromDSL([]byte(dsl), pipelineID, opts...)
	if err != nil {
		return nil, dsl, fmt.Errorf("compile pipeline dsl: %w", err)
	}
	inputs := map[string]any{}
	if s.taskCtx.Doc.ID != "" {
		inputs["doc_id"] = s.taskCtx.Doc.ID
	}
	if s.taskCtx.File != nil {
		inputs["file"] = s.taskCtx.File
	}
	inputs["tenant_id"] = s.taskCtx.Tenant.ID
	inputs["kb_id"] = s.taskCtx.KB.ID

	output, err := pipe.Run(ctx, inputs)
	if err != nil {
		return nil, dsl, err
	}
	payload, err := pipelinepkg.ExtractPayload(dsl, output)
	if err != nil {
		return nil, dsl, err
	}
	return payload, dsl, nil
}
