package dataset

import (
	"encoding/json"
	"math"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func testDatasetListService(t *testing.T) *DatasetService {
	t.Helper()

	return &DatasetService{
		kbDAO:       dao.NewKnowledgebaseDAO(),
		documentDAO: dao.NewDocumentDAO(),
		tenantDAO:   dao.NewTenantDAO(),
	}
}

func TestDatasetServiceListDatasetsFiltersByIDs(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Alpha")
	insertDatasetUpdateKB(t, "kb-2", "tenant-1", "Beta")

	ctx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
	data, total, code, err := testDatasetListService(t).ListDatasets(ctx,
		"", "", 1, 30, []dao.OrderTerm{{Column: "create_time", Desc: true}},
		"", nil, "", "tenant-1", []string{"kb-1"}, "",
	)
	if err != nil {
		t.Fatalf("ListDatasets failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if total != 1 || len(data) != 1 {
		t.Fatalf("expected exactly one dataset, got total=%d len=%d", total, len(data))
	}
	if data[0]["id"] != "kb-1" {
		t.Fatalf("expected kb-1, got %#v", data[0]["id"])
	}
	if got, ok := data[0]["keywords_similarity_weight"].(float64); !ok || math.Abs(got-0.7) > 1e-9 {
		t.Fatalf("keywords_similarity_weight = %#v, want 0.7", data[0]["keywords_similarity_weight"])
	}
	if _, exists := data[0]["vector_similarity_weight"]; exists {
		t.Fatal("response must not include vector_similarity_weight")
	}
}

func TestDatasetServiceListDatasetsIDsAccessibleViaTeamTenant(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-team", "owner-1", "Shared")
	if err := dao.DB.Create(&entity.Tenant{ID: "owner-1", Name: sptr("owner"), Status: sptr("1")}).Error; err != nil {
		t.Fatalf("insert owner tenant: %v", err)
	}
	insertDatasetUpdateTeamMember(t, "user-1", "owner-1")
	grantDatasetTestResource(t, "owner-1", "kb", "kb-team", "user-1", "read")
	if err := dao.DB.Model(&entity.Knowledgebase{}).
		Where("id = ?", "kb-team").
		Update("permission", string(entity.TenantPermissionTeam)).Error; err != nil {
		t.Fatalf("update kb permission: %v", err)
	}

	ctx := withDatasetTestContext(t.Context(), "user-1", "owner-1")
	data, total, code, err := testDatasetListService(t).ListDatasets(ctx,
		"", "", 1, 30, []dao.OrderTerm{{Column: "create_time", Desc: true}},
		"", nil, "", "user-1", []string{"kb-team"}, "",
	)
	if err != nil {
		t.Fatalf("ListDatasets failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if total != 1 || len(data) != 1 || data[0]["id"] != "kb-team" {
		t.Fatalf("expected the shared dataset, got total=%d data=%#v", total, data)
	}
}

func TestDatasetServiceListDatasetsAndFiltersUseAuthorizedScope(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-team", "owner-1", "Team")
	insertDatasetUpdateKB(t, "kb-private", "owner-1", "Private")
	insertDatasetUpdateTeamMember(t, "user-1", "owner-1")
	if err := dao.DB.Model(&entity.Knowledgebase{}).
		Where("id = ?", "kb-team").
		Update("permission", string(entity.TenantPermissionTeam)).Error; err != nil {
		t.Fatalf("share team dataset: %v", err)
	}
	grantDatasetTestResource(t, "owner-1", "kb", "kb-team", "user-1", "read")

	ctx := withDatasetTestContext(t.Context(), "user-1", "owner-1")
	data, total, code, err := testDatasetListService(t).ListDatasets(ctx,
		"", "", 1, 30, []dao.OrderTerm{{Column: "create_time", Desc: true}},
		"", []string{"owner-1"}, "", "user-1", nil, "",
	)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListDatasets = (%#v, %d, %v), want success", data, code, err)
	}
	if total != 1 || len(data) != 1 || data[0]["id"] != "kb-team" {
		t.Fatalf("ListDatasets returned %#v (total=%d), want only the shared dataset", data, total)
	}

	filters, code, err := testDatasetListService(t).ListDatasetFilters(ctx, "user-1", "")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListDatasetFilters = (%#v, %v, %v), want success", filters, code, err)
	}
	if filters["total"] != int64(1) {
		t.Fatalf("ListDatasetFilters total = %#v, want one visible dataset", filters["total"])
	}
	owners, ok := filters["filter"].(map[string]interface{})["owner"].([]*entity.DatasetOwnerFilter)
	if !ok || len(owners) != 1 {
		t.Fatalf("ListDatasetFilters owners = %#v, want one visible owner group", filters["filter"])
	}
}

func TestDatasetServiceListDatasetsForCanvasUsesCallerOwnerIntersection(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-caller-only", "user-1", "Caller only")
	insertDatasetUpdateKB(t, "kb-shared", "owner-1", "Shared")
	insertDatasetUpdateKB(t, "kb-owner-only", "owner-1", "Owner only")
	insertDatasetUpdateTeamMember(t, "user-1", "owner-1")
	if err := dao.DB.Model(&entity.Knowledgebase{}).
		Where("id = ?", "kb-shared").
		Update("permission", string(entity.TenantPermissionTeam)).Error; err != nil {
		t.Fatalf("share dataset with tenant: %v", err)
	}
	canvas := entity.UserCanvas{
		ID:         "canvas-shared",
		UserID:     "owner-1",
		Permission: string(entity.TenantPermissionTeam),
		DSL:        entity.JSONMap{},
	}
	if err := dao.DB.Create(&canvas).Error; err != nil {
		t.Fatalf("create shared canvas: %v", err)
	}
	grantDatasetTestResource(t, "owner-1", "kb", "kb-shared", "user-1", "read")
	grantDatasetTestResource(t, "owner-1", "canvas", "canvas-shared", "user-1", "read")

	service := testDatasetListService(t)
	data, total, code, err := service.ListDatasets(withDatasetTestContext(t.Context(), "user-1", "owner-1"), "", "", 1, 30,
		[]dao.OrderTerm{{Column: "create_time", Desc: true}}, "", nil, "", "user-1", nil, "canvas-shared")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListDatasets(canvas) = (%#v, %d, %v), want success", data, code, err)
	}
	if total != 1 || len(data) != 1 || data[0]["id"] != "kb-shared" {
		t.Fatalf("canvas dataset list = %#v (total=%d), want only kb-shared", data, total)
	}

	data, total, code, err = service.ListDatasets(withDatasetTestContext(t.Context(), "user-1", "owner-1"), "", "", 1, 30,
		[]dao.OrderTerm{{Column: "create_time", Desc: true}}, "", nil, "", "user-1",
		[]string{"kb-caller-only", "kb-shared", "kb-owner-only"}, "canvas-shared")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListDatasets(canvas, ids) = (%#v, %d, %v), want success", data, code, err)
	}
	if total != 1 || len(data) != 1 || data[0]["id"] != "kb-shared" {
		t.Fatalf("canvas dataset lookup = %#v (total=%d), want only kb-shared", data, total)
	}
}

func TestDatasetServiceListDatasetsPaginatesInsidePermissionScope(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-own", "owner-1", "Own")
	insertDatasetUpdateKB(t, "kb-team", "owner-1", "Shared")
	insertDatasetUpdateKB(t, "kb-private", "owner-1", "Private")
	insertDatasetUpdateKB(t, "kb-outside", "other-owner", "Outside")
	insertDatasetUpdateTeamMember(t, "user-1", "owner-1")
	if err := dao.DB.Model(&entity.Knowledgebase{}).
		Where("id = ?", "kb-own").
		Update("permission", string(entity.TenantPermissionTeam)).Error; err != nil {
		t.Fatalf("share first dataset with tenant: %v", err)
	}
	if err := dao.DB.Model(&entity.Knowledgebase{}).
		Where("id = ?", "kb-team").
		Update("permission", string(entity.TenantPermissionTeam)).Error; err != nil {
		t.Fatalf("share team dataset: %v", err)
	}
	grantDatasetTestResource(t, "owner-1", "kb", "kb-own", "user-1", "read")
	grantDatasetTestResource(t, "owner-1", "kb", "kb-team", "user-1", "read")

	service := testDatasetListService(t)
	terms := []dao.OrderTerm{{Column: "name"}}
	pageOne, total, code, err := service.ListDatasets(withDatasetTestContext(t.Context(), "user-1", "owner-1"), "", "", 1, 1, terms, "", nil, "", "user-1", nil, "")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListDatasets(page 1) = (%#v, %d, %v), want success", pageOne, code, err)
	}
	if total != 2 || len(pageOne) != 1 || pageOne[0]["id"] != "kb-own" {
		t.Fatalf("page 1 = %#v (total=%d), want kb-own and two accessible datasets", pageOne, total)
	}

	pageTwo, secondTotal, code, err := service.ListDatasets(withDatasetTestContext(t.Context(), "user-1", "owner-1"), "", "", 2, 1, terms, "", nil, "", "user-1", nil, "")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListDatasets(page 2) = (%#v, %d, %v), want success", pageTwo, code, err)
	}
	if secondTotal != 2 || len(pageTwo) != 1 || pageTwo[0]["id"] != "kb-team" {
		t.Fatalf("page 2 = %#v (total=%d), want kb-team and two accessible datasets", pageTwo, secondTotal)
	}
}

func TestDatasetServiceListDatasetFiltersReturnsEmptyOwnerList(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	ensureDatasetTestMembership(t, "user-without-datasets", "user-without-datasets", "owner")

	filters, code, err := testDatasetListService(t).ListDatasetFilters(withDatasetTestContext(t.Context(), "user-without-datasets", "user-without-datasets"), "user-without-datasets", "")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListDatasetFilters = (%#v, %v, %v), want success", filters, code, err)
	}

	owners, ok := filters["filter"].(map[string]interface{})["owner"].([]*entity.DatasetOwnerFilter)
	if !ok || owners == nil || len(owners) != 0 {
		t.Fatalf("ListDatasetFilters owners = %#v, want a non-nil empty slice", filters["filter"])
	}

	encoded, err := json.Marshal(filters)
	if err != nil {
		t.Fatalf("marshal filters: %v", err)
	}
	var response map[string]interface{}
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatalf("unmarshal filters: %v", err)
	}
	ownerJSON := response["filter"].(map[string]interface{})["owner"]
	if ownerJSON == nil {
		t.Fatal(`serialized owner is null; want []`)
	}
	if ownerList, ok := ownerJSON.([]interface{}); !ok || len(ownerList) != 0 {
		t.Fatalf("serialized owner = %#v, want []", ownerJSON)
	}
}

func TestDatasetServiceListDatasetsRejectsIDAndIDsTogether(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Alpha")

	ctx := withDatasetTestContext(t.Context(), "user-1", "tenant-1")
	_, _, code, err := testDatasetListService(t).ListDatasets(ctx,
		"kb-1", "", 1, 30, []dao.OrderTerm{{Column: "create_time", Desc: true}},
		"", nil, "", "tenant-1", []string{"kb-1"}, "",
	)
	if err == nil {
		t.Fatal("expected id/ids conflict error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	expected := "should not provide both 'id':kb-1 and 'ids':['kb-1']"
	if err.Error() != expected {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetServiceListDatasetsFiltersInaccessibleIDs(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-valid", "user-1", "Valid")
	insertDatasetUpdateKB(t, "kb-private", "owner-1", "Private")

	ctx := withDatasetTestContext(t.Context(), "user-1", "user-1")
	data, total, code, err := testDatasetListService(t).ListDatasets(ctx,
		"", "", 1, 30, []dao.OrderTerm{{Column: "create_time", Desc: true}},
		"", nil, "", "user-1", []string{"kb-valid", "kb-private", "kb-missing"}, "",
	)
	if err != nil {
		t.Fatalf("ListDatasets failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if total != 1 || len(data) != 1 || data[0]["id"] != "kb-valid" {
		t.Fatalf("expected only the valid dataset, got total=%d data=%#v", total, data)
	}
}

func TestDatasetServiceListDatasetsReturnsEmptyForAllInaccessibleIDs(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-private", "owner-1", "Private")

	ctx := withDatasetTestContext(t.Context(), "user-1", "user-1")
	data, total, code, err := testDatasetListService(t).ListDatasets(ctx,
		"", "", 1, 30, []dao.OrderTerm{{Column: "create_time", Desc: true}},
		"", nil, "", "user-1", []string{"kb-private", "kb-missing"}, "",
	)
	if err != nil {
		t.Fatalf("ListDatasets failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if data == nil || len(data) != 0 || total != 0 {
		t.Fatalf("expected a non-nil empty result, got total=%d data=%#v", total, data)
	}
}
