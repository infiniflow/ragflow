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

package document

import (
	"context"
	"errors"
	"fmt"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// ErrRerunDocumentNotFound mirrors the Python rerun_agent contract
// (api/apps/restful_apis/agent_api.py): an unknown log id, a missing
// document/dataset, and an access denial all collapse to the same
// "Document not found." message so a caller cannot probe whether a
// document exists in another tenant.
var ErrRerunDocumentNotFound = errors.New("Document not found.")

// RerunDataflow re-runs the ingestion pipeline a pipeline operation log
// belongs to. It backs POST /api/v1/agents/rerun, whose id is the
// ingestion LOG id (Python resolves it via
// PipelineOperationLogService.get_documents_info before gating access).
//
// Semantics follow the Python endpoint adapted to the Go ingestion
// machinery:
//   - resolve log -> document, then enforce DocumentService accessibility;
//   - refuse while the document is mid-run (0 < progress < 1);
//   - persist the front-end's edited DSL with dsl.path = [component_id]
//     on the log row (Python: PipelineOperationLogService.update_by_id);
//   - clear the prior parse results (chunks, counters, terminal tasks)
//     and enqueue a fresh ingestion run (StartParseDocuments with
//     RerunWithDelete), which records a new pipeline operation log the
//     front-end can track.
//
// The Go pipeline has no partial-resume entry point (execution always
// starts at the graph entry; see pipeline.Pipeline.Run), so the rerun
// re-executes the whole pipeline instead of resuming from component_id.
func (s *DocumentService) RerunDataflow(ctx context.Context, userID, logID string, dsl map[string]interface{}, componentID string) error {
	opLog, err := s.pipelineLogDAO.GetByID(ctx, dao.DB, logID)
	if err != nil || opLog == nil {
		return ErrRerunDocumentNotFound
	}
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, opLog.DocumentID)
	if err != nil || doc == nil {
		return ErrRerunDocumentNotFound
	}
	kb, err := s.kbDAO.GetByID(ctx, dao.DB, doc.KbID)
	if err != nil || kb == nil {
		return ErrRerunDocumentNotFound
	}
	if !s.Accessible(ctx, doc.ID, userID) {
		return ErrRerunDocumentNotFound
	}
	if doc.Progress > 0 && doc.Progress < 1 {
		name := opLog.DocumentName
		if doc.Name != nil && *doc.Name != "" {
			name = *doc.Name
		}
		return fmt.Errorf("`%s` is processing...", name)
	}

	if len(dsl) > 0 {
		dsl["path"] = []interface{}{componentID}
		if err := s.pipelineLogDAO.UpdateDSL(ctx, dao.DB, logID, entity.JSONMap(dsl)); err != nil {
			return fmt.Errorf("update pipeline log dsl: %w", err)
		}
	}

	return s.StartParseDocuments(ctx, doc, kb, userID, StartParseOptions{RerunWithDelete: true})
}
