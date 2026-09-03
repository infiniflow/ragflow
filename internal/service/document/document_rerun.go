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

	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// ErrRerunDocumentNotFound is the rerun endpoint's only "unknown or denied"
// answer: an unknown log id, a missing document/dataset, and an access
// denial all collapse to the same message so a caller cannot probe whether
// a document exists in another tenant.
var ErrRerunDocumentNotFound = errors.New("Document not found.")

// RerunDocumentProcessingError reports a rerun attempt while the document
// is mid-run (0 < progress < 1). The handler maps it to the data-error
// envelope, so the message is written for the caller, not the server log.
type RerunDocumentProcessingError struct {
	DocumentName string
}

func (e *RerunDocumentProcessingError) Error() string {
	return fmt.Sprintf("`%s` is processing...", e.DocumentName)
}

// RerunDocument re-runs the ingestion pipeline a pipeline operation log
// belongs to. It backs POST /api/v1/agents/rerun, whose id is the
// ingestion LOG id, resolved to its document before the accessibility
// gate.
//
// Steps: resolve log -> document and enforce DocumentService
// accessibility; refuse while the document is mid-run (0 < progress < 1);
// persist the front-end's edited DSL with dsl.path = [component_id] on
// the log row; clear the prior parse results (chunks, counters, terminal
// tasks) and enqueue a fresh ingestion run (StartParseDocuments with
// RerunWithDelete), which records a new pipeline operation log the
// front-end can track.
//
// The Go pipeline has no partial-resume entry point (execution always
// starts at the graph entry; see pipeline.Pipeline.Run), so the rerun
// re-executes the whole pipeline instead of resuming from component_id.
func (s *DocumentService) RerunDocument(ctx context.Context, userID, logID string, dsl map[string]interface{}, componentID string) error {
	// A missing row is the caller-facing not-found; any other DB error is
	// an internal failure and must not be collapsed into it.
	opLog, err := s.pipelineLogDAO.GetByID(ctx, dao.DB, logID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrRerunDocumentNotFound
	}
	if err != nil {
		return fmt.Errorf("get pipeline operation log %s: %w", logID, err)
	}
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, opLog.DocumentID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrRerunDocumentNotFound
	}
	if err != nil {
		return fmt.Errorf("get document %s: %w", opLog.DocumentID, err)
	}
	kb, err := s.kbDAO.GetByID(ctx, dao.DB, doc.KbID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrRerunDocumentNotFound
	}
	if err != nil {
		return fmt.Errorf("get knowledgebase %s: %w", doc.KbID, err)
	}
	if !s.Accessible(ctx, doc.ID, userID) {
		return ErrRerunDocumentNotFound
	}
	if doc.Progress > 0 && doc.Progress < 1 {
		name := opLog.DocumentName
		if doc.Name != nil && *doc.Name != "" {
			name = *doc.Name
		}
		return &RerunDocumentProcessingError{DocumentName: name}
	}

	if dsl != nil {
		// Persist for any non-nil dsl, including an empty map. Copy
		// before writing so the caller's DSL map is not mutated in
		// place.
		persisted := make(map[string]interface{}, len(dsl)+1)
		for k, v := range dsl {
			persisted[k] = v
		}
		persisted["path"] = []interface{}{componentID}
		if err := s.pipelineLogDAO.UpdateDSL(ctx, dao.DB, logID, entity.JSONMap(persisted)); err != nil {
			return fmt.Errorf("update pipeline log dsl: %w", err)
		}
		dsl = persisted
	}

	return s.StartParseDocuments(ctx, doc, kb, userID, StartParseOptions{
		RerunWithDelete:  true,
		RerunDSL:         entity.JSONMap(dsl),
		RerunLogID:       logID,
		RerunComponentID: componentID,
	})
}
