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
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// TestUpdateDatasetDocumentListReturnsPinnedVersion reproduces the file-level
// bug: a document pinned to a specific canvas version must be reported as pinned
// when the document list is read back, so the reopened change-parser dialog can
// show the saved pin instead of re-adopting the latest version. If the list
// projection drops pipeline_version_id (as DocumentListItem originally did), the
// returned item's PipelineVersionID is nil and this fails.
func TestUpdateDatasetDocumentListReturnsPinnedVersion(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.UserCanvasVersion{}); err != nil {
		t.Fatal(err)
	}
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 10, 5)

	// The document list JOINs file2document/file, so seed those rows too.
	if err := db.Create(&entity.File{
		ID: "file-1", TenantID: "tenant-1", CreatedBy: "user-1",
		Name: "doc.txt", Type: "doc", SourceType: "local",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.File2Document{
		ID: "f2d-1", FileID: sptr("file-1"), DocumentID: sptr("doc-1"),
	}).Error; err != nil {
		t.Fatal(err)
	}

	canvasID := "1234567890abcdef1234567890abcdef"
	insertTestCanvas(t, canvasID, "user-1")
	if err := db.Model(&entity.UserCanvas{}).Where("id = ?", canvasID).
		Update("canvas_category", "dataflow_canvas").Error; err != nil {
		t.Fatal(err)
	}

	vt := int64(100)
	if err := db.Create(&entity.UserCanvasVersion{
		ID:           "v-pinned",
		UserCanvasID: canvasID,
		DSL: entity.JSONMap{"components": map[string]any{
			"Parser:Shared": map[string]any{"obj": map[string]any{
				"component_name": "Parser",
				"params":         map[string]any{"source": "pinned", "k": "x"},
			}},
		}},
		BaseModel: entity.BaseModel{CreateTime: &vt},
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").
		Updates(map[string]any{"parser_id": "", "pipeline_id": canvasID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").
		Updates(map[string]any{"parser_id": "", "pipeline_id": canvasID}).Error; err != nil {
		t.Fatal(err)
	}

	svc := testDocumentService(t)
	pid := canvasID
	pv := "v-pinned"
	pt := 2
	_, code, err := svc.UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1", &UpdateDatasetDocumentRequest{
		ParseType:         &pt,
		PipelineID:        &pid,
		PipelineVersionID: &pv,
	}, map[string]bool{"pipeline_id": true, "pipeline_version_id": true, "parse_type": true})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateDatasetDocument err=%v code=%d", err, code)
	}

	opts := dao.DocumentListOptions{KbID: "kb-1", OrderBy: "create_time", Desc: true, Offset: 0, Limit: 10}
	docs, _, lerr := svc.ListDocumentsByDatasetIDWithOptions(documentTestContext(t.Context()), opts, 1, 10)
	if lerr != nil {
		t.Fatal(lerr)
	}
	if len(docs) == 0 {
		t.Fatal("list returned no documents")
	}
	if docs[0].PipelineVersionID == nil || *docs[0].PipelineVersionID != "v-pinned" {
		t.Fatalf("list did not return the pinned version; got %v (want v-pinned)", docs[0].PipelineVersionID)
	}
}
