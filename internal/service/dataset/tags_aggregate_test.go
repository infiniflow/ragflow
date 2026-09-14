package dataset

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func testDatasetServiceForAggregateTags(t *testing.T, loader func(ctx context.Context, tagFileID string) (map[string]int, error)) *DatasetService {
	t.Helper()
	return &DatasetService{
		kbDAO:               dao.NewKnowledgebaseDAO(),
		tagVocabularyLoader: loader,
	}
}

func insertAggregateTagsKB(t *testing.T, datasetID, tenantID, permission, tagFileID string, docNum int64) {
	t.Helper()
	parserConfig := entity.JSONMap{}
	if tagFileID != "" {
		parserConfig["tags"] = map[string]any{"tag_file_id": tagFileID}
	}
	kb := &entity.Knowledgebase{
		ID:           datasetID,
		TenantID:     tenantID,
		Name:         "kb-" + datasetID[:6],
		EmbdID:       "embedding@OpenAI",
		CreatedBy:    tenantID,
		Permission:   permission,
		ParserID:     "naive",
		ParserConfig: parserConfig,
		DocNum:       docNum,
		Status:       sptr(string(entity.StatusValid)),
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert test kb: %v", err)
	}
}

func insertAggregateTagsMembership(t *testing.T, tenantID, userID string) {
	t.Helper()
	row := &entity.UserTenant{
		ID:        tenantID + "-" + userID,
		UserID:    userID,
		TenantID:  tenantID,
		Role:      "member",
		InvitedBy: tenantID,
		Status:    sptr(string(entity.StatusValid)),
	}
	if err := dao.DB.Create(row).Error; err != nil {
		t.Fatalf("insert user_tenant: %v", err)
	}
}

func aggregateTagsResultMap(rows []map[string]interface{}) map[string]int {
	result := make(map[string]int, len(rows))
	for _, row := range rows {
		tag, _ := row["value"].(string)
		count, _ := row["count"].(int)
		result[tag] = count
	}
	return result
}

// TestDatasetServiceAggregateTagsSeedsFromTagFile verifies that the tag
// vocabulary (and its counts) come solely from the configured tag source file,
// with no chunk-level aggregation.
func TestDatasetServiceAggregateTagsSeedsFromTagFile(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "123e4567-e89b-12d3-a456-426614174000"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertAggregateTagsKB(t, kbID, "user-1", string(entity.TenantPermissionMe), "file-1", 0)

	loader := func(ctx context.Context, tagFileID string) (map[string]int, error) {
		if tagFileID == "file-1" {
			return map[string]int{"finance": 2, "urgent": 1}, nil
		}
		return nil, nil
	}
	ctx := t.Context()

	result, code, err := testDatasetServiceForAggregateTags(t, loader).AggregateTags(ctx, []string{kbInput}, "user-1")
	if err != nil {
		t.Fatalf("AggregateTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}

	got := aggregateTagsResultMap(result)
	want := map[string]int{"finance": 2, "urgent": 1}
	if len(got) != len(want) {
		t.Fatalf("result len=%d want=%d result=%v", len(got), len(want), got)
	}
	for tag, wantCount := range want {
		if got[tag] != wantCount {
			t.Fatalf("tag %q count=%d want=%d all=%v", tag, got[tag], wantCount, got)
		}
	}
}

// TestDatasetServiceAggregateTagsMergesAcrossTagFiles verifies that counts from
// multiple datasets' tag source files are merged by tag name.
func TestDatasetServiceAggregateTagsMergesAcrossTagFiles(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kb1Input := "123e4567-e89b-12d3-a456-426614174000"
	kb2Input := "223e4567-e89b-12d3-a456-426614174001"
	kb1ID := strings.ReplaceAll(kb1Input, "-", "")
	kb2ID := strings.ReplaceAll(kb2Input, "-", "")
	insertAggregateTagsKB(t, kb1ID, "user-1", string(entity.TenantPermissionMe), "file-1", 0)
	insertAggregateTagsKB(t, kb2ID, "user-1", string(entity.TenantPermissionMe), "file-2", 0)

	loader := func(ctx context.Context, tagFileID string) (map[string]int, error) {
		switch tagFileID {
		case "file-1":
			return map[string]int{"finance": 2, "urgent": 1}, nil
		case "file-2":
			return map[string]int{"finance": 1, "urgent": 3, "internal": 1}, nil
		}
		return nil, nil
	}
	ctx := t.Context()

	result, code, err := testDatasetServiceForAggregateTags(t, loader).AggregateTags(ctx, []string{kb1Input, kb2Input}, "user-1")
	if err != nil {
		t.Fatalf("AggregateTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}

	got := aggregateTagsResultMap(result)
	want := map[string]int{"finance": 3, "urgent": 4, "internal": 1}
	if len(got) != len(want) {
		t.Fatalf("result len=%d want=%d result=%v", len(got), len(want), got)
	}
	for tag, wantCount := range want {
		if got[tag] != wantCount {
			t.Fatalf("tag %q count=%d want=%d all=%v", tag, got[tag], wantCount, got)
		}
	}
}

// TestDatasetServiceAggregateTagsEmptyWhenNoTagFile verifies that a dataset
// without a configured tag source file yields an empty vocabulary and does not
// invoke the loader.
func TestDatasetServiceAggregateTagsEmptyWhenNoTagFile(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "323e4567-e89b-12d3-a456-426614174002"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertAggregateTagsKB(t, kbID, "user-1", string(entity.TenantPermissionMe), "", 0)

	loader := func(ctx context.Context, tagFileID string) (map[string]int, error) {
		t.Fatalf("loader should not be called when no tag_file_id is set")
		return nil, nil
	}
	ctx := t.Context()

	result, code, err := testDatasetServiceForAggregateTags(t, loader).AggregateTags(ctx, []string{kbInput}, "user-1")
	if err != nil {
		t.Fatalf("AggregateTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}
	if len(result) != 0 {
		t.Fatalf("result=%v want empty", result)
	}
}

func TestDatasetServiceAggregateTagsRejectsUnauthorizedDataset(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "423e4567-e89b-12d3-a456-426614174003"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertAggregateTagsKB(t, kbID, "tenant-9", string(entity.TenantPermissionMe), "", 0)
	ctx := t.Context()

	_, code, err := testDatasetServiceForAggregateTags(t, nil).AggregateTags(ctx, []string{kbInput}, "user-1")
	if err == nil {
		t.Fatal("expected authorization error")
	}
	if code != common.CodeDataError {
		t.Fatalf("code=%d want=%d", code, common.CodeDataError)
	}
	if err.Error() != "No authorization for dataset '"+kbID+"'" {
		t.Fatalf("unexpected error: %v", err)
	}
}
