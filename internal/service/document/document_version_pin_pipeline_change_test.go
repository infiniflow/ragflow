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
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/service"
)

// TestUpdateDatasetDocument_PipelineChangeWithoutVersionRejected verifies the C5/G3
// contract: switching a document to a different dataflow canvas WITHOUT supplying a
// concrete pipeline_version_id is REJECTED (design section 3: empty version is no
// longer silently backfilled to the canvas latest). The UI must surface the red
// Missing badge and the "版本信息缺失" error instead.
func TestUpdateDatasetDocument_PipelineChangeWithoutVersionRejected(t *testing.T) {
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
	_, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1",
		&UpdateDatasetDocumentRequest{ParseType: &parseType, PipelineID: sptr(canvasB)},
		map[string]bool{"pipeline_id": true})
	if err == nil || code == common.CodeSuccess {
		t.Fatalf("expected rejection of pipeline switch without version, got code=%d err=%v", code, err)
	}
	if !errors.Is(err, service.ErrPipelineVersionEmpty) {
		t.Fatalf("err = %v, want ErrPipelineVersionEmpty", err)
	}
	// The document must be left unchanged (pipeline_id and pin preserved).
	got := loadDoc(t, db, "doc-1")
	if got.PipelineID == nil || *got.PipelineID != canvasA {
		t.Fatalf("pipeline_id = %v, want unchanged %s", got.PipelineID, canvasA)
	}
	if got.PipelineVersionID == nil || *got.PipelineVersionID != "v-a" {
		t.Fatalf("PipelineVersionID = %v, want preserved v-a", got.PipelineVersionID)
	}
}
