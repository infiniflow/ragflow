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
	"ragflow/internal/entity"
)

// TestUpdateDatasetDocumentUsesPinnedPipelineVersionForParserConfig proves the
// parser_config cleaning path honors the document's sticky pinned
// pipeline_version_id (design section 5/B4) instead of silently loading the
// latest version. The document is pinned to v-pinned, whose DSL exposes a
// "source" param the request override does NOT touch, so the cleaned config's
// "source" value reveals which version's DSL was actually loaded:
//   - fixed (pinned):        source == "pinned"
//   - buggy loads latest:    source == "latest"
//   - buggy fallback (error): source absent
//
// Only the fixed behavior satisfies the assertion, so this fails before the fix
// and passes after (review finding: document_dataset_update.go ignored the
// pinned version when reloading DSL).
func TestUpdateDatasetDocumentUsesPinnedPipelineVersionForParserConfig(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.UserCanvasVersion{}); err != nil {
		t.Fatal(err)
	}
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 10, 5)

	canvasID := "1234567890abcdef1234567890abcdef"
	insertTestCanvas(t, canvasID, "user-1")
	// The dataflow read path is only taken for dataflow_canvas; insertTestCanvas
	// leaves the category at the agent default, so set it explicitly.
	if err := db.Model(&entity.UserCanvas{}).Where("id = ?", canvasID).Update("canvas_category", "dataflow_canvas").Error; err != nil {
		t.Fatal(err)
	}

	// Same component id, distinct "source" param per version. The request
	// override only sets "k", so "source" is carried verbatim from the DSL.
	pinnedDSL := entity.JSONMap{"components": map[string]any{
		"Parser:Shared": map[string]any{"obj": map[string]any{
			"component_name": "Parser",
			"params":         map[string]any{"source": "pinned", "k": "x"},
		}},
	}}
	latestDSL := entity.JSONMap{"components": map[string]any{
		"Parser:Shared": map[string]any{"obj": map[string]any{
			"component_name": "Parser",
			"params":         map[string]any{"source": "latest", "k": "x"},
		}},
	}}
	pinnedTime := int64(100)
	latestTime := int64(200)
	if err := db.Create(&entity.UserCanvasVersion{
		ID:           "v-pinned",
		UserCanvasID: canvasID,
		DSL:          pinnedDSL,
		BaseModel:    entity.BaseModel{CreateTime: &pinnedTime},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.UserCanvasVersion{
		ID:           "v-latest",
		UserCanvasID: canvasID,
		DSL:          latestDSL,
		BaseModel:    entity.BaseModel{CreateTime: &latestTime},
	}).Error; err != nil {
		t.Fatal(err)
	}

	// Bind KB + document to the canvas and the pinned (older) version.
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").
		Updates(map[string]any{"parser_id": "", "pipeline_id": canvasID, "pipeline_version_id": "v-pinned"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").
		Updates(map[string]any{"parser_id": "", "pipeline_id": canvasID, "pipeline_version_id": "v-pinned"}).Error; err != nil {
		t.Fatal(err)
	}

	svc := testDocumentService(t)
	_, code, err := svc.UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1", &UpdateDatasetDocumentRequest{
		ParserConfig: map[string]any{"Parser:Shared": map[string]any{"k": "override_val"}},
	}, map[string]bool{"parser_config": true})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateDatasetDocument err=%v code=%d", err, code)
	}

	var got entity.Document
	if err := db.Where("id = ?", "doc-1").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	comp, ok := got.ParserConfig["Parser:Shared"].(map[string]any)
	if !ok {
		t.Fatalf("cleaned parser_config missing Parser:Shared component; got: %#v", got.ParserConfig)
	}
	if comp["source"] != "pinned" {
		t.Fatalf("parser_config cleaned from wrong version: source=%v, want pinned; full=%#v", comp["source"], got.ParserConfig)
	}
}
