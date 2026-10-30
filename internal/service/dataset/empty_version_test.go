package dataset

import (
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// T39 (design C5/G3): creating a dataset in Pipeline (dataflow canvas) mode with
// no concrete pinned version must be REJECTED with ErrPipelineVersionEmpty. The
// empty version is no longer auto-backfilled to the canvas latest; the Save fails
// so the UI can show the red Missing badge and the "版本信息缺失" error. The KB
// must NOT be persisted.
func TestCreateDataset_EmptyVersionRejected(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")
	ctx := t.Context()

	canvasID := "0123456789abcdef0123456789abcdef"
	seedDataflowCanvas(t, db, canvasID)
	seedCanvasVersion(t, db, canvasID, "version0000000000000000000000aa", 0)

	parseType := 2
	result, code, err := testDatasetCreateService(t).CreateDataset(ctx, &service.CreateDatasetRequest{
		Name:       "ds-empty-version",
		PipelineID: &canvasID,
		ParseType:  &parseType,
		// PipelineVersionID omitted on purpose (empty version)
	}, testDatasetSubject("tenant-1"))
	if err == nil {
		t.Fatalf("expected rejection of empty pipeline version, got result=%v code=%d", result, code)
	}
	if code != common.CodeBadRequest {
		t.Fatalf("expected CodeBadRequest, got code=%d err=%v", code, err)
	}
	if !errors.Is(err, service.ErrPipelineVersionEmpty) {
		t.Fatalf("expected ErrPipelineVersionEmpty, got: %v", err)
	}
	var count int64
	if err := db.Model(&entity.Knowledgebase{}).Where("name = ?", "ds-empty-version").Count(&count).Error; err != nil {
		t.Fatalf("count kb: %v", err)
	}
	if count != 0 {
		t.Fatalf("empty-version create must not persist a KB, got count=%d", count)
	}
}

// T39 adversarial: an explicit EMPTY string version (not just omitted) is also
// rejected in Pipeline mode.
func TestCreateDataset_ExplicitEmptyVersionStringRejected(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")
	ctx := t.Context()

	canvasID := "0123456789abcdef0123456789abcdef"
	seedDataflowCanvas(t, db, canvasID)
	seedCanvasVersion(t, db, canvasID, "version0000000000000000000000aa", 0)

	parseType := 2
	empty := ""
	_, code, err := testDatasetCreateService(t).CreateDataset(ctx, &service.CreateDatasetRequest{
		Name:              "ds-empty-version-str",
		PipelineID:        &canvasID,
		PipelineVersionID: &empty,
		ParseType:         &parseType,
	}, testDatasetSubject("tenant-1"))
	if err == nil || code == common.CodeSuccess {
		t.Fatalf("expected rejection of explicit empty version, got code=%d err=%v", code, err)
	}
	if !errors.Is(err, service.ErrPipelineVersionEmpty) {
		t.Fatalf("expected ErrPipelineVersionEmpty, got: %v", err)
	}
}

// T39 adversarial (built-in exempt): BuiltIn mode must NOT require a version.
// An empty version in BuiltIn mode is allowed and the dataset is created.
func TestCreateDataset_BuiltinEmptyVersionAllowed(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertCreateDatasetTenant(t, "tenant-1")
	ctx := t.Context()

	parseType := 1
	general := "general"
	result, code, err := testDatasetCreateService(t).CreateDataset(ctx, &service.CreateDatasetRequest{
		Name:      "ds-builtin-no-version",
		ParserID:  &general,
		ParseType: &parseType,
	}, testDatasetSubject("tenant-1"))
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("expected BuiltIn create without version to succeed, got code=%d err=%v", code, err)
	}
	if result["id"] == nil {
		t.Fatalf("expected created dataset id, got %v", result)
	}
}

// T40 (dataset layer, design C5/G3): updating an existing Pipeline-mode dataset
// with an explicit empty version is rejected (it must not silently clear the
// pin). Used to fail with ErrPipelineVersionEmpty.
func TestUpdateDataset_ExplicitEmptyVersionRejected(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)

	canvas1 := "0123456789abcdef0123456789abcdef"
	ver1 := "version0000000000000000000000aa"
	seedDataflowCanvasWithVersion(t, db, canvas1, ver1)
	seedBoundKB(t, db, "kb-empty-upd", canvas1, ver1)

	parseType := 2
	empty := ""
	_, code, err := testDatasetUpdateService(t).UpdateDataset(withDatasetTestContext(t.Context(), "tenant-1", "tenant-1"), "kb-empty-upd", "tenant-1", service.UpdateDatasetRequest{
		PipelineID:        &canvas1,
		ParseType:         &parseType,
		PipelineVersionID: &empty,
	})
	if err == nil || code == common.CodeSuccess {
		t.Fatalf("expected rejection of explicit empty version on update, got code=%d err=%v", code, err)
	}
	if !errors.Is(err, service.ErrPipelineVersionEmpty) {
		t.Fatalf("expected ErrPipelineVersionEmpty, got: %v", err)
	}
	var kb entity.Knowledgebase
	if err := db.Where("id = ?", "kb-empty-upd").Take(&kb).Error; err != nil {
		t.Fatalf("load kb: %v", err)
	}
	if kb.PipelineVersionID == nil || *kb.PipelineVersionID != ver1 {
		t.Fatalf("pin must be preserved on rejected update, got %v", kb.PipelineVersionID)
	}
}

// T40 adversarial: switching a BuiltIn dataset to a new Pipeline canvas WITHOUT
// supplying a version is rejected (no GetLatest auto-backfill for empty version).
func TestUpdateDataset_PipelineChangeWithoutVersionRejected(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)

	canvas1 := "0123456789abcdef0123456789abcdef"
	seedDataflowCanvasWithVersion(t, db, canvas1, "version0000000000000000000000aa")
	insertDatasetUpdateKB(t, "kb-switch", "tenant-1", "Original") // builtin, no pipeline bound

	parseType := 2
	_, code, err := testDatasetUpdateService(t).UpdateDataset(withDatasetTestContext(t.Context(), "tenant-1", "tenant-1"), "kb-switch", "tenant-1", service.UpdateDatasetRequest{
		PipelineID: &canvas1,
		ParseType:  &parseType,
	})
	if err == nil || code == common.CodeSuccess {
		t.Fatalf("expected rejection of pipeline switch without version, got code=%d err=%v", code, err)
	}
	if !errors.Is(err, service.ErrPipelineVersionEmpty) {
		t.Fatalf("expected ErrPipelineVersionEmpty, got: %v", err)
	}
}

// T40 control: a partial update that omits the pipeline selection entirely must
// PRESERVE the existing pinned version (no error). The empty-version rule only
// fires when the effective mode is Pipeline with no concrete version.
func TestUpdateDataset_PreservePinnedVersionOnPartialUpdate(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)

	canvas1 := "0123456789abcdef0123456789abcdef"
	ver1 := "version0000000000000000000000aa"
	seedDataflowCanvasWithVersion(t, db, canvas1, ver1)
	seedBoundKB(t, db, "kb-preserve", canvas1, ver1)

	name := "renamed-partial"
	_, code, err := testDatasetUpdateService(t).UpdateDataset(withDatasetTestContext(t.Context(), "tenant-1", "tenant-1"), "kb-preserve", "tenant-1", service.UpdateDatasetRequest{
		Name: &name,
	})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("expected partial update (preserve pin) to succeed, got code=%d err=%v", code, err)
	}
	var kb entity.Knowledgebase
	if err := db.Where("id = ?", "kb-preserve").Take(&kb).Error; err != nil {
		t.Fatalf("load kb: %v", err)
	}
	if kb.PipelineVersionID == nil || *kb.PipelineVersionID != ver1 {
		t.Fatalf("pin must be preserved, got %v", kb.PipelineVersionID)
	}
}
