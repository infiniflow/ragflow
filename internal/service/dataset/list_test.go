package dataset

import (
	"strings"
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

	ctx := t.Context()
	data, total, code, err := testDatasetListService(t).ListDatasets(ctx,
		"", "", 1, 30, "create_time", true,
		"", nil, "", "tenant-1", []string{"kb-1"},
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
}

func TestDatasetServiceListDatasetsIDsAccessibleViaTeamTenant(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-team", "owner-1", "Shared")
	if err := dao.DB.Create(&entity.Tenant{ID: "owner-1", Name: sptr("owner"), Status: sptr("1")}).Error; err != nil {
		t.Fatalf("insert owner tenant: %v", err)
	}
	insertDatasetUpdateTeamMember(t, "user-1", "owner-1")
	if err := dao.DB.Model(&entity.Knowledgebase{}).
		Where("id = ?", "kb-team").
		Update("permission", string(entity.TenantPermissionTeam)).Error; err != nil {
		t.Fatalf("update kb permission: %v", err)
	}

	ctx := t.Context()
	data, total, code, err := testDatasetListService(t).ListDatasets(ctx,
		"", "", 1, 30, "create_time", true,
		"", nil, "", "user-1", []string{"kb-team"},
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

func TestDatasetServiceListDatasetsRejectsIDAndIDsTogether(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Alpha")

	ctx := t.Context()
	_, _, code, err := testDatasetListService(t).ListDatasets(ctx,
		"kb-1", "", 1, 30, "create_time", true,
		"", nil, "", "tenant-1", []string{"kb-1"},
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

func TestDatasetServiceListDatasetsRejectsDeniedIDs(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-private", "owner-1", "Private")

	ctx := t.Context()
	_, _, code, err := testDatasetListService(t).ListDatasets(ctx,
		"", "", 1, 30, "create_time", true,
		"", nil, "", "user-1", []string{"kb-private"},
	)
	if err == nil {
		t.Fatal("expected permission error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	expected := "user 'user-1' lacks permission for datasets: 'kb-private'"
	if !strings.Contains(err.Error(), expected) {
		t.Fatalf("unexpected error: %v", err)
	}
}
