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

package dataset

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"

	"gorm.io/gorm"
)

// seedGapCanvas creates a dataflow canvas once. Callers seed versions separately.
func seedGapCanvas(t *testing.T, db *gorm.DB, canvasID string) {
	t.Helper()
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("migrate canvas/version: %v", err)
	}
	canvas := &entity.UserCanvas{
		ID:             canvasID,
		UserID:         "tenant-1",
		CanvasCategory: "dataflow_canvas",
		Permission:     "me",
		DSL:            entity.JSONMap{"graph": "legacy"},
	}
	if err := db.Create(canvas).Error; err != nil {
		t.Fatalf("seed canvas: %v", err)
	}
}

// seedGapVersion inserts a version whose DSL carries a distinctive component
// default (source = marker). ComponentParamsDefaults surfaces that marker into
// parser_config["Parser:Shared"]["source"], which lets a test observe which version's
// DSL was used to rebuild parser_config.
func seedGapVersion(t *testing.T, db *gorm.DB, canvasID, versionID, marker string, createMs int64) {
	t.Helper()
	dsl := entity.JSONMap{"components": map[string]any{
		"Parser:Shared": map[string]any{"obj": map[string]any{
			"component_name": "Parser",
			"params":         map[string]any{"source": marker, "k": "x"},
		}},
	}}
	ct := createMs
	if err := db.Create(&entity.UserCanvasVersion{
		ID:           versionID,
		UserCanvasID: canvasID,
		DSL:          dsl,
		BaseModel:    entity.BaseModel{CreateTime: &ct},
	}).Error; err != nil {
		t.Fatalf("seed version: %v", err)
	}
}

// seedBoundKBWithVersion creates a knowledgebase already bound to a canvas + version.
func seedBoundKBWithVersion(t *testing.T, db *gorm.DB, kbID, canvasID, versionID string) {
	t.Helper()
	kb := &entity.Knowledgebase{
		ID:                kbID,
		TenantID:          "tenant-1",
		CreatedBy:         "tenant-1",
		Name:              kbID,
		EmbdID:            "BAAI/bge-large-zh-v1.5@Builtin",
		Permission:        string(entity.TenantPermissionMe),
		ParserID:          string(entity.ParserTypeNaive),
		ParserConfig:      entity.JSONMap{},
		Status:            sptr(string(entity.StatusValid)),
		PipelineID:        &canvasID,
		PipelineVersionID: &versionID,
	}
	if err := db.Create(kb).Error; err != nil {
		t.Fatalf("seed kb: %v", err)
	}
	ensureDatasetTestMembership(t, "tenant-1", "tenant-1", "owner")
}

// B1 (design section 5 fallback chain, T7g): editing parser_config on a KB pinned to a
// non-latest version must rebuild parser_config from the PINNED version DSL, not
// GetLatest. Uses pinned != latest fixture (adversarial).
func TestUpdateDataset_ParserConfigRebuildUsesPinnedVersionDSL(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")

	canvasID := "1234567890abcdef1234567890abcdef"
	// v-old is the pinned version; v-latest is newer (larger create time) so GetLatest != pinned.
	seedGapCanvas(t, db, canvasID)
	seedGapVersion(t, db, canvasID, "v-old", "OLD_MARKER", 100)
	seedGapVersion(t, db, canvasID, "v-latest", "LATEST_MARKER", 200)
	seedBoundKBWithVersion(t, db, "kb-1", canvasID, "v-old")

	// Request omits pipeline_id AND pipeline_version_id; only parser_config changes.
	_, code, err := testDatasetUpdateService(t).UpdateDataset(withDatasetTestContext(t.Context(), "tenant-1", "tenant-1"), "kb-1", "tenant-1",
		service.UpdateDatasetRequest{
			ParserConfig: map[string]interface{}{"Parser:Shared": map[string]interface{}{"k": "override"}},
		})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateDataset err=%v code=%d", err, code)
	}
	var kb entity.Knowledgebase
	if err := db.Where("id = ?", "kb-1").Take(&kb).Error; err != nil {
		t.Fatalf("load kb: %v", err)
	}
	// B4 sticky snapshot: pin must remain the bound version.
	if kb.PipelineVersionID == nil || *kb.PipelineVersionID != "v-old" {
		t.Fatalf("pin changed unexpectedly: %v, want v-old", kb.PipelineVersionID)
	}
	// B1: parser_config must be rebuilt from the PINNED (v-old) DSL, not v-latest.
	comp, ok := kb.ParserConfig["Parser:Shared"].(map[string]any)
	if !ok {
		t.Fatalf("parser_config missing Parser:Shared: %#v", kb.ParserConfig)
	}
	if comp["source"] != "OLD_MARKER" {
		t.Fatalf("parser_config rebuilt from wrong version: source=%v, want OLD_MARKER (pinned DSL)", comp["source"])
	}
}

// B2 (design section 5 canvas-context fallback, T7f): a single-field PATCH that sends ONLY
// pipeline_version_id (omitting pipeline_id) must NOT clear the pin; it must re-pin to
// the requested version using the already-bound canvas as context, and C3 must reset
// parser_config to that version's defaults.
func TestUpdateDataset_SingleFieldVersionPatchDoesNotClearPin(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")

	canvasID := "1234567890abcdef1234567890abcdef"
	seedGapCanvas(t, db, canvasID)
	seedGapVersion(t, db, canvasID, "v-old", "OLD_MARKER", 100)
	seedGapVersion(t, db, canvasID, "v-new", "NEW_MARKER", 200)
	seedBoundKBWithVersion(t, db, "kb-1", canvasID, "v-old")

	// Single-field PATCH: only pipeline_version_id, pipeline_id omitted.
	_, code, err := testDatasetUpdateService(t).UpdateDataset(withDatasetTestContext(t.Context(), "tenant-1", "tenant-1"), "kb-1", "tenant-1",
		service.UpdateDatasetRequest{
			PipelineVersionID: sptr("v-new"),
		})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateDataset err=%v code=%d", err, code)
	}
	var kb entity.Knowledgebase
	if err := db.Where("id = ?", "kb-1").Take(&kb).Error; err != nil {
		t.Fatalf("load kb: %v", err)
	}
	if kb.PipelineVersionID == nil || *kb.PipelineVersionID != "v-new" {
		t.Fatalf("pin cleared/incorrect: %v, want v-new", kb.PipelineVersionID)
	}
	// C3: version switch must reset parser_config to v-new defaults.
	comp, ok := kb.ParserConfig["Parser:Shared"].(map[string]any)
	if !ok {
		t.Fatalf("parser_config missing Parser:Shared after switch: %#v", kb.ParserConfig)
	}
	if comp["source"] != "NEW_MARKER" {
		t.Fatalf("C3 reset used wrong version: source=%v, want NEW_MARKER", comp["source"])
	}
}
