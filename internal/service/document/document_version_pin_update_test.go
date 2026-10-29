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
	"ragflow/internal/entity"
	"ragflow/internal/service"

	"gorm.io/gorm"
)

func seedDataflowCanvasCat(t *testing.T, db *gorm.DB, canvasID string) {
	t.Helper()
	insertTestCanvas(t, canvasID, "user-1")
	if err := db.Model(&entity.UserCanvas{}).Where("id = ?", canvasID).
		Update("canvas_category", "dataflow_canvas").Error; err != nil {
		t.Fatal(err)
	}
}

func seedDocVersion(t *testing.T, db *gorm.DB, canvasID, versionID string, createMs int64) {
	t.Helper()
	if err := db.AutoMigrate(&entity.UserCanvasVersion{}); err != nil {
		t.Fatal(err)
	}
	ct := createMs
	dsl := entity.JSONMap{"components": map[string]any{
		"Parser:Shared": map[string]any{"obj": map[string]any{
			"component_name": "Parser",
			"params":         map[string]any{"source": versionID, "k": "x"},
		}},
	}}
	if err := db.Create(&entity.UserCanvasVersion{
		ID:           versionID,
		UserCanvasID: canvasID,
		DSL:          dsl,
		BaseModel:    entity.BaseModel{CreateTime: &ct},
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func bindDocCanvas(t *testing.T, db *gorm.DB, kbID, docID, canvasID, versionID string) {
	t.Helper()
	upd := map[string]any{"parser_id": "", "pipeline_id": canvasID}
	if versionID != "" {
		upd["pipeline_version_id"] = versionID
	}
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", kbID).Updates(upd).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&entity.Document{}).Where("id = ?", docID).Updates(upd).Error; err != nil {
		t.Fatal(err)
	}
}

func loadDoc(t *testing.T, db *gorm.DB, docID string) entity.Document {
	t.Helper()
	var got entity.Document
	if err := db.Where("id = ?", docID).First(&got).Error; err != nil {
		t.Fatalf("reload doc %s: %v", docID, err)
	}
	return got
}

func TestUpdateDatasetDocument_PersistsPinnedPipelineVersion(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 10, 5)

	canvasID := "1234567890abcdef1234567890abcdef"
	seedDataflowCanvasCat(t, db, canvasID)
	seedDocVersion(t, db, canvasID, "v-old", 100)
	seedDocVersion(t, db, canvasID, "v-new", 200)
	bindDocCanvas(t, db, "kb-1", "doc-1", canvasID, "v-old")

	_, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1",
		&UpdateDatasetDocumentRequest{PipelineVersionID: sptr("v-new")},
		map[string]bool{"pipeline_version_id": true})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateDatasetDocument err=%v code=%d", err, code)
	}
	got := loadDoc(t, db, "doc-1")
	if got.PipelineVersionID == nil || *got.PipelineVersionID != "v-new" {
		t.Fatalf("PipelineVersionID = %v, want v-new", got.PipelineVersionID)
	}
}

func TestUpdateDatasetDocument_RejectsForeignPipelineVersion(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 10, 5)

	canvasA := "1234567890abcdef1234567890abcdef"
	canvasB := "abcdefabcdefabcdefabcdefabcdefabcd"
	seedDataflowCanvasCat(t, db, canvasA)
	seedDataflowCanvasCat(t, db, canvasB)
	seedDocVersion(t, db, canvasA, "v-a", 100)
	seedDocVersion(t, db, canvasB, "v-foreign", 100)
	bindDocCanvas(t, db, "kb-1", "doc-1", canvasA, "v-a")

	_, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1",
		&UpdateDatasetDocumentRequest{PipelineVersionID: sptr("v-foreign")},
		map[string]bool{"pipeline_version_id": true})
	if err == nil {
		t.Fatal("expected rejection of foreign pipeline version")
	}
	if code != common.CodeDataError {
		t.Fatalf("code = %d, want CodeDataError", code)
	}
	if !errors.Is(err, service.ErrPipelineVersionMissing) {
		t.Fatalf("err = %v, want ErrPipelineVersionMissing", err)
	}
	if got := loadDoc(t, db, "doc-1"); got.PipelineVersionID == nil || *got.PipelineVersionID != "v-a" {
		t.Fatalf("PipelineVersionID changed on rejected save: %v", got.PipelineVersionID)
	}
}

func TestUpdateDatasetDocument_RejectsMissingPipelineVersion(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 10, 5)

	canvasID := "1234567890abcdef1234567890abcdef"
	seedDataflowCanvasCat(t, db, canvasID)
	seedDocVersion(t, db, canvasID, "v-a", 100)
	bindDocCanvas(t, db, "kb-1", "doc-1", canvasID, "v-a")

	_, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1",
		&UpdateDatasetDocumentRequest{PipelineVersionID: sptr("does-not-exist")},
		map[string]bool{"pipeline_version_id": true})
	if err == nil || code != common.CodeDataError {
		t.Fatalf("want CodeDataError, got code=%d err=%v", code, err)
	}
	if !errors.Is(err, service.ErrPipelineVersionMissing) {
		t.Fatalf("err = %v, want ErrPipelineVersionMissing", err)
	}
}

func TestUpdateDatasetDocument_PreservesPipelineVersionWhenAbsent(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 10, 5)

	canvasID := "1234567890abcdef1234567890abcdef"
	seedDataflowCanvasCat(t, db, canvasID)
	seedDocVersion(t, db, canvasID, "v-old", 100)
	bindDocCanvas(t, db, "kb-1", "doc-1", canvasID, "v-old")

	_, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1",
		&UpdateDatasetDocumentRequest{}, map[string]bool{})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateDatasetDocument err=%v code=%d", err, code)
	}
	if got := loadDoc(t, db, "doc-1"); got.PipelineVersionID == nil || *got.PipelineVersionID != "v-old" {
		t.Fatalf("PipelineVersionID = %v, want preserved v-old", got.PipelineVersionID)
	}
}

func TestUpdateDatasetDocument_EmptyPipelineVersionTracksLatest(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 10, 5)

	canvasID := "1234567890abcdef1234567890abcdef"
	seedDataflowCanvasCat(t, db, canvasID)
	seedDocVersion(t, db, canvasID, "v-old", 100)
	bindDocCanvas(t, db, "kb-1", "doc-1", canvasID, "v-old")

	// C5/G3: an explicit EMPTY version in Pipeline mode is REJECTED (design
	// section 3: empty version is no longer auto-backfilled to latest). The pin
	// must be preserved and the save must fail with ErrPipelineVersionEmpty.
	_, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1",
		&UpdateDatasetDocumentRequest{PipelineVersionID: sptr("")},
		map[string]bool{"pipeline_version_id": true})
	if err == nil || code == common.CodeSuccess {
		t.Fatalf("expected rejection of empty pipeline version, got code=%d err=%v", code, err)
	}
	if !errors.Is(err, service.ErrPipelineVersionEmpty) {
		t.Fatalf("err = %v, want ErrPipelineVersionEmpty", err)
	}
	if got := loadDoc(t, db, "doc-1"); got.PipelineVersionID == nil || *got.PipelineVersionID != "v-old" {
		t.Fatalf("pin must be preserved on rejected save, got %v", got.PipelineVersionID)
	}
}

func TestUpdateDatasetDocument_BuiltinIgnoresPipelineVersion(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 10, 5)

	_, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1",
		&UpdateDatasetDocumentRequest{PipelineVersionID: sptr("v-whatever")},
		map[string]bool{"pipeline_version_id": true})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateDatasetDocument err=%v code=%d", err, code)
	}
	if got := loadDoc(t, db, "doc-1"); got.PipelineVersionID != nil {
		t.Fatalf("builtin document must not carry pipeline_version_id, got %v", *got.PipelineVersionID)
	}
}

func TestUpdateDatasetDocument_CleansParserConfigWithNewVersion(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 10, 5)

	canvasID := "1234567890abcdef1234567890abcdef"
	seedDataflowCanvasCat(t, db, canvasID)
	seedDocVersion(t, db, canvasID, "v-old", 100)
	seedDocVersion(t, db, canvasID, "v-new", 200)
	bindDocCanvas(t, db, "kb-1", "doc-1", canvasID, "v-old")

	_, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1",
		&UpdateDatasetDocumentRequest{
			PipelineVersionID: sptr("v-new"),
			ParserConfig:      map[string]any{"Parser:Shared": map[string]any{"k": "override"}},
		},
		map[string]bool{"pipeline_version_id": true, "parser_config": true})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateDatasetDocument err=%v code=%d", err, code)
	}
	got := loadDoc(t, db, "doc-1")
	if got.PipelineVersionID == nil || *got.PipelineVersionID != "v-new" {
		t.Fatalf("PipelineVersionID = %v, want v-new", got.PipelineVersionID)
	}
	comp, ok := got.ParserConfig["Parser:Shared"].(map[string]any)
	if !ok {
		t.Fatalf("cleaned parser_config missing Parser:Shared: %#v", got.ParserConfig)
	}
	if comp["source"] != "v-new" {
		t.Fatalf("parser_config cleaned from version %v, want v-new (new pin)", comp["source"])
	}
}

// T26/C3 (document): switching the pinned version of a pipeline document WITHOUT
// supplying parser_config resets the document's component params to the new
// version default (design section 5 + T26/A3 整体重置). The previous config must
// not be merged in.
func TestUpdateDatasetDocument_VersionSwitchResetsParserConfigToNewDefault(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 10, 5)

	canvasID := "1234567890abcdef1234567890abcdef"
	seedDataflowCanvasCat(t, db, canvasID)
	seedDocVersion(t, db, canvasID, "v-old", 100)
	seedDocVersion(t, db, canvasID, "v-new", 200)
	bindDocCanvas(t, db, "kb-1", "doc-1", canvasID, "v-old")

	// A distinctive parser_config that must NOT survive the version switch.
	if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").
		Update("parser_config", entity.JSONMap{"__c3_marker__": "OLD"}).Error; err != nil {
		t.Fatalf("seed parser_config: %v", err)
	}

	_, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1",
		&UpdateDatasetDocumentRequest{PipelineVersionID: sptr("v-new")},
		map[string]bool{"pipeline_version_id": true})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateDatasetDocument err=%v code=%d", err, code)
	}
	got := loadDoc(t, db, "doc-1")
	if got.PipelineVersionID == nil || *got.PipelineVersionID != "v-new" {
		t.Fatalf("PipelineVersionID = %v, want v-new", got.PipelineVersionID)
	}
	// C3 full reset: the pre-switch marker must be gone (not merged).
	if v, ok := got.ParserConfig["__c3_marker__"]; ok && v == "OLD" {
		t.Fatalf("parser_config still holds pre-switch marker; C3 reset did not run: %v", got.ParserConfig)
	}
	// The reset must load the NEW version's default (source=v-new), not v-old.
	comp, ok := got.ParserConfig["Parser:Shared"].(map[string]any)
	if !ok {
		t.Fatalf("reset parser_config missing Parser:Shared: %#v", got.ParserConfig)
	}
	if comp["source"] != "v-new" {
		t.Fatalf("parser_config reset to version %v, want v-new default", comp["source"])
	}
}

// T26 corner (document): re-submitting the SAME pinned version must NOT reset the
// document's parser_config (C3 fires only on an actual version change).
func TestUpdateDatasetDocument_SameVersionKeepsParserConfig(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 10, 5)

	canvasID := "1234567890abcdef1234567890abcdef"
	seedDataflowCanvasCat(t, db, canvasID)
	seedDocVersion(t, db, canvasID, "v-old", 100)
	bindDocCanvas(t, db, "kb-1", "doc-1", canvasID, "v-old")

	if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").
		Update("parser_config", entity.JSONMap{"__c3_marker__": "OLD"}).Error; err != nil {
		t.Fatalf("seed parser_config: %v", err)
	}

	_, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1",
		&UpdateDatasetDocumentRequest{PipelineVersionID: sptr("v-old")},
		map[string]bool{"pipeline_version_id": true})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateDatasetDocument err=%v code=%d", err, code)
	}
	got := loadDoc(t, db, "doc-1")
	if v, ok := got.ParserConfig["__c3_marker__"]; !ok || v != "OLD" {
		t.Fatalf("parser_config should be preserved when version unchanged: %v", got.ParserConfig)
	}
}
