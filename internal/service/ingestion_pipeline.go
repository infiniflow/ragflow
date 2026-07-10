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
	"encoding/json"
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// buildIngestionPipelineDSL returns the ingestion pipeline DSL bytes
// for a document.
//
// The document is always associated with an ingestion pipeline
// template: the customer creates the pipeline from a template and the
// chosen template id is stored on doc.PipelineID. The DSL is resolved
// from the user_canvas or canvas_template table by that id
// (resolvePipelineDSL). When no pipeline is set (or the selected one is
// missing), it falls back to the built-in General template so ingestion
// still has a valid pipeline (loadGeneralTemplateDSL).
//
// The returned bytes are suitable for task.Schema["pipeline"] and
// will be accepted by defaultPipelineDSL → pipeline.NewPipelineFromDSL.
func (s *DocumentService) buildIngestionPipelineDSL(doc *entity.Document) ([]byte, error) {
	if doc.PipelineID != nil && *doc.PipelineID != "" {
		dsl, err := s.resolvePipelineDSL(*doc.PipelineID)
		if err == nil && dsl != nil {
			return dsl, nil
		}
		common.Info(fmt.Sprintf("buildIngestionPipelineDSL: pipeline_id=%s not found, falling back to general template", *doc.PipelineID))
	}

	return s.loadGeneralTemplateDSL()
}

// resolvePipelineDSL fetches the pipeline DSL from the user_canvas
// or canvas_template table for a given pipelineID.
func (s *DocumentService) resolvePipelineDSL(pipelineID string) ([]byte, error) {
	// Try user_canvas first (user-created pipelines).
	if canvas, err := s.canvasDAO.GetByID(pipelineID); err == nil && canvas != nil {
		if dslBytes, err := json.Marshal(canvas.DSL); err == nil && len(dslBytes) > 0 {
			return dslBytes, nil
		}
	}
	// Fallback to canvas_template (system templates).
	if template, err := s.canvasTemplateDAO.GetByID(pipelineID); err == nil && template != nil {
		if dslBytes, err := json.Marshal(template.DSL); err == nil && len(dslBytes) > 0 {
			return dslBytes, nil
		}
	}
	return nil, nil
}

// loadGeneralTemplateDSL returns the built-in General ingestion pipeline
// template DSL. It is the safety-net fallback used when a document has no
// explicit pipeline_id (or the selected one is missing), so ingestion
// always has a valid pipeline to run.
//
// The General template is identified among the seeded dataflow_canvas
// templates by its English title "General".
func (s *DocumentService) loadGeneralTemplateDSL() ([]byte, error) {
	templates, err := s.canvasTemplateDAO.GetAll()
	if err != nil {
		return nil, fmt.Errorf("list canvas templates: %w", err)
	}
	for _, t := range templates {
		if t.DSL == nil {
			continue
		}
		if title, ok := t.Title["en"].(string); ok && title == "General" {
			return json.Marshal(t.DSL)
		}
	}
	return nil, fmt.Errorf("general ingestion template not found")
}
