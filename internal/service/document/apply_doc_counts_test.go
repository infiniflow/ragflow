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
	"testing"

	"ragflow/internal/entity"
)

// TestApplyDocCounts_ReparseCarriesDelta checks that re-applying a document with
// a different count - a re-parse - moves the knowledge base aggregate by the
// delta to the new absolute value, rather than summing the two runs.
func TestApplyDocCounts_ReparseCarriesDelta(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.Create(&entity.Knowledgebase{ID: "kb-1"}).Error; err != nil {
		t.Fatalf("create kb: %v", err)
	}
	if err := db.Create(&entity.Document{ID: "doc-1", KbID: "kb-1", ParserConfig: entity.JSONMap{}}).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}
	svc := testDocumentService(t)
	ctx := t.Context()

	// First parse produces 5 chunks / 100 tokens; a re-parse produces 7 / 140.
	if err := svc.ApplyDocCounts(ctx, "doc-1", "kb-1", 5, 100, 1); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := svc.ApplyDocCounts(ctx, "doc-1", "kb-1", 7, 140, 1); err != nil {
		t.Fatalf("reparse apply: %v", err)
	}

	// The document holds the new absolute counts; the KB aggregate follows the
	// delta to 7/140, not 5+7 / 100+140.
	var kb entity.Knowledgebase
	if err := db.First(&kb, "id = ?", "kb-1").Error; err != nil {
		t.Fatalf("load kb: %v", err)
	}
	var doc entity.Document
	if err := db.First(&doc, "id = ?", "doc-1").Error; err != nil {
		t.Fatalf("load doc: %v", err)
	}
	if kb.ChunkNum != 7 || kb.TokenNum != 140 {
		t.Errorf("kb = (chunk %d, token %d), want (7, 140)", kb.ChunkNum, kb.TokenNum)
	}
	if doc.ChunkNum != 7 || doc.TokenNum != 140 {
		t.Errorf("doc = (chunk %d, token %d), want (7, 140)", doc.ChunkNum, doc.TokenNum)
	}
}

// TestApplyDocCounts_KBAggregateClampsAtZero checks that an aggregate driven
// below zero from an inconsistent starting state clamps to 0 instead of
// underflowing.
func TestApplyDocCounts_KBAggregateClampsAtZero(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	// The KB aggregate is inconsistent: below this document's own contribution.
	if err := db.Create(&entity.Knowledgebase{ID: "kb-1", ChunkNum: 2, TokenNum: 40}).Error; err != nil {
		t.Fatalf("create kb: %v", err)
	}
	if err := db.Create(&entity.Document{ID: "doc-1", KbID: "kb-1", ChunkNum: 5, TokenNum: 100, ParserConfig: entity.JSONMap{}}).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}
	svc := testDocumentService(t)

	// A run that clears the document (0 chunks) drives the aggregate delta to -5;
	// 2 - 5 is negative and must clamp to 0 rather than underflow.
	if err := svc.ApplyDocCounts(t.Context(), "doc-1", "kb-1", 0, 0, 1); err != nil {
		t.Fatalf("apply: %v", err)
	}
	var kb entity.Knowledgebase
	if err := db.First(&kb, "id = ?", "kb-1").Error; err != nil {
		t.Fatalf("load kb: %v", err)
	}
	if kb.ChunkNum != 0 || kb.TokenNum != 0 {
		t.Errorf("kb = (chunk %d, token %d), want (0, 0) - aggregate must clamp, not underflow", kb.ChunkNum, kb.TokenNum)
	}
}

// TestApplyDocCounts_ProcessDurationIsAbsolute checks that process_duration is
// the last run's value, not a cumulative sum: a later run replaces the prior
// value rather than adding to it.
func TestApplyDocCounts_ProcessDurationIsAbsolute(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.Create(&entity.Knowledgebase{ID: "kb-1"}).Error; err != nil {
		t.Fatalf("create kb: %v", err)
	}
	if err := db.Create(&entity.Document{ID: "doc-1", KbID: "kb-1", ParserConfig: entity.JSONMap{}}).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}
	svc := testDocumentService(t)
	ctx := t.Context()

	// A run sets process_duration to its own value; a later run replaces it rather
	// than accumulating, so the stored value is the last run's, not the 3.5+1.25 sum.
	if err := svc.ApplyDocCounts(ctx, "doc-1", "kb-1", 5, 100, 3.5); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := svc.ApplyDocCounts(ctx, "doc-1", "kb-1", 5, 100, 1.25); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	var doc entity.Document
	if err := db.First(&doc, "id = ?", "doc-1").Error; err != nil {
		t.Fatalf("load doc: %v", err)
	}
	if doc.ProcessDuration != 1.25 {
		t.Errorf("process_duration = %v, want 1.25 (last run's value, not the 4.75 sum)", doc.ProcessDuration)
	}
}
