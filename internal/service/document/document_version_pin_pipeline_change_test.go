// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package document

import (
	"testing"

	"ragflow/internal/common"
)

// TestUpdateDatasetDocument_PipelineChangeWithoutVersionResetsPin verifies the
// sticky-snapshot contract: when a save switches the document to a different
// dataflow canvas but does NOT supply pipeline_version_id, the previously pinned
// version (which belongs to the OLD canvas) must not survive the switch. Leaving
// it in place would bind the document to canvasB while pinning canvasA's version,
// so the runtime LoadPipelineDSL ownership check fails and parser_config cleaning
// silently falls back to a merge (design C2 / B4).
func TestUpdateDatasetDocument_PipelineChangeWithoutVersionResetsPin(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 10, 5)

	canvasA := "1234567890abcdef1234567890abcdef"
	canvasB := "abcdefabcdefabcdefabcdefabcdefabcd"
	seedDataflowCanvasCat(t, db, canvasA)
	seedDataflowCanvasCat(t, db, canvasB)
	seedDocVersion(t, db, canvasA, "v-a", 100)
	seedDocVersion(t, db, canvasB, "v-b", 100)
	bindDocCanvas(t, db, "kb-1", "doc-1", canvasA, "v-a")

	// Switch to canvasB, omitting pipeline_version_id entirely.
	parseType := 2 // Pipeline mode
	_, code, err := testDocumentService(t).UpdateDatasetDocument(t.Context(), "user-1", "kb-1", "doc-1",
		&UpdateDatasetDocumentRequest{ParseType: &parseType, PipelineID: sptr(canvasB)},
		map[string]bool{"pipeline_id": true})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateDatasetDocument err=%v code=%d", err, code)
	}
	got := loadDoc(t, db, "doc-1")
	if got.PipelineID == nil || *got.PipelineID != canvasB {
		t.Fatalf("pipeline_id = %v, want %s", got.PipelineID, canvasB)
	}
	// The surviving pin must no longer reference the old canvas's version.
	if got.PipelineVersionID != nil && *got.PipelineVersionID == "v-a" {
		t.Fatalf("pin still references old canvas version v-a after pipeline switch: %v", got.PipelineVersionID)
	}
	// It should either be cleared (track latest) or pinned to canvasB's latest (v-b).
	if got.PipelineVersionID != nil && *got.PipelineVersionID != "v-b" {
		t.Fatalf("pipeline_version_id = %v, want v-b or nil", got.PipelineVersionID)
	}
}
